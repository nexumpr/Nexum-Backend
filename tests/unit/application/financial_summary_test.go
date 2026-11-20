package application_test

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	appSummary "github.com/nexumpr/Nexum-Backend/internal/application/financialsummary"
)

type filaMovimiento struct {
	Type           string
	AmountCents    int64
	ContributionID string
}

type filaAportacion struct {
	Status      string
	AmountCents int64
}

type movimientoResumen struct {
	row        filaMovimiento
	occurredAt time.Time
}

type aportacionResumen struct {
	row        filaAportacion
	occurredAt time.Time
}

type almacenResumen struct {
	zonas        map[string]string
	errZona      error
	movimientos  map[string][]movimientoResumen
	aportaciones map[string][]aportacionResumen
	totalesMov   *appSummary.MovementTotals
	totalesApor  *appSummary.ContributionTotals
	errMov       error
	errApor      error
}

func nuevoAlmacenResumen() *almacenResumen {
	return &almacenResumen{
		zonas:        map[string]string{},
		movimientos:  map[string][]movimientoResumen{},
		aportaciones: map[string][]aportacionResumen{},
	}
}

func (a *almacenResumen) LoadTimezone(_ context.Context, userID string) (string, error) {
	if a.errZona != nil {
		return "", a.errZona
	}
	return a.zonas[userID], nil
}

func (a *almacenResumen) AggregateMovements(_ context.Context, userID string, from, to time.Time) (appSummary.MovementTotals, error) {
	if a.errMov != nil {
		return appSummary.MovementTotals{}, a.errMov
	}
	if a.totalesMov != nil {
		return *a.totalesMov, nil
	}
	var total appSummary.MovementTotals
	for _, m := range a.movimientos[userID] {
		if m.occurredAt.Before(from) || !m.occurredAt.Before(to) {
			continue
		}
		switch m.row.Type {
		case "income":
			total.IncomeCents += m.row.AmountCents
		case "expense":
			total.ExpensesCents += m.row.AmountCents
		case "saving":
			if m.row.ContributionID == "" {
				total.IndependentSavingsCents += m.row.AmountCents
			}
		}
	}
	return total, nil
}

func (a *almacenResumen) AggregateContributions(_ context.Context, userID string, from, to time.Time) (appSummary.ContributionTotals, error) {
	if a.errApor != nil {
		return appSummary.ContributionTotals{}, a.errApor
	}
	if a.totalesApor != nil {
		return *a.totalesApor, nil
	}
	var total appSummary.ContributionTotals
	for _, c := range a.aportaciones[userID] {
		if c.occurredAt.Before(from) || !c.occurredAt.Before(to) {
			continue
		}
		if c.row.Status == "active" {
			total.ActiveSavingsCents += c.row.AmountCents
		}
	}
	return total, nil
}

func nuevoServicioResumen(almacen *almacenResumen) *appSummary.Service {
	return appSummary.NewService(almacen).WithClock(func() time.Time {
		return time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	})
}

func TestE026AhorrosIndependientesYAportacionesActivas(t *testing.T) {
	almacen := nuevoAlmacenResumen()
	almacen.zonas["u"] = "UTC"
	nov := time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC)
	almacen.movimientos["u"] = []movimientoResumen{
		{filaMovimiento{Type: "income", AmountCents: 500000}, nov},
		{filaMovimiento{Type: "expense", AmountCents: 100000}, nov},
		{filaMovimiento{Type: "saving", AmountCents: 50000}, nov},
		{filaMovimiento{Type: "saving", AmountCents: 30000, ContributionID: "c1"}, nov},
		{filaMovimiento{Type: "saving", AmountCents: 20000}, nov},
	}
	almacen.aportaciones["u"] = []aportacionResumen{
		{filaAportacion{Status: "active", AmountCents: 20000}, nov},
		{filaAportacion{Status: "reversed", AmountCents: 10000}, nov},
	}
	resumen, err := nuevoServicioResumen(almacen).Summary(context.Background(), "u", "2026-11")
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if resumen.Income != 500000 || resumen.Expenses != 100000 || resumen.Savings != 90000 || resumen.Net != 310000 {
		t.Fatalf("resumen inesperado: %+v", resumen)
	}
}

