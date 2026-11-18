package http

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	appOnboarding "github.com/nexumpr/Nexum-Backend/internal/application/onboarding"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// OnboardingHandler expone el estado de onboarding del titular:
// `GET /api/v1/users/me/onboarding` (E018) y
// `PATCH /api/v1/users/me/onboarding` (E019).
//
// La ruta exige JWT RS256 y sesión autoritativa: la identidad se deriva
// exclusivamente de `sub` y `sid` verificados. El cliente no puede consultar ni
// modificar el onboarding de otro usuario.
//
// Contrato: 200 con `Onboarding` y cabecera `Cache-Control: private, no-store`;
// errores con el sobre canónico `ErrorEnvelope`. Códigos contemplados: 200, 400,
// 401, 403, 404, 409, 422, 429, 500 y 503.
type OnboardingHandler struct {
	service        *appOnboarding.Service
	logger         *slog.Logger
	protected      http.Handler
	protectedPatch http.Handler
}

// OnboardingPatchMaxRequestBodyBytes acota el cuerpo de E019. El contrato admite
// cuatro pasos y tres booleanos: 16 KiB son holgados sin permitir abuso.
const OnboardingPatchMaxRequestBodyBytes int64 = 16 * 1024

// OnboardingPatchRequest es el DTO estricto de E019 (`OnboardingPatch`).
//
// `completedSteps` es obligatorio: se modela como puntero para distinguir
// ausente/null (400) de `[]` (válido). Los booleanos también son punteros para
// distinguir "no enviado" de `false` explícito, que debe persistirse como
// `denied`. Cualquier propiedad desconocida la rechaza BindJSONLimit.
type OnboardingPatchRequest struct {
	CompletedSteps *[]string `json:"completedSteps"`
	Microphone     *bool     `json:"microphone"`
	Notifications  *bool     `json:"notifications"`
	Configured     *bool     `json:"configured"`
}

// NewOnboardingHandler construye el handler. Si falta el servicio, el proveedor
// de tokens o el autorizador de sesiones, ambos endpoints responden 503
// (fail-closed): nunca se sirve ni se modifica estado de onboarding sin identidad
// verificada ni persistencia.
func NewOnboardingHandler(
	service *appOnboarding.Service,
	tokens ports.TokenProvider,
	sessions ports.SessionAuthorizer,
	logger *slog.Logger,
) *OnboardingHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &OnboardingHandler{service: service, logger: logger}
	if service != nil && tokens != nil && sessions != nil {
		protected := middleware.AuthenticateBearerWithSession(tokens, sessions)
		h.protected = protected(http.HandlerFunc(h.serveGet))
		h.protectedPatch = protected(http.HandlerFunc(h.servePatch))
	}
	return h
}

// ServeGet es el punto de entrada HTTP de E018. Fija la cabecera de caché antes
// de cualquier decisión: la información de identidad nunca se cachea.
func (h *OnboardingHandler) ServeGet(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")

	if h == nil || h.service == nil || h.protected == nil {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}

	h.protected.ServeHTTP(w, r)
}

// serveGet se ejecuta dentro de la cadena autenticada: `sub` ya está verificado.
func (h *OnboardingHandler) serveGet(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		// Defensa en profundidad: el middleware ya exige `sub`.
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}

	state, err := h.service.Get(r.Context(), userID)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}

	if err := httputil.WriteJSON(w, http.StatusOK, onboardingResponseFromState(state)); err != nil {
		h.logger.ErrorContext(r.Context(), "onboarding: fallo al serializar la respuesta",
			slog.String("error_class", "response_write"),
			slog.String("trace_id", traceID(r)))
	}
}

// onboardingResponse es el objeto `Onboarding` del contrato: exactamente siete
// campos. No usa `omitempty` en booleanos ni en `completedSteps`, y `completedAt`
// se emite como `null` explícito cuando no hay marca.
type onboardingResponse struct {
	CompletedSteps []string `json:"completedSteps"`
	Done           bool     `json:"done"`
	Version        int64    `json:"version"`
	Microphone     bool     `json:"microphone"`
	Notifications  bool     `json:"notifications"`
	Configured     bool     `json:"configured"`
	CompletedAt    *string  `json:"completedAt"`
}

