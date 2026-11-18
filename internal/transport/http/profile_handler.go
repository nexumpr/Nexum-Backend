package http

import (
	"bytes"
	"encoding/json"
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

// ProfileHandler expone `GET /api/v1/users/me` (E012) y `PATCH /api/v1/users/me` (E013). El
// router registra cada método por separado, de modo que un montaje sin E013 no habilita
// escritura.
//
// La ruta está protegida por JWT RS256 y por la sesión autoritativa en DynamoDB: la
// identidad se deriva exclusivamente de `sub` y `sid` verificados. El cliente no puede
// elegir el usuario por query, cuerpo ni cabecera.
//
// Contrato: 200 con `UserProfile` y `Cache-Control: private, no-store`; en error, el sobre
// canónico `ErrorEnvelope`. Códigos contemplados: 200, 400, 401, 403, 404, 412, 422, 428,
// 429, 500 y 503 (429 no se emite en este recorte: la cuota distribuida exige Redis y
// pertenece a una unidad con Gate propio; no se simula).
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
		h.protected = middleware.AuthenticateBearerWithSession(tokens, sessions)(http.HandlerFunc(h.dispatch))
	}
	return h
}

// ServeHTTP fija la cabecera de caché antes de cualquier decisión y delega en la cadena
// autenticada. GET y PATCH están implementados; cualquier otro verbo responde 405 aunque el
// handler se montara por error con él.
func (h *ProfileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Información de identidad y de cuenta: nunca cacheable por proxies ni por el cliente.
	w.Header().Set("Cache-Control", "private, no-store")

	if h == nil || h.service == nil || h.protected == nil {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}

	switch r.Method {
	case http.MethodGet, http.MethodPatch:
		h.protected.ServeHTTP(w, r)
	default:
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodPatch)
		writeRegisterError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "el metodo HTTP no esta permitido para este recurso", nil)
	}
}

// dispatch reparte el método ya autenticado. Sólo puede llegar GET o PATCH: `ServeHTTP`
// rechaza el resto antes de entrar en la cadena protegida.
func (h *ProfileHandler) dispatch(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPatch:
		h.servePatch(w, r)
	default:
		h.serveGet(w, r)
	}
}

// serveGet se ejecuta dentro de la cadena autenticada: `sub` ya está verificado y la sesión
// es ACTIVE, no expirada y con la época vigente.
func (h *ProfileHandler) serveGet(w http.ResponseWriter, r *http.Request) {
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

// ProfilePatchMaxRequestBodyBytes acota el cuerpo de E013: un único nombre de hasta 100
// runes no necesita más de unos pocos cientos de bytes.
const ProfilePatchMaxRequestBodyBytes int64 = 8 * 1024

// ProfilePatchRequest es el DTO estricto de E013 (OpenAPI `ProfilePatch`).
//
// `additionalProperties: false`: `BindJSONLimit` rechaza cualquier propiedad desconocida y
// un JSON concatenado. Un puntero nulo en el borde (cuerpo `null`) no es un objeto y se
// rechaza; `displayName` distingue campo ausente (`{}`) de cadena vacía, que es una
// violación de dominio, y de `null`, que no es un `string` válido. No se admiten `id`,
// `email`, `version` ni ningún otro campo del perfil: la identidad viene del claim `sub` y
// la versión de `If-Match`.
type ProfilePatchRequest struct {
	DisplayName patchDisplayName `json:"displayName"`
}

// patchDisplayName distingue tres situaciones que el contrato separa: campo ausente (no-op),
// cadena válida y `null` (rechazado: `ProfilePatch.displayName` es `type: string`).
type patchDisplayName struct {
	Presente bool
	Valor    string
}

// UnmarshalJSON implementa la distinción; un valor que no sea una cadena produce 400.
func (n *patchDisplayName) UnmarshalJSON(datos []byte) error {
	if string(bytes.TrimSpace(datos)) == "null" {
		return errors.New("displayName no admite null")
	}
	var valor string
	if err := json.Unmarshal(datos, &valor); err != nil {
		return err
	}
	n.Presente = true
	n.Valor = valor
	return nil
}

// servePatch aplica `PATCH /api/v1/users/me` (E013).
//
// Orden de decisiones: identidad verificada → `If-Match` obligatorio (428 si falta, 400 si
// está mal formado) → cuerpo estricto (400) → caso de uso (412/404/409/422/503). La
// respuesta es `UserProfile` con `ETag: "v<version>"`, de modo que el cliente puede
// encadenar una segunda modificación sin releer.
func (h *ProfileHandler) servePatch(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		// Defensa en profundidad: el middleware ya exige `sub`.
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}

	version, errIfMatch := ParseIfMatchVersion(r.Header.Get("If-Match"))
	if errIfMatch != nil {
		if errors.Is(errIfMatch, ErrIfMatchRequerido) {
			writeRegisterError(w, r, http.StatusPreconditionRequired, "PRECONDITION_REQUIRED", "solicitud no completada", nil)
			return
		}
		if errors.Is(errIfMatch, ErrIfMatchDebil) {
			// RFC 9110: un validador debil no satisface If-Match.
			writeRegisterError(w, r, http.StatusPreconditionFailed, "VERSION_CONFLICT", "solicitud no completada", nil)
			return
		}
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "If-Match", Reason: "invalid_etag"},
		})
		return
	}

	var request *ProfilePatchRequest
	if err := BindJSONLimit(w, r, &request, ProfilePatchMaxRequestBodyBytes); err != nil || request == nil {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "body", Reason: "invalid_json"},
		})
		return
	}

	var patch appProfile.Patch
	if request.DisplayName.Presente {
		nombre := request.DisplayName.Valor
		patch.DisplayName = &nombre
	}

	perfil, err := h.service.Update(r.Context(), userID, version, patch)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	// ETag de salida: misma forma que exige `If-Match` en la siguiente petición.
	w.Header().Set("ETag", FormatETag(perfil.Version))
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
