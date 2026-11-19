// Package account contiene los casos de uso de la cuenta del titular que no pertenecen a
// identidad ni a perfil: la solicitud de eliminación con purga durable (E014).
//
// No importa el SDK de AWS, ni Redis, ni HTTP. Toda la persistencia se expresa como puertos
// que implementa `internal/adapters/dynamo`; el worker es código de aplicación que se
// ejecuta dentro del binario ya desplegado.
package account

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	domainDeletion "github.com/nexumpr/Nexum-Backend/internal/domain/deletion"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
)

// Errores del caso de uso de E014. Cada uno tiene un código HTTP fijo en el transporte:
//   - ErrDeletionUnauthorized          -> 401 (cuenta inexistente, no ACTIVE o sin titularidad)
//   - ErrDeletionPasswordIncorrect     -> 422 (criterio de E007: nunca 401 por contraseña)
//   - ErrDeletionConfirmation          -> 422 (confirmación distinta de DELETE_MY_ACCOUNT)
//   - ErrDeletionConflict              -> 409 (la transacción condicional perdió una carrera)
//   - ErrDeletionUnavailable           -> 503 (falta persistencia o fallo de DynamoDB)
var (
	ErrDeletionUnauthorized      = errors.New("cuenta no autorizada para solicitar la eliminacion")
	ErrDeletionProfileNotFound   = errors.New("perfil de cuenta inexistente")
	ErrDeletionPasswordIncorrect = errors.New("contrasena actual incorrecta")
	ErrDeletionConfirmation      = errors.New("confirmacion de eliminacion invalida")
	ErrDeletionConflict          = errors.New("conflicto al registrar la solicitud de eliminacion")
	ErrDeletionUnavailable       = errors.New("solicitud de eliminacion no disponible")
)

// ConfirmationPhrase es el único valor admitido por `DeleteAccountRequest.confirmation`.
const ConfirmationPhrase = "DELETE_MY_ACCOUNT"

const (
	// deletionPasswordMaxRunes es defensa en profundidad contra cuerpos abusivos: el
	// contrato sólo declara `minLength: 1`. El tope queda muy por encima de cualquier
	// contraseña real y evita derivar Argon2id sobre entradas desproporcionadas.
	deletionPasswordMaxRunes = 1024

	// requestIDBytes es la entropía del identificador de la solicitud (UUID v4).
	requestIDBytes = 16
)

// FieldViolation describe un error atribuible a un campo del contrato. El transporte lo
// traduce a 422 sin reflejar el valor recibido ni confirmar si la cuenta existe.
type FieldViolation struct {
	Field  string
	Reason string
}

func (v *FieldViolation) Error() string {
	return fmt.Sprintf("campo %q invalido: %s", v.Field, v.Reason)
}

// DeleteAccountCommand es la entrada de aplicación de E014. `UserID` proviene
// EXCLUSIVAMENTE del claim `sub` ya verificado por el middleware: el cliente no elige la
// cuenta que elimina (R-031).
type DeleteAccountCommand struct {
	UserID          string
	Confirmation    string
	CurrentPassword string
}

// DeletionProfile es la lectura autoritativa de `USER#<id>/PROFILE` que necesita E014.
type DeletionProfile struct {
	UserID       string
	Status       string
	PasswordHash string
	// Email se usa únicamente para calcular el hash de la reserva de unicidad; el valor en
	// claro no se persiste en el control ni se registra.
	Email string
	// Version es la versión optimista del perfil. HasVersion es false en perfiles
	// anteriores a la introducción del atributo (E002 primitivo).
	Version    int64
	HasVersion bool
	// SessionEpoch es la época de revocación vigente (A6). Ausente equivale a 0.
	SessionEpoch int64
}

