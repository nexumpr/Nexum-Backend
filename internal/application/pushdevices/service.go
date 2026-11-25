// Package pushdevices contiene los casos de uso de E059 (registro de token push)
// y E060 (baja de dispositivo push).
//
// Invariantes de seguridad:
//   - El titular se deriva siempre del claim `sub` verificado por el middleware.
//   - El token nunca se guarda en claro: se sella con AES-GCM mediante un
//     `TokenSealer` inyectado y sólo se persiste el sobre cifrado junto con una
//     huella SHA-256 no reversible.
//   - La idempotencia se aísla por usuario y por payload canónico. Una clave
//     repetida con el mismo payload devuelve la respuesta estable; con otro
//     payload se rechaza con conflicto.
//   - La rotación de token para la misma etiqueta reutiliza el identificador del
//     dispositivo activo; un dispositivo dado de baja nunca se reactiva.
//   - Sin sellador o sin almacén se falla cerrado (503), nunca se confirma una
//     escritura parcial ni se devuelve un estado inventado.
package pushdevices

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	"github.com/nexumpr/Nexum-Backend/internal/domain/identity"
)

// ProviderFCM es el único proveedor admitido por el esquema PushTokenRegister.
const ProviderFCM = "fcm"

const (
	// MinTokenLen y MaxDeviceLabelRunes reproducen las cotas del OpenAPI.
	MinTokenLen         = 8
	MaxDeviceLabelRunes = 80

	// IdempotencyTTL es la ventana de replay de la Idempotency-Key.
	IdempotencyTTL = 24 * time.Hour
)

// Errores de aplicación. El transporte los traduce al ErrorEnvelope del contrato
// sin revelar tokens, huellas, claves ni datos del titular.
var (
	ErrInvalidRequest = errors.New("solicitud de dispositivo push invalida")
	ErrUnauthorized   = errors.New("dispositivo push no autorizado")
	ErrNotFound       = errors.New("dispositivo push no encontrado")
	ErrConflict       = errors.New("conflicto al registrar dispositivo push")
	ErrUnavailable    = errors.New("servicio de dispositivos push no disponible")

	// ErrIdempotencyCondition y ErrDeviceCondition son señales internas entre el
	// adaptador y el caso de uso para distinguir qué rama de la transacción
	// atómica falló. Nunca se envían al transporte tal cual.
	ErrIdempotencyCondition = errors.New("condicion de idempotencia fallida")
	ErrDeviceCondition      = errors.New("condicion de dispositivo fallida")
)

// TokenSealer cifra el token push. `crypto.Sealer` cumple esta interfaz; el AAD
// incluye usuario e identificador de dispositivo para que un sobre no sirva a
// otro titular ni a otra instalación.
type TokenSealer interface {
	Sellar(token, aad string) ([]byte, error)
}

// Device es la entidad de aplicación. TokenCipher y TokenFingerprint sólo se
// usan en la escritura/atómica; la proyección HTTP no los expone.
type Device struct {
	ID               string
	Provider         string
	DeviceLabel      string
	RegisteredAt     time.Time
	Active           bool
	TokenCipher      []byte
	TokenFingerprint string
}

// WireDevice es la proyección estricta del esquema `PushDevice` del OpenAPI.
type WireDevice struct {
	ID           string `json:"id"`
	Provider     string `json:"provider"`
	DeviceLabel  string `json:"deviceLabel"`
	RegisteredAt string `json:"registeredAt"`
}

// RegisterCommand son los datos ya autenticados que entran a E059.
type RegisterCommand struct {
	UserID         string
	IdempotencyKey string
	Provider       string
	Token          string
	DeviceLabel    string
}

// RegisterWrite es la escritura atómica solicitada al adaptador.
type RegisterWrite struct {
	UserID              string
	IdempotencyKey      string
	PayloadHash         string
	Device              Device
	PreviousFingerprint string
	ResponseJSON        []byte
	Now                 time.Time
	TTL                 time.Duration
}

