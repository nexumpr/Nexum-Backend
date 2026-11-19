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

	appRecurring "github.com/nexumpr/Nexum-Backend/internal/application/recurring"
	domainIdentity "github.com/nexumpr/Nexum-Backend/internal/domain/identity"
	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// Pruebas HTTP de E035 (`PATCH /api/v1/recurring-payments/{recurringPaymentId}`): contrato 200
// con `ETag`, `If-Match` obligatorio (428/412/400), `RecurringPatch` estricto sin `frequency`,
// `type` inmutable (422), 404 sin enumeración, 409 de concurrencia, 401 y 503 fail-closed. La
// ruta se resuelve con el Router real para que `{recurringPaymentId}` llegue por `SetPathValue`.

const (
	e035ID       = "018f90ef-08e5-7a68-b09f-2b67a47c92f9"
	e035IDAjeno  = "018f90ef-08e5-7a68-b09f-2b67a47c92fa"
	e035RutaBase = "/api/v1/recurring-payments/"
)

// e035Store implementa `appRecurring.Store` con estado y semántica de versión.
type e035Store struct {
	mu              sync.Mutex
	pagos           map[string]appRecurring.Payment
	fallo           error
	conflicto       bool
	actualizaciones int
}

func newE035Store() *e035Store {
	return &e035Store{pagos: map[string]appRecurring.Payment{}}
}

func (s *e035Store) sembrar(userID string, pago appRecurring.Payment) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pagos[userID+"|"+pago.ID] = pago
}

func (s *e035Store) Get(_ context.Context, userID, paymentID string) (*appRecurring.Payment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fallo != nil {
		return nil, s.fallo
	}
	pago, ok := s.pagos[userID+"|"+paymentID]
	if !ok {
		return nil, nil
	}
	copia := pago
	return &copia, nil
}

func (s *e035Store) Update(_ context.Context, write appRecurring.UpdateWrite) (*appRecurring.Payment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fallo != nil {
		return nil, s.fallo
	}
	if s.conflicto {
		return nil, appRecurring.ErrConflict
	}
	clave := write.UserID + "|" + write.PaymentID
	actual, ok := s.pagos[clave]
	if !ok {
		return nil, appRecurring.ErrNotFound
	}
	if actual.Version != write.ExpectedVersion {
		return nil, appRecurring.ErrVersionConflict
	}
	actualizado := write.Payment
	actualizado.ID = write.PaymentID
	actualizado.Version = actual.Version + 1
	s.pagos[clave] = actualizado
	s.actualizaciones++
	copia := actualizado
	return &copia, nil
}

func (s *e035Store) Create(context.Context, appRecurring.CreateWrite) (string, []byte, bool, error) {
	return "", nil, false, errors.New("Create no usado en E035")
}

func (s *e035Store) List(context.Context, string, string, int, appRecurring.Filter) ([]appRecurring.Payment, string, error) {
	return nil, "", errors.New("List no usado en E035")
}

func (s *e035Store) Deactivate(context.Context, string, string, time.Time) (bool, error) {
	return false, errors.New("Deactivate no usado en E035")
}

// e035Pago construye la regla de referencia: mensual, vencimiento el 5, versión indicada.
func e035Pago(id string, version int64) appRecurring.Payment {
	dia := 5
	return appRecurring.Payment{
		ID:          id,
		Name:        "Renta",
		AmountCents: 100000,
		Currency:    "MXN",
		Frequency:   domainIdentity.FrequencyMonthly,
		NextDueDate: time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC),
		Active:      true,
		Version:     version,
		Type:        "expense",
		StartDate:   time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC),
		DueDay:      &dia,
	}
}

func e035Handler(store appRecurring.Store) *internalhttp.RecurringPatchHandler {
	servicio := appRecurring.NewService(store, nil)
	return internalhttp.NewRecurringPatchHandler(servicio, e004TokensValidos(), newE004Sesiones(), nil, silencioso())
}

func e035Router(handler *internalhttp.RecurringPatchHandler) *internalhttp.Router {
	router := internalhttp.NewRouter()
	router.HandleFunc(http.MethodPatch, "/recurring-payments/{recurringPaymentId}", handler.ServeHTTP)
	return router
}

