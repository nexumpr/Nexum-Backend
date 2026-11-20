package integration_test

// Integridad financiera de la Oleada 3 sobre los servicios de aplicación REALES y un doble con
// estado COMPARTIDO (`libroMayor`). El objetivo es comprobar el cruce entre endpoints, que las
// pruebas por endpoint no pueden ver:
//
//	E044 aporta -> E026/E051 suman una sola vez -> E047 revierte -> deja de sumar;
//	E037 registra un pago -> E026 lo ve como egreso -> E050 lo consume como gasto real;
//	E042 sólo borra sin aportaciones vigentes y el tombstone desaparece de E051;
//	E048/E049 comparten ETag y CAS -> E050 usa los límites vigentes.
//
// Qué es real aquí: los casos de uso de `internal/application/*` sin sustituir, sus relojes e
// identificadores inyectados y las proyecciones de contrato (Wire), cuyos errores centinela son
// los que el transporte traduce a 400/401/404/409/412/422/428/503.
//
// Qué NO es real: no hay DynamoDB, Redis, AWS ni HTTP de red. `libroMayor` implementa los
// puertos de persistencia con las mismas reglas que documentan sus adaptadores (capacidad,
// idempotencia por usuario, CAS por versión, día civil local), pero no acredita persistencia
// real ni despliegue. Cada adaptador y cada transacción tienen sus propias pruebas contra el SDK
// y la verificación cloud sigue siendo NOT_RUN.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	appBudget "github.com/nexumpr/Nexum-Backend/internal/application/budgetset"
	appUsage "github.com/nexumpr/Nexum-Backend/internal/application/budgetusage"
	appContrib "github.com/nexumpr/Nexum-Backend/internal/application/contributions"
	appDashboard "github.com/nexumpr/Nexum-Backend/internal/application/dashboard"
	appSummary "github.com/nexumpr/Nexum-Backend/internal/application/financialsummary"
	appGoals "github.com/nexumpr/Nexum-Backend/internal/application/goals"
	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
	appRecurring "github.com/nexumpr/Nexum-Backend/internal/application/recurring"
	domainGoal "github.com/nexumpr/Nexum-Backend/internal/domain/goal"
	domainIdentity "github.com/nexumpr/Nexum-Backend/internal/domain/identity"
	"github.com/nexumpr/Nexum-Backend/internal/domain/money"
)

const (
	liUsuario     = "11111111-2222-4333-8444-555555555555"
	liOtro        = "99999999-8888-4777-8666-555555555555"
	liMetaA       = "018f90ef-08e5-7a68-b09f-0000000000a1"
	liMetaB       = "018f90ef-08e5-7a68-b09f-0000000000a2"
	liMetaLegado  = "018f90ef-08e5-7a68-b09f-0000000000a3"
	liRegla       = "018f90ef-08e5-7a68-b09f-0000000000e1"
	liMovIngreso  = "018f90ef-08e5-7a68-b09f-0000000000f1"
	liMovGasto    = "018f90ef-08e5-7a68-b09f-0000000000f2"
	liMovAhorro   = "018f90ef-08e5-7a68-b09f-0000000000f3"
	liClaveCursor = "0123456789abcdef0123456789abcdef"
)

var liAhora = time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)

// liDia devuelve un instante dentro de noviembre de 2026 (10:00 UTC).
func liDia(dia int) time.Time { return time.Date(2026, 11, dia, 10, 0, 0, 0, time.UTC) }

// ---------------------------------------------------------------------------------------------
// Doble con estado compartido
// ---------------------------------------------------------------------------------------------

// metaLibro es una meta tal como la vería la persistencia. `SinContador` reproduce un ítem
// heredado anterior a las aportaciones (sin atributo `activeContributions`).
type metaLibro struct {
	ID                  string
	Name                string
	TargetCents         int64
	SavedCents          int64
	OpeningCents        int64
	Currency            string
	TargetDate          string
	Status              string
	Version             int64
	Deleted             bool
	SinContador         bool
	ActiveContributions int64
}

// registroPago es el registro de idempotencia de E037 en el doble.
type registroPago struct {
	hash   string
	cuerpo []byte
	status int
}

type libroMayor struct {
	mu             sync.Mutex
	zona           string
	metas          map[string]map[string]*metaLibro
	aportaciones   map[string]map[string]*appContrib.Contribution
	movimientos    map[string][]appMovements.Movement
	presupuestos   map[string]map[string]*appBudget.Set
	reglas         map[string]map[string]*appRecurring.Payment
	idemAportacion map[string]appContrib.IdempotencyRecord
	idemPago       map[string]registroPago
	fechasPagadas  map[string]bool
	escriturasMeta int
}

func nuevoLibroMayor(zona string) *libroMayor {
	return &libroMayor{
		zona:           zona,
		metas:          map[string]map[string]*metaLibro{},
		aportaciones:   map[string]map[string]*appContrib.Contribution{},
		movimientos:    map[string][]appMovements.Movement{},
		presupuestos:   map[string]map[string]*appBudget.Set{},
		reglas:         map[string]map[string]*appRecurring.Payment{},
		idemAportacion: map[string]appContrib.IdempotencyRecord{},
		idemPago:       map[string]registroPago{},
		fechasPagadas:  map[string]bool{},
	}
}

func (l *libroMayor) sembrarMeta(userID string, meta metaLibro) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.metas[userID] == nil {
		l.metas[userID] = map[string]*metaLibro{}
	}
	copia := meta
	l.metas[userID][meta.ID] = &copia
}

func (l *libroMayor) sembrarMovimiento(userID string, movimiento appMovements.Movement) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.movimientos[userID] = append(l.movimientos[userID], movimiento)
}

func (l *libroMayor) sembrarRegla(userID string, regla appRecurring.Payment) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.reglas[userID] == nil {
		l.reglas[userID] = map[string]*appRecurring.Payment{}
	}
	copia := regla
	l.reglas[userID][regla.ID] = &copia
}

func (l *libroMayor) desactivarRegla(userID, reglaID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if regla, ok := l.reglas[userID][reglaID]; ok {
		regla.Active = false
	}
}

// leerMeta devuelve una copia de la meta para las aserciones.
func (l *libroMayor) leerMeta(userID, metaID string) (metaLibro, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	meta, ok := l.metas[userID][metaID]
	if !ok {
		return metaLibro{}, false
	}
	return *meta, true
}

func (l *libroMayor) numeroAportaciones(userID string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.aportaciones[userID])
}

func (l *libroMayor) numeroMovimientos(userID string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.movimientos[userID])
}

func (l *libroMayor) movimientosDe(userID string) []appMovements.Movement {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]appMovements.Movement(nil), l.movimientos[userID]...)
}

func (l *libroMayor) escriturasDeMetas() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.escriturasMeta
}

func (l *libroMayor) pagoRegistrado(userID, reglaID, fechaLocal string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.fechasPagadas[userID+"|"+reglaID+"|"+fechaLocal]
}

func copiarAportacion(c appContrib.Contribution) appContrib.Contribution {
	copia := c
	copia.Allocations = append([]appContrib.Allocation(nil), c.Allocations...)
	return copia
}

func copiarConjunto(s appBudget.Set) appBudget.Set {
	copia := s
	copia.Items = append([]appBudget.Budget(nil), s.Items...)
	return copia
}

// ---------------------------------------------------------------------------------------------
// Adaptadores de puerto sobre el libro mayor
// ---------------------------------------------------------------------------------------------

// resumenDelLibro implementa `financialsummary.Store` (E026).
type resumenDelLibro struct{ libro *libroMayor }

func (r resumenDelLibro) LoadTimezone(context.Context, string) (string, error) {
	return r.libro.zona, nil
}

func (r resumenDelLibro) AggregateMovements(_ context.Context, userID string, desde, hasta time.Time) (appSummary.MovementTotals, error) {
	r.libro.mu.Lock()
	defer r.libro.mu.Unlock()
	var total appSummary.MovementTotals
	for _, m := range r.libro.movimientos[userID] {
		if m.OccurredAt.Before(desde) || !m.OccurredAt.Before(hasta) {
			continue
		}
		switch m.Type {
		case appMovements.TypeIncome:
			total.IncomeCents += m.AmountCents
		case appMovements.TypeExpense:
			total.ExpensesCents += m.AmountCents
		case appMovements.TypeSaving:
			// El movimiento vinculado a una aportación no se suma: la aportación es la
			// autoridad contable (RFC-022 v1.2.1).
			if strings.TrimSpace(m.ContributionID) == "" {
				total.IndependentSavingsCents += m.AmountCents
			}
		default:
			return appSummary.MovementTotals{}, appSummary.ErrInconsistent
		}
	}
	return total, nil
}

func (r resumenDelLibro) AggregateContributions(_ context.Context, userID string, desde, hasta time.Time) (appSummary.ContributionTotals, error) {
	r.libro.mu.Lock()
	defer r.libro.mu.Unlock()
	var total appSummary.ContributionTotals
	for _, c := range r.libro.aportaciones[userID] {
		if c.OccurredAt.Before(desde) || !c.OccurredAt.Before(hasta) {
			continue
		}
		switch c.Status {
		case appContrib.StatusActive:
			total.ActiveSavingsCents += c.AmountCents
		case appContrib.StatusReversed:
			// Una aportación revertida no cuenta como ahorro activo.
		default:
			return appSummary.ContributionTotals{}, appSummary.ErrInconsistent
		}
	}
	return total, nil
}

