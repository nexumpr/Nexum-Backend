package identity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	domain "github.com/nexumpr/Nexum-Backend/internal/domain/identity"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
)

var (
	// ErrEmailAlreadyRegistered indica que el email ya tiene una reserva activa.
	ErrEmailAlreadyRegistered = errors.New("el correo electronico ya se encuentra registrado")

	// ErrIdempotencyConflict indica que la misma Idempotency-Key se reutilizo con otro payload.
	ErrIdempotencyConflict = errors.New("la Idempotency-Key ya fue utilizada con una carga util diferente")

	// ErrStoreUnavailable indica que DynamoDB no puede atender la operacion (503).
	ErrStoreUnavailable = errors.New("persistencia no disponible")

	// ErrRegistrationInternal indica un fallo interno no clasificable.
	ErrRegistrationInternal = errors.New("fallo interno al registrar usuario")
)

const (
	// RegisterOperation identifica la operacion en el registro de idempotencia.
	RegisterOperation = "E001_REGISTER"

	// RegisterIdempotencyTTL es la ventana de replay de la Idempotency-Key.
	RegisterIdempotencyTTL = 24 * time.Hour

	minPasswordRunes = 12
	maxPasswordRunes = 128
	minIdemKeyBytes  = 16
	maxIdemKeyBytes  = 128
)

// FieldViolation describe un error de validacion atribuible a un campo del contrato.
// Business=true se traduce a 422; Business=false a 400.
type FieldViolation struct {
	Field    string
	Reason   string
	Business bool
}

func (v *FieldViolation) Error() string {
	return fmt.Sprintf("campo %q invalido: %s", v.Field, v.Reason)
}

// RegisterCommand es la entrada de aplicacion de E001. No contiene dependencias HTTP.
type RegisterCommand struct {
	Email                string
	Password             string
	DisplayName          string
	PrivacyNoticeVersion string
	PrivacyAccepted      bool
	IdempotencyKey       string
}

// UserProfile es la respuesta contractual de E001. CreatedAt se serializa como
// RFC3339 UTC para que el replay sea byte a byte estable.
type UserProfile struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	DisplayName   string `json:"displayName"`
	EmailVerified bool   `json:"emailVerified"`
	CreatedAt     string `json:"createdAt"`
	Version       int64  `json:"version"`
}

// StoredResponse es la respuesta HTTP inmutable que se guarda con la Idempotency-Key.
// Replayed solo se marca en memoria al recuperar una respuesta ya almacenada.
type StoredResponse struct {
	Status   int
	Body     []byte
	Location string
	Replayed bool
}

// RegistrationRecord es la fila que se persiste atomicamente con la reserva de email
// y el registro de idempotencia. No contiene la contrasena en claro.
type RegistrationRecord struct {
	UserID               string
	Email                string
	DisplayName          string
	PasswordHash         string
	PrivacyNoticeVersion string
	PrivacyAcceptedAt    time.Time
	EmailVerified        bool
	Version              int64
	CreatedAt            time.Time
}

// RegisterStore es el puerto atomico de E001. CreateOrReplay debe:
//   - crear reserva de email, perfil e idempotencia en una sola transaccion;
//   - devolver la respuesta guardada si la clave ya existe con el mismo payload;
//   - devolver ErrIdempotencyConflict si el payload difiere;
//   - devolver ErrEmailAlreadyRegistered si la reserva de email ya existe;
//   - devolver ErrStoreUnavailable si la persistencia no responde.
type RegisterStore interface {
	CreateOrReplay(
		ctx context.Context,
		idempotencyKey, operation, payloadHash string,
		record RegistrationRecord,
		response StoredResponse,
		ttl time.Duration,
	) (*StoredResponse, error)
}

// RegisterService orquesta el registro E001 sin conocer HTTP ni AWS.
type RegisterService struct {
	store    RegisterStore
	hasher   ports.PasswordHasher
	now      func() time.Time
	newID    func() (string, error)
	location string
}

// NewRegisterService construye el servicio con reloj y generador de UUID reales.
func NewRegisterService(store RegisterStore, hasher ports.PasswordHasher) *RegisterService {
	return &RegisterService{
		store:    store,
		hasher:   hasher,
		now:      func() time.Time { return time.Now().UTC() },
		newID:    domain.NewUserID,
		location: "/api/v1/users/me",
	}
}

// WithClock permite inyectar un reloj determinista en pruebas.
func (s *RegisterService) WithClock(now func() time.Time) *RegisterService {
	if now != nil {
		s.now = now
	}
	return s
}

// WithIDGenerator permite inyectar un generador determinista en pruebas.
func (s *RegisterService) WithIDGenerator(gen func() (string, error)) *RegisterService {
	if gen != nil {
		s.newID = gen
	}
	return s
}

