package http

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	appExport "github.com/nexumpr/Nexum-Backend/internal/application/export"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// Transporte de E020 (`GET /api/v1/users/me/export`).
//
// Reglas:
//   - `Authorization: Bearer` RS256 y estado autoritativo de la sesión en DynamoDB.
//   - Información de identidad y financiera: `Cache-Control: private, no-store` en toda
//     respuesta, incluidos los errores.
//   - `limit` 1..100 (defecto 25) acota la PÁGINA, no la exportación.
//   - Cualquier error se escribe como envelope canónico y NUNCA incluye `nextCursor`: un fallo
//     recuperable (503) no puede confundirse con el final de la exportación.
//   - Ningún log incluye correo, importes, tokens, cursores ni claves.

// ExportHandler sirve `GET /api/v1/users/me/export` (E020).
type ExportHandler struct {
	service   *appExport.Service
	logger    *slog.Logger
	protected http.Handler
}

// NewExportHandler construye el handler. Sin servicio, sin proveedor de tokens o sin autorizador
// de sesión responde 503 (fail-closed): nunca se sirve una exportación sin verificar la sesión.
func NewExportHandler(
	service *appExport.Service,
	tokens ports.TokenProvider,
	sessions ports.SessionAuthorizer,
	logger *slog.Logger,
) *ExportHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &ExportHandler{service: service, logger: logger}
	if service != nil && tokens != nil && sessions != nil {
		h.protected = middleware.AuthenticateBearerWithSession(tokens, sessions)(http.HandlerFunc(h.serve))
	}
	return h
}

// ServeHTTP fija la cabecera de caché y delega en la cadena autenticada.
func (h *ExportHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")

	if h == nil || h.service == nil || h.protected == nil {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}
	h.protected.ServeHTTP(w, r)
}

// serve se ejecuta con la sesión ya verificada.
func (h *ExportHandler) serve(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}

	limit, errLimite := parseExportLimit(r)
	if errLimite != nil {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "limit", Reason: "out_of_range"},
		})
		return
	}
	cursor := strings.TrimSpace(r.URL.Query().Get("cursor"))
	if cursor != "" && (len(cursor) < appExport.CursorMinLength || len(cursor) > appExport.CursorMaxLength) {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "cursor", Reason: "invalid_length"},
		})
		return
	}

	pagina, err := h.service.Export(r.Context(), appExport.Query{
		UserID: userID,
		Limit:  limit,
		Cursor: cursor,
	})
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	if errEscritura := httputil.WriteJSON(w, http.StatusOK, pagina); errEscritura != nil {
		h.logger.ErrorContext(r.Context(), "exportacion: fallo al serializar la respuesta",
			slog.String("error_class", "response_write"),
			slog.String("trace_id", traceID(r)))
	}
}

// parseExportLimit valida `limit` (entero 1..100, por defecto 25). Un valor no numérico o fuera
// de rango se rechaza con 400: no se recorta en silencio. `limit=0` es inválido porque el mínimo
// del contrato es 1; sólo la ausencia del parámetro aplica el valor por defecto.
func parseExportLimit(r *http.Request) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return appExport.DefaultLimit, nil
	}
	valor, err := strconv.Atoi(raw)
	if err != nil {
		return 0, appExport.ErrInvalidRequest
	}
	if valor < 1 || valor > appExport.MaxLimit {
		return 0, appExport.ErrInvalidRequest
	}
	return valor, nil
}

// writeServiceError traduce los errores de aplicación a códigos del contrato. El cuerpo de
// error no contiene `nextCursor`, de modo que un 503 nunca se interpreta como fin de la
// exportación.
func (h *ExportHandler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, appExport.ErrInvalidRequest):
		// Cursor manipulado, vencido, de otro usuario o de otra colección.
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", nil)
	case errors.Is(err, appExport.ErrUnauthorized):
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
	case errors.Is(err, appExport.ErrNotFound):
		writeRegisterError(w, r, http.StatusNotFound, "NOT_FOUND", "recurso no encontrado", nil)
	case errors.Is(err, appExport.ErrInconsistent):
		h.logger.ErrorContext(r.Context(), "exportacion: item persistido incoherente",
			slog.String("error_class", "export_inconsistent"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
	case errors.Is(err, appExport.ErrUnavailable):
		h.logger.ErrorContext(r.Context(), "exportacion: dependencia no disponible",
			slog.String("error_class", "export_unavailable"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
	default:
		h.logger.ErrorContext(r.Context(), "exportacion: fallo interno",
			slog.String("error_class", "internal_error"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
	}
}
