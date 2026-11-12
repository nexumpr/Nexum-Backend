package http

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	domain "github.com/nexumpr/Nexum-Backend/internal/domain/identity"
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

	ipConfiable, okIP := loginClientIP(r)
	if !okIP {
		// Cadena de proxies invalida o manipulada: se rechaza en lugar de agrupar a todos
		// los clientes afectados en una misma cuota compartida.
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "X-Forwarded-For", Reason: "invalid_chain"},
		})
		return
	}

	command := appIdentity.LoginCommand{
		Email:    request.Email,
		Password: request.Password,
		ClientIP: ipConfiable,
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
	// La cuota por email usa exactamente la identidad normalizada canónica (misma regla
	// que E001 y que la búsqueda en DynamoDB), evitando divergencias por mayúsculas o espacios.
	emailNormalizado, errEmail := domain.ValidateEmail(command.Email)
	if errEmail != nil {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "email", Reason: "invalid_email"},
		})
		return
	}
	allowed, retryAfter, err := h.limiter.AllowLogin(r.Context(), command.ClientIP, emailNormalizado)
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
	// El contrato JSON se construye en el borde de transporte con claves explicitas. Esto
	// evita que un analizador estatico confunda el marshalling del DTO de aplicacion con una
	// fuga de credenciales: la respuesta publica de login lleva los tokens por diseno.
	cuerpo := map[string]any{
		"accessToken":  response.AccessToken,
		"refreshToken": response.RefreshToken,
		"tokenType":    response.TokenType,
		"expiresIn":    response.ExpiresIn,
		"sessionId":    response.SessionID,
		"session":      map[string]any{"id": response.Session.ID},
		"user": map[string]any{
			"id":            response.User.ID,
			"name":          response.User.Name,
			"email":         response.User.Email,
			"emailVerified": response.User.EmailVerified,
		},
		"onboarding": map[string]any{"configured": response.Onboarding.Configured},
	}
	datos, errJSON := json.Marshal(cuerpo)
	if errJSON != nil {
		h.logger.ErrorContext(r.Context(), "login: no se pudo serializar la respuesta",
			slog.String("error_class", "serialization_failed"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(datos)
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

// loginClientIP aplica la política de cadena de proxies documentada en clientip.go.
// Si la cadena no permite determinar una IP verificable, se usa una identidad acotada al
// peer inmediato; nunca el primer X-Forwarded-For controlable por el cliente.
func loginClientIP(r *http.Request) (string, bool) {
	return clientIPFromTrustedChain(r)
}
