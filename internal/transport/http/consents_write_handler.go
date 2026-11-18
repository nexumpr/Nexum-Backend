package http

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	appPrivacy "github.com/nexumpr/Nexum-Backend/internal/application/privacy"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// ConsentsWriteHandler expone `POST /api/v1/users/me/consents` (E017).
//
// La ruta está protegida por el middleware de autenticación JWT RS256 y por la verificación de
// sesión autoritativa: la identidad se deriva exclusivamente de `sub` y `sid` verificados. El
// cliente no puede registrar un consentimiento a nombre de otra cuenta.
//
// Contrato: 201 con el objeto `Consent` (cinco campos), cabecera `Cache-Control: private,
// no-store` y, en error, el sobre canónico `ErrorEnvelope`. Códigos contemplados: 201, 400,
// 401, 403, 404, 409, 422, 429, 500 y 503. Este recorte no incorpora limitador distribuido
// (429 no se emite): la cuota por cuenta requiere Redis y pertenece a una unidad con Gate
// propio; no se simula.
type ConsentsWriteHandler struct {
	service   *appPrivacy.ConsentsWriteService
	logger    *slog.Logger
	protected http.Handler
}

// NewConsentsWriteHandler construye el handler. Si falta el servicio, el proveedor de tokens o
// el autorizador de sesiones, el endpoint responde 503 (fail-closed): nunca se registra una
// decisión sin identidad verificada ni sin persistencia.
func NewConsentsWriteHandler(
	service *appPrivacy.ConsentsWriteService,
	tokens ports.TokenProvider,
	sessions ports.SessionAuthorizer,
	logger *slog.Logger,
) *ConsentsWriteHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &ConsentsWriteHandler{service: service, logger: logger}
	if service != nil && tokens != nil && sessions != nil {
		h.protected = middleware.AuthenticateBearerWithSession(tokens, sessions)(http.HandlerFunc(h.serve))
	}
	return h
}

// ServeHTTP fija la cabecera de caché antes de cualquier decisión y delega en la cadena
// autenticada.
func (h *ConsentsWriteHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Información de identidad y de privacidad: nunca cacheable por proxies ni por el cliente.
	w.Header().Set("Cache-Control", "private, no-store")

	if h == nil || h.service == nil || h.protected == nil {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}

	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeRegisterError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "el metodo HTTP no esta permitido para este recurso", nil)
		return
	}

	h.protected.ServeHTTP(w, r)
}

// serve se ejecuta dentro de la cadena autenticada: `sub` y `sid` ya están verificados.
func (h *ConsentsWriteHandler) serve(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		// Defensa en profundidad: el middleware ya exige `sub`.
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}

	var peticion consentRequest
	if err := BindJSON(w, r, &peticion); err != nil {
		// JSON malformado, campo desconocido, tipo incorrecto, cuerpo vacío o exceso de tamaño.
		// El motivo exacto no se refleja al cliente ni se registra.
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", nil)
		return
	}

	consent, err := h.service.Record(r.Context(), appPrivacy.ConsentCommand{
		UserID:        userID,
		NoticeVersion: peticion.NoticeVersion,
		Purpose:       peticion.Purpose,
		Granted:       peticion.Granted,
	})
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}

	// Se serializa explícitamente el DTO del contrato (los mismos cinco campos que E016): ni el
	// ítem de DynamoDB ni la decisión de aplicación llegan al cliente, de modo que `source`,
	// `createdAt`, `PK` o `SK` no pueden filtrarse. No se emite `Location`: la API no expone una
	// ruta de lectura de un consentimiento individual.
	respuesta := consentResponse{
		ID:            consent.ID,
		NoticeVersion: consent.NoticeVersion,
		Purpose:       consent.Purpose,
		Granted:       consent.Granted,
		RecordedAt:    consent.RecordedAt.UTC().Format(time.RFC3339),
	}
	if err := httputil.WriteJSON(w, http.StatusCreated, respuesta); err != nil {
		h.logger.ErrorContext(r.Context(), "consents: fallo al serializar la respuesta",
			slog.String("error_class", "response_write"),
			slog.String("trace_id", traceID(r)))
	}
}

// consentRequest es el esquema `ConsentRequest` del contrato: tres campos, sin extras.
// `granted` es un puntero para distinguir una denegación explícita (`false`) de la ausencia
// del campo, que es 422.
type consentRequest struct {
	NoticeVersion string `json:"noticeVersion"`
	Purpose       string `json:"purpose"`
	Granted       *bool  `json:"granted"`
}

// writeServiceError traduce los errores de aplicación a códigos del contrato. Nunca se
// registra el `userId`, la versión del aviso ni el propósito: sólo la clase de error.
func (h *ConsentsWriteHandler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, appPrivacy.ErrConsentDecisionInvalid):
		// `noticeVersion` vacía o fuera de rango, `purpose` fuera del enum o `granted` ausente.
		writeRegisterError(w, r, http.StatusUnprocessableEntity, "BUSINESS_RULE_VIOLATION", "solicitud no completada", nil)
	case errors.Is(err, appPrivacy.ErrConsentUnauthorized):
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
	case errors.Is(err, appPrivacy.ErrConsentConflict):
		writeRegisterError(w, r, http.StatusConflict, "CONFLICT", "solicitud no completada", nil)
	case errors.Is(err, appPrivacy.ErrConsentUnavailable):
		h.logger.ErrorContext(r.Context(), "consents: dependencia no disponible",
			slog.String("error_class", "consents_write_unavailable"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
	default:
		h.logger.ErrorContext(r.Context(), "consents: fallo interno",
			slog.String("error_class", "internal_error"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
	}
}
