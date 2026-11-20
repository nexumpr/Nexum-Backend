package http

// Pruebas del handler de E037 (`POST /api/v1/recurring-payments/{recurringPaymentId}/payments`).
//
// El servicio de aplicación es el REAL; sólo se sustituyen el almacén (por un doble con estado de
// prueba) y las dependencias de identidad, para observar códigos, cabeceras y cuerpo sin red ni
// DynamoDB. La autorización por defecto exige sesión autoritativa: sin proveedor de tokens o sin
// autorizador el handler responde 503, nunca autoriza sólo con la firma.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
	appRecurring "github.com/nexumpr/Nexum-Backend/internal/application/recurring"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
)

const (
	e037HTTPRuta   = "/api/v1/recurring-payments/" + e037ReglaHTTP + "/payments"
	e037ReglaHTTP  = "018f90ef-08e5-7a68-b09f-2b67a47c92f0"
	e037ClaveHTTP  = "clave-idempotencia-e037-0001"
	e037ClaveHTTPB = "clave-idempotencia-e037-0002"
)

var e037HTTPAhora = time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)

// e037TokensFalsos valida sólo los tokens declarados en el mapa.
type e037TokensFalsos struct {
	claims map[string]*ports.TokenClaims
}

func (e e037TokensFalsos) GenerateAccessToken(context.Context, ports.TokenClaims) (string, error) {
	return "", errors.New("no usado en la prueba")
}

func (e e037TokensFalsos) ValidateAccessToken(_ context.Context, token string) (*ports.TokenClaims, error) {
	if claims, ok := e.claims[strings.TrimSpace(token)]; ok {
		return claims, nil
	}
	return nil, ports.ErrInvalidToken
}

func (e e037TokensFalsos) GenerateRefreshToken(context.Context) (string, error) {
	return "", errors.New("no usado en la prueba")
}

// e037SesionesFalsas autoriza o rechaza la sesión según su configuración.
type e037SesionesFalsas struct {
	err error
}

func (s e037SesionesFalsas) AuthorizeSession(context.Context, string, string, int64) error {
	return s.err
}

// tokensE037 compone un proveedor con dos sujetos y un token inválido.
func tokensE037() e037TokensFalsos {
	return e037TokensFalsos{claims: map[string]*ports.TokenClaims{
		"jwt-valido":       {Subject: e037UsuarioHTTP, SessionID: "sesion-e037"},
		"jwt-sujeto-ajeno": {Subject: "usuario-e037-ajeno", SessionID: "sesion-e037-ajena"},
	}}
}

const e037UsuarioHTTP = "usuario-e037-0001"

// pagoIdemHTTP es el registro de idempotencia del doble.
type pagoIdemHTTP struct {
	hash   string
	cuerpo []byte
	status int
}

// pagoStoreHTTPFalso implementa `appRecurring.PaymentStore` con estado de prueba.
type pagoStoreHTTPFalso struct {
	mu         sync.Mutex
	regla      *appRecurring.Payment
	errRegla   error
	errZona    error
	fechas     map[string]bool
	idem       map[string]pagoIdemHTTP
	escrituras int
}

func nuevoPagoStoreHTTPFalso() *pagoStoreHTTPFalso {
	return &pagoStoreHTTPFalso{
		regla: &appRecurring.Payment{
			ID:          e037ReglaHTTP,
			Name:        "Renta",
			AmountCents: 1_000_00,
			Currency:    "MXN",
			Type:        appMovements.TypeExpense,
			Active:      true,
			Version:     1,
			NextDueDate: time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC),
			StartDate:   time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC),
		},
		fechas: map[string]bool{},
		idem:   map[string]pagoIdemHTTP{},
	}
}

func (s *pagoStoreHTTPFalso) LoadRule(_ context.Context, userID, ruleID string) (*appRecurring.Payment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.errRegla != nil {
		return nil, s.errRegla
	}
	if strings.TrimSpace(userID) != e037UsuarioHTTP || s.regla == nil || s.regla.ID != ruleID {
		return nil, nil
	}
	copia := *s.regla
	return &copia, nil
}

