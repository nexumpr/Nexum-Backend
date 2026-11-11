package http

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	"github.com/nexumpr/Nexum-Backend/internal/infrastructure/telemetry"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// RegisterRateLimiter abstrae la ventana de rate limit de registro. En produccion
// se implementa con Redis; en pruebas se usa un doble.
type RegisterRateLimiter interface {
	Allow(ctx context.Context, class, identifier string) (bool, time.Duration, error)
}

// RegisterIncomeRequest es el bloque `income` del contrato E001 V6.0.2.
type RegisterIncomeRequest struct {
	Amount    *httputil.DecimalMoney `json:"amount"`
	Frequency *string                `json:"frequency"`
}

// RegisterRecurringExpenseRequest es una fila de `recurringExpenses`.
type RegisterRecurringExpenseRequest struct {
	Concept   *string                `json:"concept"`
	Amount    *httputil.DecimalMoney `json:"amount"`
	Frequency *string                `json:"frequency"`
	Category  *string                `json:"category"`
	Type      *string                `json:"type"`
}

// RegisterPrivacyRequest es el bloque `privacy` del contrato E001 V6.0.2.
type RegisterPrivacyRequest struct {
	Accepted      *bool   `json:"accepted"`
	NoticeVersion *string `json:"noticeVersion"`
}

// RegisterClientRequest es el bloque `client`, enteramente opcional.
type RegisterClientRequest struct {
	Locale   *string `json:"locale"`
	TimeZone *string `json:"timeZone"`
}

// RegisterRequest es el DTO exacto de E001 V6.0.2. Los campos desconocidos los rechaza
// BindJSON, incluido `signupConfirm`, que Android compara en local y nunca se envia.
type RegisterRequest struct {
	Name              string                             `json:"name"`
	Email             string                             `json:"email"`
	Password          string                             `json:"password"`
	Income            *RegisterIncomeRequest             `json:"income"`
	RecurringExpenses *[]RegisterRecurringExpenseRequest `json:"recurringExpenses"`
	Privacy           *RegisterPrivacyRequest            `json:"privacy"`
	Client            *RegisterClientRequest             `json:"client"`
}

// RegisterHandler expone POST /api/v1/auth/register.
type RegisterHandler struct {
	service *appIdentity.RegisterService
	limiter RegisterRateLimiter
	logger  *slog.Logger
}

// NewRegisterHandler construye el handler con el servicio, el limitador y el logger.
// Un limitador nil se trata como dependencia no disponible (503, fail-closed).
func NewRegisterHandler(service *appIdentity.RegisterService, limiter RegisterRateLimiter, logger *slog.Logger) *RegisterHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &RegisterHandler{service: service, limiter: limiter, logger: logger}
}