// ServePatch es el punto de entrada HTTP de E019
// (`PATCH /api/v1/users/me/onboarding`).
func (h *OnboardingHandler) ServePatch(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")

	if h == nil || h.service == nil || h.protectedPatch == nil {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}

	h.protectedPatch.ServeHTTP(w, r)
}

// servePatch se ejecuta dentro de la cadena autenticada: `sub` ya está verificado
// y la sesión es autoritativa.
func (h *OnboardingHandler) servePatch(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		// Defensa en profundidad: el middleware ya exige `sub`.
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}

	var request OnboardingPatchRequest
	if err := BindJSONLimit(w, r, &request, OnboardingPatchMaxRequestBodyBytes); err != nil {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "body", Reason: "invalid_json"},
		})
		return
	}

	command, violation := toPatchCommand(request)
	if violation != nil {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{*violation})
		return
	}

	state, err := h.service.Patch(r.Context(), userID, command)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}

	if err := httputil.WriteJSON(w, http.StatusOK, onboardingResponseFromState(state)); err != nil {
		h.logger.ErrorContext(r.Context(), "onboarding: fallo al serializar la respuesta",
			slog.String("error_class", "response_write"),
			slog.String("trace_id", traceID(r)))
	}
}

// toPatchCommand traduce el DTO al comando de aplicación. Un `completedSteps`
// ausente o `null` produce 400: el contrato lo exige y no se interpreta como
// conjunto vacío. Los pasos desconocidos se rechazan en la capa de aplicación
// con 422.
func toPatchCommand(request OnboardingPatchRequest) (appOnboarding.PatchCommand, *httputil.ErrorDetail) {
	command := appOnboarding.PatchCommand{
		Microphone:    request.Microphone,
		Notifications: request.Notifications,
		Configured:    request.Configured,
	}
	if request.CompletedSteps == nil {
		return command, &httputil.ErrorDetail{Field: "completedSteps", Reason: "required"}
	}
	command.CompletedSteps = *request.CompletedSteps
	return command, nil
}

// onboardingResponseFromState reduce el estado de aplicación al DTO exacto.
func onboardingResponseFromState(state *appOnboarding.State) onboardingResponse {
	steps := state.CompletedSteps
	if steps == nil {
		steps = []string{}
	}
	return onboardingResponse{
		CompletedSteps: steps,
		Done:           state.Done,
		Version:        state.Version,
		Microphone:     state.Microphone,
		Notifications:  state.Notifications,
		Configured:     state.Configured,
		CompletedAt:    formatOnboardingTime(state.CompletedAt),
	}
}

// formatOnboardingTime serializa `date-time` en RFC3339 UTC. Sin marca devuelve
// `nil`, que `encoding/json` emite como `null` al no existir `omitempty`.
func formatOnboardingTime(value *time.Time) *string {
	if value == nil || value.IsZero() {
		return nil
	}
	texto := value.UTC().Format(time.RFC3339)
	return &texto
}

// writeServiceError traduce los errores de aplicación a códigos del contrato sin
// revelar identificadores ni el estado interno.
func (h *OnboardingHandler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	var pasoDesconocido *appOnboarding.UnknownStepError
	switch {
	case errors.As(err, &pasoDesconocido):
		// Paso fuera del catálogo cerrado: regla de negocio, no error de sintaxis.
		// No se refleja el valor recibido.
		writeRegisterError(w, r, http.StatusUnprocessableEntity, "BUSINESS_RULE_VIOLATION", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "completedSteps", Reason: "unknown_step"},
		})
	case errors.Is(err, appOnboarding.ErrConflict):
		// La versión cambió dos veces durante la escritura: el cliente debe releer
		// con E018 antes de reintentar. Nada se sobrescribió.
		writeRegisterError(w, r, http.StatusConflict, "CONFLICT", "el estado de onboarding fue modificado", nil)
	case errors.Is(err, appOnboarding.ErrNotFound):
		writeRegisterError(w, r, http.StatusNotFound, "NOT_FOUND", "recurso no encontrado", nil)
	case errors.Is(err, appOnboarding.ErrUnauthorized):
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
	case errors.Is(err, appOnboarding.ErrUnavailable):
		h.logger.ErrorContext(r.Context(), "onboarding: dependencia no disponible",
			slog.String("error_class", "onboarding_unavailable"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
	default:
		h.logger.ErrorContext(r.Context(), "onboarding: fallo interno",
			slog.String("error_class", "internal_error"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
	}
}