func e035Peticion(id, token, ifMatch, cuerpo string) *http.Request {
	req := e004Peticion(http.MethodPatch, e035RutaBase+id, token, cuerpo)
	if ifMatch != "" {
		req.Header.Set("If-Match", ifMatch)
	}
	return req
}

// TestE035HTTP200ConETag cubre T01: 200, esquema, `ETag` de la versión nueva y una escritura.
func TestE035HTTP200ConETag(t *testing.T) {
	store := newE035Store()
	store.sembrar(e004HTTPUsuario, e035Pago(e035ID, 1))

	rec := httptest.NewRecorder()
	e035Router(e035Handler(store)).ServeHTTP(rec, e035Peticion(e035ID, "jwt-valido", `"v1"`, `{"name":"Renta ajustada"}`))

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if etag := rec.Header().Get("ETag"); etag != `"v2"` {
		t.Fatalf("ETag inesperado: %q", etag)
	}
	if cache := rec.Header().Get("Cache-Control"); cache != "private, no-store" {
		t.Fatalf("Cache-Control inesperado: %q", cache)
	}
	var cuerpo map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("respuesta no es JSON: %v", err)
	}
	if len(cuerpo) != 11 {
		t.Fatalf("se esperaban once campos, obtenidos %d: %v", len(cuerpo), cuerpo)
	}
	if cuerpo["name"] != "Renta ajustada" || cuerpo["version"] != float64(2) {
		t.Fatalf("actualizacion inesperada: %v", cuerpo)
	}
	if store.actualizaciones != 1 {
		t.Fatalf("se esperaba una escritura, obtenidas %d", store.actualizaciones)
	}
}

// TestE035HTTP200NoOp: un cuerpo vacío y un `type` repetido no incrementan la versión.
func TestE035HTTP200NoOp(t *testing.T) {
	casos := []struct {
		nombre string
		cuerpo string
	}{
		{"cuerpo vacio", `{}`},
		{"type identico", `{"type":"expense"}`},
		{"valores identicos", `{"name":"Renta","amount":"1000.00","active":true}`},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := newE035Store()
			store.sembrar(e004HTTPUsuario, e035Pago(e035ID, 1))
			rec := httptest.NewRecorder()
			e035Router(e035Handler(store)).ServeHTTP(rec, e035Peticion(e035ID, "jwt-valido", `"v1"`, caso.cuerpo))
			if rec.Code != http.StatusOK {
				t.Fatalf("esperado 200, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			if etag := rec.Header().Get("ETag"); etag != `"v1"` {
				t.Fatalf("un no-op no incrementa la version: %q", etag)
			}
			if store.actualizaciones != 0 {
				t.Fatalf("un no-op no escribe: %d", store.actualizaciones)
			}
		})
	}
}

