package http

import (
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	appRecurring "github.com/nexumpr/Nexum-Backend/internal/application/recurring"
	domainRecurring "github.com/nexumpr/Nexum-Backend/internal/domain/recurring"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// Transporte de pagos recurrentes: E033 (`POST /api/v1/recurring-payments`) y E034
// (`GET /api/v1/recurring-payments/{recurringPaymentId}`).
//
// La ruta exige `Authorization: Bearer` con JWT RS256 y estado autoritativo de la sesión en
// DynamoDB: la identidad sale exclusivamente de `sub`/`sid` verificados y el cliente no puede
// crear una regla en la partición de otra cuenta.
//
// El cuerpo de éxito es el objeto `RecurringPayment` del contrato, cerrado
// (`additionalProperties: false`) con EXACTAMENTE once campos. El DTO se construye campo a
// campo en la capa de aplicación, de modo que `entityType`, `userId`, `createdAt` o cualquier
// atributo interno no tienen ningún camino de serialización.

// patronImporteRecurrente es el patrón publicado por `RecurringCreate.amount` y
// `RecurringPayment.amount`: parte entera sin ceros a la izquierda de hasta doce dígitos y
// exactamente dos decimales.
var patronImporteRecurrente = regexp.MustCompile(`^(0|[1-9][0-9]{0,11})\.[0-9]{2}$`)

// recurringMaxRequestBodyBytes acota el cuerpo de E033: once campos de texto corto no
// necesitan más. Se aplica con `MaxBytesReader` dentro de `DecodeJSON`.
const recurringMaxRequestBodyBytes int64 = 16 * 1024

// recurringCreateRequest es el DTO estricto de `RecurringCreate`.
//
// `additionalProperties: false`: `BindJSONLimit` rechaza cualquier propiedad desconocida,
// incluida `frequency` en el caso de E035 (que no la admite) o `id`/`version`, que sólo fija
// el servidor. El importe se declara como cadena: un número JSON crudo se rechaza en el
// binding (el contrato prohíbe la coma flotante monetaria).
type recurringCreateRequest struct {
	Name        string `json:"name"`
	Amount      string `json:"amount"`
	Currency    string `json:"currency"`
	Frequency   string `json:"frequency"`
	NextDueDate string `json:"nextDueDate"`
	Type        string `json:"type"`
	StartDate   string `json:"startDate"`
	DueDay      *int   `json:"dueDay"`
}

// Validate aplica las comprobaciones de ESQUEMA del contrato. Un error aquí se responde 400
// INVALID_REQUEST.
//
// La pertenencia de `frequency` a los nueve valores publicados y la de `type` al enum
// income|expense|saving son REGLAS DE NEGOCIO y se responden 422 (RFC-021 §2): el DTO sólo
// exige que estén presentes para no confundir «campo ausente» con «valor no admitido».
func (peticion recurringCreateRequest) Validate() error {
	nombre := strings.TrimSpace(peticion.Name)
	if nombre == "" {
		return errors.New("name requerido")
	}
	if len([]rune(nombre)) > domainRecurring.MaxNameRunes {
		return errors.New("name excede la longitud publicada")
	}
	if !patronImporteRecurrente.MatchString(peticion.Amount) {
		return errors.New("amount debe ser una cadena decimal con dos decimales")
	}
	if peticion.Currency != domainRecurring.CurrencyMXN {
		return errors.New("currency debe ser MXN")
	}
	if strings.TrimSpace(peticion.Frequency) == "" {
		return errors.New("frequency requerida")
	}
	if err := httputil.DateOnly(peticion.NextDueDate).Validate(); err != nil {
		return err
	}
	if err := httputil.DateOnly(peticion.StartDate).Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(peticion.Type) == "" {
		return errors.New("type requerido")
	}
	if peticion.DueDay != nil && (*peticion.DueDay < domainRecurring.MinDueDay || *peticion.DueDay > domainRecurring.MaxDueDay) {
		// `minimum: 1` y `maximum: 28` son restricciones de esquema del contrato: 400.
		return errors.New("dueDay fuera del rango publicado 1..28")
	}
	return nil
}

// cadenaRecurrente compone la cadena de una ruta privada de pagos recurrentes: autenticación
// con estado autoritativo de sesión y, si está configurada, cuota por usuario.
//
// Devuelve nil cuando falta un componente obligatorio (proveedor de tokens o autorizador de
// sesión): el handler responde 503 en lugar de autorizar sólo con la firma. La cuota es
// opcional y fail-open: sin Redis se omite (429 no se emite) en lugar de simularse.
func cadenaRecurrente(
	tokens ports.TokenProvider,
	sesiones ports.SessionAuthorizer,
	limitador LimitadorRuta,
	clase string,
	serve http.HandlerFunc,
) http.Handler {
	if tokens == nil || sesiones == nil {
		return nil
	}
	var interior http.Handler = http.HandlerFunc(serve)
	if limitador != nil {
		interior = http.HandlerFunc(LimitarPorUsuario(limitador, clase, serve))
	}
	return middleware.AuthenticateBearerWithSession(tokens, sesiones)(interior)
}

// escribirJSONCrudo emite una respuesta ya serializada por la capa de aplicación. Se usa en
// E033 para que el replay idempotente devuelva EXACTAMENTE los mismos bytes que la creación
// original, sin reserializar el recurso.
func escribirJSONCrudo(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// writeRecurringError traduce los errores de aplicación a códigos del contrato. Nunca se
// registra el importe, el nombre, el `userId` ni ningún dato financiero.
func writeRecurringError(logger *slog.Logger, operacion string, w http.ResponseWriter, r *http.Request, err error) {
	var violacion *appRecurring.Violation
	switch {
	case errors.As(err, &violacion):
		writeRegisterError(w, r, http.StatusUnprocessableEntity, "BUSINESS_RULE_VIOLATION", "solicitud no completada", []httputil.ErrorDetail{
			{Field: violacion.Field, Reason: violacion.Reason},
		})
	case errors.Is(err, appRecurring.ErrInvalidRequest):
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", nil)
	case errors.Is(err, appRecurring.ErrUnauthorized):
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
	case errors.Is(err, appRecurring.ErrNotFound):
		writeRegisterError(w, r, http.StatusNotFound, "NOT_FOUND", "solicitud no completada", nil)
	case errors.Is(err, appRecurring.ErrVersionConflict):
		writeRegisterError(w, r, http.StatusPreconditionFailed, "VERSION_CONFLICT", "solicitud no completada", nil)
	case errors.Is(err, appRecurring.ErrConflict):
		writeRegisterError(w, r, http.StatusConflict, "CONFLICT", "solicitud no completada", nil)
	case errors.Is(err, appRecurring.ErrUnavailable):
		logger.ErrorContext(r.Context(), operacion+": dependencia no disponible",
			slog.String("error_class", "recurring_unavailable"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
	default:
		logger.ErrorContext(r.Context(), operacion+": fallo interno",
			slog.String("error_class", "internal_error"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
	}
}

// ---------------- E033: POST /api/v1/recurring-payments ----------------

// RecurringCreateHandler sirve `POST /api/v1/recurring-payments` (E033).
type RecurringCreateHandler struct {
	service   *appRecurring.Service
	logger    *slog.Logger
	protegido http.Handler
}

// NewRecurringCreateHandler construye el handler. Sin servicio, sin proveedor de tokens o sin
// autorizador de sesión, el endpoint responde 503 (fail-closed).
func NewRecurringCreateHandler(
	servicio *appRecurring.Service,
	tokens ports.TokenProvider,
	sesiones ports.SessionAuthorizer,
	limitador LimitadorRuta,
	logger *slog.Logger,
) *RecurringCreateHandler {
	if logger == nil {
		logger = slog.Default()
	}
	handler := &RecurringCreateHandler{service: servicio, logger: logger}
	handler.protegido = cadenaRecurrente(tokens, sesiones, limitador, "e033", handler.serve)
	return handler
}

// ServeHTTP fija la cabecera de caché antes de cualquier decisión y delega en la cadena
// autenticada.
func (h *RecurringCreateHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Información financiera del titular: nunca cacheable por proxies ni por el cliente.
	w.Header().Set("Cache-Control", "private, no-store")

	if h == nil || h.service == nil || h.protegido == nil {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}
	h.protegido.ServeHTTP(w, r)
}

// serve se ejecuta con la sesión ya verificada: `sub` y `sid` son válidos.
//
// Orden de decisión: identidad → `Idempotency-Key` → cuerpo estricto → caso de uso. La clave
// de idempotencia precede al cuerpo porque una escritura financiera sin clave no debe
// procesar el payload en absoluto.
func (h *RecurringCreateHandler) serve(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}

	clave, errClave := RequireIdempotencyKey(r)
	if errClave != nil {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: HeaderIdempotencyKey, Reason: "required"},
		})
		return
	}

	var peticion recurringCreateRequest
	if err := BindJSONLimit(w, r, &peticion, recurringMaxRequestBodyBytes); err != nil {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "body", Reason: "invalid_json"},
		})
		return
	}

	importe, errImporte := httputil.ParseDecimalToCents(peticion.Amount)
	if errImporte != nil {
		// Defensa en profundidad: `Validate` ya aplicó el patrón publicado.
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "amount", Reason: "invalid_format"},
		})
		return
	}

	resultado, err := h.service.Create(r.Context(), appRecurring.CreateCommand{
		UserID:         userID,
		IdempotencyKey: clave,
		Name:           peticion.Name,
		AmountCents:    importe,
		Currency:       peticion.Currency,
		Frequency:      peticion.Frequency,
		NextDueDate:    peticion.NextDueDate,
		Type:           peticion.Type,
		StartDate:      peticion.StartDate,
		DueDay:         peticion.DueDay,
	})
	if err != nil {
		writeRecurringError(h.logger, "recurring-create", w, r, err)
		return
	}

	// El cuerpo lo produce la capa de aplicación y es el mismo en creación y en replay: el
	// cliente recibe bytes idénticos si repite la petición con la misma clave.
	w.Header().Set("Location", "/api/v1/recurring-payments/"+resultado.ID)
	if resultado.Replayed {
		w.Header().Set("X-Idempotent-Replay", "true")
	}
	escribirJSONCrudo(w, http.StatusCreated, resultado.Body)
}

