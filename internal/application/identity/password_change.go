package identity

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nexumpr/Nexum-Backend/internal/ports"
)

// Reglas de E007 (`POST /api/v1/auth/password/change`) sobre la época de revocación A6
// (RFC-016 §17.3, RFC-018).
var (
	// ErrPasswordChangeUnauthorized indica que la cuenta no existe, no está ACTIVE o no
	// puede acreditar su titularidad. Responde 401 con mensaje genérico: no se distingue
	// "cuenta inexistente" de "cuenta deshabilitada" (AC-A-050-03).
	ErrPasswordChangeUnauthorized = errors.New("cuenta no autorizada para cambiar la contrasena")

	// ErrPasswordChangeActualIncorrecta indica que `currentPassword` no verifica contra el
	// hash persistido. Responde 422 con mensaje genérico: nunca se revela si la cuenta
	// existe, si el hash está corrupto ni cuántos intentos quedan.
	ErrPasswordChangeActualIncorrecta = errors.New("contrasena actual incorrecta")

	// ErrPasswordChangeConflict indica que la transacción condicional perdió una carrera:
	// otra solicitud cambió la versión del perfil entre la lectura y la escritura. Responde
	// 409 y no se modificó nada.
	ErrPasswordChangeConflict = errors.New("conflicto al aplicar el cambio de contrasena")

	// ErrPasswordChangeUnavailable indica falta de persistencia, del hasher o un fallo de
	// DynamoDB. Responde 503 y nunca confirma un cambio que no se aplicó.
	ErrPasswordChangeUnavailable = errors.New("cambio de contrasena no disponible")
)

const (
	// Longitudes del contrato OpenAPI PasswordChangeRequest: `newPassword` declara
	// minLength 12. El máximo replica la política ya aprobada para E005/E006.
	ChangePasswordMinRunes = 12
	ChangePasswordMaxRunes = 128

	// `currentPassword` sólo declara minLength 1 en el contrato. Este tope es defensa en
	// profundidad contra cuerpos abusivos: queda muy por encima de cualquier contraseña
	// real, no añade un máximo contractual nuevo y evita derivar Argon2id sobre entradas
	// desproporcionadas.
	changePasswordCurrentMaxRunes = 1024
)

// PasswordChangeCommand es la entrada de aplicación de E007. No contiene dependencias HTTP.
//
// `UserID` proviene EXCLUSIVAMENTE del claim `sub` ya verificado por el middleware: el
// cliente no elige la cuenta cuyo secreto modifica (R-031).
type PasswordChangeCommand struct {
	UserID          string
	CurrentPassword string
	NewPassword     string
}

// PasswordChangeProfile es la lectura autoritativa de `USER#<id>/PROFILE`.
type PasswordChangeProfile struct {
	UserID       string
	Status       string
	PasswordHash string
	// Version es la versión optimista del perfil. HasVersion es false en perfiles
	// anteriores a la introducción del atributo (E002 primitivo).
	Version    int64
	HasVersion bool
	// SessionEpoch es la época de revocación vigente (A6). Ausente equivale a 0.
	SessionEpoch int64
}

// PasswordChangeApplication describe la transacción atómica de E007: UNA sola acción de
// escritura sobre el perfil.
//
// El incremento de `sessionEpoch` en la MISMA escritura que el nuevo `passwordHash` es lo
// que invalida todas las sesiones —incluida la actual—: `internal/application/identity/
// refresh.go` rechaza renovar cuando la época de la sesión no coincide con la del perfil y
// el middleware de autenticación rechaza cualquier access token cuyo claim `epo` quedó
// desalineado. No se reemiten tokens: el contrato responde `Accepted` y el cliente
// reautentica.
type PasswordChangeApplication struct {
	UserID       string
	PasswordHash string
	// ExpectedVersion es la versión leída del perfil. Si HasVersion es false la condición
	// usa attribute_not_exists(version) y la transacción la inicializa.
	ExpectedVersion int64
	HasVersion      bool
	// ExpectedSessionEpoch es la época leída; la transacción exige que siga siendo la
	// vigente (o ausente, para perfiles anteriores a A6) y escribe NextSessionEpoch.
	ExpectedSessionEpoch int64
	NextSessionEpoch     int64
	Now                  time.Time
}

// PasswordChangeStore es el puerto de persistencia de E007. Se implementa sobre la tabla
// única existente; nunca usa Scan ni memoria como autoridad.
type PasswordChangeStore interface {
	// LoadPasswordChangeProfile lee `USER#<id>/PROFILE` con consistencia fuerte. Devuelve
	// (nil, nil) cuando el perfil no existe.
	LoadPasswordChangeProfile(ctx context.Context, userID string) (*PasswordChangeProfile, error)
	// ApplyPasswordChange ejecuta UNA TransactWriteItems con UNA acción condicional sobre
	// el perfil: nuevo hash, `passwordChangedAt`, `sessionEpoch` incrementada y `version`
	// incrementada.
	ApplyPasswordChange(ctx context.Context, apply PasswordChangeApplication) error
}

// PasswordChangeService orquesta el cambio de contraseña de una sesión activa (E007).
type PasswordChangeService struct {
	store  PasswordChangeStore
	hasher ports.PasswordHasher
	now    func() time.Time
}

