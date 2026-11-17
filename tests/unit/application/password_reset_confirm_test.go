package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
)

// Pruebas de aplicación de E006: validación del contrato, caducidad lógica y propagación
// de versión/época a la transacción. La transacción real se cubre en tests/unit/dynamo.

type fakeResetConfirmStore struct {
	token      *appIdentity.ResetTokenRecord
	tokenErr   error
	profile    *appIdentity.ResetProfile
	profileErr error
	applyErr   error
	aplicado   *appIdentity.ResetApplication
	llamadas   int
}

func (f *fakeResetConfirmStore) LoadResetToken(context.Context, string, time.Time) (*appIdentity.ResetTokenRecord, error) {
	f.llamadas++
	return f.token, f.tokenErr
}

func (f *fakeResetConfirmStore) LoadResetProfile(context.Context, string) (*appIdentity.ResetProfile, error) {
	return f.profile, f.profileErr
}

func (f *fakeResetConfirmStore) ApplyReset(_ context.Context, apply appIdentity.ResetApplication) error {
	f.aplicado = &apply
	return f.applyErr
}

type fakeResetHasher struct{}

func (fakeResetHasher) HashPassword(password string) (string, error) { return "hash:" + password, nil }
func (fakeResetHasher) VerifyPassword(password, hash string) (bool, error) {
	return hash == "hash:"+password, nil
}

func resetValido(ahora time.Time) *fakeResetConfirmStore {
	return &fakeResetConfirmStore{
		token: &appIdentity.ResetTokenRecord{
			UserID: "usuario-1", ExpiresAt: ahora.Add(10 * time.Minute), HasExpiry: true,
		},
		profile: &appIdentity.ResetProfile{
			UserID: "usuario-1", Status: appIdentity.StatusActive, Version: 4, HasVersion: true, SessionEpoch: 2,
		},
	}
}

func TestE006ValidacionDelContrato(t *testing.T) {
	casos := []struct {
		nombre string
		cmd    appIdentity.ResetConfirmCommand
		campo  string
	}{
		{"token vacio", appIdentity.ResetConfirmCommand{NewPassword: "NewPassphrase#2026"}, "token"},
		{"token con espacio", appIdentity.ResetConfirmCommand{Token: "token con espacio", NewPassword: "NewPassphrase#2026"}, "token"},
		{"password corta", appIdentity.ResetConfirmCommand{Token: "token-valido-0123456789"}, "newPassword"},
		{"password larga", appIdentity.ResetConfirmCommand{Token: "token-valido-0123456789", NewPassword: strings.Repeat("a", 129)}, "newPassword"},
		{"password en blanco", appIdentity.ResetConfirmCommand{Token: "token-valido-0123456789", NewPassword: strings.Repeat(" ", 12)}, "newPassword"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			_, err := appIdentity.ValidateResetConfirmCommand(caso.cmd)
			var violation *appIdentity.FieldViolation
			if !errors.As(err, &violation) || violation.Field != caso.campo {
				t.Fatalf("se esperaba FieldViolation en %q, obtenido %v", caso.campo, err)
			}
		})
	}
}

func TestE006ConfirmAplicaVersionYEpoca(t *testing.T) {
	ahora := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	store := resetValido(ahora)
	servicio := appIdentity.NewPasswordResetConfirmService(store, fakeResetHasher{}).
		WithClock(func() time.Time { return ahora })

	if err := servicio.Confirm(context.Background(), appIdentity.ResetConfirmCommand{
		Token: "token-valido-0123456789", NewPassword: "NewPassphrase#2026",
	}); err != nil {
		t.Fatalf("confirmacion valida fallo: %v", err)
	}
	if store.aplicado == nil {
		t.Fatalf("no se aplico la transaccion")
	}
	if store.aplicado.ExpectedVersion != 4 || !store.aplicado.HasVersion {
		t.Fatalf("version esperada 4, obtenido %d (has=%v)", store.aplicado.ExpectedVersion, store.aplicado.HasVersion)
	}
	if store.aplicado.ExpectedSessionEpoch != 2 || store.aplicado.NextSessionEpoch != 3 {
		t.Fatalf("epoca esperada 2->3, obtenido %d->%d", store.aplicado.ExpectedSessionEpoch, store.aplicado.NextSessionEpoch)
	}
	if store.aplicado.TokenHash != appIdentity.HashToken("token-valido-0123456789") {
		t.Fatalf("el token debe resolverse por su SHA-256")
	}
	if store.aplicado.PasswordHash != "hash:NewPassphrase#2026" {
		t.Fatalf("el hash aplicado no corresponde a la contrasena nueva")
	}
}

