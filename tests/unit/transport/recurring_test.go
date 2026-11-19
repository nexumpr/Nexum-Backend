package transport_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	appRecurring "github.com/nexumpr/Nexum-Backend/internal/application/recurring"
	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// Pruebas HTTP de E033 (`POST /api/v1/recurring-payments`): contrato 201 con los once campos,
// `Idempotency-Key` obligatoria, replay con `X-Idempotent-Replay`, 409 con payload distinto,
// normalización de frecuencias (nueve aceptadas, siete emitidas), 422 de reglas de negocio,
// 400 de esquema, 401 sin identidad, 404 no aplica en creación y 503 fail-closed. Los dobles
// son exclusivamente de prueba; la composición productiva usa DynamoDB real.

const (
	e033HTTPClave = "clave-idempotencia-e033-0001"
	e033HTTPRuta  = "/api/v1/recurring-payments"
)

var e033HTTPAhora = time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)

// e033HTTPStore implementa `appRecurring.Store` con estado en memoria de prueba.
type e033HTTPStore struct {
	mu         sync.Mutex
	recursos   map[string]appRecurring.Payment
	idem       map[string]e033HTTPIdem
	escrituras []appRecurring.CreateWrite
	fallo      error
}

type e033HTTPIdem struct {
	hash string
	id   string
	body []byte
}

func newE033HTTPStore() *e033HTTPStore {
	return &e033HTTPStore{
		recursos: map[string]appRecurring.Payment{},
		idem:     map[string]e033HTTPIdem{},
	}
}

func (s *e033HTTPStore) Create(_ context.Context, write appRecurring.CreateWrite) (string, []byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fallo != nil {
		return "", nil, false, s.fallo
	}
	clave := write.UserID + "|" + write.IdempotencyKey
	if existente, ok := s.idem[clave]; ok {
		if existente.hash != write.PayloadHash {
			return "", nil, false, appRecurring.ErrConflict
		}
		return existente.id, existente.body, true, nil
	}
	s.escrituras = append(s.escrituras, write)
	s.recursos[write.UserID+"|"+write.Payment.ID] = write.Payment
	s.idem[clave] = e033HTTPIdem{hash: write.PayloadHash, id: write.Payment.ID, body: write.Body}
	return write.Payment.ID, write.Body, false, nil
}

func (s *e033HTTPStore) List(_ context.Context, userID, startSK string, limit int, filter appRecurring.Filter) ([]appRecurring.Payment, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fallo != nil {
		return nil, "", s.fallo
	}
	salida := make([]appRecurring.Payment, 0, limit)
	for clave, pago := range s.recursos {
		if !strings.HasPrefix(clave, userID+"|") {
			continue
		}
		if startSK != "" && pago.ID <= startSK {
			continue
		}
		if filter.Active != nil && pago.Active != *filter.Active {
			continue
		}
		if filter.Type != "" && pago.Type != filter.Type {
			continue
		}
		salida = append(salida, pago)
	}
	return salida, "", nil
}

func (s *e033HTTPStore) Get(_ context.Context, userID, paymentID string) (*appRecurring.Payment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fallo != nil {
		return nil, s.fallo
	}
	pago, ok := s.recursos[userID+"|"+paymentID]
	if !ok {
		return nil, nil
	}
	copia := pago
	return &copia, nil
}

func (s *e033HTTPStore) Update(_ context.Context, write appRecurring.UpdateWrite) (*appRecurring.Payment, error) {
	return nil, errors.New("no usado en E033")
}

func (s *e033HTTPStore) Deactivate(_ context.Context, _, _ string, _ time.Time) (bool, error) {
	return false, errors.New("no usado en E033")
}

// e033HTTPHandler compone el handler real sobre el doble.
func e033HTTPHandler(store appRecurring.Store) *internalhttp.RecurringCreateHandler {
	var secuencia int
	servicio := appRecurring.NewService(store, nil).
		WithClock(func() time.Time { return e033HTTPAhora }).
		WithIDGenerator(func() (string, error) {
			secuencia++
			return fmt.Sprintf("018f90ef-08e5-7a68-b09f-2b67a4%08d", secuencia), nil
		})
	return internalhttp.NewRecurringCreateHandler(servicio, e004TokensValidos(), newE004Sesiones(), nil, silencioso())
}

