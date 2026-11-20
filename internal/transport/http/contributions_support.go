package http

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	appContrib "github.com/nexumpr/Nexum-Backend/internal/application/contributions"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// Utilidades compartidas por los handlers de aportaciones E043–E047. La autenticación debe ser
// idéntica en las cinco rutas: Bearer RS256 + estado autoritativo de la sesión.
const (
	CuotaAportacionesPorMinuto int64 = 60
	MaxBodyAportaciones              = 64 << 10
)

func CadenaAportaciones(
	service *appContrib.Service,
	tokens ports.TokenProvider,
	sessions ports.SessionAuthorizer,
	serve http.HandlerFunc,
) http.Handler {
	if service == nil || tokens == nil || sessions == nil {
		return nil
	}
	return middleware.AuthenticateBearerWithSession(tokens, sessions)(http.HandlerFunc(serve))
}

func ParseLimitQuery(raw string) (int, error) {
	limpio := strings.TrimSpace(raw)
	if limpio == "" {
		return appContrib.DefaultLimit, nil
	}
	valor, err := strconv.Atoi(limpio)
	if err != nil || valor < 1 || valor > appContrib.MaxLimit {
		return 0, errors.New("limit fuera de rango")
	}
	return valor, nil
}

func WriteContributionsError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	var violacion *appContrib.FieldViolation
	switch {
	case errors.As(err, &violacion):
		writeRegisterError(w, r, http.StatusUnprocessableEntity, "BUSINESS_RULE_VIOLATION", "regla de negocio incumplida", []httputil.ErrorDetail{{Field: violacion.Field, Reason: violacion.Reason}})
	case errors.Is(err, appContrib.ErrInvalidRequest):
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", nil)
	case errors.Is(err, appContrib.ErrUnauthorized):
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
	case errors.Is(err, appContrib.ErrNotFound), errors.Is(err, appContrib.ErrGoalNotFound):
		writeRegisterError(w, r, http.StatusNotFound, "NOT_FOUND", "recurso no encontrado", nil)
	case errors.Is(err, appContrib.ErrAlreadyReversed):
		writeRegisterError(w, r, http.StatusConflict, "CONFLICT", "la aportacion ya esta revertida", nil)
	case errors.Is(err, appContrib.ErrConflict):
		writeRegisterError(w, r, http.StatusConflict, "CONFLICT", "conflicto de la operacion", nil)
	case errors.Is(err, appContrib.ErrGoalInactive):
		writeRegisterError(w, r, http.StatusUnprocessableEntity, "BUSINESS_RULE_VIOLATION", "la meta no esta activa", []httputil.ErrorDetail{{Field: "allocations.goalId", Reason: "inactive"}})
	case errors.Is(err, appContrib.ErrCapacityExceeded):
		writeRegisterError(w, r, http.StatusUnprocessableEntity, "BUSINESS_RULE_VIOLATION", "la asignacion excede la capacidad de la meta", []httputil.ErrorDetail{{Field: "allocations.amount", Reason: "exceeds_capacity"}})
	case errors.Is(err, appContrib.ErrUnavailable):
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
	default:
		if logger != nil {
			logger.ErrorContext(r.Context(), "contributions: error no clasificado", slog.String("error_class", "internal"), slog.String("trace_id", traceID(r)))
		}
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
	}
}
