package identity

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	domain "github.com/nexumpr/Nexum-Backend/internal/domain/identity"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
)

// Reglas de E002 V6.0.2 (RFC-013).
var (
	// ErrInvalidCredentials es el único error público de autenticación: no distingue
	// correo inexistente, contraseña incorrecta ni cuenta deshabilitada.
	ErrInvalidCredentials = errors.New("credenciales invalidas")

	// ErrLoginUnavailable indica que falta una dependencia crítica (persistencia o
	// material criptográfico) y el login debe fallar cerrado con 503.
	ErrLoginUnavailable = errors.New("servicio de autenticacion no disponible")

	// ErrAccountDisabled se usa sólo internamente; nunca se expone al cliente.
	ErrAccountDisabled = errors.New("la cuenta no puede autenticarse")
)

const (
	// AccessTokenTTL es la vida del access token: 900 s (política transversal H06).
	AccessTokenTTL = 900 * time.Second

	// RefreshTokenTTL es la vida del refresh token antes de exigir un nuevo login.
	RefreshTokenTTL = 30 * 24 * time.Hour

	// TokenType es el literal exacto del contrato.
	TokenType = "Bearer"

	maxLoginPasswordRunes = 128
	maxDeviceIDRunes      = 128
	maxDeviceName         = 80
	plataformaAndroid     = "android"
)

// LoginCommand es la entrada de aplicación de E002; no contiene dependencias HTTP.
type LoginCommand struct {
	Email      string
	Password   string
	DeviceID   string
	DeviceName string
	Platform   string
	ClientIP   string
}

// LoginSession es el alias de respuesta de la sesión creada. No es una sesión adicional.
type LoginSession struct {
	ID string `json:"id"`
}

