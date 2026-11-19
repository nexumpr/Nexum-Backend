package transport_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// Pruebas HTTP de E027/E028: contrato exacto, autenticación, idempotencia, precondiciones y
// fail-closed. Los dobles son exclusivamente de prueba; la composición productiva usa DynamoDB.

const (
	movHTTPID     = "018f90ef-08e5-7a68-b09f-000000000001"
	movHTTPClave  = "clave-sintetica-de-prueba-0001"
	movHTTPCursor = "clave-de-cursores-de-prueba-e027"
	movHTTPCuerpo = `{"type":"expense","amount":"1000.00","currency":"MXN","categoryId":"food","description":"Ejemplo de concepto","occurredAt":"2026-11-05T17:00:00Z"}`
)

// movHTTPStore implementa movements.Store para el transporte.
type movHTTPStore struct {
	mu       sync.Mutex
	creates  []appMovements.CreateWrite
	createFn func(appMovements.CreateWrite) (*appMovements.CreateResult, error)
	listFn   func(appMovements.ListQuery, string, int) ([]appMovements.Movement, string, error)
	queries  []appMovements.ListQuery
}

func (m *movHTTPStore) CreateMovement(_ context.Context, write appMovements.CreateWrite) (*appMovements.CreateResult, error) {
	m.mu.Lock()
	m.creates = append(m.creates, write)
	fn := m.createFn
	m.mu.Unlock()
	if fn != nil {
		return fn(write)
	}
	return &appMovements.CreateResult{Status: write.Status, Body: write.ResponseBody, Location: write.Location}, nil
}

func (m *movHTTPStore) ListMovements(_ context.Context, query appMovements.ListQuery, startSK string, limit int) ([]appMovements.Movement, string, error) {
	m.mu.Lock()
	m.queries = append(m.queries, query)
	fn := m.listFn
	m.mu.Unlock()
	if fn != nil {
		return fn(query, startSK, limit)
	}
	return nil, "", nil
}

func movHTTPHandler(store appMovements.Store) *internalhttp.MovementsHandler {
	var servicio *appMovements.Service
	if store != nil {
		servicio = appMovements.NewService(store).
			WithClock(func() time.Time { return e004HTTPAhora }).
			WithIDGenerator(func() string { return movHTTPID }).
			WithCursorKey([]byte(movHTTPCursor))
	}
	return internalhttp.NewMovementsHandler(servicio, e004TokensValidos(), newE004Sesiones(), nil, silencioso())
}

// movHTTPPeticion construye la petición autenticada con la cabecera de idempotencia opcional.
func movHTTPPeticion(metodo, ruta, token, cuerpo, clave string) *http.Request {
	peticion := e004Peticion(metodo, ruta, token, cuerpo)
	if clave != "" {
		peticion.Header.Set(internalhttp.HeaderIdempotencyKey, clave)
	}
	return peticion
}

func movHTTPEjecutar(handler http.Handler, peticion *http.Request) *httptest.ResponseRecorder {
	grabador := httptest.NewRecorder()
	handler.ServeHTTP(grabador, peticion)
	return grabador
}

func movHTTPMovimiento(id string, ocurrido time.Time) appMovements.Movement {
	return appMovements.Movement{
		ID: id, Type: "expense", AmountCents: 100000, Currency: "MXN",
		CategoryID: "food", Description: "Ejemplo de concepto", OccurredAt: ocurrido, Version: 1,
	}
}

