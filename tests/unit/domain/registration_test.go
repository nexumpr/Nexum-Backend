package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nexumpr/Nexum-Backend/internal/domain/identity"
)

func TestNewRegistrationValid(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	acceptedAt := now.Add(-time.Minute)

	reg, err := identity.NewRegistration(
		"018f90ef-08e5-7a68-b09f-2b67a47c92f0",
		"  Usuario@Example.INVALID ",
		"  Usuario Ejemplo  ",
		"$argon2id$v=19$m=65536,t=3,p=2$c2FsdA$aGFzaA",
		" 2026-11 ",
		acceptedAt,
		now,
	)
	if err != nil {
		t.Fatalf("registro valido rechazado: %v", err)
	}

	if reg.Email() != "usuario@example.invalid" {
		t.Fatalf("email no normalizado: %q", reg.Email())
	}
	if reg.DisplayName() != "Usuario Ejemplo" {
		t.Fatalf("displayName no normalizado: %q", reg.DisplayName())
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
}

func TestNewRegistrationRejectsInvalidValues(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	validHash := "$argon2id$v=19$m=65536,t=3,p=2$c2FsdA$aGFzaA"

	cases := []struct {
		name        string
		userID      string
		email       string
		displayName string
		hash        string
		privacy     string
		acceptedAt  time.Time
		want        error
	}{
		{"id vacio", " ", "a@b.invalid", "Nombre", validHash, "2026-11", now, identity.ErrEmptyUserID},
		{"email invalido", "id-1", "sin-arroba", "Nombre", validHash, "2026-11", now, identity.ErrInvalidEmail},
		{"display vacio", "id-1", "a@b.invalid", "   ", validHash, "2026-11", now, identity.ErrInvalidDisplayName},
		{"display largo", "id-1", "a@b.invalid", strings.Repeat("a", 101), validHash, "2026-11", now, identity.ErrInvalidDisplayName},
		{"display control", "id-1", "a@b.invalid", "Nombre\x00Invalido", validHash, "2026-11", now, identity.ErrInvalidDisplayName},
		{"hash vacio", "id-1", "a@b.invalid", "Nombre", "  ", "2026-11", now, identity.ErrInvalidPasswordHash},
		{"privacy vacia", "id-1", "a@b.invalid", "Nombre", validHash, "  ", now, identity.ErrInvalidPrivacyNotice},
		{"privacy larga", "id-1", "a@b.invalid", "Nombre", validHash, strings.Repeat("v", 51), now, identity.ErrInvalidPrivacyNotice},
		{"privacy control", "id-1", "a@b.invalid", "Nombre", validHash, "2026\x00-11", now, identity.ErrInvalidPrivacyNotice},
		{"aceptacion cero", "id-1", "a@b.invalid", "Nombre", validHash, "2026-11", time.Time{}, identity.ErrInvalidPrivacyAcceptance},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := identity.NewRegistration(tc.userID, tc.email, tc.displayName, tc.hash, tc.privacy, tc.acceptedAt, now)
			if !errors.Is(err, tc.want) {
				t.Fatalf("se esperaba %v, se obtuvo %v", tc.want, err)
			}
		})
	}
}

func TestNewRegistrationUsesCurrentTimeWhenZero(t *testing.T) {
	before := time.Now().UTC()
	reg, err := identity.NewRegistration(
		"id-1",
		"a@b.invalid",
		"Nombre",
		"$argon2id$v=19$m=65536,t=3,p=2$c2FsdA$aGFzaA",
		"2026-11",
		time.Now().UTC(),
		time.Time{},
	)
	if err != nil {
		t.Fatalf("registro valido rechazado: %v", err)
	}
	if reg.CreatedAt().Before(before.Add(-time.Second)) || reg.CreatedAt().After(time.Now().UTC().Add(time.Second)) {
		t.Fatalf("createdAt no usa la hora actual: %v", reg.CreatedAt())
	}
}