// LoginUser es la identidad real del perfil persistido por E001.
type LoginUser struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"emailVerified"`
}

// LoginOnboarding expone el estado real de configuración inicial.
type LoginOnboarding struct {
	Configured bool `json:"configured"`
}

// LoginResponse es el contrato completo V6.0.2 de E002. Conserva los cinco campos
// heredados de AuthTokens y añade sesión, usuario y onboarding.
type LoginResponse struct {
	AccessToken  string          `json:"accessToken"`
	RefreshToken string          `json:"refreshToken"`
	TokenType    string          `json:"tokenType"`
	ExpiresIn    int64           `json:"expiresIn"`
	SessionID    string          `json:"sessionId"`
	Session      LoginSession    `json:"session"`
	User         LoginUser       `json:"user"`
	Onboarding   LoginOnboarding `json:"onboarding"`
}

// StoredIdentity es la identidad mínima que el login necesita leer.
type StoredIdentity struct {
	UserID        string
	Email         string
	DisplayName   string
	PasswordHash  string
	EmailVerified bool
	Status        string
}

// SessionRecord es la sesión que se persiste de forma atómica con su localizador de refresh.
type SessionRecord struct {
	UserID      string
	SessionID   string
	FamilyID    string
	RefreshHash string
	AccessJTI   string
	DeviceID    string
	DeviceName  string
	Platform    string
	CreatedAt   time.Time
	ExpiresAt   time.Time
	Status      string
}

// LoginStore es el puerto de persistencia de E002: lectura de identidad y escritura
// atómica de la sesión. Nunca usa Scan ni memoria en producción.
type LoginStore interface {
	LookupIdentity(ctx context.Context, normalizedEmail string) (*StoredIdentity, error)
	OnboardingConfigured(ctx context.Context, userID string) (bool, error)
	CreateSession(ctx context.Context, record SessionRecord) error
}

// AccessTokenIssuer abstrae la emisión del JWT para poder probar sin claves reales.
type AccessTokenIssuer interface {
	IssueAccessToken(ctx context.Context, userID, sessionID string, now time.Time) (token, jti string, err error)
}

// LoginService orquesta la autenticación completa de E002.
type LoginService struct {
	store   LoginStore
	hasher  ports.PasswordHasher
	issuer  AccessTokenIssuer
	refresh func() (string, error)
	newID   func() (string, error)
	now     func() time.Time
	// decoyHash se deriva en construcción con el hasher productivo y una contraseña
	// aleatoria: garantiza formato Argon2id válido y coste comparable, sin guardar un
	// literal dudoso ni ninguna contraseña real.
	decoyHash string
}

// NewLoginService construye el servicio. Si falta una dependencia crítica, Login falla
// cerrado con ErrLoginUnavailable (nunca emite tokens ficticios).
func NewLoginService(store LoginStore, hasher ports.PasswordHasher, issuer AccessTokenIssuer) *LoginService {
	servicio := &LoginService{
		store:   store,
		hasher:  hasher,
		issuer:  issuer,
		refresh: newOpaqueToken,
		newID:   domain.NewID,
		now:     func() time.Time { return time.Now().UTC() },
	}
	if hasher != nil {
		semilla := make([]byte, 32)
		if _, err := rand.Read(semilla); err == nil {
			if hash, errHash := hasher.HashPassword(base64.RawURLEncoding.EncodeToString(semilla)); errHash == nil {
				servicio.decoyHash = hash
			}
		}
	}
	return servicio
}

// WithClock permite inyectar un reloj determinista en pruebas.
func (s *LoginService) WithClock(now func() time.Time) *LoginService {
	if now != nil {
		s.now = now
	}
	return s
}

// WithIDGenerator permite inyectar un generador determinista en pruebas.
func (s *LoginService) WithIDGenerator(gen func() (string, error)) *LoginService {
	if gen != nil {
		s.newID = gen
	}
	return s
}

// WithRefreshGenerator permite inyectar el generador del refresh opaco en pruebas.
func (s *LoginService) WithRefreshGenerator(gen func() (string, error)) *LoginService {
	if gen != nil {
		s.refresh = gen
	}
	return s
}

// ValidateLoginCommand normaliza y valida la entrada de E002 V6.0.2.
func ValidateLoginCommand(in LoginCommand) (LoginCommand, error) {
	var out LoginCommand

	cleanEmail, err := domain.ValidateEmail(in.Email)
	if err != nil {
		return out, &FieldViolation{Field: "email", Reason: "invalid_email"}
	}
	out.Email = cleanEmail

	// La contraseña no se normaliza: ni trim ni lowercase.
	if in.Password == "" {
		return out, &FieldViolation{Field: "password", Reason: "required"}
	}
	if utf8.RuneCountInString(in.Password) > maxLoginPasswordRunes {
		return out, &FieldViolation{Field: "password", Reason: "too_long"}
	}
	out.Password = in.Password

	deviceID := strings.TrimSpace(in.DeviceID)
	if utf8.RuneCountInString(deviceID) > maxDeviceIDRunes || hasControlRune(deviceID) {
		return out, &FieldViolation{Field: "deviceId", Reason: "invalid_value"}
	}
	out.DeviceID = deviceID

	deviceName := strings.TrimSpace(in.DeviceName)
	if utf8.RuneCountInString(deviceName) > maxDeviceName || hasControlRune(deviceName) {
		return out, &FieldViolation{Field: "deviceName", Reason: "invalid_value"}
	}
	out.DeviceName = deviceName

	plataforma := strings.ToLower(strings.TrimSpace(in.Platform))
	if plataforma != "" && plataforma != plataformaAndroid {
		return out, &FieldViolation{Field: "platform", Reason: "unsupported_platform"}
	}
	out.Platform = plataforma
	out.ClientIP = in.ClientIP

	return out, nil
}

// Login autentica y crea la sesión persistida. Devuelve siempre el mismo error público
// ante cualquier fallo de credenciales o de estado de cuenta (sin enumeración).
func (s *LoginService) Login(ctx context.Context, in LoginCommand) (*LoginResponse, error) {
	if s == nil || s.store == nil || s.hasher == nil || s.issuer == nil {
		return nil, ErrLoginUnavailable
	}

	cmd, err := ValidateLoginCommand(in)
	if err != nil {
		return nil, err
	}

	identity, lookupErr := s.store.LookupIdentity(ctx, cmd.Email)
	if lookupErr != nil && !errors.Is(lookupErr, ErrInvalidCredentials) {
		return nil, ErrLoginUnavailable
	}
	if identity == nil {
		// Trabajo señuelo con Argon2id real: reduce la diferencia temporal observable
		// sin prometer igualdad temporal matemática.
		if s.decoyHash != "" {
			_, _ = s.hasher.VerifyPassword(cmd.Password, s.decoyHash)
		}
		return nil, ErrInvalidCredentials
	}

	ok, verifyErr := s.hasher.VerifyPassword(cmd.Password, identity.PasswordHash)
	if verifyErr != nil || !ok {
		return nil, ErrInvalidCredentials
	}

	if !strings.EqualFold(strings.TrimSpace(identity.Status), "ACTIVE") {
		return nil, ErrInvalidCredentials
	}

	configured, errOnboarding := s.store.OnboardingConfigured(ctx, identity.UserID)
	if errOnboarding != nil {
		return nil, ErrLoginUnavailable
	}

	now := s.now().UTC()
	sessionID, err := s.newID()
	if err != nil {
		return nil, ErrLoginUnavailable
	}
	familyID, err := s.newID()
	if err != nil {
		return nil, ErrLoginUnavailable
	}
	refreshToken, err := s.refresh()
	if err != nil {
		return nil, ErrLoginUnavailable
	}

	accessToken, jti, err := s.issuer.IssueAccessToken(ctx, identity.UserID, sessionID, now)
	if err != nil {
		return nil, ErrLoginUnavailable
	}

	record := SessionRecord{
		UserID:      identity.UserID,
		SessionID:   sessionID,
		FamilyID:    familyID,
		RefreshHash: HashToken(refreshToken),
		AccessJTI:   jti,
		DeviceID:    cmd.DeviceID,
		DeviceName:  cmd.DeviceName,
		Platform:    cmd.Platform,
		CreatedAt:   now,
		ExpiresAt:   now.Add(RefreshTokenTTL),
		Status:      "ACTIVE",
	}
	if err := s.store.CreateSession(ctx, record); err != nil {
		return nil, ErrLoginUnavailable
	}

	return &LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    TokenType,
		ExpiresIn:    int64(AccessTokenTTL / time.Second),
		SessionID:    sessionID,
		Session:      LoginSession{ID: sessionID},
		User: LoginUser{
			ID:            identity.UserID,
			Name:          identity.DisplayName,
			Email:         identity.Email,
			EmailVerified: identity.EmailVerified,
		},
		Onboarding: LoginOnboarding{Configured: configured},
	}, nil
}

// SessionDigest expone el digest usado como localizador de refresh (sólo para pruebas
// y para el adaptador; el token en claro nunca se persiste).
func SessionDigest(token string) string { return HashToken(token) }

// newOpaqueToken genera 32 bytes CSPRNG en base64url (256 bits de entropía).
func newOpaqueToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("no se pudo generar el refresh token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
