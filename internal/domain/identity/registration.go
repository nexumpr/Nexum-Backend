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
	// ErrInvalidDisplayName indica que el nombre no cumple las reglas del contrato.
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

	// StatusOnboardingPending es el valor inicial de cada permiso del onboarding.
	StatusOnboardingPending = "pending"
)

// Consent es el consentimiento de privacidad sellado por el servidor: la version la
// declara el cliente y la fecha la fija siempre el servidor (nunca el cliente).
type Consent struct {
	noticeVersion string
	acceptedAt    time.Time
}

// NewConsent valida la version declarada y exige la marca del servidor.
func NewConsent(noticeVersion string, acceptedAt time.Time) (Consent, error) {
	clean, err := validatePrivacyNoticeVersion(noticeVersion)
	if err != nil {
		return Consent{}, err
	}
	if acceptedAt.IsZero() {
		return Consent{}, ErrInvalidPrivacyAcceptance
	}
	return Consent{noticeVersion: clean, acceptedAt: acceptedAt.UTC()}, nil
}

// NoticeVersion retorna la version aceptada del aviso.
func (c Consent) NoticeVersion() string { return c.noticeVersion }

// AcceptedAt retorna la marca UTC fijada por el servidor.
func (c Consent) AcceptedAt() time.Time { return c.acceptedAt }

// Onboarding es el estado inicial de permisos del dispositivo. Al registrarse no se
// ha concedido ningun permiso: se persiste como pendiente y no configurado.
type Onboarding struct {
	microphone    string
	notifications string
	configured    bool
}

// InitialOnboarding devuelve el estado normativo de apertura de cuenta.
func InitialOnboarding() Onboarding {
	return Onboarding{
		microphone:    StatusOnboardingPending,
		notifications: StatusOnboardingPending,
		configured:    false,
	}
}

// Microphone retorna el estado del permiso de microfono.
func (o Onboarding) Microphone() string { return o.microphone }

// Notifications retorna el estado del permiso de notificaciones.
func (o Onboarding) Notifications() string { return o.notifications }

// Configured indica si el onboarding se completo (siempre false al registrar).
func (o Onboarding) Configured() bool { return o.configured }

// RegistrationParams agrupa las entradas ya validadas del agregado de registro.
type RegistrationParams struct {
	UserID            string
	Email             string
	Name              string
	PasswordHash      string
	Income            InitialIncome
	RecurringExpenses []RecurringExpense
	Consent           Consent
	Client            ClientContext
}

// Registration representa el agregado de registro inicial de E001. Es puro de dominio:
// no conoce HTTP, DynamoDB ni el SDK de AWS.
type Registration struct {
	userID            string
	email             string
	displayName       string
	passwordHash      string
	income            InitialIncome
	recurringExpenses []RecurringExpense
	consent           Consent
	onboarding        Onboarding
	client            ClientContext
	emailVerified     bool
	version           int64
	createdAt         time.Time
	updatedAt         time.Time
}

// NewRegistration valida y construye el registro inicial completo: identidad,
// ingreso habitual, compromisos fijos, consentimiento, onboarding y contexto cliente.
// El email se normaliza con ValidateEmail y la contrasena ya debe llegar como hash.
func NewRegistration(p RegistrationParams, now time.Time) (*Registration, error) {
	if strings.TrimSpace(p.UserID) == "" {
		return nil, ErrEmptyUserID
	}

	cleanEmail, err := ValidateEmail(p.Email)
	if err != nil {
		return nil, err
	}

	cleanName, err := validateDisplayName(p.Name)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(p.PasswordHash) == "" {
		return nil, ErrInvalidPasswordHash
	}

	if p.Income.IsZero() {
		return nil, fmt.Errorf("%w: ingreso habitual requerido", ErrInvalidAmount)
	}

	if len(p.RecurringExpenses) > MaxRecurringExpenses {
		return nil, fmt.Errorf("%w: maximo %d", ErrTooManyRecurringExpenses, MaxRecurringExpenses)
	}
	for _, expense := range p.RecurringExpenses {
		if expense.Type() != FixedExpenseType || expense.ID() == "" {
			return nil, ErrInvalidExpenseType
		}
	}

	if p.Consent.NoticeVersion() == "" || p.Consent.AcceptedAt().IsZero() {
		return nil, ErrInvalidPrivacyAcceptance
	}

	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()

	expenses := make([]RecurringExpense, len(p.RecurringExpenses))
	copy(expenses, p.RecurringExpenses)

	return &Registration{
		userID:            strings.TrimSpace(p.UserID),
		email:             cleanEmail,
		displayName:       cleanName,
		passwordHash:      p.PasswordHash,
		income:            p.Income,
		recurringExpenses: expenses,
		consent:           p.Consent,
		onboarding:        InitialOnboarding(),
		client:            p.Client,
		emailVerified:     initialEmailVerified,
		version:           initialRegistrationVer,
		createdAt:         now,
		updatedAt:         now,
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

// Name retorna el nombre normalizado del contrato V6.0.2.
func (r *Registration) Name() string { return r.displayName }

// DisplayName retorna el nombre normalizado con el nombre heredado del atributo de
// almacenamiento (`displayName`). Es el mismo valor que Name.
func (r *Registration) DisplayName() string { return r.displayName }

// PasswordHash retorna el hash Argon2id.
func (r *Registration) PasswordHash() string { return r.passwordHash }

// Income retorna el ingreso habitual validado.
func (r *Registration) Income() InitialIncome { return r.income }

// RecurringExpenses retorna una copia de los compromisos fijos persistibles.
func (r *Registration) RecurringExpenses() []RecurringExpense {
	out := make([]RecurringExpense, len(r.recurringExpenses))
	copy(out, r.recurringExpenses)
	return out
}

// Consent retorna el consentimiento sellado por el servidor.
func (r *Registration) Consent() Consent { return r.consent }

// Onboarding retorna el estado inicial de permisos.
func (r *Registration) Onboarding() Onboarding { return r.onboarding }

// Client retorna el contexto declarado por el cliente.
func (r *Registration) Client() ClientContext { return r.client }

// PrivacyNoticeVersion retorna la version aceptada del aviso de privacidad.
func (r *Registration) PrivacyNoticeVersion() string { return r.consent.NoticeVersion() }

// PrivacyAcceptedAt retorna la marca UTC de aceptacion fijada por el servidor.
func (r *Registration) PrivacyAcceptedAt() time.Time { return r.consent.AcceptedAt() }

// EmailVerified indica si el correo fue verificado (siempre false al registrar).
func (r *Registration) EmailVerified() bool { return r.emailVerified }

// Version retorna la version optimista inicial.
func (r *Registration) Version() int64 { return r.version }

// CreatedAt retorna la fecha UTC de creacion.
func (r *Registration) CreatedAt() time.Time { return r.createdAt }

// UpdatedAt retorna la fecha UTC de ultima actualizacion.
func (r *Registration) UpdatedAt() time.Time { return r.updatedAt }
