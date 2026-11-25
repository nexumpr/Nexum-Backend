package transport_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// Pruebas HTTP de E029/E030/E031: contrato, precondiciones If-Match, 204 sin cuerpo y
// aislamiento. Los dobles son exclusivamente de prueba.

const (
	itemHTTPID = "018f90ef-08e5-7a68-b09f-0000000000aa"
)

// Los dobles de E027/E028 (definidos en movements_test.go) deben seguir satisfaciendo la
// interfaz completa; sus métodos de ítem fallan cerrados.
func (m *movHTTPStore) LoadMovement(context.Context, string, string) (*appMovements.Movement, error) {
	return nil, appMovements.ErrUnavailable
}

func (m *movHTTPStore) ReplaceMovement(context.Context, appMovements.ReplaceWrite) (*appMovements.Movement, error) {
	return nil, appMovements.ErrUnavailable
}

func (m *movHTTPStore) DeleteMovement(context.Context, appMovements.DeleteWrite) error {
	return appMovements.ErrUnavailable
}

// movItemHTTPStore implementa los cinco métodos con estado propio.
type movItemHTTPStore struct {
	*movHTTPStore
	mu          sync.Mutex
	movimientos map[string]appMovements.Movement
	replaceFn   func(appMovements.ReplaceWrite) (*appMovements.Movement, error)
	deleteFn    func(appMovements.DeleteWrite) error
	reemplazos  []appMovements.ReplaceWrite
	borrados    []appMovements.DeleteWrite
}

func newMovItemHTTPStore() *movItemHTTPStore {
	return &movItemHTTPStore{movHTTPStore: &movHTTPStore{}, movimientos: map[string]appMovements.Movement{}}
}

func (m *movItemHTTPStore) sembrar(userID string, movimiento appMovements.Movement) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.movimientos[userID+"\x00"+movimiento.ID] = movimiento
}

func (m *movItemHTTPStore) LoadMovement(_ context.Context, userID, movementID string) (*appMovements.Movement, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	movimiento, ok := m.movimientos[userID+"\x00"+movementID]
	if !ok {
		return nil, nil
	}
	copia := movimiento
	return &copia, nil
}

