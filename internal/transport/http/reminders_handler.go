package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	appReminders "github.com/nexumpr/Nexum-Backend/internal/application/reminders"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// RemindersMaxBody acota los cuerpos de E065/E066: cinco campos cortos no
// necesitan más de 4 KB. El texto del recordatorio nunca se registra.
const RemindersMaxBody int64 = 4096

var reminderIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type ReminderCreateRequest struct {
	Title     *string         `json:"title"`
	DueAt     *string         `json:"dueAt"`
	Timezone  *string         `json:"timezone"`
	Frequency json.RawMessage `json:"frequency,omitempty"`
}

type ReminderPatchRequest struct {
	Title     *string         `json:"title"`
	DueAt     *string         `json:"dueAt"`
	Active    *bool           `json:"active"`
	Frequency json.RawMessage `json:"frequency,omitempty"`
}

type remindersHandler struct {
	service   *appReminders.Service
	logger    *slog.Logger
	protected http.Handler
}

// NewRemindersListHandler construye E064 (`GET /api/v1/reminders`).
func NewRemindersListHandler(
	service *appReminders.Service,
	tokens ports.TokenProvider,
	sessions ports.SessionAuthorizer,
	limitador LimitadorRuta,
	logger *slog.Logger,
) http.Handler {
	return newRemindersHandler(http.MethodGet, service, tokens, sessions, limitador, logger)
}

// NewRemindersCreateHandler construye E065 (`POST /api/v1/reminders`).
func NewRemindersCreateHandler(
	service *appReminders.Service,
	tokens ports.TokenProvider,
	sessions ports.SessionAuthorizer,
	limitador LimitadorRuta,
	logger *slog.Logger,
) http.Handler {
	return newRemindersHandler(http.MethodPost, service, tokens, sessions, limitador, logger)
}

// NewRemindersPatchHandler construye E066 (`PATCH /api/v1/reminders/{reminderId}`).
func NewRemindersPatchHandler(
	service *appReminders.Service,
	tokens ports.TokenProvider,
	sessions ports.SessionAuthorizer,
	limitador LimitadorRuta,
	logger *slog.Logger,
) http.Handler {
	return newRemindersHandler(http.MethodPatch, service, tokens, sessions, limitador, logger)
}

// NewRemindersDeleteHandler construye E067 (`DELETE /api/v1/reminders/{reminderId}`).
func NewRemindersDeleteHandler(
	service *appReminders.Service,
	tokens ports.TokenProvider,
	sessions ports.SessionAuthorizer,
	limitador LimitadorRuta,
	logger *slog.Logger,
) http.Handler {
	return newRemindersHandler(http.MethodDelete, service, tokens, sessions, limitador, logger)
}

func newRemindersHandler(
	method string,
	service *appReminders.Service,
	tokens ports.TokenProvider,
	sessions ports.SessionAuthorizer,
	limitador LimitadorRuta,
	logger *slog.Logger,
) *remindersHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &remindersHandler{service: service, logger: logger}
	if service == nil || !service.Disponible() || tokens == nil || sessions == nil {
		return h
	}
	conCuota := LimitarPorUsuarioContrato(limitador, "analytics", func(w http.ResponseWriter, r *http.Request) {
		switch method {
		case http.MethodGet:
			h.serveList(w, r)
		case http.MethodPost:
			h.serveCreate(w, r)
		case http.MethodPatch:
			h.servePatch(w, r)
		case http.MethodDelete:
			h.serveDelete(w, r)
		default:
			writeRegisterError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "el metodo HTTP no esta permitido para este recurso", nil)
		}
	})
	h.protected = middleware.AuthenticateBearerWithSession(tokens, sessions)(conCuota)
	return h
}

// ServeHTTP fija la cabecera de caché antes de cualquier decisión y valida el
// método declarado en el registro cerrado de la ruta.
func (h *remindersHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if h == nil || h.service == nil || !h.service.Disponible() || h.protected == nil {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}
	h.protected.ServeHTTP(w, r)
}

