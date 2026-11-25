package analytics

import (
	"context"
	"encoding/json"
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

// e052RangoSource respeta los límites [From, To] que E052 envía al almacén, igual
// que la Query real de DynamoDB. Permite verificar los bordes del mes.
type e052RangoSource struct {
	movimientos []appMovements.Movement
}

func (f *e052RangoSource) ListMovements(_ context.Context, q appMovements.ListQuery, startSK string, _ int) ([]appMovements.Movement, string, error) {
	if startSK != "" {
		return nil, "", nil
	}
	salida := make([]appMovements.Movement, 0, len(f.movimientos))
	for _, movimiento := range f.movimientos {
		if q.HasFrom && movimiento.OccurredAt.Before(q.From) {
			continue
		}
		if q.HasTo && movimiento.OccurredAt.After(q.To) {
			continue
		}
		salida = append(salida, movimiento)
	}
	return salida, "", nil
}

func e052GastoEn(momento time.Time) appMovements.Movement {
	return appMovements.Movement{
		ID: "mov-" + momento.Format(time.RFC3339Nano), Type: appMovements.TypeExpense,
		AmountCents: 1000, Currency: "MXN", CategoryID: "comida", OccurredAt: momento,
	}
}

// TestE052PeriodoOmitidoUsaMesUTCActual fija el defecto: el handler envía "" y el
// insight debe publicar el mes normalizado, no el parámetro crudo.
func TestE052PeriodoOmitidoUsaMesUTCActual(t *testing.T) {
	reloj := func() time.Time { return time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC) }
	servicio := NewService(&e052FakeSource{byUser: map[string][]appMovements.Movement{
		"user-a": e052Movimientos("user-a", "comida", 12),
	}}).WithClock(reloj)

	pagina, err := servicio.Insights(context.Background(), "user-a", "")
	if err != nil {
		t.Fatalf("periodo omitido: %v", err)
	}
	if len(pagina.Items) != 1 {
		t.Fatalf("se esperaba un insight: %+v", pagina)
	}
	insight := pagina.Items[0]
	if insight.Period == "" {
		t.Fatal("Period quedo vacio con periodo omitido")
	}
	if insight.Period != "2026-11" {
		t.Fatalf("Period debe ser el mes UTC actual 2026-11, obtenido %q", insight.Period)
	}
	if !strings.Contains(insight.Explanation, "En 2026-11") {
		t.Fatalf("la explicacion debe citar el periodo normalizado: %q", insight.Explanation)
	}

	explicito, err := servicio.Insights(context.Background(), "user-a", "2026-11")
	if err != nil {
		t.Fatal(err)
	}
	if explicito.Items[0].ID != insight.ID {
		t.Fatalf("omitido y explicito deben producir el mismo id: %q vs %q", insight.ID, explicito.Items[0].ID)
	}
}

func TestE052PeriodoExplicitoSePropaga(t *testing.T) {
	servicio := NewService(&e052FakeSource{byUser: map[string][]appMovements.Movement{
		"user-a": e052Movimientos("user-a", "comida", 12),
	}}).WithClock(func() time.Time { return time.Date(2026, 12, 1, 0, 30, 0, 0, time.UTC) })

	pagina, err := servicio.Insights(context.Background(), "user-a", "2026-10")
	if err != nil {
		t.Fatalf("periodo explicito: %v", err)
	}
	insight := pagina.Items[0]
	if insight.Period != "2026-10" {
		t.Fatalf("Period debe ser el solicitado: %q", insight.Period)
	}
	if !strings.Contains(insight.Explanation, "En 2026-10") {
		t.Fatalf("la explicacion debe citar el periodo solicitado: %q", insight.Explanation)
	}
}

func TestE052PeriodoInvalido(t *testing.T) {
	servicio := NewService(&e052FakeSource{byUser: map[string][]appMovements.Movement{}})
	for _, caso := range []string{"2026-13", "2026-00", "2026-1", "2026-11-01", "2026/11", "noviembre"} {
		t.Run(caso, func(t *testing.T) {
			if _, err := servicio.Insights(context.Background(), "user-a", caso); !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("periodo %q: esperado ErrInvalidRequest, obtenido %v", caso, err)
			}
		})
	}
	// Solo espacios equivale a ausente y usa el mes UTC actual.
	if _, err := servicio.Insights(context.Background(), "user-a", "   "); err != nil {
		t.Fatalf("periodo en blanco debe tratarse como ausente: %v", err)
	}
}

