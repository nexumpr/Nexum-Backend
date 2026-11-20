package application_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	appContrib "github.com/nexumpr/Nexum-Backend/internal/application/contributions"
	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
)

// almacenAportaciones es un doble con estado del puerto. No simula DynamoDB: aplica las mismas
// reglas de capacidad y de idempotencia que el adaptador real para probar la capa de aplicación.
type almacenAportaciones struct {
	mu            sync.Mutex
	goals         map[string]appContrib.GoalSnapshot
	contributions map[string]appContrib.Contribution
	idem          map[string]appContrib.IdempotencyRecord
	createErr     error
	reverseErr    error
}

func nuevoAlmacen() *almacenAportaciones {
	return &almacenAportaciones{
		goals:         map[string]appContrib.GoalSnapshot{},
		contributions: map[string]appContrib.Contribution{},
		idem:          map[string]appContrib.IdempotencyRecord{},
	}
}

func clave(userID, id string) string          { return userID + "|" + id }
func claveIdem(userID, op, key string) string { return userID + "|" + op + "|" + key }

func (a *almacenAportaciones) LoadGoal(_ context.Context, userID, goalID string) (*appContrib.GoalSnapshot, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	g, ok := a.goals[clave(userID, goalID)]
	if !ok {
		return nil, nil
	}
	copia := g
	return &copia, nil
}

func (a *almacenAportaciones) LoadIdempotency(_ context.Context, userID, op, key string) (*appContrib.IdempotencyRecord, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	r, ok := a.idem[claveIdem(userID, op, key)]
	if !ok {
		return nil, nil
	}
	copia := r
	return &copia, nil
}

func (a *almacenAportaciones) CreateContribution(_ context.Context, write appContrib.CreateWrite) (*appContrib.StoredResponse, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.createErr != nil {
		return nil, a.createErr
	}
	if existing, ok := a.idem[claveIdem(write.UserID, write.Operation, write.IdempotencyKey)]; ok {
		if existing.PayloadHash != write.PayloadHash {
			return nil, appContrib.ErrConflict
		}
		return &appContrib.StoredResponse{Status: existing.HTTPStatus, Body: existing.ResponseBody, Location: existing.Location, Replayed: true}, nil
	}
	for _, alloc := range write.Contribution.Allocations {
		g, ok := a.goals[clave(write.UserID, alloc.GoalID)]
		if !ok {
			return nil, appContrib.ErrGoalNotFound
		}
		if !g.Active() {
			return nil, appContrib.ErrGoalInactive
		}
		if alloc.AmountCents > g.RemainingCapacityCents() {
			return nil, appContrib.ErrCapacityExceeded
		}
	}
	for _, alloc := range write.Contribution.Allocations {
		g := a.goals[clave(write.UserID, alloc.GoalID)]
		g.SavedCents += alloc.AmountCents
		g.ActiveContributions++
		g.Version++
		a.goals[clave(write.UserID, alloc.GoalID)] = g
	}
	a.contributions[clave(write.UserID, write.Contribution.ID)] = write.Contribution
	a.idem[claveIdem(write.UserID, write.Operation, write.IdempotencyKey)] = appContrib.IdempotencyRecord{
		PayloadHash:  write.PayloadHash,
		HTTPStatus:   201,
		ResponseBody: append([]byte(nil), write.ResponseBody...),
		Location:     write.Location,
		ExpiresAt:    write.ExpiresAt,
	}
	return &appContrib.StoredResponse{Status: 201, Body: write.ResponseBody, Location: write.Location}, nil
}

