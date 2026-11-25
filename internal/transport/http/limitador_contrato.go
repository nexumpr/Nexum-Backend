package http

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
)

// LimitarPorUsuarioContrato aplica la cuota por usuario autenticado y emite el código
// `RATE_LIMITED` exigido por la política H06 y por el OpenAPI de los endpoints de analítica.
//
// El helper genérico `LimitarPorUsuario` usa `TOO_MANY_REQUESTS` por compatibilidad con
// endpoints anteriores; este adaptador no lo modifica y sólo se usa donde el contrato
// declara explícitamente `RATE_LIMITED`. La cuota sigue siendo fail-open ante fallo de
// Redis: es un control de abuso, no una dependencia de datos.
func LimitarPorUsuarioContrato(limitador LimitadorRuta, clase string, next http.HandlerFunc) http.HandlerFunc {
	if limitador == nil {
		return next
	}
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := middleware.UserIDFromContext(r.Context())
		if !ok || strings.TrimSpace(userID) == "" {
			next(w, r)
			return
		}
		permitido, retryAfter, err := limitador.Allow(r.Context(), clase, userID)
		if err != nil || permitido {
			next(w, r)
			return
		}
		segundos := int64(retryAfter / time.Second)
		if segundos < 1 {
			segundos = 1
		}
		w.Header().Set("Retry-After", strconv.FormatInt(segundos, 10))
		writeRegisterError(w, r, http.StatusTooManyRequests, "RATE_LIMITED", "demasiadas solicitudes", nil)
	}
}
