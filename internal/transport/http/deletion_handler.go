package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	appAccount "github.com/nexumpr/Nexum-Backend/internal/application/account"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// DeleteAccountMaxRequestBodyBytes acota el cuerpo de E014: el contrato sólo tiene dos
// campos cortos y la contraseña se limita además en la capa de aplicación antes de derivar
// Argon2id.
const DeleteAccountMaxRequestBodyBytes int64 = 16 * 1024

// DeleteAccountRequest es el DTO estricto de E014 (OpenAPI DeleteAccountRequest).
//
// `additionalProperties: false`: `BindJSONLimit` rechaza cualquier propiedad adicional, un
// JSON concatenado y cuerpos por encima del límite. No se admiten `userId`, `sessionId` ni
// tokens: la cuenta se deriva del claim `sub`.
type DeleteAccountRequest struct {
	Confirmation    string `json:"confirmation"`
	CurrentPassword string `json:"currentPassword"`
}

// DeletionReceiptResponse es el contrato de salida `DeletionReceipt`.
//
// Se emiten los cinco campos del esquema: los cuatro requeridos (`requestId`, `status`,
// `requestedAt`, `sessionsRevoked`) y `deletionRequestId` (aditivo, RFC-016 A8). No se
// serializa ningún token, sesión, hash ni marca interna.
type DeletionReceiptResponse struct {
	RequestID         string `json:"requestId"`
	Status            string `json:"status"`
	RequestedAt       string `json:"requestedAt"`
	SessionsRevoked   bool   `json:"sessionsRevoked"`
	DeletionRequestID string `json:"deletionRequestId"`
}

// DeleteAccountHandler expone `DELETE /api/v1/users/me` (E014).
//
// Contrato congelado: Bearer obligatorio con estado autoritativo de sesión, 202 con
// `DeletionReceipt` y los códigos 400/401/404/409/422/500/503. La respuesta nunca refleja
// el cuerpo recibido ni confirma datos de la cuenta más allá del recibo de la solicitud.
type DeleteAccountHandler struct {
	service   *appAccount.DeletionService
	logger    *slog.Logger
	protected http.Handler
}

// NewDeleteAccountHandler construye el handler.
//
// El middleware se compone con `AuthenticateBearerWithSession`: sin proveedor de tokens o
// sin autorizador de sesión la ruta responde 503 en lugar de autorizar sólo con la firma.
// Si falta el servicio, también responde 503 (fail-closed).
func NewDeleteAccountHandler(
	service *appAccount.DeletionService,
	tokens ports.TokenProvider,
	sessions ports.SessionAuthorizer,
	logger *slog.Logger,
) *DeleteAccountHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &DeleteAccountHandler{service: service, logger: logger}
	if service != nil && tokens != nil && sessions != nil {
		h.protected = middleware.AuthenticateBearerWithSession(tokens, sessions)(http.HandlerFunc(h.serve))
	}
	return h
}

// ServeHTTP aplica las guardas de composición y delega en la cadena autenticada.
func (h *DeleteAccountHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Datos de cuenta: ni caché compartida ni almacenamiento intermedio.
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Pragma", "no-cache")

	if h == nil || h.service == nil || !h.service.Disponible() || h.protected == nil {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}

	h.protected.ServeHTTP(w, r)
}

// serve se ejecuta dentro de la cadena autenticada: `sub` está verificado criptográficamente
// y la sesión es ACTIVE, no expirada y con la época vigente.
func (h *DeleteAccountHandler) serve(w http.ResponseWriter, r *http.Request) {
	userID, okUser := middleware.UserIDFromContext(r.Context())
	if !okUser || strings.TrimSpace(userID) == "" {
		// Defensa en profundidad: el middleware ya exige `sub`.
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}

	if !hasJSONContentType(r) {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "Content-Type", Reason: "application_json_required"},
		})
		return
	}

	var request DeleteAccountRequest
	if err := BindJSONLimit(w, r, &request, DeleteAccountMaxRequestBodyBytes); err != nil {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "body", Reason: "invalid_json"},
		})
		return
	}

	recibo, err := h.service.Request(r.Context(), appAccount.DeleteAccountCommand{
		UserID:          userID,
		Confirmation:    request.Confirmation,
		CurrentPassword: request.CurrentPassword,
	})
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}

	// 202 Accepted: el trabajo quedó persistido y el worker es durable. No se devuelve
	// ningún token ni identificador de sesión.
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(DeletionReceiptResponse{
		RequestID:         recibo.RequestID,
		Status:            recibo.Status,
		RequestedAt:       recibo.RequestedAt.UTC().Format(time.RFC3339),
		SessionsRevoked:   recibo.SessionsRevoked,
		DeletionRequestID: recibo.DeletionRequestID,
	})
}

// writeServiceError traduce los errores de aplicación a códigos del contrato sin revelar el
// motivo exacto. Nunca se registra la contraseña, el hash ni el `userId`.
func (h *DeleteAccountHandler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	var violation *appAccount.FieldViolation
	if errors.As(err, &violation) {
		// Confirmación distinta de DELETE_MY_ACCOUNT o contraseña en blanco: 422.
		writeRegisterError(w, r, http.StatusUnprocessableEntity, "BUSINESS_RULE_VIOLATION", "solicitud no completada", []httputil.ErrorDetail{
			{Field: violation.Field, Reason: violation.Reason},
		})
		return
	}

	switch {
	case errors.Is(err, appAccount.ErrDeletionPasswordIncorrect):
		// Mismo mensaje que cualquier otra violación de regla: no se distingue "contraseña
		// incorrecta" de "política incumplida" (criterio de E007).
		writeRegisterError(w, r, http.StatusUnprocessableEntity, "BUSINESS_RULE_VIOLATION", "solicitud no completada", nil)
	case errors.Is(err, appAccount.ErrDeletionUnauthorized):
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
	case errors.Is(err, appAccount.ErrDeletionProfileNotFound):
		writeRegisterError(w, r, http.StatusNotFound, "NOT_FOUND", "recurso no encontrado", nil)
	case errors.Is(err, appAccount.ErrDeletionConflict):
		h.logger.WarnContext(r.Context(), "eliminacion de cuenta: conflicto de concurrencia",
			slog.String("error_class", "deletion_conflict"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusConflict, "CONFLICT", "solicitud no completada", nil)
	case errors.Is(err, appAccount.ErrDeletionUnavailable):
		h.logger.ErrorContext(r.Context(), "eliminacion de cuenta: dependencia no disponible",
			slog.String("error_class", "deletion_unavailable"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
	default:
		h.logger.ErrorContext(r.Context(), "eliminacion de cuenta: fallo interno",
			slog.String("error_class", "internal_error"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
	}
}
