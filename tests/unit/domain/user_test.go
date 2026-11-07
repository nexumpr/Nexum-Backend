package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/nexumpr/Nexum-Backend/internal/domain/identity"
)

func TestUserDomainEntity(t *testing.T) {
	t.Run("Creacion valida de usuario", func(t *testing.T) {
		u, err := identity.NewUser("usr-1", "usuario@nexum.com", "hash123")
		if err != nil {
			t.Fatalf("error inesperado al crear usuario: %v", err)
		}
		if u.ID() != "usr-1" {
			t.Errorf("ID esperado 'usr-1', obtenido '%s'", u.ID())
		}
		if u.Email() != "usuario@nexum.com" {
			t.Errorf("email esperado 'usuario@nexum.com', obtenido '%s'", u.Email())
		}
		if u.Status() != identity.StatusActive {
			t.Errorf("estado esperado 'ACTIVE', obtenido '%s'", u.Status())
		}
		if err := u.CanAuthenticate(); err != nil {
			t.Errorf("usuario activo debe poder autenticar: %v", err)
		}
	})

	t.Run("Rechazo de email invalido", func(t *testing.T) {
		invalidEmails := []string{"", "sin-arroba", "dos@@arrobas.com", "espacios @email.com"}
		for _, email := range invalidEmails {
			_, err := identity.NewUser("usr-2", email, "hash123")
			if err == nil {
				t.Errorf("se esperaba rechazo para email invalido: '%s'", email)
			}
			if !errors.Is(err, identity.ErrInvalidEmail) {
				t.Errorf("se esperaba ErrInvalidEmail para '%s', se obtuvo: %v", email, err)
			}
		}
	})

	t.Run("Rechazo de ID vacio", func(t *testing.T) {
		_, err := identity.NewUser("", "test@nexum.com", "hash123")
		if !errors.Is(err, identity.ErrEmptyUserID) {
			t.Errorf("se esperaba ErrEmptyUserID, obtenido: %v", err)
		}
	})

	t.Run("Ciclo de vida y eliminacion de cuenta (GOV-07)", func(t *testing.T) {
		u, err := identity.NewUser("usr-3", "borrado@nexum.com", "hash123")
		if err != nil {
			t.Fatalf("error al crear usuario: %v", err)
		}

		u.MarkDeleted()
		if u.Status() != identity.StatusDeleted {
			t.Errorf("estado esperado 'DELETED', obtenido '%s'", u.Status())
		}

		err = u.CanAuthenticate()
		if !errors.Is(err, identity.ErrUserDeleted) {
			t.Errorf("usuario eliminado debe rechazar autenticacion con ErrUserDeleted, obtenido: %v", err)
		}
	})

	t.Run("Reconstitucion fiel desde persistencia", func(t *testing.T) {
		now := time.Now().UTC()
		u, err := identity.ReconstituteUser("usr-4", "existente@nexum.com", "hashOld", identity.StatusActive, now, now)
		if err != nil {
			t.Fatalf("error al reconstituir usuario: %v", err)
		}
		if u.ID() != "usr-4" || u.Email() != "existente@nexum.com" {
			t.Errorf("usuario reconstituido no coincide: %+v", u)
		}
	})
}
