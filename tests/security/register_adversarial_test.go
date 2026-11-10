package security_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
	memorystore "github.com/nexumpr/Nexum-Backend/tests/support/memorystore"
)

type securityHasher struct{}

func (securityHasher) HashPassword(password string) (string, error) { return "hashed:" + password, nil }
func (securityHasher) VerifyPassword(password, hash string) (bool, error) {
	return hash == "hashed:"+password, nil
}

type securityLimiter struct{}

func (securityLimiter) Allow(context.Context, string, string) (bool, time.Duration, error) {
	return true, 0, nil
}

const syntheticPhrase = "ExamplePassphrase#2026"

const secureRegisterBody = `{"email":"seguridad@example.invalid","password":"` + syntheticPhrase + `","displayName":"Seguridad","privacyNoticeVersion":"2026-11","privacyAccepted":true}`

func newSecurityHandler(t *testing.T, store appIdentity.RegisterStore, logger *slog.Logger) *internalhttp.RegisterHandler {
	t.Helper()
	svc := appIdentity.NewRegisterService(store, securityHasher{}).
		WithClock(func() time.Time { return time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC) })
	return internalhttp.NewRegisterHandler(svc, securityLimiter{}, logger)
}

func securityRequest(body, key string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)
	req.Header.Set("x-request-id", "security-e001")
	return req
}

func TestRegisterRejectsInjectionInEmail(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	handler := newSecurityHandler(t, store, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	body := strings.Replace(secureRegisterBody, "seguridad@example.invalid", "a@b.invalid' OR '1'='1", 1)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, securityRequest(body, "0123456789abcdef"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status esperado 400, obtenido %d: %s", rec.Code, rec.Body.String())
	}
	if store.RecordsCount() != 0 {
		t.Fatal("una entrada inyectada no debe crear usuario")
	}
}

func TestRegisterDoesNotEchoPassword(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	handler := newSecurityHandler(t, store, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	body := strings.Replace(secureRegisterBody, syntheticPhrase, "corta", 1)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, securityRequest(body, "0123456789abcdef"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status esperado 400, obtenido %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "corta") {
		t.Fatalf("la respuesta echo la contrasena: %s", rec.Body.String())
	}
}

func TestRegisterRejectsControlCharactersInIdempotencyKey(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	handler := newSecurityHandler(t, store, slog.New(slog.NewJSONHandler(io.Discard, nil)))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, securityRequest(secureRegisterBody, "0123456789abcde\x00"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status esperado 400, obtenido %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRegisterConflictDoesNotRevealProfileData(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	handler := newSecurityHandler(t, store, slog.New(slog.NewJSONHandler(io.Discard, nil)))

	first := httptest.NewRecorder()
	handler.ServeHTTP(first, securityRequest(secureRegisterBody, "0123456789abcdef"))
	if first.Code != http.StatusCreated {
		t.Fatalf("primer registro: %d", first.Code)
	}

	second := httptest.NewRecorder()
	handler.ServeHTTP(second, securityRequest(secureRegisterBody, "fedcba9876543210"))
	if second.Code != http.StatusConflict {
		t.Fatalf("status esperado 409, obtenido %d", second.Code)
	}
	if strings.Contains(second.Body.String(), "seguridad@example.invalid") {
		t.Fatal("el conflicto no debe revelar el correo")
	}
	if strings.Contains(second.Body.String(), "Seguridad") {
		t.Fatal("el conflicto no debe revelar el nombre visible")
	}
}

func TestRegisterLogsDoNotContainSecrets(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	store.SetUnavailable(true)
	var logs bytes.Buffer
	handler := newSecurityHandler(t, store, slog.New(slog.NewJSONHandler(&logs, nil)))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, securityRequest(secureRegisterBody, "0123456789abcdef"))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status esperado 503, obtenido %d", rec.Code)
	}

	logText := logs.String()
	if strings.Contains(logText, "seguridad@example.invalid") {
		t.Fatalf("el log contiene el correo: %s", logText)
	}
	if strings.Contains(logText, syntheticPhrase) {
		t.Fatalf("el log contiene la contrasena: %s", logText)
	}
	if strings.Contains(logText, "hashed:") {
		t.Fatalf("el log contiene el hash: %s", logText)
	}
	if !strings.Contains(logText, "trace_id") {
		t.Fatalf("el log no incluye correlacion: %s", logText)
	}
}

func TestRegisterErrorEnvelopeHasNoRawError(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	store.SetUnavailable(true)
	handler := newSecurityHandler(t, store, slog.New(slog.NewJSONHandler(io.Discard, nil)))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, securityRequest(secureRegisterBody, "0123456789abcdef"))

	var envelope map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("JSON invalido: %v", err)
	}
	errObj, ok := envelope["error"].(map[string]any)
	if !ok {
		t.Fatalf("falta error: %v", envelope)
	}
	message, _ := errObj["message"].(string)
	if strings.Contains(strings.ToLower(message), "dynamo") || strings.Contains(strings.ToLower(message), "aws") {
		t.Fatalf("el mensaje expone infraestructura: %q", message)
	}
}
