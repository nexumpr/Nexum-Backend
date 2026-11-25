package bootstrap

import (
	"context"
	"errors"
	"testing"
	"time"

	appSummary "github.com/nexumpr/Nexum-Backend/internal/application/financialsummary"
	appProfile "github.com/nexumpr/Nexum-Backend/internal/application/profile"
)

type fakeProfiles struct {
	porID map[string]*appProfile.Profile
	err   error
}

func (f *fakeProfiles) Get(_ context.Context, userID string) (*appProfile.Profile, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.porID[userID], nil
}

type fakeSummaries struct {
	porID map[string]*appSummary.Summary
	err   error
}

func (f *fakeSummaries) Summary(_ context.Context, userID, _ string) (*appSummary.Summary, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.porID[userID], nil
}

func servicioDePrueba() (*Service, *fakeProfiles, *fakeSummaries) {
	perfiles := &fakeProfiles{porID: map[string]*appProfile.Profile{
		"user-a": {
			ID: "user-a", Email: "a@example.invalid", DisplayName: "Usuario A", EmailVerified: true,
			CreatedAt: time.Date(2026, 11, 5, 17, 0, 0, 0, time.UTC), Version: 3,
		},
	}}
	resumenes := &fakeSummaries{porID: map[string]*appSummary.Summary{
		"user-a": {
			Period: "2026-11", Currency: "MXN", Income: 500000, Expenses: 100000,
			Savings: 50000, Net: 350000, AsOf: time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC),
		},
	}}
	return NewService(perfiles, resumenes, "2026-11"), perfiles, resumenes
}

func TestLoadAgregaSoloLosDatosDelTitular(t *testing.T) {
	svc, _, _ := servicioDePrueba()
	wire, err := svc.Load(context.Background(), "user-a")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if wire.Profile.ID != "user-a" || wire.Profile.Version != 3 || wire.Profile.CreatedAt != "2026-11-05T17:00:00Z" {
		t.Fatalf("perfil incorrecto: %+v", wire.Profile)
	}
	if wire.FinancialSummary.Income != "5000.00" || wire.FinancialSummary.Net != "3500.00" {
		t.Fatalf("resumen incorrecto: %+v", wire.FinancialSummary)
	}
	if wire.CatalogVersion != "2026-11" {
		t.Fatalf("catalogVersion: %q", wire.CatalogVersion)
	}
}

func TestLoadErroresClasificados(t *testing.T) {
	svc, perfiles, resumenes := servicioDePrueba()

	if _, err := svc.Load(context.Background(), ""); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("sin usuario: %v", err)
	}
	perfiles.err = appProfile.ErrProfileNotFound
	if _, err := svc.Load(context.Background(), "user-a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("perfil ausente: %v", err)
	}
	perfiles.err = appProfile.ErrProfileUnavailable
	if _, err := svc.Load(context.Background(), "user-a"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("DynamoDB caído: %v", err)
	}
	perfiles.err = nil
	resumenes.err = appSummary.ErrUnavailable
	if _, err := svc.Load(context.Background(), "user-a"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("resumen no disponible: %v", err)
	}
	if NewService(nil, nil, "").Disponible() {
		t.Fatal("un servicio sin repositorios no debe estar disponible")
	}
}
