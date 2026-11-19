package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// MovementHandler expone las rutas de un movimiento concreto:
//
//	GET    /api/v1/movements/{movementId}  (E029)
//	PATCH  /api/v1/movements/{movementId}  (E030)
//	DELETE /api/v1/movements/{movementId}  (E031)
//
// El router registra cada método por separado, de modo que montar E029 no habilita escritura. El
// identificador llega por `PathValue` (nunca por query ni cabecera) y el titular se deriva
// exclusivamente del claim `sub` verificado: un identificador de otro usuario vive en otra
// partición y produce el mismo 404 que uno inexistente.
//
// Contrato: 200 con `Movement` y `ETag` en GET/PATCH, 204 sin cuerpo en DELETE, y el sobre
// canónico `ErrorEnvelope` en error. Códigos: 200, 204, 400, 401, 403, 404, 409, 412, 422, 428,
// 429, 500 y 503.
type MovementHandler struct {
	service   *appMovements.Service
	logger    *slog.Logger
	protected http.Handler
}

// NewMovementHandler construye el handler. Sin servicio, sin proveedor de tokens o sin autorizador
// de sesiones, las tres rutas responden 503 (fail-closed). El limitador es opcional: sin Redis se
// omite y no se emite 429, en lugar de simular una cuota.
func NewMovementHandler(
	service *appMovements.Service,
	tokens ports.TokenProvider,
	sessions ports.SessionAuthorizer,
	limitador LimitadorRuta,
	logger *slog.Logger,
) *MovementHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &MovementHandler{service: service, logger: logger}
	if service == nil || tokens == nil || sessions == nil {
		return h
	}

	interior := http.HandlerFunc(h.dispatch)
	if limitador != nil {
		interior = LimitarPorUsuario(limitador, movementsClaseCuota, h.dispatch)
	}
	h.protected = middleware.AuthenticateBearerWithSession(tokens, sessions)(interior)
	return h
}

// ServeHTTP fija la cabecera de caché antes de cualquier decisión y delega en la cadena protegida.
func (h *MovementHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Información financiera del titular: nunca cacheable por proxies ni por el cliente.
	w.Header().Set("Cache-Control", "private, no-store")

	if h == nil || h.service == nil || h.protected == nil {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}

	switch r.Method {
	case http.MethodGet, http.MethodPatch, http.MethodDelete:
		h.protected.ServeHTTP(w, r)
	default:
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodPatch+", "+http.MethodDelete)
		writeRegisterError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "el metodo HTTP no esta permitido para este recurso", nil)
	}
}

// dispatch reparte el método ya autenticado.
func (h *MovementHandler) dispatch(w http.ResponseWriter, r *http.Request) {
	identificador := strings.TrimSpace(r.PathValue("movementId"))
	switch r.Method {
	case http.MethodGet:
		h.serveGet(w, r, identificador)
	case http.MethodPatch:
		h.servePatch(w, r, identificador)
	default:
		h.serveDelete(w, r, identificador)
	}
}

// serveGet aplica `GET /api/v1/movements/{movementId}` (E029).
func (h *MovementHandler) serveGet(w http.ResponseWriter, r *http.Request, movementID string) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}

	movimiento, err := h.service.Get(r.Context(), userID, movementID)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	// ETag fuerte de la versión vigente: es el validador que E030 exigirá en `If-Match`.
	w.Header().Set("ETag", FormatETag(movimiento.Version))
	h.writeMovement(w, r, http.StatusOK, movimiento)
}

// movementPatchValue distingue tres situaciones que el contrato separa: campo ausente (no-op),
// cadena válida y `null` (rechazado: los tres campos de `MovementPatch` son `type: string`).
type movementPatchValue struct {
	Presente bool
	Valor    string
}

// UnmarshalJSON implementa la distinción; un `null` o un valor no textual producen 400.
func (v *movementPatchValue) UnmarshalJSON(datos []byte) error {
	if string(bytes.TrimSpace(datos)) == "null" {
		return errors.New("campo no admite null")
	}
	var valor string
	if err := json.Unmarshal(datos, &valor); err != nil {
		return err
	}
	v.Presente = true
	v.Valor = valor
	return nil
}

// movementPatchRequest es el DTO estricto de E030 (OpenAPI `MovementPatch`).
//
// `additionalProperties: false`: `BindJSONLimit` rechaza cualquier propiedad desconocida, de modo
// que `type`, `amount`, `currency`, `contributionId`, `recurringPaymentId` o `version` en el cuerpo
// producen 400 y nunca pueden alterar un importe ni un vínculo.
type movementPatchRequest struct {
	CategoryID  movementPatchValue `json:"categoryId"`
	Description movementPatchValue `json:"description"`
	OccurredAt  movementPatchValue `json:"occurredAt"`
}