// dashboardDelLibro implementa `dashboard.Store` (E051).
type dashboardDelLibro struct{ libro *libroMayor }

func (d dashboardDelLibro) ListActiveGoals(_ context.Context, userID string, limit int) ([]appDashboard.GoalRow, error) {
	d.libro.mu.Lock()
	defer d.libro.mu.Unlock()
	salida := make([]appDashboard.GoalRow, 0, len(d.libro.metas[userID]))
	for _, meta := range d.libro.metas[userID] {
		// El tombstone de E042 deja de existir para el cliente.
		if meta.Deleted || meta.Status != "active" {
			continue
		}
		salida = append(salida, appDashboard.GoalRow{
			ID: meta.ID, Name: meta.Name,
			TargetCents: meta.TargetCents, SavedCents: meta.SavedCents, OpeningCents: meta.OpeningCents,
			Currency: meta.Currency, TargetDate: meta.TargetDate, Status: meta.Status, Version: meta.Version,
		})
	}
	sort.Slice(salida, func(i, j int) bool { return salida[i].ID < salida[j].ID })
	if limit > 0 && len(salida) > limit {
		salida = salida[:limit]
	}
	return salida, nil
}

func (d dashboardDelLibro) ListLatestMovements(_ context.Context, userID string, limit int) ([]appMovements.Movement, error) {
	d.libro.mu.Lock()
	defer d.libro.mu.Unlock()
	salida := append([]appMovements.Movement(nil), d.libro.movimientos[userID]...)
	sort.Slice(salida, func(i, j int) bool {
		if !salida[i].OccurredAt.Equal(salida[j].OccurredAt) {
			return salida[i].OccurredAt.After(salida[j].OccurredAt)
		}
		return salida[i].ID > salida[j].ID
	})
	if limit > 0 && len(salida) > limit {
		salida = salida[:limit]
	}
	return salida, nil
}

// aportacionesDelLibro implementa `contributions.Store` (E043–E047).
type aportacionesDelLibro struct{ libro *libroMayor }

func (a aportacionesDelLibro) LoadGoal(_ context.Context, userID, metaID string) (*appContrib.GoalSnapshot, error) {
	a.libro.mu.Lock()
	defer a.libro.mu.Unlock()
	meta, ok := a.libro.metas[userID][metaID]
	if !ok {
		return nil, nil
	}
	return &appContrib.GoalSnapshot{
		ID: meta.ID, TargetCents: meta.TargetCents, SavedCents: meta.SavedCents,
		Status: meta.Status, Deleted: meta.Deleted,
		ActiveContributions: meta.ActiveContributions, Version: meta.Version,
	}, nil
}

func (a aportacionesDelLibro) LoadIdempotency(_ context.Context, userID, operacion, clave string) (*appContrib.IdempotencyRecord, error) {
	a.libro.mu.Lock()
	defer a.libro.mu.Unlock()
	registro, ok := a.libro.idemAportacion[userID+"|"+operacion+"|"+clave]
	if !ok {
		return nil, nil
	}
	copia := registro
	copia.ResponseBody = append([]byte(nil), registro.ResponseBody...)
	return &copia, nil
}

func (a aportacionesDelLibro) CreateContribution(_ context.Context, write appContrib.CreateWrite) (*appContrib.StoredResponse, error) {
	a.libro.mu.Lock()
	defer a.libro.mu.Unlock()

	clave := write.UserID + "|" + write.Operation + "|" + write.IdempotencyKey
	if previo, ok := a.libro.idemAportacion[clave]; ok {
		if previo.PayloadHash != write.PayloadHash {
			return nil, appContrib.ErrConflict
		}
		return &appContrib.StoredResponse{
			Status: previo.HTTPStatus, Body: append([]byte(nil), previo.ResponseBody...),
			Location: previo.Location, Replayed: true,
		}, nil
	}

	// Fase de condiciones: si una asignación no cabe, NO se aplica ninguna (misma garantía que
	// la `TransactWriteItems` del adaptador real).
	for _, asignacion := range write.Contribution.Allocations {
		meta, ok := a.libro.metas[write.UserID][asignacion.GoalID]
		if !ok {
			return nil, appContrib.ErrGoalNotFound
		}
		if meta.Status != "active" || meta.Deleted {
			return nil, appContrib.ErrGoalInactive
		}
		capacidad := meta.TargetCents - meta.SavedCents
		if capacidad < 0 {
			capacidad = 0
		}
		if asignacion.AmountCents > capacidad {
			return nil, appContrib.ErrCapacityExceeded
		}
	}
	for _, asignacion := range write.Contribution.Allocations {
		meta := a.libro.metas[write.UserID][asignacion.GoalID]
		meta.SavedCents += asignacion.AmountCents
		meta.ActiveContributions++
		meta.Version++
	}
	if a.libro.aportaciones[write.UserID] == nil {
		a.libro.aportaciones[write.UserID] = map[string]*appContrib.Contribution{}
	}
	copia := copiarAportacion(write.Contribution)
	a.libro.aportaciones[write.UserID][write.Contribution.ID] = &copia
	a.libro.movimientos[write.UserID] = append(a.libro.movimientos[write.UserID], write.Movement)
	a.libro.idemAportacion[clave] = appContrib.IdempotencyRecord{
		PayloadHash: write.PayloadHash, HTTPStatus: 201,
		ResponseBody: append([]byte(nil), write.ResponseBody...), Location: write.Location, ExpiresAt: write.ExpiresAt,
	}
	return &appContrib.StoredResponse{Status: 201, Body: write.ResponseBody, Location: write.Location}, nil
}

func (a aportacionesDelLibro) ListContributions(_ context.Context, query appContrib.ListQuery, startSK string, limit int) ([]appContrib.Contribution, string, error) {
	a.libro.mu.Lock()
	defer a.libro.mu.Unlock()
	items := make([]appContrib.Contribution, 0, len(a.libro.aportaciones[query.UserID]))
	for _, c := range a.libro.aportaciones[query.UserID] {
		if query.Status != "" && c.Status != query.Status {
			continue
		}
		if query.GoalID != "" {
			coincide := false
			for _, asignacion := range c.Allocations {
				if asignacion.GoalID == query.GoalID {
					coincide = true
					break
				}
			}
			if !coincide {
				continue
			}
		}
		if query.HasFrom && c.OccurredAt.Before(query.From) {
			continue
		}
		if query.HasTo && c.OccurredAt.After(query.To) {
			continue
		}
		items = append(items, copiarAportacion(*c))
	}
	// Orden del contrato: más recientes primero.
	sort.Slice(items, func(i, j int) bool {
		if !items[i].OccurredAt.Equal(items[j].OccurredAt) {
			return items[i].OccurredAt.After(items[j].OccurredAt)
		}
		return items[i].ID > items[j].ID
	})
	inicio := 0
	if startSK != "" {
		for i, c := range items {
			if c.ID == startSK {
				inicio = i + 1
				break
			}
		}
	}
	items = items[inicio:]
	if limit > 0 && len(items) > limit {
		return items[:limit], items[limit-1].ID, nil
	}
	return items, "", nil
}

func (a aportacionesDelLibro) LoadContribution(_ context.Context, userID, aportacionID string) (*appContrib.Contribution, error) {
	a.libro.mu.Lock()
	defer a.libro.mu.Unlock()
	c, ok := a.libro.aportaciones[userID][aportacionID]
	if !ok {
		return nil, nil
	}
	copia := copiarAportacion(*c)
	return &copia, nil
}

func (a aportacionesDelLibro) ReverseContribution(_ context.Context, write appContrib.ReverseWrite) (*appContrib.StoredResponse, error) {
	a.libro.mu.Lock()
	defer a.libro.mu.Unlock()

	clave := write.UserID + "|" + write.Operation + "|" + write.IdempotencyKey
	if previo, ok := a.libro.idemAportacion[clave]; ok {
		if previo.PayloadHash != write.PayloadHash {
			return nil, appContrib.ErrConflict
		}
		return &appContrib.StoredResponse{
			Status: previo.HTTPStatus, Body: append([]byte(nil), previo.ResponseBody...),
			Location: previo.Location, Replayed: true,
		}, nil
	}

	actual, ok := a.libro.aportaciones[write.UserID][write.Previous.ID]
	if !ok {
		return nil, appContrib.ErrNotFound
	}
	if actual.Status != appContrib.StatusActive {
		return nil, appContrib.ErrAlreadyReversed
	}
	for _, asignacion := range actual.Allocations {
		meta, ok := a.libro.metas[write.UserID][asignacion.GoalID]
		if !ok || meta.SavedCents < asignacion.AmountCents || meta.ActiveContributions < 1 {
			return nil, appContrib.ErrConflict
		}
	}
	for _, asignacion := range actual.Allocations {
		meta := a.libro.metas[write.UserID][asignacion.GoalID]
		meta.SavedCents -= asignacion.AmountCents
		meta.ActiveContributions--
		meta.Version++
	}
	copia := copiarAportacion(write.Next)
	a.libro.aportaciones[write.UserID][write.Next.ID] = &copia
	a.libro.idemAportacion[clave] = appContrib.IdempotencyRecord{
		PayloadHash: write.PayloadHash, HTTPStatus: 200,
		ResponseBody: append([]byte(nil), write.ResponseBody...), Location: write.Location, ExpiresAt: write.ExpiresAt,
	}
	return &appContrib.StoredResponse{Status: 200, Body: write.ResponseBody, Location: write.Location}, nil
}

