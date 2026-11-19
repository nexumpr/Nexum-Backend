package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	appGoals "github.com/nexumpr/Nexum-Backend/internal/application/goals"
	domainGoal "github.com/nexumpr/Nexum-Backend/internal/domain/goal"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// Transporte de las metas de ahorro. Esta unidad implementa únicamente E039
// (`POST /api/v1/goals`); E038/E040/E041 se añaden en sus propias unidades.
//
// Reglas comunes:
//   - La ruta exige `Authorization: Bearer` RS256 y estado autoritativo de la sesión en
//     DynamoDB: la identidad sale exclusivamente de `sub`/`sid` verificados.
//   - La información es financiera: toda respuesta lleva `Cache-Control: private, no-store`.
//   - El cuerpo de éxito se escribe a partir de los bytes que produjo la capa de aplicación,
//     de modo que un replay de idempotencia devuelve exactamente el mismo JSON.
//   - Ningún error revela identificadores, hashes, tokens ni configuración de AWS.

const (
	// cabeceraIdempotencia es el nombre publicado del parámetro obligatorio de E039.
	cabeceraIdempotencia = "Idempotency-Key"
	// cabeceraReplay indica que la respuesta proviene de un registro de idempotencia.
	cabeceraReplay = "X-Idempotent-Replay"
)

// patronImporteMeta es el patrón publicado de `targetAmount`/`initialSavedAmount`:
// parte entera sin ceros a la izquierda de hasta doce dígitos y exactamente dos decimales.
var patronImporteMeta = regexp.MustCompile(`^(0|[1-9][0-9]{0,11})\.[0-9]{2}$`)

// patronFechaMeta valida la forma `YYYY-MM-DD` antes de parsear el calendario real.
var patronFechaMeta = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)

// goalCreateRequest es `GoalCreate`: cuatro campos requeridos, uno opcional y ningún otro.
// `initialSavedAmount` es puntero para distinguir «ausente» (0.00 por defecto) de un valor
// presente que debe validarse.
type goalCreateRequest struct {
	Name               string  `json:"name"`
	TargetAmount       string  `json:"targetAmount"`
	Currency           string  `json:"currency"`
	TargetDate         string  `json:"targetDate"`
	InitialSavedAmount *string `json:"initialSavedAmount"`
}

// Validate aplica las comprobaciones de ESQUEMA del contrato (400 INVALID_REQUEST). Las
// reglas de negocio —monto objetivo positivo, apertura menor o igual al objetivo— se validan
// en la capa de aplicación y se responden 422.
func (r goalCreateRequest) Validate() error {
	if _, err := domainGoal.NormalizeName(r.Name); err != nil {
		return errors.New("name debe tener entre 1 y 120 caracteres")
	}
	if !patronImporteMeta.MatchString(r.TargetAmount) {
		return errors.New("targetAmount debe ser una cadena decimal con dos decimales")
	}
	if r.Currency != domainGoal.CurrencyMXN {
		return errors.New("currency debe ser MXN")
	}
	if !patronFechaMeta.MatchString(r.TargetDate) {
		return errors.New("targetDate debe tener formato YYYY-MM-DD")
	}
	if _, err := time.Parse(domainGoal.DateLayout, r.TargetDate); err != nil {
		return errors.New("targetDate no es una fecha valida")
	}
	if r.InitialSavedAmount != nil && !patronImporteMeta.MatchString(*r.InitialSavedAmount) {
		return errors.New("initialSavedAmount debe ser una cadena decimal con dos decimales")
	}
	return nil
}

// GoalsCreateHandler sirve `POST /api/v1/goals` (E039).
type GoalsCreateHandler struct {
	service   *appGoals.Service
	logger    *slog.Logger
	protected http.Handler
}

// NewGoalsCreateHandler construye el handler. Sin servicio, sin proveedor de tokens o sin
// autorizador de sesión el endpoint responde 503 (fail-closed): nunca se crea una meta sin
// identidad verificada ni sin persistencia.
func NewGoalsCreateHandler(
	service *appGoals.Service,
	tokens ports.TokenProvider,
	sessions ports.SessionAuthorizer,
	logger *slog.Logger,
) *GoalsCreateHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &GoalsCreateHandler{service: service, logger: logger}
	if service != nil && tokens != nil && sessions != nil {
		h.protected = middleware.AuthenticateBearerWithSession(tokens, sessions)(http.HandlerFunc(h.serve))
	}
	return h
}

// ServeHTTP fija la cabecera de caché y delega en la cadena autenticada.
func (h *GoalsCreateHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Información financiera del titular: nunca cacheable por proxies ni por el cliente.
	w.Header().Set("Cache-Control", "private, no-store")

	if h == nil || h.service == nil || h.protected == nil {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}
	h.protected.ServeHTTP(w, r)
}