// ---------------- E034: GET /api/v1/recurring-payments/{recurringPaymentId} ----------------

// recurringPaymentIDFromPath extrae `recurringPaymentId` del path —el router lo expone con
// `SetPathValue`, nunca por query ni por cabecera— y valida la representación canónica de
// uuid que publica el contrato (`format: uuid`). Un valor ausente o mal formado se responde
// 400 sin reflejar el valor recibido.
func recurringPaymentIDFromPath(r *http.Request) (string, bool) {
	valor := strings.TrimSpace(r.PathValue("recurringPaymentId"))
	if valor == "" {
		return "", false
	}
	return valor, appIdentity.IsUUID(valor)
}

// RecurringGetHandler sirve `GET /api/v1/recurring-payments/{recurringPaymentId}` (E034).
//
// La propiedad del recurso se resuelve por construcción: la clave es `USER#<sub>`, de modo que
// el identificador de otra cuenta no existe en la partición del llamador y produce el mismo
// 404 que uno inexistente (sin enumeración).
//
// Los ítems heredados del alta (D-E032-01) se devuelven igual que los creados por E033: la
// proyección determinista ocurre en el adaptador y aquí no se distingue su origen.
type RecurringGetHandler struct {
	service   *appRecurring.Service
	logger    *slog.Logger
	protegido http.Handler
}

