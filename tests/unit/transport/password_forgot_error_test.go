package transport_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// Pruebas HTTP de E005 (`POST /api/v1/auth/password/forgot`): ErrorEnvelope canonico con
// `traceId`, 413 contractual, 400 de payload, 422 de correo y 503 de dependencia. Los dobles
// son exclusivamente de prueba; no hay AWS, SES ni Redis.

type e005Store struct {
	admitidos []ports.ResetAdmission
	err       error
}

func (s *e005Store) Admitir(_ context.Context, adm ports.ResetAdmission) error {
	if s.err != nil {
		return s.err
	}
	s.admitidos = append(s.admitidos, adm)
	return nil
}

func (s *e005Store) Pendientes(context.Context, time.Time, int) ([]ports.ResetTrabajo, error) {
	return nil, nil
}

func (s *e005Store) Reclamar(context.Context, string, string, time.Time, time.Duration) (int, bool, error) {
	return 0, false, nil
}

func (s *e005Store) MarcarEnviado(context.Context, ports.ResetTrabajo, string, time.Time) error {
	return nil
}

func (s *e005Store) Reintentar(context.Context, ports.ResetTrabajo, int, time.Time, time.Time, bool) error {
	return nil
}

// e005Correo implementa ports.Correo: E005 no envia desde la peticion, pero el servicio exige
// un correo real para considerarse disponible.
type e005Correo struct{}

func (e005Correo) EnviarRecuperacion(context.Context, string, string, time.Time) (string, error) {
	return "", errors.New("E005 no debe enviar dentro de la peticion")
}

// e005Sellador implementa ports.Sellador con un sobre opaco determinista.
type e005Sellador struct{}

func (e005Sellador) Sellar(token, aad string) ([]byte, error) {
	return []byte(aad + "|" + token), nil
}

func (e005Sellador) Abrir(sobre []byte, _ string) (string, error) { return string(sobre), nil }

// e005Limitador implementa ports.Limitador; `err` simula Redis no disponible.
type e005Limitador struct{ err error }

func (l e005Limitador) Permitir(context.Context, string) (bool, error) {
	if l.err != nil {
		return false, l.err
	}
	return true, nil
}

func e005Handler(t *testing.T, limitador ports.Limitador) *nexumhttp.PasswordForgotHandler {
	t.Helper()
	servicio := appIdentity.NewPasswordResetService(&e005Store{}, e005Correo{}, e005Sellador{}, limitador, nil)
	return nexumhttp.NewPasswordForgotHandler(servicio, nil, nil)
}

func e005Peticion(cuerpo string) *http.Request {
	peticion := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password/forgot", strings.NewReader(cuerpo))
	peticion.Header.Set("Content-Type", "application/json")
	peticion.Header.Set("X-Request-ID", traceEnvelopeRequestID)
	peticion.Header.Set("X-Forwarded-For", "203.0.113.7")
	return peticion
}

func e005Error(t *testing.T, rec *httptest.ResponseRecorder) httputil.ErrorPayload {
	t.Helper()
	var envelope httputil.ErrorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("el cuerpo no es un ErrorEnvelope JSON: %v (%q)", err, rec.Body.String())
	}
	if envelope.Error.TraceID != traceEnvelopeRequestID {
		t.Fatalf("todo error de E005 debe llevar el traceId de la peticion: %q", envelope.Error.TraceID)
	}
	if envelope.Error.Details == nil {
		t.Fatalf("details debe ser un arreglo, no null")
	}
	return envelope.Error
}

func TestE005ErrorEnvelopeCanonico(t *testing.T) {
	casos := []struct {
		nombre  string
		handler http.Handler
		cuerpo  string
		estado  int
		codigo  string
	}{
		{
			nombre:  "503 sin servicio",
			handler: nexumhttp.NewPasswordForgotHandler(nil, nil, nil),
			cuerpo:  `{"email":"cuenta@example.invalid"}`,
			estado:  http.StatusServiceUnavailable,
			codigo:  "DEPENDENCY_UNAVAILABLE",
		},
		{
			nombre:  "400 json malformado",
			handler: e005Handler(t, e005Limitador{}),
			cuerpo:  `{"email":`,
			estado:  http.StatusBadRequest,
			codigo:  "INVALID_REQUEST",
		},
		{
			nombre:  "400 campo desconocido",
			handler: e005Handler(t, e005Limitador{}),
			cuerpo:  `{"email":"cuenta@example.invalid","otro":true}`,
			estado:  http.StatusBadRequest,
			codigo:  "INVALID_REQUEST",
		},
		{
			nombre:  "422 correo invalido",
			handler: e005Handler(t, e005Limitador{}),
			cuerpo:  `{"email":"sin-arroba"}`,
			estado:  http.StatusUnprocessableEntity,
			codigo:  "BUSINESS_RULE_VIOLATION",
		},
		{
			nombre:  "503 dependencia no disponible",
			handler: e005Handler(t, e005Limitador{err: errors.New("redis caido")}),
			cuerpo:  `{"email":"cuenta@example.invalid"}`,
			estado:  http.StatusServiceUnavailable,
			codigo:  "DEPENDENCY_UNAVAILABLE",
		},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			caso.handler.ServeHTTP(rec, e005Peticion(caso.cuerpo))
			if rec.Code != caso.estado {
				t.Fatalf("esperado %d, obtenido %d (%s)", caso.estado, rec.Code, rec.Body.String())
			}
			if rec.Header().Get("Content-Type") != "application/json; charset=utf-8" {
				t.Fatalf("Content-Type inesperado: %q", rec.Header().Get("Content-Type"))
			}
			payload := e005Error(t, rec)
			if payload.Code != caso.codigo {
				t.Fatalf("codigo esperado %q, obtenido %q", caso.codigo, payload.Code)
			}
		})
	}
}

func TestE005CuerpoExcesivoDevuelve413(t *testing.T) {
	// 4 KiB es el limite contractual del handler; el campo email tiene 254 caracteres como
	// maximo, de modo que un cuerpo mayor solo puede ser abuso.
	cuerpo := `{"email":"` + strings.Repeat("a", 5000) + `@example.invalid"}`
	rec := httptest.NewRecorder()
	e005Handler(t, e005Limitador{}).ServeHTTP(rec, e005Peticion(cuerpo))

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("esperado 413, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	payload := e005Error(t, rec)
	if payload.Code != "PAYLOAD_TOO_LARGE" {
		t.Fatalf("codigo inesperado: %q", payload.Code)
	}
}

// TestE005CuerpoValidoNoSeAdmiteSinCorreo comprueba que un JSON valido llega al servicio solo
// cuando el correo cumple el formato: el correo invalido no crea trabajo persistente.
func TestE005CorreoInvalidoNoAdmite(t *testing.T) {
	store := &e005Store{}
	servicio := appIdentity.NewPasswordResetService(store, e005Correo{}, e005Sellador{}, e005Limitador{}, nil)
	handler := nexumhttp.NewPasswordForgotHandler(servicio, nil, nil)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, e005Peticion(`{"email":"sin-arroba"}`))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("esperado 422, obtenido %d", rec.Code)
	}
	if len(store.admitidos) != 0 {
		t.Fatal("un correo invalido no puede crear trabajo persistente")
	}
}