// e033HTTPPayload es el cuerpo válido de referencia.
func e033HTTPPayload() string {
	return `{"name":"Renta","amount":"1000.00","currency":"MXN","frequency":"monthly","nextDueDate":"2026-11-05","type":"expense","startDate":"2026-10-05"}`
}

// e033HTTPPeticion construye la petición autenticada con su clave de idempotencia.
func e033HTTPPeticion(token, cuerpo, clave string) *http.Request {
	req := e004Peticion(http.MethodPost, e033HTTPRuta, token, cuerpo)
	if clave != "" {
		req.Header.Set("Idempotency-Key", clave)
	}
	return req
}

// TestE033HTTP201ContratoExacto cubre T01: código, esquema y cabeceras.
func TestE033HTTP201ContratoExacto(t *testing.T) {
	store := newE033HTTPStore()
	rec := httptest.NewRecorder()
	e033HTTPHandler(store).ServeHTTP(rec, e033HTTPPeticion("jwt-valido", e033HTTPPayload(), e033HTTPClave))

	if rec.Code != http.StatusCreated {
		t.Fatalf("esperado 201, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if cache := rec.Header().Get("Cache-Control"); cache != "private, no-store" {
		t.Fatalf("Cache-Control inesperado: %q", cache)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type inesperado: %q", ct)
	}
	var cuerpo map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("respuesta no es JSON: %v", err)
	}
	ubicacion := rec.Header().Get("Location")
	if !strings.HasPrefix(ubicacion, e033HTTPRuta+"/") {
		t.Fatalf("Location inesperada: %q", ubicacion)
	}
	if id, _ := cuerpo["id"].(string); id == "" || !strings.HasSuffix(ubicacion, id) {
		t.Fatalf("Location debe terminar con el id del recurso: %q vs %v", ubicacion, cuerpo["id"])
	}
	esperados := map[string]any{
		"name":        "Renta",
		"amount":      "1000.00",
		"currency":    "MXN",
		"frequency":   "monthly",
		"nextDueDate": "2026-11-05",
		"active":      true,
		"version":     float64(1),
		"type":        "expense",
		"startDate":   "2026-10-05",
	}
	for campo, esperado := range esperados {
		if cuerpo[campo] != esperado {
			t.Fatalf("%s esperado %v, obtenido %v", campo, esperado, cuerpo[campo])
		}
	}
	if valor, existe := cuerpo["dueDay"]; !existe || valor != nil {
		t.Fatalf("dueDay debe estar presente como null: %v", valor)
	}
	for _, prohibido := range []string{"userId", "entityType", "PK", "SK", "createdAt", "concept", "expenseType"} {
		if _, ok := cuerpo[prohibido]; ok {
			t.Fatalf("la respuesta no puede exponer %q", prohibido)
		}
	}
	if len(store.escrituras) != 1 {
		t.Fatalf("debe persistirse una regla, escrituras=%d", len(store.escrituras))
	}
}