func (a *almacenAportaciones) ListContributions(_ context.Context, query appContrib.ListQuery, startSK string, limit int) ([]appContrib.Contribution, string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	var items []appContrib.Contribution
	for _, c := range a.contributions {
		if c.UserID != query.UserID {
			continue
		}
		if query.GoalID != "" {
			found := false
			for _, alloc := range c.Allocations {
				if alloc.GoalID == query.GoalID {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if query.Status != "" && c.Status != query.Status {
			continue
		}
		if query.HasFrom && c.OccurredAt.Before(query.From) {
			continue
		}
		if query.HasTo && c.OccurredAt.After(query.To) {
			continue
		}
		items = append(items, c)
	}
	// newest-first
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			if items[j].OccurredAt.After(items[i].OccurredAt) {
				items[i], items[j] = items[j], items[i]
			}
		}
	}
	// El cursor del servicio transporta la SK (aquí el ID) del último elemento devuelto.
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
	if len(items) > limit {
		return items[:limit], items[limit-1].ID, nil
	}
	return items, "", nil
}

func (a *almacenAportaciones) LoadContribution(_ context.Context, userID, id string) (*appContrib.Contribution, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	c, ok := a.contributions[clave(userID, id)]
	if !ok {
		return nil, nil
	}
	copia := c
	return &copia, nil
}

func (a *almacenAportaciones) ReverseContribution(_ context.Context, write appContrib.ReverseWrite) (*appContrib.StoredResponse, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.reverseErr != nil {
		return nil, a.reverseErr
	}
	if existing, ok := a.idem[claveIdem(write.UserID, write.Operation, write.IdempotencyKey)]; ok {
		if existing.PayloadHash != write.PayloadHash {
			return nil, appContrib.ErrConflict
		}
		return &appContrib.StoredResponse{Status: existing.HTTPStatus, Body: existing.ResponseBody, Location: existing.Location, Replayed: true}, nil
	}
	actual, ok := a.contributions[clave(write.UserID, write.Previous.ID)]
	if !ok {
		return nil, appContrib.ErrNotFound
	}
	if actual.Status != appContrib.StatusActive {
		return nil, appContrib.ErrAlreadyReversed
	}
	for _, alloc := range actual.Allocations {
		g, ok := a.goals[clave(write.UserID, alloc.GoalID)]
		if !ok || g.SavedCents < alloc.AmountCents || g.ActiveContributions < 1 {
			return nil, appContrib.ErrConflict
		}
		g.SavedCents -= alloc.AmountCents
		g.ActiveContributions--
		g.Version++
		a.goals[clave(write.UserID, alloc.GoalID)] = g
	}
	actual.Status = write.Next.Status
	actual.Version = write.Next.Version
	actual.ReversedAt = write.Next.ReversedAt
	actual.ReverseReason = write.Next.ReverseReason
	actual.UpdatedAt = write.Next.UpdatedAt
	a.contributions[clave(write.UserID, actual.ID)] = actual
	a.idem[claveIdem(write.UserID, write.Operation, write.IdempotencyKey)] = appContrib.IdempotencyRecord{
		PayloadHash:  write.PayloadHash,
		HTTPStatus:   200,
		ResponseBody: append([]byte(nil), write.ResponseBody...),
		Location:     write.Location,
		ExpiresAt:    write.ExpiresAt,
	}
	return &appContrib.StoredResponse{Status: 200, Body: write.ResponseBody, Location: write.Location}, nil
}

func nuevoServicio(almacen *almacenAportaciones) *appContrib.Service {
	secuencia := 0
	return appContrib.NewService(almacen).
		WithCursorKey([]byte("0123456789abcdef0123456789abcdef")).
		WithClock(func() time.Time { return time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC) }).
		WithIDGenerator(func() (string, error) {
			secuencia++
			return fmt.Sprintf("018f90ef-08e5-7a68-b09f-2b67a47c92%02d", secuencia), nil
		})
}

func metaActiva(userID, id string, target, saved int64) (string, appContrib.GoalSnapshot) {
	return userID + "|" + id, appContrib.GoalSnapshot{ID: id, TargetCents: target, SavedCents: saved, Status: "active", Version: 1}
}

func TestE043PreviewCalculaRemanenteYCanCommit(t *testing.T) {
	almacen := nuevoAlmacen()
	userID := "usuario-a"
	k1, g1 := metaActiva(userID, "018f90ef-08e5-7a68-b09f-2b67a47c92f1", 100000, 0)
	k2, g2 := metaActiva(userID, "018f90ef-08e5-7a68-b09f-2b67a47c92f2", 100000, 0)
	almacen.goals[k1], almacen.goals[k2] = g1, g2
	servicio := nuevoServicio(almacen)

	preview, err := servicio.Preview(context.Background(), appContrib.PreviewCommand{
		UserID:   userID,
		Amount:   "1000.00",
		Currency: "MXN",
		Allocations: []appContrib.WireAllocation{
			{GoalID: g1.ID, Amount: "600.00"},
			{GoalID: g2.ID, Amount: "400.00"},
		},
	})
	if err != nil {
		t.Fatalf("preview valido: %v", err)
	}
	if preview.RemainingCents != 0 || !preview.CanCommit {
		t.Fatalf("esperado remaining=0 canCommit=true, obtenido %d/%v", preview.RemainingCents, preview.CanCommit)
	}
}

