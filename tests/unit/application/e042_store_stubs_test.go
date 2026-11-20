package application_test

import (
	"context"

	appGoals "github.com/nexumpr/Nexum-Backend/internal/application/goals"
)

// Stubs de E042 para los dobles de pruebas existentes. No participan en las pruebas de
// E038/E039/E041; solo completan el puerto ampliado.
func (s *e038Store) DeleteGoal(context.Context, appGoals.DeleteWrite) error {
	return appGoals.ErrUnavailable
}

func (s *e039Store) DeleteGoal(context.Context, appGoals.DeleteWrite) error {
	return appGoals.ErrUnavailable
}

func (s e041StoreFallo) DeleteGoal(context.Context, appGoals.DeleteWrite) error {
	return appGoals.ErrUnavailable
}
