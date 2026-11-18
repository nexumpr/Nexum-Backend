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

// OnboardingHandler expone `GET /api/v1/users/me/onboarding` (E018).
//
// La ruta exige JWT RS256 y sesión autoritativa: la identidad se deriva
// exclusivamente de `sub` y `sid` verificados. El cliente no puede consultar el
// onboarding de otro usuario.
//
// Contrato: 200 con `Onboarding` y cabecera `Cache-Control: private, no-store`;
// errores con el sobre canónico `ErrorEnvelope`. Códigos contemplados: 200, 400,
// 401, 403, 404, 429, 500 y 503.
type OnboardingHandler struct {
	service   *appOnboarding.Service
	logger    *slog.Logger
	protected http.Handler
}

// NewOnboardingHandler construye el handler. Si falta el servicio, el proveedor
// de tokens o el autorizador de sesiones, el endpoint responde 503 (fail-closed):
// nunca se sirve estado de onboarding sin identidad verificada ni persistencia.
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
	switch {
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
