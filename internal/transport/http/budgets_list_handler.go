package http

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	appBudget "github.com/nexumpr/Nexum-Backend/internal/application/budgetset"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// BudgetsListHandler sirve E048 (`GET /api/v1/budgets`).
type BudgetsListHandler struct {
	service   *appBudget.Service
	logger    *slog.Logger
	protected http.Handler
}

// NewBudgetsListHandler construye el handler.
func NewBudgetsListHandler(service *appBudget.Service, tokens ports.TokenProvider, sessions ports.SessionAuthorizer, limitador LimitadorRuta, logger *slog.Logger) *BudgetsListHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &BudgetsListHandler{service: service, logger: logger}
	if service != nil && tokens != nil && sessions != nil {
		interior := LimitarPorUsuario(limitador, "budgets", h.serve)
		h.protected = middleware.AuthenticateBearerWithSession(tokens, sessions)(http.HandlerFunc(interior))
	}
	return h
}

// ServeHTTP fija caché privada y valida el método.
func (h *BudgetsListHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

func (h *BudgetsListHandler) serve(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}
	limit := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		valor, err := strconv.Atoi(raw)
		if err != nil || valor < 1 || valor > appBudget.MaxLimit {
			writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{{Field: "limit", Reason: "invalid"}})
			return
		}
		limit = valor
	}
	pagina, err := h.service.List(r.Context(), userID, r.URL.Query().Get("period"), r.URL.Query().Get("cursor"), limit)
	if err != nil {
		writeBudgetsError(w, r, h.logger, err)
		return
	}
	if strings.TrimSpace(pagina.ETag) != "" {
		w.Header().Set("ETag", pagina.ETag)
	}
	respuesta := struct {
		Items      []appBudget.WireBudget `json:"items"`
		NextCursor string                 `json:"nextCursor,omitempty"`
	}{Items: pagina.Wire(), NextCursor: pagina.NextCursor}
	if err := httputil.WriteJSON(w, http.StatusOK, respuesta); err != nil {
		h.logger.ErrorContext(r.Context(), "budgets: fallo al serializar", slog.String("trace_id", traceID(r)))
	}
}

func writeBudgetsError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	switch {
	case errors.Is(err, appBudget.ErrInvalidRequest):
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", nil)
	case errors.Is(err, appBudget.ErrUnauthorized):
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
	case errors.Is(err, appBudget.ErrInconsistent):
		if logger != nil {
			logger.ErrorContext(r.Context(), "budgets: registro inconsistente", slog.String("error_class", "inconsistent"), slog.String("trace_id", traceID(r)))
		}
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
	case errors.Is(err, appBudget.ErrUnavailable):
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
	default:
		if logger != nil {
			logger.ErrorContext(r.Context(), "budgets: error no clasificado", slog.String("error_class", "internal"), slog.String("trace_id", traceID(r)))
		}
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
	}
}