// TestE028HTTP201 cubre T01: código, esquema exacto, Location y cabeceras de información privada.
func TestE028HTTP201(t *testing.T) {
	store := &movHTTPStore{}
	grabador := movHTTPEjecutar(movHTTPHandler(store), movHTTPPeticion(http.MethodPost, "/api/v1/movements", "jwt-valido", movHTTPCuerpo, movHTTPClave))

	if grabador.Code != http.StatusCreated {
		t.Fatalf("se esperaba 201 y se obtuvo %d (%s)", grabador.Code, grabador.Body.String())
	}
	if location := grabador.Header().Get("Location"); location != "/api/v1/movements/"+movHTTPID {
		t.Fatalf("Location inesperado: %q", location)
	}
	if cache := grabador.Header().Get("Cache-Control"); cache != "private, no-store" {
		t.Fatalf("Cache-Control inesperado: %q", cache)
	}
	if grabador.Header().Get("X-Idempotent-Replay") != "" {
		t.Fatal("un alta nueva no debe marcarse como replay")
	}

	var cuerpo map[string]json.RawMessage
	if err := json.Unmarshal(grabador.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("cuerpo no deserializable: %v", err)
	}
	esperados := []string{"id", "type", "amount", "currency", "categoryId", "description", "occurredAt", "version", "contributionId", "recurringPaymentId"}
	if len(cuerpo) != len(esperados) {
		t.Fatalf("campos inesperados en la respuesta: %v", cuerpo)
	}
	for _, campo := range esperados {
		if _, ok := cuerpo[campo]; !ok {
			t.Fatalf("falta el campo %q", campo)
		}
	}
	texto := grabador.Body.String()
	for _, prohibido := range []string{"payloadHash", "userId", "GSI1PK", "targetSK", "responseBody"} {
		if strings.Contains(texto, prohibido) {
			t.Fatalf("la respuesta no debe exponer %q: %s", prohibido, texto)
		}
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.creates) != 1 {
		t.Fatalf("se esperaba una escritura y hubo %d", len(store.creates))
	}
	if store.creates[0].UserID != e004HTTPUsuario {
		t.Fatalf("el titular debe venir del token, se obtuvo %q", store.creates[0].UserID)
	}
	if store.creates[0].IdempotencyKey != movHTTPClave {
		t.Fatalf("la clave de idempotencia no se propago: %q", store.creates[0].IdempotencyKey)
	}
}

// TestE028HTTPReplay comprueba que el replay devuelve el mismo cuerpo y la cabecera que lo
// identifica, sin volver a escribir.
func TestE028HTTPReplay(t *testing.T) {
	movimiento := movHTTPMovimiento(movHTTPID, e004HTTPAhora.Add(-time.Hour))
	cuerpo, err := appMovements.MarshalMovement(movimiento)
	if err != nil {
		t.Fatalf("cuerpo de prueba no serializable: %v", err)
	}
	store := &movHTTPStore{createFn: func(appMovements.CreateWrite) (*appMovements.CreateResult, error) {
		return &appMovements.CreateResult{Status: appMovements.StatusCreated, Body: cuerpo, Location: "/api/v1/movements/" + movHTTPID, Replayed: true}, nil
	}}

	grabador := movHTTPEjecutar(movHTTPHandler(store), movHTTPPeticion(http.MethodPost, "/api/v1/movements", "jwt-valido", movHTTPCuerpo, movHTTPClave))
	if grabador.Code != http.StatusCreated {
		t.Fatalf("se esperaba 201 en el replay y se obtuvo %d", grabador.Code)
	}
	if grabador.Header().Get("X-Idempotent-Replay") != "true" {
		t.Fatal("el replay debe anunciarse con X-Idempotent-Replay")
	}
	if grabador.Body.String() != string(cuerpo) {
		t.Fatalf("el cuerpo del replay debe ser identico al original: %s", grabador.Body.String())
	}
}

// TestE028HTTPAutenticacion cubre T03: sin credenciales válidas no se escribe nada.
func TestE028HTTPAutenticacion(t *testing.T) {
	casos := []struct {
		nombre string
		token  string
	}{
		{"sin cabecera", ""},
		{"token desconocido", "desconocido"},
		{"token expirado", "jwt-expirado"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := &movHTTPStore{}
			grabador := movHTTPEjecutar(movHTTPHandler(store), movHTTPPeticion(http.MethodPost, "/api/v1/movements", caso.token, movHTTPCuerpo, movHTTPClave))
			if grabador.Code != http.StatusUnauthorized {
				t.Fatalf("se esperaba 401 y se obtuvo %d", grabador.Code)
			}
			store.mu.Lock()
			defer store.mu.Unlock()
			if len(store.creates) != 0 {
				t.Fatal("una peticion no autenticada no puede llegar al almacen")
			}
		})
	}
}

// TestE028HTTPIdempotencyKeyObligatoria cubre la cabecera requerida del contrato.
func TestE028HTTPIdempotencyKeyObligatoria(t *testing.T) {
	for _, clave := range []string{"", "corta"} {
		store := &movHTTPStore{}
		grabador := movHTTPEjecutar(movHTTPHandler(store), movHTTPPeticion(http.MethodPost, "/api/v1/movements", "jwt-valido", movHTTPCuerpo, clave))
		if grabador.Code != http.StatusBadRequest {
			t.Fatalf("clave %q: se esperaba 400 y se obtuvo %d", clave, grabador.Code)
		}
		if !strings.Contains(grabador.Body.String(), "Idempotency-Key") {
			t.Fatalf("el detalle debe senalar la cabecera: %s", grabador.Body.String())
		}
		store.mu.Lock()
		if len(store.creates) != 0 {
			store.mu.Unlock()
			t.Fatal("sin clave de idempotencia no se escribe")
		}
		store.mu.Unlock()
	}
}

