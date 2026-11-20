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

// ContributionPreviewHandler sirve E043 (`POST /api/v1/goal-contributions/preview`).
//
// E043 es validacion y calculo puros: no escribe nada. La autorizacion usa la cadena real
// Bearer RS256 + estado autoritativo de la sesion en DynamoDB (`CadenaAportaciones`), el DTO
// es cerrado y los importes viajan como decimal string convertidos a centavos en la capa de
// aplicacion. La cuota por usuario es opcional: sin Redis el limitador es nil y no se emite
// 429 (no se simula una cuota inexistente).
type ContributionPreviewHandler struct {
	service   *appContrib.Service
	logger    *slog.Logger
	protected http.Handler
}

// NewContributionPreviewHandler construye E043. Sin servicio, sin proveedor de tokens o sin
// autorizador de sesiones el handler responde 503: nunca autoriza solo con la firma.
func NewContributionPreviewHandler(service *appContrib.Service, tokens ports.TokenProvider, sessions ports.SessionAuthorizer, limitador LimitadorRuta, logger *slog.Logger) *ContributionPreviewHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &ContributionPreviewHandler{service: service, logger: logger}
	// La cuota se aplica DENTRO de la cadena autenticada: `LimitarPorUsuario` necesita la
	// identidad que inyecta el middleware. Invertir el orden la desactivaria en silencio.
	interior := h.serve
	if limitador != nil {
		interior = LimitarPorUsuario(limitador, "contributions", h.serve)
	}
	if protegido := CadenaAportaciones(service, tokens, sessions, interior); protegido != nil {
		h.protected = protegido
	}
	return h
}

// ServeHTTP fija la politica de cache y delega en la cadena protegida.
func (h *ContributionPreviewHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if h == nil || h.service == nil || h.protected == nil {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}
	h.protected.ServeHTTP(w, r)
}

// contributionPreviewRequest replica `ContributionPreviewRequest` del contrato. No admite
// campos desconocidos (`BindJSONLimit` con `DisallowUnknownFields`).
type contributionPreviewRequest struct {
	Amount      string                      `json:"amount"`
	Currency    string                      `json:"currency"`
	Allocations []appContrib.WireAllocation `json:"allocations"`
}

func (h *ContributionPreviewHandler) serve(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}
	var peticion contributionPreviewRequest
	if err := BindJSONLimit(w, r, &peticion, MaxBodyAportaciones); err != nil {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", nil)
		return
	}
	resultado, err := h.service.Preview(r.Context(), appContrib.PreviewCommand{
		UserID:      userID,
		Amount:      peticion.Amount,
		Currency:    peticion.Currency,
		Allocations: peticion.Allocations,
	})
	if err != nil {
		WriteContributionsError(w, r, h.logger, err)
		return
	}
	if err := httputil.WriteJSON(w, http.StatusOK, resultado.Wire()); err != nil {
		h.logger.ErrorContext(r.Context(), "contributions: fallo al serializar preview",
			slog.String("trace_id", traceID(r)))
	}
}