func (s *pagoStoreHTTPFalso) LoadTimezone(_ context.Context, _ string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.errZona != nil {
		return "", s.errZona
	}
	return "America/Mexico_City", nil
}

func (s *pagoStoreHTTPFalso) Pay(_ context.Context, write appRecurring.PayWrite) (*appRecurring.PayResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	clave := write.UserID + "|" + write.Operation + "|" + write.IdempotencyKey
	if previo, ok := s.idem[clave]; ok {
		if previo.hash != write.PayloadHash {
			return nil, appRecurring.ErrConflict
		}
		return &appRecurring.PayResult{Status: previo.status, Body: append([]byte(nil), previo.cuerpo...), Replayed: true}, nil
	}
	if s.regla == nil || !s.regla.Active {
		return nil, appRecurring.ErrNotFound
	}
	llaveFecha := write.UserID + "|" + write.RuleID + "|" + write.LocalDate
	if s.fechas[llaveFecha] {
		return nil, appRecurring.ErrDuplicatePayment
	}
	s.fechas[llaveFecha] = true
	s.idem[clave] = pagoIdemHTTP{hash: write.PayloadHash, cuerpo: append([]byte(nil), write.ResponseBody...), status: write.Status}
	s.escrituras++
	return &appRecurring.PayResult{Status: write.Status, Body: append([]byte(nil), write.ResponseBody...)}, nil
}

func (s *pagoStoreHTTPFalso) escriturasRegistradas() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.escrituras
}

// loggerE037 descarta la salida: las pruebas no deben ensuciar la consola ni filtrar datos.
func loggerE037() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// handlerE037 compone el handler real sobre el doble con reloj e identificadores deterministas.
func handlerE037(store appRecurring.PaymentStore) *RecurringPaymentHandler {
	servicio := appRecurring.NewPaymentService(store).
		WithClock(func() time.Time { return e037HTTPAhora }).
		WithIDGenerator(func() (string, error) {
			// Identificador fijo: el cuerpo de la respuesta es determinista y el replay puede
			// compararse byte a byte.
			return strings.TrimSuffix(e037ReglaHTTP, "f0") + "f1", nil
		})
	return NewRecurringPaymentHandler(servicio, tokensE037(), e037SesionesFalsas{}, nil, loggerE037())
}

// cuerpoE037 es el payload válido de referencia.
func cuerpoE037() string {
	return `{"paidAt":"2026-11-05T23:30:00-06:00","amount":"1000.00","currency":"MXN"}`
}

// peticionE037 construye la petición autenticada con el parámetro de ruta ya resuelto.
func peticionE037(token, cuerpo, clave string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, e037HTTPRuta, strings.NewReader(cuerpo))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("recurringPaymentId", e037ReglaHTTP)
	if clave != "" {
		req.Header.Set("Idempotency-Key", clave)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}

// TestE037HTTP200ContratoExacto cubre el 200 y el esquema `Movement` del contrato.
func TestE037HTTP200ContratoExacto(t *testing.T) {
	store := nuevoPagoStoreHTTPFalso()
	rec := httptest.NewRecorder()
	handlerE037(store).ServeHTTP(rec, peticionE037("jwt-valido", cuerpoE037(), e037ClaveHTTP))

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if cache := rec.Header().Get("Cache-Control"); cache != "private, no-store" {
		t.Fatalf("Cache-Control inesperado: %q", cache)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type inesperado: %q", ct)
	}
	if replay := rec.Header().Get("X-Idempotent-Replay"); replay != "" {
		t.Fatalf("la primera respuesta no es un replay: %q", replay)
	}

	var cuerpo map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("respuesta no es JSON: %v", err)
	}
	esperados := map[string]any{
		"type":               appMovements.TypeExpense,
		"amount":             "1000.00",
		"currency":           "MXN",
		"categoryId":         "otros",
		"description":        "Renta",
		"occurredAt":         "2026-11-06T05:30:00.000Z",
		"version":            float64(1),
		"contributionId":     nil,
		"recurringPaymentId": e037ReglaHTTP,
	}
	for campo, esperado := range esperados {
		if cuerpo[campo] != esperado {
			t.Fatalf("%s esperado %v, obtenido %v", campo, esperado, cuerpo[campo])
		}
	}
	if id, _ := cuerpo["id"].(string); id == "" {
		t.Fatal("la respuesta debe incluir el identificador del movimiento")
	}
	if store.escriturasRegistradas() != 1 {
		t.Fatalf("debe persistirse un pago, escrituras=%d", store.escriturasRegistradas())
	}
	// La respuesta nunca refleja credenciales ni datos internos de persistencia.
	for _, prohibido := range []string{"jwt-valido", "Bearer", "payloadHash", "entityType", "PK", "SK"} {
		if strings.Contains(rec.Body.String(), prohibido) {
			t.Fatalf("la respuesta no puede contener %q: %s", prohibido, rec.Body.String())
		}
	}
}

