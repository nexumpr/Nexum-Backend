package transport_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// Pruebas HTTP de E006: contrato exacto (200 Accepted), rechazos 400/409/422/429/503 y
// ausencia de fugas del token o de la contraseña en cuerpos y cabeceras.

type e006Limiter struct {
	allow    bool
	retry    time.Duration
	err      error
	llamadas int
}

func (l *e006Limiter) AllowResetConfirm(_ context.Context, ip string) (bool, time.Duration, error) {
	l.llamadas++
	if ip == "" {
		return false, 0, errors.New("ip vacia")
	}
	return l.allow, l.retry, l.err
}

type e006Store struct {
	token    *appIdentity.ResetTokenRecord
	profile  *appIdentity.ResetProfile
	applyErr error
	aplicado *appIdentity.ResetApplication
}

func (s *e006Store) LoadResetToken(context.Context, string, time.Time) (*appIdentity.ResetTokenRecord, error) {
	return s.token, nil
}

func (s *e006Store) LoadResetProfile(context.Context, string) (*appIdentity.ResetProfile, error) {
	return s.profile, nil
}

func (s *e006Store) ApplyReset(_ context.Context, apply appIdentity.ResetApplication) error {
	s.aplicado = &apply
	return s.applyErr
}

type e006Hasher struct{}

func (e006Hasher) HashPassword(password string) (string, error) { return "hash:" + password, nil }
func (e006Hasher) VerifyPassword(password, hash string) (bool, error) {
	return hash == "hash:"+password, nil
}

func e006StoreValido(ahora time.Time) *e006Store {
	return &e006Store{
		token: &appIdentity.ResetTokenRecord{
			UserID: "usuario-1", ExpiresAt: ahora.Add(10 * time.Minute), HasExpiry: true,
		},
		profile: &appIdentity.ResetProfile{
			UserID: "usuario-1", Status: appIdentity.StatusActive, Version: 1, HasVersion: true,
		},
	}
}

func e006Handler(store *e006Store, limiter nexumhttp.PasswordResetConfirmLimiter) *nexumhttp.PasswordResetConfirmHandler {
	servicio := appIdentity.NewPasswordResetConfirmService(store, e006Hasher{})
	return nexumhttp.NewPasswordResetConfirmHandler(servicio, limiter, nil)
}

func e006Peticion(cuerpo string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password/reset", bytes.NewBufferString(cuerpo))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "10.42.1.10:443"
	req.Header.Set("X-Forwarded-For", "203.0.113.10, 130.176.10.20")
	return req
}

func e006Ejecutar(t *testing.T, handler http.Handler, req *http.Request) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	var cuerpo map[string]any
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
			t.Fatalf("respuesta no es JSON valido: %v (%s)", err, rec.Body.String())
		}
	}
	return rec, cuerpo
}

