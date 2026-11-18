package http

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	appProfile "github.com/nexumpr/Nexum-Backend/internal/application/profile"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// ProfileHandler expone `GET /api/v1/users/me` (E012). En su extensión E013 añade
// `PATCH /api/v1/users/me`; el router registra cada método por separado, de modo que un
// montaje sin E013 no habilita escritura.
//
// La ruta está protegida por JWT RS256 y por la sesión autoritativa en DynamoDB: la
// identidad se deriva exclusivamente de `sub` y `sid` verificados. El cliente no puede
// elegir el usuario por query, cuerpo ni cabecera.
//
// Contrato: 200 con `UserProfile` y `Cache-Control: private, no-store`; en error, el sobre
// canónico `ErrorEnvelope`. Códigos contemplados: 200, 400, 401, 403, 404, 429, 500 y 503
// (429 no se emite en este recorte: la cuota distribuida exige Redis y pertenece a una
// unidad con Gate propio; no se simula).
type ProfileHandler struct {
	service   *appProfile.Service
	logger    *slog.Logger
	protected http.Handler
}

// NewProfileHandler construye el handler. Si falta el servicio, el proveedor de tokens o el
// autorizador de sesiones, el endpoint responde 503 (fail-closed): nunca se sirve un perfil
// sin identidad verificada ni sin persistencia.
func NewProfileHandler(
	service *appProfile.Service,
	tokens ports.TokenProvider,
	sessions ports.SessionAuthorizer,
	logger *slog.Logger,
) *ProfileHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &ProfileHandler{service: service, logger: logger}
	if service != nil && tokens != nil && sessions != nil {
		h.protected = middleware.AuthenticateBearerWithSession(tokens, sessions)(http.HandlerFunc(h.serve))
	}
	return h
}

// ServeHTTP fija la cabecera de caché antes de cualquier decisión y delega en la cadena
// autenticada. Sólo GET está implementado en esta unidad.
func (h *ProfileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Información de identidad y de cuenta: nunca cacheable por proxies ni por el cliente.
	w.Header().Set("Cache-Control", "private, no-store")

	if h == nil || h.service == nil || h.protected == nil {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.protected.ServeHTTP(w, r)
	default:
		// El router sólo registra GET en esta unidad; esta guarda evita servir un método
		// no previsto si el handler se montara por error con otro verbo.
		w.Header().Set("Allow", http.MethodGet)
		writeRegisterError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "el metodo HTTP no esta permitido para este recurso", nil)
	}
}

// serve se ejecuta dentro de la cadena autenticada: `sub` ya está verificado y la sesión es
// ACTIVE, no expirada y con la época vigente.
func (h *ProfileHandler) serve(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		// Defensa en profundidad: el middleware ya exige `sub`.
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}

	perfil, err := h.service.Get(r.Context(), userID)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	h.writeProfile(w, r, perfil)
}

// userProfileResponse es el esquema `UserProfile`: exactamente seis campos, sin extras y
// sin `omitempty`. `emailVerified` se emite siempre, incluso con valor false.
type userProfileResponse struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	DisplayName   string `json:"displayName"`
	EmailVerified bool   `json:"emailVerified"`
	CreatedAt     string `json:"createdAt"`
	Version       int64  `json:"version"`
}

// writeProfile serializa explícitamente el DTO del contrato. El item de DynamoDB nunca
// llega al cliente, de modo que `passwordHash`, `sessionEpoch`, `privacy*` o cualquier
// atributo nuevo no pueden filtrarse.
func (h *ProfileHandler) writeProfile(w http.ResponseWriter, r *http.Request, perfil *appProfile.Profile) {
	if perfil == nil {
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
		return
	}
	respuesta := userProfileResponse{
		ID:            perfil.ID,
		Email:         perfil.Email,
		DisplayName:   perfil.DisplayName,
		EmailVerified: perfil.EmailVerified,
		CreatedAt:     perfil.CreatedAt.UTC().Format(time.RFC3339),
		Version:       perfil.Version,
	}
	if err := httputil.WriteJSON(w, http.StatusOK, respuesta); err != nil {
		h.logger.ErrorContext(r.Context(), "perfil: fallo al serializar la respuesta",
			slog.String("error_class", "response_write"),
			slog.String("trace_id", traceID(r)))
	}
}

// writeServiceError traduce los errores de aplicación a códigos del contrato. Nunca se
// registra el correo, el `userId`, el nombre ni ningún dato del perfil.
func (h *ProfileHandler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	var violacion *appProfile.FieldViolation
	switch {
	case errors.As(err, &violacion):
		detalles := []httputil.ErrorDetail{{Field: violacion.Field, Reason: violacion.Reason}}
		writeRegisterError(w, r, http.StatusUnprocessableEntity, "BUSINESS_RULE_VIOLATION", "solicitud no completada", detalles)
	case errors.Is(err, appProfile.ErrProfileInvalidRequest):
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", nil)
	case errors.Is(err, appProfile.ErrProfileUnauthorized):
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
	case errors.Is(err, appProfile.ErrProfileNotFound):
		writeRegisterError(w, r, http.StatusNotFound, "NOT_FOUND", "solicitud no completada", nil)
	case errors.Is(err, appProfile.ErrProfileVersionConflict):
		writeRegisterError(w, r, http.StatusPreconditionFailed, "VERSION_CONFLICT", "solicitud no completada", nil)
	case errors.Is(err, appProfile.ErrProfileConflict):
		writeRegisterError(w, r, http.StatusConflict, "CONFLICT", "solicitud no completada", nil)
	case errors.Is(err, appProfile.ErrProfileUnavailable):
		h.logger.ErrorContext(r.Context(), "perfil: dependencia no disponible",
			slog.String("error_class", "profile_unavailable"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
	default:
		h.logger.ErrorContext(r.Context(), "perfil: fallo interno",
			slog.String("error_class", "internal_error"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
	}
}
