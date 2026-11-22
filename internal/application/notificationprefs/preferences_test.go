package notificationprefs

import (
	"context"
	"errors"
	"testing"
	"time"
)

type e057Store struct {
	items map[string]*Preferences
	err   error
}

func (f *e057Store) Load(_ context.Context, userID string) (*Preferences, error) {
	if f.err != nil {
		return nil, f.err
	}
	if p := f.items[userID]; p != nil {
		copia := *p
		return &copia, nil
	}
	return nil, nil
}

func (f *e057Store) Create(_ context.Context, userID string, defaults Preferences, _ time.Time) (*Preferences, error) {
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

func TestE057DefaultsSinRegistro(t *testing.T) {
	store := &e057Store{items: map[string]*Preferences{}}
	svc := NewService(store).WithClock(func() time.Time { return time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC) })
	p, err := svc.Get(context.Background(), "user-a")
	if err != nil {
		t.Fatal(err)
	}
	if p.Version != 1 || p.PushEnabled || p.EmailEnabled || !p.ReminderEnabled ||
		!p.BudgetAlerts || !p.GoalUpdates || !p.RecurringReminders || p.StreakReminders ||
		p.QuietHours != nil || p.TimeZone != "" {
		t.Fatalf("defaults incorrectos: %+v", p)
	}
	if store.items["user-a"] == nil {
		t.Fatal("los defaults no se materializaron en persistencia")
	}
	// Segunda lectura: misma versión, sin duplicar ni incrementar.
	p2, err := svc.Get(context.Background(), "user-a")
	if err != nil || p2.Version != 1 {
		t.Fatalf("lectura repetida no idempotente: %+v %v", p2, err)
	}
}

func TestE057PreferenciasModificadasGranulares(t *testing.T) {
	modificadas := &Preferences{
		PushEnabled:        true,
		EmailEnabled:       false,
		ReminderEnabled:    false,
		Version:            4,
		BudgetAlerts:       false,
		GoalUpdates:        true,
		RecurringReminders: false,
		StreakReminders:    true,
		QuietHours:         &QuietHours{Enabled: true, Start: "22:00", End: "07:00"},
		TimeZone:           "America/New_York",
	}
	store := &e057Store{items: map[string]*Preferences{"user-a": modificadas}}
	p, err := NewService(store).Get(context.Background(), "user-a")
	if err != nil {
		t.Fatal(err)
	}
	w := p.Wire()
	if !w.PushEnabled || w.ReminderEnabled || w.BudgetAlerts || w.RecurringReminders || !w.StreakReminders {
		t.Fatalf("preferencias granulares incorrectas: %+v", w)
	}
	if w.Version != 4 || w.QuietHours == nil || !w.QuietHours.Enabled ||
		w.QuietHours.Start != "22:00" || w.QuietHours.End != "07:00" || w.TimeZone != "America/New_York" {
		t.Fatalf("quietHours/timeZone incorrectos: %+v", w)
	}
}

func TestE057AislamientoYNegativas(t *testing.T) {
	ctx := context.Background()
	store := &e057Store{items: map[string]*Preferences{
		"user-a": {PushEnabled: true, Version: 7},
	}}
	svc := NewService(store)
	b, err := svc.Get(ctx, "user-b")
	if err != nil {
		t.Fatal(err)
	}
	if b.PushEnabled || b.Version != 1 || store.items["user-b"] == nil {
		t.Fatalf("usuario B recibió estado de A o no se materializó: %+v", b)
	}
	if _, err := svc.Get(ctx, "  "); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("sin titular: esperado ErrUnauthorized, obtenido %v", err)
	}
	if _, err := NewService(nil).Get(ctx, "user-a"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("sin store: esperado ErrUnavailable, obtenido %v", err)
	}
	fallido := NewService(&e057Store{items: map[string]*Preferences{}, err: ErrUnavailable})
	if _, err := fallido.Get(ctx, "user-a"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("store caído: esperado ErrUnavailable, obtenido %v", err)
	}
}
