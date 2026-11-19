package http

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	appGoals "github.com/nexumpr/Nexum-Backend/internal/application/goals"
	domainGoal "github.com/nexumpr/Nexum-Backend/internal/domain/goal"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// Transporte del listado de metas: E038 (`GET /api/v1/goals`).
//
// Reglas:
//   - `Authorization: Bearer` RS256 y estado autoritativo de la sesión en DynamoDB.
//   - Información financiera: `Cache-Control: private, no-store` en toda respuesta de éxito.
//   - `items` es la página solicitada; `totals` describe TODA la colección (A8 de RFC-016).
//   - `nextCursor` sólo se emite cuando la persistencia confirmó que hay más filas.

// GoalsListHandler sirve `GET /api/v1/goals` (E038).
type GoalsListHandler struct {
	service   *appGoals.Service
	logger    *slog.Logger
	protected http.Handler
}

// NewGoalsListHandler construye el handler. Sin servicio, sin proveedor de tokens, sin
// autorizador de sesión o sin clave de firma de cursor responde 503 (fail-closed).
func NewGoalsListHandler(
	service *appGoals.Service,
	tokens ports.TokenProvider,
	sessions ports.SessionAuthorizer,
	logger *slog.Logger,
) *GoalsListHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &GoalsListHandler{service: service, logger: logger}
	h.protected = cadenaMetas(service, tokens, sessions, h.serve)
	return h
}

// ServeHTTP fija la cabecera de caché y delega en la cadena autenticada.
func (h *GoalsListHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")

	if h == nil || h.service == nil || h.protected == nil {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}
	h.protected.ServeHTTP(w, r)
}

// serve se ejecuta con la sesión ya verificada.
func (h *GoalsListHandler) serve(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}

	limit, errLimite := parseGoalsLimit(r)
	if errLimite != nil {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "limit", Reason: "out_of_range"},
		})
		return
	}
	cursor := strings.TrimSpace(r.URL.Query().Get("cursor"))
	if cursor != "" && (len(cursor) < appGoals.ListCursorMinLength || len(cursor) > appGoals.ListCursorMaxLength) {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "cursor", Reason: "invalid_length"},
		})
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status != "" {
		if _, errEstado := domainGoal.ParseStatus(status); errEstado != nil {
			writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
				{Field: "status", Reason: "out_of_enum"},
			})
			return
		}
	}

	pagina, err := h.service.List(r.Context(), appGoals.ListQuery{
		UserID: userID,
		Status: status,
		Limit:  limit,
		Cursor: cursor,
	})
	if err != nil {
		writeGoalsError(w, r, h.logger, err)
		return
	}
	if errEscritura := httputil.WriteJSON(w, http.StatusOK, pagina); errEscritura != nil {
		h.logger.ErrorContext(r.Context(), "goals: fallo al serializar el listado",
			slog.String("error_class", "response_write"),
			slog.String("trace_id", traceID(r)))
	}
}

// parseGoalsLimit valida `limit` (entero 1..100, por defecto 25). Un valor no numérico o fuera
// de rango se rechaza con 400: no se recorta en silencio. `limit=0` es inválido porque el mínimo
// del contrato es 1; sólo la ausencia del parámetro aplica el valor por defecto.
func parseGoalsLimit(r *http.Request) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return appGoals.DefaultListLimit, nil
	}
	valor, err := strconv.Atoi(raw)
	if err != nil {
		return 0, appGoals.ErrInvalidRequest
	}
	if valor < 1 || valor > appGoals.MaxListLimit {
		return 0, appGoals.ErrInvalidRequest
	}
	return valor, nil
}