func (m *movItemHTTPStore) ReplaceMovement(_ context.Context, write appMovements.ReplaceWrite) (*appMovements.Movement, error) {
	m.mu.Lock()
	m.reemplazos = append(m.reemplazos, write)
	fn := m.replaceFn
	m.mu.Unlock()
	if fn != nil {
		return fn(write)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.movimientos[write.UserID+"\x00"+write.MovementID] = write.Next
	resultado := write.Next
	return &resultado, nil
}

func (m *movItemHTTPStore) DeleteMovement(_ context.Context, write appMovements.DeleteWrite) error {
	m.mu.Lock()
	m.borrados = append(m.borrados, write)
	fn := m.deleteFn
	m.mu.Unlock()
	if fn != nil {
		return fn(write)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	// Tombstone lógico de E031: el ítem se conserva marcado y sube la versión.
	clave := write.UserID + "\x00" + write.MovementID
	actual, ok := m.movimientos[clave]
	if !ok {
		return appMovements.ErrConflict
	}
	if actual.Version != write.ExpectedVersion {
		return appMovements.ErrConflict
	}
	actual.Deleted = true
	actual.Version++
	m.movimientos[clave] = actual
	return nil
}

func movItemHTTPHandler(store appMovements.Store) *internalhttp.MovementHandler {
	var servicio *appMovements.Service
	if store != nil {
		servicio = appMovements.NewService(store).
			WithClock(func() time.Time { return e004HTTPAhora }).
			WithIDGenerator(func() string { return movHTTPID })
	}
	return internalhttp.NewMovementHandler(servicio, e004TokensValidos(), newE004Sesiones(), nil, silencioso())
}

// movItemHTTPPeticion construye la petición con el parámetro de ruta y las cabeceras indicadas.
func movItemHTTPPeticion(metodo, movementID, token, cuerpo, ifMatch string) *http.Request {
	ruta := "/api/v1/movements/" + movementID
	peticion := e004Peticion(metodo, ruta, token, cuerpo)
	peticion.SetPathValue("movementId", movementID)
	if ifMatch != "" {
		peticion.Header.Set("If-Match", ifMatch)
	}
	return peticion
}

func movItemHTTPMovimiento(id string, ocurrido time.Time) appMovements.Movement {
	return appMovements.Movement{
		ID: id, Type: "expense", AmountCents: 100000, Currency: "MXN",
		CategoryID: "food", Description: "Ejemplo de concepto", OccurredAt: ocurrido, Version: 1,
	}
}

// TestE029HTTP200 cubre el contrato del detalle: 200, ETag y exactamente diez campos.
func TestE029HTTP200(t *testing.T) {
	store := newMovItemHTTPStore()
	store.sembrar(e004HTTPUsuario, movItemHTTPMovimiento(itemHTTPID, e004HTTPAhora.Add(-time.Hour)))

	grabador := movHTTPEjecutar(movItemHTTPHandler(store), movItemHTTPPeticion(http.MethodGet, itemHTTPID, "jwt-valido", "", ""))
	if grabador.Code != http.StatusOK {
		t.Fatalf("se esperaba 200 y se obtuvo %d (%s)", grabador.Code, grabador.Body.String())
	}
	if etag := grabador.Header().Get("ETag"); etag != `"v1"` {
		t.Fatalf("ETag inesperado: %q", etag)
	}
	if cache := grabador.Header().Get("Cache-Control"); cache != "private, no-store" {
		t.Fatalf("Cache-Control inesperado: %q", cache)
	}
	var cuerpo map[string]json.RawMessage
	if err := json.Unmarshal(grabador.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("cuerpo no deserializable: %v", err)
	}
	if len(cuerpo) != 10 {
		t.Fatalf("campos inesperados: %v", cuerpo)
	}
	for _, prohibido := range []string{"payloadHash", "userId", "targetSK", "createdAt"} {
		if strings.Contains(grabador.Body.String(), prohibido) {
			t.Fatalf("la respuesta no debe exponer %q", prohibido)
		}
	}
}

// TestE029HTTPAislamientoYErrores cubre 404 de recurso ajeno, 400 de forma y 401.
func TestE029HTTPAislamientoYErrores(t *testing.T) {
	store := newMovItemHTTPStore()
	store.sembrar(e004HTTPOtroUser, movItemHTTPMovimiento(itemHTTPID, e004HTTPAhora.Add(-time.Hour)))

	if grabador := movHTTPEjecutar(movItemHTTPHandler(store), movItemHTTPPeticion(http.MethodGet, itemHTTPID, "jwt-valido", "", "")); grabador.Code != http.StatusNotFound {
		t.Fatalf("un movimiento ajeno debe ser 404 y se obtuvo %d", grabador.Code)
	}
	if grabador := movHTTPEjecutar(movItemHTTPHandler(store), movItemHTTPPeticion(http.MethodGet, "no-es-uuid", "jwt-valido", "", "")); grabador.Code != http.StatusBadRequest {
		t.Fatalf("un id mal formado debe ser 400 y se obtuvo %d", grabador.Code)
	}
	if grabador := movHTTPEjecutar(movItemHTTPHandler(store), movItemHTTPPeticion(http.MethodGet, itemHTTPID, "", "", "")); grabador.Code != http.StatusUnauthorized {
		t.Fatalf("sin credenciales debe ser 401 y se obtuvo %d", grabador.Code)
	}
	if grabador := movHTTPEjecutar(movItemHTTPHandler(nil), movItemHTTPPeticion(http.MethodGet, itemHTTPID, "jwt-valido", "", "")); grabador.Code != http.StatusServiceUnavailable {
		t.Fatalf("sin servicio debe ser 503 y se obtuvo %d", grabador.Code)
	}
}

// TestE030HTTPPrecondiciones cubre 428, 400 y 412 de `If-Match`.
func TestE030HTTPPrecondiciones(t *testing.T) {
	store := newMovItemHTTPStore()
	store.sembrar(e004HTTPUsuario, movItemHTTPMovimiento(itemHTTPID, e004HTTPAhora.Add(-time.Hour)))
	handler := movItemHTTPHandler(store)
	cuerpo := `{"description":"nueva"}`

	if grabador := movHTTPEjecutar(handler, movItemHTTPPeticion(http.MethodPatch, itemHTTPID, "jwt-valido", cuerpo, "")); grabador.Code != http.StatusPreconditionRequired {
		t.Fatalf("sin If-Match se esperaba 428 y se obtuvo %d", grabador.Code)
	}
	if grabador := movHTTPEjecutar(handler, movItemHTTPPeticion(http.MethodPatch, itemHTTPID, "jwt-valido", cuerpo, "v1")); grabador.Code != http.StatusBadRequest {
		t.Fatalf("un ETag sin comillas debe ser 400 y se obtuvo %d", grabador.Code)
	}
	if grabador := movHTTPEjecutar(handler, movItemHTTPPeticion(http.MethodPatch, itemHTTPID, "jwt-valido", cuerpo, `W/"v1"`)); grabador.Code != http.StatusPreconditionFailed {
		t.Fatalf("un validador debil no satisface If-Match: se esperaba 412 y se obtuvo %d", grabador.Code)
	}
	if grabador := movHTTPEjecutar(handler, movItemHTTPPeticion(http.MethodPatch, itemHTTPID, "jwt-valido", cuerpo, `"v7"`)); grabador.Code != http.StatusPreconditionFailed {
		t.Fatalf("una version obsoleta debe ser 412 y se obtuvo %d", grabador.Code)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.reemplazos) != 0 {
		t.Fatal("ninguna precondicion fallida puede escribir")
	}
}

// TestE030HTTP200 cubre la modificación correcta: 200, ETag de la versión nueva y cuerpo.
func TestE030HTTP200(t *testing.T) {
	store := newMovItemHTTPStore()
	store.sembrar(e004HTTPUsuario, movItemHTTPMovimiento(itemHTTPID, e004HTTPAhora.Add(-time.Hour)))

	grabador := movHTTPEjecutar(movItemHTTPHandler(store), movItemHTTPPeticion(http.MethodPatch, itemHTTPID, "jwt-valido", `{"description":"nueva","categoryId":"transport"}`, `"v1"`))
	if grabador.Code != http.StatusOK {
		t.Fatalf("se esperaba 200 y se obtuvo %d (%s)", grabador.Code, grabador.Body.String())
	}
	if etag := grabador.Header().Get("ETag"); etag != `"v2"` {
		t.Fatalf("ETag inesperado: %q", etag)
	}
	var cuerpo map[string]any
	if err := json.Unmarshal(grabador.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("cuerpo no deserializable: %v", err)
	}
	if cuerpo["version"] != float64(2) || cuerpo["description"] != "nueva" || cuerpo["categoryId"] != "transport" {
		t.Fatalf("cuerpo inesperado: %v", cuerpo)
	}
	if cuerpo["amount"] != "1000.00" {
		t.Fatal("el importe no es editable y debe conservarse")
	}
}

// TestE030HTTPCuerpoInvalido cubre 400 de campos no editables y 422 de dominio.
func TestE030HTTPCuerpoInvalido(t *testing.T) {
	store := newMovItemHTTPStore()
	store.sembrar(e004HTTPUsuario, movItemHTTPMovimiento(itemHTTPID, e004HTTPAhora.Add(-time.Hour)))
	handler := movItemHTTPHandler(store)

	casos := []struct {
		nombre   string
		cuerpo   string
		esperado int
		campo    string
	}{
		{"importe no editable", `{"amount":"5.00"}`, http.StatusBadRequest, ""},
		{"tipo no editable", `{"type":"income"}`, http.StatusBadRequest, ""},
		{"campo desconocido", `{"otro":1}`, http.StatusBadRequest, ""},
		{"descripcion vacia", `{"description":"  "}`, http.StatusUnprocessableEntity, "description"},
		{"fecha invalida", `{"occurredAt":"ayer"}`, http.StatusBadRequest, ""},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			grabador := movHTTPEjecutar(handler, movItemHTTPPeticion(http.MethodPatch, itemHTTPID, "jwt-valido", caso.cuerpo, `"v1"`))
			if grabador.Code != caso.esperado {
				t.Fatalf("se esperaba %d y se obtuvo %d (%s)", caso.esperado, grabador.Code, grabador.Body.String())
			}
			if caso.campo != "" && !strings.Contains(grabador.Body.String(), caso.campo) {
				t.Fatalf("el detalle debe nombrar %q: %s", caso.campo, grabador.Body.String())
			}
		})
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.reemplazos) != 0 {
		t.Fatal("un cuerpo invalido no puede escribir")
	}
}

