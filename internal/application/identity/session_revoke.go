package identity

import (
	"context"
	"errors"
	"strings"
	"time"
)

// Reglas de E009 (`DELETE /api/v1/auth/sessions/{sessionId}`): revocar una sesión concreta del
// propio titular.
//
// El contrato (OpenAPI 3.1, operationId `nexum_e009`) exige `Authorization: Bearer`, el
// parámetro de ruta `sessionId` (uuid) y responde **204 sin cuerpo**. Una sesión ajena o
// inexistente responde **404 homogéneo**: no se distingue "no existe" de "es de otro usuario",
// de modo que la ruta no permite enumerar sesiones ajenas (R-031, AC-A-050-03).
//
// Invariantes de esta capa:
//   - La identidad llega exclusivamente de `sub` del JWT verificado; el `sessionId` de la ruta
//     NUNCA autoriza por sí solo: siempre se compara con el propietario persistido.
//   - La revocación es durable y observable por cualquier instancia: estado de la sesión en
//     DynamoDB, sin caché como autoridad (GOV-02, A-080 §2.6).
//   - Revocar una sesión ya revocada es idempotente (204) y no reescribe el registro.
var (
	// ErrSessionRevokeNotFound cubre sesión inexistente, purgada por TTL, expirada, con otra
	// entidad en la misma clave o perteneciente a otro usuario. Todas se responden con el mismo
	// 404 sin revelar la causa.
	ErrSessionRevokeNotFound = errors.New("sesion no encontrada")

	// ErrSessionRevokeUnauthorized indica que el token no identifica usuario. El middleware ya
	// lo impide; esta guarda es defensa en profundidad (401).
	ErrSessionRevokeUnauthorized = errors.New("revocacion de sesion no autorizada")

	// ErrSessionRevokeConflict indica que la sesión cambió entre la lectura y la escritura y
	// sigue activa tras releer: la operación debe reintentarse (409). Nunca se responde 204 sin
	// revocación confirmada.
	ErrSessionRevokeConflict = errors.New("conflicto al revocar la sesion")

	// ErrSessionRevokeUnavailable indica falta de persistencia o un fallo de servicio (503).
	ErrSessionRevokeUnavailable = errors.New("servicio de revocacion no disponible")
)

// RevokeReasonUserRevokedSession es el motivo durable de una revocación solicitada por el
// titular desde otro dispositivo. Se distingue de `USER_LOGOUT` (E004) y del replay de refresh
// (E003), de modo que la auditoría identifica el origen real.
const RevokeReasonUserRevokedSession = "USER_REVOKED_SESSION"

// sessionRevokeUUIDLengths es la forma 8-4-4-4-12 de un uuid (RFC 4122 en representación
// canónica). El contrato declara `format: uuid` para el parámetro de ruta.
var sessionRevokeUUIDLengths = [5]int{8, 4, 4, 4, 12}

// RevokeSessionRecord es la lectura autoritativa mínima de una sesión antes de revocarla.
// No incluye tokens, jti, IP ni agente de usuario: sólo lo necesario para decidir propiedad,
// estado y compare-and-swap.
type RevokeSessionRecord struct {
	UserID      string
	SessionID   string
	RefreshHash string
	Status      string
	Version     int64
	HasVersion  bool
	ExpiresAt   time.Time
	HasExpiry   bool
}

// SessionRevocation es la escritura condicional que aplica el adaptador. `ExpectedVersion` sólo
// se usa cuando `HasVersion` es cierto.
type SessionRevocation struct {
	UserID          string
	SessionID       string
	RefreshHash     string
	Reason          string
	Now             time.Time
	ExpectedVersion int64
	HasVersion      bool
}

// SessionRevokeStore es el puerto de persistencia de E009. Nunca usa Scan: lee una única clave
// y escribe en una sola transacción condicional.
type SessionRevokeStore interface {
	LoadSessionForRevoke(ctx context.Context, userID, sessionID string) (*RevokeSessionRecord, error)
	RevokeOwnedSession(ctx context.Context, revocation SessionRevocation) error
}

// SessionRevokeService orquesta la revocación con verificación de propiedad.
type SessionRevokeService struct {
	store SessionRevokeStore
	now   func() time.Time
}