// presupuestosDelLibro implementa `budgetset.Store` y `budgetset.WriteStore` (E048/E049).
type presupuestosDelLibro struct{ libro *libroMayor }

func (p presupuestosDelLibro) LoadSet(_ context.Context, userID, periodo string) (*appBudget.Set, error) {
	p.libro.mu.Lock()
	defer p.libro.mu.Unlock()
	conjunto, ok := p.libro.presupuestos[userID][periodo]
	if !ok {
		return nil, nil
	}
	copia := copiarConjunto(*conjunto)
	return &copia, nil
}

func (p presupuestosDelLibro) ListAllSets(_ context.Context, userID string) ([]appBudget.Set, error) {
	p.libro.mu.Lock()
	defer p.libro.mu.Unlock()
	salida := make([]appBudget.Set, 0, len(p.libro.presupuestos[userID]))
	for _, conjunto := range p.libro.presupuestos[userID] {
		salida = append(salida, copiarConjunto(*conjunto))
	}
	sort.Slice(salida, func(i, j int) bool { return salida[i].Period < salida[j].Period })
	return salida, nil
}

func (p presupuestosDelLibro) ReplaceSet(_ context.Context, userID string, conjunto *appBudget.Set, versionEsperada int64) error {
	p.libro.mu.Lock()
	defer p.libro.mu.Unlock()
	if p.libro.presupuestos[userID] == nil {
		p.libro.presupuestos[userID] = map[string]*appBudget.Set{}
	}
	actual, existe := p.libro.presupuestos[userID][conjunto.Period]
	if versionEsperada == 0 {
		if existe {
			return appBudget.ErrVersionConflict
		}
	} else if !existe || actual.Version != versionEsperada {
		return appBudget.ErrVersionConflict
	}
	copia := copiarConjunto(*conjunto)
	p.libro.presupuestos[userID][conjunto.Period] = &copia
	return nil
}

// usoDelLibro implementa `budgetusage.Store` (E050).
type usoDelLibro struct{ libro *libroMayor }

func (u usoDelLibro) LoadTimezone(context.Context, string) (string, error) {
	return u.libro.zona, nil
}

func (u usoDelLibro) LoadSet(ctx context.Context, userID, periodo string) (*appBudget.Set, error) {
	return presupuestosDelLibro(u).LoadSet(ctx, userID, periodo)
}

func (u usoDelLibro) AggregateExpenses(_ context.Context, userID string, desde, hasta time.Time) (map[string]int64, error) {
	u.libro.mu.Lock()
	defer u.libro.mu.Unlock()
	gastos := map[string]int64{}
	for _, m := range u.libro.movimientos[userID] {
		if m.Type != appMovements.TypeExpense {
			continue
		}
		if m.OccurredAt.Before(desde) || !m.OccurredAt.Before(hasta) {
			continue
		}
		gastos[m.CategoryID] += m.AmountCents
	}
	return gastos, nil
}

// pagosDelLibro implementa `recurring.PaymentStore` (E037).
type pagosDelLibro struct{ libro *libroMayor }

func (p pagosDelLibro) LoadRule(_ context.Context, userID, reglaID string) (*appRecurring.Payment, error) {
	p.libro.mu.Lock()
	defer p.libro.mu.Unlock()
	regla, ok := p.libro.reglas[userID][reglaID]
	if !ok {
		return nil, nil
	}
	copia := *regla
	return &copia, nil
}

func (p pagosDelLibro) LoadTimezone(context.Context, string) (string, error) {
	return p.libro.zona, nil
}

func (p pagosDelLibro) Pay(_ context.Context, write appRecurring.PayWrite) (*appRecurring.PayResult, error) {
	p.libro.mu.Lock()
	defer p.libro.mu.Unlock()

	clave := write.UserID + "|" + write.Operation + "|" + write.IdempotencyKey
	if previo, ok := p.libro.idemPago[clave]; ok {
		if previo.hash != write.PayloadHash {
			return nil, appRecurring.ErrConflict
		}
		return &appRecurring.PayResult{
			Status: previo.status, Body: append([]byte(nil), previo.cuerpo...), Replayed: true,
		}, nil
	}
	regla, ok := p.libro.reglas[write.UserID][write.RuleID]
	if !ok || regla.ID != write.RuleID {
		return nil, appRecurring.ErrNotFound
	}
	if !regla.Active {
		return nil, appRecurring.ErrRuleInactive
	}
	llaveFecha := write.UserID + "|" + write.RuleID + "|" + write.LocalDate
	if p.libro.fechasPagadas[llaveFecha] {
		return nil, appRecurring.ErrDuplicatePayment
	}
	p.libro.fechasPagadas[llaveFecha] = true
	p.libro.movimientos[write.UserID] = append(p.libro.movimientos[write.UserID], write.Movement)
	p.libro.idemPago[clave] = registroPago{
		hash: write.PayloadHash, cuerpo: append([]byte(nil), write.ResponseBody...), status: write.Status,
	}
	return &appRecurring.PayResult{Status: write.Status, Body: append([]byte(nil), write.ResponseBody...)}, nil
}

// metasDelLibro implementa los métodos de `goals.Store` que usa E042. Los demás quedan
// delegados al `Store` embebido (nulo) porque esta suite no ejecuta E038/E039/E041.
type metasDelLibro struct {
	appGoals.Store
	libro *libroMayor
}

func (m metasDelLibro) LoadGoal(_ context.Context, userID, metaID string) (*domainGoal.Goal, error) {
	m.libro.mu.Lock()
	defer m.libro.mu.Unlock()
	meta, ok := m.libro.metas[userID][metaID]
	if !ok {
		return nil, nil
	}
	return &domainGoal.Goal{
		ID: meta.ID, UserID: userID, Name: meta.Name,
		TargetAmount: money.MustNew(meta.TargetCents, "MXN"),
		SavedAmount:  money.MustNew(meta.SavedCents, "MXN"),
		OpeningSaved: money.MustNew(meta.OpeningCents, "MXN"),
		Status:       domainGoal.Status(meta.Status),
		Deleted:      meta.Deleted,
		// Un ítem heredado sin contador se interpreta como cero (spec de E042).
		ActiveContributions: meta.ActiveContributions,
		Version:             meta.Version,
	}, nil
}

func (m metasDelLibro) DeleteGoal(_ context.Context, write appGoals.DeleteWrite) error {
	m.libro.mu.Lock()
	defer m.libro.mu.Unlock()
	meta, ok := m.libro.metas[write.UserID][write.GoalID]
	if !ok || meta.Deleted {
		return appGoals.ErrNotFound
	}
	if meta.ActiveContributions > 0 {
		return appGoals.ErrGoalActiveContributions
	}
	meta.Deleted = true
	meta.Status = "archived"
	meta.SavedCents = meta.OpeningCents
	meta.Version++
	meta.SinContador = false
	m.libro.escriturasMeta++
	return nil
}

// ---------------------------------------------------------------------------------------------
// Composición de servicios reales
// ---------------------------------------------------------------------------------------------

type serviciosOleada3 struct {
	libro        *libroMayor
	summary      *appSummary.Service
	dashboard    *appDashboard.Service
	aportaciones *appContrib.Service
	presupuestos *appBudget.Service
	uso          *appUsage.Service
	pagos        *appRecurring.PaymentService
	metas        *appGoals.Service
}

// secuenciaIDs genera UUID con forma válida y secuencia determinista.
func secuenciaIDs(prefijo string) func() (string, error) {
	var mu sync.Mutex
	n := 100
	return func() (string, error) {
		mu.Lock()
		defer mu.Unlock()
		n++
		return fmt.Sprintf("%s%012d", prefijo, n), nil
	}
}

func montarServicios(libro *libroMayor) *serviciosOleada3 {
	ahora := func() time.Time { return liAhora }
	summary := appSummary.NewService(resumenDelLibro{libro}).WithClock(ahora)
	return &serviciosOleada3{
		libro:     libro,
		summary:   summary,
		dashboard: appDashboard.NewService(summary, dashboardDelLibro{libro}),
		aportaciones: appContrib.NewService(aportacionesDelLibro{libro}).
			WithCursorKey([]byte(liClaveCursor)).
			WithClock(ahora).
			WithIDGenerator(secuenciaIDs("018f90ef-08e5-7a68-b09f-")),
		presupuestos: appBudget.NewService(presupuestosDelLibro{libro}).
			WithCursorKey([]byte(liClaveCursor)).
			WithClock(ahora),
		uso: appUsage.NewService(usoDelLibro{libro}).WithClock(ahora),
		pagos: appRecurring.NewPaymentService(pagosDelLibro{libro}).
			WithClock(ahora).
			WithIDGenerator(secuenciaIDs("018f90ef-08e5-7a68-b09f-")).
			WithLocationResolver(func(string) *time.Location {
				return time.FixedZone("America/Mexico_City", -6*60*60)
			}),
		metas: appGoals.NewService(metasDelLibro{libro: libro}).WithClock(ahora),
	}
}