// NewRecurringGetHandler construye el handler. Sin servicio, sin proveedor de tokens o sin
// autorizador de sesión, el endpoint responde 503 (fail-closed).
func NewRecurringGetHandler(
	servicio *appRecurring.Service,
	tokens ports.TokenProvider,
	sesiones ports.SessionAuthorizer,
	limitador LimitadorRuta,
	logger *slog.Logger,
) *RecurringGetHandler {
	if logger == nil {
		logger = slog.Default()
	}
	handler := &RecurringGetHandler{service: servicio, logger: logger}
	handler.protegido = cadenaRecurrente(tokens, sesiones, limitador, "e034", handler.serve)
	return handler
}

// ServeHTTP fija la cabecera de caché antes de cualquier decisión y delega en la cadena
// autenticada.
func (h *RecurringGetHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Información financiera del titular: nunca cacheable por proxies ni por el cliente.
	w.Header().Set("Cache-Control", "private, no-store")

	if h == nil || h.service == nil || h.protegido == nil {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}
	h.protegido.ServeHTTP(w, r)
}

// serve se ejecuta con la sesión ya verificada.
func (h *RecurringGetHandler) serve(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}

	paymentID, valido := recurringPaymentIDFromPath(r)
	if !valido {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "recurringPaymentId", Reason: "invalid_format"},
		})
		return
	}

	pago, err := h.service.Get(r.Context(), userID, paymentID)
	if err != nil {
		writeRecurringError(h.logger, "recurring-get", w, r, err)
		return
	}

	cuerpo, err := pago.Body()
	if err != nil {
		h.logger.ErrorContext(r.Context(), "recurring-get: fallo al serializar la respuesta",
			slog.String("error_class", "response_write"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
		return
	}

	// ETag fuerte de la versión vigente: es el validador que E035 exige en `If-Match`.
	w.Header().Set("ETag", FormatETag(pago.Version))
	escribirJSONCrudo(w, http.StatusOK, cuerpo)
}