// TestE037HTTPReplayYDuplicado cubre la idempotencia y el control por fecha local en el borde.
func TestE037HTTPReplayYDuplicado(t *testing.T) {
	handler := handlerE037(nuevoPagoStoreHTTPFalso())

	primera := httptest.NewRecorder()
	handler.ServeHTTP(primera, peticionE037("jwt-valido", cuerpoE037(), e037ClaveHTTP))
	if primera.Code != http.StatusOK {
		t.Fatalf("primer pago: %d (%s)", primera.Code, primera.Body.String())
	}

	replay := httptest.NewRecorder()
	handler.ServeHTTP(replay, peticionE037("jwt-valido", cuerpoE037(), e037ClaveHTTP))
	if replay.Code != http.StatusOK {
		t.Fatalf("el replay debe devolver 200, obtenido %d", replay.Code)
	}
	if replay.Header().Get("X-Idempotent-Replay") != "true" {
		t.Fatal("el replay debe declararse con X-Idempotent-Replay")
	}
	if replay.Body.String() != primera.Body.String() {
		t.Fatalf("el replay debe devolver bytes identicos:\n%s\n%s", primera.Body.String(), replay.Body.String())
	}

	duplicado := httptest.NewRecorder()
	handler.ServeHTTP(duplicado, peticionE037("jwt-valido", cuerpoE037(), e037ClaveHTTPB))
	if duplicado.Code != http.StatusConflict {
		t.Fatalf("un segundo pago el mismo dia local debe ser 409, obtenido %d (%s)", duplicado.Code, duplicado.Body.String())
	}
	if !strings.Contains(duplicado.Body.String(), "CONFLICT") {
		t.Fatalf("codigo de error inesperado: %s", duplicado.Body.String())
	}
}

// TestE037HTTP401 cubre la ausencia de identidad verificada.
func TestE037HTTP401(t *testing.T) {
	casos := []struct {
		nombre   string
		cabecera string
	}{
		{"sin cabecera", ""},
		{"esquema basico", "Basic abc"},
		{"token desconocido", "Bearer desconocido"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := nuevoPagoStoreHTTPFalso()
			req := httptest.NewRequest(http.MethodPost, e037HTTPRuta, strings.NewReader(cuerpoE037()))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Idempotency-Key", e037ClaveHTTP)
			req.SetPathValue("recurringPaymentId", e037ReglaHTTP)
			if caso.cabecera != "" {
				req.Header.Set("Authorization", caso.cabecera)
			}
			rec := httptest.NewRecorder()
			handlerE037(store).ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("esperado 401, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			if store.escriturasRegistradas() != 0 {
				t.Fatal("una peticion no autenticada no puede escribir")
			}
		})
	}
}