func TestE026SinDobleConteoDeMovimientoVinculado(t *testing.T) {
	almacen := nuevoAlmacenResumen()
	almacen.zonas["u"] = "UTC"
	nov := time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC)
	almacen.movimientos["u"] = []movimientoResumen{
		{filaMovimiento{Type: "saving", AmountCents: 30000, ContributionID: "c1"}, nov},
	}
	almacen.aportaciones["u"] = []aportacionResumen{
		{filaAportacion{Status: "active", AmountCents: 30000}, nov},
	}
	resumen, err := nuevoServicioResumen(almacen).Summary(context.Background(), "u", "2026-11")
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if resumen.Savings != 30000 {
		t.Fatalf("savings esperado 30000 (solo aportación), obtenido %d", resumen.Savings)
	}
}

func TestE026PeriodoVacioYZonaHoraria(t *testing.T) {
	almacen := nuevoAlmacenResumen()
	almacen.zonas["u"] = "America/Mexico_City"
	almacen.movimientos["u"] = []movimientoResumen{
		{filaMovimiento{Type: "income", AmountCents: 100}, time.Date(2026, 11, 1, 5, 0, 0, 0, time.UTC)},
	}
	servicio := nuevoServicioResumen(almacen)
	nov, err := servicio.Summary(context.Background(), "u", "2026-11")
	if err != nil || nov.Income != 0 {
		t.Fatalf("noviembre no debe incluir octubre: %v/%d", err, nov.Income)
	}
	oct, err := servicio.Summary(context.Background(), "u", "2026-10")
	if err != nil || oct.Income != 100 {
		t.Fatalf("octubre debe incluir el movimiento: %v/%d", err, oct.Income)
	}
	vacio, err := servicio.Summary(context.Background(), "u", "")
	if err != nil || vacio.Period != "2026-11" {
		t.Fatalf("periodo por defecto: %v/%q", err, vacio.Period)
	}
}

func TestE026SinDatosYPeriodoInvalido(t *testing.T) {
	almacen := nuevoAlmacenResumen()
	servicio := nuevoServicioResumen(almacen)
	resumen, err := servicio.Summary(context.Background(), "u", "2026-11")
	if err != nil || resumen.Income != 0 || resumen.Expenses != 0 || resumen.Savings != 0 || resumen.Net != 0 {
		t.Fatalf("sin datos: %v/%+v", err, resumen)
	}
	for _, periodo := range []string{"2026-13", "2026-1", "202611", "2026-00"} {
		if _, err := servicio.Summary(context.Background(), "u", periodo); !errors.Is(err, appSummary.ErrInvalidPeriod) {
			t.Fatalf("periodo %q debe ser 400, obtenido %v", periodo, err)
		}
	}
}

func TestE026AislamientoEntreUsuarios(t *testing.T) {
	almacen := nuevoAlmacenResumen()
	nov := time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC)
	almacen.movimientos["a"] = []movimientoResumen{{filaMovimiento{Type: "income", AmountCents: 1000}, nov}}
	almacen.movimientos["b"] = []movimientoResumen{{filaMovimiento{Type: "income", AmountCents: 2000}, nov}}
	servicio := nuevoServicioResumen(almacen)
	a, _ := servicio.Summary(context.Background(), "a", "2026-11")
	b, _ := servicio.Summary(context.Background(), "b", "2026-11")
	if a.Income != 1000 || b.Income != 2000 {
		t.Fatalf("aislamiento roto: a=%d b=%d", a.Income, b.Income)
	}
}

func TestE026ErrorDePersistenciaNoDevuelve200(t *testing.T) {
	almacen := nuevoAlmacenResumen()
	almacen.errZona = appSummary.ErrUnavailable
	if _, err := nuevoServicioResumen(almacen).Summary(context.Background(), "u", "2026-11"); !errors.Is(err, appSummary.ErrUnavailable) {
		t.Fatalf("error de PREFERENCES debe ser 503, obtenido %v", err)
	}
	almacen2 := nuevoAlmacenResumen()
	almacen2.errMov = appSummary.ErrInconsistent
	if _, err := nuevoServicioResumen(almacen2).Summary(context.Background(), "u", "2026-11"); !errors.Is(err, appSummary.ErrInconsistent) {
		t.Fatalf("registro inconsistente debe propagarse, obtenido %v", err)
	}
}

func TestE026OverflowFallaCerrado(t *testing.T) {
	almacen := nuevoAlmacenResumen()
	almacen.totalesMov = &appSummary.MovementTotals{IndependentSavingsCents: math.MaxInt64}
	almacen.totalesApor = &appSummary.ContributionTotals{ActiveSavingsCents: 1}
	if _, err := nuevoServicioResumen(almacen).Summary(context.Background(), "u", "2026-11"); !errors.Is(err, appSummary.ErrOverflow) {
		t.Fatalf("esperado overflow, obtenido %v", err)
	}
}
