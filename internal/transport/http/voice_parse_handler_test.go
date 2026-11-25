package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/nexumpr/Nexum-Backend/internal/application/voiceparse"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// Dobles de identidad de prueba: tokens literales (sin clave real) y sesiones
// autorizadas, siguiendo el patrón de los tests HTTP de la Oleada 4.
const (
	o5RutaVoz     = "/api/v1/voice/parse"
	o5RutaDemo    = "/api/v1/demo/bootstrap"
	o5UsuarioA    = "11111111-2222-4333-8444-555555555555"
	o5UsuarioB    = "99999999-8888-4777-8666-555555555555"
	o5TokenA      = "jwt-o5-a"
	o5TokenB      = "jwt-o5-b"
	o5TokenSinSID = "jwt-o5-sin-sesion"
)

type o5TokensFalsos struct {
	claims map[string]*ports.TokenClaims
}

func (p o5TokensFalsos) GenerateAccessToken(context.Context, ports.TokenClaims) (string, error) {
	return "", errors.New("no usado en la prueba")
}

func (p o5TokensFalsos) GenerateRefreshToken(context.Context) (string, error) {
	return "", errors.New("no usado en la prueba")
}

func (p o5TokensFalsos) ValidateAccessToken(_ context.Context, token string) (*ports.TokenClaims, error) {
	if claims, ok := p.claims[strings.TrimSpace(token)]; ok {
		copia := *claims
		return &copia, nil
	}
	return nil, ports.ErrInvalidToken
}

func o5Tokens() o5TokensFalsos {
	return o5TokensFalsos{claims: map[string]*ports.TokenClaims{
		o5TokenA:      {Subject: o5UsuarioA, SessionID: "sesion-o5-a"},
		o5TokenB:      {Subject: o5UsuarioB, SessionID: "sesion-o5-b"},
		o5TokenSinSID: {Subject: o5UsuarioA},
	}}
}

type o5SesionesFalsas struct {
	err error
}

func (s o5SesionesFalsas) AuthorizeSession(context.Context, string, string, int64) error {
	return s.err
}

type o5LimitadorFalso struct {
	permitido bool
	retry     time.Duration
	err       error
}

func (l o5LimitadorFalso) Allow(context.Context, string, string) (bool, time.Duration, error) {
	return l.permitido, l.retry, l.err
}

func o5Peticion(metodo, ruta, token, cuerpo string) *http.Request {
	req := httptest.NewRequest(metodo, ruta, strings.NewReader(cuerpo))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if cuerpo != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

func o5Logger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func o5Envelope(t *testing.T, rec *httptest.ResponseRecorder) httputil.ErrorEnvelope {
	t.Helper()
	var envelope httputil.ErrorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("la respuesta no es un ErrorEnvelope: %v; cuerpo=%s", err, rec.Body.String())
	}
	return envelope
}

func TestE075HTTPContratoTextoValido(t *testing.T) {
	handler := NewVoiceParseHandler(o5Tokens(), o5SesionesFalsas{}, nil, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, o5Peticion(http.MethodPost, o5RutaVoz, o5TokenA,
		`{"text":"Registrar gasto de cien pesos","locale":"es-MX"}`))

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, esperado 200; cuerpo=%s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("Cache-Control inesperado: %q", rec.Header().Get("Cache-Control"))
	}
	if !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("Content-Type inesperado: %q", rec.Header().Get("Content-Type"))
	}

	var resultado voiceparse.VoiceParseWire
	if err := json.Unmarshal(rec.Body.Bytes(), &resultado); err != nil {
		t.Fatalf("cuerpo inválido: %v", err)
	}
	if resultado.Intent != voiceparse.IntentCreateMovement || resultado.ParsedAmount != "100.00" ||
		!resultado.RequiresConfirmation || resultado.Currency != "MXN" {
		t.Fatalf("respuesta fuera del contrato: %+v", resultado)
	}

	campos := map[string]any{}
	if err := json.Unmarshal(rec.Body.Bytes(), &campos); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if len(campos) != 4 {
		t.Fatalf("campos extra en la respuesta: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), o5UsuarioA) {
		t.Fatal("la respuesta no debe exponer el sujeto autenticado")
	}
}

func TestE075HTTPErroresDeRequest(t *testing.T) {
	textoLargo := fmt.Sprintf(`{"text":"%s","locale":"es-MX"}`, strings.Repeat("a", voiceparse.MaxTextRunes+1))
	casos := []struct {
		nombre  string
		cuerpo  string
		status  int
		code    string
		headers map[string]string
	}{
		{"json invalido", `{`, http.StatusBadRequest, "INVALID_REQUEST", nil},
		{"campo desconocido", `{"text":"Gasto de 100","locale":"es-MX","extra":1}`, http.StatusBadRequest, "INVALID_REQUEST", nil},
		{"locale distinto", `{"text":"Gasto de 100","locale":"en-US"}`, http.StatusBadRequest, "INVALID_REQUEST", nil},
		{"texto vacio", `{"text":"","locale":"es-MX"}`, http.StatusBadRequest, "INVALID_REQUEST", nil},
		{"texto demasiado largo", textoLargo, http.StatusBadRequest, "INVALID_REQUEST", nil},
		{"sin cuerpo", "", http.StatusBadRequest, "INVALID_REQUEST", nil},
		{"content type invalido", `{"text":"Gasto de 100","locale":"es-MX"}`, http.StatusBadRequest, "INVALID_REQUEST", map[string]string{"Content-Type": "text/plain"}},
		{"monto negativo", `{"text":"Gasto de -100 pesos","locale":"es-MX"}`, http.StatusUnprocessableEntity, "BUSINESS_RULE_VIOLATION", nil},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			handler := NewVoiceParseHandler(o5Tokens(), o5SesionesFalsas{}, nil, nil)
			req := o5Peticion(http.MethodPost, o5RutaVoz, o5TokenA, caso.cuerpo)
			for clave, valor := range caso.headers {
				req.Header.Set(clave, valor)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != caso.status {
				t.Fatalf("status=%d, esperado %d; cuerpo=%s", rec.Code, caso.status, rec.Body.String())
			}
			if envelope := o5Envelope(t, rec); envelope.Error.Code != caso.code {
				t.Fatalf("code=%q, esperado %q; envelope=%+v", envelope.Error.Code, caso.code, envelope)
			}
		})
	}
}

