package http

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// PasswordChangeMaxRequestBodyBytes acota el cuerpo de E007 (dos contraseñas).
const PasswordChangeMaxRequestBodyBytes int64 = 16 * 1024

// PasswordChangeLimiter evalúa la cuota de intentos de cambio de contraseña. La clave es el
// `userId` ya verificado, no un valor enviado por el cliente.
type PasswordChangeLimiter interface {
	Permitir(ctx context.Context, clave string) (bool, time.Duration, error)
}

// PasswordChangeRequest es el DTO estricto de E007 (OpenAPI PasswordChangeRequest).
//
// Los nombres son exactamente `currentPassword` y `newPassword`; `BindJSONLimit` rechaza
// cualquier propiedad adicional, un JSON concatenado y cuerpos por encima del límite. No se
// admiten `userId`, `sessionId`, `refreshToken` ni tokens de salida: la cuenta se deriva del
// claim `sub` y el contrato de éxito es `Accepted`.
type PasswordChangeRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

// PasswordChangeHandler expone POST /api/v1/auth/password/change (E007).
//
// Contrato congelado: Bearer obligatorio, 200 `Accepted {status, requestId}` y los códigos
// 400/401/409/422/429/500/503. Nunca devuelve ni registra contraseñas, hashes o identificadores
// de usuario, y no emite tokens nuevos: el incremento de época invalida todas las sesiones y
// el cliente reautentica.
type PasswordChangeHandler struct {
	service   *appIdentity.PasswordChangeService
	limiter   PasswordChangeLimiter
	logger    *slog.Logger
	protected http.Handler
}

// NewPasswordChangeHandler construye el handler.
//
// El middleware se compone con `AuthenticateBearerWithSession`: sin proveedor de tokens o sin
// autorizador de sesión la ruta responde 503 en lugar de autorizar sólo con la firma. Si falta
// el servicio, el hasher (vía `Disponible`) o el limitador, también responde 503 (fail-closed).
func NewPasswordChangeHandler(
	service *appIdentity.PasswordChangeService,
	limiter PasswordChangeLimiter,
	tokens ports.TokenProvider,
	sessions ports.SessionAuthorizer,
	logger *slog.Logger,
) *PasswordChangeHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &PasswordChangeHandler{service: service, limiter: limiter, logger: logger}
	if service != nil && tokens != nil && sessions != nil {
		h.protected = middleware.AuthenticateBearerWithSession(tokens, sessions)(http.HandlerFunc(h.serve))
	}
	return h
}

// ServeHTTP aplica las guardas de composición y delega en la cadena autenticada.
func (h *PasswordChangeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")

	if h == nil || h.service == nil || !h.service.Disponible() || h.limiter == nil || h.protected == nil {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}

	h.protected.ServeHTTP(w, r)
}

// serve se ejecuta dentro de la cadena autenticada: en este punto `sub` está verificado
// criptográficamente y la sesión es ACTIVE, no expirada y con la época vigente.
func (h *PasswordChangeHandler) serve(w http.ResponseWriter, r *http.Request) {
	userID, okUser := middleware.UserIDFromContext(r.Context())
	if !okUser || strings.TrimSpace(userID) == "" {
		// Defensa en profundidad: el middleware ya exige `sub`.
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}

	if !hasJSONContentType(r) {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "Content-Type", Reason: "application_json_required"},
		})
		return
	}

	var request PasswordChangeRequest
	if err := BindJSONLimit(w, r, &request, PasswordChangeMaxRequestBodyBytes); err != nil {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "body", Reason: "invalid_json"},
		})
		return
	}

	allowed, retryAfter, errLimiter := h.limiter.Permitir(r.Context(), userID)
	if errLimiter != nil {
		// Redis no disponible: fail-closed, no se degrada a "sin límite".
		h.logger.ErrorContext(r.Context(), "cambio de contrasena: limitador no disponible",
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

	err := h.service.Change(r.Context(), appIdentity.PasswordChangeCommand{
		UserID:          userID,
		CurrentPassword: request.CurrentPassword,
		NewPassword:     request.NewPassword,
	})
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}

	// Contrato: 200 Accepted {status, requestId}. No se serializa ningún token, `changedAt`
	// ni contador: la sesión quedó revocada por época y el cliente debe reautenticar.
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(acceptedResponse{Status: "accepted", RequestID: nuevoRequestID()})
}

// writeServiceError traduce los errores de aplicación a códigos del contrato sin revelar el
// motivo exacto. Nunca se registra la contraseña, el hash ni el `userId`.
func (h *PasswordChangeHandler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	var violation *appIdentity.FieldViolation
	if errors.As(err, &violation) {
		// Política de contraseña o campo obligatorio vacío: 422.
		writeRegisterError(w, r, http.StatusUnprocessableEntity, "BUSINESS_RULE_VIOLATION", "solicitud no completada", []httputil.ErrorDetail{
			{Field: violation.Field, Reason: violation.Reason},
		})
		return
	}

	switch {
	case errors.Is(err, appIdentity.ErrPasswordChangeActualIncorrecta):
		// Mismo mensaje que cualquier otra violación de regla: no se distingue "contraseña
		// incorrecta" de "política incumplida" ni se confirma que la cuenta exista.
		writeRegisterError(w, r, http.StatusUnprocessableEntity, "BUSINESS_RULE_VIOLATION", "solicitud no completada", nil)
	case errors.Is(err, appIdentity.ErrPasswordChangeUnauthorized):
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
	case errors.Is(err, appIdentity.ErrPasswordChangeConflict):
		h.logger.WarnContext(r.Context(), "cambio de contrasena: conflicto de concurrencia",
			slog.String("error_class", "password_change_conflict"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusConflict, "CONFLICT", "solicitud no completada", nil)
	case errors.Is(err, appIdentity.ErrPasswordChangeUnavailable):
		h.logger.ErrorContext(r.Context(), "cambio de contrasena: dependencia no disponible",
			slog.String("error_class", "password_change_unavailable"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
	default:
		h.logger.ErrorContext(r.Context(), "cambio de contrasena: fallo interno",
			slog.String("error_class", "internal_error"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
	}
}
