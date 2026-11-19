package http

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	appGoals "github.com/nexumpr/Nexum-Backend/internal/application/goals"
	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	domainGoal "github.com/nexumpr/Nexum-Backend/internal/domain/goal"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// Transporte de E041 (`PATCH /api/v1/goals/{goalId}`).
//
// Reglas:
//   - `Authorization: Bearer` RS256 y estado autoritativo de la sesión en DynamoDB.
//   - `If-Match` OBLIGATORIO: sin cabecera 428; ETag débil u obsoleto 412; formato inválido 400.
//     La versión se publica como ETag fuerte `"vN"`, el mismo que devuelve E040.
//   - El DTO es cerrado: `{name?, targetAmount?, targetDate?, status?}`. `openingSaved`,
//     `savedAmount`, `currency`, `version` e `id` no se aceptan en el cuerpo (campo desconocido
//     ⇒ 400), de modo que el cliente no puede reescribir historia ni saltarse el CAS.
//   - La respuesta 200 devuelve el contrato `Goal` completo con el `ETag` de la nueva versión.

// goalPatchRequest es `GoalPatch`. Todos los campos son opcionales, pero al menos uno debe
// llegar: un PATCH sin cambios no incrementa la versión ni escribe nada.
type goalPatchRequest struct {
	Name         *string `json:"name"`
	TargetAmount *string `json:"targetAmount"`
	TargetDate   *string `json:"targetDate"`
	Status       *string `json:"status"`
}

// Validate aplica las comprobaciones de ESQUEMA del contrato (400 INVALID_REQUEST). Las reglas
// de negocio —objetivo positivo, objetivo por debajo del ahorro, transiciones— se responden 422.
func (r goalPatchRequest) Validate() error {
	if r.Name == nil && r.TargetAmount == nil && r.TargetDate == nil && r.Status == nil {
		return errors.New("la modificacion no contiene cambios")
	}
	if r.Name != nil {
		if _, err := domainGoal.NormalizeName(*r.Name); err != nil {
			return errors.New("name debe tener entre 1 y 120 caracteres")
		}
	}
	if r.TargetAmount != nil && !patronImporteMeta.MatchString(*r.TargetAmount) {
		return errors.New("targetAmount debe ser una cadena decimal con dos decimales")
	}
	if r.TargetDate != nil {
		if !patronFechaMeta.MatchString(*r.TargetDate) {
			return errors.New("targetDate debe tener formato YYYY-MM-DD")
		}
		if _, err := time.Parse(domainGoal.DateLayout, *r.TargetDate); err != nil {
			return errors.New("targetDate no es una fecha valida")
		}
	}
	if r.Status != nil {
		if _, err := domainGoal.ParseStatus(*r.Status); err != nil {
			return errors.New("status debe ser active, achieved o archived")
		}
	}
	return nil
}

// GoalPatchHandler sirve `PATCH /api/v1/goals/{goalId}` (E041).
type GoalPatchHandler struct {
	service   *appGoals.Service
	logger    *slog.Logger
	protected http.Handler
}

// NewGoalPatchHandler construye el handler. Sin servicio, sin proveedor de tokens o sin
// autorizador de sesión responde 503 (fail-closed).
func NewGoalPatchHandler(
	service *appGoals.Service,
	tokens ports.TokenProvider,
	sessions ports.SessionAuthorizer,
	logger *slog.Logger,
) *GoalPatchHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &GoalPatchHandler{service: service, logger: logger}
	h.protected = cadenaMetas(service, tokens, sessions, h.serve)
	return h
}

// ServeHTTP fija la cabecera de caché y delega en la cadena autenticada.
func (h *GoalPatchHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")

	if h == nil || h.service == nil || h.protected == nil {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}
	h.protected.ServeHTTP(w, r)
}

// serve se ejecuta con la sesión ya verificada.
//
// Orden de decisión: identidad → goalId → If-Match → cuerpo. La precondición precede al cuerpo
// porque una escritura sin validador no debe llegar a evaluarse como operación financiera.
func (h *GoalPatchHandler) serve(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}

	goalID := strings.TrimSpace(r.PathValue("goalId"))
	if !appIdentity.IsUUID(goalID) {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "goalId", Reason: "invalid_format"},
		})
		return
	}

	expectedVersion, errPrecondicion := parseIfMatchFuerte(r.Header.Get("If-Match"))
	if errPrecondicion != nil {
		escribirErrorPrecondicionMeta(w, r, errPrecondicion)
		return
	}

	var peticion goalPatchRequest
	if err := BindJSON(w, r, &peticion); err != nil {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", nil)
		return
	}

	var targetCents *int64
	if peticion.TargetAmount != nil {
		cents, errImporte := httputil.ParseDecimalToCents(*peticion.TargetAmount)
		if errImporte != nil {
			writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
				{Field: "targetAmount", Reason: "invalid_format"},
			})
			return
		}
		targetCents = &cents
	}

	actualizada, err := h.service.Update(r.Context(), appGoals.UpdateCommand{
		UserID:          userID,
		GoalID:          goalID,
		ExpectedVersion: expectedVersion,
		Name:            peticion.Name,
		TargetAmount:    targetCents,
		TargetDate:      peticion.TargetDate,
		Status:          peticion.Status,
	})
	if err != nil {
		writeGoalsError(w, r, h.logger, err)
		return
	}
	contract, errProyeccion := appGoals.ContractOf(actualizada)
	if errProyeccion != nil {
		writeGoalsError(w, r, h.logger, errProyeccion)
		return
	}

	w.Header().Set("ETag", FormatETag(actualizada.Version))
	if errEscritura := httputil.WriteJSON(w, http.StatusOK, contract); errEscritura != nil {
		h.logger.ErrorContext(r.Context(), "goals: fallo al serializar la meta modificada",
			slog.String("error_class", "response_write"),
			slog.String("trace_id", traceID(r)))
	}
}

// escribirErrorPrecondicionMeta traduce los fallos de `If-Match` a los códigos del contrato:
// 428 sin cabecera, 412 para un validador débil (RFC 9110 exige comparación fuerte) y 400 para
// cualquier formato que no sea `"vN"`.
func escribirErrorPrecondicionMeta(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrIfMatchRequerido):
		writeRegisterError(w, r, http.StatusPreconditionRequired, "PRECONDITION_REQUIRED", "If-Match requerido", []httputil.ErrorDetail{
			{Field: "If-Match", Reason: "required"},
		})
	case errors.Is(err, errIfMatchDebil):
		writeRegisterError(w, r, http.StatusPreconditionFailed, "VERSION_CONFLICT", "If-Match debe ser un ETag fuerte", nil)
	default:
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "If-Match", Reason: "invalid_format"},
		})
	}
}