// TestE033HTTPNormalizaFrecuencias comprueba los nueve valores publicados y la emisión de los
// siete canónicos.
func TestE033HTTPNormalizaFrecuencias(t *testing.T) {
	casos := []struct {
		entrada  string
		canonica string
		inicio   string
		proximo  string
	}{
		{"weekly", "weekly", "2026-11-01", "2026-11-08"},
		{"fortnightly", "fortnightly", "2026-11-01", "2026-11-15"},
		{"biweekly", "fortnightly", "2026-11-01", "2026-11-15"},
		{"monthly", "monthly", "2026-10-05", "2026-11-05"},
		{"bimonthly", "bimonthly", "2026-10-05", "2026-12-05"},
		{"quarterly", "quarterly", "2026-10-05", "2027-01-05"},
		{"semiannual", "semiannual", "2026-10-05", "2027-04-05"},
		{"annual", "annual", "2026-10-05", "2027-10-05"},
		{"yearly", "annual", "2026-10-05", "2027-10-05"},
	}
	for _, caso := range casos {
		t.Run(caso.entrada, func(t *testing.T) {
			cuerpo := fmt.Sprintf(`{"name":"Renta","amount":"1000.00","currency":"MXN","frequency":%q,"nextDueDate":%q,"type":"expense","startDate":%q}`,
				caso.entrada, caso.proximo, caso.inicio)
			rec := httptest.NewRecorder()
			e033HTTPHandler(newE033HTTPStore()).ServeHTTP(rec, e033HTTPPeticion("jwt-valido", cuerpo, e033HTTPClave))
			if rec.Code != http.StatusCreated {
				t.Fatalf("frecuencia %q debe aceptarse: %d (%s)", caso.entrada, rec.Code, rec.Body.String())
			}
			var respuesta map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &respuesta); err != nil {
				t.Fatalf("respuesta no es JSON: %v", err)
			}
			if respuesta["frequency"] != caso.canonica {
				t.Fatalf("se esperaba %q, obtenido %v", caso.canonica, respuesta["frequency"])
			}
		})
	}
}

// TestE033HTTPIdempotencyKeyObligatoria cubre el 400 de la cabecera.
func TestE033HTTPIdempotencyKeyObligatoria(t *testing.T) {
	casos := []struct {
		nombre string
		clave  string
	}{
		{"ausente", ""},
		{"demasiado corta", "corta"},
		{"demasiado larga", strings.Repeat("x", 129)},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := newE033HTTPStore()
			rec := httptest.NewRecorder()
			e033HTTPHandler(store).ServeHTTP(rec, e033HTTPPeticion("jwt-valido", e033HTTPPayload(), caso.clave))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("esperado 400, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			if len(store.escrituras) != 0 {
				t.Fatal("sin clave valida no se escribe nada")
			}
		})
	}
}

// TestE033HTTPReplayYConflicto cubre T07 en el transporte.
func TestE033HTTPReplayYConflicto(t *testing.T) {
	handler := e033HTTPHandler(newE033HTTPStore())

	primera := httptest.NewRecorder()
	handler.ServeHTTP(primera, e033HTTPPeticion("jwt-valido", e033HTTPPayload(), e033HTTPClave))
	if primera.Code != http.StatusCreated {
		t.Fatalf("primera creacion: %d (%s)", primera.Code, primera.Body.String())
	}
	if primera.Header().Get("X-Idempotent-Replay") != "" {
		t.Fatal("la primera respuesta no es un replay")
	}

	segunda := httptest.NewRecorder()
	handler.ServeHTTP(segunda, e033HTTPPeticion("jwt-valido", e033HTTPPayload(), e033HTTPClave))
	if segunda.Code != http.StatusCreated {
		t.Fatalf("el replay debe devolver 201, obtenido %d", segunda.Code)
	}
	if segunda.Header().Get("X-Idempotent-Replay") != "true" {
		t.Fatal("el replay debe declararse con X-Idempotent-Replay")
	}
	if segunda.Body.String() != primera.Body.String() {
		t.Fatalf("el replay debe devolver bytes identicos:\n%s\n%s", primera.Body.String(), segunda.Body.String())
	}

	distinta := httptest.NewRecorder()
	handler.ServeHTTP(distinta, e033HTTPPeticion("jwt-valido",
		`{"name":"Renta ajustada","amount":"1000.00","currency":"MXN","frequency":"monthly","nextDueDate":"2026-11-05","type":"expense","startDate":"2026-10-05"}`,
		e033HTTPClave))
	if distinta.Code != http.StatusConflict {
		t.Fatalf("se esperaba 409, obtenido %d (%s)", distinta.Code, distinta.Body.String())
	}
}