// IdempotencyRecord es el registro leído para decidir entre replay y conflicto.
// La clave original nunca se persiste: sólo su SHA-256 en la SK física.
type IdempotencyRecord struct {
	DeviceID     string
	PayloadHash  string
	ResponseJSON []byte
	ExpiresAt    int64
}

// RegistrationResult es la salida de E059. ResponseJSON se reenvía byte a byte
// en un replay para que la respuesta estable no dependa del reloj del nodo.
type RegistrationResult struct {
	Device       Device
	ResponseJSON []byte
	Replayed     bool
}

// Store es el puerto de persistencia. La implementación real es
// *dynamo.PushDevicesStore; las pruebas usan dobles con estado.
type Store interface {
	LoadIdempotency(ctx context.Context, userID, key string, now time.Time) (*IdempotencyRecord, error)
	FindActiveByLabel(ctx context.Context, userID, provider, label string) (*Device, error)
	Register(ctx context.Context, write RegisterWrite) error
	GetDevice(ctx context.Context, userID, deviceID string) (*Device, error)
	Revoke(ctx context.Context, userID, deviceID string, now time.Time) (bool, error)
}

// Service implementa E059 y E060.
type Service struct {
	store  Store
	sealer TokenSealer
	now    func() time.Time
	ttl    time.Duration
}

// NewService construye el servicio. Sin almacén o sin sellador el servicio
// queda inactivo y todas las operaciones responden ErrUnavailable.
func NewService(store Store, sealer TokenSealer) *Service {
	return &Service{store: store, sealer: sealer, now: time.Now, ttl: IdempotencyTTL}
}

// WithClock inyecta un reloj determinista para pruebas.
func (s *Service) WithClock(now func() time.Time) *Service {
	if s != nil && now != nil {
		s.now = now
	}
	return s
}

// WithIdempotencyTTL ajusta la ventana de replay (pruebas y política futura).
func (s *Service) WithIdempotencyTTL(ttl time.Duration) *Service {
	if s != nil && ttl > 0 {
		s.ttl = ttl
	}
	return s
}

// Register ejecuta E059: valida, detecta replay/conflicto, reutiliza la
// instalación activa de la misma etiqueta, sella el token y escribe de forma
// atómica dispositivo y registro de idempotencia.
func (s *Service) Register(ctx context.Context, cmd RegisterCommand) (*RegistrationResult, error) {
	if s == nil || s.store == nil || s.sealer == nil {
		return nil, ErrUnavailable
	}
	userID := strings.TrimSpace(cmd.UserID)
	if userID == "" {
		return nil, ErrUnauthorized
	}
	provider := strings.TrimSpace(cmd.Provider)
	label := cmd.DeviceLabel
	if !appIdentity.ValidIdempotencyKey(cmd.IdempotencyKey) ||
		provider != ProviderFCM ||
		len(cmd.Token) < MinTokenLen ||
		!utf8.ValidString(label) || strings.TrimSpace(label) == "" ||
		utf8.RuneCountInString(label) > MaxDeviceLabelRunes {
		return nil, ErrInvalidRequest
	}

	now := s.now().UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	payloadHash := hashPayload(provider, cmd.Token, label)

	prev, err := s.store.LoadIdempotency(ctx, userID, strings.TrimSpace(cmd.IdempotencyKey), now)
	if err != nil {
		return nil, ErrUnavailable
	}
	if prev != nil {
		if !strings.EqualFold(prev.PayloadHash, payloadHash) {
			return nil, ErrConflict
		}
		return replay(prev), nil
	}

	anterior, err := s.store.FindActiveByLabel(ctx, userID, provider, label)
	if err != nil {
		return nil, ErrUnavailable
	}
	deviceID := ""
	fingerprintAnterior := ""
	if anterior != nil {
		deviceID = anterior.ID
		fingerprintAnterior = anterior.TokenFingerprint
	} else {
		deviceID, err = identity.NewID()
		if err != nil {
			return nil, ErrUnavailable
		}
	}

	aad := userID + "|" + deviceID
	cipher, err := s.sealer.Sellar(cmd.Token, aad)
	if err != nil || len(cipher) == 0 {
		return nil, ErrUnavailable
	}

	dispositivo := Device{
		ID:               deviceID,
		Provider:         provider,
		DeviceLabel:      label,
		RegisteredAt:     now,
		Active:           true,
		TokenCipher:      cipher,
		TokenFingerprint: hashToken(cmd.Token),
	}
	responseJSON, err := json.Marshal(WireDevice{
		ID:           dispositivo.ID,
		Provider:     dispositivo.Provider,
		DeviceLabel:  dispositivo.DeviceLabel,
		RegisteredAt: now.Format(time.RFC3339Nano),
	})
	if err != nil {
		return nil, ErrUnavailable
	}

	err = s.store.Register(ctx, RegisterWrite{
		UserID:              userID,
		IdempotencyKey:      strings.TrimSpace(cmd.IdempotencyKey),
		PayloadHash:         payloadHash,
		Device:              dispositivo,
		PreviousFingerprint: fingerprintAnterior,
		ResponseJSON:        responseJSON,
		Now:                 now,
		TTL:                 s.ttl,
	})
	if errors.Is(err, ErrIdempotencyCondition) {
		// Otra instancia ganó la carrera con esta clave. El registro ya escrito
		// es la única autoridad para decidir entre replay y conflicto.
		ganador, errLectura := s.store.LoadIdempotency(ctx, userID, strings.TrimSpace(cmd.IdempotencyKey), now)
		if errLectura != nil {
			return nil, ErrUnavailable
		}
		if ganador == nil {
			return nil, ErrConflict
		}
		if !strings.EqualFold(ganador.PayloadHash, payloadHash) {
			return nil, ErrConflict
		}
		return replay(ganador), nil
	}
	if err != nil {
		return nil, ErrUnavailable
	}
	return &RegistrationResult{Device: dispositivo, ResponseJSON: responseJSON}, nil
}