func TestE043PreviewCapacidadInsuficiente(t *testing.T) {
	almacen := nuevoAlmacen()
	userID := "usuario-a"
	k1, g1 := metaActiva(userID, "018f90ef-08e5-7a68-b09f-2b67a47c92f1", 50000, 0)
	almacen.goals[k1] = g1
	servicio := nuevoServicio(almacen)

	preview, err := servicio.Preview(context.Background(), appContrib.PreviewCommand{
		UserID:   userID,
		Amount:   "600.00",
		Currency: "MXN",
		Allocations: []appContrib.WireAllocation{
			{GoalID: g1.ID, Amount: "600.00"},
		},
	})
	if err != nil {
		t.Fatalf("preview capacidad: %v", err)
	}
	if preview.CanCommit || preview.RemainingCents != 10000 {
		t.Fatalf("esperado canCommit=false remaining=100.00, obtenido %v/%d", preview.CanCommit, preview.RemainingCents)
	}
}

func TestE043PreviewRechazaMetaRepetidaYExcesoDeMetas(t *testing.T) {
	almacen := nuevoAlmacen()
	servicio := nuevoServicio(almacen)
	base := "018f90ef-08e5-7a68-b09f-2b67a47c92f1"
	_, err := servicio.Preview(context.Background(), appContrib.PreviewCommand{
		UserID: "u", Amount: "10.00", Currency: "MXN",
		Allocations: []appContrib.WireAllocation{{GoalID: base, Amount: "5.00"}, {GoalID: base, Amount: "5.00"}},
	})
	var fv *appContrib.FieldViolation
	if !errors.As(err, &fv) || fv.Reason != "duplicate" {
		t.Fatalf("esperado duplicate, obtenido %v", err)
	}
	allocs := make([]appContrib.WireAllocation, 0, 13)
	for i := 0; i < 13; i++ {
		allocs = append(allocs, appContrib.WireAllocation{GoalID: "018f90ef-08e5-7a68-b09f-2b67a47c92f" + string(rune('0'+i)), Amount: "1.00"})
	}
	_, err = servicio.Preview(context.Background(), appContrib.PreviewCommand{UserID: "u", Amount: "13.00", Currency: "MXN", Allocations: allocs})
	if !errors.As(err, &fv) || fv.Reason != "too_many" {
		t.Fatalf("esperado too_many, obtenido %v", err)
	}
}

func TestE044CreaAportacionYReplay(t *testing.T) {
	almacen := nuevoAlmacen()
	userID := "usuario-a"
	k1, g1 := metaActiva(userID, "018f90ef-08e5-7a68-b09f-2b67a47c92f1", 100000, 0)
	k2, g2 := metaActiva(userID, "018f90ef-08e5-7a68-b09f-2b67a47c92f2", 100000, 0)
	almacen.goals[k1], almacen.goals[k2] = g1, g2
	servicio := nuevoServicio(almacen)
	cmd := appContrib.CreateCommand{
		UserID:         userID,
		IdempotencyKey: "idem-key-0000001",
		Amount:         "1000.00",
		Currency:       "MXN",
		OccurredAt:     "2026-11-19T10:00:00Z",
		Allocations: []appContrib.WireAllocation{
			{GoalID: g1.ID, Amount: "600.00"},
			{GoalID: g2.ID, Amount: "400.00"},
		},
	}
	r1, err := servicio.Create(context.Background(), cmd)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if r1.Status != 201 || r1.Replayed {
		t.Fatalf("esperado 201 nuevo, obtenido %d/%v", r1.Status, r1.Replayed)
	}
	if almacen.goals[k1].SavedCents != 60000 || almacen.goals[k2].SavedCents != 40000 {
		t.Fatalf("metas no actualizadas: %d/%d", almacen.goals[k1].SavedCents, almacen.goals[k2].SavedCents)
	}
	if len(almacen.contributions) != 1 {
		t.Fatalf("esperada 1 aportacion, obtenidas %d", len(almacen.contributions))
	}
	for _, c := range almacen.contributions {
		if c.Status != appContrib.StatusActive || c.AmountCents != 100000 {
			t.Fatalf("aportacion inesperada: %+v", c)
		}
	}
	r2, err := servicio.Create(context.Background(), cmd)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if !r2.Replayed || string(r2.Body) != string(r1.Body) {
		t.Fatalf("replay no identico")
	}
	cmd.Amount = "999.00"
	cmd.Allocations = []appContrib.WireAllocation{{GoalID: g1.ID, Amount: "999.00"}}
	if _, err := servicio.Create(context.Background(), cmd); !errors.Is(err, appContrib.ErrConflict) {
		t.Fatalf("esperado conflicto por payload distinto, obtenido %v", err)
	}
}

