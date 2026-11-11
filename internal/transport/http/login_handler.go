package http

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// LoginMaxRequestBodyBytes es el límite propio de E002 (política transversal: 256 KiB).
const LoginMaxRequestBodyBytes int64 = 256 * 1024

// LoginRateLimiter evalúa las dos cuotas simultáneas de E002: por IP y por email.
type LoginRateLimiter interface {
	AllowLogin(ctx context.Context, ip, normalizedEmail string) (bool, time.Duration, error)
}

// LoginRequest es el DTO estricto de E002 V6.0.2. Los tres campos de dispositivo son
// opcionales; cualquier propiedad desconocida la rechaza BindJSON.
type LoginRequest struct {
	Email      string  `json:"email"`
	Password   string  `json:"password"`
	DeviceID   *string `json:"deviceId"`
	DeviceName *string `json:"deviceName"`
	Platform   *string `json:"platform"`
}

// LoginHandler expone POST /api/v1/auth/login.
type LoginHandler struct {
	service *appIdentity.LoginService
	limiter LoginRateLimiter
	logger  *slog.Logger
}

// NewLoginHandler construye el handler. Un limitador nil falla cerrado con 503.
func NewLoginHandler(service *appIdentity.LoginService, limiter LoginRateLimiter, logger *slog.Logger) *LoginHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &LoginHandler{service: service, limiter: limiter, logger: logger}
}

// ServeHTTP implementa el contrato de E002.
func (h *LoginHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	var request LoginRequest
	if err := BindJSONLimit(w, r, &request, LoginMaxRequestBodyBytes); err != nil {
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

	command := appIdentity.LoginCommand{
		Email:    request.Email,
		Password: request.Password,
		ClientIP: registerClientIP(r),
	}
	if request.DeviceID != nil {
		command.DeviceID = *request.DeviceID
	}
	if request.DeviceName != nil {
		command.DeviceName = *request.DeviceName
	}
	if request.Platform != nil {
		command.Platform = *request.Platform
	}

	if h.limiter == nil {
		h.logger.ErrorContext(r.Context(), "login rechazado: rate limiter no configurado",
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}
	allowed, retryAfter, err := h.limiter.AllowLogin(r.Context(), command.ClientIP, strings.TrimSpace(command.Email))
	if err != nil {
		h.logger.ErrorContext(r.Context(), "login rechazado: fallo del rate limiter",
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

	response, err := h.service.Login(r.Context(), command)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

func (h *LoginHandler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	var violation *appIdentity.FieldViolation
	if errors.As(err, &violation) {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: violation.Field, Reason: violation.Reason},
		})
		return
	}

	switch {
	case errors.Is(err, appIdentity.ErrInvalidCredentials):
		// Mensaje genérico e idéntico para correo inexistente, contraseña incorrecta y
		// cuenta deshabilitada: sin enumeración de cuentas.
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Credenciales invalidas.", nil)
	case errors.Is(err, appIdentity.ErrLoginUnavailable):
		h.logger.ErrorContext(r.Context(), "login: dependencia no disponible",
			slog.String("error_class", "login_unavailable"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
	default:
		h.logger.ErrorContext(r.Context(), "login: fallo interno",
			slog.String("error_class", "internal_error"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
	}
}

var _ = strconv.Itoa
