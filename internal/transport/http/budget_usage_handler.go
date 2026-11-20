package http

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	appUsage "github.com/nexumpr/Nexum-Backend/internal/application/budgetusage"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// BudgetUsageHandler sirve E050 (`GET /api/v1/budgets/usage`).
type BudgetUsageHandler struct {
	service   *appUsage.Service
	logger    *slog.Logger
	protected http.Handler
}

// NewBudgetUsageHandler construye el handler.
func NewBudgetUsageHandler(service *appUsage.Service, tokens ports.TokenProvider, sessions ports.SessionAuthorizer, limitador LimitadorRuta, logger *slog.Logger) *BudgetUsageHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &BudgetUsageHandler{service: service, logger: logger}
	if service != nil && tokens != nil && sessions != nil {
		protegido := middleware.AuthenticateBearerWithSession(tokens, sessions)(http.HandlerFunc(h.serve))
		h.protected = LimitarPorUsuario(limitador, "budgets", protegido.ServeHTTP)
	}
	return h
}

// ServeHTTP fija caché y valida el método.
func (h *BudgetUsageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

func (h *BudgetUsageHandler) serve(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}
	items, err := h.service.Usage(r.Context(), userID, r.URL.Query().Get("period"))
	if err != nil {
		writeBudgetUsageError(w, r, h.logger, err)
		return
	}
	respuesta := struct {
		Items []appUsage.WireUsage `json:"items"`
	}{Items: appUsage.WireItems(items)}
	if err := httputil.WriteJSON(w, http.StatusOK, respuesta); err != nil {
		h.logger.ErrorContext(r.Context(), "budgets: fallo al serializar uso", slog.String("trace_id", traceID(r)))
	}
}

func writeBudgetUsageError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	switch {
	case errors.Is(err, appUsage.ErrInvalidPeriod):
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{{Field: "period", Reason: "invalid_format"}})
	case errors.Is(err, appUsage.ErrUnauthorized):
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
	case errors.Is(err, appUsage.ErrInconsistent):
		if logger != nil {
			logger.ErrorContext(r.Context(), "budgets: uso inconsistente", slog.String("error_class", "inconsistent"), slog.String("trace_id", traceID(r)))
		}
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
	case errors.Is(err, appUsage.ErrOverflow):
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "volumen del periodo excede el limite de calculo", nil)
	case errors.Is(err, appUsage.ErrUnavailable):
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
	default:
		if logger != nil {
			logger.ErrorContext(r.Context(), "budgets: error no clasificado", slog.String("error_class", "internal"), slog.String("trace_id", traceID(r)))
		}
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
	}
}
