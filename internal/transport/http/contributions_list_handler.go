package http

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	appContrib "github.com/nexumpr/Nexum-Backend/internal/application/contributions"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// ContributionListHandler sirve E045 (`GET /api/v1/goal-contributions`).
//
// Contrato:
//   - `Authorization: Bearer` RS256 y estado autoritativo de la sesión en DynamoDB.
//   - Sin cuerpo de petición: el `userId` se deriva siempre del sujeto autenticado.
//   - Filtros `goalId`, `from`, `to`, `status` y paginación `limit`/`cursor`.
//   - El cursor es opaco, firmado con HMAC, ligado al usuario y a la firma exacta de filtros.
//   - Sin clave de firma de cursor el listado responde 503: nunca se pagina sin firmar.
type ContributionListHandler struct {
	service   *appContrib.Service
	logger    *slog.Logger
	protected http.Handler
}

// NewContributionListHandler construye E045. Sin servicio, sin proveedor de tokens o sin
// autorizador de sesiones el handler responde 503 (fail-closed) y nunca autoriza sólo con la
// firma del token.
func NewContributionListHandler(service *appContrib.Service, tokens ports.TokenProvider, sessions ports.SessionAuthorizer, limitador LimitadorRuta, logger *slog.Logger) *ContributionListHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &ContributionListHandler{service: service, logger: logger}
	protegido := CadenaAportaciones(service, tokens, sessions, h.serve)
	if protegido != nil {
		h.protected = LimitarPorUsuario(limitador, "contributions", protegido.ServeHTTP)
	}
	return h
}

// ServeHTTP fija la caché privada y delega en la cadena autenticada.
func (h *ContributionListHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if h == nil || h.service == nil || h.protected == nil {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}
	h.protected.ServeHTTP(w, r)
}

func (h *ContributionListHandler) serve(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}
	limit, err := ParseLimitQuery(r.URL.Query().Get("limit"))
	if err != nil {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{{Field: "limit", Reason: "invalid"}})
		return
	}
	query := appContrib.ListQuery{
		UserID: userID,
		Limit:  limit,
		Cursor: r.URL.Query().Get("cursor"),
		GoalID: r.URL.Query().Get("goalId"),
		Status: r.URL.Query().Get("status"),
	}
	if desde := strings.TrimSpace(r.URL.Query().Get("from")); desde != "" {
		t, errDesde := time.Parse(time.RFC3339, desde)
		if errDesde != nil {
			writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{{Field: "from", Reason: "invalid_format"}})
			return
		}
		query.From, query.HasFrom = t, true
	}
	if hasta := strings.TrimSpace(r.URL.Query().Get("to")); hasta != "" {
		t, errHasta := time.Parse(time.RFC3339, hasta)
		if errHasta != nil {
			writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{{Field: "to", Reason: "invalid_format"}})
			return
		}
		query.To, query.HasTo = t, true
	}
	pagina, err := h.service.List(r.Context(), query)
	if err != nil {
		WriteContributionsError(w, r, h.logger, err)
		return
	}
	items := make([]appContrib.WireContribution, 0, len(pagina.Items))
	for _, c := range pagina.Items {
		items = append(items, c.Wire())
	}
	respuesta := struct {
		Items      []appContrib.WireContribution `json:"items"`
		NextCursor string                        `json:"nextCursor,omitempty"`
	}{Items: items, NextCursor: pagina.NextCursor}
	if err := httputil.WriteJSON(w, http.StatusOK, respuesta); err != nil {
		h.logger.ErrorContext(r.Context(), "contributions: fallo al serializar el listado", slog.String("trace_id", traceID(r)))
	}
}
