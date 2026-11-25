package analytics

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
)

// e053PaginatedSource simula varias páginas REALES de una Query: el cursor entrante es
// el índice de la página siguiente. Permite comprobar que E053 agrega todas las páginas
// antes de calcular porcentajes y que nunca recurre a un Scan.
type e053PaginatedSource struct {
	pages    map[string][][]appMovements.Movement
	err      error
	llamadas int
}

func (f *e053PaginatedSource) ListMovements(_ context.Context, q appMovements.ListQuery, startSK string, _ int) ([]appMovements.Movement, string, error) {
	if f.err != nil {
		return nil, "", f.err
	}
	f.llamadas++
	indice := 0
	if startSK != "" {
		var err error
		indice, err = strconv.Atoi(startSK)
		if err != nil || indice < 1 {
			return nil, "", errors.New("cursor de prueba invalido")
		}
	}
	paginas := f.pages[q.UserID]
	if indice >= len(paginas) {
		return nil, "", nil
	}
	siguiente := ""
	if indice+1 < len(paginas) {
		siguiente = strconv.Itoa(indice + 1)
	}
	// El almacén real aplica las cotas de fecha en la propia clave de ordenación;
	// el fake las respeta para que la prueba de periodo sea significativa.
	filtradas := make([]appMovements.Movement, 0, len(paginas[indice]))
	for _, movimiento := range paginas[indice] {
		if q.HasFrom && movimiento.OccurredAt.Before(q.From) {
			continue
		}
		if q.HasTo && !movimiento.OccurredAt.Before(q.To.Add(time.Millisecond)) {
			continue
		}
		filtradas = append(filtradas, movimiento)
	}
	return filtradas, siguiente, nil
}

func e053Movimiento(categoria string, cents int64, tipo string) appMovements.Movement {
	return appMovements.Movement{
		ID:          "e053-" + categoria + "-" + strconv.FormatInt(cents, 10),
		Type:        tipo,
		AmountCents: cents,
		Currency:    "MXN",
		CategoryID:  categoria,
		OccurredAt:  time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC),
	}
}

func e053Servicio(fuente *e053PaginatedSource) *Service {
	return NewService(fuente).WithClock(func() time.Time {
		return time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	})
}

func TestE053PorcentajesExactosYSuma100(t *testing.T) {
	fuente := &e053PaginatedSource{pages: map[string][][]appMovements.Movement{
		"user-a": {
			{e053Movimiento("food", 5000, appMovements.TypeExpense)},
			{
				e053Movimiento("transport", 3000, appMovements.TypeExpense),
				e053Movimiento("home", 2000, appMovements.TypeExpense),
			},
		},
	}}
	pagina, err := e053Servicio(fuente).Categories(context.Background(), "user-a", "2026-11")
	if err != nil {
		t.Fatalf("categorias: %v", err)
	}
	if fuente.llamadas != 2 {
		t.Fatalf("esperadas 2 paginas Query, obtenidas %d", fuente.llamadas)
	}
	if len(pagina.Items) != 3 {
		t.Fatalf("esperadas 3 categorias, obtenidas %d", len(pagina.Items))
	}
	esperados := map[string]struct {
		amount     string
		percentage int
	}{
		"food":      {"50.00", 50},
		"transport": {"30.00", 30},
		"home":      {"20.00", 20},
	}
	suma := 0
	for _, item := range pagina.Items {
		esperado, ok := esperados[item.CategoryID]
		if !ok {
			t.Fatalf("categoria inesperada %q", item.CategoryID)
		}
		if FormatoCents(item.AmountCents) != esperado.amount || item.Percentage != esperado.percentage {
			t.Fatalf("%s: esperado %s/%d, obtenido %s/%d", item.CategoryID, esperado.amount,
				esperado.percentage, FormatoCents(item.AmountCents), item.Percentage)
		}
		if item.Currency != "MXN" || item.Period != "2026-11" {
			t.Fatalf("campos de contrato incorrectos: %+v", item)
		}
		suma += item.Percentage
	}
	if suma != 100 {
		t.Fatalf("los porcentajes deben sumar 100, suman %d", suma)
	}
	if pagina.NextCursor != "" {
		t.Fatalf("E053 no declara cursor de entrada: se encontro %q", pagina.NextCursor)
	}
	wire := pagina.Wire()
	if len(wire.Items) != 3 || wire.Items[0].Amount != "50.00" || wire.Items[0].Currency != "MXN" {
		t.Fatalf("proyeccion Wire incorrecta: %+v", wire.Items)
	}
}

