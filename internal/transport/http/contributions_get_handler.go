package http

import (
	"log/slog"
	"net/http"
	"strings"

	appContrib "github.com/nexumpr/Nexum-Backend/internal/application/contributions"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// ContributionGetHandler sirve E046 (`GET /api/v1/goal-contributions/{contributionId}`).
//
// Contrato:
//   - `Authorization: Bearer` RS256 y estado autoritativo de la sesión en DynamoDB.
//   - Sin cuerpo de petición; el `userId` se deriva siempre del sujeto autenticado.
//   - Resolución O(1) por el localizador `CONTRIBUTIONPTR#<id>` dentro de `USER#<sub>`; nunca
//     `Scan` y nunca la partición de otro usuario.
//   - Un recurso ajeno y un recurso inexistente producen el mismo 404: no se filtra existencia.
type ContributionGetHandler struct {
	service   *appContrib.Service
	logger    *slog.Logger
	protected http.Handler
}

// NewContributionGetHandler construye E046. Sin servicio, sin proveedor de tokens o sin
// autorizador de sesiones el handler responde 503 (fail-closed).
func NewContributionGetHandler(service *appContrib.Service, tokens ports.TokenProvider, sessions ports.SessionAuthorizer, limitador LimitadorRuta, logger *slog.Logger) *ContributionGetHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &ContributionGetHandler{service: service, logger: logger}
	protegido := CadenaAportaciones(service, tokens, sessions, h.serve)
	if protegido != nil {
		h.protected = LimitarPorUsuario(limitador, "contributions", protegido.ServeHTTP)
	}
	return h
}

// ServeHTTP fija la caché privada y delega en la cadena autenticada.
func (h *ContributionGetHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if h == nil || h.service == nil || h.protected == nil {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}
	h.protected.ServeHTTP(w, r)
}

func (h *ContributionGetHandler) serve(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}
	id := strings.TrimSpace(r.PathValue("contributionId"))
	aportacion, err := h.service.Get(r.Context(), userID, id)
	if err != nil {
		WriteContributionsError(w, r, h.logger, err)
		return
	}
	if err := httputil.WriteJSON(w, http.StatusOK, aportacion.Wire()); err != nil {
		h.logger.ErrorContext(r.Context(), "contributions: fallo al serializar la aportacion", slog.String("trace_id", traceID(r)))
	}
}
