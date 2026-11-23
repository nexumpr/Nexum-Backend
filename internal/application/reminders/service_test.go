package reminders

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"
)

type fakeStore struct {
	items map[string]*Reminder
	idem  map[string]IdempotencyRecord
}

func newFakeStore() *fakeStore {
	return &fakeStore{items: map[string]*Reminder{}, idem: map[string]IdempotencyRecord{}}
}

func itemKey(userID, id string) string { return userID + "|" + id }

func copia(r *Reminder) *Reminder {
	if r == nil {
		return nil
	}
	c := *r
	return &c
}

func (f *fakeStore) List(_ context.Context, query ListQuery) (*ListResult, error) {
	items := make([]Reminder, 0)
	for _, r := range f.items {
		if r.Deleted || (query.Active != nil && r.Active != *query.Active) {
			continue
		}
		items = append(items, *copia(r))
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].DueAt.Equal(items[j].DueAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].DueAt.Before(items[j].DueAt)
	})
	if query.After != nil {
		indices := -1
		for i := range items {
			if items[i].ID == query.After.SK {
				indices = i
				break
			}
		}
		if indices >= 0 {
			items = items[indices+1:]
		}
	}
	resultado := &ListResult{Items: []Reminder{}}
	for i, r := range items {
		if i >= query.Limit {
			resultado.NextKey = &Key{PK: "USER#" + query.UserID, SK: items[i-1].ID}
			break
		}
		resultado.Items = append(resultado.Items, r)
	}
	return resultado, nil
}

func (f *fakeStore) Get(_ context.Context, userID, id string) (*Reminder, error) {
	return copia(f.items[itemKey(userID, id)]), nil
}

func (f *fakeStore) FindIdempotency(_ context.Context, userID, keyHash string) (*IdempotencyRecord, error) {
	record, ok := f.idem[userID+"|"+keyHash]
	if !ok {
		return nil, nil
	}
	return &record, nil
}

func (f *fakeStore) Create(_ context.Context, input CreateInput) error {
	key := itemKey(input.UserID, input.ID)
	if _, ok := f.items[key]; ok {
		return ErrConflict
	}
	dueAt, err := time.Parse(time.RFC3339, input.DueAt)
	if err != nil {
		return ErrInvalidRequest
	}
	f.items[key] = &Reminder{
		ID: input.ID, Title: input.Title, DueAt: dueAt, Timezone: input.Timezone,
		Active: true, Version: 1, Frequency: input.Frequency,
	}
	f.idem[input.UserID+"|"+input.KeyHash] = IdempotencyRecord{
		ReminderID: input.ID, PayloadHash: input.PayloadHash,
	}
	return nil
}

func (f *fakeStore) Update(_ context.Context, input UpdateInput) (*Reminder, error) {
	key := itemKey(input.UserID, input.ID)
	actual, ok := f.items[key]
	if !ok || actual.Deleted {
		return nil, ErrNotFound
	}
	if actual.Version != input.ExpectedVersion {
		return nil, ErrConflict
	}
	if input.Title != nil {
		actual.Title = *input.Title
	}
	if input.DueAt != nil {
		parsed, err := time.Parse(time.RFC3339, *input.DueAt)
		if err != nil {
			return nil, ErrInvalidRequest
		}
		actual.DueAt = parsed
	}
	if input.Active != nil {
		actual.Active = *input.Active
	}
	if input.Frequency.Set {
		actual.Frequency = input.Frequency.Value
	}
	actual.Version++
	return copia(actual), nil
}

func (f *fakeStore) Delete(_ context.Context, userID, id string, expectedVersion int64) error {
	key := itemKey(userID, id)
	actual, ok := f.items[key]
	if !ok || actual.Deleted {
		return ErrNotFound
	}
	if actual.Version != expectedVersion {
		return ErrConflict
	}
	actual.Deleted = true
	actual.Active = false
	actual.Version++
	return nil
}

func servicioDePrueba(t *testing.T) (*Service, *fakeStore) {
	t.Helper()
	store := newFakeStore()
	svc := NewService(store, []byte("clave-de-prueba-no-productiva-oleada5"))
	svc.WithClock(func() time.Time { return time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC) })
	return svc, store
}

func crearDePrueba(t *testing.T, svc *Service, userID, clave, titulo string) *Reminder {
	t.Helper()
	return crearDePruebaConID(t, svc, userID, clave, titulo, "11111111-2222-4333-8444-555555555555")
}

