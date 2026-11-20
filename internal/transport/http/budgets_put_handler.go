package http

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	appBudget "github.com/nexumpr/Nexum-Backend/internal/application/budgetset"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// BudgetsReplaceHandler sirve E049 (`PUT /api/v1/budgets`).
type BudgetsReplaceHandler struct {
	service   *appBudget.Service
	logger    *slog.Logger
	protected http.Handler
}

// NewBudgetsReplaceHandler construye el handler.
func NewBudgetsReplaceHandler(service *appBudget.Service, tokens ports.TokenProvider, sessions ports.SessionAuthorizer, limitador LimitadorRuta, logger *slog.Logger) *BudgetsReplaceHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &BudgetsReplaceHandler{service: service, logger: logger}
	if service != nil && tokens != nil && sessions != nil {
		protegido := middleware.AuthenticateBearerWithSession(tokens, sessions)(http.HandlerFunc(h.serve))
		h.protected = LimitarPorUsuario(limitador, "budgets", protegido.ServeHTTP)
	}
	return h
}

// ServeHTTP fija caché y valida el método.
func (h *BudgetsReplaceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if r.Method != http.MethodPut {
		w.Header().Set("Allow", http.MethodPut)
		writeRegisterError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "el metodo HTTP no esta permitido para este recurso", nil)
		return
	}
	if h == nil || h.service == nil || h.protected == nil {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}
	h.protected.ServeHTTP(w, r)
}

type budgetsReplaceRequest struct {
	Period   string                  `json:"period"`
	Currency string                  `json:"currency"`
	Budgets  []appBudget.BudgetInput `json:"budgets"`
}

func (h *BudgetsReplaceHandler) serve(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}
	var peticion budgetsReplaceRequest
	if err := BindJSONLimit(w, r, &peticion, 64<<10); err != nil {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", nil)
		return
	}
	pagina, err := h.service.Replace(r.Context(), appBudget.ReplaceCommand{
		UserID:   userID,
		IfMatch:  r.Header.Get("If-Match"),
		Period:   peticion.Period,
		Currency: peticion.Currency,
		Budgets:  peticion.Budgets,
	})
	if err != nil {
		writeBudgetsReplaceError(w, r, h.logger, err)
		return
	}
	if strings.TrimSpace(pagina.ETag) != "" {
		w.Header().Set("ETag", pagina.ETag)
	}
	respuesta := struct {
		Items []appBudget.WireBudget `json:"items"`
	}{Items: pagina.Wire()}
	if err := httputil.WriteJSON(w, http.StatusOK, respuesta); err != nil {
		h.logger.ErrorContext(r.Context(), "budgets: fallo al serializar reemplazo", slog.String("trace_id", traceID(r)))
	}
}

func writeBudgetsReplaceError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	var violacion *appBudget.Violation
	switch {
	case errors.As(err, &violacion):
		writeRegisterError(w, r, http.StatusUnprocessableEntity, "BUSINESS_RULE_VIOLATION", "regla de negocio incumplida", []httputil.ErrorDetail{{Field: violacion.Field, Reason: violacion.Reason}})
	case errors.Is(err, appBudget.ErrPreconditionRequired):
		writeRegisterError(w, r, http.StatusPreconditionRequired, "PRECONDITION_REQUIRED", "If-Match requerido", []httputil.ErrorDetail{{Field: "If-Match", Reason: "required"}})
	case errors.Is(err, appBudget.ErrPreconditionInvalid):
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "If-Match invalido", []httputil.ErrorDetail{{Field: "If-Match", Reason: "invalid_format"}})
	case errors.Is(err, appBudget.ErrVersionConflict):
		writeRegisterError(w, r, http.StatusPreconditionFailed, "VERSION_CONFLICT", "version del conjunto desactualizada", nil)
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
