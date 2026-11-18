package http

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// SessionsHandler expone `GET /api/v1/auth/sessions` (E008).
//
// La ruta está protegida por el middleware de autenticación JWT RS256 y por la verificación de
// sesión autoritativa: la identidad se deriva exclusivamente de `sub` y `sid` verificados. El
// cliente no puede elegir el usuario ni enumerar sesiones ajenas.
//
// Contrato: 200 con `PageOfSession`, cabecera `Cache-Control: private, no-store` y, en error,
// el sobre canónico `ErrorEnvelope`. Códigos contemplados: 200, 400, 401, 403, 404, 429, 500 y
// 503. Este recorte no incorpora limitador distribuido (429 no se emite): la cuota por cuenta
// requiere Redis y pertenece a una unidad con Gate propio; no se simula.
type SessionsHandler struct {
	service   *appIdentity.SessionsService
	logger    *slog.Logger
	protected http.Handler
}

// NewSessionsHandler construye el handler. Si falta el servicio, el proveedor de tokens o el
// autorizador de sesiones, el endpoint responde 503 (fail-closed): nunca se lista sin identidad
// verificada ni sin persistencia.
func NewSessionsHandler(
	service *appIdentity.SessionsService,
	tokens ports.TokenProvider,
	sessions ports.SessionAuthorizer,
	logger *slog.Logger,
) *SessionsHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &SessionsHandler{service: service, logger: logger}
	if service != nil && tokens != nil && sessions != nil {
		h.protected = middleware.AuthenticateBearerWithSession(tokens, sessions)(http.HandlerFunc(h.serve))
	}
	return h
}

// ServeHTTP fija la cabecera de caché antes de cualquier decisión y delega en la cadena
// autenticada.
func (h *SessionsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Información de identidad: nunca cacheable por proxies ni por el cliente.
	w.Header().Set("Cache-Control", "private, no-store")

	if h == nil || h.service == nil || h.protected == nil {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}

	h.protected.ServeHTTP(w, r)
}

// serve se ejecuta dentro de la cadena autenticada: `sub` y `sid` ya están verificados.
func (h *SessionsHandler) serve(w http.ResponseWriter, r *http.Request) {
	userID, okUser := middleware.UserIDFromContext(r.Context())
	sessionID, okSession := middleware.SessionIDFromContext(r.Context())
	if !okUser || !okSession || userID == "" || sessionID == "" {
		// Defensa en profundidad: el middleware ya exige ambos claims.
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}

	limit, errLimite := parseSessionsLimit(r)
	if errLimite != nil {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "limit", Reason: "out_of_range"},
		})
		return
	}

	cursor := strings.TrimSpace(r.URL.Query().Get("cursor"))
	if cursor != "" && (len(cursor) < appIdentity.SessionsCursorMinLength || len(cursor) > appIdentity.SessionsCursorMaxLength) {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "cursor", Reason: "invalid_length"},
		})
		return
	}

	page, err := h.service.ListSessions(r.Context(), appIdentity.SessionsQuery{
		UserID: userID,
		Limit:  limit,
		Cursor: cursor,
	})
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}

	respuesta := pageOfSessionResponse{Items: make([]sessionResponse, 0, len(page.Items))}
	for _, item := range page.Items {
		respuesta.Items = append(respuesta.Items, sessionResponse{
			ID:         item.SessionID,
			CreatedAt:  formatSessionTime(item.CreatedAt),
			LastSeenAt: formatSessionTime(item.LastSeenAt),
			Current:    item.SessionID == sessionID,
			Revoked:    item.Revoked,
		})
	}
	respuesta.NextCursor = page.NextCursor

	// Se serializa explícitamente el DTO del contrato: ni el ítem de DynamoDB ni la fila de
	// aplicación llegan al cliente, de modo que `refreshHash`, `accessJti`, `familyId`,
	// `deviceId`, `deviceName`, `platform` o cualquier atributo nuevo no pueden filtrarse.
	if err := httputil.WriteJSON(w, http.StatusOK, respuesta); err != nil {
		h.logger.ErrorContext(r.Context(), "sessions: fallo al serializar la respuesta",
			slog.String("error_class", "response_write"),
			slog.String("trace_id", traceID(r)))
	}
}

// sessionResponse es el objeto `Session` del contrato: exactamente cinco campos, sin extras.
type sessionResponse struct {
	ID         string `json:"id"`
	CreatedAt  string `json:"createdAt"`
	LastSeenAt string `json:"lastSeenAt"`
	Current    bool   `json:"current"`
	Revoked    bool   `json:"revoked"`
}

// pageOfSessionResponse es `PageOfSession`: `items` siempre presente y `nextCursor` opcional.
type pageOfSessionResponse struct {
	Items      []sessionResponse `json:"items"`
	NextCursor string            `json:"nextCursor,omitempty"`
}

// parseSessionsLimit valida `limit` (entero 1..100, por defecto 25). Un valor no numérico o
// fuera de rango se rechaza con 400: no se recorta en silencio. `limit=0` es inválido porque el
// mínimo del contrato es 1; sólo la ausencia del parámetro aplica el valor por defecto.
func parseSessionsLimit(r *http.Request) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return appIdentity.SessionsDefaultLimit, nil
	}
	valor, err := strconv.Atoi(raw)
	if err != nil {
		return 0, appIdentity.ErrSessionsInvalidRequest
	}
	if valor < 1 || valor > appIdentity.SessionsMaxLimit {
		return 0, appIdentity.ErrSessionsInvalidRequest
	}
	return valor, nil
}

// formatSessionTime serializa en RFC3339 UTC, el formato `date-time` del contrato.
func formatSessionTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339)
}

// writeServiceError traduce los errores de aplicación a códigos del contrato. Nunca se
// registra el `userId`, el `sessionId`, el cursor ni ningún dato de la sesión.
func (h *SessionsHandler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, appIdentity.ErrSessionsInvalidRequest):
		// Cursor manipulado, vencido, de otro usuario o emitido con otros filtros.
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", nil)
	case errors.Is(err, appIdentity.ErrSessionsUnauthorized):
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
	case errors.Is(err, appIdentity.ErrSessionsUnavailable):
		h.logger.ErrorContext(r.Context(), "sessions: dependencia no disponible",
			slog.String("error_class", "sessions_unavailable"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
	default:
		h.logger.ErrorContext(r.Context(), "sessions: fallo interno",
			slog.String("error_class", "internal_error"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
	}
}