// ---------------------------------------------------------------------------------------------
// Utilidades de aserción
// ---------------------------------------------------------------------------------------------

func exigirCentavos(t *testing.T, nombre string, obtenido, esperado int64) {
	t.Helper()
	if obtenido != esperado {
		t.Fatalf("%s: esperado %d centavos, obtenido %d", nombre, esperado, obtenido)
	}
}

func exigirWire(t *testing.T, nombre, obtenido, esperado string) {
	t.Helper()
	if obtenido != esperado {
		t.Fatalf("%s: esperado %q, obtenido %q", nombre, esperado, obtenido)
	}
}

func decodificarAportacion(t *testing.T, cuerpo []byte) appContrib.WireContribution {
	t.Helper()
	var wire appContrib.WireContribution
	if err := json.Unmarshal(cuerpo, &wire); err != nil {
		t.Fatalf("respuesta de aportacion no es JSON: %v", err)
	}
	return wire
}

// ---------------------------------------------------------------------------------------------
// E026 + E043 + E044 + E045 + E046 + E047 + E051
// ---------------------------------------------------------------------------------------------

func TestIntegracionAportacionesSumaUnaSolaVezYSeRevierte(t *testing.T) {
	libro := nuevoLibroMayor("UTC")
	libro.sembrarMeta(liUsuario, metaLibro{
		ID: liMetaA, Name: "Viaje", TargetCents: 100000, SavedCents: 0, OpeningCents: 0,
		Currency: "MXN", TargetDate: "2026-12-31", Status: "active", Version: 1,
	})
	libro.sembrarMeta(liUsuario, metaLibro{
		ID: liMetaB, Name: "Emergencias", TargetCents: 50000, SavedCents: 10000, OpeningCents: 10000,
		Currency: "MXN", TargetDate: "2027-06-30", Status: "active", Version: 3,
	})
	libro.sembrarMovimiento(liUsuario, appMovements.Movement{
		ID: liMovIngreso, Type: appMovements.TypeIncome, AmountCents: 500000, Currency: "MXN",
		CategoryID: "ingreso", Description: "Nomina", OccurredAt: liDia(3), Version: 1,
	})
	libro.sembrarMovimiento(liUsuario, appMovements.Movement{
		ID: liMovGasto, Type: appMovements.TypeExpense, AmountCents: 100000, Currency: "MXN",
		CategoryID: "comida", Description: "Super", OccurredAt: liDia(4), Version: 1,
	})
	libro.sembrarMovimiento(liUsuario, appMovements.Movement{
		ID: liMovAhorro, Type: appMovements.TypeSaving, AmountCents: 30000, Currency: "MXN",
		CategoryID: "ahorro", Description: "Ahorro propio", OccurredAt: liDia(5), Version: 1,
	})

	senal := montarServicios(libro)
	ctx := context.Background()

	// E043: el preview exige suma exacta y NO escribe.
	preview, err := senal.aportaciones.Preview(ctx, appContrib.PreviewCommand{
		UserID: liUsuario, Amount: "200.00", Currency: "MXN",
		Allocations: []appContrib.WireAllocation{{GoalID: liMetaA, Amount: "200.00"}},
	})
	if err != nil {
		t.Fatalf("preview valido: %v", err)
	}
	exigirCentavos(t, "preview.remaining", preview.RemainingCents, 0)
	if !preview.CanCommit {
		t.Fatal("un reparto exacto y con capacidad debe poder confirmarse")
	}
	if _, err := senal.aportaciones.Preview(ctx, appContrib.PreviewCommand{
		UserID: liUsuario, Amount: "200.00", Currency: "MXN",
		Allocations: []appContrib.WireAllocation{{GoalID: liMetaA, Amount: "150.00"}},
	}); err == nil {
		t.Fatal("E043 debe rechazar una suma incompleta igual que E044")
	} else {
		var violacion *appContrib.FieldViolation
		if !errors.As(err, &violacion) || violacion.Reason != "sum_mismatch" {
			t.Fatalf("E043: esperado sum_mismatch, obtenido %v", err)
		}
	}
	if libro.numeroAportaciones(liUsuario) != 0 || libro.numeroMovimientos(liUsuario) != 3 {
		t.Fatal("E043 no puede crear aportaciones ni movimientos")
	}

	// E044 es atómica: si una asignación no cabe, no se aplica ninguna.
	_, err = senal.aportaciones.Create(ctx, appContrib.CreateCommand{
		UserID: liUsuario, IdempotencyKey: "idem-aportacion-000001", Amount: "1450.00",
		Currency: "MXN", OccurredAt: "2026-11-10T10:00:00Z",
		Allocations: []appContrib.WireAllocation{
			{GoalID: liMetaA, Amount: "1000.00"},
			{GoalID: liMetaB, Amount: "450.00"},
		},
	})
	if !errors.Is(err, appContrib.ErrCapacityExceeded) {
		t.Fatalf("esperado capacidad excedida, obtenido %v", err)
	}
	if libro.numeroAportaciones(liUsuario) != 0 || libro.numeroMovimientos(liUsuario) != 3 {
		t.Fatal("una aportacion rechazada no puede dejar rastro")
	}
	if metaA, _ := libro.leerMeta(liUsuario, liMetaA); metaA.SavedCents != 0 {
		t.Fatalf("la meta A no debe cambiar: %d", metaA.SavedCents)
	}

	// Tres aportaciones reales: A sola, B sola y una dividida.
	alta := func(clave, importe, momento string, asignaciones []appContrib.WireAllocation) appContrib.WireContribution {
		t.Helper()
		respuesta, err := senal.aportaciones.Create(ctx, appContrib.CreateCommand{
			UserID: liUsuario, IdempotencyKey: clave, Amount: importe, Currency: "MXN",
			OccurredAt: momento, Allocations: asignaciones,
		})
		if err != nil {
			t.Fatalf("alta %s: %v", clave, err)
		}
		if respuesta.Status != 201 || respuesta.Replayed {
			t.Fatalf("alta %s: esperado 201 nuevo, obtenido %d/%v", clave, respuesta.Status, respuesta.Replayed)
		}
		return decodificarAportacion(t, respuesta.Body)
	}
	aportacion1 := alta("idem-aportacion-000001", "200.00", "2026-11-10T10:00:00Z",
		[]appContrib.WireAllocation{{GoalID: liMetaA, Amount: "200.00"}})
	aportacion2 := alta("idem-aportacion-000002", "50.00", "2026-11-11T10:00:00Z",
		[]appContrib.WireAllocation{{GoalID: liMetaB, Amount: "50.00"}})
	aportacion3 := alta("idem-aportacion-000003", "25.00", "2026-11-12T10:00:00Z",
		[]appContrib.WireAllocation{{GoalID: liMetaA, Amount: "10.00"}, {GoalID: liMetaB, Amount: "15.00"}})
	exigirWire(t, "aportacion1.amount", aportacion1.Amount, "200.00")
	exigirWire(t, "aportacion1.status", aportacion1.Status, appContrib.StatusActive)
	exigirCentavos(t, "aportacion1.version", aportacion1.Version, 1)

	// Replay de E044: mismo cuerpo, sin movimiento adicional.
	replay, err := senal.aportaciones.Create(ctx, appContrib.CreateCommand{
		UserID: liUsuario, IdempotencyKey: "idem-aportacion-000001", Amount: "200.00",
		Currency: "MXN", OccurredAt: "2026-11-10T10:00:00Z",
		Allocations: []appContrib.WireAllocation{{GoalID: liMetaA, Amount: "200.00"}},
	})
	if err != nil || !replay.Replayed {
		t.Fatalf("replay de E044: %v/%v", err, replay != nil && replay.Replayed)
	}
	if libro.numeroMovimientos(liUsuario) != 6 {
		t.Fatalf("el replay no puede duplicar el movimiento: %d", libro.numeroMovimientos(liUsuario))
	}

	// El movimiento vinculado conserva el vínculo y no se suma aparte.
	var ligado *appMovements.Movement
	for _, m := range libro.movimientosDe(liUsuario) {
		if m.ContributionID == aportacion1.ID {
			copia := m
			ligado = &copia
		}
	}
	if ligado == nil {
		t.Fatal("E044 debe crear el movimiento de ahorro vinculado")
	}
	if ligado.Type != appMovements.TypeSaving || ligado.AmountCents != 20000 || ligado.RecurringPaymentID != "" {
		t.Fatalf("movimiento vinculado inesperado: %+v", *ligado)
	}

	// E026: savings = 300.00 independientes + 275.00 de aportaciones activas = 575.00.
	resumen, err := senal.summary.Summary(ctx, liUsuario, "2026-11")
	if err != nil {
		t.Fatalf("E026: %v", err)
	}
	exigirCentavos(t, "E026.income", resumen.Income, 500000)
	exigirCentavos(t, "E026.expenses", resumen.Expenses, 100000)
	exigirCentavos(t, "E026.savings", resumen.Savings, 57500)
	exigirCentavos(t, "E026.net", resumen.Net, 342500)
	wireResumen := resumen.Wire()
	exigirWire(t, "E026.wire.savings", wireResumen.Savings, "575.00")
	exigirWire(t, "E026.wire.net", wireResumen.Net, "3425.00")

	// E051 comparte el resumen y proyecta metas y movimientos recientes.
	tablero, err := senal.dashboard.Get(ctx, liUsuario, "2026-11")
	if err != nil {
		t.Fatalf("E051: %v", err)
	}
	exigirWire(t, "E051.summary.savings", tablero.Summary.Savings, "575.00")
	exigirWire(t, "E051.summary.net", tablero.Summary.Net, "3425.00")
	if len(tablero.Goals) != 2 {
		t.Fatalf("E051: esperadas 2 metas activas, obtenidas %d", len(tablero.Goals))
	}
	exigirWire(t, "E051.metaA.saved", tablero.Goals[0].SavedAmount, "210.00")
	exigirWire(t, "E051.metaA.opening", tablero.Goals[0].OpeningSaved, "0.00")
	exigirWire(t, "E051.metaB.saved", tablero.Goals[1].SavedAmount, "165.00")
	exigirWire(t, "E051.metaB.opening", tablero.Goals[1].OpeningSaved, "100.00")
	if len(tablero.LatestMovements) != 6 {
		t.Fatalf("E051: esperados 6 movimientos, obtenidos %d", len(tablero.LatestMovements))
	}
	reciente := tablero.LatestMovements[0]
	if reciente.ContributionID == nil || *reciente.ContributionID != aportacion3.ID {
		t.Fatalf("E051 debe devolver primero el movimiento vinculado a la aportacion mas reciente: %+v", reciente)
	}
	if reciente.RecurringPaymentID != nil {
		t.Fatal("un movimiento de aportacion no puede declarar regla recurrente")
	}

	// E045 pagina sin duplicados, con cursor ligado al usuario y a los filtros.
	pagina1, err := senal.aportaciones.List(ctx, appContrib.ListQuery{UserID: liUsuario, Limit: 2})
	if err != nil {
		t.Fatalf("E045 pagina 1: %v", err)
	}
	if len(pagina1.Items) != 2 || pagina1.NextCursor == "" {
		t.Fatalf("E045: esperada pagina de 2 con cursor, obtenido %d/%q", len(pagina1.Items), pagina1.NextCursor)
	}
	if pagina1.Items[0].ID != aportacion3.ID || pagina1.Items[1].ID != aportacion2.ID {
		t.Fatalf("E045: orden inesperado %s/%s", pagina1.Items[0].ID, pagina1.Items[1].ID)
	}
	pagina2, err := senal.aportaciones.List(ctx, appContrib.ListQuery{
		UserID: liUsuario, Limit: 2, Cursor: pagina1.NextCursor,
	})
	if err != nil {
		t.Fatalf("E045 pagina 2: %v", err)
	}
	if len(pagina2.Items) != 1 || pagina2.NextCursor != "" || pagina2.Items[0].ID != aportacion1.ID {
		t.Fatalf("E045: segunda pagina inesperada: %d/%q", len(pagina2.Items), pagina2.NextCursor)
	}
	vistos := map[string]bool{}
	for _, c := range append(append([]appContrib.Contribution{}, pagina1.Items...), pagina2.Items...) {
		if vistos[c.ID] {
			t.Fatalf("E045 duplico la aportacion %s entre paginas", c.ID)
		}
		vistos[c.ID] = true
	}
	if ajenas, err := senal.aportaciones.List(ctx, appContrib.ListQuery{UserID: liOtro, Limit: 2}); err != nil || len(ajenas.Items) != 0 {
		t.Fatalf("E045 no puede listar aportaciones ajenas: %v/%d", err, len(ajenas.Items))
	}
	if _, err := senal.aportaciones.List(ctx, appContrib.ListQuery{
		UserID: liOtro, Limit: 2, Cursor: pagina1.NextCursor,
	}); !errors.Is(err, appContrib.ErrInvalidRequest) {
		t.Fatalf("E045: un cursor de otro titular debe rechazarse, obtenido %v", err)
	}
	porMeta, err := senal.aportaciones.List(ctx, appContrib.ListQuery{UserID: liUsuario, Limit: 10, GoalID: liMetaA})
	if err != nil || len(porMeta.Items) != 2 {
		t.Fatalf("E045 filtro por meta: %v/%d", err, len(porMeta.Items))
	}
	revertidas, err := senal.aportaciones.List(ctx, appContrib.ListQuery{UserID: liUsuario, Limit: 10, Status: appContrib.StatusReversed})
	if err != nil || len(revertidas.Items) != 0 {
		t.Fatalf("E045: no hay revertidas todavia: %v/%d", err, len(revertidas.Items))
	}

	// E046: detalle propio y 404 homogeneo para otro titular.
	detalle, err := senal.aportaciones.Get(ctx, liUsuario, aportacion1.ID)
	if err != nil || detalle == nil || detalle.AmountCents != 20000 {
		t.Fatalf("E046 detalle: %v/%+v", err, detalle)
	}
	if _, err := senal.aportaciones.Get(ctx, liOtro, aportacion1.ID); !errors.Is(err, appContrib.ErrNotFound) {
		t.Fatalf("E046: la aportacion ajena debe ser 404, obtenido %v", err)
	}

	// E047: reversión única. La meta A conserva solo la parte de la aportación 3.
	reversion, err := senal.aportaciones.Reverse(ctx, appContrib.ReverseCommand{
		UserID: liUsuario, ContributionID: aportacion1.ID,
		IdempotencyKey: "idem-reversion-000001", Reason: "Correccion autorizada",
	})
	if err != nil {
		t.Fatalf("E047: %v", err)
	}
	revertida := decodificarAportacion(t, reversion.Body)
	exigirWire(t, "E047.status", revertida.Status, appContrib.StatusReversed)
	exigirCentavos(t, "E047.version", revertida.Version, 2)
	metaA, _ := libro.leerMeta(liUsuario, liMetaA)
	exigirCentavos(t, "E047.metaA.saved", metaA.SavedCents, 1000)
	exigirCentavos(t, "E047.metaA.active", metaA.ActiveContributions, 1)

	// El movimiento histórico NO se borra: la autoridad es el estado de la aportación.
	if libro.numeroMovimientos(liUsuario) != 6 {
		t.Fatalf("E047 no puede borrar movimientos: %d", libro.numeroMovimientos(liUsuario))
	}

	resumen, err = senal.summary.Summary(ctx, liUsuario, "2026-11")
	if err != nil {
		t.Fatalf("E026 tras reversion: %v", err)
	}
	exigirCentavos(t, "E026.revertido.savings", resumen.Savings, 37500)
	exigirCentavos(t, "E026.revertido.net", resumen.Net, 362500)
	exigirWire(t, "E026.revertido.wire.savings", resumen.Wire().Savings, "375.00")

	tablero, err = senal.dashboard.Get(ctx, liUsuario, "2026-11")
	if err != nil {
		t.Fatalf("E051 tras reversion: %v", err)
	}
	exigirWire(t, "E051.revertido.metaA.saved", tablero.Goals[0].SavedAmount, "10.00")
	exigirWire(t, "E051.revertido.savings", tablero.Summary.Savings, "375.00")

	revertidas, err = senal.aportaciones.List(ctx, appContrib.ListQuery{UserID: liUsuario, Limit: 10, Status: appContrib.StatusReversed})
	if err != nil || len(revertidas.Items) != 1 || revertidas.Items[0].ID != aportacion1.ID {
		t.Fatalf("E045 tras reversion: %v/%d", err, len(revertidas.Items))
	}

	// Replay de la misma clave y doble reversión con otra clave.
	if repetida, err := senal.aportaciones.Reverse(ctx, appContrib.ReverseCommand{
		UserID: liUsuario, ContributionID: aportacion1.ID,
		IdempotencyKey: "idem-reversion-000001", Reason: "Correccion autorizada",
	}); err != nil || !repetida.Replayed {
		t.Fatalf("E047 replay: %v/%v", err, repetida != nil && repetida.Replayed)
	}
	if _, err := senal.aportaciones.Reverse(ctx, appContrib.ReverseCommand{
		UserID: liUsuario, ContributionID: aportacion1.ID,
		IdempotencyKey: "idem-reversion-000002", Reason: "Segundo intento",
	}); !errors.Is(err, appContrib.ErrAlreadyReversed) {
		t.Fatalf("E047: la doble reversion debe ser 409, obtenido %v", err)
	}
	metaA, _ = libro.leerMeta(liUsuario, liMetaA)
	exigirCentavos(t, "E047.doble.metaA.saved", metaA.SavedCents, 1000)

	// Segunda reversión válida: la meta A queda sin aportaciones vigentes.
	if _, err := senal.aportaciones.Reverse(ctx, appContrib.ReverseCommand{
		UserID: liUsuario, ContributionID: aportacion3.ID,
		IdempotencyKey: "idem-reversion-000003", Reason: "Ajuste final",
	}); err != nil {
		t.Fatalf("E047 segunda reversion: %v", err)
	}
	metaA, _ = libro.leerMeta(liUsuario, liMetaA)
	exigirCentavos(t, "E047.final.metaA.saved", metaA.SavedCents, 0)
	exigirCentavos(t, "E047.final.metaA.active", metaA.ActiveContributions, 0)
	metaB, _ := libro.leerMeta(liUsuario, liMetaB)
	exigirCentavos(t, "E047.final.metaB.saved", metaB.SavedCents, 15000)

	resumen, err = senal.summary.Summary(ctx, liUsuario, "2026-11")
	if err != nil {
		t.Fatalf("E026 final: %v", err)
	}
	exigirCentavos(t, "E026.final.savings", resumen.Savings, 35000)
	exigirCentavos(t, "E026.final.net", resumen.Net, 365000)
	exigirWire(t, "E026.final.wire.net", resumen.Wire().Net, "3650.00")
}

