package identity

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

// Errores de E011 (confirmacion de verificacion de correo).
//
// El contrato OpenAPI fija 200/400/409/422/429/500/503. La ruta es publica: la autoridad es
// el token de un solo uso, no una sesion.
var (
	// ErrEmailVerificationTokenInvalido: token inexistente, expirado, de otro proposito o de
	// una cuenta que ya no existe o no esta ACTIVE. Responde 400 generico (no enumera).
	ErrEmailVerificationTokenInvalido = errors.New("token de verificacion invalido")
	// ErrEmailVerificationTokenUsado: el token ya se consumio (replay). Responde 409.
	ErrEmailVerificationTokenUsado = errors.New("token de verificacion ya utilizado")
	// ErrEmailVerificationConflicto: la transaccion atomica perdio una carrera (el perfil ya
	// estaba verificado o su version cambio). Responde 409.
	ErrEmailVerificationConflicto = errors.New("conflicto al confirmar la verificacion")
	// ErrEmailVerificationConfirmNoDisponible: falta persistencia o un fallo real de DynamoDB.
	// Responde 503 y nunca confirma una verificacion que no se aplico.
	ErrEmailVerificationConfirmNoDisponible = errors.New("confirmacion de verificacion no disponible")
)

// VerifyEmailTokenMinRunes es el minLength del contrato OpenAPI (VerifyEmailRequest.token).
const VerifyEmailTokenMinRunes = 10

// verifyEmailTokenMaxRunes acota el token sin admitir cuerpos abusivos: el emisor de E010
// produce 43 caracteres base64url.
const verifyEmailTokenMaxRunes = 512

// Proposito y entidad del item ONE_USE. Un token de otro flujo nunca verifica un correo.
const (
	EmailVerificationPurpose    = "EMAIL_VERIFY"
	EmailVerificationEntityType = "EMAIL_VERIFICATION"
)

// EmailVerificationTokenRecord es la lectura autoritativa de
// `ONE_USE#<sha256(token)>/VERIFY`.
type EmailVerificationTokenRecord struct {
	UserID     string
	Email      string
	Purpose    string
	EntityType string
	ExpiresAt  time.Time
	HasExpiry  bool
	Used       bool
}

// EmailVerificationConfirmation describe la transaccion atomica de E011: dos acciones sobre
// dos items distintos (DynamoDB prohibe dos acciones sobre el mismo item).
type EmailVerificationConfirmation struct {
	UserID    string
	TokenHash string
	// ExpectedVersion es la version leida del perfil. Si HasVersion es false (perfil legado
	// sin atributo) la condicion usa attribute_not_exists(version).
	ExpectedVersion int64
	HasVersion      bool
	Now             time.Time
}

// EmailVerificationConfirmStore es el puerto de persistencia de E011. Extiende el de E010 con
// la lectura del token y la transaccion de consumo.
type EmailVerificationConfirmStore interface {
	EmailVerificationStore
	// LoadEmailVerificationToken lee el token con consistencia fuerte y aplica la caducidad
	// logica (no depende de cuando DynamoDB lo elimine fisicamente por TTL).
	LoadEmailVerificationToken(ctx context.Context, tokenHash string) (*EmailVerificationTokenRecord, error)
	// ConfirmEmailVerification ejecuta UNA TransactWriteItems con dos acciones: marcar
	// emailVerified en el perfil y consumir el token.
	ConfirmEmailVerification(ctx context.Context, confirmation EmailVerificationConfirmation) error
}

// EmailVerificationConfirmService orquesta la confirmacion de E011.
type EmailVerificationConfirmService struct {
	store EmailVerificationConfirmStore
	now   func() time.Time
}

// NewEmailVerificationConfirmService construye el servicio. Sin store, Confirmar falla
// cerrado con ErrEmailVerificationConfirmNoDisponible.
func NewEmailVerificationConfirmService(store EmailVerificationConfirmStore) *EmailVerificationConfirmService {
	return &EmailVerificationConfirmService{
		store: store,
		now:   func() time.Time { return time.Now().UTC() },
	}
}

// WithClock permite inyectar un reloj determinista en pruebas.
func (s *EmailVerificationConfirmService) WithClock(now func() time.Time) *EmailVerificationConfirmService {
	if now != nil {
		s.now = now
	}
	return s
}

