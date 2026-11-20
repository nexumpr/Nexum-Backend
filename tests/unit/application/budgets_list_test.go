package application_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	appBudget "github.com/nexumpr/Nexum-Backend/internal/application/budgetset"
)

type almacenPresupuestos struct {
	sets map[string]*appBudget.Set
	err  error
}

func (a *almacenPresupuestos) LoadSet(_ context.Context, userID, period string) (*appBudget.Set, error) {
	if a.err != nil {
		return nil, a.err
	}
	if set, ok := a.sets[userID+"|"+period]; ok {
		copia := *set
		return &copia, nil
	}
	return nil, nil
}

func (a *almacenPresupuestos) ListAllSets(_ context.Context, userID string) ([]appBudget.Set, error) {
	if a.err != nil {
		return nil, a.err
	}
	var salida []appBudget.Set
	for clave, set := range a.sets {
		if len(clave) > len(userID) && clave[:len(userID)] == userID {
			salida = append(salida, *set)
		}
	}
	return salida, nil
}

func servicioPresupuestos(almacen *almacenPresupuestos) *appBudget.Service {
	return appBudget.NewService(almacen).
		WithCursorKey([]byte("0123456789abcdef0123456789abcdef")).
		WithClock(func() time.Time { return time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC) })
}

func TestE048ListadoPaginadoConETag(t *testing.T) {
	items := make([]appBudget.Budget, 0, 25)
	for i := 0; i < 25; i++ {
		items = append(items, appBudget.Budget{CategoryID: fmt.Sprintf("cat%02d", i), LimitAmountCents: int64(10000 + i)})
	}
	almacen := &almacenPresupuestos{sets: map[string]*appBudget.Set{
		"u|2026-11": {Period: "2026-11", Currency: "MXN", Version: 3, Items: items},
	}}
	servicio := servicioPresupuestos(almacen)
	p1, err := servicio.List(context.Background(), "u", "2026-11", "", 10)
	if err != nil {
		t.Fatalf("pagina 1: %v", err)
	}
	if len(p1.Items) != 10 || p1.NextCursor == "" || p1.ETag != `"b2026-11-3"` {
		t.Fatalf("pagina 1 inesperada: %d/%q/%q", len(p1.Items), p1.NextCursor, p1.ETag)
	}
	p2, err := servicio.List(context.Background(), "u", "2026-11", p1.NextCursor, 10)
	if err != nil || len(p2.Items) != 10 {
		t.Fatalf("pagina 2: %v/%d", err, len(p2.Items))
	}
	p3, err := servicio.List(context.Background(), "u", "2026-11", p2.NextCursor, 10)
	if err != nil || len(p3.Items) != 5 || p3.NextCursor != "" {
		t.Fatalf("pagina 3: %v/%d/%q", err, len(p3.Items), p3.NextCursor)
	}
}

func TestE048PeriodoVacioYErrores(t *testing.T) {
	almacen := &almacenPresupuestos{sets: map[string]*appBudget.Set{}}
	servicio := servicioPresupuestos(almacen)
	p, err := servicio.List(context.Background(), "u", "2026-11", "", 25)
	if err != nil {
		t.Fatalf("periodo vacio: %v", err)
	}
	if len(p.Items) != 0 || p.ETag != `"b2026-11-0"` || p.NextCursor != "" {
		t.Fatalf("periodo vacio inesperado: %+v", p)
	}
	for _, periodo := range []string{"2026-13", "2026-1", "202611"} {
		if _, err := servicio.List(context.Background(), "u", periodo, "", 25); !errors.Is(err, appBudget.ErrInvalidRequest) {
			t.Fatalf("periodo %q debe ser 400, obtenido %v", periodo, err)
		}
	}
	if _, err := servicio.List(context.Background(), "u", "2026-11", "cursor-invalido", 25); !errors.Is(err, appBudget.ErrInvalidRequest) {
		t.Fatalf("cursor invalido debe ser 400, obtenido %v", err)
	}
}

func TestE048SinPeriodoListaTodos(t *testing.T) {
	almacen := &almacenPresupuestos{sets: map[string]*appBudget.Set{
		"u|2026-10": {Period: "2026-10", Currency: "MXN", Version: 1, Items: []appBudget.Budget{{CategoryID: "comida", LimitAmountCents: 1000}}},
		"u|2026-11": {Period: "2026-11", Currency: "MXN", Version: 2, Items: []appBudget.Budget{{CategoryID: "transporte", LimitAmountCents: 2000}}},
	}}
	servicio := servicioPresupuestos(almacen)
	p, err := servicio.List(context.Background(), "u", "", "", 25)
	if err != nil {
		t.Fatalf("sin periodo: %v", err)
	}
	if len(p.Items) != 2 || p.Items[0].Period != "2026-10" || p.Items[1].Period != "2026-11" {
		t.Fatalf("items inesperados: %+v", p.Items)
	}
	if p.ETag != "" {
		t.Fatalf("sin periodo no debe emitir ETag: %q", p.ETag)
	}
}
