package transport_test

import (
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

// Pruebas HTTP de E011: ruta pública (sin Bearer), 200 con esquema exacto, 400/409/422/429/503
// y ausencia de fuga del token en cuerpo y cabeceras.

const (
	ev011Usuario = "11111111-2222-4333-8444-555555555555"
	ev011Token   = "token-sintetico-de-verificacion-0123456789"
)

var ev011Ahora = time.Date(2026, 11, 17, 11, 0, 0, 0, time.UTC)

type ev011Store struct {
	token      *appIdentity.EmailVerificationTokenRecord
	profile    *appIdentity.EmailVerificationProfile
	confirmErr error
	applied    []appIdentity.EmailVerificationConfirmation
}

func (s *ev011Store) LoadEmailVerificationProfile(context.Context, string) (*appIdentity.EmailVerificationProfile, error) {
	return s.profile, nil
}

func (s *ev011Store) SaveEmailVerificationToken(context.Context, appIdentity.EmailVerificationToken) error {
	return nil
}

func (s *ev011Store) LoadEmailVerificationToken(context.Context, string) (*appIdentity.EmailVerificationTokenRecord, error) {
	return s.token, nil
}

func (s *ev011Store) ConfirmEmailVerification(_ context.Context, confirmation appIdentity.EmailVerificationConfirmation) error {
	s.applied = append(s.applied, confirmation)
	return s.confirmErr
}

type ev011Limiter struct {
	allow    bool
	retry    time.Duration
	err      error
	llamadas int
}

func (l *ev011Limiter) AllowEmailVerificationConfirm(_ context.Context, ip string) (bool, time.Duration, error) {
	l.llamadas++
	if ip == "" {
		return false, 0, errors.New("ip vacia")
	}
	return l.allow, l.retry, l.err
}

func ev011StoreValido() *ev011Store {
	return &ev011Store{
		token: &appIdentity.EmailVerificationTokenRecord{
			UserID:     ev011Usuario,
			Email:      "verificacion@example.invalid",
			Purpose:    appIdentity.EmailVerificationPurpose,
			EntityType: appIdentity.EmailVerificationEntityType,
			ExpiresAt:  ev011Ahora.Add(10 * time.Minute),
			HasExpiry:  true,
		},
		profile: &appIdentity.EmailVerificationProfile{
			UserID:     ev011Usuario,
			Email:      "verificacion@example.invalid",
			Status:     appIdentity.StatusActive,
			Version:    3,
			HasVersion: true,
		},
	}
}

func ev011Handler(store *ev011Store, limiter nexumhttp.EmailVerificationConfirmLimiter) *nexumhttp.EmailVerificationConfirmHandler {
	servicio := appIdentity.NewEmailVerificationConfirmService(store).
		WithClock(func() time.Time { return ev011Ahora })
	return nexumhttp.NewEmailVerificationConfirmHandler(servicio, limiter, silencioso())
}

func ev011Peticion(cuerpo string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/email-verification/confirm", strings.NewReader(cuerpo))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "10.42.1.10:443"
	req.Header.Set("X-Forwarded-For", "203.0.113.10, 130.176.10.20")
	return req
}

func ev011Ejecutar(t *testing.T, handler http.Handler, req *http.Request) (*httptest.ResponseRecorder, map[string]any) {
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

func TestE011HTTP200EsquemaExactoYPublico(t *testing.T) {
	store := ev011StoreValido()
	limiter := &ev011Limiter{allow: true}
	handler := ev011Handler(store, limiter)

	req := ev011Peticion(`{"token":"` + ev011Token + `"}`)
	if req.Header.Get("Authorization") != "" {
		t.Fatalf("la prueba debe ejecutarse sin Bearer")
	}
	rec, cuerpo := ev011Ejecutar(t, handler, req)
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
		t.Fatalf("Accepted solo admite status y requestId, obtenido %v", cuerpo)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("la respuesta no puede cachearse")
	}
	if len(store.applied) != 1 || limiter.llamadas != 1 {
		t.Fatalf("debe evaluarse la cuota y aplicarse la transaccion una vez")
	}
	if strings.Contains(rec.Body.String(), ev011Token) {
		t.Fatalf("la respuesta no puede contener el token")
	}
	for nombre, valores := range rec.Header() {
		for _, valor := range valores {
			if strings.Contains(valor, ev011Token) {
				t.Fatalf("la cabecera %q no puede filtrar el token", nombre)
			}
		}
	}
}

func TestE011HTTP400DelContrato(t *testing.T) {
	casos := []struct {
		nombre string
		cuerpo string
	}{
		{"json malformado", `{"token":`},
		{"campo desconocido", `{"token":"` + ev011Token + `","email":"otro@example.invalid"}`},
		{"tipo incorrecto", `{"token":123}`},
		{"token corto", `{"token":"123456789"}`},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := ev011StoreValido()
			handler := ev011Handler(store, &ev011Limiter{allow: true})
			rec, respuesta := ev011Ejecutar(t, handler, ev011Peticion(caso.cuerpo))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("esperado 400, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			errorObj, _ := respuesta["error"].(map[string]any)
			if errorObj["code"] != "INVALID_REQUEST" {
				t.Fatalf("codigo esperado INVALID_REQUEST, obtenido %v", errorObj["code"])
			}
			if len(store.applied) != 0 {
				t.Fatalf("un cuerpo invalido no puede aplicar la transaccion")
			}
		})
	}
}

