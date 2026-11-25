package http

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/nexumpr/Nexum-Backend/internal/application/voiceparse"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

const maxVoiceParseBodyBytes int64 = 4096

// VoiceParseHandler sirve E075 (`POST /api/v1/voice/parse`). Sólo interpreta
// texto transcrito: no recibe audio, no escribe DynamoDB y no llama a E028.
type VoiceParseHandler struct {
	service   *voiceparse.Service
	logger    *slog.Logger
	protected http.Handler
}

// NewVoiceParseHandler construye el handler. Sin tokens o sin autorizador de
// sesión todas las peticiones responden 503 (fail-closed). El limitador se
// acepta por consistencia con el resto de rutas protegidas; si es nil no hay
// cuota, pero la autenticación autoritativa sigue siendo obligatoria.
func NewVoiceParseHandler(
	tokens ports.TokenProvider,
	sessions ports.SessionAuthorizer,
	limitador LimitadorRuta,
	logger *slog.Logger,
) *VoiceParseHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &VoiceParseHandler{service: voiceparse.NewService(), logger: logger}
	if tokens == nil || sessions == nil {
		return h
	}
	conCuota := LimitarPorUsuarioContrato(limitador, "analytics", h.serve)
	h.protected = middleware.AuthenticateBearerWithSession(tokens, sessions)(conCuota)
	return h
}

// ServeHTTP fija Cache-Control antes de cualquier decisión. Los métodos y los
// parámetros de consulta no declarados se rechazan con 400 INVALID_REQUEST para
// no emitir estados HTTP ausentes del contrato.
func (h *VoiceParseHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "method", Reason: "method_not_allowed"},
		})
		return
	}
	if len(r.URL.Query()) != 0 {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "query", Reason: "query_not_allowed"},
		})
		return
	}
	if h == nil || h.service == nil || h.protected == nil {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}
	h.protected.ServeHTTP(w, r)
}

func (h *VoiceParseHandler) serve(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}

	var req voiceparse.VoiceParseRequestWire
	if err := httputil.DecodeJSON(w, r, &req, maxVoiceParseBodyBytes); err != nil {
		// El error de decode puede contener nombres de campo; nunca se registra
		// porque el cuerpo incluye texto potencialmente sensible.
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "body", Reason: "invalid_request_body"},
		})
		return
	}

	resultado, err := h.service.Parse(req)
	if err != nil {
		h.writeParseError(w, r, err)
		return
	}
	if err := httputil.WriteJSON(w, http.StatusOK, resultado); err != nil {
		// Sólo se registra la clase y el trace: jamás el texto del usuario.
		h.logger.ErrorContext(r.Context(), "E075: fallo al serializar", slog.String("trace_id", traceID(r)))
	}
}

func (h *VoiceParseHandler) writeParseError(w http.ResponseWriter, r *http.Request, err error) {
	var reqErr *voiceparse.RequestError
	switch {
	case errors.As(err, &reqErr):
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: reqErr.Field, Reason: reqErr.Reason},
		})
	case errors.Is(err, voiceparse.ErrNegativeAmount):
		writeRegisterError(w, r, http.StatusUnprocessableEntity, "BUSINESS_RULE_VIOLATION", "regla de negocio no satisfecha", []httputil.ErrorDetail{
			{Field: "text", Reason: "negative_amount"},
		})
	default:
		h.logger.ErrorContext(r.Context(), "E075: error no clasificado",
			slog.String("error_class", "internal"), slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
	}
}
