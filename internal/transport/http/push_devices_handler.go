package http

import (
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	appPush "github.com/nexumpr/Nexum-Backend/internal/application/pushdevices"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// PushDevicesMaxBody acota el cuerpo de E059: tres campos cortos no necesitan
// más de 2 KB. El token push se valida antes de cifrarse y nunca se registra.
const PushDevicesMaxBody int64 = 2048

var pushDeviceIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// PushTokenRegisterRequest es el DTO estricto de `PushTokenRegister`. Los
// punteros distinguen un campo ausente de un valor presente; `BindJSONLimit`
// rechaza propiedades desconocidas.
type PushTokenRegisterRequest struct {
	Provider    *string `json:"provider"`
	Token       *string `json:"token"`
	DeviceLabel *string `json:"deviceLabel"`
}

// PushDevicesHandler sirve E059: `POST /api/v1/devices/push-tokens`.
type PushDevicesHandler struct {
	service   *appPush.Service
	logger    *slog.Logger
	protected http.Handler
}

// NewPushDevicesHandler construye el handler de registro. Sin servicio, tokens
// o autorizador todas las peticiones responden 503 (fail-closed).
func NewPushDevicesHandler(
	service *appPush.Service,
	tokens ports.TokenProvider,
	sessions ports.SessionAuthorizer,
	limitador LimitadorRuta,
	logger *slog.Logger,
) *PushDevicesHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &PushDevicesHandler{service: service, logger: logger}
	if service == nil || tokens == nil || sessions == nil {
		return h
	}
	conCuota := LimitarPorUsuarioContrato(limitador, "push", h.serve)
	h.protected = middleware.AuthenticateBearerWithSession(tokens, sessions)(conCuota)
	return h
}

// ServeHTTP fija la cabecera de caché antes de cualquier decisión.
func (h *PushDevicesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeRegisterError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "el metodo HTTP no esta permitido para este recurso", nil)
		return
	}
	if h == nil || h.service == nil || h.protected == nil {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}
	h.protected.ServeHTTP(w, r)
}

func (h *PushDevicesHandler) serve(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
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
	var req PushTokenRegisterRequest
	if err := BindJSONLimit(w, r, &req, PushDevicesMaxBody); err != nil {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "body", Reason: "invalid_json_or_unknown_field"},
		})
		return
	}
	if req.Provider == nil || req.Token == nil || req.DeviceLabel == nil {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "body", Reason: "required_field_missing"},
		})
		return
	}

	resultado, err := h.service.Register(r.Context(), appPush.RegisterCommand{
		UserID:         userID,
		IdempotencyKey: clave,
		Provider:       *req.Provider,
		Token:          *req.Token,
		DeviceLabel:    *req.DeviceLabel,
	})
	if err != nil {
		writePushDevicesError(w, r, h.logger, err)
		return
	}

	w.Header().Set("Location", "/api/v1/devices/push-tokens/"+resultado.Device.ID)
	if err := httputil.WriteJSONBytes(w, http.StatusCreated, resultado.ResponseJSON); err != nil {
		h.logger.ErrorContext(r.Context(), "E059: fallo al serializar", slog.String("trace_id", traceID(r)))
	}
}

// PushDeviceDeleteHandler sirve E060:
// `DELETE /api/v1/devices/push-tokens/{deviceRegistrationId}`.
type PushDeviceDeleteHandler struct {
	service   *appPush.Service
	logger    *slog.Logger
	protected http.Handler
}

// NewPushDeviceDeleteHandler construye el handler de baja. Sin servicio, tokens
// o autorizador todas las peticiones responden 503 (fail-closed).
func NewPushDeviceDeleteHandler(
	service *appPush.Service,
	tokens ports.TokenProvider,
	sessions ports.SessionAuthorizer,
	limitador LimitadorRuta,
	logger *slog.Logger,
) *PushDeviceDeleteHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &PushDeviceDeleteHandler{service: service, logger: logger}
	if service == nil || tokens == nil || sessions == nil {
		return h
	}
	conCuota := LimitarPorUsuarioContrato(limitador, "push", h.serve)
	h.protected = middleware.AuthenticateBearerWithSession(tokens, sessions)(conCuota)
	return h
}

// ServeHTTP fija la cabecera de caché antes de cualquier decisión.
func (h *PushDeviceDeleteHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

func (h *PushDeviceDeleteHandler) serve(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}
	deviceID := pushDeviceIDFromPath(r)
	if !pushDeviceIDPattern.MatchString(deviceID) {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "deviceRegistrationId", Reason: "invalid_format"},
		})
		return
	}
	if err := h.service.Revoke(r.Context(), userID, deviceID); err != nil {
		writePushDevicesError(w, r, h.logger, err)
		return
	}
	// 204 contractual: sin cuerpo, sin Content-Type y sin cabeceras de recurso.
	w.WriteHeader(http.StatusNoContent)
}

// pushDeviceIDFromPath acepta el valor expuesto por el router con SetPathValue
// y, como respaldo para composiciones sin router, el último segmento de la URL.
// Nunca lee query, cabeceras ni cuerpo.
func pushDeviceIDFromPath(r *http.Request) string {
	if r == nil {
		return ""
	}
	if valor := strings.TrimSpace(r.PathValue("deviceRegistrationId")); valor != "" {
		return valor
	}
	ruta := strings.TrimSuffix(r.URL.Path, "/")
	if indice := strings.LastIndex(ruta, "/"); indice >= 0 {
		return strings.TrimSpace(ruta[indice+1:])
	}
	return strings.TrimSpace(ruta)
}

func writePushDevicesError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	switch {
	case errors.Is(err, appPush.ErrInvalidRequest):
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", nil)
	case errors.Is(err, appPush.ErrUnauthorized):
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
	case errors.Is(err, appPush.ErrNotFound):
		writeRegisterError(w, r, http.StatusNotFound, "NOT_FOUND", "el recurso solicitado no existe", nil)
	case errors.Is(err, appPush.ErrConflict):
		writeRegisterError(w, r, http.StatusConflict, "CONFLICT", "conflicto al registrar el dispositivo push", nil)
	case errors.Is(err, appPush.ErrUnavailable):
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
	default:
		if logger != nil {
			logger.ErrorContext(r.Context(), "E059/E060: error no clasificado", slog.String("error_class", "internal"), slog.String("trace_id", traceID(r)))
		}
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
	}
}
