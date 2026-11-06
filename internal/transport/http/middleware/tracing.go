package middleware

import (
	"net/http"

	"github.com/nexumpr/Nexum-Backend/internal/infrastructure/telemetry"
)

// TraceMiddleware extrae el traceparent W3C y X-Request-ID, los inyecta en el contexto
// y agrega los encabezados correspondientes en la respuesta HTTP.
func TraceMiddleware() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			span := telemetry.ExtractOrGenerateTraceContext(r)
			span.Name = r.Method + " " + r.URL.Path
			span.SetAttribute("http.method", r.Method)
			span.SetAttribute("http.path", r.URL.Path)

			// Propagar en cabeceras de respuesta
			w.Header().Set(telemetry.TraceParentHeader, span.W3CTraceparent())
			w.Header().Set(telemetry.RequestIDHeader, span.RequestID)

			ctx := telemetry.InjectTraceContext(r.Context(), span)
			defer span.End()

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
