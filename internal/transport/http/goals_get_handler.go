package http

import (
	"log/slog"
	"net/http"
	"strings"

	appGoals "github.com/nexumpr/Nexum-Backend/internal/application/goals"
	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// Transporte de lectura de una meta: E040 (`GET /api/v1/goals/{goalId}`).
//
// Reglas:
//   - `Authorization: Bearer` RS256 y estado autoritativo de la sesión en DynamoDB.
//   - Información financiera: `Cache-Control: private, no-store` en toda respuesta de éxito.
//   - La meta se proyecta SIEMPRE al DTO cerrado de nueve campos: `userId`, `entityType`,
//     `createdAt` y cualquier atributo interno no tienen camino de serialización.
//   - Un recurso ajeno y uno inexistente producen el mismo 404: no se enumera.

// GoalGetHandler sirve `GET /api/v1/goals/{goalId}` (E040).
type GoalGetHandler struct {
	service   *appGoals.Service
	logger    *slog.Logger
	protected http.Handler
}

// NewGoalGetHandler construye el handler. Sin servicio, sin proveedor de tokens o sin
// autorizador de sesión responde 503 (fail-closed).
func NewGoalGetHandler(
	service *appGoals.Service,
	tokens ports.TokenProvider,
	sessions ports.SessionAuthorizer,
	logger *slog.Logger,
) *GoalGetHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &GoalGetHandler{service: service, logger: logger}
	h.protected = cadenaMetas(service, tokens, sessions, h.serve)
	return h
}

// ServeHTTP fija la cabecera de caché y delega en la cadena autenticada.
func (h *GoalGetHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")

	if h == nil || h.service == nil || h.protected == nil {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}
	h.protected.ServeHTTP(w, r)
}

// serve se ejecuta con la sesión ya verificada.
func (h *GoalGetHandler) serve(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}

	goalID := strings.TrimSpace(r.PathValue("goalId"))
	if !appIdentity.IsUUID(goalID) {
		// El parámetro declara `format: uuid`: un valor con otra forma es una solicitud
		// inválida y no se refleja en la respuesta.
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "goalId", Reason: "invalid_format"},
		})
		return
	}

	goal, err := h.service.Get(r.Context(), userID, goalID)
	if err != nil {
		writeGoalsError(w, r, h.logger, err)
		return
	}
	contract, errProyeccion := appGoals.ContractOf(goal)
	if errProyeccion != nil {
		writeGoalsError(w, r, h.logger, errProyeccion)
		return
	}

	// `ETag` fuerte `"vN"`: es el validador que E041 exige en `If-Match`.
	w.Header().Set("ETag", FormatETag(goal.Version))
	if errEscritura := httputil.WriteJSON(w, http.StatusOK, contract); errEscritura != nil {
		h.logger.ErrorContext(r.Context(), "goals: fallo al serializar la meta",
			slog.String("error_class", "response_write"),
			slog.String("trace_id", traceID(r)))
	}
}