// TestE030HTTP409 cubre la carrera sin cambio de versión.
func TestE030HTTP409(t *testing.T) {
	store := newMovItemHTTPStore()
	store.sembrar(e004HTTPUsuario, movItemHTTPMovimiento(itemHTTPID, e004HTTPAhora.Add(-time.Hour)))
	store.replaceFn = func(appMovements.ReplaceWrite) (*appMovements.Movement, error) {
		return nil, appMovements.ErrConflict
	}
	grabador := movHTTPEjecutar(movItemHTTPHandler(store), movItemHTTPPeticion(http.MethodPatch, itemHTTPID, "jwt-valido", `{"description":"nueva"}`, `"v1"`))
	if grabador.Code != http.StatusConflict {
		t.Fatalf("se esperaba 409 y se obtuvo %d (%s)", grabador.Code, grabador.Body.String())
	}
}

// TestE031HTTP204 cubre el borrado correcto: 204 sin cuerpo y sin Content-Type.
func TestE031HTTP204(t *testing.T) {
	store := newMovItemHTTPStore()
	store.sembrar(e004HTTPUsuario, movItemHTTPMovimiento(itemHTTPID, e004HTTPAhora.Add(-time.Hour)))

	grabador := movHTTPEjecutar(movItemHTTPHandler(store), movItemHTTPPeticion(http.MethodDelete, itemHTTPID, "jwt-valido", "", ""))
	if grabador.Code != http.StatusNoContent {
		t.Fatalf("se esperaba 204 y se obtuvo %d (%s)", grabador.Code, grabador.Body.String())
	}
	if grabador.Body.Len() != 0 {
		t.Fatalf("204 no puede llevar cuerpo: %q", grabador.Body.String())
	}
	if tipo := grabador.Header().Get("Content-Type"); tipo != "" {
		t.Fatalf("204 no debe declarar Content-Type: %q", tipo)
	}
	if cache := grabador.Header().Get("Cache-Control"); cache != "private, no-store" {
		t.Fatalf("Cache-Control inesperado: %q", cache)
	}
}