// Revoke ejecuta E060: sólo puede revocar el dispositivo de la partición del
// titular. Una baja repetida es 204; un dispositivo ajeno o inexistente es 404
// con el mismo envelope, sin revelar cuál de los dos casos ocurrió.
func (s *Service) Revoke(ctx context.Context, userID, deviceID string) error {
	if s == nil || s.store == nil {
		return ErrUnavailable
	}
	userID = strings.TrimSpace(userID)
	deviceID = strings.TrimSpace(deviceID)
	if userID == "" {
		return ErrUnauthorized
	}
	if deviceID == "" {
		return ErrInvalidRequest
	}

	actual, err := s.store.GetDevice(ctx, userID, deviceID)
	if err != nil {
		return ErrUnavailable
	}
	if actual == nil {
		return ErrNotFound
	}
	if !actual.Active {
		return nil
	}

	now := s.now().UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	ok, err := s.store.Revoke(ctx, userID, deviceID, now)
	if err != nil {
		return ErrUnavailable
	}
	if ok {
		return nil
	}
	// La condición `active = true` falló: otra instancia pudo revocar primero.
	actual, err = s.store.GetDevice(ctx, userID, deviceID)
	if err != nil {
		return ErrUnavailable
	}
	if actual == nil {
		return ErrNotFound
	}
	if !actual.Active {
		return nil
	}
	return ErrConflict
}

func replay(record *IdempotencyRecord) *RegistrationResult {
	return &RegistrationResult{
		Device:       Device{ID: record.DeviceID, Active: true},
		ResponseJSON: append([]byte(nil), record.ResponseJSON...),
		Replayed:     true,
	}
}

// hashPayload es la representación canónica exigida por el contrato:
// SHA256("provider|token|deviceLabel"). Ninguno de los tres valores se persiste
// en claro en el registro de idempotencia.
func hashPayload(provider, token, label string) string {
	suma := sha256.Sum256([]byte(provider + "|" + token + "|" + label))
	return hex.EncodeToString(suma[:])
}

func hashToken(token string) string {
	suma := sha256.Sum256([]byte(token))
	return hex.EncodeToString(suma[:])
}