// ---------------------------------------------------------------------------------------------
// E042: 409 con aportaciones vigentes, tombstone idempotente, meta heredada sin contador
// ---------------------------------------------------------------------------------------------

func TestIntegracionE042TombstoneYAportacionesVigentes(t *testing.T) {
	libro := nuevoLibroMayor("UTC")
	libro.sembrarMeta(liUsuario, metaLibro{
		ID: liMetaA, Name: "Viaje", TargetCents: 100000, SavedCents: 0, OpeningCents: 0,
		Currency: "MXN", TargetDate: "2026-12-31", Status: "active", Version: 1,
	})
	libro.sembrarMeta(liUsuario, metaLibro{
		ID: liMetaB, Name: "Emergencias", TargetCents: 100000, SavedCents: 0, OpeningCents: 0,
		Currency: "MXN", TargetDate: "2027-06-30", Status: "active", Version: 1,
	})
	// Ítem heredado: sin contador de aportaciones y con ahorro previo.
	libro.sembrarMeta(liUsuario, metaLibro{
		ID: liMetaLegado, Name: "Heredada", TargetCents: 80000, SavedCents: 50000, OpeningCents: 30000,
		Currency: "MXN", TargetDate: "2027-01-31", Status: "active", Version: 7, SinContador: true,
	})
	senal := montarServicios(libro)
	ctx := context.Background()

	// Una aportación real crea el contador vigente que bloquea el borrado.
	respuesta, err := senal.aportaciones.Create(ctx, appContrib.CreateCommand{
		UserID: liUsuario, IdempotencyKey: "idem-aportacion-000010", Amount: "50.00",
		Currency: "MXN", OccurredAt: "2026-11-10T10:00:00Z",
		Allocations: []appContrib.WireAllocation{{GoalID: liMetaA, Amount: "50.00"}},
	})
	if err != nil {
		t.Fatalf("alta de la aportacion: %v", err)
	}
	aportacion := decodificarAportacion(t, respuesta.Body)

	if err := senal.metas.Delete(ctx, liUsuario, liMetaA); !errors.Is(err, appGoals.ErrGoalActiveContributions) {
		t.Fatalf("E042 con aportaciones vigentes debe ser 409, obtenido %v", err)
	}
	if metaA, _ := libro.leerMeta(liUsuario, liMetaA); metaA.Deleted {
		t.Fatal("un 409 no puede escribir el tombstone")
	}
	if libro.escriturasDeMetas() != 0 {
		t.Fatalf("un 409 no puede escribir: %d", libro.escriturasDeMetas())
	}

	// Propiedad: otra cuenta nunca alcanza la meta (404) y tampoco escribe.
	if err := senal.metas.Delete(ctx, liOtro, liMetaA); !errors.Is(err, appGoals.ErrNotFound) {
		t.Fatalf("E042 ajena debe ser 404, obtenido %v", err)
	}
	if err := senal.metas.Delete(ctx, liUsuario, "018f90ef-08e5-7a68-b09f-0000000000ff"); !errors.Is(err, appGoals.ErrNotFound) {
		t.Fatalf("E042 inexistente debe ser 404, obtenido %v", err)
	}
	if libro.escriturasDeMetas() != 0 {
		t.Fatalf("un 404 no puede escribir: %d", libro.escriturasDeMetas())
	}

	// Tras revertir la aportación, el borrado procede y deja el tombstone (204 sin cuerpo).
	if _, err := senal.aportaciones.Reverse(ctx, appContrib.ReverseCommand{
		UserID: liUsuario, ContributionID: aportacion.ID,
		IdempotencyKey: "idem-reversion-000010", Reason: "Liberar la meta",
	}); err != nil {
		t.Fatalf("reversion previa al borrado: %v", err)
	}
	if err := senal.metas.Delete(ctx, liUsuario, liMetaA); err != nil {
		t.Fatalf("E042 sin aportaciones debe ser 204, obtenido %v", err)
	}
	metaA, _ := libro.leerMeta(liUsuario, liMetaA)
	if !metaA.Deleted || metaA.Status != "archived" {
		t.Fatalf("tombstone incompleto: %+v", metaA)
	}
	exigirCentavos(t, "E042.tombstone.saved", metaA.SavedCents, 0)
	exigirCentavos(t, "E042.tombstone.version", metaA.Version, 4)
	if libro.escriturasDeMetas() != 1 {
		t.Fatalf("el tombstone debe escribirse una vez: %d", libro.escriturasDeMetas())
	}

	// Segundo DELETE: idempotente, sin nueva escritura.
	if err := senal.metas.Delete(ctx, liUsuario, liMetaA); err != nil {
		t.Fatalf("segundo DELETE debe ser idempotente: %v", err)
	}
	if libro.escriturasDeMetas() != 1 {
		t.Fatalf("un tombstone existente no se reescribe: %d", libro.escriturasDeMetas())
	}

	// Meta heredada sin contador: se borra y el tombstone conserva el ahorro de apertura.
	if err := senal.metas.Delete(ctx, liUsuario, liMetaLegado); err != nil {
		t.Fatalf("E042 sobre meta heredada debe proceder: %v", err)
	}
	legado, _ := libro.leerMeta(liUsuario, liMetaLegado)
	if !legado.Deleted || legado.Status != "archived" {
		t.Fatalf("tombstone heredado incompleto: %+v", legado)
	}
	exigirCentavos(t, "E042.heredada.saved", legado.SavedCents, 30000)
	exigirCentavos(t, "E042.heredada.version", legado.Version, 8)
	if libro.escriturasDeMetas() != 2 {
		t.Fatalf("escrituras tras la meta heredada: %d", libro.escriturasDeMetas())
	}

	// Meta sin historial: tombstone directo.
	if err := senal.metas.Delete(ctx, liUsuario, liMetaB); err != nil {
		t.Fatalf("E042 sobre meta sin aportaciones: %v", err)
	}

	// E051: ninguna meta borrada reaparece y el resumen no inventa ahorro.
	tablero, err := senal.dashboard.Get(ctx, liUsuario, "2026-11")
	if err != nil {
		t.Fatalf("E051 tras borrados: %v", err)
	}
	if len(tablero.Goals) != 0 {
		t.Fatalf("E051 no puede listar metas borradas: %d", len(tablero.Goals))
	}
	exigirWire(t, "E051.sin.metas.savings", tablero.Summary.Savings, "0.00")
	exigirWire(t, "E051.sin.metas.net", tablero.Summary.Net, "0.00")

	resumen, err := senal.summary.Summary(ctx, liUsuario, "2026-11")
	if err != nil {
		t.Fatalf("E026 sin datos: %v", err)
	}
	exigirCentavos(t, "E026.sin.datos", resumen.Savings+resumen.Income+resumen.Expenses+resumen.Net, 0)
}

