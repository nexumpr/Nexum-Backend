package goals

import (
	"context"
	"fmt"
	"strings"
)

// Caso de uso de E042 (`DELETE /api/v1/goals/{goalId}`).
//
// Semántica (RFC-022 §6, alternativa B):
//   - La identidad llega exclusivamente de `sub` del JWT verificado; una meta de otro titular y
//     una inexistente producen el MISMO 404.
//   - Con aportaciones vigentes (`activeContributions > 0`) se responde 409 y NO se modifica
//     nada: una aportación puede repartirse entre varias metas y el contrato `Contribution`
//     tiene un único `status`, de modo que revertirla aquí afectaría a metas ajenas a esta
//     operación. El cliente revierte primero con E047 y después repite el borrado.
//   - Sin aportaciones vigentes se escribe un tombstone atómico —`deleted=true`,
//     `status=archived`, `savedAmount=openingSaved`, `version+1`— y se responde 204 sin cuerpo.
//     El item NO se elimina: se preserva el historial económico (GOV-07, R-030).
//   - Repetir el borrado sobre un tombstone es idempotente (204 sin escritura nueva): el
//     cliente puede reintentar sin duplicar efectos.
//   - E040/E041 tratan `deleted=true` como inexistente y E038/E051 la excluyen de página y
//     totales; el invariante se aplica en el adaptador y aquí, no en el cliente.

// Delete elimina lógicamente una meta del titular.
//
// Secuencia: validar entrada → leer la meta con consistencia fuerte (404 homogéneo) → salir sin
// escribir si ya está borrada → rechazar con 409 si tiene aportaciones vigentes → tombstone con
// compare-and-swap en el adaptador. Un fallo de persistencia se propaga como ErrUnavailable y
// nunca se presenta como 204: no se confirma un borrado que no se escribió.
func (s *Service) Delete(ctx context.Context, userID, goalID string) error {
	if s == nil || s.store == nil || s.now == nil {
		return ErrUnavailable
	}
	cleanUser := strings.TrimSpace(userID)
	if cleanUser == "" {
		return ErrUnauthorized
	}
	cleanGoal := strings.TrimSpace(goalID)
	if cleanGoal == "" {
		return fmt.Errorf("%w: goalId requerido", ErrInvalidRequest)
	}

	current, errLectura := s.store.LoadGoal(ctx, cleanUser, cleanGoal)
	if errLectura != nil {
		return normalizeStoreError(errLectura)
	}
	if current == nil {
		return ErrNotFound
	}
	if current.Deleted {
		// Segundo DELETE: ya no hay nada que escribir y el resultado es el mismo 204.
		return nil
	}
	if current.ActiveContributions > 0 {
		// Sin escritura: ni la meta ni sus aportaciones se tocan.
		return ErrGoalActiveContributions
	}

	if errEscritura := s.store.DeleteGoal(ctx, DeleteWrite{
		UserID: cleanUser,
		GoalID: cleanGoal,
		Now:    s.now().UTC(),
	}); errEscritura != nil {
		return normalizeStoreError(errEscritura)
	}
	return nil
}
