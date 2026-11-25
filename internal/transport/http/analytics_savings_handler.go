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

// AnalyticsSavingsHandler sirve E056 (`GET /api/v1/analytics/savings-streak`).
type AnalyticsSavingsHandler struct {
	service   *analytics.Service
	logger    *slog.Logger
	protected http.Handler
}

// NewAnalyticsSavingsHandler construye el handler. Sin servicio, tokens o autorizador,
// todas las peticiones responden 503 (fail-closed). El limitador es opcional.
func NewAnalyticsSavingsHandler(
	service *analytics.Service,
	tokens ports.TokenProvider,
	sessions ports.SessionAuthorizer,
	limitador LimitadorRuta,
	logger *slog.Logger,
) *AnalyticsSavingsHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &AnalyticsSavingsHandler{service: service, logger: logger}
	if service == nil || tokens == nil || sessions == nil {
		return h
	}
	conCuota := LimitarPorUsuarioContrato(limitador, "analytics", h.serve)
	h.protected = middleware.AuthenticateBearerWithSession(tokens, sessions)(conCuota)
	return h
}

// ServeHTTP fija la cabecera de caché antes de cualquier decisión y admite sólo GET.
func (h *AnalyticsSavingsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

func (h *AnalyticsSavingsHandler) serve(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}

	racha, err := h.service.SavingsStreak(r.Context(), userID)
	if err != nil {
		writeSavingsError(w, r, h.logger, err)
		return
	}
	if err := httputil.WriteJSON(w, http.StatusOK, racha.Wire()); err != nil {
		h.logger.ErrorContext(r.Context(), "E056: fallo al serializar", slog.String("trace_id", traceID(r)))
	}
}

func writeSavingsError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	switch {
	case errors.Is(err, analytics.ErrInvalidRequest):
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", nil)
	case errors.Is(err, analytics.ErrUnauthorized):
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
	case errors.Is(err, analytics.ErrUnavailable):
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
	default:
		if logger != nil {
			logger.ErrorContext(r.Context(), "E056: error no clasificado", slog.String("error_class", "internal"), slog.String("trace_id", traceID(r)))
		}
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
	}
}