func crearDePruebaConID(t *testing.T, svc *Service, userID, clave, titulo, id string) *Reminder {
	t.Helper()
	svc.WithIDGenerator(func() (string, error) { return id, nil })
	reminder, err := svc.Create(context.Background(), CreateCommand{
		UserID: userID, IdempotencyKey: clave, Title: titulo,
		DueAt: "2026-11-24T10:00:00Z", Timezone: "America/Mexico_City",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return reminder
}

func TestCreateIdempotenteYConflictoPorCargaDistinta(t *testing.T) {
	svc, _ := servicioDePrueba(t)
	clave := "clave-idempotencia-de-prueba-01"
	primero := crearDePrueba(t, svc, "user-a", clave, "Pagar luz")
	segundo, err := svc.Create(context.Background(), CreateCommand{
		UserID: "user-a", IdempotencyKey: clave, Title: "Pagar luz",
		DueAt: "2026-11-24T10:00:00Z", Timezone: "America/Mexico_City",
	})
	if err != nil {
		t.Fatalf("replay con la misma carga: %v", err)
	}
	if primero.ID != segundo.ID || primero.Version != segundo.Version {
		t.Fatal("la misma clave con la misma carga debe devolver el mismo recurso")
	}
	_, err = svc.Create(context.Background(), CreateCommand{
		UserID: "user-a", IdempotencyKey: clave, Title: "Otro titulo distinto",
		DueAt: "2026-11-24T10:00:00Z", Timezone: "America/Mexico_City",
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("esperado ErrConflict, obtenido %v", err)
	}
}

func TestCreateRechazaZonaInvalidaYClaveCorta(t *testing.T) {
	svc, _ := servicioDePrueba(t)
	_, err := svc.Create(context.Background(), CreateCommand{
		UserID: "user-a", IdempotencyKey: "corta", Title: "Pagar luz",
		DueAt: "2026-11-24T10:00:00Z", Timezone: "America/Mexico_City",
	})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("clave corta: esperado ErrInvalidRequest, obtenido %v", err)
	}
	_, err = svc.Create(context.Background(), CreateCommand{
		UserID: "user-a", IdempotencyKey: "clave-idempotencia-de-prueba-02", Title: "Pagar luz",
		DueAt: "2026-11-24T10:00:00Z", Timezone: "Zona/No_Existe",
	})
	if !errors.Is(err, ErrBusinessRule) {
		t.Fatalf("zona invalida: esperado ErrBusinessRule, obtenido %v", err)
	}
}

func TestCursorQuedaLigadoAIdentidadYFiltros(t *testing.T) {
	svc, _ := servicioDePrueba(t)
	crearDePruebaConID(t, svc, "user-a", "clave-idempotencia-de-prueba-03", "Primero", "11111111-2222-4333-8444-555555555555")
	crearDePruebaConID(t, svc, "user-a", "clave-idempotencia-de-prueba-03b", "Segundo", "22222222-3333-4444-8555-666666666666")
	pagina, err := svc.List(context.Background(), "user-a", 1, "", nil)
	if err != nil || pagina.NextCursor == "" {
		t.Fatalf("List: err=%v cursor=%q", err, pagina.NextCursor)
	}
	if _, err := svc.List(context.Background(), "user-b", 1, pagina.NextCursor, nil); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("cursor de otro usuario debe fallar: %v", err)
	}
	if _, err := svc.List(context.Background(), "user-a", 1, pagina.NextCursor, boolPtr(true)); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("cursor con filtro distinto debe fallar: %v", err)
	}
	if _, err := svc.List(context.Background(), "user-a", 1, pagina.NextCursor+"x", nil); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("cursor alterado debe fallar: %v", err)
	}
}

func TestUpdateCASYFrecuenciaNull(t *testing.T) {
	svc, _ := servicioDePrueba(t)
	frecuencia := FrequencyWeekly
	creado := crearDePrueba(t, svc, "user-a", "clave-idempotencia-de-prueba-04", "Ahorro")
	_ = frecuencia
	_, err := svc.Update(context.Background(), PatchCommand{
		UserID: "user-a", ReminderID: creado.ID, ExpectedVersion: 99, Title: strPtr("Nuevo"),
	})
	if !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("esperado ErrVersionConflict, obtenido %v", err)
	}
	actualizado, err := svc.Update(context.Background(), PatchCommand{
		UserID: "user-a", ReminderID: creado.ID, ExpectedVersion: creado.Version,
		Frequency: FrequencyPatch{Set: true, Value: nil},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if actualizado.Frequency != nil || actualizado.Version != creado.Version+1 {
		t.Fatalf("frequency:null debe cancelar la recurrencia y aumentar la versión: %+v", actualizado)
	}
}

func TestDeleteEsIdempotenteYAisladoPorUsuario(t *testing.T) {
	svc, _ := servicioDePrueba(t)
	creado := crearDePrueba(t, svc, "user-a", "clave-idempotencia-de-prueba-05", "Recordatorio")
	if err := svc.Delete(context.Background(), "user-b", creado.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("otro usuario no debe borrar: %v", err)
	}
	if err := svc.Delete(context.Background(), "user-a", creado.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := svc.Delete(context.Background(), "user-a", creado.ID); err != nil {
		t.Fatalf("Delete repetido debe ser idempotente: %v", err)
	}
	if _, err := svc.Get(context.Background(), "user-a", creado.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("el borrado lógico debe ocultar el recurso: %v", err)
	}
}

func strPtr(v string) *string { return &v }
func boolPtr(v bool) *bool    { return &v }
