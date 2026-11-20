package application_test

import (
	"context"
	"errors"
	"testing"

	appGoals "github.com/nexumpr/Nexum-Backend/internal/application/goals"
	domainGoal "github.com/nexumpr/Nexum-Backend/internal/domain/goal"
	"github.com/nexumpr/Nexum-Backend/internal/domain/money"
)

type fakeDeleteStore struct {
	appGoals.Store
	goal    *domainGoal.Goal
	err     error
	deleted bool
}

func (f *fakeDeleteStore) LoadGoal(context.Context, string, string) (*domainGoal.Goal, error) {
	if f.goal == nil {
		return nil, nil
	}
	copia := *f.goal
	return &copia, nil
}

func (f *fakeDeleteStore) DeleteGoal(_ context.Context, _ appGoals.DeleteWrite) error {
	f.deleted = true
	return f.err
}

func metaBorrado(active int64, deleted bool) *domainGoal.Goal {
	zero := money.MustNew(0, "MXN")
	return &domainGoal.Goal{
		ID: "018f90ef-08e5-7a68-b09f-2b67a47c92f1", UserID: "u", Name: "Meta",
		TargetAmount: money.MustNew(100000, "MXN"), SavedAmount: zero, OpeningSaved: zero,
		Status: domainGoal.StatusActive, Version: 1, ActiveContributions: active, Deleted: deleted,
	}
}

func TestE042DeleteAlternativaB(t *testing.T) {
	store := &fakeDeleteStore{goal: metaBorrado(0, false)}
	servicio := appGoals.NewService(store)
	if err := servicio.Delete(context.Background(), "u", store.goal.ID); err != nil {
		t.Fatalf("borrado valido: %v", err)
	}
	if !store.deleted {
		t.Fatal("debe escribir el tombstone")
	}

	store = &fakeDeleteStore{goal: metaBorrado(1, false)}
	if err := appGoals.NewService(store).Delete(context.Background(), "u", store.goal.ID); !errors.Is(err, appGoals.ErrGoalActiveContributions) {
		t.Fatalf("con aportaciones debe ser 409, obtenido %v", err)
	}
	if store.deleted {
		t.Fatal("un 409 no puede escribir")
	}

	store = &fakeDeleteStore{goal: metaBorrado(0, true)}
	if err := appGoals.NewService(store).Delete(context.Background(), "u", store.goal.ID); err != nil {
		t.Fatalf("segundo DELETE debe ser idempotente: %v", err)
	}
	if store.deleted {
		t.Fatal("un tombstone existente no se reescribe")
	}

	store = &fakeDeleteStore{}
	if err := appGoals.NewService(store).Delete(context.Background(), "u", "018f90ef-08e5-7a68-b09f-2b67a47c92f1"); !errors.Is(err, appGoals.ErrNotFound) {
		t.Fatalf("meta inexistente debe ser 404, obtenido %v", err)
	}
}