func TestE006ConfirmRechazaTokenUsado(t *testing.T) {
	ahora := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	store := resetValido(ahora)
	store.token.Used = true
	servicio := appIdentity.NewPasswordResetConfirmService(store, fakeResetHasher{})

	err := servicio.Confirm(context.Background(), appIdentity.ResetConfirmCommand{
		Token: "token-valido-0123456789", NewPassword: "NewPassphrase#2026",
	})
	if !errors.Is(err, appIdentity.ErrResetTokenUsado) {
		t.Fatalf("se esperaba ErrResetTokenUsado, obtenido %v", err)
	}
	if store.aplicado != nil {
		t.Fatalf("un token usado no puede aplicar la transaccion")
	}
}

func TestE006ConfirmRechazaExpiradoSinAplicar(t *testing.T) {
	ahora := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	store := resetValido(ahora)
	store.token.ExpiresAt = ahora
	servicio := appIdentity.NewPasswordResetConfirmService(store, fakeResetHasher{})

	err := servicio.Confirm(context.Background(), appIdentity.ResetConfirmCommand{
		Token: "token-valido-0123456789", NewPassword: "NewPassphrase#2026",
	})
	if !errors.Is(err, ports.ErrResetTokenInvalido) {
		t.Fatalf("se esperaba token invalido, obtenido %v", err)
	}
	if store.aplicado != nil {
		t.Fatalf("un token expirado no puede aplicar la transaccion")
	}
}

func TestE006ConfirmFallaCerrado(t *testing.T) {
	ahora := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)

	casos := []struct {
		nombre string
		store  *fakeResetConfirmStore
		espera error
	}{
		{"error al leer token", func() *fakeResetConfirmStore {
			s := resetValido(ahora)
			s.tokenErr = errors.New("dynamodb caido")
			return s
		}(), appIdentity.ErrResetConfirmUnavailable},
		{"error al leer perfil", func() *fakeResetConfirmStore {
			s := resetValido(ahora)
			s.profileErr = errors.New("dynamodb caido")
			return s
		}(), appIdentity.ErrResetConfirmUnavailable},
		{"cuenta deshabilitada", func() *fakeResetConfirmStore {
			s := resetValido(ahora)
			s.profile.Status = "DISABLED"
			return s
		}(), ports.ErrResetTokenInvalido},
		{"conflicto de version", func() *fakeResetConfirmStore {
			s := resetValido(ahora)
			s.applyErr = appIdentity.ErrResetEnProceso
			return s
		}(), appIdentity.ErrResetEnProceso},
		{"token consumido en carrera", func() *fakeResetConfirmStore {
			s := resetValido(ahora)
			s.applyErr = appIdentity.ErrResetTokenUsado
			return s
		}(), appIdentity.ErrResetTokenUsado},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			servicio := appIdentity.NewPasswordResetConfirmService(caso.store, fakeResetHasher{}).
				WithClock(func() time.Time { return ahora })
			err := servicio.Confirm(context.Background(), appIdentity.ResetConfirmCommand{
				Token: "token-valido-0123456789", NewPassword: "NewPassphrase#2026",
			})
			if !errors.Is(err, caso.espera) {
				t.Fatalf("se esperaba %v, obtenido %v", caso.espera, err)
			}
		})
	}
}

func TestE006SinDependenciasRespondeNoDisponible(t *testing.T) {
	servicio := appIdentity.NewPasswordResetConfirmService(nil, nil)
	if servicio.Disponible() {
		t.Fatalf("sin store ni hasher el servicio no esta disponible")
	}
	err := servicio.Confirm(context.Background(), appIdentity.ResetConfirmCommand{
		Token: "token-valido-0123456789", NewPassword: "NewPassphrase#2026",
	})
	if !errors.Is(err, appIdentity.ErrResetConfirmUnavailable) {
		t.Fatalf("se esperaba ErrResetConfirmUnavailable, obtenido %v", err)
	}
}
