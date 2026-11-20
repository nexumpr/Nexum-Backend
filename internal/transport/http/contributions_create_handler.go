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

// ContributionCreateHandler sirve E044 (`POST /api/v1/goal-contributions`).
//
// La escritura es atomica en la capa de aplicacion/adaptador: aportacion, puntero, un
// movimiento de ahorro, punteros de movimiento, metas afectadas y registro de idempotencia en
// una sola `TransactWriteItems` condicionada. El transporte solo valida forma, identidad y
// cuota; nunca confirma una escritura parcial. `Idempotency-Key` es obligatoria y el replay
// estable se devuelve con `X-Idempotent-Replay: true`.
type ContributionCreateHandler struct {
	service   *appContrib.Service
	logger    *slog.Logger
	protected http.Handler
}

// NewContributionCreateHandler construye E044. Sin servicio, sin proveedor de tokens o sin
// autorizador de sesiones el handler responde 503: nunca autoriza solo con la firma.
func NewContributionCreateHandler(service *appContrib.Service, tokens ports.TokenProvider, sessions ports.SessionAuthorizer, limitador LimitadorRuta, logger *slog.Logger) *ContributionCreateHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &ContributionCreateHandler{service: service, logger: logger}
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
func (h *ContributionCreateHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if h == nil || h.service == nil || h.protected == nil {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}
	h.protected.ServeHTTP(w, r)
}

// contributionCreateRequest replica `ContributionCreate` del contrato. No admite campos
// desconocidos (`BindJSONLimit` con `DisallowUnknownFields`).
type contributionCreateRequest struct {
	Amount      string                      `json:"amount"`
	Currency    string                      `json:"currency"`
	Allocations []appContrib.WireAllocation `json:"allocations"`
	OccurredAt  string                      `json:"occurredAt"`
}

func (h *ContributionCreateHandler) serve(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}
	key := strings.TrimSpace(r.Header.Get(HeaderIdempotencyKey))
	if key == "" {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada",
			[]httputil.ErrorDetail{{Field: HeaderIdempotencyKey, Reason: "required"}})
		return
	}
	var peticion contributionCreateRequest
	if err := BindJSONLimit(w, r, &peticion, MaxBodyAportaciones); err != nil {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", nil)
		return
	}
	resultado, err := h.service.Create(r.Context(), appContrib.CreateCommand{
		UserID:         userID,
		IdempotencyKey: key,
		Amount:         peticion.Amount,
		Currency:       peticion.Currency,
		Allocations:    peticion.Allocations,
		OccurredAt:     peticion.OccurredAt,
	})
	if err != nil {
		WriteContributionsError(w, r, h.logger, err)
		return
	}
	if strings.TrimSpace(resultado.Location) != "" {
		w.Header().Set("Location", resultado.Location)
	}
	if resultado.Replayed {
		w.Header().Set("X-Idempotent-Replay", "true")
	}
	if err := httputil.WriteJSONBytes(w, resultado.Status, resultado.Body); err != nil {
		h.logger.ErrorContext(r.Context(), "contributions: fallo al escribir la respuesta",
			slog.String("trace_id", traceID(r)))
	}
}