// Disponible indica si el endpoint puede operar de verdad.
func (s *EmailVerificationConfirmService) Disponible() bool {
	return s != nil && s.store != nil
}

// ValidateVerifyEmailToken valida el token sin normalizarlo: es un secreto sensible a
// mayusculas y espacios y nunca se registra.
func ValidateVerifyEmailToken(token string) (string, error) {
	if token == "" {
		return "", &FieldViolation{Field: "token", Reason: "required", Business: true}
	}
	if utf8.RuneCountInString(token) > verifyEmailTokenMaxRunes {
		return "", &FieldViolation{Field: "token", Reason: "too_long", Business: true}
	}
	for _, r := range token {
		if r <= 0x20 || r == 0x7f {
			return "", &FieldViolation{Field: "token", Reason: "invalid_value", Business: true}
		}
	}
	return token, nil
}

// Confirmar consume el token y marca el correo como verificado en una sola transaccion.
//
// Secuencia: validar token → leer ONE_USE#<sha256>/VERIFY con consistencia fuerte → rechazar
// usado (409), expirado o ajeno (400) → leer el perfil ACTIVE → aplicar la transaccion. La
// verificacion nunca se marca antes de consumir el token.
func (s *EmailVerificationConfirmService) Confirmar(ctx context.Context, token string) error {
	if !s.Disponible() {
		return ErrEmailVerificationConfirmNoDisponible
	}

	valor, err := ValidateVerifyEmailToken(token)
	if err != nil {
		return err
	}

	ahora := s.now().UTC()
	hash := HashToken(valor)

	registro, err := s.store.LoadEmailVerificationToken(ctx, hash)
	if err != nil {
		return ErrEmailVerificationConfirmNoDisponible
	}
	if registro == nil {
		return ErrEmailVerificationTokenInvalido
	}
	if !strings.EqualFold(registro.Purpose, EmailVerificationPurpose) ||
		!strings.EqualFold(registro.EntityType, EmailVerificationEntityType) {
		// Un token de otro flujo (por ejemplo recuperacion) no verifica correos.
		return ErrEmailVerificationTokenInvalido
	}
	if registro.Used {
		return ErrEmailVerificationTokenUsado
	}
	if !registro.HasExpiry || !registro.ExpiresAt.After(ahora) {
		// Caducidad logica explicita: no se depende de la purga fisica por TTL.
		return ErrEmailVerificationTokenInvalido
	}
	userID := strings.TrimSpace(registro.UserID)
	if userID == "" {
		return ErrEmailVerificationTokenInvalido
	}

	perfil, err := s.store.LoadEmailVerificationProfile(ctx, userID)
	if err != nil {
		return ErrEmailVerificationConfirmNoDisponible
	}
	if perfil == nil {
		return ErrEmailVerificationTokenInvalido
	}
	if !strings.EqualFold(strings.TrimSpace(perfil.Status), StatusActive) {
		return ErrEmailVerificationTokenInvalido
	}
	if perfil.EmailVerified {
		// Ya verificado por otra confirmacion: el token no se consume y se responde 409.
		return ErrEmailVerificationConflicto
	}

	err = s.store.ConfirmEmailVerification(ctx, EmailVerificationConfirmation{
		UserID:          userID,
		TokenHash:       hash,
		ExpectedVersion: perfil.Version,
		HasVersion:      perfil.HasVersion,
		Now:             ahora,
	})
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrEmailVerificationTokenUsado):
		return ErrEmailVerificationTokenUsado
	case errors.Is(err, ErrEmailVerificationConflicto):
		// Si el perfil desaparecio entre la lectura y la transaccion, la respuesta correcta
		// es el rechazo generico (400), no un conflicto de concurrencia.
		actual, errPerfil := s.store.LoadEmailVerificationProfile(ctx, userID)
		if errPerfil != nil {
			return ErrEmailVerificationConfirmNoDisponible
		}
		if actual == nil {
			return ErrEmailVerificationTokenInvalido
		}
		return ErrEmailVerificationConflicto
	default:
		return ErrEmailVerificationConfirmNoDisponible
	}
}
