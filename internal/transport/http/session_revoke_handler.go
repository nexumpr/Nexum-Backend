package http

import (
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

// SessionRevokeHandler expone `DELETE /api/v1/auth/sessions/{sessionId}` (E009).
//
// La ruta está protegida por el middleware de autenticación JWT RS256 y por la verificación de
// sesión autoritativa: la identidad se deriva exclusivamente de `sub` verificado. El
// `sessionId` de la ruta es un RECURSO, no una credencial: sólo se revoca si pertenece al
// titular; en caso contrario la respuesta es el mismo 404 que una sesión inexistente.
//
// Contrato: 204 sin cuerpo. Códigos contemplados: 204, 400, 401, 403, 404, 409, 422, 429, 500
// y 503. Este recorte no incorpora limitador distribuido (429 no se emite): la cuota por cuenta
// requiere Redis y pertenece a una unidad con Gate propio; no se simula.
type SessionRevokeHandler struct {
	service   *appIdentity.SessionRevokeService
	logger    *slog.Logger
	protected http.Handler
}

// NewSessionRevokeHandler construye el handler. Si falta el servicio, el proveedor de tokens o
// el autorizador de sesiones, el endpoint responde 503 (fail-closed): nunca se revoca sin
// identidad verificada ni sin persistencia.
func NewSessionRevokeHandler(
	service *appIdentity.SessionRevokeService,
	tokens ports.TokenProvider,
	sessions ports.SessionAuthorizer,
	logger *slog.Logger,
) *SessionRevokeHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &SessionRevokeHandler{service: service, logger: logger}
	if service != nil && tokens != nil && sessions != nil {
		h.protected = middleware.AuthenticateBearerWithSession(tokens, sessions)(http.HandlerFunc(h.serve))
	}
	return h
}

// ServeHTTP fija la cabecera de caché antes de cualquier decisión y delega en la cadena
// autenticada.
func (h *SessionRevokeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Una revocación nunca debe cachearse ni almacenarse en proxies ni en el cliente.
	w.Header().Set("Cache-Control", "no-store")

	if h == nil || h.service == nil || h.protected == nil {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}

	h.protected.ServeHTTP(w, r)
}

// serve se ejecuta dentro de la cadena autenticada: `sub` ya está verificado.
func (h *SessionRevokeHandler) serve(w http.ResponseWriter, r *http.Request) {
	userID, okUser := middleware.UserIDFromContext(r.Context())
	if !okUser || userID == "" {
		// Defensa en profundidad: el middleware ya exige `sub`.
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}

	sessionID := sessionIDFromPath(r)
	if sessionID == "" {
		// El router garantiza un segmento no vacío; esta guarda evita revocar la sesión
		// equivocada si la ruta se monta de otra forma.
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "sessionId", Reason: "invalid_format"},
		})
		return
	}

	err := h.service.RevokeOwnedSession(r.Context(), userID, sessionID, time.Now().UTC())
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}

	// Contrato: 204 sin cuerpo. No se serializa nada después de esta línea.
	w.WriteHeader(http.StatusNoContent)
}

// sessionIDFromPath obtiene el parámetro de ruta expuesto por el router con `SetPathValue`.
// Como respaldo, acepta el último segmento de la ruta: el handler sólo se monta en
// `/auth/sessions/{sessionId}` y así sigue funcionando si se invoca sin el router (pruebas y
// composiciones alternativas). Nunca se lee de query, cabecera ni cuerpo.
func sessionIDFromPath(r *http.Request) string {
	if r == nil {
		return ""
	}
	if valor := strings.TrimSpace(r.PathValue("sessionId")); valor != "" {
		return valor
	}
	ruta := strings.TrimSuffix(r.URL.Path, "/")
	if ruta == "" {
		return ""
	}
	if indice := strings.LastIndex(ruta, "/"); indice >= 0 {
		return strings.TrimSpace(ruta[indice+1:])
	}
	return strings.TrimSpace(ruta)
}

// writeServiceError traduce los errores de aplicación a códigos del contrato. Nunca se registra
// el `userId`, el `sessionId`, el digest del refresh ni ningún dato de la sesión.
func (h *SessionRevokeHandler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, appIdentity.ErrSessionsInvalidRequest):
		// `sessionId` que no es un uuid canónico: 400 sin reflejar el valor recibido.
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "sessionId", Reason: "invalid_format"},
		})
	case errors.Is(err, appIdentity.ErrSessionRevokeUnauthorized):
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
	case errors.Is(err, appIdentity.ErrSessionRevokeNotFound):
		// Mismo mensaje para sesión ajena, inexistente, expirada o purgada: sin enumeración.
		writeRegisterError(w, r, http.StatusNotFound, "RESOURCE_NOT_FOUND", "el recurso solicitado no existe", nil)
	case errors.Is(err, appIdentity.ErrSessionRevokeConflict):
		h.logger.WarnContext(r.Context(), "session_revoke: conflicto de concurrencia",
			slog.String("error_class", "session_revoke_conflict"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusConflict, "CONFLICT", "Conflicto de concurrencia; reintente la operacion.", nil)
	case errors.Is(err, appIdentity.ErrSessionRevokeUnavailable):
		h.logger.ErrorContext(r.Context(), "session_revoke: dependencia no disponible",
			slog.String("error_class", "session_revoke_unavailable"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
	default:
		h.logger.ErrorContext(r.Context(), "session_revoke: fallo interno",
			slog.String("error_class", "internal_error"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
	}
}