func TestE011HTTP422TokenVacio(t *testing.T) {
	handler := ev011Handler(ev011StoreValido(), &ev011Limiter{allow: true})
	rec, respuesta := ev011Ejecutar(t, handler, ev011Peticion(`{"token":""}`))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("esperado 422, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	errorObj, _ := respuesta["error"].(map[string]any)
	if errorObj["code"] != "BUSINESS_RULE_VIOLATION" {
		t.Fatalf("codigo esperado BUSINESS_RULE_VIOLATION, obtenido %v", errorObj["code"])
	}
}

func TestE011HTTPErroresDelContrato(t *testing.T) {
	casos := []struct {
		nombre   string
		preparar func(*ev011Store, *ev011Limiter)
		esperado int
		codigo   string
	}{
		{"replay", func(s *ev011Store, _ *ev011Limiter) {
			s.token.Used = true
		}, http.StatusConflict, "CONFLICT"},
		{"token inexistente", func(s *ev011Store, _ *ev011Limiter) {
			s.token = nil
		}, http.StatusBadRequest, "INVALID_REQUEST"},
		{"token expirado", func(s *ev011Store, _ *ev011Limiter) {
			s.token.ExpiresAt = ev011Ahora.Add(-time.Minute)
		}, http.StatusBadRequest, "INVALID_REQUEST"},
		{"perfil inactivo", func(s *ev011Store, _ *ev011Limiter) {
			s.profile.Status = "DISABLED"
		}, http.StatusBadRequest, "INVALID_REQUEST"},
		{"conflicto de transaccion", func(s *ev011Store, _ *ev011Limiter) {
			s.confirmErr = appIdentity.ErrEmailVerificationConflicto
		}, http.StatusConflict, "CONFLICT"},
		{"cuota agotada", func(_ *ev011Store, l *ev011Limiter) {
			l.allow = false
			l.retry = 15 * time.Minute
		}, http.StatusTooManyRequests, "RATE_LIMITED"},
		{"store caido", func(s *ev011Store, _ *ev011Limiter) {
			s.confirmErr = appIdentity.ErrEmailVerificationConfirmNoDisponible
		}, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE"},
		{"redis caido", func(_ *ev011Store, l *ev011Limiter) {
			l.err = errors.New("redis caido")
		}, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := ev011StoreValido()
			limiter := &ev011Limiter{allow: true}
			caso.preparar(store, limiter)
			handler := ev011Handler(store, limiter)

			rec, respuesta := ev011Ejecutar(t, handler, ev011Peticion(`{"token":"`+ev011Token+`"}`))
			if rec.Code != caso.esperado {
				t.Fatalf("esperado %d, obtenido %d (%s)", caso.esperado, rec.Code, rec.Body.String())
			}
			errorObj, _ := respuesta["error"].(map[string]any)
			if errorObj["code"] != caso.codigo {
				t.Fatalf("codigo esperado %q, obtenido %v", caso.codigo, errorObj["code"])
			}
			if caso.nombre == "cuota agotada" && rec.Header().Get("Retry-After") != "900" {
				t.Fatalf("429 debe indicar Retry-After, obtenido %q", rec.Header().Get("Retry-After"))
			}
		})
	}
}

func TestE011HTTP503SinDependencias(t *testing.T) {
	casos := map[string]*nexumhttp.EmailVerificationConfirmHandler{
		"sin servicio": nexumhttp.NewEmailVerificationConfirmHandler(nil, &ev011Limiter{allow: true}, silencioso()),
		"sin limitador": func() *nexumhttp.EmailVerificationConfirmHandler {
			servicio := appIdentity.NewEmailVerificationConfirmService(ev011StoreValido())
			return nexumhttp.NewEmailVerificationConfirmHandler(servicio, nil, silencioso())
		}(),
	}
	for nombre, handler := range casos {
		t.Run(nombre, func(t *testing.T) {
			rec, _ := ev011Ejecutar(t, handler, ev011Peticion(`{"token":"`+ev011Token+`"}`))
			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("esperado 503 fail-closed, obtenido %d", rec.Code)
			}
		})
	}
}