// TestE035HTTP200DueDayTriEstado: `dueDay: null` elimina el día fijo.
func TestE035HTTP200DueDayTriEstado(t *testing.T) {
	store := newE035Store()
	store.sembrar(e004HTTPUsuario, e035Pago(e035ID, 1))

	rec := httptest.NewRecorder()
	e035Router(e035Handler(store)).ServeHTTP(rec, e035Peticion(e035ID, "jwt-valido", `"v1"`, `{"dueDay":null}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	var cuerpo map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("respuesta no es JSON: %v", err)
	}
	if valor, existe := cuerpo["dueDay"]; !existe || valor != nil {
		t.Fatalf("dueDay debe quedar null: %v", valor)
	}
}

// TestE035HTTP400 cubre los rechazos de esquema.
func TestE035HTTP400(t *testing.T) {
	casos := []struct {
		nombre  string
		id      string
		ifMatch string
		cuerpo  string
	}{
		{"uuid invalido", "no-es-uuid", `"v1"`, `{"name":"X"}`},
		{"If-Match sin comillas", e035ID, `v1`, `{"name":"X"}`},
		{"If-Match vacio entre comillas", e035ID, `""`, `{"name":"X"}`},
		{"campo frequency desconocido", e035ID, `"v1"`, `{"frequency":"monthly"}`},
		{"campo version desconocido", e035ID, `"v1"`, `{"version":2}`},
		{"json malformado", e035ID, `"v1"`, `{"name":`},
		{"dueDay fuera de rango", e035ID, `"v1"`, `{"dueDay":29}`},
		{"importe numerico", e035ID, `"v1"`, `{"amount":1000}`},
		{"fecha no civil", e035ID, `"v1"`, `{"nextDueDate":"05/11/2026"}`},
		{"nombre vacio", e035ID, `"v1"`, `{"name":"   "}`},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := newE035Store()
			store.sembrar(e004HTTPUsuario, e035Pago(e035ID, 1))
			rec := httptest.NewRecorder()
			e035Router(e035Handler(store)).ServeHTTP(rec, e035Peticion(caso.id, "jwt-valido", caso.ifMatch, caso.cuerpo))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("esperado 400, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			if store.actualizaciones != 0 {
				t.Fatal("una solicitud invalida no puede escribir")
			}
		})
	}
}

// TestE035HTTP428: sin `If-Match` no se evalúa siquiera el cuerpo.
func TestE035HTTP428(t *testing.T) {
	store := newE035Store()
	store.sembrar(e004HTTPUsuario, e035Pago(e035ID, 1))
	rec := httptest.NewRecorder()
	e035Router(e035Handler(store)).ServeHTTP(rec, e035Peticion(e035ID, "jwt-valido", "", `{"name":"X"}`))
	if rec.Code != http.StatusPreconditionRequired {
		t.Fatalf("esperado 428, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "If-Match") {
		t.Fatalf("el detalle debe nombrar la precondicion: %s", rec.Body.String())
	}
	if store.actualizaciones != 0 {
		t.Fatal("sin precondicion no se escribe")
	}
}

// TestE035HTTP412 cubre el validador obsoleto y el débil.
func TestE035HTTP412(t *testing.T) {
	casos := []struct {
		nombre  string
		ifMatch string
	}{
		{"version obsoleta", `"v9"`},
		{"validador debil", `W/"v1"`},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := newE035Store()
			store.sembrar(e004HTTPUsuario, e035Pago(e035ID, 1))
			rec := httptest.NewRecorder()
			e035Router(e035Handler(store)).ServeHTTP(rec, e035Peticion(e035ID, "jwt-valido", caso.ifMatch, `{"name":"X"}`))
			if rec.Code != http.StatusPreconditionFailed {
				t.Fatalf("esperado 412, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			if store.actualizaciones != 0 {
				t.Fatal("una precondicion fallida no escribe")
			}
		})
	}
}

// TestE035HTTP422 cubre las reglas de negocio: tipo inmutable, importe y coherencia de fechas.
func TestE035HTTP422(t *testing.T) {
	casos := []struct {
		nombre string
		campo  string
		cuerpo string
	}{
		{"type distinto", "type", `{"type":"income"}`},
		{"importe cero", "amount", `{"amount":"0.00"}`},
		{"fecha incoherente", "nextDueDate", `{"nextDueDate":"2026-11-09"}`},
		{"dueDay incoherente", "dueDay", `{"dueDay":6}`},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := newE035Store()
			store.sembrar(e004HTTPUsuario, e035Pago(e035ID, 1))
			rec := httptest.NewRecorder()
			e035Router(e035Handler(store)).ServeHTTP(rec, e035Peticion(e035ID, "jwt-valido", `"v1"`, caso.cuerpo))
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("esperado 422, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), caso.campo) {
				t.Fatalf("el detalle debe nombrar %q: %s", caso.campo, rec.Body.String())
			}
			if store.actualizaciones != 0 {
				t.Fatal("una violacion de negocio no escribe")
			}
		})
	}
}

// TestE035HTTP404NoEnumera: recurso ajeno e inexistente producen el mismo 404.
func TestE035HTTP404NoEnumera(t *testing.T) {
	store := newE035Store()
	store.sembrar(e004HTTPOtroUser, e035Pago(e035IDAjeno, 1))

	ajeno := httptest.NewRecorder()
	e035Router(e035Handler(store)).ServeHTTP(ajeno, e035Peticion(e035IDAjeno, "jwt-valido", `"v1"`, `{"name":"X"}`))
	if ajeno.Code != http.StatusNotFound {
		t.Fatalf("un recurso ajeno debe ser 404, obtenido %d (%s)", ajeno.Code, ajeno.Body.String())
	}

	inexistente := httptest.NewRecorder()
	e035Router(e035Handler(store)).ServeHTTP(inexistente, e035Peticion(e035ID, "jwt-valido", `"v1"`, `{"name":"X"}`))
	if inexistente.Code != http.StatusNotFound {
		t.Fatalf("un recurso inexistente debe ser 404, obtenido %d (%s)", inexistente.Code, inexistente.Body.String())
	}

	if ajeno.Body.String() != inexistente.Body.String() {
		t.Fatalf("los dos 404 deben ser identicos:\n%s\n%s", ajeno.Body.String(), inexistente.Body.String())
	}
	if strings.Contains(ajeno.Body.String(), e035IDAjeno) {
		t.Fatal("el 404 no puede reflejar el identificador solicitado")
	}
}

// TestE035HTTP409: un conflicto de concurrencia agotado no se disfraza de éxito.
func TestE035HTTP409(t *testing.T) {
	store := newE035Store()
	store.sembrar(e004HTTPUsuario, e035Pago(e035ID, 1))
	store.conflicto = true

	rec := httptest.NewRecorder()
	e035Router(e035Handler(store)).ServeHTTP(rec, e035Peticion(e035ID, "jwt-valido", `"v1"`, `{"name":"X"}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("esperado 409, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
}

// TestE035HTTP401 cubre la ausencia de identidad verificada.
func TestE035HTTP401(t *testing.T) {
	casos := []struct {
		nombre   string
		cabecera string
	}{
		{"sin cabecera", ""},
		{"esquema basico", "Basic abc"},
		{"token desconocido", "Bearer desconocido"},
		{"token expirado", "Bearer jwt-expirado"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := newE035Store()
			store.sembrar(e004HTTPUsuario, e035Pago(e035ID, 1))
			req := e035Peticion(e035ID, "", `"v1"`, `{"name":"X"}`)
			if caso.cabecera != "" {
				req.Header.Set("Authorization", caso.cabecera)
			}
			rec := httptest.NewRecorder()
			e035Router(e035Handler(store)).ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("esperado 401, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			if store.actualizaciones != 0 {
				t.Fatal("una peticion no autenticada no puede escribir")
			}
		})
	}
}

// TestE035HTTP503FallaCerrado: sin servicio o con un fallo de persistencia no se escribe nada.
func TestE035HTTP503FallaCerrado(t *testing.T) {
	casos := []struct {
		nombre  string
		handler *internalhttp.RecurringPatchHandler
	}{
		{"sin servicio", internalhttp.NewRecurringPatchHandler(nil, e004TokensValidos(), newE004Sesiones(), nil, silencioso())},
		{"sin tokens", internalhttp.NewRecurringPatchHandler(appRecurring.NewService(newE035Store(), nil), nil, newE004Sesiones(), nil, silencioso())},
		{"sin autorizador", internalhttp.NewRecurringPatchHandler(appRecurring.NewService(newE035Store(), nil), e004TokensValidos(), nil, nil, silencioso())},
		{"sin almacen", internalhttp.NewRecurringPatchHandler(appRecurring.NewService(nil, nil), e004TokensValidos(), newE004Sesiones(), nil, silencioso())},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			e035Router(caso.handler).ServeHTTP(rec, e035Peticion(e035ID, "jwt-valido", `"v1"`, `{"name":"X"}`))
			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("esperado 503, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			if cache := rec.Header().Get("Cache-Control"); cache != "private, no-store" {
				t.Fatalf("tambien en 503 la respuesta es privada, obtenido %q", cache)
			}
		})
	}

	store := newE035Store()
	store.sembrar(e004HTTPUsuario, e035Pago(e035ID, 1))
	store.fallo = errors.New("dynamodb no disponible")
	rec := httptest.NewRecorder()
	e035Router(e035Handler(store)).ServeHTTP(rec, e035Peticion(e035ID, "jwt-valido", `"v1"`, `{"name":"X"}`))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("un fallo de persistencia debe ser 503, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "dynamodb") {
		t.Fatalf("la respuesta no puede filtrar detalles de infraestructura: %s", rec.Body.String())
	}
}
