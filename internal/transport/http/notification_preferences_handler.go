package http

import (
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/nexumpr/Nexum-Backend/internal/application/notificationprefs"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// NotificationPreferencesMaxBody acota el cuerpo de E058: nueve campos cortos
// no necesitan más de 2 KB; un cuerpo mayor se rechaza sin leerse entero.
const NotificationPreferencesMaxBody int64 = 2048

var notificationQuietHour = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

// NotificationPreferencesUpdateRequest es `NotificationPreferencesUpdate` del
// contrato. Los punteros distinguen "ausente" de "presente": los tres
// booleanos requeridos deben venir; los opcionales ausentes toman el default
// del esquema. `BindJSONLimit` prohíbe campos desconocidos.
type NotificationPreferencesUpdateRequest struct {
	PushEnabled        *bool                   `json:"pushEnabled"`
	EmailEnabled       *bool                   `json:"emailEnabled"`
	ReminderEnabled    *bool                   `json:"reminderEnabled"`
	BudgetAlerts       *bool                   `json:"budgetAlerts"`
	GoalUpdates        *bool                   `json:"goalUpdates"`
	RecurringReminders *bool                   `json:"recurringReminders"`
	StreakReminders    *bool                   `json:"streakReminders"`
	QuietHours         *notificationQuietHours `json:"quietHours"`
	TimeZone           *string                 `json:"timeZone"`
}

type notificationQuietHours struct {
	Enabled *bool  `json:"enabled"`
	Start   string `json:"start"`
	End     string `json:"end"`
}

// NotificationPreferencesHandler sirve E057 (`GET`) y E058 (`PUT`) sobre
// `/api/v1/notification-preferences`.
type NotificationPreferencesHandler struct {
	service   *notificationprefs.Service
	logger    *slog.Logger
	protected http.Handler
}

// NewNotificationPreferencesHandler construye el handler. Sin servicio, tokens
// o autorizador, todas las peticiones responden 503 (fail-closed). El
// limitador es opcional; la autenticación nunca lo es.
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
	conCuota := LimitarPorUsuarioContrato(limitador, "preferences", h.serve)
	h.protected = middleware.AuthenticateBearerWithSession(tokens, sessions)(conCuota)
	return h
}

// ServeHTTP fija la cabecera de caché antes de cualquier decisión y despacha
// por método. Otros métodos se rechazan con 405 contractual.
func (h *NotificationPreferencesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	switch r.Method {
	case http.MethodGet, http.MethodPut:
	default:
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodPut)
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
	if r.Method == http.MethodPut {
		h.servePut(w, r, userID)
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

func (h *NotificationPreferencesHandler) servePut(w http.ResponseWriter, r *http.Request, userID string) {
	if !hasJSONContentType(r) {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "Content-Type", Reason: "application_json_required"},
		})
		return
	}
	var req NotificationPreferencesUpdateRequest
	if err := BindJSONLimit(w, r, &req, NotificationPreferencesMaxBody); err != nil {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "body", Reason: "invalid_json"},
		})
		return
	}
	if req.PushEnabled == nil || req.EmailEnabled == nil || req.ReminderEnabled == nil {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "body", Reason: "required_boolean_missing"},
		})
		return
	}

	valor := notificationprefs.Defaults()
	valor.PushEnabled = *req.PushEnabled
	valor.EmailEnabled = *req.EmailEnabled
	valor.ReminderEnabled = *req.ReminderEnabled
	if req.BudgetAlerts != nil {
		valor.BudgetAlerts = *req.BudgetAlerts
	}
	if req.GoalUpdates != nil {
		valor.GoalUpdates = *req.GoalUpdates
	}
	if req.RecurringReminders != nil {
		valor.RecurringReminders = *req.RecurringReminders
	}
	if req.StreakReminders != nil {
		valor.StreakReminders = *req.StreakReminders
	}

	if req.QuietHours != nil {
		if req.QuietHours.Enabled == nil {
			writeNotificationViolation(w, r, h.logger, "quietHours.enabled", "required")
			return
		}
		inicio, fin := strings.TrimSpace(req.QuietHours.Start), strings.TrimSpace(req.QuietHours.End)
		if (inicio != "" && !notificationQuietHour.MatchString(inicio)) || (fin != "" && !notificationQuietHour.MatchString(fin)) {
			writeNotificationViolation(w, r, h.logger, "quietHours", "invalid_time_range")
			return
		}
		if inicio != "" && fin != "" && inicio == fin {
			writeNotificationViolation(w, r, h.logger, "quietHours", "equal_start_end")
			return
		}
		valor.QuietHours = &notificationprefs.QuietHours{Enabled: *req.QuietHours.Enabled, Start: inicio, End: fin}
	}
	if req.TimeZone != nil {
		zona, errZona := ValidarZonaIANA(*req.TimeZone)
		if errZona != nil {
			writeNotificationViolation(w, r, h.logger, "timeZone", "invalid_timezone")
			return
		}
		valor.TimeZone = zona
	}

	actualizadas, err := h.service.Update(r.Context(), notificationprefs.UpdateCommand{UserID: userID, Value: valor})
	if err != nil {
		writeNotificationPrefsError(w, r, h.logger, err)
		return
	}
	if err := httputil.WriteJSON(w, http.StatusOK, actualizadas.Wire()); err != nil {
		h.logger.ErrorContext(r.Context(), "E058: fallo al serializar", slog.String("trace_id", traceID(r)))
	}
}

func writeNotificationViolation(w http.ResponseWriter, r *http.Request, logger *slog.Logger, field, reason string) {
	writeNotificationPrefsError(w, r, logger, &notificationprefs.FieldViolation{Field: field, Reason: reason})
}

func writeNotificationPrefsError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	var violacion *notificationprefs.FieldViolation
	if errors.As(err, &violacion) {
		writeRegisterError(w, r, http.StatusUnprocessableEntity, "BUSINESS_RULE_VIOLATION", "solicitud no completada", []httputil.ErrorDetail{
			{Field: violacion.Field, Reason: violacion.Reason},
		})
		return
	}
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
			logger.ErrorContext(r.Context(), "E057/E058: error no clasificado", slog.String("error_class", "internal"), slog.String("trace_id", traceID(r)))
		}
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
	}
}
