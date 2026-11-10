package identity

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	// ErrInvalidDisplayName indica que el nombre visible no cumple las reglas del contrato.
	ErrInvalidDisplayName = errors.New("nombre visible invalido")

	// ErrInvalidPasswordHash indica que falta el hash Argon2id obligatorio.
	ErrInvalidPasswordHash = errors.New("hash de contrasena obligatorio")

	// ErrInvalidPrivacyNotice indica que la version del aviso de privacidad no es valida.
	ErrInvalidPrivacyNotice = errors.New("version del aviso de privacidad invalida")

	// ErrInvalidPrivacyAcceptance indica que falta la marca temporal de aceptacion.
	ErrInvalidPrivacyAcceptance = errors.New("aceptacion del aviso de privacidad obligatoria")
)

const (
	minDisplayNameRunes    = 1
	maxDisplayNameRunes    = 100
	minPrivacyVersionRunes = 1
	maxPrivacyVersionRunes = 50
	initialRegistrationVer = int64(1)
	initialEmailVerified   = false
)

// Registration representa el agregado de registro inicial de E001. Es puro de dominio:
// no conoce HTTP, DynamoDB ni el SDK de AWS.
type Registration struct {
	userID               string
	email                string
	displayName          string
	passwordHash         string
	privacyNoticeVersion string
	privacyAcceptedAt    time.Time
	emailVerified        bool
	version              int64
	createdAt            time.Time
	updatedAt            time.Time
}

// NewRegistration valida y construye el registro inicial. El email se normaliza con
// ValidateEmail y la contraseña ya debe llegar como hash Argon2id (nunca en claro).
func NewRegistration(
	userID, email, displayName, passwordHash, privacyNoticeVersion string,
	privacyAcceptedAt, now time.Time,
) (*Registration, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, ErrEmptyUserID
	}

	cleanEmail, err := ValidateEmail(email)
	if err != nil {
		return nil, err
	}

	cleanDisplayName, err := validateDisplayName(displayName)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(passwordHash) == "" {
		return nil, ErrInvalidPasswordHash
	}

	cleanPrivacyVersion, err := validatePrivacyNoticeVersion(privacyNoticeVersion)
	if err != nil {
		return nil, err
	}

	if privacyAcceptedAt.IsZero() {
		return nil, ErrInvalidPrivacyAcceptance
	}

	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()

	return &Registration{
		userID:               strings.TrimSpace(userID),
		email:                cleanEmail,
		displayName:          cleanDisplayName,
		passwordHash:         passwordHash,
		privacyNoticeVersion: cleanPrivacyVersion,
		privacyAcceptedAt:    privacyAcceptedAt.UTC(),
		emailVerified:        initialEmailVerified,
		version:              initialRegistrationVer,
		createdAt:            now,
		updatedAt:            now,
	}, nil
}

func validateDisplayName(value string) (string, error) {
	clean := strings.TrimSpace(value)
	if clean == "" {
		return "", fmt.Errorf("%w: no puede estar vacio", ErrInvalidDisplayName)
	}
	if !utf8.ValidString(clean) {
		return "", fmt.Errorf("%w: no es UTF-8 valido", ErrInvalidDisplayName)
	}
	runes := utf8.RuneCountInString(clean)
	if runes < minDisplayNameRunes || runes > maxDisplayNameRunes {
		return "", fmt.Errorf("%w: longitud fuera de rango", ErrInvalidDisplayName)
	}
	for _, r := range clean {
		if unicode.IsControl(r) {
			return "", fmt.Errorf("%w: contiene caracteres de control", ErrInvalidDisplayName)
		}
	}
	return clean, nil
}

func validatePrivacyNoticeVersion(value string) (string, error) {
	clean := strings.TrimSpace(value)
	if clean == "" {
		return "", fmt.Errorf("%w: no puede estar vacia", ErrInvalidPrivacyNotice)
	}
	if !utf8.ValidString(clean) {
		return "", fmt.Errorf("%w: no es UTF-8 valido", ErrInvalidPrivacyNotice)
	}
	runes := utf8.RuneCountInString(clean)
	if runes < minPrivacyVersionRunes || runes > maxPrivacyVersionRunes {
		return "", fmt.Errorf("%w: longitud fuera de rango", ErrInvalidPrivacyNotice)
	}
	for _, r := range clean {
		if unicode.IsControl(r) {
			return "", fmt.Errorf("%w: contiene caracteres de control", ErrInvalidPrivacyNotice)
		}
	}
	return clean, nil
}

// UserID retorna el identificador del usuario.
func (r *Registration) UserID() string { return r.userID }

// Email retorna el email normalizado.
func (r *Registration) Email() string { return r.email }

// DisplayName retorna el nombre visible normalizado.
func (r *Registration) DisplayName() string { return r.displayName }

// PasswordHash retorna el hash Argon2id.
func (r *Registration) PasswordHash() string { return r.passwordHash }

// PrivacyNoticeVersion retorna la version aceptada del aviso de privacidad.
func (r *Registration) PrivacyNoticeVersion() string { return r.privacyNoticeVersion }

// PrivacyAcceptedAt retorna la marca UTC de aceptacion.
func (r *Registration) PrivacyAcceptedAt() time.Time { return r.privacyAcceptedAt }

// EmailVerified indica si el correo fue verificado (siempre false al registrar).
func (r *Registration) EmailVerified() bool { return r.emailVerified }

// Version retorna la version optimista inicial.
func (r *Registration) Version() int64 { return r.version }

// CreatedAt retorna la fecha UTC de creacion.
func (r *Registration) CreatedAt() time.Time { return r.createdAt }

// UpdatedAt retorna la fecha UTC de ultima actualizacion.
func (r *Registration) UpdatedAt() time.Time { return r.updatedAt }