// NewPasswordChangeService construye el servicio. Si falta una dependencia crítica, Change
// falla cerrado con ErrPasswordChangeUnavailable (nunca simula el cambio).
func NewPasswordChangeService(store PasswordChangeStore, hasher ports.PasswordHasher) *PasswordChangeService {
	return &PasswordChangeService{
		store:  store,
		hasher: hasher,
		now:    func() time.Time { return time.Now().UTC() },
	}
}

// WithClock permite inyectar un reloj determinista en pruebas.
func (s *PasswordChangeService) WithClock(now func() time.Time) *PasswordChangeService {
	if now != nil {
		s.now = now
	}
	return s
}

// Disponible indica si el endpoint puede operar de verdad.
func (s *PasswordChangeService) Disponible() bool {
	return s != nil && s.store != nil && s.hasher != nil
}

// ValidatePasswordChangeCommand valida el cuerpo de E007 sin normalizar las contraseñas: ni
// trim ni lowercase. `currentPassword` es un secreto vigente y `newPassword` puede contener
// espacios significativos; sólo se rechazan longitudes fuera de política y valores en blanco.
func ValidatePasswordChangeCommand(in PasswordChangeCommand) (PasswordChangeCommand, error) {
	var out PasswordChangeCommand

	if strings.TrimSpace(in.CurrentPassword) == "" {
		return out, &FieldViolation{Field: "currentPassword", Reason: "required", Business: true}
	}
	if utf8.RuneCountInString(in.CurrentPassword) > changePasswordCurrentMaxRunes {
		return out, &FieldViolation{Field: "currentPassword", Reason: "too_long", Business: true}
	}
	out.CurrentPassword = in.CurrentPassword

	runas := utf8.RuneCountInString(in.NewPassword)
	if runas < ChangePasswordMinRunes || runas > ChangePasswordMaxRunes {
		return out, &FieldViolation{Field: "newPassword", Reason: "invalid_length", Business: true}
	}
	if strings.TrimSpace(in.NewPassword) == "" {
		return out, &FieldViolation{Field: "newPassword", Reason: "blank", Business: true}
	}
	out.NewPassword = in.NewPassword

	out.UserID = strings.TrimSpace(in.UserID)
	return out, nil
}

// Change aplica el cambio real de contraseña.
//
// Secuencia: validar entrada → exigir identidad → leer el perfil con consistencia fuerte →
// exigir ACTIVE → verificar `currentPassword` contra el hash persistido → derivar Argon2id
// del nuevo secreto → aplicar UNA transacción condicional que incrementa la época.
//
// Un fallo en cualquier paso deja la contraseña intacta. `currentPassword` incorrecta y
// cuenta no utilizable devuelven errores distintos pero ambos genéricos en HTTP; en ningún
// caso se confirma un cambio que no se persistió.
func (s *PasswordChangeService) Change(ctx context.Context, in PasswordChangeCommand) error {
	if !s.Disponible() {
		return ErrPasswordChangeUnavailable
	}

	cmd, err := ValidatePasswordChangeCommand(in)
	if err != nil {
		return err
	}
	if cmd.UserID == "" {
		// Sin `sub` verificado no hay titularidad que acreditar.
		return ErrPasswordChangeUnauthorized
	}

	profile, err := s.store.LoadPasswordChangeProfile(ctx, cmd.UserID)
	if err != nil {
		return ErrPasswordChangeUnavailable
	}
	if profile == nil {
		return ErrPasswordChangeUnauthorized
	}
	if profile.UserID != "" && profile.UserID != cmd.UserID {
		// Defensa en profundidad: el ítem leído debe pertenecer al sujeto del token.
		return ErrPasswordChangeUnauthorized
	}
	if !strings.EqualFold(strings.TrimSpace(profile.Status), StatusActive) {
		return ErrPasswordChangeUnauthorized
	}
	if strings.TrimSpace(profile.PasswordHash) == "" {
		// Sin hash no se puede acreditar la contraseña vigente: se rechaza en lugar de
		// aceptar un cambio sobre una cuenta incoherente.
		return ErrPasswordChangeUnauthorized
	}

	verificada, err := s.hasher.VerifyPassword(cmd.CurrentPassword, profile.PasswordHash)
	if err != nil {
		// El hasher Argon2id devuelve `ports.ErrInvalidPassword` cuando la contraseña no
		// coincide. Es una violación de regla (422), no una caída de dependencia.
		if errors.Is(err, ports.ErrInvalidPassword) {
			return ErrPasswordChangeActualIncorrecta
		}
		return ErrPasswordChangeUnavailable
	}
	if !verificada {
		return ErrPasswordChangeActualIncorrecta
	}

	passwordHash, err := s.hasher.HashPassword(cmd.NewPassword)
	if err != nil {
		return ErrPasswordChangeUnavailable
	}

	err = s.store.ApplyPasswordChange(ctx, PasswordChangeApplication{
		UserID:               cmd.UserID,
		PasswordHash:         passwordHash,
		ExpectedVersion:      profile.Version,
		HasVersion:           profile.HasVersion,
		ExpectedSessionEpoch: profile.SessionEpoch,
		NextSessionEpoch:     profile.SessionEpoch + 1,
		Now:                  s.now().UTC(),
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrPasswordChangeConflict):
			return ErrPasswordChangeConflict
		case errors.Is(err, ErrPasswordChangeUnauthorized):
			return ErrPasswordChangeUnauthorized
		default:
			return ErrPasswordChangeUnavailable
		}
	}
	return nil
}