func TestE075HTTPAutenticacionYFailClosed(t *testing.T) {
	cuerpo := `{"text":"Gasto de 100 pesos","locale":"es-MX"}`
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
			handler := NewVoiceParseHandler(caso.tokens, caso.sesiones, nil, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, o5Peticion(http.MethodPost, o5RutaVoz, caso.token, cuerpo))
			if rec.Code != caso.status {
				t.Fatalf("status=%d, esperado %d; cuerpo=%s", rec.Code, caso.status, rec.Body.String())
			}
			if envelope := o5Envelope(t, rec); envelope.Error.Code == "" {
				t.Fatal("todo error debe usar ErrorEnvelope")
			}
		})
	}
}

func TestE075HTTPUsuarioAYBSinEfectosNiLogsDeTexto(t *testing.T) {
	const secreto = "dato-sensible-o5"
	cuerpo := `{"text":"Registrar gasto de cien pesos ` + secreto + `","locale":"es-MX"}`
	var buf bytes.Buffer
	handler := NewVoiceParseHandler(o5Tokens(), o5SesionesFalsas{}, nil, o5Logger(&buf))

	respuestas := make([]string, 0, 2)
	for _, token := range []string{o5TokenA, o5TokenB} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, o5Peticion(http.MethodPost, o5RutaVoz, token, cuerpo))
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d; cuerpo=%s", rec.Code, rec.Body.String())
		}
		respuestas = append(respuestas, rec.Body.String())
	}
	if respuestas[0] != respuestas[1] {
		t.Fatalf("usuarios distintos deben obtener la misma respuesta: %q vs %q", respuestas[0], respuestas[1])
	}
	for _, respuesta := range respuestas {
		if strings.Contains(respuesta, secreto) || strings.Contains(respuesta, o5UsuarioA) || strings.Contains(respuesta, o5UsuarioB) {
			t.Fatalf("la respuesta filtró texto del usuario o identidad: %q", respuesta)
		}
	}
	if strings.Contains(buf.String(), secreto) {
		t.Fatalf("el logger registró texto del usuario: %q", buf.String())
	}
	if reflect.TypeOf(voiceparse.Service{}).NumField() != 0 {
		t.Fatal("el servicio de parsing no debe tener dependencias con efectos")
	}

	// La ruta de error tampoco debe registrar el texto sensible.
	buf.Reset()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, o5Peticion(http.MethodPost, o5RutaVoz, o5TokenA,
		`{"text":"Gasto de -100 `+secreto+`","locale":"es-MX"}`))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d; cuerpo=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(buf.String(), secreto) {
		t.Fatalf("el logger registró texto del usuario en la ruta de error: %q", buf.String())
	}
}

func TestE075HTTPCuotaYMetodo(t *testing.T) {
	cuerpo := `{"text":"Gasto de 100 pesos","locale":"es-MX"}`

	limitado := NewVoiceParseHandler(o5Tokens(), o5SesionesFalsas{},
		o5LimitadorFalso{permitido: false, retry: 2 * time.Second}, nil)
	rec := httptest.NewRecorder()
	limitado.ServeHTTP(rec, o5Peticion(http.MethodPost, o5RutaVoz, o5TokenA, cuerpo))
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "2" {
		t.Fatalf("status=%d Retry-After=%q; cuerpo=%s", rec.Code, rec.Header().Get("Retry-After"), rec.Body.String())
	}
	if envelope := o5Envelope(t, rec); envelope.Error.Code != "RATE_LIMITED" {
		t.Fatalf("code=%q, esperado RATE_LIMITED", envelope.Error.Code)
	}

	falloAbierto := NewVoiceParseHandler(o5Tokens(), o5SesionesFalsas{},
		o5LimitadorFalso{err: errors.New("redis caido")}, nil)
	rec = httptest.NewRecorder()
	falloAbierto.ServeHTTP(rec, o5Peticion(http.MethodPost, o5RutaVoz, o5TokenA, cuerpo))
	if rec.Code != http.StatusOK {
		t.Fatalf("un fallo de cuota no debe tumbar el endpoint: %d", rec.Code)
	}

	handler := NewVoiceParseHandler(o5Tokens(), o5SesionesFalsas{}, nil, nil)
	for _, caso := range []struct {
		nombre string
		req    *http.Request
	}{
		{"metodo no declarado", o5Peticion(http.MethodGet, o5RutaVoz, o5TokenA, "")},
		{"query no declarada", o5Peticion(http.MethodPost, o5RutaVoz+"?x=1", o5TokenA, cuerpo)},
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