// ---------------------------------------------------------------------------------------------
// E037: pago real, idempotencia, duplicado por fecha civil y reflejo en E026/E050
// ---------------------------------------------------------------------------------------------

func TestIntegracionE037PagoRealIdempotenciaYDuplicado(t *testing.T) {
	libro := nuevoLibroMayor("America/Mexico_City")
	libro.sembrarRegla(liUsuario, appRecurring.Payment{
		ID: liRegla, Name: "Renta", AmountCents: 100000, Currency: "MXN",
		Frequency:   domainIdentity.FrequencyMonthly,
		NextDueDate: time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC),
		Active:      true,
		Version:     1,
		Type:        appMovements.TypeExpense,
		StartDate:   time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC),
	})
	senal := montarServicios(libro)
	ctx := context.Background()

	base := appRecurring.PayCommand{
		UserID: liUsuario, RuleID: liRegla,
		IdempotencyKey: "idem-pago-000000001",
		PaidAt:         "2026-11-05T23:30:00-06:00",
		Amount:         "1000.00", Currency: "MXN",
	}

	// Pago real: 200 con el movimiento derivado exacto.
	resultado, err := senal.pagos.Pay(ctx, base)
	if err != nil {
		t.Fatalf("E037 pago: %v", err)
	}
	if resultado.Status != appRecurring.PayStatusOK || resultado.Replayed {
		t.Fatalf("E037: esperado 200 nuevo, obtenido %d/%v", resultado.Status, resultado.Replayed)
	}
	var movimiento appMovements.MovementResponse
	if err := json.Unmarshal(resultado.Body, &movimiento); err != nil {
		t.Fatalf("E037 respuesta no JSON: %v", err)
	}
	exigirWire(t, "E037.type", movimiento.Type, appMovements.TypeExpense)
	exigirWire(t, "E037.amount", movimiento.Amount, "1000.00")
	exigirWire(t, "E037.category", movimiento.CategoryID, "otros")
	exigirWire(t, "E037.description", movimiento.Description, "Renta")
	exigirWire(t, "E037.occurredAt", movimiento.OccurredAt, "2026-11-06T05:30:00.000Z")
	if movimiento.RecurringPaymentID == nil || *movimiento.RecurringPaymentID != liRegla {
		t.Fatalf("el pago debe quedar vinculado a la regla: %+v", movimiento)
	}
	if movimiento.ContributionID != nil {
		t.Fatal("un pago recurrente no declara aportacion")
	}
	if libro.numeroMovimientos(liUsuario) != 1 {
		t.Fatalf("E037 debe persistir un movimiento: %d", libro.numeroMovimientos(liUsuario))
	}
	if !libro.pagoRegistrado(liUsuario, liRegla, "2026-11-05") {
		t.Fatal("el control de duplicado debe usar el dia civil del titular")
	}

	// Replay: mismos bytes, sin escritura nueva.
	replay, err := senal.pagos.Pay(ctx, base)
	if err != nil || !replay.Replayed || string(replay.Body) != string(resultado.Body) {
		t.Fatalf("E037 replay: %v/%v", err, replay != nil && replay.Replayed)
	}
	if libro.numeroMovimientos(liUsuario) != 1 {
		t.Fatalf("el replay no puede duplicar el pago: %d", libro.numeroMovimientos(liUsuario))
	}

	// Misma clave con otra carga: 409 sin escritura.
	conflicto := base
	conflicto.Amount = "900.00"
	if _, err := senal.pagos.Pay(ctx, conflicto); !errors.Is(err, appRecurring.ErrConflict) {
		t.Fatalf("E037 clave reutilizada debe ser 409, obtenido %v", err)
	}

	// Misma regla y mismo dia civil con otra clave: duplicado 409.
	duplicado := base
	duplicado.IdempotencyKey = "idem-pago-000000002"
	duplicado.PaidAt = "2026-11-05T08:00:00-06:00"
	errDuplicado := error(nil)
	if _, errDuplicado = senal.pagos.Pay(ctx, duplicado); !errors.Is(errDuplicado, appRecurring.ErrDuplicatePayment) {
		t.Fatalf("E037 duplicado por fecha debe ser 409, obtenido %v", errDuplicado)
	}
	// El centinela específico debe seguir siendo un conflicto del contrato (409), no un 400/500.
	if !errors.Is(errDuplicado, appRecurring.ErrConflict) {
		t.Fatalf("el duplicado debe seguir siendo un conflicto del contrato: %v", errDuplicado)
	}

	// El cambio de dia UTC no permite un segundo pago de la misma ocurrencia.
	// 2026-11-06T05:30Z sigue siendo dia 5 en America/Mexico_City.
	trampaUTC := base
	trampaUTC.IdempotencyKey = "idem-pago-000000003"
	trampaUTC.PaidAt = "2026-11-06T05:30:00Z"
	if _, err := senal.pagos.Pay(ctx, trampaUTC); !errors.Is(err, appRecurring.ErrDuplicatePayment) {
		t.Fatalf("E037: el dia UTC no puede habilitar otro pago, obtenido %v", err)
	}
	if libro.numeroMovimientos(liUsuario) != 1 {
		t.Fatalf("ningun intento rechazado puede escribir: %d", libro.numeroMovimientos(liUsuario))
	}

	// El dia civil siguiente sí es una ocurrencia distinta.
	siguiente := base
	siguiente.IdempotencyKey = "idem-pago-000000004"
	siguiente.PaidAt = "2026-11-06T00:30:00-06:00"
	if _, err := senal.pagos.Pay(ctx, siguiente); err != nil {
		t.Fatalf("E037 dia local 6: %v", err)
	}
	if libro.numeroMovimientos(liUsuario) != 2 {
		t.Fatalf("E037 debe persistir el segundo pago: %d", libro.numeroMovimientos(liUsuario))
	}

	// Regla inactiva: 409 sin escribir; regla ajena/inexistente: 404.
	libro.desactivarRegla(liUsuario, liRegla)
	inactiva := base
	inactiva.IdempotencyKey = "idem-pago-000000005"
	inactiva.PaidAt = "2026-11-07T00:30:00-06:00"
	if _, err := senal.pagos.Pay(ctx, inactiva); !errors.Is(err, appRecurring.ErrRuleInactive) {
		t.Fatalf("E037 regla inactiva debe ser 409, obtenido %v", err)
	}
	ajena := base
	ajena.UserID = liOtro
	ajena.IdempotencyKey = "idem-pago-000000006"
	ajena.PaidAt = "2026-11-07T00:30:00-06:00"
	if _, err := senal.pagos.Pay(ctx, ajena); !errors.Is(err, appRecurring.ErrNotFound) {
		t.Fatalf("E037 regla ajena debe ser 404, obtenido %v", err)
	}
	if libro.numeroMovimientos(liUsuario) != 2 {
		t.Fatalf("los rechazos no pueden escribir: %d", libro.numeroMovimientos(liUsuario))
	}

	// E026: los pagos son egresos reales del periodo.
	resumen, err := senal.summary.Summary(ctx, liUsuario, "2026-11")
	if err != nil {
		t.Fatalf("E026 tras pagos: %v", err)
	}
	exigirCentavos(t, "E037.E026.expenses", resumen.Expenses, 200000)
	exigirCentavos(t, "E037.E026.income", resumen.Income, 0)
	exigirCentavos(t, "E037.E026.savings", resumen.Savings, 0)
	exigirCentavos(t, "E037.E026.net", resumen.Net, -200000)
	exigirWire(t, "E037.E026.wire.net", resumen.Wire().Net, "-2000.00")

	// E050 consume el egreso real en la categoría del pago.
	conjunto, err := senal.presupuestos.Replace(ctx, appBudget.ReplaceCommand{
		UserID: liUsuario, IfMatch: `"b2026-11-0"`, Period: "2026-11", Currency: "MXN",
		Budgets: []appBudget.BudgetInput{{CategoryID: "otros", LimitAmount: "2500.00"}},
	})
	if err != nil {
		t.Fatalf("E049 para E050: %v", err)
	}
	exigirWire(t, "E049.etag", conjunto.ETag, `"b2026-11-1"`)
	uso, err := senal.uso.Usage(ctx, liUsuario, "2026-11")
	if err != nil {
		t.Fatalf("E050: %v", err)
	}
	if len(uso) != 1 {
		t.Fatalf("E050: esperada una categoria, obtenidas %d", len(uso))
	}
	exigirCentavos(t, "E050.otros.used", uso[0].UsedCents, 200000)
	exigirCentavos(t, "E050.otros.limit", uso[0].LimitCents, 250000)
	wireUso := appUsage.WireItems(uso)
	exigirWire(t, "E050.wire.used", wireUso[0].UsedAmount, "2000.00")

	// Un movimiento de ahorro de la misma categoría NO es egreso y no altera el uso.
	libro.sembrarMovimiento(liUsuario, appMovements.Movement{
		ID: "018f90ef-08e5-7a68-b09f-0000000000f9", Type: appMovements.TypeSaving,
		AmountCents: 5000, Currency: "MXN", CategoryID: "otros", Description: "Apartado",
		OccurredAt: liDia(8), Version: 1,
	})
	uso, err = senal.uso.Usage(ctx, liUsuario, "2026-11")
	if err != nil {
		t.Fatalf("E050 tras ahorro: %v", err)
	}
	exigirCentavos(t, "E050.otros.used.tras.ahorro", uso[0].UsedCents, 200000)
}

