package goals

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	domainGoal "github.com/nexumpr/Nexum-Backend/internal/domain/goal"
)

// Caso de uso de E041 (`PATCH /api/v1/goals/{goalId}`).
//
// Invariantes:
//   - La identidad llega exclusivamente de `sub` del JWT verificado; una meta de otro titular y
//     una inexistente producen el MISMO 404.
//   - `If-Match` es obligatorio: la versión esperada viaja desde el transporte y el adaptador la
//     impone con CAS. Si no coincide, no se aplica ninguna escritura (412).
//   - No se puede reducir `targetAmount` por debajo de `savedAmount` (422). La comprobación se
//     hace aquí para dar un error preciso y se repite de forma atómica en la condición de
//     DynamoDB para cubrir una aportación concurrente.
//   - `openingSaved` y `savedAmount` son inmutables en este contrato; sólo cambian `name`,
//     `targetAmount`, `targetDate` y `status`.

// UpdateCommand es la entrada de E041 ya normalizada por el transporte.
type UpdateCommand struct {
	UserID          string
	GoalID          string
	ExpectedVersion int64
	Name            *string
	TargetAmount    *int64
	TargetDate      *string
	Status          *string
}

// UpdateWrite es la escritura ya validada que recibe la persistencia.
type UpdateWrite struct {
	UserID          string
	GoalID          string
	Patch           domainGoal.Patch
	ExpectedVersion int64
	Now             time.Time
}

// Update aplica la modificación de E041.
//
// Secuencia: validar entrada → construir y validar el patch de dominio → leer la meta con
// consistencia fuerte (404 homogéneo) → prevalidar objetivo y transición → escritura con CAS.
// Un fracaso de la condición se traduce en 412 (versión obsoleta), 422 (objetivo bajo el ahorro)
// o 409 (conflicto tras agotar los reintentos), nunca en una escritura parcial.
func (s *Service) Update(ctx context.Context, cmd UpdateCommand) (*domainGoal.Goal, error) {
	if s == nil || s.store == nil || s.now == nil {
		return nil, ErrUnavailable
	}
	userID := strings.TrimSpace(cmd.UserID)
	if userID == "" {
		return nil, ErrUnauthorized
	}
	goalID := strings.TrimSpace(cmd.GoalID)
	if goalID == "" {
		return nil, fmt.Errorf("%w: goalId requerido", ErrInvalidRequest)
	}
	if cmd.ExpectedVersion < 1 {
		return nil, fmt.Errorf("%w: If-Match invalido", ErrInvalidRequest)
	}

	patch, errPatch := buildPatch(cmd)
	if errPatch != nil {
		return nil, errPatch
	}

	current, errLectura := s.store.LoadGoal(ctx, userID, goalID)
	if errLectura != nil {
		return nil, normalizeStoreError(errLectura)
	}
	if current == nil {
		return nil, ErrNotFound
	}
	// E042 deja un tombstone con `deleted=true` (RFC-022 §6): una meta borrada no se puede
	// modificar y se responde el MISMO 404 que una inexistente. El adaptador impone además la
	// condición `deleted = false` en la escritura para cubrir una carrera con el borrado.
	if current.Deleted {
		return nil, ErrNotFound
	}

	// Un patch vacio es valido para el contrato (`GoalPatch.required = []`): no escribe
	// nada, no incrementa la version y devuelve el recurso actual con su ETag. La
	// precondicion se sigue evaluando: un If-Match obsoleto es 412 aunque no haya cambios.
	if patch.Empty() {
		if current.Version != cmd.ExpectedVersion {
			return nil, ErrVersionConflict
		}
		return current, nil
	}

	// Prevalidación con la versión leída: da el error preciso (422) sin depender del orden en
	// que DynamoDB evalúe la condición. La autoridad final sigue siendo la condición atómica.
	if patch.TargetCents != nil && *patch.TargetCents < current.SavedAmount.Amount() {
		return nil, ErrTargetBelowSaved
	}
	if patch.Status != nil && !current.Status.CanTransitionTo(*patch.Status) {
		return nil, ErrInvalidTransition
	}
	if _, errDominio := patch.Apply(*current, s.now().UTC()); errDominio != nil {
		return nil, mapUpdateDomainError(errDominio)
	}

	actualizada, errEscritura := s.store.UpdateGoal(ctx, UpdateWrite{
		UserID:          userID,
		GoalID:          goalID,
		Patch:           patch,
		ExpectedVersion: cmd.ExpectedVersion,
		Now:             s.now().UTC(),
	})
	if errEscritura != nil {
		return nil, normalizeStoreError(errEscritura)
	}
	if actualizada == nil {
		return nil, ErrInternal
	}
	if err := actualizada.Validate(); err != nil {
		return nil, ErrInconsistent
	}
	return actualizada, nil
}

// buildPatch convierte la entrada del transporte en el patch de dominio, validando el formato.
func buildPatch(cmd UpdateCommand) (domainGoal.Patch, error) {
	var patch domainGoal.Patch
	if cmd.Name != nil {
		clean, err := domainGoal.NormalizeName(*cmd.Name)
		if err != nil {
			return domainGoal.Patch{}, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
		}
		patch.Name = &clean
	}
	if cmd.TargetAmount != nil {
		if err := domainGoal.ValidateTargetAmountCents(*cmd.TargetAmount); err != nil {
			if err == domainGoal.ErrTargetNotPositive {
				return domainGoal.Patch{}, ErrTargetNotPositive
			}
			return domainGoal.Patch{}, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
		}
		target := *cmd.TargetAmount
		patch.TargetCents = &target
	}
	if cmd.TargetDate != nil {
		fecha, err := time.Parse(domainGoal.DateLayout, strings.TrimSpace(*cmd.TargetDate))
		if err != nil {
			return domainGoal.Patch{}, fmt.Errorf("%w: targetDate invalida", ErrInvalidRequest)
		}
		if err := domainGoal.ValidateTargetDate(fecha); err != nil {
			return domainGoal.Patch{}, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
		}
		patch.TargetDate = &fecha
	}
	if cmd.Status != nil {
		estado, err := domainGoal.ParseStatus(*cmd.Status)
		if err != nil {
			return domainGoal.Patch{}, fmt.Errorf("%w: status fuera del enum", ErrInvalidRequest)
		}
		patch.Status = &estado
	}
	// Un patch sin campos no es un error: el contrato GoalPatch no exige ninguno y el caso
	// de uso lo resuelve como no-op sin escritura (ver Update).
	return patch, nil
}

// mapUpdateDomainError traduce los invariantes de dominio. El orden importa: una transición no
// admitida y un objetivo bajo el ahorro tienen códigos y detalles distintos (ambos 422).
func mapUpdateDomainError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, domainGoal.ErrTargetBelowSaved):
		return ErrTargetBelowSaved
	case errors.Is(err, domainGoal.ErrTransitionInvalid):
		return ErrInvalidTransition
	case errors.Is(err, domainGoal.ErrTargetNotPositive):
		return ErrTargetNotPositive
	case errors.Is(err, domainGoal.ErrNameInvalid),
		errors.Is(err, domainGoal.ErrTargetDateInvalid),
		errors.Is(err, domainGoal.ErrStatusInvalid),
		errors.Is(err, domainGoal.ErrCurrencyInvalid),
		errors.Is(err, domainGoal.ErrAmountOutOfRange):
		return fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	case errors.Is(err, domainGoal.ErrEmptyPatch):
		return fmt.Errorf("%w: la modificacion no contiene cambios", ErrInvalidRequest)
	default:
		return ErrInconsistent
	}
}