// TestE031HTTPErrores cubre 409 por aportación, 404, 400 y 401.
func TestE031HTTPErrores(t *testing.T) {
	ligado := movItemHTTPMovimiento(itemHTTPID, e004HTTPAhora.Add(-time.Hour))
	ligado.ContributionID = "018f90ef-08e5-7a68-b09f-0000000000bb"
	store := newMovItemHTTPStore()
	store.sembrar(e004HTTPUsuario, ligado)

	if grabador := movHTTPEjecutar(movItemHTTPHandler(store), movItemHTTPPeticion(http.MethodDelete, itemHTTPID, "jwt-valido", "", "")); grabador.Code != http.StatusConflict {
		t.Fatalf("una aportacion confirmada debe ser 409 y se obtuvo %d", grabador.Code)
	}
	if grabador := movHTTPEjecutar(movItemHTTPHandler(store), movItemHTTPPeticion(http.MethodDelete, "018f90ef-08e5-7a68-b09f-0000000000cc", "jwt-valido", "", "")); grabador.Code != http.StatusNotFound {
		t.Fatalf("un movimiento inexistente debe ser 404 y se obtuvo %d", grabador.Code)
	}
	if grabador := movHTTPEjecutar(movItemHTTPHandler(store), movItemHTTPPeticion(http.MethodDelete, "no-uuid", "jwt-valido", "", "")); grabador.Code != http.StatusBadRequest {
		t.Fatalf("un id mal formado debe ser 400 y se obtuvo %d", grabador.Code)
	}
	if grabador := movHTTPEjecutar(movItemHTTPHandler(store), movItemHTTPPeticion(http.MethodDelete, itemHTTPID, "", "", "")); grabador.Code != http.StatusUnauthorized {
		t.Fatalf("sin credenciales debe ser 401 y se obtuvo %d", grabador.Code)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.borrados) != 0 {
		t.Fatal("ningun error puede llegar al almacen como borrado")
	}
}

