package financial

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

var (
	// ErrIdempotencyConflict se retorna cuando la misma Idempotency-Key se utiliza con una carga útil diferente (HTTP 409 Conflict, A-070 §2.7).
	ErrIdempotencyConflict = errors.New("conflicto de idempotencia: la clave ya fue utilizada con una carga util diferente (409 Conflict)")

	// ErrOperationInProgress se retorna cuando una solicitud con la misma clave ya está siendo procesada en paralelo.
	ErrOperationInProgress = errors.New("operacion en curso: solicitud concurrente en proceso")
)

// IdempotentRecord almacena la traza y resultado inmutable de una operación financiera protegida (A-070 §2.7).
type IdempotentRecord struct {
	Key            string
	UserID         string
	Operation      string
	PayloadHash    string
	ResponseStatus int
	ResponseBody   string
	CreatedAt      time.Time
	ExpiresAt      time.Time
	IsCompleted    bool
}

// IdempotencyStore define el contrato para el almacenamiento de claves de idempotencia.
type IdempotencyStore interface {
	LockOrGet(ctx context.Context, key, userID, operation, payloadHash string, ttl time.Duration) (*IdempotentRecord, error)
	SaveResponse(ctx context.Context, key, userID, operation string, status int, responseBody string) error
}

// MemoryIdempotencyStore implementación en memoria para pruebas concurrentes locales.
type MemoryIdempotencyStore struct {
	mu      sync.Mutex
	records map[string]IdempotentRecord
}

// NewMemoryIdempotencyStore inicializa el almacén de idempotencia.
func NewMemoryIdempotencyStore() *MemoryIdempotencyStore {
	return &MemoryIdempotencyStore{
		records: make(map[string]IdempotentRecord),
	}
}

func recordKey(userID, operation, key string) string {
	return userID + "::" + operation + "::" + key
}

// HashPayload calcula el hash SHA-256 canónico del cuerpo de la petición.
func HashPayload(payload []byte) string {
	h := sha256.Sum256(payload)
	return hex.EncodeToString(h[:])
}

// LockOrGet intenta registrar o recuperar una clave de idempotencia de forma atómica.
func (m *MemoryIdempotencyStore) LockOrGet(ctx context.Context, key, userID, operation, payloadHash string, ttl time.Duration) (*IdempotentRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	k := recordKey(userID, operation, key)
	existing, exists := m.records[k]

	now := time.Now().UTC()
	if exists {
		// Comprobar expiración
		if now.After(existing.ExpiresAt) {
			delete(m.records, k)
		} else {
			// Clave existente activa: validar hash del payload
			if existing.PayloadHash != payloadHash {
				return nil, ErrIdempotencyConflict
			}
			if !existing.IsCompleted {
				return nil, ErrOperationInProgress
			}
			return &existing, nil
		}
	}

	// Registrar nueva clave en estado bloqueado/en curso
	newRec := IdempotentRecord{
		Key:            key,
		UserID:         userID,
		Operation:      operation,
		PayloadHash:    payloadHash,
		CreatedAt:      now,
		ExpiresAt:      now.Add(ttl),
		IsCompleted:    false,
		ResponseStatus: 0,
		ResponseBody:   "",
	}
	m.records[k] = newRec
	return nil, nil // Nil record indica que la operación puede proceder
}

// SaveResponse almacena de forma inmutable la respuesta generada por la operación.
func (m *MemoryIdempotencyStore) SaveResponse(ctx context.Context, key, userID, operation string, status int, responseBody string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	k := recordKey(userID, operation, key)
	rec, exists := m.records[k]
	if !exists {
		return errors.New("registro de idempotencia no encontrado para guardar respuesta")
	}

	rec.ResponseStatus = status
	rec.ResponseBody = responseBody
	rec.IsCompleted = true
	m.records[k] = rec
	return nil
}
