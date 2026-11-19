package http

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

const (
	// movementsMaxRequestBodyBytes acota el cuerpo de E028: seis campos cortos no necesitan
	// más de unos cientos de bytes.
	movementsMaxRequestBodyBytes int64 = 16 * 1024

	// movementsClaseCuota es la clase de la cuota distribuida por usuario. Se comparte entre
	// las rutas de movimientos: es una cuota de familia, no por endpoint.
	movementsClaseCuota = "movements"
)

// MovementsHandler expone `POST /api/v1/movements` (E028) y `GET /api/v1/movements` (E027).
//
// Ambas rutas van protegidas por JWT RS256 y por la sesión autoritativa en DynamoDB: la
// identidad se deriva exclusivamente de `sub` verificado, y `AuthorizeSession` exige además
// `PROFILE.status = ACTIVE` con la época vigente, de modo que una cuenta en proceso de borrado
// (E014) no puede crear ni listar movimientos. El cliente no puede elegir el titular por query,
// cuerpo ni cabecera.
//
// Contrato: 201 con `Movement` y `Location` en el alta; 200 con `PageOfMovement` en el listado;
// `Cache-Control: private, no-store` siempre (información financiera); en error, el sobre
// canónico `ErrorEnvelope`. Códigos: 200, 201, 400, 401, 403, 404, 409, 422, 429, 500 y 503.
type MovementsHandler struct {
	service *appMovements.Service
	logger  *slog.Logger
	create  http.Handler
	list    http.Handler
}

// NewMovementsHandler construye el handler. Sin servicio, sin proveedor de tokens o sin
// autorizador de sesiones, ambas rutas responden 503 (fail-closed): nunca se sirve ni se escribe
// un movimiento sin identidad verificada ni sin persistencia. El limitador es opcional: sin
// Redis se omite y no se emite 429, en lugar de simular una cuota.
func NewMovementsHandler(
	service *appMovements.Service,
	tokens ports.TokenProvider,
	sessions ports.SessionAuthorizer,
	limitador LimitadorRuta,
	logger *slog.Logger,
) *MovementsHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &MovementsHandler{service: service, logger: logger}
	if service == nil || tokens == nil || sessions == nil {
		return h
	}

	interior := func(serve http.HandlerFunc) http.Handler {
		if limitador == nil {
			return http.HandlerFunc(serve)
		}
		return http.HandlerFunc(LimitarPorUsuario(limitador, movementsClaseCuota, serve))
	}

	h.create = middleware.AuthenticateBearerWithSession(tokens, sessions)(interior(h.serveCreate))
	h.list = middleware.AuthenticateBearerWithSession(tokens, sessions)(interior(h.serveList))
	return h
}

// ServeHTTP fija la cabecera de caché antes de cualquier decisión y reparte por método.
func (h *MovementsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Información financiera del titular: nunca cacheable por proxies ni por el cliente.
	w.Header().Set("Cache-Control", "private, no-store")

	if h == nil || h.service == nil || h.create == nil || h.list == nil {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}

	switch r.Method {
	case http.MethodPost:
		h.create.ServeHTTP(w, r)
	case http.MethodGet:
		h.list.ServeHTTP(w, r)
	default:
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodPost)
		writeRegisterError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "el metodo HTTP no esta permitido para este recurso", nil)
	}
}

// movementCreateRequest es el DTO estricto de E028 (OpenAPI `MovementCreate`).
//
// `additionalProperties: false`: `BindJSONLimit` rechaza cualquier propiedad desconocida y un
// JSON concatenado. `amount` es `string` en el contrato: un número JSON produce 400 al no poder
// deserializarse como cadena, de modo que un `float64` nunca llega al dominio.
type movementCreateRequest struct {
	Type        string `json:"type"`
	Amount      string `json:"amount"`
	Currency    string `json:"currency"`
	CategoryID  string `json:"categoryId"`
	Description string `json:"description"`
	OccurredAt  string `json:"occurredAt"`
}

// serveCreate aplica `POST /api/v1/movements` (E028).
//
// Orden de decisiones: identidad verificada → `Idempotency-Key` obligatoria (400) → cuerpo
// estricto (400) → caso de uso (422/409/503). La respuesta es el cuerpo 201 canónico, que la
// idempotencia persiste tal cual: un replay devuelve exactamente los mismos bytes y añade
// `X-Idempotent-Replay: true`.
func (h *MovementsHandler) serveCreate(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		// Defensa en profundidad: el middleware ya exige `sub`.
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}

	clave, errClave := RequireIdempotencyKey(r)
	if errClave != nil {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: HeaderIdempotencyKey, Reason: "required_or_invalid_length"},
		})
		return
	}

	var peticion movementCreateRequest
	if err := BindJSONLimit(w, r, &peticion, movementsMaxRequestBodyBytes); err != nil {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "body", Reason: "invalid_json"},
		})
		return
	}

	resultado, err := h.service.Create(r.Context(), userID, appMovements.CreateCommand{
		Type:           peticion.Type,
		Amount:         peticion.Amount,
		Currency:       peticion.Currency,
		CategoryID:     peticion.CategoryID,
		Description:    peticion.Description,
		OccurredAt:     peticion.OccurredAt,
		IdempotencyKey: clave,
	})
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	if resultado == nil || len(resultado.Body) == 0 {
		h.writeServiceError(w, r, appMovements.ErrInternal)
		return
	}

	if location := strings.TrimSpace(resultado.Location); location != "" {
		w.Header().Set("Location", location)
	}
	if resultado.Replayed {
		// El cliente puede distinguir una reproducción de un alta nueva sin comparar cuerpos.
		w.Header().Set("X-Idempotent-Replay", "true")
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(resultado.Status)
	if _, errEscritura := w.Write(resultado.Body); errEscritura != nil {
		h.logger.ErrorContext(r.Context(), "movimientos: fallo al escribir la respuesta",
			slog.String("error_class", "response_write"),
			slog.String("trace_id", traceID(r)))
	}
}

