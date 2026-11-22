package http

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/nexumpr/Nexum-Backend/internal/application/analytics"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// AnalyticsInsightsHandler sirve E052 (`GET /api/v1/analytics/insights`).
//
// La identidad se deriva del `sub` verificado y se comprueba contra la sesión autoritativa;
// el cliente no puede elegir titular. La caché se desactiva siempre porque la respuesta
// contiene información financiera del usuario.
type AnalyticsInsightsHandler struct {
	service   *analytics.Service
	logger    *slog.Logger
	protected http.Handler
}

// NewAnalyticsInsightsHandler construye el handler. Sin servicio, tokens o autorizador,
// todas las peticiones responden 503 (fail-closed). El limitador es opcional.
func NewAnalyticsInsightsHandler(
	service *analytics.Service,
	tokens ports.TokenProvider,
	sessions ports.SessionAuthorizer,
	limitador LimitadorRuta,
	logger *slog.Logger,
) *AnalyticsInsightsHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &AnalyticsInsightsHandler{service: service, logger: logger}
	if service == nil || tokens == nil || sessions == nil {
		return h
	}
	autenticado := middleware.AuthenticateBearerWithSession(tokens, sessions)(http.HandlerFunc(h.serve))
	h.protected = LimitarPorUsuario(limitador, "analytics", autenticado.ServeHTTP)
	return h
}

// ServeHTTP fija la cabecera de caché antes de cualquier decisión y admite sólo GET.
func (h *AnalyticsInsightsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeRegisterError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "el metodo HTTP no esta permitido para este recurso", nil)
		return
	}
	if h == nil || h.service == nil || h.protected == nil {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}
	h.protected.ServeHTTP(w, r)
}

func (h *AnalyticsInsightsHandler) serve(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}

	pagina, err := h.service.Insights(r.Context(), userID, r.URL.Query().Get("period"))
	if err != nil {
		writeAnalyticsError(w, r, h.logger, err)
		return
	}
	if err := httputil.WriteJSON(w, http.StatusOK, pagina.Wire()); err != nil {
		h.logger.ErrorContext(r.Context(), "E052: fallo al serializar", slog.String("trace_id", traceID(r)))
	}
}

func writeAnalyticsError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	switch {
	case errors.Is(err, analytics.ErrInvalidRequest):
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{{Field: "period", Reason: "invalid"}})
	case errors.Is(err, analytics.ErrUnauthorized):
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
	case errors.Is(err, analytics.ErrUnavailable):
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
	default:
		if logger != nil {
			logger.ErrorContext(r.Context(), "E052: error no clasificado", slog.String("error_class", "internal"), slog.String("trace_id", traceID(r)))
		}
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
	}
}
