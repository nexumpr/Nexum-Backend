package telemetry_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	infraTelemetry "github.com/nexumpr/Nexum-Backend/internal/infrastructure/telemetry"
)

func peticionConTrace(traceparent string, requestID string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health/live", nil)
	if traceparent != "" {
		req.Header.Set(infraTelemetry.TraceParentHeader, traceparent)
	}
	if requestID != "" {
		req.Header.Set(infraTelemetry.RequestIDHeader, requestID)
	}
	return req
}

func TestTraceparentValidoSePropaga(t *testing.T) {
	trace := "4bf92f3577b34da6a3ce929d0e0e4736"
	span := infraTelemetry.ExtractOrGenerateTraceContext(
		peticionConTrace("00-"+trace+"-00f067aa0ba902b7-01", "req-1"),
	)
	if span.TraceID != trace {
		t.Errorf("traceId esperado %q, obtenido %q", trace, span.TraceID)
	}
	if span.RequestID != "req-1" {
		t.Errorf("requestId esperado req-1, obtenido %q", span.RequestID)
	}
	if len(span.SpanID) != 16 {
		t.Errorf("spanId debe ser 16 hex, obtenido %q", span.SpanID)
	}
}

func TestTraceparentInvalidoGeneraNuevo(t *testing.T) {
	for _, invalido := range []string{"basura", "00-corto-00f067aa0ba902b7-01", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7"} {
		span := infraTelemetry.ExtractOrGenerateTraceContext(peticionConTrace(invalido, ""))
		if len(span.TraceID) != 32 || strings.Contains(span.TraceID, "corto") {
			t.Errorf("cabecera %q: se esperaba traceId nuevo de 32 hex, obtenido %q", invalido, span.TraceID)
		}
	}
}

func TestCaracterizaDefectoA090HeadersNoValidados(t *testing.T) {
	// Caracteriza A-090: sólo se comprueban longitudes, no hexadecimalidad ni flags,
	// y X-Request-ID no tiene límite. No se corrige aquí (spec ajena).
	span := infraTelemetry.ExtractOrGenerateTraceContext(
		peticionConTrace("00-zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz-00f067aa0ba902b7-01", ""),
	)
	if span.TraceID != strings.Repeat("z", 32) {
		t.Errorf("se esperaba que el traceId no hex se aceptara (defecto A-090), obtenido %q", span.TraceID)
	}
	largo := strings.Repeat("a", 10000)
	span = infraTelemetry.ExtractOrGenerateTraceContext(peticionConTrace("", largo))
	if span.RequestID != largo {
		t.Errorf("se esperaba que X-Request-ID largo se propagara (defecto A-090)")
	}
}