// servePatch aplica `PATCH /api/v1/movements/{movementId}` (E030).
//
// Orden de decisiones: identidad → `If-Match` obligatorio (428 si falta, 412 si es débil, 400 si
// está mal formado) → cuerpo estricto (400) → caso de uso (404/412/409/422/503). La respuesta es
// `Movement` con el `ETag` de la versión nueva, de modo que el cliente puede encadenar otra
// modificación sin releer.
func (h *MovementHandler) servePatch(w http.ResponseWriter, r *http.Request, movementID string) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}

	version, errIfMatch := ParseIfMatchVersion(r.Header.Get("If-Match"))
	if errIfMatch != nil {
		if errors.Is(errIfMatch, ErrIfMatchRequerido) {
			writeRegisterError(w, r, http.StatusPreconditionRequired, "PRECONDITION_REQUIRED", "solicitud no completada", nil)
			return
		}
		if errors.Is(errIfMatch, ErrIfMatchDebil) {
			// RFC 9110: un validador débil nunca satisface If-Match.
			writeRegisterError(w, r, http.StatusPreconditionFailed, "VERSION_CONFLICT", "solicitud no completada", nil)
			return
		}
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "If-Match", Reason: "invalid_etag"},
		})
		return
	}

	var peticion *movementPatchRequest
	if err := BindJSONLimit(w, r, &peticion, movementsMaxRequestBodyBytes); err != nil || peticion == nil {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "body", Reason: "invalid_json"},
		})
		return
	}

	var parche appMovements.Patch
	if peticion.CategoryID.Presente {
		valor := peticion.CategoryID.Valor
		parche.CategoryID = &valor
	}
	if peticion.Description.Presente {
		valor := peticion.Description.Valor
		parche.Description = &valor
	}
	if peticion.OccurredAt.Presente {
		valor := peticion.OccurredAt.Valor
		parche.OccurredAt = &valor
	}

	movimiento, err := h.service.Patch(r.Context(), userID, movementID, version, parche)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	w.Header().Set("ETag", FormatETag(movimiento.Version))
	h.writeMovement(w, r, http.StatusOK, movimiento)
}

// serveDelete aplica `DELETE /api/v1/movements/{movementId}` (E031). Responde 204 sin cuerpo: no
// se emite `Content-Type` ni ningún documento.
func (h *MovementHandler) serveDelete(w http.ResponseWriter, r *http.Request, movementID string) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}

	if err := h.service.Delete(r.Context(), userID, movementID); err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeMovement serializa el cuerpo canónico de un movimiento. Usa la MISMA proyección que el 201
// de E028, de modo que el cliente ve exactamente el mismo esquema en todos los códigos de éxito.
func (h *MovementHandler) writeMovement(w http.ResponseWriter, r *http.Request, status int, movimiento *appMovements.Movement) {
	if movimiento == nil {
		h.writeServiceError(w, r, appMovements.ErrInternal)
		return
	}
	cuerpo, err := appMovements.MarshalMovement(*movimiento)
	if err != nil {
		h.writeServiceError(w, r, appMovements.ErrInternal)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if _, errEscritura := w.Write(cuerpo); errEscritura != nil {
		h.logger.ErrorContext(r.Context(), "movimiento: fallo al escribir la respuesta",
			slog.String("error_class", "response_write"),
			slog.String("trace_id", traceID(r)))
	}
}

// writeServiceError delega en el mapeo compartido de la familia de movimientos.
func (h *MovementHandler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	writeMovementError(w, r, h.logger, err)
}

// writeMovementError traduce los errores de aplicación a códigos del contrato. Nunca se registra
// el `userId`, el `movementId`, el importe, la descripción ni el cursor.
func writeMovementError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	if logger == nil {
		logger = slog.Default()
	}
	var violacion *appMovements.FieldViolation
	switch {
	case errors.As(err, &violacion):
		writeRegisterError(w, r, http.StatusUnprocessableEntity, "BUSINESS_RULE_VIOLATION", "solicitud no completada", []httputil.ErrorDetail{
			{Field: violacion.Field, Reason: violacion.Reason},
		})
	case errors.Is(err, appMovements.ErrInvalidRequest):
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", nil)
	case errors.Is(err, appMovements.ErrUnauthorized):
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
	case errors.Is(err, appMovements.ErrNotFound):
		writeRegisterError(w, r, http.StatusNotFound, "NOT_FOUND", "solicitud no completada", nil)
	case errors.Is(err, appMovements.ErrVersionConflict):
		writeRegisterError(w, r, http.StatusPreconditionFailed, "VERSION_CONFLICT", "solicitud no completada", nil)
	case errors.Is(err, appMovements.ErrContributionLinked):
		writeRegisterError(w, r, http.StatusConflict, "CONFLICT", "solicitud no completada", nil)
	case errors.Is(err, appMovements.ErrConflict):
		writeRegisterError(w, r, http.StatusConflict, "CONFLICT", "solicitud no completada", nil)
	case errors.Is(err, appMovements.ErrUnavailable):
		logger.ErrorContext(r.Context(), "movimientos: dependencia no disponible",
			slog.String("error_class", "movements_unavailable"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
	default:
		logger.ErrorContext(r.Context(), "movimientos: fallo interno",
			slog.String("error_class", "internal_error"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
	}
}