// TestE037HTTP400Esquema cubre los rechazos de forma.
func TestE037HTTP400Esquema(t *testing.T) {
	casos := []struct {
		nombre    string
		cuerpo    string
		clave     string
		pathValue string
	}{
		{"sin Idempotency-Key", cuerpoE037(), "", e037ReglaHTTP},
		{"clave corta", cuerpoE037(), "corta", e037ReglaHTTP},
		{"json malformado", `{"paidAt":`, e037ClaveHTTP, e037ReglaHTTP},
		{"campo desconocido", `{"paidAt":"2026-11-05T23:30:00-06:00","amount":"1000.00","currency":"MXN","type":"expense"}`, e037ClaveHTTP, e037ReglaHTTP},
		{"importe numerico", `{"paidAt":"2026-11-05T23:30:00-06:00","amount":1000,"currency":"MXN"}`, e037ClaveHTTP, e037ReglaHTTP},
		{"importe sin decimales", `{"paidAt":"2026-11-05T23:30:00-06:00","amount":"1000","currency":"MXN"}`, e037ClaveHTTP, e037ReglaHTTP},
		{"fecha no RFC3339", `{"paidAt":"05/11/2026","amount":"1000.00","currency":"MXN"}`, e037ClaveHTTP, e037ReglaHTTP},
		{"falta paidAt", `{"amount":"1000.00","currency":"MXN"}`, e037ClaveHTTP, e037ReglaHTTP},
		{"falta currency", `{"paidAt":"2026-11-05T23:30:00-06:00","amount":"1000.00"}`, e037ClaveHTTP, e037ReglaHTTP},
		{"identificador no uuid", cuerpoE037(), e037ClaveHTTP, "no-es-uuid"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := nuevoPagoStoreHTTPFalso()
			req := httptest.NewRequest(http.MethodPost, e037HTTPRuta, strings.NewReader(caso.cuerpo))
			req.Header.Set("Content-Type", "application/json")
			if caso.clave != "" {
				req.Header.Set("Idempotency-Key", caso.clave)
			}
			req.Header.Set("Authorization", "Bearer jwt-valido")
			req.SetPathValue("recurringPaymentId", caso.pathValue)

			rec := httptest.NewRecorder()
			handlerE037(store).ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("esperado 400, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "INVALID_REQUEST") {
				t.Fatalf("codigo de error inesperado: %s", rec.Body.String())
			}
			if store.escriturasRegistradas() != 0 {
				t.Fatal("una solicitud invalida no puede escribir")
			}
		})
	}
}

// TestE037HTTP404Y409 cubre regla inexistente/ajena e inactiva.
func TestE037HTTP404Y409(t *testing.T) {
	t.Run("inexistente", func(t *testing.T) {
		store := nuevoPagoStoreHTTPFalso()
		store.regla = nil
		rec := httptest.NewRecorder()
		handlerE037(store).ServeHTTP(rec, peticionE037("jwt-valido", cuerpoE037(), e037ClaveHTTP))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("esperado 404, obtenido %d (%s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("ajena", func(t *testing.T) {
		store := nuevoPagoStoreHTTPFalso()
		rec := httptest.NewRecorder()
		handlerE037(store).ServeHTTP(rec, peticionE037("jwt-sujeto-ajeno", cuerpoE037(), e037ClaveHTTP))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("esperado 404 sin revelar existencia, obtenido %d (%s)", rec.Code, rec.Body.String())
		}
		if store.escriturasRegistradas() != 0 {
			t.Fatal("una regla ajena no puede generar un pago")
		}
	})

	t.Run("inactiva", func(t *testing.T) {
		store := nuevoPagoStoreHTTPFalso()
		store.regla.Active = false
		rec := httptest.NewRecorder()
		handlerE037(store).ServeHTTP(rec, peticionE037("jwt-valido", cuerpoE037(), e037ClaveHTTP))
		if rec.Code != http.StatusConflict {
			t.Fatalf("esperado 409, obtenido %d (%s)", rec.Code, rec.Body.String())
		}
	})
}

