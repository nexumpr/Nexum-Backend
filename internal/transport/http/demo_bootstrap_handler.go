package http

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/nexumpr/Nexum-Backend/internal/application/demobootstrap"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// DemoBootstrapHandler sirve E074 (`GET /api/v1/demo/bootstrap`). La respuesta
// es estable e independiente del sujeto; el GET no modifica ningún dato.
type DemoBootstrapHandler struct {
	service   *demobootstrap.Service
	logger    *slog.Logger
	protected http.Handler
}

// NewDemoBootstrapHandler construye el handler. enabled debe venir de
// configuración controlada (por ejemplo NEXUM_DEMO_ENABLED); nunca se fuerza a
// true aquí. Sin tokens o sin autorizador, todas las peticiones responden 503.
func NewDemoBootstrapHandler(
	enabled bool,
	tokens ports.TokenProvider,
	sessions ports.SessionAuthorizer,
	limitador LimitadorRuta,
	logger *slog.Logger,
) *DemoBootstrapHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &DemoBootstrapHandler{service: demobootstrap.NewService(enabled), logger: logger}
	if tokens == nil || sessions == nil {
		return h
	}
	conCuota := LimitarPorUsuarioContrato(limitador, "analytics", h.serve)
	h.protected = middleware.AuthenticateBearerWithSession(tokens, sessions)(conCuota)
	return h
}

// ServeHTTP fija Cache-Control antes de cualquier decisión. El único método y
// la ausencia de parámetros son parte del contrato; se rechazan con 400 para no
// emitir códigos no declarados en el OpenAPI.
func (h *DemoBootstrapHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	if h == nil || h.service == nil || h.protected == nil {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}
	h.protected.ServeHTTP(w, r)
}

func (h *DemoBootstrapHandler) serve(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}
	// El sujeto sólo se usa para autorizar: nunca entra en la respuesta ni en logs.
	if err := httputil.WriteJSON(w, http.StatusOK, h.service.Bootstrap()); err != nil {
		h.logger.ErrorContext(r.Context(), "E074: fallo al serializar", slog.String("trace_id", traceID(r)))
	}
}