func TestE044CapacidadYMetaInactiva(t *testing.T) {
	almacen := nuevoAlmacen()
	userID := "usuario-a"
	k1, g1 := metaActiva(userID, "018f90ef-08e5-7a68-b09f-2b67a47c92f1", 50000, 0)
	almacen.goals[k1] = g1
	servicio := nuevoServicio(almacen)
	_, err := servicio.Create(context.Background(), appContrib.CreateCommand{
		UserID: userID, IdempotencyKey: "idem-key-0000002", Amount: "600.00", Currency: "MXN", OccurredAt: "2026-11-19T10:00:00Z",
		Allocations: []appContrib.WireAllocation{{GoalID: g1.ID, Amount: "600.00"}},
	})
	if !errors.Is(err, appContrib.ErrCapacityExceeded) {
		t.Fatalf("esperado capacidad excedida, obtenido %v", err)
	}
	g1.Status = "archived"
	almacen.goals[k1] = g1
	_, err = servicio.Create(context.Background(), appContrib.CreateCommand{
		UserID: userID, IdempotencyKey: "idem-key-0000003", Amount: "10.00", Currency: "MXN", OccurredAt: "2026-11-19T10:00:00Z",
		Allocations: []appContrib.WireAllocation{{GoalID: g1.ID, Amount: "10.00"}},
	})
	if !errors.Is(err, appContrib.ErrGoalInactive) {
		t.Fatalf("esperado meta inactiva, obtenido %v", err)
	}
}

func TestE045ListadoPaginadoYPropiedad(t *testing.T) {
	almacen := nuevoAlmacen()
	userID := "usuario-a"
	for i, id := range []string{
		"018f90ef-08e5-7a68-b09f-2b67a47c92f1",
		"018f90ef-08e5-7a68-b09f-2b67a47c92f2",
		"018f90ef-08e5-7a68-b09f-2b67a47c92f3",
	} {
		almacen.contributions[clave(userID, id)] = appContrib.Contribution{
			ID: id, UserID: userID, AmountCents: 1000, Currency: "MXN",
			Allocations: []appContrib.Allocation{{GoalID: "018f90ef-08e5-7a68-b09f-2b67a47c92f9", AmountCents: 1000}},
			OccurredAt:  time.Date(2026, 11, 1+i, 10, 0, 0, 0, time.UTC),
			Status:      appContrib.StatusActive, Version: 1,
		}
	}
	servicio := nuevoServicio(almacen)
	p1, err := servicio.List(context.Background(), appContrib.ListQuery{UserID: userID, Limit: 2})
	if err != nil {
		t.Fatalf("list 1: %v", err)
	}
	if len(p1.Items) != 2 || p1.NextCursor == "" {
		t.Fatalf("esperada pagina de 2 con cursor, obtenido %d/%q", len(p1.Items), p1.NextCursor)
	}
	p2, err := servicio.List(context.Background(), appContrib.ListQuery{UserID: userID, Limit: 2, Cursor: p1.NextCursor})
	if err != nil {
		t.Fatalf("list 2: %v", err)
	}
	if len(p2.Items) < 1 {
		t.Fatalf("esperada segunda pagina con elementos")
	}
	if _, err := servicio.List(context.Background(), appContrib.ListQuery{UserID: "otro", Limit: 2, Cursor: p1.NextCursor}); !errors.Is(err, appContrib.ErrInvalidRequest) {
		t.Fatalf("cursor de otro usuario debe fallar, obtenido %v", err)
	}
}