// ---------------------------------------------------------------------------------------------
// E048 + E049 + E050: ETag, 428/412, CAS y uso desde egresos reales
// ---------------------------------------------------------------------------------------------

func TestIntegracionE048E049E050ETagCASYExpenseReal(t *testing.T) {
	libro := nuevoLibroMayor("UTC")
	libro.sembrarMovimiento(liUsuario, appMovements.Movement{
		ID: "018f90ef-08e5-7a68-b09f-000000000101", Type: appMovements.TypeExpense,
		AmountCents: 25000, Currency: "MXN", CategoryID: "comida", Description: "Super",
		OccurredAt: liDia(10), Version: 1,
	})
	libro.sembrarMovimiento(liUsuario, appMovements.Movement{
		ID: "018f90ef-08e5-7a68-b09f-000000000102", Type: appMovements.TypeExpense,
		AmountCents: 10000, Currency: "MXN", CategoryID: "transporte", Description: "Camion",
		OccurredAt: liDia(11), Version: 1,
	})
	libro.sembrarMovimiento(liUsuario, appMovements.Movement{
		ID: "018f90ef-08e5-7a68-b09f-000000000103", Type: appMovements.TypeSaving,
		AmountCents: 5000, Currency: "MXN", CategoryID: "ahorro", Description: "Ahorro",
		OccurredAt: liDia(10), Version: 1,
	})
	senal := montarServicios(libro)
	ctx := context.Background()

	// E048 sin conjunto: ETag de versión cero y página vacía.
	vacia, err := senal.presupuestos.List(ctx, liUsuario, "2026-11", "", 0)
	if err != nil {
		t.Fatalf("E048 sin conjunto: %v", err)
	}
	exigirWire(t, "E048.vacia.etag", vacia.ETag, `"b2026-11-0"`)
	if len(vacia.Items) != 0 || vacia.NextCursor != "" {
		t.Fatalf("E048 sin conjunto debe estar vacia: %d/%q", len(vacia.Items), vacia.NextCursor)
	}
	if uso, err := senal.uso.Usage(ctx, liUsuario, "2026-11"); err != nil || len(uso) != 0 {
		t.Fatalf("E050 sin presupuesto no debe inventar filas: %v/%d", err, len(uso))
	}

	entrada := []appBudget.BudgetInput{
		{CategoryID: "comida", LimitAmount: "1000.00"},
		{CategoryID: "transporte", LimitAmount: "500.00"},
	}

	// E049: 428 sin If-Match, 400 con validador débil, 412 con otro periodo.
	if _, err := senal.presupuestos.Replace(ctx, appBudget.ReplaceCommand{
		UserID: liUsuario, Period: "2026-11", Currency: "MXN", Budgets: entrada,
	}); !errors.Is(err, appBudget.ErrPreconditionRequired) {
		t.Fatalf("E049 sin If-Match debe ser 428, obtenido %v", err)
	}
	if _, err := senal.presupuestos.Replace(ctx, appBudget.ReplaceCommand{
		UserID: liUsuario, IfMatch: `W/"b2026-11-0"`, Period: "2026-11", Currency: "MXN", Budgets: entrada,
	}); !errors.Is(err, appBudget.ErrPreconditionInvalid) {
		t.Fatalf("E049 con validador debil debe ser 400, obtenido %v", err)
	}
	if _, err := senal.presupuestos.Replace(ctx, appBudget.ReplaceCommand{
		UserID: liUsuario, IfMatch: `"b2026-10-0"`, Period: "2026-11", Currency: "MXN", Budgets: entrada,
	}); !errors.Is(err, appBudget.ErrVersionConflict) {
		t.Fatalf("E049 con ETag de otro periodo debe ser 412, obtenido %v", err)
	}

	// Creación con el ETag vigente.
	creado, err := senal.presupuestos.Replace(ctx, appBudget.ReplaceCommand{
		UserID: liUsuario, IfMatch: `"b2026-11-0"`, Period: "2026-11", Currency: "MXN", Budgets: entrada,
	})
	if err != nil {
		t.Fatalf("E049 creacion: %v", err)
	}
	exigirWire(t, "E049.creacion.etag", creado.ETag, `"b2026-11-1"`)
	if len(creado.Items) != 2 {
		t.Fatalf("E049: esperadas 2 categorias, obtenidas %d", len(creado.Items))
	}
	exigirWire(t, "E049.item.limit", creado.Wire()[0].LimitAmount, "1000.00")

	// E048 pagina de uno en uno con el ETag del conjunto.
	pagina1, err := senal.presupuestos.List(ctx, liUsuario, "2026-11", "", 1)
	if err != nil {
		t.Fatalf("E048 pagina 1: %v", err)
	}
	exigirWire(t, "E048.pagina1.etag", pagina1.ETag, `"b2026-11-1"`)
	if len(pagina1.Items) != 1 || pagina1.NextCursor == "" {
		t.Fatalf("E048: esperada una fila con cursor, obtenido %d/%q", len(pagina1.Items), pagina1.NextCursor)
	}
	pagina2, err := senal.presupuestos.List(ctx, liUsuario, "2026-11", pagina1.NextCursor, 1)
	if err != nil {
		t.Fatalf("E048 pagina 2: %v", err)
	}
	if len(pagina2.Items) != 1 || pagina2.NextCursor != "" {
		t.Fatalf("E048: segunda pagina inesperada %d/%q", len(pagina2.Items), pagina2.NextCursor)
	}
	if pagina1.Items[0].CategoryID == pagina2.Items[0].CategoryID {
		t.Fatal("E048 no puede repetir la categoria entre paginas")
	}
	if _, err := senal.presupuestos.List(ctx, liOtro, "2026-11", pagina1.NextCursor, 1); !errors.Is(err, appBudget.ErrInvalidRequest) {
		t.Fatalf("E048: el cursor de otro titular debe rechazarse, obtenido %v", err)
	}
	if _, err := senal.presupuestos.List(ctx, liUsuario, "2026-11", pagina1.NextCursor, 2); !errors.Is(err, appBudget.ErrInvalidRequest) {
		t.Fatalf("E048: el cursor esta ligado al filtro (limit), obtenido %v", err)
	}

	// E050: el uso consume los egresos reales del periodo y no el ahorro.
	uso, err := senal.uso.Usage(ctx, liUsuario, "2026-11")
	if err != nil {
		t.Fatalf("E050: %v", err)
	}
	if len(uso) != 2 {
		t.Fatalf("E050: esperadas 2 categorias, obtenidas %d", len(uso))
	}
	exigirWire(t, "E050.comida.id", uso[0].CategoryID, "comida")
	exigirCentavos(t, "E050.comida.used", uso[0].UsedCents, 25000)
	exigirCentavos(t, "E050.comida.limit", uso[0].LimitCents, 100000)
	exigirWire(t, "E050.comida.wire.used", appUsage.WireItems(uso)[0].UsedAmount, "250.00")
	exigirWire(t, "E050.transporte.id", uso[1].CategoryID, "transporte")
	exigirCentavos(t, "E050.transporte.used", uso[1].UsedCents, 10000)
	exigirCentavos(t, "E050.transporte.limit", uso[1].LimitCents, 50000)

	// ETag obsoleto: 412 sin sobrescribir.
	if _, err := senal.presupuestos.Replace(ctx, appBudget.ReplaceCommand{
		UserID: liUsuario, IfMatch: `"b2026-11-0"`, Period: "2026-11", Currency: "MXN", Budgets: entrada,
	}); !errors.Is(err, appBudget.ErrVersionConflict) {
		t.Fatalf("E049 con ETag obsoleto debe ser 412, obtenido %v", err)
	}

	// CAS concurrente desde la misma versión: una sola escritura.
	var wg sync.WaitGroup
	var mu sync.Mutex
	exitos, conflictos := 0, 0
	for i := 0; i < 2; i++ {
		categoria := fmt.Sprintf("cat%d", i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := senal.presupuestos.Replace(ctx, appBudget.ReplaceCommand{
				UserID: liUsuario, IfMatch: `"b2026-11-1"`, Period: "2026-11", Currency: "MXN",
				Budgets: []appBudget.BudgetInput{{CategoryID: categoria, LimitAmount: "100.00"}},
			})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				exitos++
			case errors.Is(err, appBudget.ErrVersionConflict):
				conflictos++
			default:
				t.Errorf("error inesperado en la carrera de E049: %v", err)
			}
		}()
	}
	wg.Wait()
	if exitos != 1 || conflictos != 1 {
		t.Fatalf("E049 CAS: esperado 1 exito y 1 conflicto, obtenido %d/%d", exitos, conflictos)
	}
	trasCarrera, err := senal.presupuestos.List(ctx, liUsuario, "2026-11", "", 25)
	if err != nil {
		t.Fatalf("E048 tras la carrera: %v", err)
	}
	exigirWire(t, "E049.carrera.etag", trasCarrera.ETag, `"b2026-11-2"`)
	if len(trasCarrera.Items) != 1 {
		t.Fatalf("E049 CAS no puede dejar dos conjuntos: %d", len(trasCarrera.Items))
	}

	// Reemplazo final: E050 refleja los límites vigentes y no oculta el sobregiro.
	if _, err := senal.presupuestos.Replace(ctx, appBudget.ReplaceCommand{
		UserID: liUsuario, IfMatch: `"b2026-11-2"`, Period: "2026-11", Currency: "MXN",
		Budgets: []appBudget.BudgetInput{{CategoryID: "comida", LimitAmount: "200.00"}},
	}); err != nil {
		t.Fatalf("E049 reemplazo final: %v", err)
	}
	uso, err = senal.uso.Usage(ctx, liUsuario, "2026-11")
	if err != nil {
		t.Fatalf("E050 final: %v", err)
	}
	if len(uso) != 1 {
		t.Fatalf("E050 final: esperada una categoria, obtenidas %d", len(uso))
	}
	exigirCentavos(t, "E050.final.used", uso[0].UsedCents, 25000)
	exigirCentavos(t, "E050.final.limit", uso[0].LimitCents, 20000)
	wireFinal := appUsage.WireItems(uso)[0]
	exigirWire(t, "E050.final.wire.used", wireFinal.UsedAmount, "250.00")
	exigirWire(t, "E050.final.wire.limit", wireFinal.LimitAmount, "200.00")
}
