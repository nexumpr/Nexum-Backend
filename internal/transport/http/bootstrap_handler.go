package http

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	appBootstrap "github.com/nexumpr/Nexum-Backend/internal/application/bootstrap"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// BootstrapHandler sirve E021 (`GET /api/v1/bootstrap`). Sólo agrega el perfil
// propio y el resumen financiero del periodo vigente; nunca expone sesión,
// credenciales ni atributos internos de DynamoDB.
type BootstrapHandler struct {
	service   *appBootstrap.Service
	logger    *slog.Logger
	protected http.Handler
}

// NewBootstrapHandler construye el handler. Sin servicio, tokens o autorizador
// de sesión, todas las peticiones responden 503 (fail-closed).
func NewBootstrapHandler(
	service *appBootstrap.Service,
	tokens ports.TokenProvider,
	sessions ports.SessionAuthorizer,
	limitador LimitadorRuta,
	logger *slog.Logger,
) *BootstrapHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &BootstrapHandler{service: service, logger: logger}
	if service == nil || !service.Disponible() || tokens == nil || sessions == nil {
		return h
	}
	conCuota := LimitarPorUsuarioContrato(limitador, "analytics", h.serve)
	h.protected = middleware.AuthenticateBearerWithSession(tokens, sessions)(conCuota)
	return h
}

// ServeHTTP fija la caché privada antes de cualquier decisión. La ruta no
// acepta otros métodos ni parámetros declarados; se rechazan con 400 para no
// emitir códigos ajenos al contrato.
func (h *BootstrapHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
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
	if h == nil || h.service == nil || !h.service.Disponible() || h.protected == nil {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}
	h.protected.ServeHTTP(w, r)
}

func (h *BootstrapHandler) serve(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}
	datos, err := h.service.Load(r.Context(), userID)
	if err != nil {
		writeBootstrapError(w, r, h.logger, err)
		return
	}
	if err := httputil.WriteJSON(w, http.StatusOK, datos); err != nil {
		h.logger.ErrorContext(r.Context(), "E021: fallo al serializar", slog.String("trace_id", traceID(r)))
	}
}

func writeBootstrapError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	switch {
	case errors.Is(err, appBootstrap.ErrUnauthorized):
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
	case errors.Is(err, appBootstrap.ErrNotFound):
		writeRegisterError(w, r, http.StatusNotFound, "NOT_FOUND", "recurso no encontrado", nil)
	case errors.Is(err, appBootstrap.ErrUnavailable):
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
	default:
		if logger != nil {
			logger.ErrorContext(r.Context(), "E021: error no clasificado", slog.String("trace_id", traceID(r)))
		}
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "solicitud no completada", nil)
	}
}