// ServeHTTP implementa el contrato HTTP de E001.
func (h *RegisterHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.service == nil {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}

	if !hasJSONContentType(r) {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "Content-Type", Reason: "application_json_required"},
		})
		return
	}

	if _, err := RequireIdempotencyKey(r); err != nil {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: HeaderIdempotencyKey, Reason: "required_or_invalid"},
		})
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get(HeaderIdempotencyKey))
	if !appIdentity.ValidIdempotencyKey(idempotencyKey) {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: HeaderIdempotencyKey, Reason: "invalid"},
		})
		return
	}

	if h.limiter == nil {
		h.logger.ErrorContext(r.Context(), "registro rechazado: rate limiter no configurado",
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}
	allowed, retryAfter, err := h.limiter.Allow(r.Context(), "auth_register", registerClientIP(r))
	if err != nil {
		h.logger.ErrorContext(r.Context(), "registro rechazado: fallo del rate limiter",
			slog.String("error_class", "rate_limiter_unavailable"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}
	if !allowed {
		w.Header().Set("Retry-After", retryAfterHeader(retryAfter))
		writeRegisterError(w, r, http.StatusTooManyRequests, "RATE_LIMITED", "demasiadas solicitudes", nil)
		return
	}

	var request RegisterRequest
	if err := BindJSON(w, r, &request); err != nil {
		h.writeBindError(w, r, err, "body")
		return
	}

	command, violation := toRegisterCommand(request)
	if violation != nil {
		h.writeViolation(w, r, violation)
		return
	}
	command.IdempotencyKey = idempotencyKey

	result, err := h.service.Register(r.Context(), command)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	if result == nil {
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
		return
	}

	if result.Replayed {
		w.Header().Set("X-Idempotent-Replay", "true")
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if result.Location != "" {
		w.Header().Set("Location", result.Location)
	}
	w.WriteHeader(result.Status)
	_, _ = w.Write(result.Body)
}

// toRegisterCommand traduce el DTO al comando de aplicacion. Los bloques obligatorios
// que llegan ausentes producen 400 con el campo exacto; los importes se convierten a
// centavos enteros en el borde, sin coma flotante (A-040 §2.2).
func toRegisterCommand(request RegisterRequest) (appIdentity.RegisterCommand, *appIdentity.FieldViolation) {
	command := appIdentity.RegisterCommand{
		Name:     request.Name,
		Email:    request.Email,
		Password: request.Password,
	}

	if request.Income == nil {
		return command, &appIdentity.FieldViolation{Field: "income", Reason: "required"}
	}
	if request.Income.Amount == nil {
		return command, &appIdentity.FieldViolation{Field: "income.amount", Reason: "required"}
	}
	if request.Income.Frequency == nil {
		return command, &appIdentity.FieldViolation{Field: "income.frequency", Reason: "required"}
	}
	command.Income = appIdentity.IncomeCommand{
		AmountCents: request.Income.Amount.Cents,
		Frequency:   *request.Income.Frequency,
	}

	if request.RecurringExpenses == nil {
		return command, &appIdentity.FieldViolation{Field: "recurringExpenses", Reason: "required"}
	}
	expenses := make([]appIdentity.RecurringExpenseCommand, 0, len(*request.RecurringExpenses))
	for i, item := range *request.RecurringExpenses {
		prefix := "recurringExpenses[" + strconv.Itoa(i) + "]"
		if item.Concept == nil {
			return command, &appIdentity.FieldViolation{Field: prefix + ".concept", Reason: "required"}
		}
		if item.Amount == nil {
			return command, &appIdentity.FieldViolation{Field: prefix + ".amount", Reason: "required"}
		}
		if item.Frequency == nil {
			return command, &appIdentity.FieldViolation{Field: prefix + ".frequency", Reason: "required"}
		}
		if item.Category == nil {
			return command, &appIdentity.FieldViolation{Field: prefix + ".category", Reason: "required"}
		}
		if item.Type == nil {
			return command, &appIdentity.FieldViolation{Field: prefix + ".type", Reason: "required"}
		}
		expenses = append(expenses, appIdentity.RecurringExpenseCommand{
			Concept:     *item.Concept,
			AmountCents: item.Amount.Cents,
			Frequency:   *item.Frequency,
			Category:    *item.Category,
			Type:        *item.Type,
		})
	}
	command.RecurringExpenses = expenses

	if request.Privacy == nil {
		return command, &appIdentity.FieldViolation{Field: "privacy", Reason: "required"}
	}
	if request.Privacy.Accepted == nil {
		return command, &appIdentity.FieldViolation{Field: "privacy.accepted", Reason: "required"}
	}
	if request.Privacy.NoticeVersion == nil {
		return command, &appIdentity.FieldViolation{Field: "privacy.noticeVersion", Reason: "required"}
	}
	command.Privacy = appIdentity.PrivacyCommand{
		Accepted:      *request.Privacy.Accepted,
		NoticeVersion: *request.Privacy.NoticeVersion,
	}

	if request.Client != nil {
		if request.Client.Locale != nil {
			command.Client.Locale = *request.Client.Locale
		}
		if request.Client.TimeZone != nil {
			command.Client.TimeZone = *request.Client.TimeZone
		}
	}

	return command, nil
}

func (h *RegisterHandler) writeBindError(w http.ResponseWriter, r *http.Request, err error, field string) {
	var violation *appIdentity.FieldViolation
	if errors.As(err, &violation) {
		h.writeViolation(w, r, violation)
		return
	}
	writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
		{Field: field, Reason: "invalid_json"},
	})
}

func (h *RegisterHandler) writeViolation(w http.ResponseWriter, r *http.Request, violation *appIdentity.FieldViolation) {
	status := http.StatusBadRequest
	code := "INVALID_REQUEST"
	if violation.Business {
		status = http.StatusUnprocessableEntity
		code = "BUSINESS_RULE_VIOLATION"
	}
	writeRegisterError(w, r, status, code, "solicitud no completada", []httputil.ErrorDetail{
		{Field: violation.Field, Reason: violation.Reason},
	})
}

func (h *RegisterHandler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	var violation *appIdentity.FieldViolation
	if errors.As(err, &violation) {
		h.writeViolation(w, r, violation)
		return
	}

	switch {
	case errors.Is(err, appIdentity.ErrEmailAlreadyRegistered):
		writeRegisterError(w, r, http.StatusConflict, "CONFLICT", "el correo electronico ya se encuentra registrado", nil)
	case errors.Is(err, appIdentity.ErrIdempotencyConflict):
		writeRegisterError(w, r, http.StatusConflict, "CONFLICT", "la Idempotency-Key ya fue utilizada con una carga util diferente", nil)
	case errors.Is(err, appIdentity.ErrTransactionTooLarge):
		writeRegisterError(w, r, http.StatusUnprocessableEntity, "BUSINESS_RULE_VIOLATION", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "recurringExpenses", Reason: "too_many_items"},
		})
	case errors.Is(err, appIdentity.ErrStoreUnavailable):
		h.logger.ErrorContext(r.Context(), "registro: persistencia no disponible",
			slog.String("error_class", "store_unavailable"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
	default:
		h.logger.ErrorContext(r.Context(), "registro: fallo interno",
			slog.String("error_class", "internal_error"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
	}
}

func hasJSONContentType(r *http.Request) bool {
	value := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	return strings.HasPrefix(value, "application/json")
}

func registerClientIP(r *http.Request) string {
	// D2: CloudFront -> VPC Origin -> ALB interno. El ALB no es accesible desde
	// Internet; X-Forwarded-For lo fija el proxy de confianza. Se toma el primer salto.
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
		part := strings.TrimSpace(strings.Split(forwarded, ",")[0])
		if part != "" {
			return part
		}
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	if remote := strings.TrimSpace(r.RemoteAddr); remote != "" {
		return remote
	}
	return "unknown"
}

func retryAfterHeader(wait time.Duration) string {
	seconds := int64(wait / time.Second)
	if wait%time.Second != 0 {
		seconds++
	}
	if seconds < 1 {
		seconds = 1
	}
	return strconv.FormatInt(seconds, 10)
}

func traceID(r *http.Request) string {
	if span := telemetry.FromContext(r.Context()); span != nil {
		if span.TraceID != "" {
			return span.TraceID
		}
		if span.RequestID != "" {
			return span.RequestID
		}
	}
	if id := strings.TrimSpace(r.Header.Get(telemetry.RequestIDHeader)); id != "" {
		return id
	}
	if id := strings.TrimSpace(r.Header.Get("X-Trace-ID")); id != "" {
		return id
	}
	return ""
}

func writeRegisterError(w http.ResponseWriter, r *http.Request, status int, code, message string, details []httputil.ErrorDetail) {
	_ = httputil.WriteErrorEnvelope(w, status, code, message, traceID(r), details)
}
