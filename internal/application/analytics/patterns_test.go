package analytics

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
)

func e054Movimiento(categoria string, dia int, cents int64, tipo string) appMovements.Movement {
	return appMovements.Movement{
		ID:          "e054-" + categoria + "-" + time.Duration(dia).String(),
		Type:        tipo,
		AmountCents: cents,
		Currency:    "MXN",
		CategoryID:  categoria,
		OccurredAt:  time.Date(2026, 11, dia, 12, 0, 0, 0, time.UTC),
	}
}

func e054Servicio(fuente *e053PaginatedSource) *Service {
	return NewService(fuente).WithClock(func() time.Time {
		return time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	})
}

func TestE054PatronConMuestraReal(t *testing.T) {
	fuente := &e053PaginatedSource{pages: map[string][][]appMovements.Movement{
		"user-a": {{
			e054Movimiento("food", 2, 1000, appMovements.TypeExpense),
			e054Movimiento("food", 9, 2000, appMovements.TypeExpense),
			e054Movimiento("food", 16, 3000, appMovements.TypeExpense),
			e054Movimiento("food", 23, 4000, appMovements.TypeExpense),
		}},
	}}
	pagina, err := e054Servicio(fuente).Patterns(context.Background(), "user-a", "2026-11")
	if err != nil {
		t.Fatalf("patrones: %v", err)
	}
	if len(pagina.Items) != 1 {
		t.Fatalf("esperado 1 patron, obtenidos %d", len(pagina.Items))
	}
	item := pagina.Items[0]
	if item.CategoryID != "food" || item.SampleSize != 4 || item.Period != "2026-11" {
		t.Fatalf("patron incorrecto: %+v", item)
	}
	if !strings.Contains(item.Description, "4 movimientos en 4 dias") {
		t.Fatalf("la descripcion debe citar la muestra real: %q", item.Description)
	}
	if pagina.NextCursor != "" {
		t.Fatalf("E054 no declara cursor de entrada: %q", pagina.NextCursor)
	}
	wire := pagina.Wire()
	if len(wire.Items) != 1 || wire.Items[0].SampleSize != 4 {
		t.Fatalf("proyeccion Wire incorrecta: %+v", wire.Items)
	}
}

func TestE054SinPatronSinMuestraSuficiente(t *testing.T) {
	ctx := context.Background()
	pocos := &e053PaginatedSource{pages: map[string][][]appMovements.Movement{
		"user-a": {{
			e054Movimiento("food", 2, 1000, appMovements.TypeExpense),
			e054Movimiento("food", 9, 1000, appMovements.TypeExpense),
		}},
	}}
	pagina, err := e054Servicio(pocos).Patterns(ctx, "user-a", "2026-11")
	if err != nil {
		t.Fatal(err)
	}
	if len(pagina.Items) != 0 {
		t.Fatalf("2 muestras no bastan: %+v", pagina.Items)
	}

	// Tres cargos el mismo dia no son cadencia: no hay fechas distintas observadas.
	mismoDia := &e053PaginatedSource{pages: map[string][][]appMovements.Movement{
		"user-a": {{
			e054Movimiento("food", 2, 1000, appMovements.TypeExpense),
			e054Movimiento("food", 2, 1000, appMovements.TypeExpense),
			e054Movimiento("food", 2, 1000, appMovements.TypeExpense),
		}},
	}}
	pagina, err = e054Servicio(mismoDia).Patterns(ctx, "user-a", "2026-11")
	if err != nil {
		t.Fatal(err)
	}
	if len(pagina.Items) != 0 {
		t.Fatalf("un solo dia no es patron de cadencia: %+v", pagina.Items)
	}
}

func TestE054IgnoraIngresosReversasYCategoriasVacias(t *testing.T) {
	fuente := &e053PaginatedSource{pages: map[string][][]appMovements.Movement{
		"user-a": {{
			e054Movimiento("nomina", 2, 500000, appMovements.TypeIncome),
			e054Movimiento("nomina", 9, 500000, appMovements.TypeIncome),
			e054Movimiento("nomina", 16, 500000, appMovements.TypeIncome),
			e054Movimiento("reversa", 3, -1000, appMovements.TypeExpense),
			e054Movimiento("reversa", 10, -1000, appMovements.TypeExpense),
			e054Movimiento("reversa", 17, -1000, appMovements.TypeExpense),
		}},
	}}
	pagina, err := e054Servicio(fuente).Patterns(context.Background(), "user-a", "2026-11")
	if err != nil {
		t.Fatal(err)
	}
	if len(pagina.Items) != 0 {
		t.Fatalf("ingresos y reversas no pueden generar patrones de gasto: %+v", pagina.Items)
	}
}

func TestE054OrdenDeterministaYAislamiento(t *testing.T) {
	fuente := &e053PaginatedSource{pages: map[string][][]appMovements.Movement{
		"user-a": {{
			e054Movimiento("transport", 1, 100, appMovements.TypeExpense),
			e054Movimiento("transport", 8, 100, appMovements.TypeExpense),
			e054Movimiento("transport", 15, 100, appMovements.TypeExpense),
			e054Movimiento("food", 2, 100, appMovements.TypeExpense),
			e054Movimiento("food", 9, 100, appMovements.TypeExpense),
			e054Movimiento("food", 16, 100, appMovements.TypeExpense),
			e054Movimiento("food", 23, 100, appMovements.TypeExpense),
		}},
		"user-b": {{e054Movimiento("food", 5, 100, appMovements.TypeExpense)}},
	}}
	servicio := e054Servicio(fuente)
	a, err := servicio.Patterns(context.Background(), "user-a", "2026-11")
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Items) != 2 || a.Items[0].CategoryID != "food" || a.Items[1].CategoryID != "transport" {
		t.Fatalf("orden por muestra real roto: %+v", a.Items)
	}
	b, err := servicio.Patterns(context.Background(), "user-b", "2026-11")
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Items) != 0 {
		t.Fatalf("user B recibio datos de A: %+v", b.Items)
	}
	otro, err := servicio.Patterns(context.Background(), "user-a", "2026-10")
	if err != nil {
		t.Fatal(err)
	}
	if len(otro.Items) != 0 {
		t.Fatalf("periodo sin datos devolvio otro mes: %+v", otro.Items)
	}
}

func TestE054Negativas(t *testing.T) {
	ctx := context.Background()
	fuente := &e053PaginatedSource{pages: map[string][][]appMovements.Movement{
		"user-a": {{e054Movimiento("food", 2, 100, appMovements.TypeExpense)}},
	}}
	servicio := e054Servicio(fuente)

	if _, err := servicio.Patterns(ctx, "user-a", "2026-12-31"); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("periodo invalido: esperado ErrInvalidRequest, obtenido %v", err)
	}
	if _, err := servicio.Patterns(ctx, "", "2026-11"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("sin titular: esperado ErrUnauthorized, obtenido %v", err)
	}
	if _, err := NewService(nil).Patterns(ctx, "user-a", "2026-11"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("sin fuente: esperado ErrUnavailable, obtenido %v", err)
	}
	caido := e054Servicio(&e053PaginatedSource{err: appMovements.ErrUnavailable})
	if _, err := caido.Patterns(ctx, "user-a", "2026-11"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Dynamo caido: esperado ErrUnavailable, obtenido %v", err)
	}
}
