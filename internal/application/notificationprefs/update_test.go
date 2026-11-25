package notificationprefs

import (
	"context"
	"errors"
	"testing"
	"time"
)

type e058Store struct {
	items        map[string]*Preferences
	err          error
	updateErr    error
	updateCalls  int
	lastExpected int64
}

func (f *e058Store) Load(_ context.Context, userID string) (*Preferences, error) {
	if f.err != nil {
		return nil, f.err
	}
	if p := f.items[userID]; p != nil {
		copia := *p
		return &copia, nil
	}
	return nil, nil
}

func (f *e058Store) Create(_ context.Context, userID string, defaults Preferences, _ time.Time) (*Preferences, error) {
	if f.err != nil {
		return nil, f.err
	}
	if p := f.items[userID]; p != nil {
		copia := *p
		return &copia, nil
	}
	copia := defaults
	f.items[userID] = &copia
	return &copia, nil
}

func (f *e058Store) Update(_ context.Context, userID string, update Update) (*Preferences, error) {
	f.updateCalls++
	f.lastExpected = update.ExpectedVersion
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	resultado := update.Value
	resultado.Version = update.ExpectedVersion + 1
	f.items[userID] = &resultado
	return &resultado, nil
}

func e058NuevoValor() Preferences {
	v := Defaults()
	v.PushEnabled = true
	v.EmailEnabled = true
	v.ReminderEnabled = false
	v.StreakReminders = true
	v.QuietHours = &QuietHours{Enabled: true, Start: "22:30", End: "06:15"}
	v.TimeZone = "America/New_York"
	return v
}

func TestE058ActualizaConCASyVersionIncrementada(t *testing.T) {
	store := &e058Store{items: map[string]*Preferences{
		"user-a": {PushEnabled: false, EmailEnabled: false, ReminderEnabled: true, Version: 4},
	}}
	svc := NewService(store).WithClock(func() time.Time { return time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC) })
	actual, err := svc.Update(context.Background(), UpdateCommand{UserID: "user-a", Value: e058NuevoValor()})
	if err != nil {
		t.Fatal(err)
	}
	if store.lastExpected != 4 || store.updateCalls != 1 {
		t.Fatalf("CAS no usó la versión observada: esperado 4, obtenido %d (%d llamadas)", store.lastExpected, store.updateCalls)
	}
	if actual.Version != 5 || !actual.PushEnabled || !actual.EmailEnabled || actual.ReminderEnabled ||
		!actual.StreakReminders || actual.QuietHours == nil || actual.QuietHours.Start != "22:30" ||
		actual.TimeZone != "America/New_York" {
		t.Fatalf("estado resultante incorrecto: %+v", actual)
	}
}

func TestE058MaterializaAntesDeActualizar(t *testing.T) {
	store := &e058Store{items: map[string]*Preferences{}}
	actual, err := NewService(store).Update(context.Background(), UpdateCommand{UserID: "user-b", Value: e058NuevoValor()})
	if err != nil {
		t.Fatal(err)
	}
	if actual.Version != 2 || store.lastExpected != 1 {
		t.Fatalf("versión incorrecta tras materializar: %+v (esperado CAS 1)", actual)
	}
}

func TestE058Negativas(t *testing.T) {
	ctx := context.Background()
	store := &e058Store{items: map[string]*Preferences{}}
	svc := NewService(store)
	if _, err := svc.Update(ctx, UpdateCommand{UserID: "  ", Value: e058NuevoValor()}); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("sin titular: %v", err)
	}
	if _, err := NewService(nil).Update(ctx, UpdateCommand{UserID: "user-a", Value: e058NuevoValor()}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("sin store: %v", err)
	}
	sinUpdate := &soloStoreE058{items: map[string]*Preferences{}}
	if _, err := NewService(sinUpdate).Update(ctx, UpdateCommand{UserID: "user-a", Value: e058NuevoValor()}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("store sin CAS: %v", err)
	}
	conflicto := &e058Store{items: map[string]*Preferences{"user-a": {Version: 2}}, updateErr: ErrVersionConflict}
	if _, err := NewService(conflicto).Update(ctx, UpdateCommand{UserID: "user-a", Value: e058NuevoValor()}); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("conflicto de versión: %v", err)
	}
}

type soloStoreE058 struct {
	items map[string]*Preferences
}

func (f *soloStoreE058) Load(_ context.Context, userID string) (*Preferences, error) {
	if p := f.items[userID]; p != nil {
		copia := *p
		return &copia, nil
	}
	return nil, nil
}

func (f *soloStoreE058) Create(_ context.Context, userID string, defaults Preferences, _ time.Time) (*Preferences, error) {
	copia := defaults
	f.items[userID] = &copia
	return &copia, nil
}
