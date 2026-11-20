package application_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	appBudget "github.com/nexumpr/Nexum-Backend/internal/application/budgetset"
)

type almacenPresupuestosEscritura struct {
	mu   sync.Mutex
	sets map[string]*appBudget.Set
}

func (a *almacenPresupuestosEscritura) LoadSet(_ context.Context, userID, period string) (*appBudget.Set, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if set, ok := a.sets[userID+"|"+period]; ok {
		copia := *set
		return &copia, nil
	}
	return nil, nil
}
func (a *almacenPresupuestosEscritura) ListAllSets(_ context.Context, userID string) ([]appBudget.Set, error) {
	return nil, nil
}
func (a *almacenPresupuestosEscritura) ReplaceSet(_ context.Context, userID string, set *appBudget.Set, expectedVersion int64) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	clave := userID + "|" + set.Period
	actual, existe := a.sets[clave]
	if expectedVersion == 0 {
		if existe {
			return appBudget.ErrVersionConflict
		}
	} else if !existe || actual.Version != expectedVersion {
		return appBudget.ErrVersionConflict
	}
	copia := *set
	a.sets[clave] = &copia
	return nil
}

func servicioPresupuestosEscritura(almacen *almacenPresupuestosEscritura) *appBudget.Service {
	return appBudget.NewService(almacen).
		WithCursorKey([]byte("0123456789abcdef0123456789abcdef")).
		WithClock(func() time.Time { return time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC) })
}

func TestE049CreacionReemplazoYPrecondiciones(t *testing.T) {
	almacen := &almacenPresupuestosEscritura{sets: map[string]*appBudget.Set{}}
	servicio := servicioPresupuestosEscritura(almacen)
	entrada := []appBudget.BudgetInput{{CategoryID: "comida", LimitAmount: "1000.00"}}

	if _, err := servicio.Replace(context.Background(), appBudget.ReplaceCommand{UserID: "u", Period: "2026-11", Currency: "MXN", Budgets: entrada}); !errors.Is(err, appBudget.ErrPreconditionRequired) {
		t.Fatalf("sin If-Match debe ser 428, obtenido %v", err)
	}
	if _, err := servicio.Replace(context.Background(), appBudget.ReplaceCommand{UserID: "u", IfMatch: `W/"b2026-11-0"`, Period: "2026-11", Currency: "MXN", Budgets: entrada}); !errors.Is(err, appBudget.ErrPreconditionInvalid) {
		t.Fatalf("validador debil debe ser 400, obtenido %v", err)
	}
	p1, err := servicio.Replace(context.Background(), appBudget.ReplaceCommand{UserID: "u", IfMatch: `"b2026-11-0"`, Period: "2026-11", Currency: "MXN", Budgets: entrada})
	if err != nil {
		t.Fatalf("creacion: %v", err)
	}
	if p1.ETag != `"b2026-11-1"` || len(p1.Items) != 1 || p1.Items[0].Version != 1 {
		t.Fatalf("creacion inesperada: %+v", p1)
	}
	p2, err := servicio.Replace(context.Background(), appBudget.ReplaceCommand{UserID: "u", IfMatch: `"b2026-11-1"`, Period: "2026-11", Currency: "MXN", Budgets: []appBudget.BudgetInput{{CategoryID: "transporte", LimitAmount: "500.00"}}})
	if err != nil || p2.ETag != `"b2026-11-2"` {
		t.Fatalf("reemplazo: %v/%+v", err, p2)
	}
	if _, err := servicio.Replace(context.Background(), appBudget.ReplaceCommand{UserID: "u", IfMatch: `"b2026-11-0"`, Period: "2026-11", Currency: "MXN", Budgets: entrada}); !errors.Is(err, appBudget.ErrVersionConflict) {
		t.Fatalf("etag obsoleto debe ser 412, obtenido %v", err)
	}
	if _, err := servicio.Replace(context.Background(), appBudget.ReplaceCommand{UserID: "u", IfMatch: `"b2026-10-1"`, Period: "2026-11", Currency: "MXN", Budgets: entrada}); !errors.Is(err, appBudget.ErrVersionConflict) {
		t.Fatalf("etag de otro periodo debe ser 412, obtenido %v", err)
	}
}

func TestE049ValidacionesYLimite50(t *testing.T) {
	almacen := &almacenPresupuestosEscritura{sets: map[string]*appBudget.Set{}}
	servicio := servicioPresupuestosEscritura(almacen)
	casos := []struct {
		nombre  string
		budgets []appBudget.BudgetInput
	}{
		{"duplicado", []appBudget.BudgetInput{{CategoryID: "comida", LimitAmount: "1.00"}, {CategoryID: "comida", LimitAmount: "2.00"}}},
		{"negativo", []appBudget.BudgetInput{{CategoryID: "comida", LimitAmount: "-1.00"}}},
		{"vacio", []appBudget.BudgetInput{{CategoryID: "", LimitAmount: "1.00"}}},
	}
	for _, caso := range casos {
		_, err := servicio.Replace(context.Background(), appBudget.ReplaceCommand{UserID: "u", IfMatch: `"b2026-11-0"`, Period: "2026-11", Currency: "MXN", Budgets: caso.budgets})
		var violacion *appBudget.Violation
		if !errors.As(err, &violacion) {
			t.Fatalf("%s: esperado 422, obtenido %v", caso.nombre, err)
		}
	}
	items := make([]appBudget.BudgetInput, 0, 50)
	for i := 0; i < 50; i++ {
		items = append(items, appBudget.BudgetInput{CategoryID: fmt.Sprintf("cat%02d", i), LimitAmount: "1.00"})
	}
	pagina, err := servicio.Replace(context.Background(), appBudget.ReplaceCommand{UserID: "u", IfMatch: `"b2026-11-0"`, Period: "2026-11", Currency: "MXN", Budgets: items})
	if err != nil || len(pagina.Items) != 50 {
		t.Fatalf("50 categorias: %v/%d", err, len(pagina.Items))
	}
}

func TestE049ConcurrenciaUnaSolaEscritura(t *testing.T) {
	almacen := &almacenPresupuestosEscritura{sets: map[string]*appBudget.Set{}}
	servicio := servicioPresupuestosEscritura(almacen)
	if _, err := servicio.Replace(context.Background(), appBudget.ReplaceCommand{UserID: "u", IfMatch: `"b2026-11-0"`, Period: "2026-11", Currency: "MXN", Budgets: []appBudget.BudgetInput{{CategoryID: "comida", LimitAmount: "1.00"}}}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	var wg sync.WaitGroup
	var ok, conflict int
	var mu sync.Mutex
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, err := servicio.Replace(context.Background(), appBudget.ReplaceCommand{
				UserID: "u", IfMatch: `"b2026-11-1"`, Period: "2026-11", Currency: "MXN",
				Budgets: []appBudget.BudgetInput{{CategoryID: fmt.Sprintf("cat%d", n), LimitAmount: "2.00"}},
			})
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				ok++
			} else if errors.Is(err, appBudget.ErrVersionConflict) {
				conflict++
			} else {
				t.Errorf("error inesperado: %v", err)
			}
		}(i)
	}
	wg.Wait()
	if ok != 1 || conflict != 1 {
		t.Fatalf("esperado 1 exito y 1 conflicto, obtenido %d/%d", ok, conflict)
	}
}
