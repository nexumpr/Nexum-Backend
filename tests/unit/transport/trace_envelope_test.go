package transport_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// Pruebas de correlacion del ErrorEnvelope: el `traceId` de los errores emitidos por el
// middleware de autenticacion debe salir del contexto de rastreo (W3C `traceparent` /
// `X-Request-ID`) y no de la cabecera no contractual `X-Trace-ID`, que era la causa del
// `"traceId": ""` observado en produccion.

const (
	traceEnvelopeTraceID   = "4bf92f3577b34da6a3ce929d0e0e4736"
	traceEnvelopeParentID  = "00f067aa0ba902b7"
	traceEnvelopeRequestID = "req-abc-123"
)

func traceEnvelopeError(t *testing.T, rec *httptest.ResponseRecorder) httputil.ErrorPayload {
	t.Helper()
	var envelope httputil.ErrorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("el cuerpo no es un ErrorEnvelope JSON: %v (%q)", err, rec.Body.String())
	}
	return envelope.Error
}

// traceEnvelopeCadena reproduce la composicion de produccion que importa aqui: rastreo global
// seguido del middleware de autenticacion.
func traceEnvelopeCadena(t *testing.T, tokens ports.TokenProvider, autorizador ports.SessionAuthorizer) http.Handler {
	t.Helper()
	protegido := middleware.AuthenticateBearerWithSession(tokens, autorizador)(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) },
	))
	return middleware.TraceMiddleware()(protegido)
}

func TestErrorEnvelopeTraceIDDesdeTraceparent(t *testing.T) {
	cadena := traceEnvelopeCadena(t, &e004Tokens{}, newE004Sesiones())
	peticion := httptest.NewRequest(http.MethodGet, "/protegido", nil)
	peticion.Header.Set("traceparent", "00-"+traceEnvelopeTraceID+"-"+traceEnvelopeParentID+"-01")
	rec := httptest.NewRecorder()

	cadena.ServeHTTP(rec, peticion)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("sin Authorization se esperaba 401, obtenido %d", rec.Code)
	}
	payload := traceEnvelopeError(t, rec)
	if payload.TraceID != traceEnvelopeTraceID {
		t.Fatalf("traceId debe ser el trace-id W3C del contexto: esperado %q, obtenido %q", traceEnvelopeTraceID, payload.TraceID)
	}
	if rec.Header().Get("X-Request-ID") == "" {
		t.Fatal("la respuesta debe propagar X-Request-ID")
	}
}

func TestErrorEnvelopeTraceIDDesdeRequestID(t *testing.T) {
	cadena := traceEnvelopeCadena(t, &e004Tokens{}, newE004Sesiones())
	peticion := httptest.NewRequest(http.MethodGet, "/protegido", nil)
	peticion.Header.Set("X-Request-ID", traceEnvelopeRequestID)
	rec := httptest.NewRecorder()

	cadena.ServeHTTP(rec, peticion)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("sin Authorization se esperaba 401, obtenido %d", rec.Code)
	}
	payload := traceEnvelopeError(t, rec)
	if payload.TraceID == "" {
		t.Fatal("con X-Request-ID el traceId no puede quedar vacio")
	}
	// Sin `traceparent` el middleware genera un trace-id W3C; el envelope debe llevar ese
	// mismo valor para que el cliente correlacione con la cabecera `traceparent` de respuesta.
	respuesta := rec.Header().Get("traceparent")
	partes := strings.Split(respuesta, "-")
	if len(partes) != 4 || partes[1] != payload.TraceID {
		t.Fatalf("traceId %q no coincide con el traceparent de respuesta %q", payload.TraceID, respuesta)
	}
	if rec.Header().Get("X-Request-ID") != traceEnvelopeRequestID {
		t.Fatalf("X-Request-ID debe conservarse: %q", rec.Header().Get("X-Request-ID"))
	}
}

func TestErrorEnvelopeTraceIDEn503DeSesion(t *testing.T) {
	tokens := e004TokensValidos()
	sesiones := newE004Sesiones()
	sesiones.fallo = ports.ErrSessionUnavailable
	cadena := traceEnvelopeCadena(t, tokens, sesiones)

	peticion := httptest.NewRequest(http.MethodGet, "/protegido", nil)
	peticion.Header.Set("traceparent", "00-"+traceEnvelopeTraceID+"-"+traceEnvelopeParentID+"-01")
	peticion.Header.Set("Authorization", "Bearer jwt-valido")
	rec := httptest.NewRecorder()

	cadena.ServeHTTP(rec, peticion)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("un fallo de sesion debe ser 503, obtenido %d", rec.Code)
	}
	payload := traceEnvelopeError(t, rec)
	if payload.Code != "DEPENDENCY_UNAVAILABLE" {
		t.Fatalf("codigo inesperado: %q", payload.Code)
	}
	if payload.TraceID != traceEnvelopeTraceID {
		t.Fatalf("el 503 tambien debe llevar traceId: esperado %q, obtenido %q", traceEnvelopeTraceID, payload.TraceID)
	}
}

// TestTraceIDForResponsePrecedencia fija el orden documentado del helper sin depender del
// middleware completo: contexto > X-Request-ID > X-Trace-ID > vacio.
func TestTraceIDForResponsePrecedencia(t *testing.T) {
	casos := []struct {
		nombre string
		header [][2]string
		espera string
	}{
		{"sin cabeceras", nil, ""},
		{"solo heredada", [][2]string{{"X-Trace-ID", "heredado-1"}}, "heredado-1"},
		{"request-id gana a heredada", [][2]string{{"X-Request-ID", "req-1"}, {"X-Trace-ID", "heredado-1"}}, "req-1"},
		{"espacios se recortan", [][2]string{{"X-Request-ID", "  req-2  "}}, "req-2"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			peticion := httptest.NewRequest(http.MethodGet, "/protegido", nil)
			for _, cabecera := range caso.header {
				peticion.Header.Set(cabecera[0], cabecera[1])
			}
			if obtenido := middleware.TraceIDForResponse(peticion); obtenido != caso.espera {
				t.Fatalf("esperado %q, obtenido %q", caso.espera, obtenido)
			}
		})
	}
}
