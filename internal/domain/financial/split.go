package financial

import (
	"errors"
	"sort"
)

// GoalTarget representa una meta con su estado actual de ahorro y su objetivo (A-070 §2.4, §2.5).
type GoalTarget struct {
	GoalID            string
	CurrentSavedCents int64
	TargetCents       int64
}

// RemainingCents calcula el monto que falta para completar la meta.
func (g GoalTarget) RemainingCents() int64 {
	rem := g.TargetCents - g.CurrentSavedCents
	if rem < 0 {
		return 0
	}
	return rem
}

// SplitResult representa el resultado de la distribución de aportación a metas.
type SplitResult struct {
	Allocations        map[string]int64
	UndistributedCents int64
}

// DistributeEquitably divide un monto en centavos de forma equitativa entre las metas provistas (A-070 §2.5, AC-A-070-01).
// Reglas:
//  1. Ordenamiento determinista estable por GoalID para evitar diferencias entre nodos.
//  2. Manejo de fracciones indivisibles de centavos: los centavos sobrantes (ej: 2 centavos entre 3 metas)
//     se asignan de a 1 centavo a las primeras metas según el orden estable.
//  3. Saturación de metas: ninguna meta recibe más de su RemainingCents. El excedente se redistribuye
//     entre las metas restantes que aún tengan capacidad.
//  4. Si todas las metas alcanzan su target y aún sobra monto, el sobrante se reporta en UndistributedCents.
func DistributeEquitably(totalCents int64, goals []GoalTarget) (*SplitResult, error) {
	if totalCents < 0 {
		return nil, errors.New("el monto a distribuir no puede ser negativo")
	}
	if len(goals) == 0 {
		return &SplitResult{
			Allocations:        make(map[string]int64),
			UndistributedCents: totalCents,
		}, nil
	}

	// Ordenamiento determinista estable por GoalID
	sortedGoals := make([]GoalTarget, len(goals))
	copy(sortedGoals, goals)
	sort.Slice(sortedGoals, func(i, j int) bool {
		return sortedGoals[i].GoalID < sortedGoals[j].GoalID
	})

	allocations := make(map[string]int64, len(sortedGoals))
	for _, g := range sortedGoals {
		allocations[g.GoalID] = 0
	}

	remainingToDistribute := totalCents

	// Iterar mientras haya monto disponible y metas con capacidad restante
	for remainingToDistribute > 0 {
		// Filtrar metas que aún tienen capacidad disponible
		var eligible []*GoalTarget
		for i := range sortedGoals {
			currentAlloc := allocations[sortedGoals[i].GoalID]
			if sortedGoals[i].RemainingCents()-currentAlloc > 0 {
				eligible = append(eligible, &sortedGoals[i])
			}
		}

		if len(eligible) == 0 {
			// Todas las metas alcanzaron su target
			break
		}

		numEligible := int64(len(eligible))
		if remainingToDistribute < numEligible {
			// Menos centavos que metas disponibles (ej: 1 o 2 centavos entre 3 metas, AC-A-070-01)
			// Asignar 1 centavo a cada una en orden hasta agotar
			for i := 0; i < int(remainingToDistribute); i++ {
				g := eligible[i]
				allocations[g.GoalID]++
			}
			remainingToDistribute = 0
			break
		}

		// Distribución base equitativa
		baseShare := remainingToDistribute / numEligible
		remainder := remainingToDistribute % numEligible

		// Comprobar si alguna meta se satura con su baseShare
		anySaturated := false
		for i, g := range eligible {
			currentAlloc := allocations[g.GoalID]
			capacity := g.RemainingCents() - currentAlloc
			share := baseShare
			if int64(i) < remainder {
				share++
			}

			if capacity <= share {
				// Meta se satura completamente
				allocations[g.GoalID] += capacity
				remainingToDistribute -= capacity
				anySaturated = true
				break // Reiniciar ronda con las metas restantes para redistribuir el sobrante
			}
		}

		// Si ninguna meta se saturó en esta ronda, asignar todas las partes
		if !anySaturated {
			for i, g := range eligible {
				share := baseShare
				if int64(i) < remainder {
					share++
				}
				allocations[g.GoalID] += share
				remainingToDistribute -= share
			}
		}
	}

	return &SplitResult{
		Allocations:        allocations,
		UndistributedCents: remainingToDistribute,
	}, nil
}