// pageOfMovementResponse es `PageOfMovement`: `items` siempre presente y `nextCursor` sólo si
// hay más elementos.
type pageOfMovementResponse struct {
	Items      []appMovements.MovementResponse `json:"items"`
	NextCursor string                          `json:"nextCursor,omitempty"`
}

// serveList aplica `GET /api/v1/movements` (E027).
//
// Orden de decisiones: identidad verificada → parámetros (400) → caso de uso (503). El cursor es
// opaco y va firmado: si se manipula, si pertenece a otro usuario o si se reutiliza con otros
// filtros, el servicio devuelve 400 en lugar de mezclar páginas.
func (h *MovementsHandler) serveList(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}

	consulta, violacion := movementsListQuery(r, userID)
	if violacion != nil {
		// Un parámetro de consulta fuera de forma es un 400 del contrato, no una regla de
		// negocio del recurso: no se reutiliza el mapeo 422 del cuerpo.
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: violacion.Field, Reason: violacion.Reason},
		})
		return
	}

	pagina, err := h.service.List(r.Context(), consulta)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}

	respuesta := pageOfMovementResponse{Items: make([]appMovements.MovementResponse, 0, len(pagina.Items))}
	for _, movimiento := range pagina.Items {
		respuesta.Items = append(respuesta.Items, appMovements.NewMovementResponse(movimiento))
	}
	respuesta.NextCursor = pagina.NextCursor

	// Se serializa explícitamente el DTO del contrato: el ítem de DynamoDB nunca llega al
	// cliente, de modo que `payloadHash`, `GSI1PK`, `targetSK` o cualquier atributo nuevo no
	// pueden filtrarse.
	if err := httputil.WriteJSON(w, http.StatusOK, respuesta); err != nil {
		h.logger.ErrorContext(r.Context(), "movimientos: fallo al serializar la respuesta",
			slog.String("error_class", "response_write"),
			slog.String("trace_id", traceID(r)))
	}
}

// movementsListQuery valida la forma de los parámetros de E027.
//
// `limit` debe ser un entero 1..100 (por defecto 25) y no se recorta en silencio; `cursor` debe
// respetar 8..2048 caracteres; `type` debe pertenecer al enum publicado; `from`/`to` deben ser
// `date-time` RFC 3339. Los rangos y la coherencia `from <= to` los revalida el servicio.
func movementsListQuery(r *http.Request, userID string) (appMovements.ListQuery, *appMovements.FieldViolation) {
	consulta := appMovements.ListQuery{UserID: userID}
	valores := r.URL.Query()

	if bruto := strings.TrimSpace(valores.Get("limit")); bruto != "" {
		limite, err := strconv.Atoi(bruto)
		if err != nil || limite < 1 || limite > appMovements.MaxLimit {
			return consulta, &appMovements.FieldViolation{Field: "limit", Reason: "out_of_range"}
		}
		consulta.Limit = limite
	}

	if bruto := strings.TrimSpace(valores.Get("cursor")); bruto != "" {
		if len(bruto) < appMovements.CursorMinLength || len(bruto) > appMovements.CursorMaxLength {
			return consulta, &appMovements.FieldViolation{Field: "cursor", Reason: "invalid_length"}
		}
		consulta.Cursor = bruto
	}

	if bruto := strings.TrimSpace(valores.Get("type")); bruto != "" {
		tipo, err := appMovements.NormalizeType(bruto)
		if err != nil {
			return consulta, &appMovements.FieldViolation{Field: "type", Reason: "unsupported"}
		}
		consulta.Type = tipo
	}

	if bruto := strings.TrimSpace(valores.Get("from")); bruto != "" {
		instante, err := time.Parse(time.RFC3339Nano, bruto)
		if err != nil {
			return consulta, &appMovements.FieldViolation{Field: "from", Reason: "invalid_date_time"}
		}
		consulta.From = instante.UTC()
		consulta.HasFrom = true
	}

	if bruto := strings.TrimSpace(valores.Get("to")); bruto != "" {
		instante, err := time.Parse(time.RFC3339Nano, bruto)
		if err != nil {
			return consulta, &appMovements.FieldViolation{Field: "to", Reason: "invalid_date_time"}
		}
		consulta.To = instante.UTC()
		consulta.HasTo = true
	}

	return consulta, nil
}

// writeServiceError delega en el mapeo compartido de la familia de movimientos para que el
// listado, el alta y las rutas por identificador traduzcan los errores de forma idéntica.
func (h *MovementsHandler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	writeMovementError(w, r, h.logger, err)
}
