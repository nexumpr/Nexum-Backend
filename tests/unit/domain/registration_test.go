package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nexumpr/Nexum-Backend/internal/domain/identity"
)

const validHash = "$argon2id$v=19$m=65536,t=3,p=2$c2FsdA$aGFzaA"

func mustIncome(t *testing.T) identity.InitialIncome {
	t.Helper()
	income, err := identity.NewInitialIncome(1200000, "monthly")
	if err != nil {
		t.Fatalf("ingreso valido rechazado: %v", err)
	}
	return income
}

func mustConsent(t *testing.T, at time.Time) identity.Consent {
	t.Helper()
	consent, err := identity.NewConsent("2026-11", at)
	if err != nil {
		t.Fatalf("consentimiento valido rechazado: %v", err)
	}
	return consent
}

func mustExpense(t *testing.T, id string) identity.RecurringExpense {
	t.Helper()
	expense, err := identity.NewRecurringExpense("Internet", 69900, "monthly", identity.CategoryServices, identity.FixedExpenseType)
	if err != nil {
		t.Fatalf("gasto valido rechazado: %v", err)
	}
	withID, err := expense.WithID(id)
	if err != nil {
		t.Fatalf("identificador de gasto rechazado: %v", err)
	}
	return withID
}

func validParams(t *testing.T, acceptedAt time.Time) identity.RegistrationParams {
	t.Helper()
	return identity.RegistrationParams{
		UserID:            "018f90ef-08e5-7a68-b09f-2b67a47c92f0",
		Email:             "  Usuario@Example.INVALID ",
		Name:              "  Usuario Ejemplo  ",
		PasswordHash:      validHash,
		Income:            mustIncome(t),
		RecurringExpenses: []identity.RecurringExpense{mustExpense(t, "rec-1")},
		Consent:           mustConsent(t, acceptedAt),
	}
}

func TestNewRegistrationValid(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	acceptedAt := now.Add(-time.Minute)

	reg, err := identity.NewRegistration(validParams(t, acceptedAt), now)
	if err != nil {
		t.Fatalf("registro valido rechazado: %v", err)
	}

	if reg.Email() != "usuario@example.invalid" {
		t.Fatalf("email no normalizado: %q", reg.Email())
	}
	if reg.Name() != "Usuario Ejemplo" || reg.DisplayName() != "Usuario Ejemplo" {
		t.Fatalf("name/displayName no normalizado: %q / %q", reg.Name(), reg.DisplayName())
	}
	if reg.PrivacyNoticeVersion() != "2026-11" {
		t.Fatalf("version de privacidad no normalizada: %q", reg.PrivacyNoticeVersion())
	}
	if reg.EmailVerified() {
		t.Fatal("emailVerified debe ser false al registrar")
	}
	if reg.Version() != 1 {
		t.Fatalf("version inicial esperada 1, obtenida %d", reg.Version())
	}
	if !reg.CreatedAt().Equal(now) {
		t.Fatalf("createdAt esperado %v, obtenido %v", now, reg.CreatedAt())
	}
	if !reg.PrivacyAcceptedAt().Equal(acceptedAt) {
		t.Fatalf("privacyAcceptedAt esperado %v, obtenido %v", acceptedAt, reg.PrivacyAcceptedAt())
	}
	if reg.Income().AmountCents() != 1200000 || reg.Income().Frequency() != identity.FrequencyMonthly {
		t.Fatalf("ingreso inesperado: %+v", reg.Income())
	}
	if len(reg.RecurringExpenses()) != 1 {
		t.Fatalf("compromisos esperados 1, obtenidos %d", len(reg.RecurringExpenses()))
	}
	if reg.Onboarding().Microphone() != "pending" || reg.Onboarding().Notifications() != "pending" || reg.Onboarding().Configured() {
		t.Fatalf("onboarding inicial inesperado: %+v", reg.Onboarding())
	}
}

func TestNewRegistrationRejectsInvalidValues(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name   string
		mutate func(*identity.RegistrationParams)
		want   error
	}{
		{"id vacio", func(p *identity.RegistrationParams) { p.UserID = " " }, identity.ErrEmptyUserID},
		{"email invalido", func(p *identity.RegistrationParams) { p.Email = "sin-arroba" }, identity.ErrInvalidEmail},
		{"name vacio", func(p *identity.RegistrationParams) { p.Name = "   " }, identity.ErrInvalidDisplayName},
		{"name largo", func(p *identity.RegistrationParams) { p.Name = strings.Repeat("a", 101) }, identity.ErrInvalidDisplayName},
		{"name control", func(p *identity.RegistrationParams) { p.Name = "Nombre\x00Invalido" }, identity.ErrInvalidDisplayName},
		{"hash vacio", func(p *identity.RegistrationParams) { p.PasswordHash = "  " }, identity.ErrInvalidPasswordHash},
		{"ingreso ausente", func(p *identity.RegistrationParams) { p.Income = identity.InitialIncome{} }, identity.ErrInvalidAmount},
		{"consent vacio", func(p *identity.RegistrationParams) { p.Consent = identity.Consent{} }, identity.ErrInvalidPrivacyAcceptance},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			params := validParams(t, now)
			tc.mutate(&params)
			_, err := identity.NewRegistration(params, now)
			if !errors.Is(err, tc.want) {
				t.Fatalf("se esperaba %v, se obtuvo %v", tc.want, err)
			}
		})
	}
}

func TestNewRegistrationUsesCurrentTimeWhenZero(t *testing.T) {
	before := time.Now().UTC()
	params := validParams(t, time.Now().UTC())
	reg, err := identity.NewRegistration(params, time.Time{})
	if err != nil {
		t.Fatalf("registro valido rechazado: %v", err)
	}
	if reg.CreatedAt().Before(before.Add(-time.Second)) || reg.CreatedAt().After(time.Now().UTC().Add(time.Second)) {
		t.Fatalf("createdAt no usa la hora actual: %v", reg.CreatedAt())
	}
}

func TestNewRegistrationCopiesExpenses(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	params := validParams(t, now)
	reg, err := identity.NewRegistration(params, now)
	if err != nil {
		t.Fatalf("registro valido rechazado: %v", err)
	}
	params.RecurringExpenses[0] = identity.RecurringExpense{}
	if len(reg.RecurringExpenses()) != 1 || reg.RecurringExpenses()[0].ID() != "rec-1" {
		t.Fatal("el agregado debe copiar la lista de compromisos")
	}
}

func TestNewRegistrationRejectsExpenseWithoutID(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	params := validParams(t, now)
	params.RecurringExpenses = []identity.RecurringExpense{mustExpense(t, "rec-1")}
	params.RecurringExpenses[0] = identity.RecurringExpense{}
	_, err := identity.NewRegistration(params, now)
	if !errors.Is(err, identity.ErrInvalidExpenseType) {
		t.Fatalf("se esperaba ErrInvalidExpenseType, se obtuvo %v", err)
	}
}

func TestNewRegistrationRejectsTooManyExpenses(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	params := validParams(t, now)
	params.RecurringExpenses = make([]identity.RecurringExpense, identity.MaxRecurringExpenses+1)
	for i := range params.RecurringExpenses {
		params.RecurringExpenses[i] = mustExpense(t, "rec-"+strings.Repeat("x", 1)+string(rune('a'+i%26))+"-"+string(rune('a'+i/26)))
	}
	_, err := identity.NewRegistration(params, now)
	if !errors.Is(err, identity.ErrTooManyRecurringExpenses) {
		t.Fatalf("se esperaba ErrTooManyRecurringExpenses, se obtuvo %v", err)
	}
}