// TestE033HTTP422ReglasDeNegocio comprueba las violaciones 422 con su campo.
func TestE033HTTP422ReglasDeNegocio(t *testing.T) {
	casos := []struct {
		nombre string
		campo  string
		cuerpo string
	}{
		{"frecuencia fuera del conjunto", "frequency",
			`{"name":"Renta","amount":"1000.00","currency":"MXN","frequency":"daily","nextDueDate":"2026-11-05","type":"expense","startDate":"2026-10-05"}`},
		{"tipo desconocido", "type",
			`{"name":"Renta","amount":"1000.00","currency":"MXN","frequency":"monthly","nextDueDate":"2026-11-05","type":"fixed","startDate":"2026-10-05"}`},
		{"importe cero", "amount",
			`{"name":"Renta","amount":"0.00","currency":"MXN","frequency":"monthly","nextDueDate":"2026-11-05","type":"expense","startDate":"2026-10-05"}`},
		{"fecha incoherente", "nextDueDate",
			`{"name":"Renta","amount":"1000.00","currency":"MXN","frequency":"weekly","nextDueDate":"2026-11-09","type":"expense","startDate":"2026-11-01"}`},
		{"dueDay no aplica a semanal", "dueDay",
			`{"name":"Renta","amount":"1000.00","currency":"MXN","frequency":"weekly","nextDueDate":"2026-11-08","type":"expense","startDate":"2026-11-01","dueDay":5}`},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := newE033HTTPStore()
			rec := httptest.NewRecorder()
			e033HTTPHandler(store).ServeHTTP(rec, e033HTTPPeticion("jwt-valido", caso.cuerpo, e033HTTPClave))
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("esperado 422, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			var sobre struct {
				Error struct {
					Code    string `json:"code"`
					TraceID string `json:"traceId"`
					Details []struct {
						Field  string `json:"field"`
						Reason string `json:"reason"`
					} `json:"details"`
				} `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &sobre); err != nil {
				t.Fatalf("respuesta no es JSON: %v", err)
			}
			if sobre.Error.Code != "BUSINESS_RULE_VIOLATION" {
				t.Fatalf("codigo inesperado: %q", sobre.Error.Code)
			}
			if len(sobre.Error.Details) != 1 || sobre.Error.Details[0].Field != caso.campo {
				t.Fatalf("se esperaba detalle de %q, obtenido %+v", caso.campo, sobre.Error.Details)
			}
			if len(store.escrituras) != 0 {
				t.Fatal("una regla invalida no puede escribir nada")
			}
		})
	}
}

// TestE033HTTP400Esquema comprueba los rechazos de esquema.
func TestE033HTTP400Esquema(t *testing.T) {
	casos := []struct {
		nombre string
		cuerpo string
	}{
		{"json malformado", `{"name":`},
		{"campo desconocido", `{"name":"Renta","amount":"1000.00","currency":"MXN","frequency":"monthly","nextDueDate":"2026-11-05","type":"expense","startDate":"2026-10-05","id":"x"}`},
		{"importe numerico", `{"name":"Renta","amount":1000,"currency":"MXN","frequency":"monthly","nextDueDate":"2026-11-05","type":"expense","startDate":"2026-10-05"}`},
		{"divisa no publicada", `{"name":"Renta","amount":"1000.00","currency":"USD","frequency":"monthly","nextDueDate":"2026-11-05","type":"expense","startDate":"2026-10-05"}`},
		{"dueDay fuera de rango", `{"name":"Renta","amount":"1000.00","currency":"MXN","frequency":"monthly","nextDueDate":"2026-11-05","type":"expense","startDate":"2026-10-05","dueDay":29}`},
		{"fecha no civil", `{"name":"Renta","amount":"1000.00","currency":"MXN","frequency":"monthly","nextDueDate":"05/11/2026","type":"expense","startDate":"2026-10-05"}`},
		{"falta type", `{"name":"Renta","amount":"1000.00","currency":"MXN","frequency":"monthly","nextDueDate":"2026-11-05","startDate":"2026-10-05"}`},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := newE033HTTPStore()
			rec := httptest.NewRecorder()
			e033HTTPHandler(store).ServeHTTP(rec, e033HTTPPeticion("jwt-valido", caso.cuerpo, e033HTTPClave))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("esperado 400, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "ErrorEnvelope") && !strings.Contains(rec.Body.String(), "\"error\"") {
				t.Fatalf("debe devolver el sobre canonico: %s", rec.Body.String())
			}
			if len(store.escrituras) != 0 {
				t.Fatal("una solicitud invalida no puede escribir nada")
			}
		})
	}
}

// TestE033HTTP401 cubre T02: sin identidad verificada no se toca la persistencia.
func TestE033HTTP401(t *testing.T) {
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
			store := newE033HTTPStore()
			req := httptest.NewRequest(http.MethodPost, e033HTTPRuta, strings.NewReader(e033HTTPPayload()))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Idempotency-Key", e033HTTPClave)
			if caso.cabecera != "" {
				req.Header.Set("Authorization", caso.cabecera)
			}
			rec := httptest.NewRecorder()
			e033HTTPHandler(store).ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("esperado 401, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			if len(store.escrituras) != 0 {
				t.Fatal("una peticion no autenticada no puede escribir")
			}
		})
	}
}

// TestE033HTTP503FallaCerrado: sin servicio o sin almacén no se confirma nada.
func TestE033HTTP503FallaCerrado(t *testing.T) {
	casos := []struct {
		nombre  string
		handler *internalhttp.RecurringCreateHandler
	}{
		{"sin servicio", internalhttp.NewRecurringCreateHandler(nil, e004TokensValidos(), newE004Sesiones(), nil, silencioso())},
		{"sin tokens", internalhttp.NewRecurringCreateHandler(appRecurring.NewService(newE033HTTPStore(), nil), nil, newE004Sesiones(), nil, silencioso())},
		{"sin autorizador", internalhttp.NewRecurringCreateHandler(appRecurring.NewService(newE033HTTPStore(), nil), e004TokensValidos(), nil, nil, silencioso())},
		{"sin almacen", internalhttp.NewRecurringCreateHandler(appRecurring.NewService(nil, nil), e004TokensValidos(), newE004Sesiones(), nil, silencioso())},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			caso.handler.ServeHTTP(rec, e033HTTPPeticion("jwt-valido", e033HTTPPayload(), e033HTTPClave))
			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("esperado 503, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			if cache := rec.Header().Get("Cache-Control"); cache != "private, no-store" {
				t.Fatalf("tambien en 503 la respuesta es privada, obtenido %q", cache)
			}
		})
	}
}

// TestE033HTTPAislamientoEntreUsuarios: dos sujetos con la misma clave crean recursos
// distintos y la persistencia recibe cada identidad por separado.
func TestE033HTTPAislamientoEntreUsuarios(t *testing.T) {
	store := newE033HTTPStore()
	handler := e033HTTPHandler(store)

	primera := httptest.NewRecorder()
	handler.ServeHTTP(primera, e033HTTPPeticion("jwt-valido", e033HTTPPayload(), e033HTTPClave))
	if primera.Code != http.StatusCreated {
		t.Fatalf("alta de A: %d (%s)", primera.Code, primera.Body.String())
	}

	segunda := httptest.NewRecorder()
	handler.ServeHTTP(segunda, e033HTTPPeticion("jwt-sujeto-ajeno", e033HTTPPayload(), e033HTTPClave))
	if segunda.Code != http.StatusCreated {
		t.Fatalf("alta de B: %d (%s)", segunda.Code, segunda.Body.String())
	}

	var cuerpoA, cuerpoB map[string]any
	if err := json.Unmarshal(primera.Body.Bytes(), &cuerpoA); err != nil {
		t.Fatalf("respuesta A no es JSON: %v", err)
	}
	if err := json.Unmarshal(segunda.Body.Bytes(), &cuerpoB); err != nil {
		t.Fatalf("respuesta B no es JSON: %v", err)
	}
	if cuerpoA["id"] == cuerpoB["id"] {
		t.Fatal("dos cuentas no pueden compartir el registro de idempotencia")
	}
	if len(store.escrituras) != 2 {
		t.Fatalf("cada cuenta debe tener su propia regla, escrituras=%d", len(store.escrituras))
	}
	if store.escrituras[0].UserID == store.escrituras[1].UserID {
		t.Fatal("la persistencia debe recibir identidades distintas")
	}
}