// NewSessionRevokeService construye el servicio. Sin almacén todas las llamadas fallan cerrado
// con ErrSessionRevokeUnavailable: nunca se responde 204 sin revocación verificable.
func NewSessionRevokeService(store SessionRevokeStore) *SessionRevokeService {
	return &SessionRevokeService{
		store: store,
		now:   func() time.Time { return time.Now().UTC() },
	}
}

// WithClock permite inyectar un reloj determinista en pruebas.
func (s *SessionRevokeService) WithClock(now func() time.Time) *SessionRevokeService {
	if now != nil {
		s.now = now
	}
	return s
}

// RevokeOwnedSession revoca la sesión indicada si pertenece al usuario autenticado.
//
// Secuencia: validar formato del uuid → leer la sesión con consistencia fuerte → comprobar
// propiedad, tipo de entidad y expiración → aplicar la transición condicional. Si la sesión ya
// está revocada se devuelve nil (204 idempotente) sin reescribir. Si el CAS falla, se relee una
// vez: revocada ⇒ 204; sigue activa ⇒ 409.
func (s *SessionRevokeService) RevokeOwnedSession(ctx context.Context, userID, sessionID string, now time.Time) error {
	if s == nil || s.store == nil {
		return ErrSessionRevokeUnavailable
	}

	userID = strings.TrimSpace(userID)
	if userID == "" {
		return ErrSessionRevokeUnauthorized
	}

	sessionID = strings.TrimSpace(sessionID)
	if !IsUUID(sessionID) {
		return ErrSessionsInvalidRequest
	}

	if now.IsZero() {
		now = s.now()
	}
	now = now.UTC()

	record, err := s.store.LoadSessionForRevoke(ctx, userID, sessionID)
	if err != nil {
		return ErrSessionRevokeUnavailable
	}
	if record == nil {
		return ErrSessionRevokeNotFound
	}
	if strings.TrimSpace(record.UserID) == "" || record.UserID != userID {
		// Sesión ajena: mismo 404 que inexistente, sin revelar que existe.
		return ErrSessionRevokeNotFound
	}
	if record.HasExpiry && !now.Before(record.ExpiresAt.UTC()) {
		// Expirada: E003 tampoco podría rotarla. Se trata como inexistente.
		return ErrSessionRevokeNotFound
	}
	if SessionIsRevoked(record.Status) {
		// Idempotente: ya revocada, no se reescribe ni se actualiza `revokedAt`.
		return nil
	}

	err = s.store.RevokeOwnedSession(ctx, SessionRevocation{
		UserID:          userID,
		SessionID:       sessionID,
		RefreshHash:     record.RefreshHash,
		Reason:          RevokeReasonUserRevokedSession,
		Now:             now,
		ExpectedVersion: record.Version,
		HasVersion:      record.HasVersion,
	})
	if err == nil {
		return nil
	}
	if !errors.Is(err, ErrSessionRevokeConflict) {
		return ErrSessionRevokeUnavailable
	}

	// CAS perdido: otra petición (u otro nodo) modificó la sesión. Se relee UNA vez para
	// distinguir "ya revocada por el ganador" (204) de "sigue activa con otra versión" (409).
	releida, errRelectura := s.store.LoadSessionForRevoke(ctx, userID, sessionID)
	if errRelectura != nil {
		return ErrSessionRevokeUnavailable
	}
	if releida == nil || strings.TrimSpace(releida.UserID) == "" || releida.UserID != userID {
		return ErrSessionRevokeNotFound
	}
	if SessionIsRevoked(releida.Status) {
		return nil
	}
	return ErrSessionRevokeConflict
}

// IsUUID valida la representación canónica 8-4-4-4-12 hexadecimal del parámetro `sessionId`.
// No acepta llaves, mayúsculas fuera de rango ni separadores distintos.
func IsUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	posiciones := 0
	for indice, longitud := range sessionRevokeUUIDLengths {
		if indice > 0 {
			if value[posiciones] != '-' {
				return false
			}
			posiciones++
		}
		for i := 0; i < longitud; i++ {
			caracter := value[posiciones+i]
			switch {
			case caracter >= '0' && caracter <= '9':
			case caracter >= 'a' && caracter <= 'f':
			case caracter >= 'A' && caracter <= 'F':
			default:
				return false
			}
		}
		posiciones += longitud
	}
	return posiciones == len(value)
}
