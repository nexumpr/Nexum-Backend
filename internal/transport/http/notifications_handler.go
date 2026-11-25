package http

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/nexumpr/Nexum-Backend/internal/application/notifications"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

var notificationIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// NotificationsHandler sirve E061 (GET), E062 (PATCH) y E063 (POST) sobre
// `/api/v1/notifications` y sus subrecursos. Un solo punto de composición evita
// que un endpoint reuse un subject o una clave de firma distinta.
type NotificationsHandler struct {
	service   *notifications.Service
	cursorKey []byte
	logger    *slog.Logger
	protected http.Handler
}

// NewNotificationsHandler construye el handler. Sin servicio, tokens,
// autorizador o clave HMAC estable, todas las peticiones responden 503
// (fail-closed): nunca se genera una clave efífera en producción.
func NewNotificationsHandler(
	service *notifications.Service,
	tokens ports.TokenProvider,
	sessions ports.SessionAuthorizer,
	limitador LimitadorRuta,
	cursorKey []byte,
	logger *slog.Logger,
) *NotificationsHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &NotificationsHandler{service: service, cursorKey: cursorKey, logger: logger}
	if service == nil || tokens == nil || sessions == nil || len(cursorKey) == 0 {
		return h
	}
	conCuota := LimitarPorUsuarioContrato(limitador, "notifications", h.serve)
	h.protected = middleware.AuthenticateBearerWithSession(tokens, sessions)(conCuota)
	return h
}

// ServeHTTP fija Cache-Control antes de cualquier decisión y despacha por
// método. El subject se deriva siempre del JWT verificado.
func (h *NotificationsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	switch r.Method {
	case http.MethodGet, http.MethodPatch, http.MethodPost:
	default:
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodPatch+", "+http.MethodPost)
		writeRegisterError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "el metodo HTTP no esta permitido para este recurso", nil)
		return
	}
	if h == nil || h.service == nil || h.protected == nil || len(h.cursorKey) == 0 {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}
	h.protected.ServeHTTP(w, r)
}

func (h *NotificationsHandler) serve(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}
	userID = strings.TrimSpace(userID)
	switch r.Method {
	case http.MethodGet:
		h.serveList(w, r, userID)
	case http.MethodPatch:
		h.serveMarkRead(w, r, userID)
	case http.MethodPost:
		h.serveReadAll(w, r, userID)
	}
}

func (h *NotificationsHandler) serveList(w http.ResponseWriter, r *http.Request, userID string) {
	params, ok := h.parseListParams(w, r)
	if !ok {
		return
	}
	pagina, err := h.service.List(r.Context(), userID, params)
	if err != nil {
		writeNotificationsError(w, r, h.logger, err)
		return
	}
	if err := httputil.WriteJSON(w, http.StatusOK, pagina.Wire()); err != nil {
		h.logger.ErrorContext(r.Context(), "E061: fallo al serializar", slog.String("trace_id", traceID(r)))
	}
}

func (h *NotificationsHandler) parseListParams(w http.ResponseWriter, r *http.Request) (notifications.ListParams, bool) {
	var params notifications.ListParams
	query := r.URL.Query()
	params.Limit = notifications.DefaultLimit
	if raw := query.Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > notifications.MaxLimit {
			writeNotificationsViolation(w, r, "limit", "out_of_range")
			return params, false
		}
		params.Limit = limit
	}
	params.Cursor = query.Get("cursor")
	if params.Cursor != "" && (len(params.Cursor) < notifications.MinCursorLength || len(params.Cursor) > notifications.MaxCursorLength) {
		writeNotificationsViolation(w, r, "cursor", "invalid_cursor")
		return params, false
	}
	if raw := query.Get("isRead"); raw != "" {
		switch strings.ToLower(raw) {
		case "true":
			params.IsRead = func() *bool { v := true; return &v }()
		case "false":
			params.IsRead = func() *bool { v := false; return &v }()
		default:
			writeNotificationsViolation(w, r, "isRead", "invalid_boolean")
			return params, false
		}
	}
	if raw := query.Get("type"); raw != "" {
		tipo, ok := notifications.ParseType(raw)
		if !ok {
			writeNotificationsViolation(w, r, "type", "invalid_type")
			return params, false
		}
		params.Type = string(tipo)
	}
	return params, true
}

func (h *NotificationsHandler) serveMarkRead(w http.ResponseWriter, r *http.Request, userID string) {
	id := strings.TrimSpace(r.PathValue("notificationId"))
	if !notificationIDPattern.MatchString(id) {
		writeNotificationsViolation(w, r, "notificationId", "invalid_uuid")
		return
	}
	if !notificacionesSinCuerpo(r) {
		writeNotificationsViolation(w, r, "body", "body_not_allowed")
		return
	}
	notificacion, err := h.service.MarkRead(r.Context(), userID, id)
	if err != nil {
		writeNotificationsError(w, r, h.logger, err)
		return
	}
	if err := httputil.WriteJSON(w, http.StatusOK, notificacion.Wire()); err != nil {
		h.logger.ErrorContext(r.Context(), "E062: fallo al serializar", slog.String("trace_id", traceID(r)))
	}
}

func (h *NotificationsHandler) serveReadAll(w http.ResponseWriter, r *http.Request, userID string) {
	if len(r.URL.Query()) != 0 || !notificacionesSinCuerpo(r) {
		writeNotificationsViolation(w, r, "body", "body_not_allowed")
		return
	}
	aceptado, err := h.service.StartReadAll(r.Context(), userID)
	if err != nil {
		writeNotificationsError(w, r, h.logger, err)
		return
	}
	if err := httputil.WriteJSON(w, http.StatusAccepted, aceptado.Wire()); err != nil {
		h.logger.ErrorContext(r.Context(), "E063: fallo al serializar", slog.String("trace_id", traceID(r)))
	}
}

func notificacionesSinCuerpo(r *http.Request) bool {
	if r.ContentLength > 0 {
		return false
	}
	if r.Body == nil || r.Body == http.NoBody {
		return true
	}
	lectura, err := io.ReadAll(io.LimitReader(r.Body, 1))
	return err == nil && len(lectura) == 0
}

func writeNotificationsViolation(w http.ResponseWriter, r *http.Request, field, reason string) {
	writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
		{Field: field, Reason: reason},
	})
}

func writeNotificationsError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	switch {
	case errors.Is(err, notifications.ErrInvalidRequest):
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", nil)
	case errors.Is(err, notifications.ErrUnauthorized):
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
	case errors.Is(err, notifications.ErrNotFound):
		writeRegisterError(w, r, http.StatusNotFound, "NOT_FOUND", "recurso no encontrado", nil)
	case errors.Is(err, notifications.ErrConflict):
		writeRegisterError(w, r, http.StatusConflict, "CONFLICT", "conflicto al procesar notificaciones", nil)
	case errors.Is(err, notifications.ErrUnavailable):
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
	default:
		if logger != nil {
			logger.ErrorContext(r.Context(), "E061/E062/E063: error no clasificado",
				slog.String("error_class", "internal"), slog.String("trace_id", traceID(r)))
		}
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
	}
}