func (h *remindersHandler) userID(w http.ResponseWriter, r *http.Request) (string, bool) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return "", false
	}
	return userID, true
}

func (h *remindersHandler) serveList(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(w, r)
	if !ok {
		return
	}
	limit := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > appReminders.MaxLimit {
			writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
				{Field: "limit", Reason: "out_of_range"},
			})
			return
		}
		limit = parsed
	}
	var active *bool
	if raw := strings.TrimSpace(r.URL.Query().Get("active")); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
				{Field: "active", Reason: "invalid_boolean"},
			})
			return
		}
		active = &parsed
	}
	pagina, err := h.service.List(r.Context(), userID, limit, r.URL.Query().Get("cursor"), active)
	if err != nil {
		writeRemindersError(w, r, h.logger, err)
		return
	}
	if err := httputil.WriteJSON(w, http.StatusOK, pagina.Wire()); err != nil {
		h.logger.ErrorContext(r.Context(), "E064: fallo al serializar", slog.String("trace_id", traceID(r)))
	}
}

func (h *remindersHandler) serveCreate(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(w, r)
	if !ok {
		return
	}
	clave, errClave := RequireIdempotencyKey(r)
	if errClave != nil {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: HeaderIdempotencyKey, Reason: "required_or_invalid_length"},
		})
		return
	}
	if !hasJSONContentType(r) {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "Content-Type", Reason: "application_json_required"},
		})
		return
	}
	var body ReminderCreateRequest
	if err := BindJSONLimit(w, r, &body, RemindersMaxBody); err != nil {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "body", Reason: "invalid_json_or_unknown_field"},
		})
		return
	}
	if body.Title == nil || body.DueAt == nil || body.Timezone == nil {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "body", Reason: "required_field_missing"},
		})
		return
	}
	frecuencia, err := parseReminderCreateFrequency(body.Frequency)
	if err != nil {
		writeRemindersError(w, r, h.logger, err)
		return
	}
	reminder, err := h.service.Create(r.Context(), appReminders.CreateCommand{
		UserID: userID, IdempotencyKey: clave,
		Title: *body.Title, DueAt: *body.DueAt, Timezone: *body.Timezone, Frequency: frecuencia,
	})
	if err != nil {
		writeRemindersError(w, r, h.logger, err)
		return
	}
	w.Header().Set("Location", "/api/v1/reminders/"+reminder.ID)
	if err := httputil.WriteJSON(w, http.StatusCreated, reminder.Wire()); err != nil {
		h.logger.ErrorContext(r.Context(), "E065: fallo al serializar", slog.String("trace_id", traceID(r)))
	}
}

func (h *remindersHandler) servePatch(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(w, r)
	if !ok {
		return
	}
	id := reminderIDFromPath(r)
	if !reminderIDPattern.MatchString(id) {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "reminderId", Reason: "invalid_format"},
		})
		return
	}
	version, errIfMatch := ParseIfMatchVersion(r.Header.Get("If-Match"))
	if errIfMatch != nil {
		switch {
		case errors.Is(errIfMatch, ErrIfMatchRequerido):
			writeRegisterError(w, r, http.StatusPreconditionRequired, "PRECONDITION_REQUIRED", "solicitud no completada", nil)
		case errors.Is(errIfMatch, ErrIfMatchDebil):
			writeRegisterError(w, r, http.StatusPreconditionFailed, "VERSION_CONFLICT", "solicitud no completada", nil)
		default:
			writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
				{Field: "If-Match", Reason: "invalid_header"},
			})
		}
		return
	}
	if !hasJSONContentType(r) {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "Content-Type", Reason: "application_json_required"},
		})
		return
	}
	var body ReminderPatchRequest
	if err := BindJSONLimit(w, r, &body, RemindersMaxBody); err != nil {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "body", Reason: "invalid_json_or_unknown_field"},
		})
		return
	}
	frecuencia, err := parseReminderPatchFrequency(body.Frequency)
	if err != nil {
		writeRemindersError(w, r, h.logger, err)
		return
	}
	reminder, err := h.service.Update(r.Context(), appReminders.PatchCommand{
		UserID: userID, ReminderID: id, ExpectedVersion: version,
		Title: body.Title, DueAt: body.DueAt, Active: body.Active, Frequency: frecuencia,
	})
	if err != nil {
		writeRemindersError(w, r, h.logger, err)
		return
	}
	w.Header().Set("ETag", FormatETag(reminder.Version))
	if err := httputil.WriteJSON(w, http.StatusOK, reminder.Wire()); err != nil {
		h.logger.ErrorContext(r.Context(), "E066: fallo al serializar", slog.String("trace_id", traceID(r)))
	}
}

