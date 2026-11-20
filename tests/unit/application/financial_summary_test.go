package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	appSummary "github.com/nexumpr/Nexum-Backend/internal/application/financialsummary"
)

type movimientoResumen struct {
	row        appSummary.MovementRow
	occurredAt time.Time
}

type aportacionResumen struct {
	row        appSummary.ContributionRow
	occurredAt time.Time
}

type almacenResumen struct {
	zonas          map[string]string
	movimientos    map[string][]movimientoResumen
	aportaciones   map[string][]aportacionResumen
	errMovimientos error
}

func nuevoAlmacenResumen() *almacenResumen {
	return &almacenResumen{
		zonas:        map[string]string{},
		movimientos:  map[string][]movimientoResumen{},
		aportaciones: map[string][]aportacionResumen{},
	}
}

func (a *almacenResumen) LoadTimezone(_ context.Context, userID string) (string, error) {
	return a.zonas[userID], nil
}

func (a *almacenResumen) ListMovements(_ context.Context, userID string, from, to time.Time) ([]appSummary.MovementRow, error) {
	if a.errMovimientos != nil {
		return nil, a.errMovimientos
	}
	var salida []appSummary.MovementRow
	for _, m := range a.movimientos[userID] {
		if !m.occurredAt.Before(from) && m.occurredAt.Before(to) {
			salida = append(salida, m.row)
		}
	}
	return salida, nil
}

func (a *almacenResumen) ListContributions(_ context.Context, userID string, from, to time.Time) ([]appSummary.ContributionRow, error) {
	var salida []appSummary.ContributionRow
	for _, c := range a.aportaciones[userID] {
		if !c.occurredAt.Before(from) && c.occurredAt.Before(to) {
			salida = append(salida, c.row)
		}
	}
	return salida, nil
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
		{appSummary.MovementRow{Type: "income", AmountCents: 500000}, nov},
		{appSummary.MovementRow{Type: "expense", AmountCents: 100000}, nov},
		{appSummary.MovementRow{Type: "saving", AmountCents: 50000}, nov},                       // independiente E028
		{appSummary.MovementRow{Type: "saving", AmountCents: 30000, ContributionID: "c1"}, nov}, // vinculado
		{appSummary.MovementRow{Type: "saving", AmountCents: 20000}, nov},                       // independiente E037
	}
	almacen.aportaciones["u"] = []aportacionResumen{
		{appSummary.ContributionRow{Status: "active", AmountCents: 20000}, nov},
		{appSummary.ContributionRow{Status: "reversed", AmountCents: 10000}, nov},
	}
	resumen, err := nuevoServicioResumen(almacen).Summary(context.Background(), "u", "2026-11")
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if resumen.Income != 500000 || resumen.Expenses != 100000 {
		t.Fatalf("income/expenses inesperados: %+v", resumen)
	}
	// 50000 + 20000 independientes + 20000 aportación activa = 90000. El vinculado de 30000
	// no se suma y la aportación revertida no cuenta.
	if resumen.Savings != 90000 {
		t.Fatalf("savings esperado 90000, obtenido %d", resumen.Savings)
	}
	if resumen.Net != 310000 {
		t.Fatalf("net esperado 310000, obtenido %d", resumen.Net)
	}
	if resumen.Period != "2026-11" || resumen.Currency != "MXN" {
		t.Fatalf("periodo/divisa inesperados: %+v", resumen)
	}
}

func TestE026SinDobleConteoDeMovimientoVinculado(t *testing.T) {
	almacen := nuevoAlmacenResumen()
	almacen.zonas["u"] = "UTC"
	nov := time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC)
	almacen.movimientos["u"] = []movimientoResumen{
		{appSummary.MovementRow{Type: "saving", AmountCents: 30000, ContributionID: "c1"}, nov},
	}
	almacen.aportaciones["u"] = []aportacionResumen{
		{appSummary.ContributionRow{Status: "active", AmountCents: 30000}, nov},
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
	// 2026-11-01T05:00Z = 2026-10-31 23:00 en México: pertenece a octubre, no a noviembre.
	almacen.movimientos["u"] = []movimientoResumen{
		{appSummary.MovementRow{Type: "income", AmountCents: 100}, time.Date(2026, 11, 1, 5, 0, 0, 0, time.UTC)},
	}
	servicio := nuevoServicioResumen(almacen)
	nov, err := servicio.Summary(context.Background(), "u", "2026-11")
	if err != nil {
		t.Fatalf("noviembre: %v", err)
	}
	if nov.Income != 0 {
		t.Fatalf("noviembre no debe incluir el movimiento de octubre: %d", nov.Income)
	}
	oct, err := servicio.Summary(context.Background(), "u", "2026-10")
	if err != nil {
		t.Fatalf("octubre: %v", err)
	}
	if oct.Income != 100 {
		t.Fatalf("octubre debe incluir el movimiento: %d", oct.Income)
	}
	vacio, err := servicio.Summary(context.Background(), "u", "")
	if err != nil {
		t.Fatalf("periodo vacio: %v", err)
	}
	if vacio.Period != "2026-11" {
		t.Fatalf("periodo por defecto esperado 2026-11, obtenido %q", vacio.Period)
	}
}

func TestE026SinDatosYPeriodoInvalido(t *testing.T) {
	almacen := nuevoAlmacenResumen()
	servicio := nuevoServicioResumen(almacen)
	resumen, err := servicio.Summary(context.Background(), "u", "2026-11")
	if err != nil {
		t.Fatalf("sin datos: %v", err)
	}
	if resumen.Income != 0 || resumen.Expenses != 0 || resumen.Savings != 0 || resumen.Net != 0 {
		t.Fatalf("sin datos debe devolver ceros: %+v", resumen)
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
	almacen.movimientos["a"] = []movimientoResumen{
		{appSummary.MovementRow{Type: "income", AmountCents: 1000}, nov},
	}
	almacen.movimientos["b"] = []movimientoResumen{
		{appSummary.MovementRow{Type: "income", AmountCents: 2000}, nov},
	}
	servicio := nuevoServicioResumen(almacen)
	a, _ := servicio.Summary(context.Background(), "a", "2026-11")
	b, _ := servicio.Summary(context.Background(), "b", "2026-11")
	if a.Income != 1000 || b.Income != 2000 {
		t.Fatalf("aislamiento roto: a=%d b=%d", a.Income, b.Income)
	}
}
