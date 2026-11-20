package http

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	appDashboard "github.com/nexumpr/Nexum-Backend/internal/application/dashboard"
	appSummary "github.com/nexumpr/Nexum-Backend/internal/application/financialsummary"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// DashboardHandler sirve E051 (`GET /api/v1/dashboard`).
type DashboardHandler struct {
	service   *appDashboard.Service
	logger    *slog.Logger
	protected http.Handler
}

// NewDashboardHandler construye el handler. Sin servicio/tokens/autorizador responde 503.
func NewDashboardHandler(service *appDashboard.Service, tokens ports.TokenProvider, sessions ports.SessionAuthorizer, limitador LimitadorRuta, logger *slog.Logger) *DashboardHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &DashboardHandler{service: service, logger: logger}
	if service != nil && tokens != nil && sessions != nil {
		protegido := middleware.AuthenticateBearerWithSession(tokens, sessions)(http.HandlerFunc(h.serve))
		h.protected = LimitarPorUsuario(limitador, "dashboard", protegido.ServeHTTP)
	}
	return h
}

// ServeHTTP fija caché privada y valida el método.
func (h *DashboardHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

func (h *DashboardHandler) serve(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}
	dashboard, err := h.service.Get(r.Context(), userID, r.URL.Query().Get("period"))
	if err != nil {
		writeDashboardError(w, r, h.logger, err)
		return
	}
	if err := httputil.WriteJSON(w, http.StatusOK, dashboard); err != nil {
		h.logger.ErrorContext(r.Context(), "dashboard: fallo al serializar", slog.String("trace_id", traceID(r)))
	}
}

func writeDashboardError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	switch {
	case errors.Is(err, appSummary.ErrInvalidPeriod):
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{{Field: "period", Reason: "invalid_format"}})
	case errors.Is(err, appDashboard.ErrUnauthorized), errors.Is(err, appSummary.ErrUnauthorized):
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
	case errors.Is(err, appDashboard.ErrInvalid), errors.Is(err, appSummary.ErrInconsistent), errors.Is(err, appSummary.ErrOverflow):
		if logger != nil {
			logger.ErrorContext(r.Context(), "dashboard: dato inconsistente", slog.String("error_class", "inconsistent"), slog.String("trace_id", traceID(r)))
		}
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
	case errors.Is(err, appDashboard.ErrUnavailable), errors.Is(err, appSummary.ErrUnavailable):
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
	default:
		if logger != nil {
			logger.ErrorContext(r.Context(), "dashboard: error no clasificado", slog.String("error_class", "internal"), slog.String("trace_id", traceID(r)))
		}
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
	}
}