// TestE037HTTP422ReglasDeNegocio comprueba las violaciones con su campo.
func TestE037HTTP422ReglasDeNegocio(t *testing.T) {
	casos := []struct {
		nombre string
		campo  string
		cuerpo string
	}{
		{"importe cero", "amount", `{"paidAt":"2026-11-05T23:30:00-06:00","amount":"0.00","currency":"MXN"}`},
		{"divisa no publicada", "currency", `{"paidAt":"2026-11-05T23:30:00-06:00","amount":"1000.00","currency":"USD"}`},
		{"fecha muy futura", "paidAt", `{"paidAt":"2027-01-01T00:00:00Z","amount":"1000.00","currency":"MXN"}`},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := nuevoPagoStoreHTTPFalso()
			rec := httptest.NewRecorder()
			handlerE037(store).ServeHTTP(rec, peticionE037("jwt-valido", caso.cuerpo, e037ClaveHTTP))
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("esperado 422, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			var sobre struct {
				Error struct {
					Code    string `json:"code"`
					Details []struct {
						Field string `json:"field"`
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
			if store.escriturasRegistradas() != 0 {
				t.Fatal("una solicitud con regla de negocio incumplida no puede escribir")
			}
		})
	}
}

// TestE037HTTP429 comprueba que la cuota por usuario responde 429 con Retry-After.
func TestE037HTTP429(t *testing.T) {
	servicio := appRecurring.NewPaymentService(nuevoPagoStoreHTTPFalso()).
		WithClock(func() time.Time { return e037HTTPAhora }).
		WithIDGenerator(func() (string, error) {
			return strings.TrimSuffix(e037ReglaHTTP, "f0") + "f1", nil
		})
	handler := NewRecurringPaymentHandler(servicio, tokensE037(), e037SesionesFalsas{},
		limitadorFalso{permitido: false, retry: 7 * time.Second}, loggerE037())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, peticionE037("jwt-valido", cuerpoE037(), e037ClaveHTTP))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("esperado 429, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if retry := rec.Header().Get("Retry-After"); retry != "7" {
		t.Fatalf("Retry-After inesperado: %q", retry)
	}
}

// TestE037HTTP405: sólo POST está publicado.
func TestE037HTTP405(t *testing.T) {
	store := nuevoPagoStoreHTTPFalso()
	req := httptest.NewRequest(http.MethodGet, e037HTTPRuta, nil)
	req.Header.Set("Authorization", "Bearer jwt-valido")
	req.SetPathValue("recurringPaymentId", e037ReglaHTTP)
	rec := httptest.NewRecorder()
	handlerE037(store).ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("esperado 405, obtenido %d", rec.Code)
	}
	if allow := rec.Header().Get("Allow"); allow != http.MethodPost {
		t.Fatalf("Allow inesperado: %q", allow)
	}
}

// TestE037HTTP503FallaCerrado: sin servicio, sin tokens, sin autorizador o sin persistencia no se
// confirma nada.
func TestE037HTTP503FallaCerrado(t *testing.T) {
	servicioSinRegla := func() *appRecurring.PaymentService {
		store := nuevoPagoStoreHTTPFalso()
		store.errRegla = errors.New("dynamodb no disponible")
		return appRecurring.NewPaymentService(store).
			WithClock(func() time.Time { return e037HTTPAhora }).
			WithIDGenerator(func() (string, error) { return e037ReglaHTTP, nil })
	}()
	servicioSesionNoDeterminable := func() *appRecurring.PaymentService {
		return appRecurring.NewPaymentService(nuevoPagoStoreHTTPFalso()).
			WithClock(func() time.Time { return e037HTTPAhora }).
			WithIDGenerator(func() (string, error) { return e037ReglaHTTP, nil })
	}()

	casos := []struct {
		nombre  string
		handler *RecurringPaymentHandler
	}{
		{"sin servicio", NewRecurringPaymentHandler(nil, tokensE037(), e037SesionesFalsas{}, nil, loggerE037())},
		{"sin tokens", NewRecurringPaymentHandler(servicioSesionNoDeterminable, nil, e037SesionesFalsas{}, nil, loggerE037())},
		{"sin autorizador", NewRecurringPaymentHandler(servicioSesionNoDeterminable, tokensE037(), nil, nil, loggerE037())},
		{"almacen no disponible", NewRecurringPaymentHandler(servicioSinRegla, tokensE037(), e037SesionesFalsas{}, nil, loggerE037())},
		{"sesion no determinable", NewRecurringPaymentHandler(servicioSesionNoDeterminable, tokensE037(), e037SesionesFalsas{err: ports.ErrSessionUnavailable}, nil, loggerE037())},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			caso.handler.ServeHTTP(rec, peticionE037("jwt-valido", cuerpoE037(), e037ClaveHTTP))
			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("esperado 503, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			// También en 503 la respuesta es privada.
			if cache := rec.Header().Get("Cache-Control"); cache != "private, no-store" {
				t.Fatalf("Cache-Control inesperado en 503: %q", cache)
			}
		})
	}
}