func TestE052DatosInsuficientesNoEmitePeriodoVacio(t *testing.T) {
	servicio := NewService(&e052FakeSource{byUser: map[string][]appMovements.Movement{
		"user-a": e052Movimientos("user-a", "comida", 11),
	}}).WithClock(func() time.Time { return time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC) })

	pagina, err := servicio.Insights(context.Background(), "user-a", "")
	if err != nil {
		t.Fatal(err)
	}
	if pagina.EnoughData || len(pagina.Items) != 0 {
		t.Fatalf("11 movimientos no deben producir insight: %+v", pagina)
	}
	wire := pagina.Wire()
	if wire.Items == nil {
		t.Fatal("items debe serializarse como [] y no null")
	}
	if len(wire.Items) != 0 {
		t.Fatalf("items debe estar vacio: %+v", wire.Items)
	}
}

func TestE052PeriodoNuncaVacioYConformeAlPatron(t *testing.T) {
	patron := regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)
	servicio := NewService(&e052FakeSource{byUser: map[string][]appMovements.Movement{
		"user-a": e052Movimientos("user-a", "comida", 12),
	}}).WithClock(func() time.Time { return time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC) })

	for _, solicitado := range []string{"", "2026-11", "2025-01"} {
		pagina, err := servicio.Insights(context.Background(), "user-a", solicitado)
		if err != nil {
			t.Fatalf("period=%q: %v", solicitado, err)
		}
		wire := pagina.Wire()
		if len(wire.Items) != 1 {
			t.Fatalf("period=%q: sin insight", solicitado)
		}
		if wire.Items[0].Period == "" {
			t.Fatalf("period=%q: period vacio en el contrato", solicitado)
		}
		if !patron.MatchString(wire.Items[0].Period) {
			t.Fatalf("period=%q no cumple el pattern del schema: %q", solicitado, wire.Items[0].Period)
		}
		crudo, err := json.Marshal(wire)
		if err != nil {
			t.Fatal(err)
		}
		var decodificado struct {
			Items []struct {
				Period *string `json:"period"`
			} `json:"items"`
		}
		if err := json.Unmarshal(crudo, &decodificado); err != nil {
			t.Fatal(err)
		}
		if len(decodificado.Items) != 1 || decodificado.Items[0].Period == nil || *decodificado.Items[0].Period == "" {
			t.Fatalf("el JSON debe incluir period no vacio: %s", crudo)
		}
	}
}

func TestE052LimitesDeMes(t *testing.T) {
	noviembre := []time.Time{
		time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 11, 30, 23, 59, 59, 999000000, time.UTC),
	}
	movimientos := make([]appMovements.Movement, 0, len(noviembre)+2)
	for _, momento := range noviembre {
		movimientos = append(movimientos, e052GastoEn(momento))
	}
	// Fuera del mes: no deben contar ni alterar el importe.
	movimientos = append(movimientos,
		e052GastoEn(time.Date(2026, 10, 31, 23, 59, 59, 999000000, time.UTC)),
		e052GastoEn(time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)),
	)
	servicio := NewService(&e052RangoSource{movimientos: movimientos}).
		WithClock(func() time.Time { return time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC) })

	pagina, err := servicio.Insights(context.Background(), "user-a", "2026-11")
	if err != nil {
		t.Fatal(err)
	}
	if !pagina.EnoughData || len(pagina.Items) != 1 {
		t.Fatalf("se esperaba un insight: %+v", pagina)
	}
	if pagina.Items[0].EvidenceCount != 12 {
		t.Fatalf("evidenceCount debe excluir octubre y diciembre: %d", pagina.Items[0].EvidenceCount)
	}
	if !strings.Contains(pagina.Items[0].Explanation, "120.00") {
		t.Fatalf("el importe debe ser el de noviembre (120.00): %q", pagina.Items[0].Explanation)
	}
}
