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
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// Pruebas HTTP de E010: 202 con esquema exacto, 401 sin Bearer (middleware real), 400 si se
// envia cuerpo, 409/404/403/429/503 y ausencia de fuga del token.

type ev010Store struct {
	profile *appIdentity.EmailVerificationProfile
	err     error
	saved   []appIdentity.EmailVerificationToken
}

func (s *ev010Store) LoadEmailVerificationProfile(context.Context, string) (*appIdentity.EmailVerificationProfile, error) {
	return s.profile, s.err
}

func (s *ev010Store) SaveEmailVerificationToken(_ context.Context, token appIdentity.EmailVerificationToken) error {
	s.saved = append(s.saved, token)
	return nil
}

type ev010Mailer struct {
	err      error
	llamadas int
	token    string
	destino  string
}

func (m *ev010Mailer) EnviarVerificacion(_ context.Context, destino, token string, _ time.Time) (string, error) {
	m.llamadas++
	m.destino = destino
	m.token = token
	if m.err != nil {
		return "", m.err
	}
	return "mensaje-sintetico", nil
}

type ev010Limiter struct {
	allow    bool
	err      error
	llamadas int
}

func (l *ev010Limiter) Permitir(context.Context, string) (bool, error) {
	l.llamadas++
	if l.err != nil {
		return false, l.err
	}
	return l.allow, nil
}

func ev010Perfil() *appIdentity.EmailVerificationProfile {
	return &appIdentity.EmailVerificationProfile{
		UserID: e004HTTPUsuario,
		Email:  "verificacion@example.invalid",
		Status: appIdentity.StatusActive,
	}
}

func ev010Handler(store *ev010Store, correo *ev010Mailer, limitador *ev010Limiter) *nexumhttp.EmailVerificationRequestHandler {
	var (
		storeI     appIdentity.EmailVerificationStore  = store
		correoI    appIdentity.EmailVerificationMailer = correo
		limitadorI ports.Limitador                     = limitador
	)
	servicio := appIdentity.NewEmailVerificationRequestService(storeI, correoI, limitadorI, silencioso())
	return nexumhttp.NewEmailVerificationRequestHandler(servicio, e004TokensValidos(), newE004Sesiones(), silencioso())
}

func ev010Ejecutar(t *testing.T, handler http.Handler, req *http.Request) (*httptest.ResponseRecorder, map[string]any) {
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

func TestE010HTTP202EsquemaExacto(t *testing.T) {
	store := &ev010Store{profile: ev010Perfil()}
	correo := &ev010Mailer{}
	limitador := &ev010Limiter{allow: true}
	handler := ev010Handler(store, correo, limitador)

	rec, cuerpo := ev010Ejecutar(t, handler, e004Peticion(http.MethodPost, "/api/v1/auth/email-verification/request", "jwt-valido", ""))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("esperado 202, obtenido %d (%s)", rec.Code, rec.Body.String())
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
	if len(store.saved) != 1 || correo.llamadas != 1 {
		t.Fatalf("debe persistirse y enviarse un unico token")
	}
	if strings.Contains(rec.Body.String(), correo.token) || strings.Contains(rec.Body.String(), store.saved[0].TokenHash) {
		t.Fatalf("la respuesta no puede contener el token ni su digest")
	}
	for nombre, valores := range rec.Header() {
		for _, valor := range valores {
			if correo.token != "" && strings.Contains(valor, correo.token) {
				t.Fatalf("la cabecera %q no puede filtrar el token", nombre)
			}
		}
	}
}

func TestE010HTTP401SinBearer(t *testing.T) {
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
			correo := &ev010Mailer{}
			handler := ev010Handler(&ev010Store{profile: ev010Perfil()}, correo, &ev010Limiter{allow: true})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/email-verification/request", strings.NewReader(""))
			if caso.cabecera != "" {
				req.Header.Set("Authorization", caso.cabecera)
			}
			rec, _ := ev010Ejecutar(t, handler, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("esperado 401, obtenido %d", rec.Code)
			}
			if correo.llamadas != 0 {
				t.Fatalf("sin identidad verificada no puede enviarse ningun correo")
			}
		})
	}
}