// TestE031HTTP204Idempotente cubre la autorización APRUEBO_E031_IDEMPOTENTE_204_CON_PUNTERO: el
// segundo DELETE responde 204 sin cuerpo y sin volver a escribir; después, GET y PATCH dan 404.
func TestE031HTTP204Idempotente(t *testing.T) {
	store := newMovItemHTTPStore()
	store.sembrar(e004HTTPUsuario, movItemHTTPMovimiento(itemHTTPID, e004HTTPAhora.Add(-time.Hour)))
	handler := movItemHTTPHandler(store)

	for intento := 1; intento <= 2; intento++ {
		grabador := movHTTPEjecutar(handler, movItemHTTPPeticion(http.MethodDelete, itemHTTPID, "jwt-valido", "", ""))
		if grabador.Code != http.StatusNoContent {
			t.Fatalf("DELETE %d: se esperaba 204 y se obtuvo %d (%s)", intento, grabador.Code, grabador.Body.String())
		}
		if grabador.Body.Len() != 0 {
			t.Fatalf("DELETE %d: 204 no puede llevar cuerpo", intento)
		}
	}
	store.mu.Lock()
	escrituras := len(store.borrados)
	tombstone, existe := store.movimientos[e004HTTPUsuario+"\x00"+itemHTTPID]
	store.mu.Unlock()
	if escrituras != 1 {
		t.Fatalf("el segundo DELETE no puede escribir: %d", escrituras)
	}
	if !existe || !tombstone.Deleted {
		t.Fatalf("el movimiento debe conservarse como tombstone: %+v", tombstone)
	}

	if grabador := movHTTPEjecutar(handler, movItemHTTPPeticion(http.MethodGet, itemHTTPID, "jwt-valido", "", "")); grabador.Code != http.StatusNotFound {
		t.Fatalf("leer un borrado debe ser 404 y se obtuvo %d", grabador.Code)
	}
	if grabador := movHTTPEjecutar(handler, movItemHTTPPeticion(http.MethodPatch, itemHTTPID, "jwt-valido", `{"description":"nueva"}`, `"v2"`)); grabador.Code != http.StatusNotFound {
		t.Fatalf("editar un borrado debe ser 404 y se obtuvo %d", grabador.Code)
	}
}

// TestE031HTTPAislamiento cubre que el borrado de A no afecta al movimiento de B.
func TestE031HTTPAislamiento(t *testing.T) {
	store := newMovItemHTTPStore()
	store.sembrar(e004HTTPUsuario, movItemHTTPMovimiento(itemHTTPID, e004HTTPAhora.Add(-time.Hour)))
	store.sembrar(e004HTTPOtroUser, movItemHTTPMovimiento(itemHTTPID, e004HTTPAhora.Add(-time.Hour)))
	handler := movItemHTTPHandler(store)

	if grabador := movHTTPEjecutar(handler, movItemHTTPPeticion(http.MethodDelete, itemHTTPID, "jwt-valido", "", "")); grabador.Code != http.StatusNoContent {
		t.Fatalf("se esperaba 204 y se obtuvo %d", grabador.Code)
	}
	if grabador := movHTTPEjecutar(handler, movItemHTTPPeticion(http.MethodGet, itemHTTPID, "jwt-valido", "", "")); grabador.Code != http.StatusNotFound {
		t.Fatalf("A ya no debe leerlo: %d", grabador.Code)
	}
	store.mu.Lock()
	ajeno, existe := store.movimientos[e004HTTPOtroUser+"\x00"+itemHTTPID]
	store.mu.Unlock()
	if !existe || ajeno.Deleted {
		t.Fatalf("el movimiento de B debe seguir intacto: %+v", ajeno)
	}
}

// TestMovementHTTP405 comprueba el método no permitido en la ruta por identificador.
func TestMovementHTTP405(t *testing.T) {
	grabador := movHTTPEjecutar(movItemHTTPHandler(newMovItemHTTPStore()), movItemHTTPPeticion(http.MethodPost, itemHTTPID, "jwt-valido", "", ""))
	if grabador.Code != http.StatusMethodNotAllowed {
		t.Fatalf("se esperaba 405 y se obtuvo %d", grabador.Code)
	}
	allow := grabador.Header().Get("Allow")
	for _, metodo := range []string{http.MethodGet, http.MethodPatch, http.MethodDelete} {
		if !strings.Contains(allow, metodo) {
			t.Fatalf("Allow debe incluir %s: %q", metodo, allow)
		}
	}
}
