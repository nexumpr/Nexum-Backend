package http

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net/http"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// LogoutMaxBodyBytes acota la lectura del cuerpo de E004. El contrato NO admite cuerpo: se lee
// un máximo pequeño sólo para poder rechazar de forma explícita cualquier contenido enviado.
const LogoutMaxBodyBytes int64 = 1024

// LogoutHandler expone POST /api/v1/auth/logout (E004).
//
// La ruta está protegida por el middleware de autenticación JWT RS256 y por la verificación de
// sesión autoritativa: la identidad se deriva exclusivamente de `sub` y `sid` verificados. El
// cliente no envía `userId`, `sessionId`, `refreshToken` ni `allDevices`, y no se exige
// `Idempotency-Key`.
type LogoutHandler struct {
	service   *appIdentity.LogoutService
	logger    *slog.Logger
	protected http.Handler
}

// NewLogoutHandler construye el handler. Si falta el servicio o el proveedor de tokens, el
// endpoint responde 503 (fail-closed): nunca se responde 204 sin revocación verificable.
func NewLogoutHandler(
	service *appIdentity.LogoutService,
	tokens ports.TokenProvider,
	sessions ports.SessionAuthorizer,
	logger *slog.Logger,
) *LogoutHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &LogoutHandler{service: service, logger: logger}
	// Ambos componentes son obligatorios: sin proveedor de tokens no hay identidad y sin
	// autorizador de sesiones no hay comprobación de revocación. En cualquiera de los dos
	// casos el endpoint responde 503 y nunca autoriza "sólo con la firma".
	if service != nil && tokens != nil && sessions != nil {
		h.protected = middleware.AuthenticateBearerWithSession(tokens, sessions)(http.HandlerFunc(h.serve))
	}
	return h
}

// ServeHTTP responde 204 sin contenido, o el error contractual correspondiente.
func (h *LogoutHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Una respuesta de cierre de sesión nunca debe cachearse ni almacenarse en proxies.
	w.Header().Set("Cache-Control", "no-store")

	if h == nil || h.service == nil || h.protected == nil {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}

	h.protected.ServeHTTP(w, r)
}

// serve se ejecuta dentro de la cadena autenticada: en este punto `sub` y `sid` están
// verificados criptográficamente y la sesión es ACTIVE y no expirada.
func (h *LogoutHandler) serve(w http.ResponseWriter, r *http.Request) {
	if !rejectLogoutBody(w, r) {
		return
	}

	userID, okUser := middleware.UserIDFromContext(r.Context())
	sessionID, okSession := middleware.SessionIDFromContext(r.Context())
	if !okUser || !okSession || userID == "" || sessionID == "" {
		// Defensa en profundidad: el middleware ya exige ambos claims.
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}

	err := h.service.Logout(r.Context(), appIdentity.LogoutCommand{UserID: userID, SessionID: sessionID})
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}

	// Contrato: 204 sin cuerpo. No se serializa nada después de esta línea.
	w.WriteHeader(http.StatusNoContent)
}

// rejectLogoutBody rechaza cualquier cuerpo no vacío. Devuelve false si ya se respondió.
//
// El contrato no define requestBody: ni `{}` ni ningún otro JSON son requisitos ni están
// admitidos. Cualquier contenido se rechaza con 400 sin reflejar el cuerpo recibido.
func rejectLogoutBody(w http.ResponseWriter, r *http.Request) bool {
	if r.Body == nil || r.ContentLength == 0 {
		return true
	}

	datos, err := io.ReadAll(io.LimitReader(r.Body, LogoutMaxBodyBytes+1))
	if err != nil {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "body", Reason: "unreadable"},
		})
		return false
	}
	if len(bytes.TrimSpace(datos)) == 0 {
		// Cuerpo declarado pero vacío o sólo espacios: equivale a ausencia de cuerpo.
		return true
	}
	if int64(len(datos)) > LogoutMaxBodyBytes {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "body", Reason: "body_not_allowed"},
		})
		return false
	}
	writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
		{Field: "body", Reason: "body_not_allowed"},
	})
	return false
}

// writeServiceError traduce los errores de aplicación a códigos del contrato. Nunca se
// registra el token, el digest, el `userId` ni el `sessionId`.
func (h *LogoutHandler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, appIdentity.ErrLogoutUnauthorized):
		// Mismo mensaje para sesión inexistente, ajena, revocada o expirada: no hay
		// enumeración de sesiones ni de cuentas (AC-A-050-03).
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
	case errors.Is(err, appIdentity.ErrLogoutConflict):
		h.logger.WarnContext(r.Context(), "logout: conflicto de concurrencia al revocar",
			slog.String("error_class", "logout_conflict"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusConflict, "CONFLICT", "Conflicto de concurrencia; reintente la operacion.", nil)
	case errors.Is(err, appIdentity.ErrLogoutUnavailable):
		h.logger.ErrorContext(r.Context(), "logout: dependencia no disponible",
			slog.String("error_class", "logout_unavailable"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
	default:
		h.logger.ErrorContext(r.Context(), "logout: fallo interno",
			slog.String("error_class", "internal_error"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
	}
}