func TestE053MayorRestoDeterminista(t *testing.T) {
	fuente := &e053PaginatedSource{pages: map[string][][]appMovements.Movement{
		"user-a": {{
			e053Movimiento("transport", 100, appMovements.TypeExpense),
			e053Movimiento("food", 100, appMovements.TypeExpense),
			e053Movimiento("home", 100, appMovements.TypeExpense),
		}},
	}}
	primero, err := e053Servicio(fuente).Categories(context.Background(), "user-a", "2026-11")
	if err != nil {
		t.Fatal(err)
	}
	suma := 0
	porCategoria := map[string]int{}
	for _, item := range primero.Items {
		suma += item.Percentage
		porCategoria[item.CategoryID] = item.Percentage
	}
	if suma != 100 {
		t.Fatalf("suma de porcentajes %d, esperada 100", suma)
	}
	if porCategoria["food"] != 34 || porCategoria["home"] != 33 || porCategoria["transport"] != 33 {
		t.Fatalf("reparto por mayor resto incorrecto: %+v", porCategoria)
	}
	if primero.Items[0].CategoryID != "food" {
		t.Fatalf("orden determinista por centavos/categoria roto: %+v", primero.Items)
	}
}

func TestE053DatosCeroYSoloIngresos(t *testing.T) {
	ctx := context.Background()
	vacio, err := e053Servicio(&e053PaginatedSource{}).Categories(ctx, "user-a", "2026-11")
	if err != nil {
		t.Fatalf("sin datos: %v", err)
	}
	if len(vacio.Items) != 0 || vacio.Wire().Items == nil {
		t.Fatalf("sin datos debe dar items=[]: %+v", vacio)
	}

	soloIngresos := &e053PaginatedSource{pages: map[string][][]appMovements.Movement{
		"user-a": {{e053Movimiento("nomina", 900000, appMovements.TypeIncome)}},
	}}
	pagina, err := e053Servicio(soloIngresos).Categories(ctx, "user-a", "2026-11")
	if err != nil {
		t.Fatalf("solo ingresos: %v", err)
	}
	if len(pagina.Items) != 0 {
		t.Fatalf("los ingresos no son gasto: %+v", pagina.Items)
	}
}

func TestE053AislamientoYAislamientoDePeriodo(t *testing.T) {
	fuente := &e053PaginatedSource{pages: map[string][][]appMovements.Movement{
		"user-a": {{e053Movimiento("food", 10000, appMovements.TypeExpense)}},
		"user-b": {{e053Movimiento("transport", 5000, appMovements.TypeExpense)}},
	}}
	servicio := e053Servicio(fuente)
	a, err := servicio.Categories(context.Background(), "user-a", "2026-11")
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Items) != 1 || a.Items[0].CategoryID != "food" || a.Items[0].Percentage != 100 {
		t.Fatalf("user A recibio datos ajenos: %+v", a.Items)
	}

	// El mes que no existe en la fuente no puede mezclarse con el mes consultado.
	otro, err := servicio.Categories(context.Background(), "user-a", "2026-10")
	if err != nil {
		t.Fatal(err)
	}
	if len(otro.Items) != 0 {
		t.Fatalf("periodo sin datos devolvio datos de otro mes: %+v", otro.Items)
	}
}

func TestE053Negativas(t *testing.T) {
	ctx := context.Background()
	fuente := &e053PaginatedSource{pages: map[string][][]appMovements.Movement{
		"user-a": {{e053Movimiento("food", 100, appMovements.TypeExpense)}},
	}}
	servicio := e053Servicio(fuente)

	if _, err := servicio.Categories(ctx, "user-a", "2026-13"); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("periodo invalido: esperado ErrInvalidRequest, obtenido %v", err)
	}
	if _, err := servicio.Categories(ctx, " ", "2026-11"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("sin titular: esperado ErrUnauthorized, obtenido %v", err)
	}
	if _, err := NewService(nil).Categories(ctx, "user-a", "2026-11"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("sin fuente: esperado ErrUnavailable, obtenido %v", err)
	}
	conFallo := e053Servicio(&e053PaginatedSource{err: appMovements.ErrUnavailable})
	if _, err := conFallo.Categories(ctx, "user-a", "2026-11"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Dynamo caido: esperado ErrUnavailable, obtenido %v", err)
	}
}

func TestE053MontoMaximoDelContrato(t *testing.T) {
	wire := FormatoCents(99999999999999)
	if wire != "999999999999.99" {
		t.Fatalf("importe maximo del pattern OpenAPI mal formateado: %q", wire)
	}
}
