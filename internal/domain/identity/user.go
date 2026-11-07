package identity

import (
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
)

var (
	// ErrInvalidEmail indica que el correo electrónico no cumple con la sintaxis RFC.
	ErrInvalidEmail = errors.New("formato de correo electronico invalido")

	// ErrEmptyUserID indica que el identificador de usuario es requerido.
	ErrEmptyUserID = errors.New("el identificador de usuario no puede estar vacio")

	// ErrUserDeleted indica que la cuenta del usuario ha sido dada de baja (GOV-07).
	ErrUserDeleted = errors.New("la cuenta de usuario se encuentra eliminada")

	// ErrUserInactive indica que el usuario aún no está activado.
	ErrUserInactive = errors.New("la cuenta de usuario no esta activa")
)

// UserStatus define el ciclo de vida de una cuenta en el dominio.
type UserStatus string

const (
	StatusPending UserStatus = "PENDING"
	StatusActive  UserStatus = "ACTIVE"
	StatusDeleted UserStatus = "DELETED"
)

// User representa la entidad raíz de agregación de identidad en el dominio (A-020 §2.3, GOV-06, GOV-07).
// Es puramente de dominio y no tiene dependencias de persistencia, AWS ni librerías de transporte.
type User struct {
	id           string
	email        string
	passwordHash string
	status       UserStatus
	createdAt    time.Time
	updatedAt    time.Time
}

// NewUser crea una nueva entidad de usuario validando invariantes esenciales.
func NewUser(id, email, passwordHash string) (*User, error) {
	if strings.TrimSpace(id) == "" {
		return nil, ErrEmptyUserID
	}

	cleanEmail, err := ValidateEmail(email)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	return &User{
		id:           strings.TrimSpace(id),
		email:        cleanEmail,
		passwordHash: passwordHash,
		status:       StatusActive,
		createdAt:    now,
		updatedAt:    now,
	}, nil
}

// ReconstituteUser reconstruye una entidad de usuario persistida sin disparar eventos de creación.
func ReconstituteUser(id, email, passwordHash string, status UserStatus, createdAt, updatedAt time.Time) (*User, error) {
	if strings.TrimSpace(id) == "" {
		return nil, ErrEmptyUserID
	}
	cleanEmail, err := ValidateEmail(email)
	if err != nil {
		return nil, err
	}
	return &User{
		id:           id,
		email:        cleanEmail,
		passwordHash: passwordHash,
		status:       status,
		createdAt:    createdAt,
		updatedAt:    updatedAt,
	}, nil
}

// ID retorna el identificador único del usuario.
func (u *User) ID() string {
	return u.id
}

// Email retorna el correo normalizado.
func (u *User) Email() string {
	return u.email
}

// PasswordHash retorna el hash seguro (Argon2id) de la contraseña.
func (u *User) PasswordHash() string {
	return u.passwordHash
}

// Status retorna el estado actual de la cuenta.
func (u *User) Status() UserStatus {
	return u.status
}

// CreatedAt retorna la fecha de creación en UTC.
func (u *User) CreatedAt() time.Time {
	return u.createdAt
}

// UpdatedAt retorna la fecha de última actualización en UTC.
func (u *User) UpdatedAt() time.Time {
	return u.updatedAt
}

// CanAuthenticate comprueba si el usuario puede iniciar sesión en el sistema.
func (u *User) CanAuthenticate() error {
	if u.status == StatusDeleted {
		return ErrUserDeleted
	}
	if u.status != StatusActive {
		return ErrUserInactive
	}
	return nil
}

// MarkDeleted aplica el borrado lógico de cuenta según las reglas de privacidad de GOV-07.
func (u *User) MarkDeleted() {
	u.status = StatusDeleted
	u.updatedAt = time.Now().UTC()
}

// ValidateEmail normaliza y valida la sintaxis de un correo electrónico.
func ValidateEmail(email string) (string, error) {
	clean := strings.TrimSpace(strings.ToLower(email))
	addr, err := mail.ParseAddress(clean)
	if err != nil || addr.Address != clean {
		return "", fmt.Errorf("%w: '%s'", ErrInvalidEmail, email)
	}
	return clean, nil
}
