package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nexumpr/Nexum-Backend/internal/ports"
)

func TestE074HTTPDesactivacionSeguraYConfiguracion(t *testing.T) {
	casos := []struct {
		nombre  string
		enabled bool
	}{
		{"desactivado por defecto", false},
		{"habilitado por configuracion", true},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			handler := NewDemoBootstrapHandler(caso.enabled, o5Tokens(), o5SesionesFalsas{}, nil, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, o5Peticion(http.MethodGet, o5RutaDemo, o5TokenA, ""))

			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d, esperado 200; cuerpo=%s", rec.Code, rec.Body.String())
			}
			if rec.Header().Get("Cache-Control") != "private, no-store" {
				t.Fatalf("Cache-Control inesperado: %q", rec.Header().Get("Cache-Control"))
			}
			var cuerpo struct {
				Enabled bool   `json:"enabled"`
				Scope   string `json:"scope"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
				t.Fatalf("cuerpo inválido: %v", err)
			}
			if cuerpo.Enabled != caso.enabled || cuerpo.Scope != "sandbox" {
				t.Fatalf("respuesta fuera del contrato: %+v", cuerpo)
			}
			campos := map[string]any{}
			if err := json.Unmarshal(rec.Body.Bytes(), &campos); err != nil {
				t.Fatalf("json.Unmarshal: %v", err)
			}
			if len(campos) != 2 {
				t.Fatalf("campos extra en la respuesta: %s", rec.Body.String())
			}
		})
	}
}

func TestE074HTTPUsuarioAYBEstableSinPII(t *testing.T) {
	handler := NewDemoBootstrapHandler(true, o5Tokens(), o5SesionesFalsas{}, nil, nil)
	respuestas := make([]string, 0, 4)
	for i := 0; i < 2; i++ {
		for _, token := range []string{o5TokenA, o5TokenB} {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, o5Peticion(http.MethodGet, o5RutaDemo, token, ""))
			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d; cuerpo=%s", rec.Code, rec.Body.String())
			}
			respuestas = append(respuestas, rec.Body.String())
		}
	}
	base := respuestas[0]
	for _, respuesta := range respuestas {
		if respuesta != base {
			t.Fatalf("la respuesta debe ser estable e independiente del usuario: %q vs %q", base, respuesta)
		}
		if strings.Contains(respuesta, o5UsuarioA) || strings.Contains(respuesta, o5UsuarioB) ||
			strings.Contains(respuesta, "sesion-o5") {
			t.Fatalf("la respuesta filtró identidad: %q", respuesta)
		}
	}
}

func TestE074HTTPAutenticacionYFailClosed(t *testing.T) {
	casos := []struct {
		nombre   string
		tokens   ports.TokenProvider
		sesiones ports.SessionAuthorizer
		token    string
		status   int
	}{
		{"sin token", o5Tokens(), o5SesionesFalsas{}, "", http.StatusUnauthorized},
		{"token desconocido", o5Tokens(), o5SesionesFalsas{}, "jwt-invalido", http.StatusUnauthorized},
		{"token sin sesion", o5Tokens(), o5SesionesFalsas{}, o5TokenSinSID, http.StatusUnauthorized},
		{"sesion revocada", o5Tokens(), o5SesionesFalsas{err: ports.ErrSessionUnauthorized}, o5TokenA, http.StatusUnauthorized},
		{"estado de sesion no disponible", o5Tokens(), o5SesionesFalsas{err: ports.ErrSessionUnavailable}, o5TokenA, http.StatusServiceUnavailable},
		{"sin proveedor de tokens", nil, o5SesionesFalsas{}, o5TokenA, http.StatusServiceUnavailable},
		{"sin autorizador de sesion", o5Tokens(), nil, o5TokenA, http.StatusServiceUnavailable},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			handler := NewDemoBootstrapHandler(true, caso.tokens, caso.sesiones, nil, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, o5Peticion(http.MethodGet, o5RutaDemo, caso.token, ""))
			if rec.Code != caso.status {
				t.Fatalf("status=%d, esperado %d; cuerpo=%s", rec.Code, caso.status, rec.Body.String())
			}
			if envelope := o5Envelope(t, rec); envelope.Error.Code == "" {
				t.Fatal("todo error debe usar ErrorEnvelope")
			}
		})
	}
}

func TestE074HTTPCuotaMetodoYParametros(t *testing.T) {
	limitado := NewDemoBootstrapHandler(true, o5Tokens(), o5SesionesFalsas{},
		o5LimitadorFalso{permitido: false, retry: 3 * time.Second}, nil)
	rec := httptest.NewRecorder()
	limitado.ServeHTTP(rec, o5Peticion(http.MethodGet, o5RutaDemo, o5TokenA, ""))
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "3" {
		t.Fatalf("status=%d Retry-After=%q; cuerpo=%s", rec.Code, rec.Header().Get("Retry-After"), rec.Body.String())
	}
	if envelope := o5Envelope(t, rec); envelope.Error.Code != "RATE_LIMITED" {
		t.Fatalf("code=%q, esperado RATE_LIMITED", envelope.Error.Code)
	}

	handler := NewDemoBootstrapHandler(true, o5Tokens(), o5SesionesFalsas{}, nil, nil)
	for _, caso := range []struct {
		nombre string
		req    *http.Request
	}{
		{"metodo no declarado", o5Peticion(http.MethodPost, o5RutaDemo, o5TokenA, "")},
		{"query no declarada", o5Peticion(http.MethodGet, o5RutaDemo+"?user=otro", o5TokenA, "")},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, caso.req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status=%d, esperado 400; cuerpo=%s", rec.Code, rec.Body.String())
			}
			if envelope := o5Envelope(t, rec); envelope.Error.Code != "INVALID_REQUEST" {
				t.Fatalf("code=%q, esperado INVALID_REQUEST", envelope.Error.Code)
			}
		})
	}
}
