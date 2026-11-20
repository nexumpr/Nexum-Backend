package transport_test

import (
	"context"

	appGoals "github.com/nexumpr/Nexum-Backend/internal/application/goals"
)

func (s *e039HTTPStore) DeleteGoal(context.Context, appGoals.DeleteWrite) error {
	return appGoals.ErrUnavailable
}

func (s *e040HTTPStore) DeleteGoal(context.Context, appGoals.DeleteWrite) error {
	return appGoals.ErrUnavailable
}

func (s e041HTTPStoreFallo) DeleteGoal(context.Context, appGoals.DeleteWrite) error {
	return appGoals.ErrUnavailable
}
