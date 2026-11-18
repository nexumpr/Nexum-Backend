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

// PreferencesPatchMaxRequestBodyBytes limita el cuerpo de E069: tres campos cortos no necesitan
// más de 1 KB y un cuerpo mayor se rechaza con 400 sin leerlo entero.
const PreferencesPatchMaxRequestBodyBytes int64 = 1024

// PreferencesPatchRequest es `UserPreferencesPatch` del contrato: `additionalProperties: false`
// y ningún campo obligatorio (el ejemplo del OpenAPI es `{}`). Los punteros distinguen "ausente"
// de "presente" porque el PATCH es parcial; `timeZone` es el alias A8 de `timezone`.
type PreferencesPatchRequest struct {
	Locale   *string `json:"locale"`
	Timezone *string `json:"timezone"`
	TimeZone *string `json:"timeZone"`
}

// PreferencesPatchHandler expone `PATCH /api/v1/users/me/preferences` (E069).
//
// La ruta está protegida por el middleware de autenticación JWT RS256 y por la verificación de
// sesión autoritativa: el `userId` se deriva exclusivamente de `sub`. La escritura es un
// compare-and-swap por `If-Match`, de modo que dos clientes no pueden pisarse una modificación.
//
// Contrato (OpenAPI `nexum_e069`):
//   - `If-Match` obligatorio con ETag `"vN"`: 428 si falta, 400 si está mal formado y 412 si no
//     coincide con la versión vigente.
//   - `locale` limitado al enum `es-MX|en-US`; 422 si no pertenece.
//   - `timezone` o el alias exacto `timeZone`, con zona IANA real (422 si es inválida). Si
//     llegan ambos con valores distintos, 422: nunca se elige uno en silencio.
//   - 200 con `UserPreferences` (cuatro campos, `timeZone` como alias exacto) y ETag de la
//     versión resultante; 409 si el CAS agota sus reintentos y 503 si falta persistencia.
//
// No se compone limitador distribuido en este recorte: **429 no se emite** (`429_NOT_EMITTED`).
type PreferencesPatchHandler struct {
	service   *appPreferences.Service
	logger    *slog.Logger
	protected http.Handler
}

// NewPreferencesPatchHandler construye el handler. Sin servicio, sin proveedor de tokens o sin
// autorizador de sesiones, el endpoint responde 503 (fail-closed): no se escribe ninguna
// preferencia sin identidad verificada ni sin persistencia real.
func NewPreferencesPatchHandler(
	service *appPreferences.Service,
	tokens ports.TokenProvider,
	sessions ports.SessionAuthorizer,
	logger *slog.Logger,
) *PreferencesPatchHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &PreferencesPatchHandler{service: service, logger: logger}
	if service != nil && service.Disponible() && tokens != nil && sessions != nil {
		h.protected = middleware.AuthenticateBearerWithSession(tokens, sessions)(http.HandlerFunc(h.serve))
	}
	return h
}

// ServeHTTP fija la cabecera de caché antes de cualquier decisión y delega en la cadena
// autenticada. Preferencias personales: nunca cacheables por proxies ni por el cliente.
func (h *PreferencesPatchHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")

	if h == nil || h.service == nil || !h.service.Disponible() || h.protected == nil {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}

	h.protected.ServeHTTP(w, r)
}

// serve se ejecuta dentro de la cadena autenticada: `sub` ya está verificado y la sesión es
// ACTIVE, no expirada y con la época vigente.
func (h *PreferencesPatchHandler) serve(w http.ResponseWriter, r *http.Request) {
	userID, okUser := middleware.UserIDFromContext(r.Context())
	if !okUser || strings.TrimSpace(userID) == "" {
		// Defensa en profundidad: el middleware ya exige `sub`.
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}

	// If-Match primero: es la precondición del contrato y su ausencia tiene código propio (428).
	version, errIfMatch := ParseIfMatchVersion(r.Header.Get("If-Match"))
	if errIfMatch != nil {
		if errors.Is(errIfMatch, ErrIfMatchRequerido) {
			writeRegisterError(w, r, http.StatusPreconditionRequired, "PRECONDITION_REQUIRED", "solicitud no completada", nil)
			return
		}
		if errors.Is(errIfMatch, ErrIfMatchDebil) {
			writeRegisterError(w, r, http.StatusPreconditionFailed, "VERSION_CONFLICT", "solicitud no completada", nil)
			return
		}
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "If-Match", Reason: "invalid_header"},
		})
		return
	}

	if !hasJSONContentType(r) {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "Content-Type", Reason: "application_json_required"},
		})
		return
	}

	var request PreferencesPatchRequest
	if err := BindJSONLimit(w, r, &request, PreferencesPatchMaxRequestBodyBytes); err != nil {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "body", Reason: "invalid_json"},
		})
		return
	}

	locale, hasLocale, okLocale := resolverLocale(request.Locale)
	if !okLocale {
		h.writeViolation(w, r, "locale", "unsupported_locale")
		return
	}

	timezone, hasTimezone, okTimezone := resolverZona(request.Timezone, request.TimeZone)
	if !okTimezone {
		h.writeViolation(w, r, "timeZone", "timezone_alias_mismatch")
		return
	}
	if hasTimezone {
		zona, errZona := ValidarZonaIANA(timezone)
		if errZona != nil {
			h.writeViolation(w, r, "timezone", "invalid_timezone")
			return
		}
		timezone = zona
	}

	prefs, err := h.service.Apply(r.Context(), appPreferences.ApplyCommand{
		UserID:          userID,
		Locale:          locale,
		Timezone:        timezone,
		HasLocale:       hasLocale,
		HasTimezone:     hasTimezone,
		ExpectedVersion: version,
	})
	if err != nil {
		writePreferencesError(w, r, h.logger, err)
		return
	}

	// ETag de la versión resultante: el cliente ya puede encadenar el siguiente PATCH.
	w.Header().Set("ETag", FormatETag(prefs.Version))
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

// resolverLocale valida el enum cerrado del contrato. Ausente es válido (no modifica nada).
func resolverLocale(valor *string) (locale string, presente bool, ok bool) {
	if valor == nil {
		return "", false, true
	}
	canonico, valido := ValidarLocale(*valor)
	if !valido {
		return "", true, false
	}
	return canonico, true, true
}

// resolverZona unifica `timezone` y su alias `timeZone`. Si ambos llegan con valores distintos
// se rechaza: el alias no puede interpretarse como una segunda zona silenciosa.
func resolverZona(timezone, timeZone *string) (zona string, presente bool, ok bool) {
	canonica := ""
	hayCanonica := false
	if timezone != nil {
		canonica = strings.TrimSpace(*timezone)
		hayCanonica = true
	}
	alias := ""
	hayAlias := false
	if timeZone != nil {
		alias = strings.TrimSpace(*timeZone)
		hayAlias = true
	}
	if hayCanonica && hayAlias && canonica != alias {
		return "", true, false
	}
	if hayCanonica {
		return canonica, true, true
	}
	if hayAlias {
		return alias, true, true
	}
	return "", false, true
}

// writeViolation emite el 422 del contrato con el campo responsable, sin incluir el valor
// rechazado (podría ser un dato del cliente).
func (h *PreferencesPatchHandler) writeViolation(w http.ResponseWriter, r *http.Request, field, reason string) {
	writePreferencesError(w, r, h.logger, &appPreferences.FieldViolation{Field: field, Reason: reason})
}