// serve se ejecuta con la sesión ya verificada.
//
// Orden de decisión: identidad → Idempotency-Key → cuerpo → importes. La clave precede al
// cuerpo porque una escritura sin idempotencia no debe evaluarse como operación financiera.
func (h *GoalsCreateHandler) serve(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}

	clave, errClave := RequireIdempotencyKey(r)
	if errClave != nil {
		razon := "invalid_length"
		if strings.TrimSpace(r.Header.Get(cabeceraIdempotencia)) == "" {
			razon = "required"
		}
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: cabeceraIdempotencia, Reason: razon},
		})
		return
	}

	var peticion goalCreateRequest
	if err := BindJSON(w, r, &peticion); err != nil {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", nil)
		return
	}

	targetCents, errImporte := httputil.ParseDecimalToCents(peticion.TargetAmount)
	if errImporte != nil {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "targetAmount", Reason: "invalid_format"},
		})
		return
	}
	var openingCents int64
	if peticion.InitialSavedAmount != nil {
		openingCents, errImporte = httputil.ParseDecimalToCents(*peticion.InitialSavedAmount)
		if errImporte != nil {
			writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
				{Field: "initialSavedAmount", Reason: "invalid_format"},
			})
			return
		}
	}

	resultado, err := h.service.Create(r.Context(), appGoals.CreateCommand{
		UserID:            userID,
		IdempotencyKey:    clave,
		Name:              peticion.Name,
		TargetAmountCents: targetCents,
		InitialSavedCents: openingCents,
		Currency:          peticion.Currency,
		TargetDate:        peticion.TargetDate,
	})
	if err != nil {
		writeGoalsError(w, r, h.logger, err)
		return
	}

	if strings.TrimSpace(resultado.Location) != "" {
		w.Header().Set("Location", resultado.Location)
	}
	if resultado.Replayed {
		w.Header().Set(cabeceraReplay, "true")
	}
	if errEscritura := httputil.WriteJSON(w, resultado.Status, json.RawMessage(resultado.Body)); errEscritura != nil {
		h.logger.ErrorContext(r.Context(), "goals: fallo al serializar la respuesta",
			slog.String("error_class", "response_write"),
			slog.String("trace_id", traceID(r)))
	}
}

// writeGoalsError traduce los errores de aplicación a los códigos del contrato. Nunca se
// registra el `userId`, el identificador de la meta, la clave de idempotencia ni importes.
func writeGoalsError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	if logger == nil {
		logger = slog.Default()
	}
	switch {
	case errors.Is(err, appGoals.ErrUnauthorized):
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
	case errors.Is(err, appGoals.ErrInvalidRequest):
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", nil)
	case errors.Is(err, appGoals.ErrNotFound):
		writeRegisterError(w, r, http.StatusNotFound, "NOT_FOUND", "recurso no encontrado", nil)
	case errors.Is(err, appGoals.ErrConflict):
		writeRegisterError(w, r, http.StatusConflict, "CONFLICT", "la clave de idempotencia ya se uso con otro contenido", nil)
	case errors.Is(err, appGoals.ErrVersionConflict):
		writeRegisterError(w, r, http.StatusPreconditionFailed, "VERSION_CONFLICT", "la version enviada no es la vigente", nil)
	case errors.Is(err, appGoals.ErrTargetNotPositive):
		writeRegisterError(w, r, http.StatusUnprocessableEntity, "BUSINESS_RULE_VIOLATION", "el monto objetivo debe ser positivo", []httputil.ErrorDetail{
			{Field: "targetAmount", Reason: "out_of_range"},
		})
	case errors.Is(err, appGoals.ErrOpeningExceedsTarget):
		writeRegisterError(w, r, http.StatusUnprocessableEntity, "BUSINESS_RULE_VIOLATION", "el ahorro inicial excede el monto objetivo", []httputil.ErrorDetail{
			{Field: "initialSavedAmount", Reason: "out_of_range"},
		})
	case errors.Is(err, appGoals.ErrTargetBelowSaved):
		writeRegisterError(w, r, http.StatusUnprocessableEntity, "BUSINESS_RULE_VIOLATION", "el monto objetivo no puede ser menor que el ahorro acumulado", []httputil.ErrorDetail{
			{Field: "targetAmount", Reason: "below_saved_amount"},
		})
	case errors.Is(err, appGoals.ErrInvalidTransition):
		writeRegisterError(w, r, http.StatusUnprocessableEntity, "BUSINESS_RULE_VIOLATION", "transicion de estado no admitida", []httputil.ErrorDetail{
			{Field: "status", Reason: "invalid_transition"},
		})
	case errors.Is(err, appGoals.ErrInconsistent):
		logger.ErrorContext(r.Context(), "goals: item persistido incoherente",
			slog.String("error_class", "goals_inconsistent"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
	case errors.Is(err, appGoals.ErrUnavailable):
		logger.ErrorContext(r.Context(), "goals: dependencia no disponible",
			slog.String("error_class", "goals_unavailable"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
	default:
		logger.ErrorContext(r.Context(), "goals: fallo interno",
			slog.String("error_class", "internal_error"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
	}
}
