package http

import (
	"log/slog"
	"net/http"
	"strings"

	appGoals "github.com/nexumpr/Nexum-Backend/internal/application/goals"
	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
)

// GoalDeleteHandler sirve E042 (`DELETE /api/v1/goals/{goalId}`).
type GoalDeleteHandler struct {
	service   *appGoals.Service
	logger    *slog.Logger
	protected http.Handler
}

// NewGoalDeleteHandler construye el handler. Sin servicio, tokens o autorizador responde 503.
func NewGoalDeleteHandler(service *appGoals.Service, tokens ports.TokenProvider, sessions ports.SessionAuthorizer, limitador LimitadorRuta, logger *slog.Logger) *GoalDeleteHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &GoalDeleteHandler{service: service, logger: logger}
	protegido := cadenaMetas(service, tokens, sessions, LimitarPorUsuario(limitador, "goals", h.serve))
	if protegido != nil {
		h.protected = protegido
	}
	return h
}

// ServeHTTP fija caché y valida el método.
func (h *GoalDeleteHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if r.Method != http.MethodDelete {
		w.Header().Set("Allow", http.MethodDelete)
		writeRegisterError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "el metodo HTTP no esta permitido para este recurso", nil)
		return
	}
	if h == nil || h.service == nil || h.protected == nil {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}
	h.protected.ServeHTTP(w, r)
}

func (h *GoalDeleteHandler) serve(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}
	goalID := strings.TrimSpace(r.PathValue("goalId"))
	if !appIdentity.IsUUID(goalID) {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", nil)
		return
	}
	if err := h.service.Delete(r.Context(), userID, goalID); err != nil {
		writeGoalsError(w, r, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
