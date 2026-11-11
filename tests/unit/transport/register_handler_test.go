package transport_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
	memorystore "github.com/nexumpr/Nexum-Backend/tests/support/memorystore"
)

type handlerHasher struct{}

func (handlerHasher) HashPassword(password string) (string, error) { return "hashed:" + password, nil }
func (handlerHasher) VerifyPassword(password, hash string) (bool, error) {
	return hash == "hashed:"+password, nil
}

type fakeRegisterLimiter struct {
	allowed    bool
	retryAfter time.Duration
	err        error
	class      string
	identifier string
	calls      int
}

func (f *fakeRegisterLimiter) Allow(_ context.Context, class, identifier string) (bool, time.Duration, error) {
	f.calls++
	f.class = class
	f.identifier = identifier
	if f.err != nil {
		return false, 0, f.err
	}
	return f.allowed, f.retryAfter, nil
}

type handlerFixture struct {
	handler *internalhttp.RegisterHandler
	store   *memorystore.RegistrationStore
	limiter *fakeRegisterLimiter
}

func newHandlerFixture(t *testing.T) handlerFixture {
	t.Helper()
	store := memorystore.NewRegistrationStore()
	var seq atomic.Int64
	svc := appIdentity.NewRegisterService(store, handlerHasher{}).
		WithClock(func() time.Time { return time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC) }).
		WithIDGenerator(func() (string, error) {
			return fmt.Sprintf("018f90ef-08e5-4a68-b09f-%012x", seq.Add(1)), nil
		})
	limiter := &fakeRegisterLimiter{allowed: true}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	return handlerFixture{
		handler: internalhttp.NewRegisterHandler(svc, limiter, logger),
		store:   store,
		limiter: limiter,
	}
}

const syntheticPhrase = "ExamplePassphrase#2026"

// Cuerpo valido del contrato V6.0.2 completo: identidad, ingreso, un gasto fijo,
// privacidad y contexto de cliente.
const validRegisterBody = `{"name":"Usuario Ejemplo","email":"usuario@example.invalid","password":"` + syntheticPhrase + `",` +
	`"income":{"amount":"12000.00","frequency":"monthly"},` +
	`"recurringExpenses":[{"concept":"Internet","amount":"699.00","frequency":"monthly","category":"Servicios","type":"fixed"}],` +
	`"privacy":{"accepted":true,"noticeVersion":"2026-11"},` +
	`"client":{"locale":"es-MX","timeZone":"America/Mexico_City"}}`

// Cuerpo sin gastos recurrentes: la lista vacia es valida.
const emptyExpensesRegisterBody = `{"name":"Sin Gastos","email":"sin.gastos@example.invalid","password":"` + syntheticPhrase + `",` +
	`"income":{"amount":"9500.50","frequency":"fortnightly"},` +
	`"recurringExpenses":[],` +
	`"privacy":{"accepted":true,"noticeVersion":"2026-11"}}`

func registerRequest(body, key string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	req.Header.Set("x-request-id", "trace-e001-123")
	return req
}

func serveRegister(f handlerFixture, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	return rec
}

func decodeRegisterResponse(t *testing.T, rec *httptest.ResponseRecorder) appIdentity.RegisterResponse {
	t.Helper()
	var response appIdentity.RegisterResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("respuesta no es JSON valido: %v (%s)", err, rec.Body.String())
	}
	return response
}

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) httputil.ErrorEnvelope {
	t.Helper()
	var envelope httputil.ErrorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("error no es JSON valido: %v (%s)", err, rec.Body.String())
	}
	return envelope
}

