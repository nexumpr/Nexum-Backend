package http

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	appPreferences "github.com/nexumpr/Nexum-Backend/internal/application/preferences"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// PreferencesHandler expone `GET /api/v1/users/me/preferences` (E068).
//
// La ruta está protegida por el middleware de autenticación JWT RS256 y por la verificación de
// sesión autoritativa: el `userId` se deriva exclusivamente de `sub`. El cliente no puede
// elegir titular ni observar preferencias ajenas.
//
// Contrato (OpenAPI `nexum_e068`): 200 con `UserPreferences`, que tras la enmienda A8 incluye
// `locale`, `timezone`, `version` y el alias exacto `timeZone`. Cabecera
// `Cache-Control: private, no-store` incluso en error, porque el cuerpo describe al titular.
//
// Códigos contemplados: 200, 400, 401, 403, 404, 429, 500 y 503. Este recorte no compone
// limitador distribuido, por lo que **429 no se emite** (`429_NOT_EMITTED`); el 404 no se usa:
// preferencias ausentes se materializan, no se declaran inexistentes.
type PreferencesHandler struct {
	service   *appPreferences.Service
	logger    *slog.Logger
	protected http.Handler
}

// NewPreferencesHandler construye el handler. Sin servicio, sin proveedor de tokens o sin
// autorizador de sesiones, el endpoint responde 503 (fail-closed): nunca se sirven preferencias
// sin identidad verificada ni sin persistencia real.
func NewPreferencesHandler(
	service *appPreferences.Service,
	tokens ports.TokenProvider,
	sessions ports.SessionAuthorizer,
	logger *slog.Logger,
) *PreferencesHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &PreferencesHandler{service: service, logger: logger}
	if service != nil && service.Disponible() && tokens != nil && sessions != nil {
		h.protected = middleware.AuthenticateBearerWithSession(tokens, sessions)(http.HandlerFunc(h.serve))
	}
	return h
}

// ServeHTTP fija la cabecera de caché antes de cualquier decisión y delega en la cadena
// autenticada.
func (h *PreferencesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Preferencias personales: nunca cacheables por proxies ni por el cliente.
	w.Header().Set("Cache-Control", "private, no-store")

	if h == nil || h.service == nil || !h.service.Disponible() || h.protected == nil {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}

	h.protected.ServeHTTP(w, r)
}

// serve se ejecuta dentro de la cadena autenticada: `sub` ya está verificado y la sesión es
// ACTIVE, no expirada y con la época vigente.
func (h *PreferencesHandler) serve(w http.ResponseWriter, r *http.Request) {
	userID, okUser := middleware.UserIDFromContext(r.Context())
	if !okUser || strings.TrimSpace(userID) == "" {
		// Defensa en profundidad: el middleware ya exige `sub`.
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}

	prefs, err := h.service.Get(r.Context(), userID)
	if err != nil {
		writePreferencesError(w, r, h.logger, err)
		return
	}

	respuesta := preferencesResponse{
		Locale:   prefs.Locale,
		Timezone: prefs.Timezone,
		Version:  prefs.Version,
		// A8: alias exacto del mismo valor; se emite siempre, nunca se omite.
		TimeZone: prefs.Timezone,
	}
	if errWrite := httputil.WriteJSON(w, http.StatusOK, respuesta); errWrite != nil {
		h.logger.ErrorContext(r.Context(), "preferencias: fallo al serializar la respuesta",
			slog.String("error_class", "response_write"),
			slog.String("trace_id", traceID(r)))
	}
}

// preferencesResponse es el objeto `UserPreferences` del contrato: cuatro campos, sin extras.
// `additionalProperties: false` en OpenAPI obliga a no añadir nada más.
type preferencesResponse struct {
	Locale   string `json:"locale"`
	Timezone string `json:"timezone"`
	Version  int64  `json:"version"`
	TimeZone string `json:"timeZone"`
}

// writePreferencesError traduce los errores de aplicación a los códigos del contrato. Nunca se
// registra el `userId`, el valor de una zona ni el contenido de una preferencia.
func writePreferencesError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	if logger == nil {
		logger = slog.Default()
	}

	var violacion *appPreferences.FieldViolation
	if errors.As(err, &violacion) {
		writeRegisterError(w, r, http.StatusUnprocessableEntity, "BUSINESS_RULE_VIOLATION", "solicitud no completada", []httputil.ErrorDetail{
			{Field: violacion.Field, Reason: violacion.Reason},
		})
		return
	}

	switch {
	case errors.Is(err, appPreferences.ErrPreferencesInvalidRequest):
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", nil)
	case errors.Is(err, appPreferences.ErrPreferencesUnauthorized):
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
	case errors.Is(err, appPreferences.ErrPreferencesVersionConflict):
		writeRegisterError(w, r, http.StatusPreconditionFailed, "VERSION_CONFLICT", "solicitud no completada", nil)
	case errors.Is(err, appPreferences.ErrPreferencesConflict):
		logger.WarnContext(r.Context(), "preferencias: conflicto de escritura",
			slog.String("error_class", "preferences_conflict"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusConflict, "CONFLICT", "solicitud no completada", nil)
	case errors.Is(err, appPreferences.ErrPreferencesUnavailable):
		logger.ErrorContext(r.Context(), "preferencias: dependencia no disponible",
			slog.String("error_class", "preferences_unavailable"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
	default:
		logger.ErrorContext(r.Context(), "preferencias: fallo interno",
			slog.String("error_class", "internal_error"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
	}
}
