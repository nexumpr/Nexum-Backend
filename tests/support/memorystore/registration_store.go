// Package memorystore contiene dobles de prueba. NO se compone en cmd/server ni
// sustituye a DynamoDB en producción; solo permite probar la lógica atómica y la
// concurrencia sin depender de AWS.
package memorystore

import (
	"context"
	"strings"
	"sync"
	"time"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
)

type idempotencyRecord struct {
	payloadHash      string
	response         appIdentity.StoredResponse
	passwordVerifier string
	expiresAt        time.Time
}

// RegistrationStore es un doble concurrente de appIdentity.RegisterStore.
type RegistrationStore struct {
	mu          sync.Mutex
	idempotency map[string]idempotencyRecord
	emails      map[string]string
	records     map[string]appIdentity.RegistrationRecord
	// expenses acumula los compromisos persistidos por usuario para poder comprobar
	// que un replay no los duplica (no existe en DynamoDB: alli son items RECURRING#).
	expenses    map[string]int
	unavailable bool
	now         func() time.Time
}

// NewRegistrationStore crea el doble vacío.
func NewRegistrationStore() *RegistrationStore {
	return &RegistrationStore{
		idempotency: make(map[string]idempotencyRecord),
		emails:      make(map[string]string),
		records:     make(map[string]appIdentity.RegistrationRecord),
		expenses:    make(map[string]int),
	}
}

// SetUnavailable simula la caída de DynamoDB.
func (s *RegistrationStore) SetUnavailable(unavailable bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unavailable = unavailable
}

// SetClock inyecta un reloj determinista. Debe llamarse antes de las pruebas concurrentes.
func (s *RegistrationStore) SetClock(now func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.now = now
}

// CreateOrReplay implementa la semántica atómica de E001 para pruebas.
func (s *RegistrationStore) CreateOrReplay(
	_ context.Context,
	idempotencyKey, operation, payloadHash string,
	record appIdentity.RegistrationRecord,
	response appIdentity.StoredResponse,
	ttl time.Duration,
) (*appIdentity.StoredResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.unavailable {
		return nil, appIdentity.ErrStoreUnavailable
	}
	if ttl <= 0 {
		ttl = appIdentity.RegisterIdempotencyTTL
	}
	now := time.Now().UTC()
	if s.now != nil {
		now = s.now().UTC()
	}

	key := operation + ":" + idempotencyKey
	if existing, ok := s.idempotency[key]; ok && now.Before(existing.expiresAt) {
		if existing.payloadHash != payloadHash {
			return nil, appIdentity.ErrIdempotencyConflict
		}
		copied := existing.response
		copied.Body = append([]byte(nil), existing.response.Body...)
		copied.Replayed = true
		copied.PasswordVerifier = existing.passwordVerifier
		return &copied, nil
	}

	emailKey := strings.ToLower(strings.TrimSpace(record.Email))
	if _, exists := s.emails[emailKey]; exists {
		return nil, appIdentity.ErrEmailAlreadyRegistered
	}
	if _, exists := s.records[record.UserID]; exists {
		return nil, appIdentity.ErrRegistrationInternal
	}

	s.emails[emailKey] = record.UserID
	s.records[record.UserID] = record
	s.expenses[record.UserID] = len(record.RecurringExpenses)
	s.idempotency[key] = idempotencyRecord{
		payloadHash:      payloadHash,
		response:         response,
		passwordVerifier: response.PasswordVerifier,
		expiresAt:        now.Add(ttl),
	}

	return nil, nil
}

// Registration devuelve la fila persistida por userId (solo para aserciones de prueba).
func (s *RegistrationStore) Registration(userID string) (appIdentity.RegistrationRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[userID]
	return record, ok
}

// ExpensesCount devuelve cuantos compromisos fijos tiene persistidos el usuario.
func (s *RegistrationStore) ExpensesCount(userID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.expenses[userID]
}

// EmailReserved indica si el email ya tiene reserva (solo para aserciones).
func (s *RegistrationStore) EmailReserved(email string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.emails[strings.ToLower(strings.TrimSpace(email))]
	return ok
}

// RecordsCount devuelve el número de registros persistidos.
func (s *RegistrationStore) RecordsCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.records)
}