// ValidateRegisterCommand normaliza y valida la entrada. Devuelve una copia normalizada.
func ValidateRegisterCommand(in RegisterCommand) (RegisterCommand, error) {
	normalized := in

	cleanEmail, err := domain.ValidateEmail(in.Email)
	if err != nil {
		return normalized, &FieldViolation{Field: "email", Reason: "invalid_email"}
	}
	normalized.Email = cleanEmail

	passwordRunes := utf8.RuneCountInString(in.Password)
	if passwordRunes < minPasswordRunes || passwordRunes > maxPasswordRunes {
		return normalized, &FieldViolation{Field: "password", Reason: "invalid_length"}
	}
	if strings.TrimSpace(in.Password) == "" {
		return normalized, &FieldViolation{Field: "password", Reason: "blank", Business: true}
	}

	displayName := strings.TrimSpace(in.DisplayName)
	if displayName == "" {
		return normalized, &FieldViolation{Field: "displayName", Reason: "blank", Business: true}
	}
	if utf8.RuneCountInString(displayName) > 100 {
		return normalized, &FieldViolation{Field: "displayName", Reason: "too_long"}
	}
	if hasControlRune(displayName) {
		return normalized, &FieldViolation{Field: "displayName", Reason: "control_characters", Business: true}
	}
	normalized.DisplayName = displayName

	privacyVersion := strings.TrimSpace(in.PrivacyNoticeVersion)
	if privacyVersion == "" {
		return normalized, &FieldViolation{Field: "privacyNoticeVersion", Reason: "required"}
	}
	if utf8.RuneCountInString(privacyVersion) > 50 {
		return normalized, &FieldViolation{Field: "privacyNoticeVersion", Reason: "too_long"}
	}
	if hasControlRune(privacyVersion) {
		return normalized, &FieldViolation{Field: "privacyNoticeVersion", Reason: "control_characters", Business: true}
	}
	normalized.PrivacyNoticeVersion = privacyVersion

	if !in.PrivacyAccepted {
		return normalized, &FieldViolation{Field: "privacyAccepted", Reason: "must_be_true", Business: true}
	}

	if !ValidIdempotencyKey(in.IdempotencyKey) {
		return normalized, &FieldViolation{Field: "Idempotency-Key", Reason: "invalid"}
	}
	normalized.IdempotencyKey = strings.TrimSpace(in.IdempotencyKey)

	return normalized, nil
}

// ValidIdempotencyKey aplica el contrato 16..128 caracteres imprimibles ASCII.
func ValidIdempotencyKey(key string) bool {
	trimmed := strings.TrimSpace(key)
	if len(trimmed) < minIdemKeyBytes || len(trimmed) > maxIdemKeyBytes {
		return false
	}
	for _, r := range trimmed {
		if r < 0x21 || r > 0x7e {
			return false
		}
	}
	return true
}

func hasControlRune(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

// Register ejecuta E001. La contrasena se hashea con Argon2id antes de persistir y
// nunca se devuelve ni se registra.
func (s *RegisterService) Register(ctx context.Context, in RegisterCommand) (*StoredResponse, error) {
	if s.store == nil || s.hasher == nil {
		return nil, ErrStoreUnavailable
	}

	normalized, err := ValidateRegisterCommand(in)
	if err != nil {
		return nil, err
	}

	passwordHash, err := s.hasher.HashPassword(normalized.Password)
	if err != nil {
		return nil, ErrRegistrationInternal
	}

	now := s.now().UTC()
	userID, err := s.newID()
	if err != nil {
		return nil, ErrRegistrationInternal
	}

	registration, err := domain.NewRegistration(
		userID,
		normalized.Email,
		normalized.DisplayName,
		passwordHash,
		normalized.PrivacyNoticeVersion,
		now,
		now,
	)
	if err != nil {
		return nil, ErrRegistrationInternal
	}

	profile := UserProfile{
		ID:            registration.UserID(),
		Email:         registration.Email(),
		DisplayName:   registration.DisplayName(),
		EmailVerified: registration.EmailVerified(),
		CreatedAt:     registration.CreatedAt().UTC().Format(time.RFC3339),
		Version:       registration.Version(),
	}

	body, err := json.Marshal(profile)
	if err != nil {
		return nil, ErrRegistrationInternal
	}

	record := RegistrationRecord{
		UserID:               registration.UserID(),
		Email:                registration.Email(),
		DisplayName:          registration.DisplayName(),
		PasswordHash:         registration.PasswordHash(),
		PrivacyNoticeVersion: registration.PrivacyNoticeVersion(),
		PrivacyAcceptedAt:    registration.PrivacyAcceptedAt(),
		EmailVerified:        registration.EmailVerified(),
		Version:              registration.Version(),
		CreatedAt:            registration.CreatedAt(),
	}

	response := StoredResponse{
		Status:   201,
		Body:     body,
		Location: s.location,
	}

	replayed, err := s.store.CreateOrReplay(
		ctx,
		normalized.IdempotencyKey,
		RegisterOperation,
		payloadHash(normalized),
		record,
		response,
		RegisterIdempotencyTTL,
	)
	if err != nil {
		return nil, err
	}
	if replayed != nil {
		return replayed, nil
	}
	return &response, nil
}

// payloadHash calcula un hash canonico y determinista del payload normalizado.
// No usa JSON para evitar diferencias de orden/espaciado y no incluye la clave de idempotencia.
func payloadHash(in RegisterCommand) string {
	h := sha256.New()
	parts := []string{in.Email, in.Password, in.DisplayName, in.PrivacyNoticeVersion}
	for _, part := range parts {
		h.Write([]byte(strconv.Itoa(len(part))))
		h.Write([]byte(":"))
		h.Write([]byte(part))
	}
	if in.PrivacyAccepted {
		h.Write([]byte("1"))
	} else {
		h.Write([]byte("0"))
	}
	return hex.EncodeToString(h.Sum(nil))
}