func TestE046DetalleYReversion(t *testing.T) {
	almacen := nuevoAlmacen()
	userID := "usuario-a"
	id := "018f90ef-08e5-7a68-b09f-2b67a47c92f1"
	k1, g1 := metaActiva(userID, "018f90ef-08e5-7a68-b09f-2b67a47c92f2", 100000, 50000)
	g1.ActiveContributions = 1
	almacen.goals[k1] = g1
	almacen.contributions[clave(userID, id)] = appContrib.Contribution{
		ID: id, UserID: userID, AmountCents: 50000, Currency: "MXN",
		Allocations: []appContrib.Allocation{{GoalID: g1.ID, AmountCents: 50000}},
		OccurredAt:  time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC),
		Status:      appContrib.StatusActive, Version: 1,
	}
	servicio := nuevoServicio(almacen)
	c, err := servicio.Get(context.Background(), userID, id)
	if err != nil || c == nil {
		t.Fatalf("get: %v", err)
	}
	if _, err := servicio.Get(context.Background(), "otro", id); !errors.Is(err, appContrib.ErrNotFound) {
		t.Fatalf("otro usuario debe 404, obtenido %v", err)
	}
	cmd := appContrib.ReverseCommand{UserID: userID, ContributionID: id, IdempotencyKey: "idem-rev-0000001", Reason: "Correccion autorizada"}
	r1, err := servicio.Reverse(context.Background(), cmd)
	if err != nil {
		t.Fatalf("reverse: %v", err)
	}
	if r1.Status != 200 || r1.Replayed {
		t.Fatalf("esperado 200 nuevo, obtenido %d/%v", r1.Status, r1.Replayed)
	}
	if almacen.goals[k1].SavedCents != 0 || almacen.goals[k1].ActiveContributions != 0 {
		t.Fatalf("meta no revertida: %+v", almacen.goals[k1])
	}
	r2, err := servicio.Reverse(context.Background(), cmd)
	if err != nil || !r2.Replayed {
		t.Fatalf("replay de reversion: %v/%v", err, r2 != nil && r2.Replayed)
	}
	cmd.IdempotencyKey = "idem-rev-0000002"
	if _, err := servicio.Reverse(context.Background(), cmd); !errors.Is(err, appContrib.ErrAlreadyReversed) {
		t.Fatalf("doble reversion debe 409, obtenido %v", err)
	}
}

func TestE047ConcurrenciaUnaSolaReversion(t *testing.T) {
	almacen := nuevoAlmacen()
	userID := "usuario-a"
	id := "018f90ef-08e5-7a68-b09f-2b67a47c92f1"
	k1, g1 := metaActiva(userID, "018f90ef-08e5-7a68-b09f-2b67a47c92f2", 100000, 50000)
	g1.ActiveContributions = 1
	almacen.goals[k1] = g1
	almacen.contributions[clave(userID, id)] = appContrib.Contribution{
		ID: id, UserID: userID, AmountCents: 50000, Currency: "MXN",
		Allocations: []appContrib.Allocation{{GoalID: g1.ID, AmountCents: 50000}},
		OccurredAt:  time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC),
		Status:      appContrib.StatusActive, Version: 1,
	}
	servicio := nuevoServicio(almacen)
	var wg sync.WaitGroup
	var ok, conflict int
	var mu sync.Mutex
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, err := servicio.Reverse(context.Background(), appContrib.ReverseCommand{
				UserID: userID, ContributionID: id,
				IdempotencyKey: "idem-concurrente-000" + string(rune('1'+n)),
				Reason:         "Correccion concurrente",
			})
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				ok++
			} else if errors.Is(err, appContrib.ErrAlreadyReversed) || errors.Is(err, appContrib.ErrConflict) {
				conflict++
			} else {
				t.Errorf("error inesperado: %v", err)
			}
		}(i)
	}
	wg.Wait()
	if ok != 1 || conflict != 1 {
		t.Fatalf("esperado 1 ok y 1 conflicto, obtenido %d/%d", ok, conflict)
	}
	if almacen.goals[k1].SavedCents != 0 {
		t.Fatalf("doble descuento: %d", almacen.goals[k1].SavedCents)
	}
}

