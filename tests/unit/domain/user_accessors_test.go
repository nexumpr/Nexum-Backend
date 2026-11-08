package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/nexumpr/Nexum-Backend/internal/domain/identity"
)

func TestUserAccessorsYMetadatos(t *testing.T) {
	creado := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	actualizado := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	u, err := identity.ReconstituteUser("usr-1", "Persona@Nexum.com", "argon2id$hash", identity.StatusPending, creado, actualizado)
	if err != nil {
		t.Fatalf("ReconstituteUser no debe fallar: %v", err)
	}
	if u.PasswordHash() != "argon2id$hash" {
		t.Errorf("PasswordHash inesperado: %q", u.PasswordHash())
	}
	if !u.CreatedAt().Equal(creado) || !u.UpdatedAt().Equal(actualizado) {
		t.Errorf("metadatos temporales alterados: %v %v", u.CreatedAt(), u.UpdatedAt())
	}
	if u.Email() != "persona@nexum.com" {
		t.Errorf("el email debe normalizarse a minúsculas: %q", u.Email())
	}
	if !errors.Is(u.CanAuthenticate(), identity.ErrUserInactive) {
		t.Errorf("un usuario PENDING no debe autenticarse")
	}
}

func TestUserBajaActualizaFechaYBloquea(t *testing.T) {
	u, err := identity.NewUser("usr-2", "baja@nexum.com", "hash")
	if err != nil {
		t.Fatalf("NewUser falló: %v", err)
	}
	antes := u.UpdatedAt()
	time.Sleep(2 * time.Millisecond)
	u.MarkDeleted()
	if u.Status() != identity.StatusDeleted {
		t.Errorf("estado esperado DELETED, obtenido %q", u.Status())
	}
	if !u.UpdatedAt().After(antes) {
		t.Errorf("MarkDeleted debe avanzar updatedAt")
	}
	if !errors.Is(u.CanAuthenticate(), identity.ErrUserDeleted) {
		t.Errorf("un usuario DELETED debe rechazar autenticación con ErrUserDeleted")
	}
}

func TestValidacionesDeIdentidad(t *testing.T) {
	if _, err := identity.NewUser("   ", "valido@nexum.com", "hash"); !errors.Is(err, identity.ErrEmptyUserID) {
		t.Errorf("id vacío debe devolver ErrEmptyUserID, obtenido %v", err)
	}
	if _, err := identity.NewUser("usr-3", "no-es-un-email", "hash"); !errors.Is(err, identity.ErrInvalidEmail) {
		t.Errorf("email inválido debe devolver ErrInvalidEmail, obtenido %v", err)
	}
	if _, err := identity.NewUser("usr-3", "Dos Palabras <dos@nexum.com>", "hash"); !errors.Is(err, identity.ErrInvalidEmail) {
		t.Errorf("formato con nombre visible debe rechazarse en el dominio")
	}
	if _, err := identity.ReconstituteUser("   ", "valido@nexum.com", "hash", identity.StatusActive, time.Now(), time.Now()); !errors.Is(err, identity.ErrEmptyUserID) {
		t.Errorf("ReconstituteUser con id vacío debe devolver ErrEmptyUserID")
	}
	if _, err := identity.ReconstituteUser("usr-4", "invalido", "hash", identity.StatusActive, time.Now(), time.Now()); !errors.Is(err, identity.ErrInvalidEmail) {
		t.Errorf("ReconstituteUser con email inválido debe devolver ErrInvalidEmail")
	}
	if email, err := identity.ValidateEmail("  MiXeD@Nexum.COM  "); err != nil || email != "mixed@nexum.com" {
		t.Errorf("ValidateEmail debe normalizar: %q %v", email, err)
	}
}
