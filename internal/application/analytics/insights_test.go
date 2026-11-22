package analytics

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
)

type e052FakeSource struct {
	byUser     map[string][]appMovements.Movement
	err        error
	noProgress bool
	llamadas   int
}

func (f *e052FakeSource) ListMovements(_ context.Context, q appMovements.ListQuery, startSK string, _ int) ([]appMovements.Movement, string, error) {
	if f.err != nil {
		return nil, "", f.err
	}
	f.llamadas++
	movimientos := append([]appMovements.Movement(nil), f.byUser[q.UserID]...)
	if f.noProgress {
		return movimientos, "sin-progreso", nil
	}
	if startSK != "" {
		return nil, "", nil
	}
	return movimientos, "", nil
}

func e052Movimientos(userID, categoria string, cantidad int) []appMovements.Movement {
	salida := make([]appMovements.Movement, 0, cantidad)
	base := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	for i := 0; i < cantidad; i++ {
		salida = append(salida, appMovements.Movement{
			ID:          "mov-" + userID + "-" + categoria + "-" + string(rune('a'+i)),
			Type:        appMovements.TypeExpense,
			AmountCents: 10000,
			CategoryID:  categoria,
			OccurredAt:  base.Add(time.Duration(i) * time.Hour),
		})
	}
	return salida
}

func TestE052UmbralDatosSuficientes(t *testing.T) {
	ctx := context.Background()
	reloj := func() time.Time { return time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC) }

	pocos := NewService(&e052FakeSource{byUser: map[string][]appMovements.Movement{
		"user-a": e052Movimientos("user-a", "comida", 11),
	}}).WithClock(reloj)
	pagina, err := pocos.Insights(ctx, "user-a", "2026-11")
	if err != nil {
		t.Fatalf("11 movimientos: %v", err)
	}
	if pagina.EnoughData || len(pagina.Items) != 0 || pagina.MinimumSamples != MinimumSamples {
		t.Fatalf("11 movimientos deben dar enoughData=false e items vacios: %+v", pagina)
	}

	completos := NewService(&e052FakeSource{byUser: map[string][]appMovements.Movement{
		"user-a": e052Movimientos("user-a", "comida", 12),
	}}).WithClock(reloj)
	pagina, err = completos.Insights(ctx, "user-a", "2026-11")
	if err != nil {
		t.Fatalf("12 movimientos: %v", err)
	}
	if !pagina.EnoughData || pagina.MinimumSamples != MinimumSamples || len(pagina.Items) != 1 {
		t.Fatalf("12 movimientos deben dar un insight: %+v", pagina)
	}
	insight := pagina.Items[0]
	if insight.EvidenceCount != 12 || insight.Period != "2026-11" || insight.ModelVersion != ModelVersion {
		t.Fatalf("insight incorrecto: %+v", insight)
	}
	if !strings.Contains(insight.Explanation, "comida") || !strings.Contains(insight.Explanation, "1200.00") {
		t.Fatalf("la explicacion debe citar categoria e importe exacto: %q", insight.Explanation)
	}
	if pagina.NextCursor != "" {
		t.Fatalf("E052 no declara parametro cursor: se encontro %q", pagina.NextCursor)
	}
}

func TestE052PeriodoPorDefectoYValidacion(t *testing.T) {
	reloj := func() time.Time { return time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC) }
	servicio := NewService(&e052FakeSource{byUser: map[string][]appMovements.Movement{}}).WithClock(reloj)

	pagina, err := servicio.Insights(context.Background(), "user-a", "")
	if err != nil {
		t.Fatalf("periodo ausente: %v", err)
	}
	if !pagina.EnoughData && pagina.MinimumSamples != MinimumSamples {
		t.Fatalf("respuesta sin datos inconsistente: %+v", pagina)
	}

	if _, err := servicio.Insights(context.Background(), "user-a", "2026-13"); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("periodo invalido: esperado ErrInvalidRequest, obtenido %v", err)
	}
	if _, err := servicio.Insights(context.Background(), "user-a", "2026-1"); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("periodo mal formado: esperado ErrInvalidRequest, obtenido %v", err)
	}
}

func TestE052AislamientoPorTitular(t *testing.T) {
	fuente := &e052FakeSource{byUser: map[string][]appMovements.Movement{
		"user-a": e052Movimientos("user-a", "comida", 12),
		"user-b": e052Movimientos("user-b", "transporte", 12),
	}}
	servicio := NewService(fuente).WithClock(func() time.Time { return time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC) })

	paginaA, err := servicio.Insights(context.Background(), "user-a", "2026-11")
	if err != nil {
		t.Fatalf("user A: %v", err)
	}
	if len(paginaA.Items) != 1 || !strings.Contains(paginaA.Items[0].Explanation, "comida") {
		t.Fatalf("user A recibio datos ajenos o ninguno: %+v", paginaA)
	}
	if strings.Contains(paginaA.Items[0].Explanation, "transporte") {
		t.Fatalf("fuga de categoria de user B: %q", paginaA.Items[0].Explanation)
	}
}

func TestE052FallosControlados(t *testing.T) {
	ctx := context.Background()
	if _, err := NewService(nil).Insights(ctx, "user-a", "2026-11"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("sin fuente: esperado ErrUnavailable, obtenido %v", err)
	}
	fuente := &e052FakeSource{err: appMovements.ErrUnavailable}
	if _, err := NewService(fuente).Insights(ctx, "user-a", "2026-11"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Dynamo caido: esperado ErrUnavailable, obtenido %v", err)
	}
	sinProgreso := &e052FakeSource{noProgress: true, byUser: map[string][]appMovements.Movement{
		"user-a": e052Movimientos("user-a", "comida", 12),
	}}
	if _, err := NewService(sinProgreso).Insights(ctx, "user-a", "2026-11"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("cursor sin progreso: esperado ErrUnavailable, obtenido %v", err)
	}
	if _, err := NewService(&e052FakeSource{}).Insights(ctx, " ", "2026-11"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("sin titular: esperado ErrUnauthorized, obtenido %v", err)
	}
}

func TestE052IDDeterministaSinPII(t *testing.T) {
	reloj := func() time.Time { return time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC) }
	nuevo := func(user string) *Service {
		return NewService(&e052FakeSource{byUser: map[string][]appMovements.Movement{
			user: e052Movimientos(user, "comida", 12),
		}}).WithClock(reloj)
	}
	a1, err := nuevo("user-a").Insights(context.Background(), "user-a", "2026-11")
	if err != nil {
		t.Fatal(err)
	}
	a2, err := nuevo("user-a").Insights(context.Background(), "user-a", "2026-11")
	if err != nil {
		t.Fatal(err)
	}
	b, err := nuevo("user-b").Insights(context.Background(), "user-b", "2026-11")
	if err != nil {
		t.Fatal(err)
	}
	if a1.Items[0].ID != a2.Items[0].ID {
		t.Fatal("el id debe ser determinista")
	}
	if a1.Items[0].ID == b.Items[0].ID {
		t.Fatal("titulares distintos no pueden compartir id")
	}
	uuid := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-5[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !uuid.MatchString(a1.Items[0].ID) {
		t.Fatalf("id no es UUID v5: %q", a1.Items[0].ID)
	}
	if strings.Contains(a1.Items[0].ID, "user-a") || strings.Contains(a1.Items[0].Explanation, "user-a") {
		t.Fatal("la respuesta no debe exponer el identificador del titular")
	}
}
