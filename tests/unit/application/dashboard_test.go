package application_test

import (
	"context"
	"testing"
	"time"

	appDashboard "github.com/nexumpr/Nexum-Backend/internal/application/dashboard"
	appSummary "github.com/nexumpr/Nexum-Backend/internal/application/financialsummary"
	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
)

type resumenStoreFake struct{}

func (resumenStoreFake) LoadTimezone(context.Context, string) (string, error) { return "UTC", nil }
func (resumenStoreFake) AggregateMovements(context.Context, string, time.Time, time.Time) (appSummary.MovementTotals, error) {
	return appSummary.MovementTotals{IncomeCents: 100000, ExpensesCents: 20000, IndependentSavingsCents: 10000}, nil
}
func (resumenStoreFake) AggregateContributions(context.Context, string, time.Time, time.Time) (appSummary.ContributionTotals, error) {
	return appSummary.ContributionTotals{ActiveSavingsCents: 5000}, nil
}

type dashboardStoreFake struct {
	goals []appDashboard.GoalRow
	movs  []appMovements.Movement
}

func (d dashboardStoreFake) ListActiveGoals(context.Context, string, int) ([]appDashboard.GoalRow, error) {
	return d.goals, nil
}
func (d dashboardStoreFake) ListLatestMovements(context.Context, string, int) ([]appMovements.Movement, error) {
	return d.movs, nil
}

func TestE051DashboardRealSinCache(t *testing.T) {
	summary := appSummary.NewService(resumenStoreFake{}).WithClock(func() time.Time {
		return time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	})
	store := dashboardStoreFake{
		goals: []appDashboard.GoalRow{{
			ID: "018f90ef-08e5-7a68-b09f-2b67a47c92f1", Name: "Viaje",
			TargetCents: 100000, SavedCents: 25000, OpeningCents: 5000,
			Currency: "MXN", TargetDate: "2026-12-31", Status: "active", Version: 2,
		}},
		movs: []appMovements.Movement{{
			ID: "018f90ef-08e5-7a68-b09f-2b67a47c92f2", Type: "expense", AmountCents: 20000,
			Currency: "MXN", CategoryID: "comida", Description: "Comida", OccurredAt: time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC), Version: 1,
		}},
	}
	dashboard, err := appDashboard.NewService(summary, store).Get(context.Background(), "u", "2026-11")
	if err != nil {
		t.Fatalf("dashboard: %v", err)
	}
	if dashboard.Period != "2026-11" || dashboard.Summary.Income != "1000.00" || dashboard.Summary.Net != "650.00" {
		t.Fatalf("resumen inesperado: %+v", dashboard)
	}
	if len(dashboard.Goals) != 1 || dashboard.Goals[0].OpeningSaved != "50.00" || dashboard.Goals[0].Version != 2 {
		t.Fatalf("metas inesperadas: %+v", dashboard.Goals)
	}
	if len(dashboard.LatestMovements) != 1 || dashboard.LatestMovements[0].ID == "" {
		t.Fatalf("movimientos inesperados: %+v", dashboard.LatestMovements)
	}
	if dashboard.AsOf != "2026-11-19T12:00:00Z" {
		t.Fatalf("asOf inesperado: %q", dashboard.AsOf)
	}
}

func TestE051SinUsuario(t *testing.T) {
	summary := appSummary.NewService(resumenStoreFake{})
	if _, err := appDashboard.NewService(summary, dashboardStoreFake{}).Get(context.Background(), "", "2026-11"); err == nil {
		t.Fatal("sin usuario debe fallar")
	}
}