func TestE044MovimientoDeAhorroVinculado(t *testing.T) {
	// El movimiento de ahorro se construye en el servicio; el almacén de este paquete no lo
	// persiste, pero se comprueba la proyección contractual del movimiento.
	m := appMovements.Movement{
		ID: "018f90ef-08e5-7a68-b09f-2b67a47c92f9", Type: appMovements.TypeSaving,
		AmountCents: 1000, Currency: "MXN", CategoryID: "goal", Description: "Aportacion a metas",
		OccurredAt: time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC), Version: 1,
		ContributionID: "018f90ef-08e5-7a68-b09f-2b67a47c92f1",
	}
	body, err := appMovements.MarshalMovement(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(body), `"contributionId":"018f90ef-08e5-7a68-b09f-2b67a47c92f1"`) {
		t.Fatalf("movimiento sin vinculo: %s", body)
	}
	if strings.Contains(string(body), `"recurringPaymentId":"`) {
		t.Fatalf("recurringPaymentId debe ser null")
	}
}

// TestE043E044ParidadDeSumaExacta comprueba que el preview y el alta aplican la MISMA regla
// estructural: suma exacta, sin metas duplicadas y con importes positivos. El preview no puede
// ser mas permisivo que E044.
func TestE043E044ParidadDeSumaExacta(t *testing.T) {
	almacen := nuevoAlmacen()
	userID := "usuario-a"
	k1, g1 := metaActiva(userID, "018f90ef-08e5-7a68-b09f-2b67a47c92f1", 100000, 0)
	almacen.goals[k1] = g1
	servicio := nuevoServicio(almacen)

	casos := []struct {
		nombre     string
		amount     string
		allocation string
		reason     string
	}{
		{"suma incompleta", "1000.00", "500.00", "sum_mismatch"},
		{"suma excedida", "500.00", "600.00", "sum_exceeds_amount"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre+"/preview", func(t *testing.T) {
			_, err := servicio.Preview(context.Background(), appContrib.PreviewCommand{
				UserID: userID, Amount: caso.amount, Currency: "MXN",
				Allocations: []appContrib.WireAllocation{{GoalID: g1.ID, Amount: caso.allocation}},
			})
			var fv *appContrib.FieldViolation
			if !errors.As(err, &fv) || fv.Reason != caso.reason {
				t.Fatalf("preview: esperado %s, obtenido %v", caso.reason, err)
			}
		})
		t.Run(caso.nombre+"/create", func(t *testing.T) {
			_, err := servicio.Create(context.Background(), appContrib.CreateCommand{
				UserID: userID, IdempotencyKey: "idem-paridad-0001", Amount: caso.amount,
				Currency: "MXN", OccurredAt: "2026-11-19T10:00:00Z",
				Allocations: []appContrib.WireAllocation{{GoalID: g1.ID, Amount: caso.allocation}},
			})
			var fv *appContrib.FieldViolation
			if !errors.As(err, &fv) || fv.Reason != caso.reason {
				t.Fatalf("create: esperado %s, obtenido %v", caso.reason, err)
			}
		})
	}

	// Los mismos datos validos deben ser aceptados por ambos.
	validos := []appContrib.WireAllocation{{GoalID: g1.ID, Amount: "600.00"}}
	if _, err := servicio.Preview(context.Background(), appContrib.PreviewCommand{UserID: userID, Amount: "600.00", Currency: "MXN", Allocations: validos}); err != nil {
		t.Fatalf("preview valido: %v", err)
	}
	if _, err := servicio.Create(context.Background(), appContrib.CreateCommand{
		UserID: userID, IdempotencyKey: "idem-paridad-0002", Amount: "600.00", Currency: "MXN",
		OccurredAt: "2026-11-19T10:00:00Z", Allocations: validos,
	}); err != nil {
		t.Fatalf("create valido: %v", err)
	}
}