func TestRegisterHandlerCreated(t *testing.T) {
	f := newHandlerFixture(t)
	rec := serveRegister(f, registerRequest(validRegisterBody, "0123456789abcdef"))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status esperado 201, obtenido %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Location"); got != "/api/v1/users/me" {
		t.Fatalf("Location inesperado: %q", got)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("Content-Type inesperado: %q", got)
	}
	if got := rec.Header().Get("X-Idempotent-Replay"); got != "" {
		t.Fatalf("la primera respuesta no debe marcarse como replay: %q", got)
	}

	response := decodeRegisterResponse(t, rec)
	if response.User.Email != "usuario@example.invalid" || response.User.Name != "Usuario Ejemplo" {
		t.Fatalf("usuario inesperado: %+v", response.User)
	}
	if response.User.EmailVerified {
		t.Fatal("emailVerified debe ser false")
	}
	if response.User.CreatedAt != "2026-11-10T12:00:00Z" {
		t.Fatalf("createdAt inesperado: %q", response.User.CreatedAt)
	}
	if response.Income.Amount != "12000.00" || response.Income.MonthlyEquivalent != "12000.00" {
		t.Fatalf("ingreso inesperado: %+v", response.Income)
	}
	if len(response.RecurringPayments) != 1 {
		t.Fatalf("compromisos esperados 1, hay %d", len(response.RecurringPayments))
	}
	if response.Consent.NoticeVersion != "2026-11" || !response.Consent.AcceptedByServer {
		t.Fatalf("consentimiento inesperado: %+v", response.Consent)
	}
	if response.Onboarding.Configured || response.Onboarding.Microphone != "pending" {
		t.Fatalf("onboarding inesperado: %+v", response.Onboarding)
	}
	if f.limiter.class != "auth_register" || f.limiter.calls != 1 {
		t.Fatalf("rate limit inesperado: clase %q, llamadas %d", f.limiter.class, f.limiter.calls)
	}
}

