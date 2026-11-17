package identity

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nexumpr/Nexum-Backend/internal/ports"
)

// Reglas de E006 (RFC-016 §17.3).
var (
	// ErrResetTokenUsado indica reutilización de un token ya consumido. El contrato
	// responde 409 CONFLICT; el mensaje público es genérico y no revela la cuenta.
	ErrResetTokenUsado = errors.New("token de recuperacion ya utilizado")

	// ErrResetEnProceso indica que la transacción atómica perdió una carrera (otra
	// solicitud consumió el token o cambió la versión del perfil). Responde 409.
	ErrResetEnProceso = errors.New("conflicto al aplicar el cambio de contrasena")

	// ErrResetConfirmUnavailable indica falta de persistencia, del hasher o un fallo de
	// DynamoDB. Responde 503 y nunca confirma un cambio que no se aplicó.
	ErrResetConfirmUnavailable = errors.New("cambio de contrasena no disponible")
)

const (
	// Longitudes del contrato OpenAPI PasswordResetRequest.
	ResetPasswordMinRunes = 12
	ResetPasswordMaxRunes = 128

	// El emisor de E005 produce 43 caracteres base64url; el límite deja margen sin
	// admitir cuerpos abusivos.
	resetTokenMaxRunes = 512
)

// ResetConfirmCommand es la entrada de aplicación de E006. No contiene dependencias HTTP.
type ResetConfirmCommand struct {
	Token       string
	NewPassword string
}

// ResetTokenRecord es la lectura autoritativa de `RESET#<sha256(token)>/RESET`.
type ResetTokenRecord struct {
	UserID    string
	ExpiresAt time.Time
	HasExpiry bool
	Used      bool
}

// ResetProfile es el perfil mínimo necesario para aplicar el cambio.
type ResetProfile struct {
	UserID       string
	Status       string
	Version      int64
	HasVersion   bool
	SessionEpoch int64
}

// ResetApplication describe la transacción atómica de E006: dos acciones sobre dos ítems.
type ResetApplication struct {
	UserID       string
	TokenHash    string
	PasswordHash string
	// ExpectedVersion es la versión leída del perfil. Si HasVersion es false (perfil
	// legado sin atributo) la condición usa attribute_not_exists(version).
	ExpectedVersion int64
	HasVersion      bool
	// ExpectedSessionEpoch es la época leída del perfil; la transacción exige que siga
	// siendo la vigente (o ausente, para perfiles anteriores a A6) y escribe NextSessionEpoch.
	ExpectedSessionEpoch int64
	NextSessionEpoch     int64
	Now                  time.Time
}

// ResetConfirmStore es el puerto de persistencia de E006. Se implementa sobre la tabla
// única existente; nunca usa Scan ni memoria como autoridad.
type ResetConfirmStore interface {
	// LoadResetToken lee el token con consistencia fuerte y aplica la caducidad lógica
	// (no depende de cuándo DynamoDB lo elimine físicamente por TTL).
	LoadResetToken(ctx context.Context, tokenHash string, now time.Time) (*ResetTokenRecord, error)
	// LoadResetProfile lee `USER#<id>/PROFILE` con consistencia fuerte.
	LoadResetProfile(ctx context.Context, userID string) (*ResetProfile, error)
	// ApplyReset ejecuta UNA TransactWriteItems con dos acciones sobre dos ítems:
	// actualización condicional del perfil y consumo condicional del token.
	ApplyReset(ctx context.Context, apply ResetApplication) error
}

// PasswordResetConfirmService orquesta la confirmación de E006 (A6 incluido).
type PasswordResetConfirmService struct {
	store  ResetConfirmStore
	hasher ports.PasswordHasher
	now    func() time.Time
}

// NewPasswordResetConfirmService construye el servicio. Si falta una dependencia crítica,
// Confirm falla cerrado con ErrResetConfirmUnavailable (nunca simula el cambio).
func NewPasswordResetConfirmService(store ResetConfirmStore, hasher ports.PasswordHasher) *PasswordResetConfirmService {
	return &PasswordResetConfirmService{
		store:  store,
		hasher: hasher,
		now:    func() time.Time { return time.Now().UTC() },
	}
}

// WithClock permite inyectar un reloj determinista en pruebas.
func (s *PasswordResetConfirmService) WithClock(now func() time.Time) *PasswordResetConfirmService {
	if now != nil {
		s.now = now
	}
	return s
}