func TestE010HTTP400SiSeEnviaCuerpo(t *testing.T) {
	correo := &ev010Mailer{}
	handler := ev010Handler(&ev010Store{profile: ev010Perfil()}, correo, &ev010Limiter{allow: true})

	for _, cuerpo := range []string{`{}`, `{"email":"otro@example.invalid"}`} {
		req := e004Peticion(http.MethodPost, "/api/v1/auth/email-verification/request", "jwt-valido", cuerpo)
		rec, respuesta := ev010Ejecutar(t, handler, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("el contrato no admite cuerpo: esperado 400, obtenido %d (%s)", rec.Code, rec.Body.String())
		}
		errorObj, _ := respuesta["error"].(map[string]any)
		if errorObj["code"] != "INVALID_REQUEST" {
			t.Fatalf("codigo esperado INVALID_REQUEST, obtenido %v", errorObj["code"])
		}
	}
	if correo.llamadas != 0 {
		t.Fatalf("un cuerpo rechazado no puede enviar correo")
	}
}

func TestE010HTTPErroresDelContrato(t *testing.T) {
	casos := []struct {
		nombre   string
		preparar func(*ev010Store, *ev010Mailer, *ev010Limiter)
		esperado int
		codigo   string
	}{
		{"ya verificado", func(s *ev010Store, _ *ev010Mailer, _ *ev010Limiter) {
			s.profile.EmailVerified = true
		}, http.StatusConflict, "CONFLICT"},
		{"perfil ausente", func(s *ev010Store, _ *ev010Mailer, _ *ev010Limiter) {
			s.profile = nil
		}, http.StatusNotFound, "NOT_FOUND"},
		{"cuenta inactiva", func(s *ev010Store, _ *ev010Mailer, _ *ev010Limiter) {
			s.profile.Status = "DISABLED"
		}, http.StatusForbidden, "FORBIDDEN"},
		{"cuota agotada", func(_ *ev010Store, _ *ev010Mailer, l *ev010Limiter) {
			l.allow = false
		}, http.StatusTooManyRequests, "RATE_LIMITED"},
		{"ses caido", func(_ *ev010Store, m *ev010Mailer, _ *ev010Limiter) {
			m.err = errors.New("ses rechazo el envio")
		}, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE"},
		{"redis caido", func(_ *ev010Store, _ *ev010Mailer, l *ev010Limiter) {
			l.err = errors.New("redis caido")
		}, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := &ev010Store{profile: ev010Perfil()}
			correo := &ev010Mailer{}
			limitador := &ev010Limiter{allow: true}
			caso.preparar(store, correo, limitador)
			handler := ev010Handler(store, correo, limitador)

			rec, respuesta := ev010Ejecutar(t, handler, e004Peticion(http.MethodPost, "/api/v1/auth/email-verification/request", "jwt-valido", ""))
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

func TestE010HTTP503SinDependencias(t *testing.T) {
	casos := map[string]*nexumhttp.EmailVerificationRequestHandler{
		"sin servicio": nexumhttp.NewEmailVerificationRequestHandler(nil, e004TokensValidos(), newE004Sesiones(), silencioso()),
		"sin limitador": func() *nexumhttp.EmailVerificationRequestHandler {
			servicio := appIdentity.NewEmailVerificationRequestService(
				&ev010Store{profile: ev010Perfil()}, &ev010Mailer{}, nil, silencioso())
			return nexumhttp.NewEmailVerificationRequestHandler(servicio, e004TokensValidos(), newE004Sesiones(), silencioso())
		}(),
		"sin autorizador": func() *nexumhttp.EmailVerificationRequestHandler {
			servicio := appIdentity.NewEmailVerificationRequestService(
				&ev010Store{profile: ev010Perfil()}, &ev010Mailer{}, &ev010Limiter{allow: true}, silencioso())
			return nexumhttp.NewEmailVerificationRequestHandler(servicio, e004TokensValidos(), nil, silencioso())
		}(),
	}
	for nombre, handler := range casos {
		t.Run(nombre, func(t *testing.T) {
			rec, _ := ev010Ejecutar(t, handler, e004Peticion(http.MethodPost, "/api/v1/auth/email-verification/request", "jwt-valido", ""))
			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("esperado 503 fail-closed, obtenido %d", rec.Code)
			}
		})
	}
}
