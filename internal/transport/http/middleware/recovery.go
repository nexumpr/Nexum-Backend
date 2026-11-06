package middleware

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
	"github.com/nexumpr/Nexum-Backend/pkg/telemetry"
)

// Recovery captura panics no controlados en la cadena HTTP (A-030 §2.3, AC-A-030-02).
// Evita la caída del proceso servidor y responde 500 JSON sin filtrar trazas ni secretos.
func Recovery(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					ctx := r.Context()
					errStr := fmt.Sprintf("%v", rec)

					attrs := telemetry.ContextAttrs(ctx)
					attrs = append(attrs, slog.String("panic", errStr))
					logger.LogAttrs(ctx, slog.LevelError, "panico no controlado en handler HTTP recuperado", attrs...)

					// Respuesta controlada sin exponer stack trace
					httputil.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "ocurrio un error inesperado al procesar la solicitud")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

