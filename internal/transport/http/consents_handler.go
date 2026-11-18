package http

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	appPrivacy "github.com/nexumpr/Nexum-Backend/internal/application/privacy"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// ConsentsHandler expone `GET /api/v1/users/me/consents` (E016).
//
// La ruta está protegida por el middleware de autenticación JWT RS256 y por la verificación de
// sesión autoritativa: la identidad se deriva exclusivamente de `sub` y `sid` verificados. El
// cliente no puede elegir el usuario ni enumerar consentimientos ajenos.
//
// Contrato: 200 con `PageOfConsent`, cabecera `Cache-Control: private, no-store` y, en error,
// el sobre canónico `ErrorEnvelope`. Códigos contemplados: 200, 400, 401, 403, 404, 429, 500 y
// 503. Este recorte no incorpora limitador distribuido (429 no se emite): la cuota por cuenta
// requiere Redis y pertenece a una unidad con Gate propio; no se simula.
type ConsentsHandler struct {
	service   *appPrivacy.ConsentsService
	logger    *slog.Logger
	protected http.Handler
}

// NewConsentsHandler construye el handler. Si falta el servicio, el proveedor de tokens o el
// autorizador de sesiones, el endpoint responde 503 (fail-closed): nunca se lista sin identidad
// verificada ni sin persistencia.
func NewConsentsHandler(
	service *appPrivacy.ConsentsService,
	tokens ports.TokenProvider,
	sessions ports.SessionAuthorizer,
	logger *slog.Logger,
) *ConsentsHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &ConsentsHandler{service: service, logger: logger}
	if service != nil && tokens != nil && sessions != nil {
		h.protected = middleware.AuthenticateBearerWithSession(tokens, sessions)(http.HandlerFunc(h.serve))
	}
	return h
}

// ServeHTTP fija la cabecera de caché antes de cualquier decisión y delega en la cadena
// autenticada.
func (h *ConsentsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Información de identidad y de privacidad: nunca cacheable por proxies ni por el cliente.
	w.Header().Set("Cache-Control", "private, no-store")

	if h == nil || h.service == nil || h.protected == nil {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}

	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeRegisterError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "el metodo HTTP no esta permitido para este recurso", nil)
		return
	}

	h.protected.ServeHTTP(w, r)
}

// serve se ejecuta dentro de la cadena autenticada: `sub` y `sid` ya están verificados.
func (h *ConsentsHandler) serve(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		// Defensa en profundidad: el middleware ya exige `sub`.
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}

	limit, errLimite := parseConsentsLimit(r)
	if errLimite != nil {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "limit", Reason: "out_of_range"},
		})
		return
	}

	cursor := strings.TrimSpace(r.URL.Query().Get("cursor"))
	if cursor != "" && (len(cursor) < appPrivacy.ConsentsCursorMinLength || len(cursor) > appPrivacy.ConsentsCursorMaxLength) {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "cursor", Reason: "invalid_length"},
		})
		return
	}

	page, err := h.service.List(r.Context(), appPrivacy.ConsentsQuery{
		UserID: userID,
		Limit:  limit,
		Cursor: cursor,
	})
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}

	respuesta := pageOfConsentResponse{Items: make([]consentResponse, 0, len(page.Items))}
	for _, item := range page.Items {
		respuesta.Items = append(respuesta.Items, consentResponse{
			ID:            item.ID,
			NoticeVersion: item.NoticeVersion,
			Purpose:       item.Purpose,
			Granted:       item.Granted,
			RecordedAt:    item.RecordedAt.UTC().Format(time.RFC3339),
		})
	}
	respuesta.NextCursor = page.NextCursor

	// Se serializa explícitamente el DTO del contrato: ni el ítem de DynamoDB ni la fila de
	// aplicación llegan al cliente, de modo que `source`, `createdAt`, `acceptedAt` o cualquier
	// atributo nuevo no pueden filtrarse.
	if err := httputil.WriteJSON(w, http.StatusOK, respuesta); err != nil {
		h.logger.ErrorContext(r.Context(), "consents: fallo al serializar la respuesta",
			slog.String("error_class", "response_write"),
			slog.String("trace_id", traceID(r)))
	}
}

// consentResponse es el objeto `Consent` del contrato: exactamente cinco campos, sin extras y
// sin `omitempty` (un `granted:false` debe emitirse siempre).
type consentResponse struct {
	ID            string `json:"id"`
	NoticeVersion string `json:"noticeVersion"`
	Purpose       string `json:"purpose"`
	Granted       bool   `json:"granted"`
	RecordedAt    string `json:"recordedAt"`
}

// pageOfConsentResponse es `PageOfConsent`: `items` siempre presente y `nextCursor` opcional.
type pageOfConsentResponse struct {
	Items      []consentResponse `json:"items"`
	NextCursor string            `json:"nextCursor,omitempty"`
}

// parseConsentsLimit valida `limit` (entero 1..100, por defecto 25). Un valor no numérico o
// fuera de rango se rechaza con 400: no se recorta en silencio. `limit=0` es inválido porque el
// mínimo del contrato es 1; sólo la ausencia del parámetro aplica el valor por defecto.
func parseConsentsLimit(r *http.Request) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return appPrivacy.ConsentsDefaultLimit, nil
	}
	valor, err := strconv.Atoi(raw)
	if err != nil {
		return 0, appPrivacy.ErrConsentsInvalidRequest
	}
	if valor < 1 || valor > appPrivacy.ConsentsMaxLimit {
		return 0, appPrivacy.ErrConsentsInvalidRequest
	}
	return valor, nil
}

// writeServiceError traduce los errores de aplicación a códigos del contrato. Nunca se
// registra el `userId`, el cursor ni ningún dato del consentimiento.
func (h *ConsentsHandler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, appPrivacy.ErrConsentsInvalidRequest):
		// Cursor manipulado, vencido, de otro usuario o emitido con otros filtros.
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", nil)
	case errors.Is(err, appPrivacy.ErrConsentsUnauthorized):
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
	case errors.Is(err, appPrivacy.ErrConsentsUnavailable):
		h.logger.ErrorContext(r.Context(), "consents: dependencia no disponible",
			slog.String("error_class", "consents_unavailable"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
	default:
		h.logger.ErrorContext(r.Context(), "consents: fallo interno",
			slog.String("error_class", "internal_error"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
	}
}