// Disponible indica si el endpoint puede operar de verdad.
func (s *PasswordResetConfirmService) Disponible() bool {
	return s != nil && s.store != nil && s.hasher != nil
}

// ValidateResetConfirmCommand valida el cuerpo de E006 sin normalizar el token: es un
// secreto sensible a mayúsculas y espacios, y nunca se registra.
func ValidateResetConfirmCommand(in ResetConfirmCommand) (ResetConfirmCommand, error) {
	var out ResetConfirmCommand

	if in.Token == "" {
		return out, &FieldViolation{Field: "token", Reason: "required", Business: true}
	}
	if utf8.RuneCountInString(in.Token) > resetTokenMaxRunes {
		return out, &FieldViolation{Field: "token", Reason: "too_long", Business: true}
	}
	for _, r := range in.Token {
		if r <= 0x20 || r == 0x7f {
			return out, &FieldViolation{Field: "token", Reason: "invalid_value", Business: true}
		}
	}
	out.Token = in.Token

	// La contraseña no se normaliza: ni trim ni lowercase.
	runas := utf8.RuneCountInString(in.NewPassword)
	if runas < ResetPasswordMinRunes || runas > ResetPasswordMaxRunes {
		return out, &FieldViolation{Field: "newPassword", Reason: "invalid_length", Business: true}
	}
	if strings.TrimSpace(in.NewPassword) == "" {
		return out, &FieldViolation{Field: "newPassword", Reason: "blank", Business: true}
	}
	out.NewPassword = in.NewPassword

	return out, nil
}

// Confirm aplica el cambio real de contraseña.
//
// Secuencia: validar entrada → resolver `RESET#<sha256(token)>` con consistencia fuerte →
// rechazar usado (409), expirado o inexistente (400) → leer el perfil ACTIVE → derivar
// Argon2id → aplicar UNA transacción con el cambio de perfil y el consumo del token. La
// contraseña nunca se modifica si la transacción no confirma.
func (s *PasswordResetConfirmService) Confirm(ctx context.Context, in ResetConfirmCommand) error {
	if !s.Disponible() {
		return ErrResetConfirmUnavailable
	}

	cmd, err := ValidateResetConfirmCommand(in)
	if err != nil {
		return err
	}

	now := s.now().UTC()
	tokenHash := HashToken(cmd.Token)

	token, err := s.store.LoadResetToken(ctx, tokenHash, now)
	if err != nil {
		return ErrResetConfirmUnavailable
	}
	if token == nil {
		return ports.ErrResetTokenInvalido
	}
	if token.Used {
		return ErrResetTokenUsado
	}
	if !token.HasExpiry || !token.ExpiresAt.After(now) {
		// Caducidad lógica explícita: no se depende de la purga física por TTL.
		return ports.ErrResetTokenInvalido
	}
	if strings.TrimSpace(token.UserID) == "" {
		return ports.ErrResetTokenInvalido
	}

	profile, err := s.store.LoadResetProfile(ctx, token.UserID)
	if err != nil {
		return ErrResetConfirmUnavailable
	}
	if profile == nil {
		return ports.ErrResetTokenInvalido
	}
	if profile.UserID != "" && profile.UserID != token.UserID {
		return ports.ErrResetTokenInvalido
	}
	if !strings.EqualFold(strings.TrimSpace(profile.Status), StatusActive) {
		return ports.ErrResetTokenInvalido
	}

	passwordHash, err := s.hasher.HashPassword(cmd.NewPassword)
	if err != nil {
		return ErrResetConfirmUnavailable
	}

	err = s.store.ApplyReset(ctx, ResetApplication{
		UserID:               token.UserID,
		TokenHash:            tokenHash,
		PasswordHash:         passwordHash,
		ExpectedVersion:      profile.Version,
		HasVersion:           profile.HasVersion,
		ExpectedSessionEpoch: profile.SessionEpoch,
		NextSessionEpoch:     profile.SessionEpoch + 1,
		Now:                  now,
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrResetEnProceso):
			return ErrResetEnProceso
		case errors.Is(err, ErrResetTokenUsado):
			return ErrResetTokenUsado
		default:
			return ErrResetConfirmUnavailable
		}
	}
	return nil
}
