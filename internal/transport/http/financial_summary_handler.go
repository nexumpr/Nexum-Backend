package http

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	appSummary "github.com/nexumpr/Nexum-Backend/internal/application/financialsummary"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// FinancialSummaryHandler sirve E026 (`GET /api/v1/financial-summary`).
type FinancialSummaryHandler struct {
	service   *appSummary.Service
	logger    *slog.Logger
	protected http.Handler
}

// NewFinancialSummaryHandler construye el handler. Sin servicio, tokens o autorizador
// responde 503 (fail-closed).
func NewFinancialSummaryHandler(
	service *appSummary.Service,
	tokens ports.TokenProvider,
	sessions ports.SessionAuthorizer,
	limitador LimitadorRuta,
	logger *slog.Logger,
) *FinancialSummaryHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &FinancialSummaryHandler{service: service, logger: logger}
	if service != nil && tokens != nil && sessions != nil {
		interior := LimitarPorUsuario(limitador, "financial-summary", h.serve)
		h.protected = middleware.AuthenticateBearerWithSession(tokens, sessions)(http.HandlerFunc(interior))
	}
	return h
}

// ServeHTTP fija caché privada y valida el método.
func (h *FinancialSummaryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeRegisterError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "el metodo HTTP no esta permitido para este recurso", nil)
		return
	}
	if h == nil || h.service == nil || h.protected == nil {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}
	h.protected.ServeHTTP(w, r)
}

func (h *FinancialSummaryHandler) serve(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}
	resumen, err := h.service.Summary(r.Context(), userID, r.URL.Query().Get("period"))
	if err != nil {
		writeFinancialSummaryError(w, r, h.logger, err)
		return
	}
	if err := httputil.WriteJSON(w, http.StatusOK, resumen.Wire()); err != nil {
		h.logger.ErrorContext(r.Context(), "financial-summary: fallo al serializar", slog.String("trace_id", traceID(r)))
	}
}

func writeFinancialSummaryError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	switch {
	case errors.Is(err, appSummary.ErrInvalidPeriod):
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{{Field: "period", Reason: "invalid_format"}})
	case errors.Is(err, appSummary.ErrUnauthorized):
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
	case errors.Is(err, appSummary.ErrUnavailable):
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
	default:
		if logger != nil {
			logger.ErrorContext(r.Context(), "financial-summary: error no clasificado", slog.String("error_class", "internal"), slog.String("trace_id", traceID(r)))
		}
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
	}
}