// TestE028HTTPCuerpoInvalido cubre T02: JSON malformado, campo desconocido y tipo incorrecto.
func TestE028HTTPCuerpoInvalido(t *testing.T) {
	casos := []struct {
		nombre string
		cuerpo string
	}{
		{"json malformado", `{"type":`},
		{"campo desconocido", `{"type":"expense","amount":"1000.00","currency":"MXN","categoryId":"food","description":"Ejemplo","occurredAt":"2026-11-05T17:00:00Z","extra":1}`},
		{"importe como numero", `{"type":"expense","amount":1000.00,"currency":"MXN","categoryId":"food","description":"Ejemplo","occurredAt":"2026-11-05T17:00:00Z"}`},
		{"tipo no editable", `{"type":"income","amount":"1000.00","currency":"MXN","categoryId":"food","description":"Ejemplo","occurredAt":"2026-11-05T17:00:00Z","version":9}`},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := &movHTTPStore{}
			grabador := movHTTPEjecutar(movHTTPHandler(store), movHTTPPeticion(http.MethodPost, "/api/v1/movements", "jwt-valido", caso.cuerpo, movHTTPClave))
			if grabador.Code != http.StatusBadRequest {
				t.Fatalf("se esperaba 400 y se obtuvo %d (%s)", grabador.Code, grabador.Body.String())
			}
			store.mu.Lock()
			defer store.mu.Unlock()
			if len(store.creates) != 0 {
				t.Fatal("un cuerpo invalido no puede llegar al almacen")
			}
		})
	}
}

// TestE028HTTP422 cubre las reglas de negocio del alta.
func TestE028HTTP422(t *testing.T) {
	casos := []struct {
		nombre string
		cuerpo string
		campo  string
	}{
		{"tipo fuera del enum", `{"type":"transfer","amount":"1000.00","currency":"MXN","categoryId":"food","description":"Ejemplo","occurredAt":"2026-11-05T17:00:00Z"}`, "type"},
		{"importe cero", `{"type":"expense","amount":"0.00","currency":"MXN","categoryId":"food","description":"Ejemplo","occurredAt":"2026-11-05T17:00:00Z"}`, "amount"},
		{"divisa no admitida", `{"type":"expense","amount":"1000.00","currency":"USD","categoryId":"food","description":"Ejemplo","occurredAt":"2026-11-05T17:00:00Z"}`, "currency"},
		{"descripcion vacia", `{"type":"expense","amount":"1000.00","currency":"MXN","categoryId":"food","description":"  ","occurredAt":"2026-11-05T17:00:00Z"}`, "description"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := &movHTTPStore{}
			grabador := movHTTPEjecutar(movHTTPHandler(store), movHTTPPeticion(http.MethodPost, "/api/v1/movements", "jwt-valido", caso.cuerpo, movHTTPClave))
			if grabador.Code != http.StatusUnprocessableEntity {
				t.Fatalf("se esperaba 422 y se obtuvo %d (%s)", grabador.Code, grabador.Body.String())
			}
			if !strings.Contains(grabador.Body.String(), caso.campo) {
				t.Fatalf("el detalle debe nombrar el campo %q: %s", caso.campo, grabador.Body.String())
			}
		})
	}
}

