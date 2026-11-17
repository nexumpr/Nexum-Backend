package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
)

// EmailVerificationRequestHandler expone POST /api/v1/auth/email-verification/request (E010).
//
// El contrato OpenAPI exige `Authorization: Bearer` y NO define cuerpo: el destinatario se
// obtiene del perfil autenticado y el cliente nunca envia el correo. La identidad se deriva
// exclusivamente de `sub` verificado y la sesion se comprueba contra el estado autoritativo.
type EmailVerificationRequestHandler struct {
	service   *appIdentity.EmailVerificationRequestService
	logger    *slog.Logger
	protected http.Handler
}

// NewEmailVerificationRequestHandler construye el handler. Si falta el servicio, el proveedor
// de tokens o el autorizador de sesiones, el endpoint responde 503 (fail-closed): nunca se
// envia un codigo de verificacion sin identidad verificada.
func NewEmailVerificationRequestHandler(
	service *appIdentity.EmailVerificationRequestService,
	tokens ports.TokenProvider,
	sessions ports.SessionAuthorizer,
	logger *slog.Logger,
) *EmailVerificationRequestHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &EmailVerificationRequestHandler{service: service, logger: logger}
	if service != nil && service.Disponible() && tokens != nil && sessions != nil {
		h.protected = middleware.AuthenticateBearerWithSession(tokens, sessions)(http.HandlerFunc(h.serve))
	}
	return h
}

// ServeHTTP aplica la cadena autenticada y responde 202 o el error contractual.
func (h *EmailVerificationRequestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")

	if h == nil || h.service == nil || !h.service.Disponible() || h.protected == nil {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}
	h.protected.ServeHTTP(w, r)
}

// serve se ejecuta dentro de la cadena autenticada: en este punto `sub` esta verificado y la
// sesion es ACTIVE y no expirada.
func (h *EmailVerificationRequestHandler) serve(w http.ResponseWriter, r *http.Request) {
	// El contrato no define requestBody: cualquier contenido se rechaza con 400 sin reflejarlo.
	if !rejectLogoutBody(w, r) {
		return
	}

	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		// Defensa en profundidad: el middleware ya exige el claim `sub`.
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}

	if err := h.service.Solicitar(r.Context(), userID); err != nil {
		h.writeServiceError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(acceptedResponse{Status: "accepted", RequestID: nuevoRequestID()})
}

// writeServiceError traduce los errores de aplicacion. Nunca se registra el token, el correo
// ni el userId: solo la clase de error.
func (h *EmailVerificationRequestHandler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, appIdentity.ErrEmailVerificationYaVerificado):
		writeRegisterError(w, r, http.StatusConflict, "CONFLICT", "solicitud no completada", nil)
	case errors.Is(err, appIdentity.ErrEmailVerificationCuota):
		w.Header().Set("Retry-After", "900")
		writeRegisterError(w, r, http.StatusTooManyRequests, "RATE_LIMITED", "demasiadas solicitudes", nil)
	case errors.Is(err, appIdentity.ErrEmailVerificationPerfilAusente):
		writeRegisterError(w, r, http.StatusNotFound, "NOT_FOUND", "solicitud no completada", nil)
	case errors.Is(err, appIdentity.ErrEmailVerificationCuentaInactiva):
		writeRegisterError(w, r, http.StatusForbidden, "FORBIDDEN", "solicitud no completada", nil)
	case errors.Is(err, appIdentity.ErrEmailVerificationNoDisponible),
		errors.Is(err, appIdentity.ErrEmailVerificationEnvio):
		h.logger.ErrorContext(r.Context(), "verificacion de correo: dependencia no disponible",
			slog.String("error_class", "email_verification_unavailable"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
	default:
		h.logger.ErrorContext(r.Context(), "verificacion de correo: fallo interno",
			slog.String("error_class", "internal_error"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
	}
}
