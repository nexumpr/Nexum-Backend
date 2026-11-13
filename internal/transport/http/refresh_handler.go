package http

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// RefreshMaxRequestBodyBytes acota el cuerpo de E003. El contrato sólo admite el token
// opaco y (RFC-014) un `deviceId` opcional: 8 KiB es holgura suficiente y reduce la
// superficie frente a cuerpos abusivos.
const RefreshMaxRequestBodyBytes int64 = 8 * 1024

// RefreshRateLimiter evalúa las dos cuotas de E003: por IP y por digest del token.
type RefreshRateLimiter interface {
	AllowRefresh(ctx context.Context, ip, tokenDigest string) (bool, time.Duration, error)
}

// RefreshRequest es el DTO estricto de E003. `refreshToken` es obligatorio; `deviceId` es
// opcional y sólo actúa como señal de consistencia con el dispositivo que creó la sesión
// (RFC-014). Cualquier propiedad desconocida la rechaza BindJSONLimit.
type RefreshRequest struct {
	RefreshToken string  `json:"refreshToken"`
	DeviceID     *string `json:"deviceId"`
}

// RefreshHandler expone POST /api/v1/auth/refresh.
type RefreshHandler struct {
	service *appIdentity.RefreshService
	limiter RefreshRateLimiter
	logger  *slog.Logger
}

// NewRefreshHandler construye el handler. Un limitador nil falla cerrado con 503.
func NewRefreshHandler(service *appIdentity.RefreshService, limiter RefreshRateLimiter, logger *slog.Logger) *RefreshHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &RefreshHandler{service: service, limiter: limiter, logger: logger}
}

// ServeHTTP implementa el contrato de E003.
func (h *RefreshHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// La respuesta contiene credenciales: nunca debe cachearse en el cliente ni en proxies.
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Pragma", "no-cache")

	if h == nil || h.service == nil {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}

	if !hasJSONContentType(r) {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "Content-Type", Reason: "application_json_required"},
		})
		return
	}

	var request RefreshRequest
	if err := BindJSONLimit(w, r, &request, RefreshMaxRequestBodyBytes); err != nil {
		var violation *appIdentity.FieldViolation
		if errors.As(err, &violation) {
			writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
				{Field: violation.Field, Reason: violation.Reason},
			})
			return
		}
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "body", Reason: "invalid_json"},
		})
		return
	}

	ipConfiable, okIP := clientIPFromTrustedChain(r)
	if !okIP {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "X-Forwarded-For", Reason: "invalid_chain"},
		})
		return
	}

	command := appIdentity.RefreshCommand{
		RefreshToken: request.RefreshToken,
		ClientIP:     ipConfiable,
	}
	if request.DeviceID != nil {
		command.DeviceID = *request.DeviceID
	}

	if h.limiter == nil {
		h.logger.ErrorContext(r.Context(), "refresh rechazado: rate limiter no configurado",
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}
	// El digest identifica la cuota sin que Redis llegue a conocer el token en claro.
	allowed, retryAfter, errLimit := h.limiter.AllowRefresh(r.Context(), command.ClientIP, appIdentity.HashToken(command.RefreshToken))
	if errLimit != nil {
		h.logger.ErrorContext(r.Context(), "refresh rechazado: fallo del rate limiter",
			slog.String("error_class", "rate_limiter_unavailable"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}
	if !allowed {
		w.Header().Set("Retry-After", retryAfterHeader(retryAfter))
		writeRegisterError(w, r, http.StatusTooManyRequests, "RATE_LIMITED", "demasiadas solicitudes", nil)
		return
	}

	tokens, err := h.service.Refresh(r.Context(), command)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	// El contrato se construye en el borde de transporte con claves explícitas: la
	// respuesta pública de refresh lleva tokens por diseño y nada más.
	cuerpo := map[string]any{
		"accessToken":  tokens.AccessToken,
		"refreshToken": tokens.RefreshToken,
		"tokenType":    tokens.TokenType,
		"expiresIn":    tokens.ExpiresIn,
		"sessionId":    tokens.SessionID,
		"session":      map[string]any{"id": tokens.Session.ID},
	}
	datos, errJSON := json.Marshal(cuerpo)
	if errJSON != nil {
		h.logger.ErrorContext(r.Context(), "refresh: no se pudo serializar la respuesta",
			slog.String("error_class", "serialization_failed"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(datos)
}

// writeServiceError traduce los errores de aplicación a códigos del contrato. Nunca se
// registra el token, el digest ni datos de la sesión.
func (h *RefreshHandler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	var violation *appIdentity.FieldViolation
	if errors.As(err, &violation) {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: violation.Field, Reason: violation.Reason},
		})
		return
	}

	switch {
	case errors.Is(err, appIdentity.ErrInvalidRefreshToken):
		// Mismo mensaje para token inexistente, caducado, revocado o manipulado: no hay
		// enumeración de sesiones.
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Refresh token invalido.", nil)
	case errors.Is(err, appIdentity.ErrRefreshReplay):
		h.logger.WarnContext(r.Context(), "refresh: reutilizacion de token detectada; familia revocada",
			slog.String("error_class", "refresh_replay"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusConflict, "CONFLICT", "El refresh token ya fue utilizado; la sesion fue revocada.", nil)
	case errors.Is(err, appIdentity.ErrRefreshConflict):
		h.logger.WarnContext(r.Context(), "refresh: conflicto de concurrencia sin reutilizacion",
			slog.String("error_class", "refresh_conflict"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusConflict, "CONFLICT", "Conflicto de concurrencia; reintente la operacion.", nil)
	case errors.Is(err, appIdentity.ErrRefreshUnavailable):
		h.logger.ErrorContext(r.Context(), "refresh: dependencia no disponible",
			slog.String("error_class", "refresh_unavailable"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
	default:
		h.logger.ErrorContext(r.Context(), "refresh: fallo interno",
			slog.String("error_class", "internal_error"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
	}
}
