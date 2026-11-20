package http

// Transporte del pago de una regla recurrente (E037):
// `POST /api/v1/recurring-payments/{recurringPaymentId}/payments`.
//
// La ruta exige `Authorization: Bearer` con JWT RS256 y estado autoritativo de la sesión en
// DynamoDB (E004): la identidad sale exclusivamente de `sub`/`sid` verificados, de modo que el
// cliente no puede registrar un pago en la partición de otra cuenta. El identificador de la regla
// llega por `PathValue` —nunca por query ni por cabecera— y se valida como uuid.
//
// Contrato: 200 con el objeto `Movement` del OpenAPI (mismos bytes que el alta de E028 y la
// lectura de E029), `Idempotency-Key` obligatoria, `X-Idempotent-Replay: true` cuando la respuesta
// procede de una operación ya confirmada, y el sobre canónico `ErrorEnvelope` en error. Códigos:
// 200, 400, 401, 404, 405, 409, 422, 429, 500 y 503.

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	appRecurring "github.com/nexumpr/Nexum-Backend/internal/application/recurring"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

const (
	// recurringPaymentPayMaxRequestBodyBytes acota el cuerpo de E037: tres campos cortos no
	// necesitan más. Se aplica con `MaxBytesReader` dentro de `DecodeJSON`.
	recurringPaymentPayMaxRequestBodyBytes int64 = 4 * 1024

	// recurringPaymentPayClaseCuota es la clase de la cuota distribuida por usuario. Se declara
	// como constante propia para no compartir el contador de E033 (escritura de reglas) con el
	// de pagos.
	recurringPaymentPayClaseCuota = "e037"
)

// recurringPaymentPayRequest es el DTO estricto de `RecurringPaymentCreate`.
//
// `additionalProperties: false`: `BindJSONLimit` rechaza cualquier propiedad desconocida, de modo
// que `type`, `categoryId`, `recurringPaymentId`, `version` o cualquier otro campo interno no
// tienen camino de entrada. `amount` es `string` en el contrato: un número JSON produce 400 al no
// poder deserializarse como cadena, así que un `float64` nunca llega al dominio.
type recurringPaymentPayRequest struct {
	PaidAt   string `json:"paidAt"`
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

// Validate aplica las comprobaciones de ESQUEMA del contrato (400): presencia de los tres campos
// obligatorios, `date-time` RFC 3339 para `paidAt` y el patrón decimal publicado para `amount`.
// La positividad del importe, la divisa admitida y el estado de la regla son reglas de negocio y
// se responden 422/409 en la capa de aplicación.
func (peticion recurringPaymentPayRequest) Validate() error {
	if strings.TrimSpace(peticion.PaidAt) == "" {
		return errors.New("paidAt requerido")
	}
	if _, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(peticion.PaidAt)); err != nil {
		return errors.New("paidAt debe ser un date-time RFC 3339")
	}
	if !patronImporteRecurrente.MatchString(peticion.Amount) {
		return errors.New("amount debe ser una cadena decimal con dos decimales")
	}
	if strings.TrimSpace(peticion.Currency) == "" {
		return errors.New("currency requerida")
	}
	return nil
}

// RecurringPaymentHandler sirve el registro manual de un pago de regla recurrente (E037).
//
// No programa ocurrencias futuras ni modifica la regla: crea el movimiento real del pago, lo
// vincula a la regla y bloquea un segundo pago de la misma regla en la misma fecha civil local.
type RecurringPaymentHandler struct {
	service   *appRecurring.PaymentService
	logger    *slog.Logger
	protegido http.Handler
}

// NewRecurringPaymentHandler construye el handler. Sin servicio, sin proveedor de tokens o sin
// autorizador de sesión, el endpoint responde 503 (fail-closed): nunca se escribe un movimiento
// sin identidad verificada ni sin persistencia. La cuota por usuario es opcional: sin Redis se
// omite (429 no se emite) en lugar de simularse.
func NewRecurringPaymentHandler(
	servicio *appRecurring.PaymentService,
	tokens ports.TokenProvider,
	sesiones ports.SessionAuthorizer,
	limitador LimitadorRuta,
	logger *slog.Logger,
) *RecurringPaymentHandler {
	if logger == nil {
		logger = slog.Default()
	}
	handler := &RecurringPaymentHandler{service: servicio, logger: logger}
	handler.protegido = cadenaRecurrente(tokens, sesiones, limitador, recurringPaymentPayClaseCuota, handler.serve)
	return handler
}

// ServeHTTP fija la cabecera de caché antes de cualquier decisión, rechaza métodos distintos de
// POST y delega en la cadena autenticada.
func (h *RecurringPaymentHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Información financiera del titular: nunca cacheable por proxies ni por el cliente.
	w.Header().Set("Cache-Control", "private, no-store")

	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeRegisterError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "el metodo HTTP no esta permitido para este recurso", nil)
		return
	}

	if h == nil || h.service == nil || h.protegido == nil {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}
	h.protegido.ServeHTTP(w, r)
}

// serve se ejecuta con la sesión ya verificada.
//
// Orden de decisión: identidad → identificador de la regla → `Idempotency-Key` → cuerpo estricto
// → caso de uso. La clave de idempotencia precede al cuerpo porque una escritura financiera sin
// clave no debe procesar el payload en absoluto.
func (h *RecurringPaymentHandler) serve(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}

	ruleID, valido := recurringPaymentIDFromPath(r)
	if !valido {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "recurringPaymentId", Reason: "invalid_format"},
		})
		return
	}

	clave, errClave := RequireIdempotencyKey(r)
	if errClave != nil {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: HeaderIdempotencyKey, Reason: "required_or_invalid_length"},
		})
		return
	}

	var peticion recurringPaymentPayRequest
	if err := BindJSONLimit(w, r, &peticion, recurringPaymentPayMaxRequestBodyBytes); err != nil {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "body", Reason: "invalid_json"},
		})
		return
	}

	resultado, err := h.service.Pay(r.Context(), appRecurring.PayCommand{
		UserID:         userID,
		RuleID:         ruleID,
		IdempotencyKey: clave,
		PaidAt:         peticion.PaidAt,
		Amount:         peticion.Amount,
		Currency:       peticion.Currency,
	})
	if err != nil {
		writeRecurringError(h.logger, "recurring-pay", w, r, err)
		return
	}
	if resultado == nil || len(resultado.Body) == 0 {
		writeRecurringError(h.logger, "recurring-pay", w, r, appRecurring.ErrUnavailable)
		return
	}

	if resultado.Replayed {
		// El cliente puede distinguir una reproducción de un pago nuevo sin comparar cuerpos.
		w.Header().Set("X-Idempotent-Replay", "true")
	}
	// El cuerpo lo produce la capa de aplicación con la misma proyección `Movement` que
	// E028/E029: el replay devuelve exactamente los mismos bytes que el 200 original.
	escribirJSONCrudo(w, resultado.Status, resultado.Body)
}
