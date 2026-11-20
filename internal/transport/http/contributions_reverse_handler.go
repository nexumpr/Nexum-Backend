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

// ContributionReverseHandler sirve E047
// (`POST /api/v1/goal-contributions/{contributionId}/reverse`).
//
// Contrato:
//   - `Authorization: Bearer` RS256 y estado autoritativo de la sesión en DynamoDB.
//   - `Idempotency-Key` OBLIGATORIA (16..128 caracteres).
//   - DTO cerrado `ReverseContributionRequest` con `reason` obligatorio; campos desconocidos
//     producen 400.
//   - La reversión es exactamente una vez: una segunda operación sobre una aportación ya
//     revertida responde 409 y no altera saldos.
//   - La respuesta es reproducible byte a byte: una repetición idéntica devuelve el mismo
//     cuerpo/estado con `X-Idempotent-Replay: true`.
type ContributionReverseHandler struct {
	service   *appContrib.Service
	logger    *slog.Logger
	protected http.Handler
}

// NewContributionReverseHandler construye E047. Sin servicio, sin proveedor de tokens o sin
// autorizador de sesiones el handler responde 503 (fail-closed).
func NewContributionReverseHandler(service *appContrib.Service, tokens ports.TokenProvider, sessions ports.SessionAuthorizer, limitador LimitadorRuta, logger *slog.Logger) *ContributionReverseHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &ContributionReverseHandler{service: service, logger: logger}
	protegido := CadenaAportaciones(service, tokens, sessions, h.serve)
	if protegido != nil {
		h.protected = LimitarPorUsuario(limitador, "contributions", protegido.ServeHTTP)
	}
	return h
}

// ServeHTTP fija la caché privada y delega en la cadena autenticada.
func (h *ContributionReverseHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if h == nil || h.service == nil || h.protected == nil {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}
	h.protected.ServeHTTP(w, r)
}

type contributionReverseRequest struct {
	Reason string `json:"reason"`
}

func (h *ContributionReverseHandler) serve(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{{Field: "Idempotency-Key", Reason: "required"}})
		return
	}
	var peticion contributionReverseRequest
	if err := BindJSONLimit(w, r, &peticion, MaxBodyAportaciones); err != nil {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", nil)
		return
	}
	resultado, err := h.service.Reverse(r.Context(), appContrib.ReverseCommand{
		UserID:         userID,
		ContributionID: strings.TrimSpace(r.PathValue("contributionId")),
		IdempotencyKey: key,
		Reason:         peticion.Reason,
	})
	if err != nil {
		WriteContributionsError(w, r, h.logger, err)
		return
	}
	if resultado.Replayed {
		w.Header().Set("X-Idempotent-Replay", "true")
	}
	if err := httputil.WriteJSONBytes(w, resultado.Status, resultado.Body); err != nil {
		h.logger.ErrorContext(r.Context(), "contributions: fallo al escribir la reversion", slog.String("trace_id", traceID(r)))
	}
}