// DeletionApplication describe la transacción atómica de 3 ítems del diseño §2.
type DeletionApplication struct {
	UserID    string
	RequestID string
	EmailHash string
	// ExpectedVersion y ExpectedSessionEpoch son los valores leídos; la condición del
	// perfil exige que sigan siendo los vigentes (o su ausencia) además de `status=ACTIVE`.
	ExpectedVersion      int64
	HasVersion           bool
	ExpectedSessionEpoch int64
	// Now fija `deletedAt`, `dueAt` y `requestedAt` con el reloj del servidor.
	Now time.Time
}

// DeletionStore es el puerto de persistencia del caso de uso. Se implementa sobre la tabla
// única existente; nunca usa Scan ni memoria como autoridad.
type DeletionStore interface {
	// LoadDeletionProfile lee `USER#<id>/PROFILE` con consistencia fuerte. Devuelve
	// (nil, nil) cuando el perfil no existe.
	LoadDeletionProfile(ctx context.Context, userID string) (*DeletionProfile, error)
	// ScheduleDeletion ejecuta UNA TransactWriteItems de 3 ítems: perfil a
	// DELETION_PENDING con época y versión incrementadas, control del trabajo y entrada de
	// cola. Cualquier fallo de condición cancela la transacción completa.
	ScheduleDeletion(ctx context.Context, app DeletionApplication) error
}

// DeletionReceipt es la respuesta de aplicación de E014 (contrato `DeletionReceipt`).
type DeletionReceipt struct {
	RequestID         string
	Status            string
	RequestedAt       time.Time
	SessionsRevoked   bool
	DeletionRequestID string
}

// DeletionService orquesta la solicitud de eliminación.
type DeletionService struct {
	store  DeletionStore
	hasher ports.PasswordHasher
	now    func() time.Time
	newID  func() (string, error)
}

// NewDeletionService construye el servicio. Si falta una dependencia crítica, Request falla
// cerrado con ErrDeletionUnavailable: nunca se simula una eliminación.
func NewDeletionService(store DeletionStore, hasher ports.PasswordHasher) *DeletionService {
	return &DeletionService{
		store:  store,
		hasher: hasher,
		now:    func() time.Time { return time.Now().UTC() },
		newID:  newRequestID,
	}
}

// WithClock permite inyectar un reloj determinista en pruebas.
func (s *DeletionService) WithClock(now func() time.Time) *DeletionService {
	if now != nil {
		s.now = now
	}
	return s
}

// WithRequestIDGenerator permite inyectar un generador determinista en pruebas.
func (s *DeletionService) WithRequestIDGenerator(gen func() (string, error)) *DeletionService {
	if gen != nil {
		s.newID = gen
	}
	return s
}

// Disponible indica si el endpoint puede operar de verdad.
func (s *DeletionService) Disponible() bool {
	return s != nil && s.store != nil && s.hasher != nil
}

