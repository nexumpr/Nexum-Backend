package goals

import (
	"context"
	"strings"

	domainGoal "github.com/nexumpr/Nexum-Backend/internal/domain/goal"
)

// Caso de uso de E040 (`GET /api/v1/goals/{goalId}`).
//
// La identidad llega exclusivamente de `sub` del JWT verificado. Una meta de otro titular y una
// meta inexistente producen el MISMO 404: el endpoint no es un oráculo de existencia de
// identificadores ajenos (E040 §1, §5 T03).

// Get devuelve la meta del titular.
func (s *Service) Get(ctx context.Context, userID, goalID string) (*domainGoal.Goal, error) {
	if s == nil || s.store == nil {
		return nil, ErrUnavailable
	}
	cleanUser := strings.TrimSpace(userID)
	if cleanUser == "" {
		return nil, ErrUnauthorized
	}
	cleanGoal := strings.TrimSpace(goalID)
	if cleanGoal == "" {
		return nil, ErrInvalidRequest
	}

	goal, err := s.store.LoadGoal(ctx, cleanUser, cleanGoal)
	if err != nil {
		return nil, normalizeStoreError(err)
	}
	if goal == nil {
		return nil, ErrNotFound
	}
	// El adaptador ya valida al mapear; esta comprobación es defensa en profundidad para que
	// ninguna proyección incoherente llegue al contrato.
	if err := goal.Validate(); err != nil {
		return nil, ErrInconsistent
	}
	return goal, nil
}