// TestE028HTTP409Y503 cubre la traducción de conflictos y de dependencia no disponible.
func TestE028HTTP409Y503(t *testing.T) {
	conflicto := &movHTTPStore{createFn: func(appMovements.CreateWrite) (*appMovements.CreateResult, error) {
		return nil, appMovements.ErrConflict
	}}
	if grabador := movHTTPEjecutar(movHTTPHandler(conflicto), movHTTPPeticion(http.MethodPost, "/api/v1/movements", "jwt-valido", movHTTPCuerpo, movHTTPClave)); grabador.Code != http.StatusConflict {
		t.Fatalf("se esperaba 409 y se obtuvo %d", grabador.Code)
	}

	caido := &movHTTPStore{createFn: func(appMovements.CreateWrite) (*appMovements.CreateResult, error) {
		return nil, appMovements.ErrUnavailable
	}}
	if grabador := movHTTPEjecutar(movHTTPHandler(caido), movHTTPPeticion(http.MethodPost, "/api/v1/movements", "jwt-valido", movHTTPCuerpo, movHTTPClave)); grabador.Code != http.StatusServiceUnavailable {
		t.Fatalf("se esperaba 503 y se obtuvo %d", grabador.Code)
	}

	// Sin servicio compuesto (sin persistencia) el endpoint falla cerrado.
	sinServicio := internalhttp.NewMovementsHandler(nil, e004TokensValidos(), newE004Sesiones(), nil, silencioso())
	if grabador := movHTTPEjecutar(sinServicio, movHTTPPeticion(http.MethodPost, "/api/v1/movements", "jwt-valido", movHTTPCuerpo, movHTTPClave)); grabador.Code != http.StatusServiceUnavailable {
		t.Fatalf("se esperaba 503 sin servicio y se obtuvo %d", grabador.Code)
	}
}

// TestE027HTTPListado cubre T01 del listado: 200, `items` siempre presente y `nextCursor` sólo
// cuando hay más.
func TestE027HTTPListado(t *testing.T) {
	store := &movHTTPStore{listFn: func(_ appMovements.ListQuery, _ string, _ int) ([]appMovements.Movement, string, error) {
		return []appMovements.Movement{movHTTPMovimiento(movHTTPID, e004HTTPAhora.Add(-time.Hour))}, "MOVEMENT#2026-11-15T11:00:00.000Z#" + movHTTPID, nil
	}}
	grabador := movHTTPEjecutar(movHTTPHandler(store), movHTTPPeticion(http.MethodGet, "/api/v1/movements", "jwt-valido", "", ""))

	if grabador.Code != http.StatusOK {
		t.Fatalf("se esperaba 200 y se obtuvo %d (%s)", grabador.Code, grabador.Body.String())
	}
	if cache := grabador.Header().Get("Cache-Control"); cache != "private, no-store" {
		t.Fatalf("Cache-Control inesperado: %q", cache)
	}
	var respuesta struct {
		Items      []map[string]json.RawMessage `json:"items"`
		NextCursor string                       `json:"nextCursor"`
	}
	if err := json.Unmarshal(grabador.Body.Bytes(), &respuesta); err != nil {
		t.Fatalf("cuerpo no deserializable: %v", err)
	}
	if len(respuesta.Items) != 1 || respuesta.NextCursor == "" {
		t.Fatalf("pagina inesperada: %+v", respuesta)
	}
	if len(respuesta.Items[0]) != 10 {
		t.Fatalf("cada movimiento debe tener exactamente diez campos: %v", respuesta.Items[0])
	}
	for _, prohibido := range []string{"payloadHash", "userId", "targetSK"} {
		if strings.Contains(grabador.Body.String(), prohibido) {
			t.Fatalf("la respuesta no debe exponer %q", prohibido)
		}
	}

	// Sin movimientos, `items` es una lista vacía, nunca `null`.
	vacio := &movHTTPStore{}
	grabadorVacio := movHTTPEjecutar(movHTTPHandler(vacio), movHTTPPeticion(http.MethodGet, "/api/v1/movements", "jwt-valido", "", ""))
	if grabadorVacio.Code != http.StatusOK {
		t.Fatalf("se esperaba 200 y se obtuvo %d", grabadorVacio.Code)
	}
	if !strings.Contains(grabadorVacio.Body.String(), `"items":[]`) {
		t.Fatalf("items debe serializarse como lista vacia: %s", grabadorVacio.Body.String())
	}
	if strings.Contains(grabadorVacio.Body.String(), "nextCursor") {
		t.Fatalf("sin mas elementos no debe emitirse nextCursor: %s", grabadorVacio.Body.String())
	}
}