// Request aplica la secuencia del diseño §2:
//
//  1. validar el cuerpo sin normalizar la contraseña;
//  2. exigir identidad derivada del token;
//  3. leer el perfil con consistencia fuerte y exigir `ACTIVE`;
//  4. verificar la contraseña con Argon2id (incorrecta -> 422, nunca 401);
//  5. registrar la transacción de 3 ítems y devolver el recibo de aceptación.
//
// La transacción incrementa `sessionEpoch` y cambia el estado del perfil a
// `DELETION_PENDING` en la misma escritura atómica: a partir de ese instante
// `AuthorizeSession` rechaza cualquier access token (status ≠ ACTIVE y época desalineada) y
// E003/E004 dejan de operar en ambos nodos. Por eso `SessionsRevoked` es verdadero.
func (s *DeletionService) Request(ctx context.Context, in DeleteAccountCommand) (DeletionReceipt, error) {
	if !s.Disponible() {
		return DeletionReceipt{}, ErrDeletionUnavailable
	}

	userID := strings.TrimSpace(in.UserID)
	if userID == "" {
		return DeletionReceipt{}, ErrDeletionUnauthorized
	}
	if !strings.EqualFold(strings.TrimSpace(in.Confirmation), ConfirmationPhrase) {
		return DeletionReceipt{}, &FieldViolation{Field: "confirmation", Reason: "invalid_value"}
	}
	if strings.TrimSpace(in.CurrentPassword) == "" {
		return DeletionReceipt{}, &FieldViolation{Field: "currentPassword", Reason: "required"}
	}
	if utf8.RuneCountInString(in.CurrentPassword) > deletionPasswordMaxRunes {
		return DeletionReceipt{}, &FieldViolation{Field: "currentPassword", Reason: "too_long"}
	}

	profile, err := s.store.LoadDeletionProfile(ctx, userID)
	if err != nil {
		return DeletionReceipt{}, ErrDeletionUnavailable
	}
	if profile == nil {
		// El perfil ya no existe (purga completada o cuenta nunca creada). El contrato
		// admite 401/404 para este caso; se usa 404 porque la identidad autenticada es
		// válida y lo único que falta es el recurso propio.
		return DeletionReceipt{}, ErrDeletionProfileNotFound
	}
	if profile.UserID != "" && profile.UserID != userID {
		// Defensa en profundidad: el ítem leído debe pertenecer al sujeto del token.
		return DeletionReceipt{}, ErrDeletionUnauthorized
	}
	if !strings.EqualFold(strings.TrimSpace(profile.Status), ProfileStatusActive) {
		// Una cuenta ya en DELETION_PENDING, DELETED o deshabilitada no vuelve a pedir
		// eliminación: la segunda llamada autenticada queda en 401 y no crea otro trabajo.
		return DeletionReceipt{}, ErrDeletionUnauthorized
	}
	if strings.TrimSpace(profile.PasswordHash) == "" {
		return DeletionReceipt{}, ErrDeletionUnauthorized
	}

	verificada, err := s.hasher.VerifyPassword(in.CurrentPassword, profile.PasswordHash)
	if err != nil {
		// El hasher Argon2id real devuelve `ports.ErrInvalidPassword` cuando la contraseña no
		// coincide. Eso es una violación de regla (422), no una caída de dependencia.
		if errors.Is(err, ports.ErrInvalidPassword) {
			return DeletionReceipt{}, ErrDeletionPasswordIncorrect
		}
		return DeletionReceipt{}, ErrDeletionUnavailable
	}
	if !verificada {
		return DeletionReceipt{}, ErrDeletionPasswordIncorrect
	}

	requestID, err := s.newID()
	if err != nil || strings.TrimSpace(requestID) == "" {
		return DeletionReceipt{}, ErrDeletionUnavailable
	}
	now := s.now().UTC()

	err = s.store.ScheduleDeletion(ctx, DeletionApplication{
		UserID:               userID,
		RequestID:            requestID,
		EmailHash:            domainDeletion.EmailHash(profile.Email),
		ExpectedVersion:      profile.Version,
		HasVersion:           profile.HasVersion,
		ExpectedSessionEpoch: profile.SessionEpoch,
		Now:                  now,
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrDeletionConflict):
			return DeletionReceipt{}, ErrDeletionConflict
		case errors.Is(err, ErrDeletionUnauthorized):
			return DeletionReceipt{}, ErrDeletionUnauthorized
		default:
			return DeletionReceipt{}, ErrDeletionUnavailable
		}
	}

	return DeletionReceipt{
		RequestID:         requestID,
		Status:            "scheduled",
		RequestedAt:       now,
		SessionsRevoked:   true,
		DeletionRequestID: requestID,
	}, nil
}

// ProfileStatusActive replica el valor canónico del perfil sin acoplar este paquete al de
// identidad: el contrato físico es el literal `ACTIVE` escrito por E001/E002. Se exporta
// porque el adaptador DynamoDB lo usa en las condiciones de la transacción.
const ProfileStatusActive = "ACTIVE"

// newRequestID genera un UUID v4 sin dependencias externas.
func newRequestID() (string, error) {
	var b [requestIDBytes]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
