package middleware

import (
	"net/http"
	"strings"

	"github.com/nexumpr/Nexum-Backend/internal/infrastructure/telemetry"
)

// TraceIDForResponse devuelve el identificador de correlación que debe viajar en el
// `traceId` del `ErrorEnvelope` de las respuestas emitidas por el middleware.
//
// Orden de resolución, idéntico al que usan los handlers de transporte:
//  1. `TraceSpan.TraceID` inyectado por `TraceMiddleware` (parte trace-id del W3C
//     `traceparent` recibido o generado para esta petición);
//  2. `TraceSpan.RequestID` (cabecera `X-Request-ID` recibida o generada);
//  3. cabecera `X-Request-ID` de la petición, si el middleware de rastreo no se ejecutó;
//  4. cabecera heredada `X-Trace-ID`.
//
// Sin ninguna de ellas devuelve cadena vacía: el envelope no inventa un identificador que
// el cliente no pueda correlacionar con `traceparent`/`X-Request-ID`.
func TraceIDForResponse(r *http.Request) string {
	if r == nil {
		return ""
	}
	if span := telemetry.FromContext(r.Context()); span != nil {
		if span.TraceID != "" {
			return span.TraceID
		}
		if span.RequestID != "" {
			return span.RequestID
		}
	}
	if id := strings.TrimSpace(r.Header.Get(telemetry.RequestIDHeader)); id != "" {
		return id
	}
	return strings.TrimSpace(r.Header.Get("X-Trace-ID"))
}