func (h *remindersHandler) serveDelete(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(w, r)
	if !ok {
		return
	}
	id := reminderIDFromPath(r)
	if !reminderIDPattern.MatchString(id) {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "reminderId", Reason: "invalid_format"},
		})
		return
	}
	if err := h.service.Delete(r.Context(), userID, id); err != nil {
		writeRemindersError(w, r, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func reminderIDFromPath(r *http.Request) string {
	if r == nil {
		return ""
	}
	if valor := strings.TrimSpace(r.PathValue("reminderId")); valor != "" {
		return valor
	}
	partes := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(partes) == 0 {
		return ""
	}
	return partes[len(partes)-1]
}

func parseReminderCreateFrequency(raw json.RawMessage) (*appReminders.Frequency, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var valor string
	if err := json.Unmarshal(raw, &valor); err != nil {
		return nil, appReminders.ErrInvalidRequest
	}
	frecuencia, ok := appReminders.ParseFrequency(valor)
	if !ok {
		return nil, appReminders.ErrInvalidRequest
	}
	return &frecuencia, nil
}

func parseReminderPatchFrequency(raw json.RawMessage) (appReminders.FrequencyPatch, error) {
	if len(raw) == 0 {
		return appReminders.FrequencyPatch{}, nil
	}
	if string(raw) == "null" {
		return appReminders.FrequencyPatch{Set: true, Value: nil}, nil
	}
	var valor string
	if err := json.Unmarshal(raw, &valor); err != nil {
		return appReminders.FrequencyPatch{}, appReminders.ErrInvalidRequest
	}
	frecuencia, ok := appReminders.ParseFrequency(valor)
	if !ok {
		return appReminders.FrequencyPatch{}, appReminders.ErrInvalidRequest
	}
	return appReminders.FrequencyPatch{Set: true, Value: &frecuencia}, nil
}

func writeRemindersError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	switch {
	case errors.Is(err, appReminders.ErrInvalidRequest):
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", nil)
	case errors.Is(err, appReminders.ErrUnauthorized):
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
	case errors.Is(err, appReminders.ErrNotFound):
		writeRegisterError(w, r, http.StatusNotFound, "NOT_FOUND", "recurso no encontrado", nil)
	case errors.Is(err, appReminders.ErrConflict):
		writeRegisterError(w, r, http.StatusConflict, "CONFLICT", "solicitud no completada", nil)
	case errors.Is(err, appReminders.ErrVersionConflict):
		writeRegisterError(w, r, http.StatusPreconditionFailed, "VERSION_CONFLICT", "solicitud no completada", nil)
	case errors.Is(err, appReminders.ErrPreconditionRequired):
		writeRegisterError(w, r, http.StatusPreconditionRequired, "PRECONDITION_REQUIRED", "solicitud no completada", nil)
	case errors.Is(err, appReminders.ErrBusinessRule):
		writeRegisterError(w, r, http.StatusUnprocessableEntity, "BUSINESS_RULE_VIOLATION", "solicitud no completada", nil)
	case errors.Is(err, appReminders.ErrUnavailable):
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
	default:
		if logger != nil {
			logger.ErrorContext(r.Context(), "recordatorios: error no clasificado", slog.String("trace_id", traceID(r)))
		}
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "solicitud no completada", nil)
	}
}