// TestE027HTTPParametros cubre T07: límites y filtros mal formados son 400.
func TestE027HTTPParametros(t *testing.T) {
	casos := []struct {
		nombre string
		query  string
		campo  string
	}{
		{"limite cero", "?limit=0", "limit"},
		{"limite excesivo", "?limit=101", "limit"},
		{"limite no numerico", "?limit=abc", "limit"},
		{"tipo fuera del enum", "?type=transfer", "type"},
		{"fecha desde invalida", "?from=ayer", "from"},
		{"fecha hasta invalida", "?to=2026-13-40T00:00:00Z", "to"},
		{"cursor demasiado corto", "?cursor=abc", "cursor"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			grabador := movHTTPEjecutar(movHTTPHandler(&movHTTPStore{}), movHTTPPeticion(http.MethodGet, "/api/v1/movements"+caso.query, "jwt-valido", "", ""))
			if grabador.Code != http.StatusBadRequest {
				t.Fatalf("se esperaba 400 y se obtuvo %d (%s)", grabador.Code, grabador.Body.String())
			}
			if !strings.Contains(grabador.Body.String(), caso.campo) {
				t.Fatalf("el detalle debe nombrar %q: %s", caso.campo, grabador.Body.String())
			}
		})
	}

	// Rango incoherente: el servicio lo rechaza y el transporte responde 400.
	grabador := movHTTPEjecutar(movHTTPHandler(&movHTTPStore{}), movHTTPPeticion(http.MethodGet,
		"/api/v1/movements?from=2026-11-18T00:00:00Z&to=2026-11-17T00:00:00Z", "jwt-valido", "", ""))
	if grabador.Code != http.StatusBadRequest {
		t.Fatalf("se esperaba 400 para un rango incoherente y se obtuvo %d", grabador.Code)
	}
}

// TestE027HTTPCursorYClave comprueba el rechazo de un cursor no verificable y el fail-closed sin
// clave de firma.
func TestE027HTTPCursorYClave(t *testing.T) {
	grabador := movHTTPEjecutar(movHTTPHandler(&movHTTPStore{}), movHTTPPeticion(http.MethodGet,
		"/api/v1/movements?cursor=cursor-sintetico-no-firmado", "jwt-valido", "", ""))
	if grabador.Code != http.StatusBadRequest {
		t.Fatalf("se esperaba 400 con cursor no verificado y se obtuvo %d", grabador.Code)
	}

	sinClave := appMovements.NewService(&movHTTPStore{})
	handler := internalhttp.NewMovementsHandler(sinClave, e004TokensValidos(), newE004Sesiones(), nil, silencioso())
	grabadorSinClave := movHTTPEjecutar(handler, movHTTPPeticion(http.MethodGet, "/api/v1/movements", "jwt-valido", "", ""))
	if grabadorSinClave.Code != http.StatusServiceUnavailable {
		t.Fatalf("se esperaba 503 sin clave de cursor y se obtuvo %d", grabadorSinClave.Code)
	}
}

// TestE027HTTPAutenticacionYMetodo cubre 401 y 405 del listado.
func TestE027HTTPAutenticacionYMetodo(t *testing.T) {
	if grabador := movHTTPEjecutar(movHTTPHandler(&movHTTPStore{}), movHTTPPeticion(http.MethodGet, "/api/v1/movements", "", "", "")); grabador.Code != http.StatusUnauthorized {
		t.Fatalf("se esperaba 401 y se obtuvo %d", grabador.Code)
	}

	grabador := movHTTPEjecutar(movHTTPHandler(&movHTTPStore{}), movHTTPPeticion(http.MethodPut, "/api/v1/movements", "jwt-valido", "", ""))
	if grabador.Code != http.StatusMethodNotAllowed {
		t.Fatalf("se esperaba 405 y se obtuvo %d", grabador.Code)
	}
	if allow := grabador.Header().Get("Allow"); !strings.Contains(allow, http.MethodGet) || !strings.Contains(allow, http.MethodPost) {
		t.Fatalf("Allow inesperado: %q", allow)
	}
}

// TestE028HTTPErrorDeServicioNoFiltra comprueba que un error interno no expone detalles.
func TestE028HTTPErrorDeServicioNoFiltra(t *testing.T) {
	fallo := &movHTTPStore{createFn: func(appMovements.CreateWrite) (*appMovements.CreateResult, error) {
		return nil, errors.New("fallo sintetico con detalle interno")
	}}
	grabador := movHTTPEjecutar(movHTTPHandler(fallo), movHTTPPeticion(http.MethodPost, "/api/v1/movements", "jwt-valido", movHTTPCuerpo, movHTTPClave))
	if grabador.Code != http.StatusInternalServerError {
		t.Fatalf("se esperaba 500 y se obtuvo %d", grabador.Code)
	}
	if strings.Contains(grabador.Body.String(), "detalle interno") {
		t.Fatalf("el mensaje interno no debe filtrarse: %s", grabador.Body.String())
	}
	if !strings.Contains(grabador.Body.String(), "traceId") {
		t.Fatalf("el sobre de error debe incluir traceId: %s", grabador.Body.String())
	}
}
