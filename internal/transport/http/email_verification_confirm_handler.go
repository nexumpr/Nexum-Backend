package http

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"
	"unicode/utf8"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// EmailVerificationConfirmMaxRequestBodyBytes acota el cuerpo de E011 (solo un token).
const EmailVerificationConfirmMaxRequestBodyBytes int64 = 16 * 1024

// EmailVerificationConfirmLimiter evalua la cuota de intentos de confirmacion por IP.
type EmailVerificationConfirmLimiter interface {
	AllowEmailVerificationConfirm(ctx context.Context, ip string) (bool, time.Duration, error)
}

// EmailVerificationConfirmRequest es el DTO estricto de E011 (OpenAPI VerifyEmailRequest).
// Solo `token`; cualquier propiedad desconocida la rechaza BindJSONLimit.
type EmailVerificationConfirmRequest struct {
	Token string `json:"token"`
}

// EmailVerificationConfirmHandler expone POST /api/v1/auth/email-verification/confirm (E011).
//
// La ruta es PUBLICA (`security: []`): no exige Bearer. La autoridad es el token de un solo
// uso. Contrato: 200 Accepted {status, requestId}; 400/409/422/429/500/503. Nunca devuelve ni
// registra el token.
type EmailVerificationConfirmHandler struct {
	service *appIdentity.EmailVerificationConfirmService
	limiter EmailVerificationConfirmLimiter
	logger  *slog.Logger
}

// NewEmailVerificationConfirmHandler construye el handler. Sin servicio disponible o sin
// limitador, el endpoint responde 503 en lugar de operar sin control.
func NewEmailVerificationConfirmHandler(
	service *appIdentity.EmailVerificationConfirmService,
	limiter EmailVerificationConfirmLimiter,
	logger *slog.Logger,
) *EmailVerificationConfirmHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &EmailVerificationConfirmHandler{service: service, limiter: limiter, logger: logger}
}

// ServeHTTP implementa el contrato de E011.
func (h *EmailVerificationConfirmHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")

	if h == nil || h.service == nil || !h.service.Disponible() || h.limiter == nil {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}

	if !hasJSONContentType(r) {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "Content-Type", Reason: "application_json_required"},
		})
		return
	}

	var request EmailVerificationConfirmRequest
	if err := BindJSONLimit(w, r, &request, EmailVerificationConfirmMaxRequestBodyBytes); err != nil {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "body", Reason: "invalid_json"},
		})
		return
	}

	// `token` es obligatorio (422, regla de negocio) y ademas minLength 10 en el contrato.
	// Un token no vacio pero demasiado corto es una violacion de esquema en el borde (400),
	// antes de consumir cuota o tocar persistencia.
	if request.Token == "" {
		writeRegisterError(w, r, http.StatusUnprocessableEntity, "BUSINESS_RULE_VIOLATION", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "token", Reason: "required"},
		})
		return
	}
	if utf8.RuneCountInString(request.Token) < appIdentity.VerifyEmailTokenMinRunes {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "token", Reason: "too_short"},
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

	allowed, retryAfter, errLimiter := h.limiter.AllowEmailVerificationConfirm(r.Context(), ip)
	if errLimiter != nil {
		h.logger.ErrorContext(r.Context(), "verificacion de correo: limitador no disponible",
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

	if err := h.service.Confirmar(r.Context(), request.Token); err != nil {
		h.writeServiceError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(acceptedResponse{Status: "accepted", RequestID: nuevoRequestID()})
}

// writeServiceError traduce los errores de aplicacion. Nunca se registra el token.
func (h *EmailVerificationConfirmHandler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	var violation *appIdentity.FieldViolation
	if errors.As(err, &violation) {
		writeRegisterError(w, r, http.StatusUnprocessableEntity, "BUSINESS_RULE_VIOLATION", "solicitud no completada", []httputil.ErrorDetail{
			{Field: violation.Field, Reason: violation.Reason},
		})
		return
	}

	switch {
	case errors.Is(err, appIdentity.ErrEmailVerificationTokenUsado),
		errors.Is(err, appIdentity.ErrEmailVerificationConflicto):
		// Replay o carrera perdida: conflicto generico, sin revelar detalles.
		writeRegisterError(w, r, http.StatusConflict, "CONFLICT", "solicitud no completada", nil)
	case errors.Is(err, appIdentity.ErrEmailVerificationTokenInvalido):
		// Inexistente, expirado, de otro proposito o de cuenta no ACTIVE: rechazo generico.
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", nil)
	case errors.Is(err, appIdentity.ErrEmailVerificationConfirmNoDisponible):
		h.logger.ErrorContext(r.Context(), "verificacion de correo: dependencia no disponible",
			slog.String("error_class", "email_verification_confirm_unavailable"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
	default:
		h.logger.ErrorContext(r.Context(), "verificacion de correo: fallo interno",
			slog.String("error_class", "internal_error"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
	}
}
