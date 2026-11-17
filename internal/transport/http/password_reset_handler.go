package http

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// PasswordResetConfirmMaxRequestBodyBytes acota el cuerpo de E006 (token y contraseña).
const PasswordResetConfirmMaxRequestBodyBytes int64 = 16 * 1024

// PasswordResetConfirmLimiter evalúa la cuota de intentos de confirmación por IP.
type PasswordResetConfirmLimiter interface {
	AllowResetConfirm(ctx context.Context, ip string) (bool, time.Duration, error)
}

// PasswordResetConfirmRequest es el DTO estricto de E006 (OpenAPI PasswordResetRequest).
// Los nombres son exactamente `token` y `newPassword`; cualquier propiedad desconocida la
// rechaza BindJSONLimit y no se añaden campos incompatibles con Android.
type PasswordResetConfirmRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"newPassword"`
}

// PasswordResetConfirmHandler expone POST /api/v1/auth/password/reset (E006).
//
// Contrato congelado (RFC-016 §17.3): público, sin Bearer; 200 Accepted {status, requestId};
// 400/409/422/429/500/503. Nunca devuelve ni registra el token o la contraseña.
type PasswordResetConfirmHandler struct {
	service *appIdentity.PasswordResetConfirmService
	limiter PasswordResetConfirmLimiter
	logger  *slog.Logger
}

// NewPasswordResetConfirmHandler construye el handler. Sin servicio disponible o sin
// limitador, el endpoint responde 503 en lugar de operar sin control.
func NewPasswordResetConfirmHandler(
	service *appIdentity.PasswordResetConfirmService,
	limiter PasswordResetConfirmLimiter,
	logger *slog.Logger,
) *PasswordResetConfirmHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &PasswordResetConfirmHandler{service: service, limiter: limiter, logger: logger}
}

// ServeHTTP implementa el contrato de E006.
func (h *PasswordResetConfirmHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")

	if h == nil || h.service == nil || !h.service.Disponible() {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}
	if h.limiter == nil {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}

	if !hasJSONContentType(r) {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "Content-Type", Reason: "application_json_required"},
		})
		return
	}

	var request PasswordResetConfirmRequest
	if err := BindJSONLimit(w, r, &request, PasswordResetConfirmMaxRequestBodyBytes); err != nil {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "body", Reason: "invalid_json"},
		})
		return
	}

	ip, okIP := clientIPFromTrustedChain(r)
	if !okIP {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "X-Forwarded-For", Reason: "invalid_chain"},
		})
		return
	}

	allowed, retryAfter, errLimiter := h.limiter.AllowResetConfirm(r.Context(), ip)
	if errLimiter != nil {
		h.logger.ErrorContext(r.Context(), "reset: limitador no disponible",
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

	err := h.service.Confirm(r.Context(), appIdentity.ResetConfirmCommand{
		Token:       request.Token,
		NewPassword: request.NewPassword,
	})
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(acceptedResponse{Status: "accepted", RequestID: nuevoRequestID()})
}

func (h *PasswordResetConfirmHandler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	var violation *appIdentity.FieldViolation
	if errors.As(err, &violation) {
		// Regla de negocio del payload (longitud de contraseña, token vacío): 422.
		writeRegisterError(w, r, http.StatusUnprocessableEntity, "BUSINESS_RULE_VIOLATION", "solicitud no completada", []httputil.ErrorDetail{
			{Field: violation.Field, Reason: violation.Reason},
		})
		return
	}

	switch {
	case errors.Is(err, appIdentity.ErrResetTokenUsado), errors.Is(err, appIdentity.ErrResetEnProceso):
		// Token ya consumido o carrera perdida: conflicto genérico, sin revelar detalles.
		writeRegisterError(w, r, http.StatusConflict, "CONFLICT", "solicitud no completada", nil)
	case errors.Is(err, ports.ErrResetTokenInvalido):
		// Inexistente o expirado: rechazo genérico del contrato (400).
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", nil)
	case errors.Is(err, appIdentity.ErrResetConfirmUnavailable):
		h.logger.ErrorContext(r.Context(), "reset: dependencia no disponible",
			slog.String("error_class", "reset_confirm_unavailable"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
	default:
		h.logger.ErrorContext(r.Context(), "reset: fallo interno",
			slog.String("error_class", "internal_error"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
	}
}