func TestRegisterHandlerSinGastosDevuelveListaVacia(t *testing.T) {
	f := newHandlerFixture(t)
	rec := serveRegister(f, registerRequest(emptyExpensesRegisterBody, "0123456789abcdef"))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status esperado 201, obtenido %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"recurringPayments":[]`) {
		t.Fatalf("recurringPayments debe serializarse como []: %s", rec.Body.String())
	}
	response := decodeRegisterResponse(t, rec)
	if response.Income.Frequency != "fortnightly" || response.Income.MonthlyEquivalent != "19001.00" {
		t.Fatalf("equivalente mensual inesperado: %+v", response.Income)
	}
}

func TestRegisterHandlerReplay(t *testing.T) {
	f := newHandlerFixture(t)
	key := "0123456789abcdef"
	first := serveRegister(f, registerRequest(validRegisterBody, key))
	second := serveRegister(f, registerRequest(validRegisterBody, key))

	if first.Code != http.StatusCreated || second.Code != http.StatusCreated {
		t.Fatalf("status inesperados: %d y %d", first.Code, second.Code)
	}
	if second.Header().Get("X-Idempotent-Replay") != "true" {
		t.Fatal("el replay debe marcarse con X-Idempotent-Replay")
	}
	if first.Body.String() != second.Body.String() {
		t.Fatalf("replay no estable:\n%s\n%s", first.Body.String(), second.Body.String())
	}
}

func TestRegisterHandlerMismaClaveOtraContrasena(t *testing.T) {
	f := newHandlerFixture(t)
	key := "0123456789abcdef"
	if rec := serveRegister(f, registerRequest(validRegisterBody, key)); rec.Code != http.StatusCreated {
		t.Fatalf("primer registro: %d", rec.Code)
	}
	other := strings.Replace(validRegisterBody, syntheticPhrase, "OtraPassphrase#2026", 1)
	rec := serveRegister(f, registerRequest(other, key))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status esperado 409, obtenido %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRegisterHandlerMissingIdempotencyKey(t *testing.T) {
	f := newHandlerFixture(t)
	rec := serveRegister(f, registerRequest(validRegisterBody, ""))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status esperado 400, obtenido %d", rec.Code)
	}
	env := decodeError(t, rec)
	if env.Error.Code != "INVALID_REQUEST" {
		t.Fatalf("codigo inesperado: %q", env.Error.Code)
	}
	if len(env.Error.Details) != 1 || env.Error.Details[0].Field != "Idempotency-Key" {
		t.Fatalf("detalle inesperado: %+v", env.Error.Details)
	}
}

func TestRegisterHandlerRejectsInvalidContentType(t *testing.T) {
	f := newHandlerFixture(t)
	req := registerRequest(validRegisterBody, "0123456789abcdef")
	req.Header.Set("Content-Type", "text/plain")
	rec := serveRegister(f, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status esperado 400, obtenido %d", rec.Code)
	}
	env := decodeError(t, rec)
	if len(env.Error.Details) != 1 || env.Error.Details[0].Field != "Content-Type" {
		t.Fatalf("detalle inesperado: %+v", env.Error.Details)
	}
}

func TestRegisterHandlerMalformedJSON(t *testing.T) {
	f := newHandlerFixture(t)
	rec := serveRegister(f, registerRequest(`{"name":`, "0123456789abcdef"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status esperado 400, obtenido %d", rec.Code)
	}
	env := decodeError(t, rec)
	if env.Error.Code != "INVALID_REQUEST" {
		t.Fatalf("codigo inesperado: %q", env.Error.Code)
	}
}

func TestRegisterHandlerRejectsUnknownField(t *testing.T) {
	f := newHandlerFixture(t)
	body := strings.TrimSuffix(validRegisterBody, "}") + `,"campoExtra":true}`
	rec := serveRegister(f, registerRequest(body, "0123456789abcdef"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status esperado 400, obtenido %d", rec.Code)
	}
}

// `signupConfirm` lo compara Android en local y nunca debe enviarse al servidor.
func TestRegisterHandlerRejectsSignupConfirm(t *testing.T) {
	f := newHandlerFixture(t)
	body := strings.TrimSuffix(validRegisterBody, "}") + `,"signupConfirm":"` + syntheticPhrase + `"}`
	rec := serveRegister(f, registerRequest(body, "0123456789abcdef"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status esperado 400, obtenido %d", rec.Code)
	}
}

// Los campos heredados de H06 ya no forman parte del contrato publico.
func TestRegisterHandlerRejectsLegacyFields(t *testing.T) {
	f := newHandlerFixture(t)
	for _, sufijo := range []string{
		`,"displayName":"Otro"`,
		`,"privacyAccepted":true`,
		`,"privacyNoticeVersion":"2026-11"`,
		`,"signupIncomeMonthly":12000`,
		`,"signupFixedMonthly":699`,
		`,"userId":"018f90ef-08e5-4a68-b09f-000000000001"`,
		`,"birthDate":"1990-01-01"`,
		`,"phone":"5555555555"`,
	} {
		body := strings.TrimSuffix(validRegisterBody, "}") + sufijo + "}"
		rec := serveRegister(f, registerRequest(body, "0123456789abcdef"))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("campo heredado %q debía rechazarse, status %d", sufijo, rec.Code)
		}
	}
}

func TestRegisterHandlerRejectsWrongType(t *testing.T) {
	f := newHandlerFixture(t)
	casos := []string{
		strings.Replace(validRegisterBody, `"accepted":true`, `"accepted":"true"`, 1),
		// Importe numerico: el contrato exige string decimal exacto.
		strings.Replace(validRegisterBody, `"amount":"12000.00"`, `"amount":12000.00`, 1),
		// Un decimal con un solo digito tampoco es valido.
		strings.Replace(validRegisterBody, `"amount":"699.00"`, `"amount":"699.0"`, 1),
	}
	for _, body := range casos {
		rec := serveRegister(f, registerRequest(body, "0123456789abcdef"))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("payload %q debía rechazarse, status %d: %s", body, rec.Code, rec.Body.String())
		}
	}
}

func TestRegisterHandlerPrivacyNotAccepted(t *testing.T) {
	f := newHandlerFixture(t)
	body := strings.Replace(validRegisterBody, `"accepted":true`, `"accepted":false`, 1)
	rec := serveRegister(f, registerRequest(body, "0123456789abcdef"))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status esperado 422, obtenido %d: %s", rec.Code, rec.Body.String())
	}
	env := decodeError(t, rec)
	if env.Error.Code != "BUSINESS_RULE_VIOLATION" {
		t.Fatalf("codigo inesperado: %q", env.Error.Code)
	}
	if len(env.Error.Details) != 1 || env.Error.Details[0].Field != "privacy.accepted" {
		t.Fatalf("detalle inesperado: %+v", env.Error.Details)
	}
}

func TestRegisterHandlerBloquesObligatoriosAusentes(t *testing.T) {
	f := newHandlerFixture(t)
	casos := []struct {
		body  string
		field string
	}{
		{`{"name":"X","email":"a@b.invalid","password":"` + syntheticPhrase + `","recurringExpenses":[],"privacy":{"accepted":true,"noticeVersion":"2026-11"}}`, "income"},
		{`{"name":"X","email":"a@b.invalid","password":"` + syntheticPhrase + `","income":{"amount":"1.00","frequency":"monthly"},"privacy":{"accepted":true,"noticeVersion":"2026-11"}}`, "recurringExpenses"},
		{`{"name":"X","email":"a@b.invalid","password":"` + syntheticPhrase + `","income":{"amount":"1.00","frequency":"monthly"},"recurringExpenses":[]}`, "privacy"},
		{`{"name":"X","email":"a@b.invalid","password":"` + syntheticPhrase + `","income":{"frequency":"monthly"},"recurringExpenses":[],"privacy":{"accepted":true,"noticeVersion":"2026-11"}}`, "income.amount"},
		{`{"name":"X","email":"a@b.invalid","password":"` + syntheticPhrase + `","income":{"amount":"1.00"},"recurringExpenses":[],"privacy":{"accepted":true,"noticeVersion":"2026-11"}}`, "income.frequency"},
	}
	for _, caso := range casos {
		rec := serveRegister(f, registerRequest(caso.body, "0123456789abcdef"))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("campo %s ausente debía dar 400, status %d: %s", caso.field, rec.Code, rec.Body.String())
		}
		env := decodeError(t, rec)
		if len(env.Error.Details) != 1 || env.Error.Details[0].Field != caso.field {
			t.Fatalf("campo esperado %q, detalle %+v", caso.field, env.Error.Details)
		}
	}
}

func TestRegisterHandlerShortPassword(t *testing.T) {
	f := newHandlerFixture(t)
	body := strings.Replace(validRegisterBody, `"`+syntheticPhrase+`"`, `"corta"`, 1)
	rec := serveRegister(f, registerRequest(body, "0123456789abcdef"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status esperado 400, obtenido %d: %s", rec.Code, rec.Body.String())
	}
	env := decodeError(t, rec)
	if len(env.Error.Details) != 1 || env.Error.Details[0].Field != "password" {
		t.Fatalf("detalle inesperado: %+v", env.Error.Details)
	}
	if strings.Contains(rec.Body.String(), "corta") {
		t.Fatal("la respuesta no debe repetir la contrasena")
	}
}

func TestRegisterHandlerGastoIncompleto(t *testing.T) {
	f := newHandlerFixture(t)
	body := strings.Replace(validRegisterBody,
		`{"concept":"Internet","amount":"699.00","frequency":"monthly","category":"Servicios","type":"fixed"}`,
		`{"concept":"Internet","amount":"699.00","frequency":"monthly","category":"Servicios"}`, 1)
	rec := serveRegister(f, registerRequest(body, "0123456789abcdef"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status esperado 400, obtenido %d: %s", rec.Code, rec.Body.String())
	}
	env := decodeError(t, rec)
	if len(env.Error.Details) != 1 || env.Error.Details[0].Field != "recurringExpenses[0].type" {
		t.Fatalf("detalle inesperado: %+v", env.Error.Details)
	}
}

func TestRegisterHandlerCategoriaYTipoInvalidos(t *testing.T) {
	f := newHandlerFixture(t)
	categoria := strings.Replace(validRegisterBody, `"category":"Servicios"`, `"category":"Mascotas"`, 1)
	rec := serveRegister(f, registerRequest(categoria, "0123456789abcdef"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("categoria invalida: status %d", rec.Code)
	}
	if env := decodeError(t, rec); env.Error.Details[0].Field != "recurringExpenses[0].category" {
		t.Fatalf("detalle inesperado: %+v", env.Error.Details)
	}

	tipo := strings.Replace(validRegisterBody, `"type":"fixed"`, `"type":"variable"`, 1)
	rec = serveRegister(f, registerRequest(tipo, "0123456789abcdef"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("tipo invalido: status %d", rec.Code)
	}
	if env := decodeError(t, rec); env.Error.Details[0].Field != "recurringExpenses[0].type" {
		t.Fatalf("detalle inesperado: %+v", env.Error.Details)
	}
}

func TestRegisterHandlerImporteCero(t *testing.T) {
	f := newHandlerFixture(t)
	body := strings.Replace(validRegisterBody, `"amount":"12000.00"`, `"amount":"0.00"`, 1)
	rec := serveRegister(f, registerRequest(body, "0123456789abcdef"))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status esperado 422, obtenido %d: %s", rec.Code, rec.Body.String())
	}
	if env := decodeError(t, rec); env.Error.Details[0].Field != "income.amount" {
		t.Fatalf("detalle inesperado: %+v", env.Error.Details)
	}
}

func TestRegisterHandlerDuplicateEmail(t *testing.T) {
	f := newHandlerFixture(t)
	first := serveRegister(f, registerRequest(validRegisterBody, "0123456789abcdef"))
	if first.Code != http.StatusCreated {
		t.Fatalf("primer registro: %d", first.Code)
	}

	rec := serveRegister(f, registerRequest(validRegisterBody, "fedcba9876543210"))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status esperado 409, obtenido %d: %s", rec.Code, rec.Body.String())
	}
	env := decodeError(t, rec)
	if env.Error.Code != "CONFLICT" {
		t.Fatalf("codigo inesperado: %q", env.Error.Code)
	}
	if strings.Contains(rec.Body.String(), "usuario@example.invalid") {
		t.Fatal("el error de conflicto no debe repetir el correo")
	}
}

func TestRegisterHandlerIdempotencyConflict(t *testing.T) {
	f := newHandlerFixture(t)
	key := "0123456789abcdef"
	if rec := serveRegister(f, registerRequest(validRegisterBody, key)); rec.Code != http.StatusCreated {
		t.Fatalf("primer registro: %d", rec.Code)
	}

	other := strings.Replace(validRegisterBody, "Usuario Ejemplo", "Otro Nombre", 1)
	rec := serveRegister(f, registerRequest(other, key))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status esperado 409, obtenido %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRegisterHandlerStoreUnavailable(t *testing.T) {
	f := newHandlerFixture(t)
	f.store.SetUnavailable(true)
	rec := serveRegister(f, registerRequest(validRegisterBody, "0123456789abcdef"))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status esperado 503, obtenido %d", rec.Code)
	}
	env := decodeError(t, rec)
	if env.Error.Code != "DEPENDENCY_UNAVAILABLE" {
		t.Fatalf("codigo inesperado: %q", env.Error.Code)
	}
}

func TestRegisterHandlerRateLimited(t *testing.T) {
	f := newHandlerFixture(t)
	f.limiter.allowed = false
	f.limiter.retryAfter = 30 * time.Second
	rec := serveRegister(f, registerRequest(validRegisterBody, "0123456789abcdef"))

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status esperado 429, obtenido %d", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got != "30" {
		t.Fatalf("Retry-After esperado 30, obtenido %q", got)
	}
	env := decodeError(t, rec)
	if env.Error.Code != "RATE_LIMITED" {
		t.Fatalf("codigo inesperado: %q", env.Error.Code)
	}
	// El rate limit se aplica antes de tocar la persistencia.
	if f.store.RecordsCount() != 0 {
		t.Fatal("una peticion limitada no debe persistir nada")
	}
}

func TestRegisterHandlerRateLimiterError(t *testing.T) {
	f := newHandlerFixture(t)
	f.limiter.err = errors.New("redis caido")
	rec := serveRegister(f, registerRequest(validRegisterBody, "0123456789abcdef"))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status esperado 503, obtenido %d", rec.Code)
	}
}

func TestRegisterHandlerNilLimiterFailsClosed(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	svc := appIdentity.NewRegisterService(store, handlerHasher{})
	handler := internalhttp.NewRegisterHandler(svc, nil, slog.New(slog.NewJSONHandler(io.Discard, nil)))

	req := registerRequest(validRegisterBody, "0123456789abcdef")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status esperado 503, obtenido %d", rec.Code)
	}
}

func TestRegisterHandlerTraceIDInError(t *testing.T) {
	f := newHandlerFixture(t)
	req := registerRequest(`{"name":`, "0123456789abcdef")
	req.Header.Set("x-request-id", "trace-explicito-001")
	rec := serveRegister(f, req)

	env := decodeError(t, rec)
	if env.Error.TraceID != "trace-explicito-001" {
		t.Fatalf("traceId esperado trace-explicito-001, obtenido %q", env.Error.TraceID)
	}
}

func TestRegisterHandlerOversizedBody(t *testing.T) {
	f := newHandlerFixture(t)
	body := `{"email":"` + strings.Repeat("a", 2*1024*1024) + `"}`
	rec := serveRegister(f, registerRequest(body, "0123456789abcdef"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status esperado 400, obtenido %d", rec.Code)
	}
}

func TestRegisterErrorDetailContract(t *testing.T) {
	f := newHandlerFixture(t)
	rec := serveRegister(f, registerRequest(`{"name":`, "0123456789abcdef"))

	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("JSON invalido: %v", err)
	}
	errObj, ok := raw["error"].(map[string]any)
	if !ok {
		t.Fatalf("falta error: %v", raw)
	}
	details, ok := errObj["details"].([]any)
	if !ok || len(details) == 0 {
		t.Fatalf("falta details: %v", errObj)
	}
	detail, ok := details[0].(map[string]any)
	if !ok {
		t.Fatalf("detalle invalido: %v", details[0])
	}
	if _, ok := detail["field"]; !ok {
		t.Fatalf("el detalle no contiene field: %v", detail)
	}
	if _, ok := detail["reason"]; !ok {
		t.Fatalf("el detalle no contiene reason: %v", detail)
	}
	if _, ok := detail["issue"]; ok {
		t.Fatalf("el detalle usa issue en lugar de reason: %v", detail)
	}
}