func TestE006HTTP200Accepted(t *testing.T) {
	store := e006StoreValido(time.Now().UTC())
	limiter := &e006Limiter{allow: true}
	handler := e006Handler(store, limiter)

	rec, cuerpo := e006Ejecutar(t, handler, e006Peticion(`{"token":"token-valido-0123456789","newPassword":"NewPassphrase#2026"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if cuerpo["status"] != "accepted" {
		t.Fatalf("status esperado accepted, obtenido %v", cuerpo["status"])
	}
	id, _ := cuerpo["requestId"].(string)
	if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(id) {
		t.Fatalf("requestId debe ser UUID v4, obtenido %q", id)
	}
	if len(cuerpo) != 2 {
		t.Fatalf("el contrato Accepted solo admite status y requestId, obtenido %v", cuerpo)
	}
	if store.aplicado == nil {
		t.Fatalf("debe aplicarse la transaccion")
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("la respuesta no puede cachearse")
	}
	if limiter.llamadas != 1 {
		t.Fatalf("la cuota debe evaluarse una vez, obtenido %d", limiter.llamadas)
	}
}

func TestE006HTTPRechazosDelContrato(t *testing.T) {
	casos := []struct {
		nombre   string
		cuerpo   string
		espera   int
		codigo   string
		preparar func(*e006Store, *e006Limiter)
	}{
		{"json malformado", `{"token":`, http.StatusBadRequest, "INVALID_REQUEST", nil},
		{"campo desconocido", `{"token":"token-valido-0123456789","newPassword":"NewPassphrase#2026","resetToken":"x"}`, http.StatusBadRequest, "INVALID_REQUEST", nil},
		{"tipo incorrecto", `{"token":123,"newPassword":"NewPassphrase#2026"}`, http.StatusBadRequest, "INVALID_REQUEST", nil},
		{"password corta", `{"token":"token-valido-0123456789","newPassword":"corta"}`, http.StatusUnprocessableEntity, "BUSINESS_RULE_VIOLATION", nil},
		{"token vacio", `{"token":"","newPassword":"NewPassphrase#2026"}`, http.StatusUnprocessableEntity, "BUSINESS_RULE_VIOLATION", nil},
		{"token usado", `{"token":"token-valido-0123456789","newPassword":"NewPassphrase#2026"}`, http.StatusConflict, "CONFLICT", func(s *e006Store, _ *e006Limiter) {
			s.token.Used = true
		}},
		{"token inexistente", `{"token":"token-valido-0123456789","newPassword":"NewPassphrase#2026"}`, http.StatusBadRequest, "INVALID_REQUEST", func(s *e006Store, _ *e006Limiter) {
			s.token = nil
		}},
		{"token expirado", `{"token":"token-valido-0123456789","newPassword":"NewPassphrase#2026"}`, http.StatusBadRequest, "INVALID_REQUEST", func(s *e006Store, _ *e006Limiter) {
			s.token.ExpiresAt = time.Now().UTC().Add(-time.Minute)
		}},
		{"cuota agotada", `{"token":"token-valido-0123456789","newPassword":"NewPassphrase#2026"}`, http.StatusTooManyRequests, "RATE_LIMITED", func(_ *e006Store, l *e006Limiter) {
			l.allow = false
			l.retry = 15 * time.Minute
		}},
		{"limitador caido", `{"token":"token-valido-0123456789","newPassword":"NewPassphrase#2026"}`, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", func(_ *e006Store, l *e006Limiter) {
			l.err = errors.New("redis caido")
		}},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := e006StoreValido(time.Now().UTC())
			limiter := &e006Limiter{allow: true}
			if caso.preparar != nil {
				caso.preparar(store, limiter)
			}
			rec, cuerpo := e006Ejecutar(t, e006Handler(store, limiter), e006Peticion(caso.cuerpo))
			if rec.Code != caso.espera {
				t.Fatalf("esperado %d, obtenido %d (%s)", caso.espera, rec.Code, rec.Body.String())
			}
			errorObj, _ := cuerpo["error"].(map[string]any)
			if errorObj == nil || errorObj["code"] != caso.codigo {
				t.Fatalf("codigo esperado %q, obtenido %v", caso.codigo, cuerpo)
			}
			if strings.Contains(rec.Body.String(), "token-valido-0123456789") || strings.Contains(rec.Body.String(), "NewPassphrase#2026") {
				t.Fatalf("la respuesta no puede filtrar el token ni la contrasena")
			}
			if caso.espera == http.StatusTooManyRequests && rec.Header().Get("Retry-After") != "900" {
				t.Fatalf("Retry-After esperado 900, obtenido %q", rec.Header().Get("Retry-After"))
			}
		})
	}
}

func TestE006HTTPFallaCerradoSinComponentes(t *testing.T) {
	// Servicio sin store: 503, nunca 200 ficticio.
	servicio := appIdentity.NewPasswordResetConfirmService(nil, nil)
	handler := nexumhttp.NewPasswordResetConfirmHandler(servicio, &e006Limiter{allow: true}, nil)
	rec, _ := e006Ejecutar(t, handler, e006Peticion(`{"token":"token-valido-0123456789","newPassword":"NewPassphrase#2026"}`))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("esperado 503, obtenido %d", rec.Code)
	}

	// Servicio valido sin limitador: 503 (no se opera sin cuota).
	store := e006StoreValido(time.Now().UTC())
	handler = e006Handler(store, nil)
	rec, _ = e006Ejecutar(t, handler, e006Peticion(`{"token":"token-valido-0123456789","newPassword":"NewPassphrase#2026"}`))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("sin limitador se esperaba 503, obtenido %d", rec.Code)
	}
	if store.aplicado != nil {
		t.Fatalf("sin limitador no se puede aplicar la transaccion")
	}
}

func TestE006HTTPContentTypeYIP(t *testing.T) {
	store := e006StoreValido(time.Now().UTC())
	handler := e006Handler(store, &e006Limiter{allow: true})

	sinTipo := e006Peticion(`{"token":"token-valido-0123456789","newPassword":"NewPassphrase#2026"}`)
	sinTipo.Header.Del("Content-Type")
	rec, _ := e006Ejecutar(t, handler, sinTipo)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("sin Content-Type se esperaba 400, obtenido %d", rec.Code)
	}

	ipInvalida := e006Peticion(`{"token":"token-valido-0123456789","newPassword":"NewPassphrase#2026"}`)
	ipInvalida.Header.Set("X-Forwarded-For", "no-es-una-ip")
	rec, _ = e006Ejecutar(t, handler, ipInvalida)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("cadena de proxies invalida debe ser 400, obtenido %d", rec.Code)
	}
	if store.aplicado != nil {
		t.Fatalf("una peticion invalida no puede aplicar la transaccion")
	}
}

func TestE006HTTPErrorInternoNoFiltra(t *testing.T) {
	store := e006StoreValido(time.Now().UTC())
	store.applyErr = appIdentity.ErrResetConfirmUnavailable
	rec, cuerpo := e006Ejecutar(t, e006Handler(store, &e006Limiter{allow: true}), e006Peticion(`{"token":"token-valido-0123456789","newPassword":"NewPassphrase#2026"}`))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("esperado 503, obtenido %d", rec.Code)
	}
	errorObj, _ := cuerpo["error"].(map[string]any)
	if errorObj == nil || errorObj["code"] != "DEPENDENCY_UNAVAILABLE" {
		t.Fatalf("codigo esperado DEPENDENCY_UNAVAILABLE, obtenido %v", cuerpo)
	}
	// El cuerpo de error no incluye detalles internos ni el token.
	if detalles, ok := errorObj["details"].([]any); !ok || len(detalles) != 0 {
		t.Fatalf("details debe ser una lista vacia, obtenido %v", errorObj["details"])
	}
}
