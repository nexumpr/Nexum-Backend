package http

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/nexumpr/Nexum-Backend/internal/application/notificationprefs"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// NotificationPreferencesHandler sirve E057 (`GET`) y, en su momento, E058
// (`PUT`) sobre `/api/v1/notification-preferences`.
type NotificationPreferencesHandler struct {
	service   *notificationprefs.Service
	logger    *slog.Logger
	protected http.Handler
}

// NewNotificationPreferencesHandler construye el handler. Sin servicio, tokens
// o autorizador, todas las peticiones responden 503 (fail-closed). El
// limitador es opcional; sin él no se aplica cuota, pero la autenticación sigue
// siendo obligatoria.
func NewNotificationPreferencesHandler(
	service *notificationprefs.Service,
	tokens ports.TokenProvider,
	sessions ports.SessionAuthorizer,
	limitador LimitadorRuta,
	logger *slog.Logger,
) *NotificationPreferencesHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &NotificationPreferencesHandler{service: service, logger: logger}
	if service == nil || tokens == nil || sessions == nil {
		return h
	}
	conCuota := LimitarPorUsuarioContrato(limitador, "analytics", h.serve)
	h.protected = middleware.AuthenticateBearerWithSession(tokens, sessions)(conCuota)
	return h
}

// ServeHTTP fija la cabecera de caché antes de cualquier decisión y despacha
// por método: GET es E057. Otros métodos se rechazan con 405 contractual.
func (h *NotificationPreferencesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

func (h *NotificationPreferencesHandler) serve(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}

	preferencias, err := h.service.Get(r.Context(), userID)
	if err != nil {
		writeNotificationPrefsError(w, r, h.logger, err)
		return
	}
	if err := httputil.WriteJSON(w, http.StatusOK, preferencias.Wire()); err != nil {
		h.logger.ErrorContext(r.Context(), "E057: fallo al serializar", slog.String("trace_id", traceID(r)))
	}
}

func writeNotificationPrefsError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	switch {
	case errors.Is(err, notificationprefs.ErrInvalidRequest):
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", nil)
	case errors.Is(err, notificationprefs.ErrUnauthorized):
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
	case errors.Is(err, notificationprefs.ErrVersionConflict), errors.Is(err, notificationprefs.ErrConflict):
		writeRegisterError(w, r, http.StatusConflict, "CONFLICT", "conflicto al guardar preferencias", nil)
	case errors.Is(err, notificationprefs.ErrUnavailable):
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
	default:
		if logger != nil {
			logger.ErrorContext(r.Context(), "E057: error no clasificado", slog.String("error_class", "internal"), slog.String("trace_id", traceID(r)))
		}
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
	}
}
