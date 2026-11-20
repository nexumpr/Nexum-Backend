package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	appBudget "github.com/nexumpr/Nexum-Backend/internal/application/budgetset"
	appUsage "github.com/nexumpr/Nexum-Backend/internal/application/budgetusage"
)

type almacenUso struct {
	set      *appBudget.Set
	gastos   map[string]int64
	errZona  error
	errGasto error
}

func (a *almacenUso) LoadTimezone(context.Context, string) (string, error) { return "UTC", a.errZona }
func (a *almacenUso) LoadSet(context.Context, string, string) (*appBudget.Set, error) {
	if a.set == nil {
		return nil, nil
	}
	copia := *a.set
	return &copia, nil
}
func (a *almacenUso) AggregateExpenses(context.Context, string, time.Time, time.Time) (map[string]int64, error) {
	if a.errGasto != nil {
		return nil, a.errGasto
	}
	return a.gastos, nil
}

func servicioUso(almacen *almacenUso) *appUsage.Service {
	return appUsage.NewService(almacen).WithClock(func() time.Time {
		return time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	})
}

func TestE050UsoDesdeMovimientosReales(t *testing.T) {
	almacen := &almacenUso{
		set: &appBudget.Set{Period: "2026-11", Currency: "MXN", Version: 1, Items: []appBudget.Budget{
			{CategoryID: "comida", LimitAmountCents: 100000},
			{CategoryID: "transporte", LimitAmountCents: 50000},
		}},
		gastos: map[string]int64{"comida": 25000},
	}
	items, err := servicioUso(almacen).Usage(context.Background(), "u", "2026-11")
	if err != nil {
		t.Fatalf("usage: %v", err)
	}
	if len(items) != 2 || items[0].CategoryID != "comida" || items[0].UsedCents != 25000 || items[0].LimitCents != 100000 {
		t.Fatalf("items inesperados: %+v", items)
	}
	if items[1].CategoryID != "transporte" || items[1].UsedCents != 0 {
		t.Fatalf("categoria sin gasto inesperada: %+v", items[1])
	}
}

func TestE050SinPresupuestoYErrores(t *testing.T) {
	items, err := servicioUso(&almacenUso{}).Usage(context.Background(), "u", "2026-11")
	if err != nil || len(items) != 0 {
		t.Fatalf("sin presupuesto: %v/%d", err, len(items))
	}
	if _, err := servicioUso(&almacenUso{}).Usage(context.Background(), "u", "2026-13"); !errors.Is(err, appUsage.ErrInvalidPeriod) {
		t.Fatalf("periodo invalido: %v", err)
	}
	if _, err := servicioUso(&almacenUso{errZona: appUsage.ErrUnavailable}).Usage(context.Background(), "u", "2026-11"); !errors.Is(err, appUsage.ErrUnavailable) {
		t.Fatalf("error de preferencias: %v", err)
	}
}
