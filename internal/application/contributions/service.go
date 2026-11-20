package contributions

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
	domainIdentity "github.com/nexumpr/Nexum-Backend/internal/domain/identity"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

const createAttempts = 2

// Service orquesta los casos de uso de aportaciones. Es inmutable una vez construido: los
// métodos `With*` devuelven una copia segura de compartir entre peticiones.
type Service struct {
	store     Store
	now       func() time.Time
	newID     func() (string, error)
	cursorKey []byte
	cursorTTL time.Duration
}

// NewService construye el servicio. Sin almacén todas las operaciones fallan cerrado.
func NewService(store Store) *Service {
	return &Service{
		store:     store,
		now:       func() time.Time { return time.Now().UTC() },
		newID:     domainIdentity.NewID,
		cursorTTL: httputil.DefaultCursorTTL,
	}
}

// WithCursorKey fija la clave HMAC de los cursores de E045. Sin clave el listado falla 503.
func (s *Service) WithCursorKey(key []byte) *Service {
	if len(key) == 0 {
		return s
	}
	copia := *s
	copia.cursorKey = append([]byte(nil), key...)
	return &copia
}

// WithClock inyecta un reloj determinista en pruebas.
func (s *Service) WithClock(now func() time.Time) *Service {
	if now == nil {
		return s
	}
	copia := *s
	copia.now = now
	return &copia
}

// WithIDGenerator inyecta un generador determinista en pruebas.
func (s *Service) WithIDGenerator(gen func() (string, error)) *Service {
	if gen == nil {
		return s
	}
	copia := *s
	copia.newID = gen
	return &copia
}

// WithCursorTTL permite acortar la vigencia del cursor en pruebas.
func (s *Service) WithCursorTTL(ttl time.Duration) *Service {
	if ttl == 0 {
		return s
	}
	copia := *s
	copia.cursorTTL = ttl
	return &copia
}

// Preview ejecuta E043. No escribe nada.
func (s *Service) Preview(ctx context.Context, cmd PreviewCommand) (*PreviewResult, error) {
	if s == nil || s.store == nil {
		return nil, ErrUnavailable
	}
	userID := strings.TrimSpace(cmd.UserID)
	if userID == "" {
		return nil, ErrUnauthorized
	}
	amount, err := ParseAmount(cmd.Amount)
	if err != nil {
		return nil, err
	}
	if amount <= 0 {
		return nil, violacion("amount", "out_of_range")
	}
	currency, err := NormalizeCurrency(cmd.Currency)
	if err != nil {
		return nil, err
	}
	allocations, suma, err := NormalizeAllocations(cmd.Allocations, amount, true)
	if err != nil {
		return nil, err
	}

	goals, err := s.loadGoals(ctx, userID, allocations)
	if err != nil {
		return nil, err
	}
	remaining := amount - suma
	if remaining < 0 {
		remaining = 0
	}
	canCommit := true
	for _, a := range allocations {
		goal := goals[a.GoalID]
		if !goal.Active() {
			canCommit = false
			continue
		}
		if a.AmountCents > goal.RemainingCapacityCents() {
			canCommit = false
			remaining += a.AmountCents - goal.RemainingCapacityCents()
		}
	}
	return &PreviewResult{
		AmountCents:    amount,
		Currency:       currency,
		Allocations:    allocations,
		RemainingCents: remaining,
		CanCommit:      canCommit && remaining == 0,
	}, nil
}

// Create ejecuta E044.
func (s *Service) Create(ctx context.Context, cmd CreateCommand) (*StoredResponse, error) {
	if s == nil || s.store == nil || s.newID == nil {
		return nil, ErrUnavailable
	}
	userID := strings.TrimSpace(cmd.UserID)
	if userID == "" {
		return nil, ErrUnauthorized
	}
	key := strings.TrimSpace(cmd.IdempotencyKey)
	if key == "" {
		return nil, fmt.Errorf("%w: Idempotency-Key requerida", ErrInvalidRequest)
	}
	if len(key) < idempotencyMinBytes || len(key) > idempotencyMaxBytes {
		return nil, fmt.Errorf("%w: Idempotency-Key fuera de rango", ErrInvalidRequest)
	}
	amount, err := ParseAmount(cmd.Amount)
	if err != nil {
		return nil, err
	}
	if amount <= 0 {
		return nil, violacion("amount", "out_of_range")
	}
	currency, err := NormalizeCurrency(cmd.Currency)
	if err != nil {
		return nil, err
	}
	allocations, _, err := NormalizeAllocations(cmd.Allocations, amount, true)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	occurred, err := NormalizeOccurredAt(cmd.OccurredAt, now)
	if err != nil {
		return nil, err
	}
	payloadHash := PayloadHash(amount, currency, allocations, appMovements.FormatOccurredAt(occurred))

	record, err := s.store.LoadIdempotency(ctx, userID, OperationCreate, key)
	if err != nil {
		return nil, normalizeStoreError(err)
	}
	if record != nil && recordReusable(record, now) {
		if record.PayloadHash != payloadHash {
			return nil, ErrConflict
		}
		if record.HTTPStatus < 100 || record.HTTPStatus > 599 || len(record.ResponseBody) == 0 {
			return nil, ErrInternal
		}
		return &StoredResponse{Status: record.HTTPStatus, Body: record.ResponseBody, Location: record.Location, Replayed: true}, nil
	}

	goals, err := s.loadGoals(ctx, userID, allocations)
	if err != nil {
		return nil, err
	}

	for intento := 0; intento < createAttempts; intento++ {
		contributionID, err := s.newID()
		if err != nil || strings.TrimSpace(contributionID) == "" {
			return nil, ErrInternal
		}
		movementID, err := s.newID()
		if err != nil || strings.TrimSpace(movementID) == "" {
			return nil, ErrInternal
		}
		contribution := Contribution{
			ID:          contributionID,
			UserID:      userID,
			AmountCents: amount,
			Currency:    currency,
			Allocations: allocations,
			OccurredAt:  occurred,
			Status:      StatusActive,
			Version:     1,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		body, err := contribution.Body()
		if err != nil {
			return nil, ErrInternal
		}
		movement := appMovements.Movement{
			ID:             movementID,
			Type:           appMovements.TypeSaving,
			AmountCents:    amount,
			Currency:       currency,
			CategoryID:     goalCategory,
			Description:    movementDescription,
			OccurredAt:     occurred,
			Version:        1,
			ContributionID: contributionID,
			CreatedAt:      now,
		}
		result, err := s.store.CreateContribution(ctx, CreateWrite{
			UserID:         userID,
			Contribution:   contribution,
			Movement:       movement,
			Goals:          goals,
			IdempotencyKey: key,
			Operation:      OperationCreate,
			PayloadHash:    payloadHash,
			ResponseBody:   body,
			Location:       "/api/v1/goal-contributions/" + contributionID,
			Now:            now,
			ExpiresAt:      now.Add(IdempotencyTTL),
		})
		if errors.Is(err, ErrIDCollision) {
			continue
		}
		if err != nil {
			return nil, normalizeStoreError(err)
		}
		return result, nil
	}
	return nil, ErrInternal
}

// Get ejecuta E046.
func (s *Service) Get(ctx context.Context, userID, contributionID string) (*Contribution, error) {
	if s == nil || s.store == nil {
		return nil, ErrUnavailable
	}
	titular := strings.TrimSpace(userID)
	if titular == "" {
		return nil, ErrUnauthorized
	}
	id := strings.TrimSpace(contributionID)
	if !ValidContributionID(id) {
		return nil, fmt.Errorf("%w: contributionId", ErrInvalidRequest)
	}
	c, err := s.store.LoadContribution(ctx, titular, id)
	if err != nil {
		return nil, normalizeStoreError(err)
	}
	if c == nil {
		return nil, ErrNotFound
	}
	return c, nil
}

// List ejecuta E045.
func (s *Service) List(ctx context.Context, query ListQuery) (*Page, error) {
	if s == nil || s.store == nil {
		return nil, ErrUnavailable
	}
	if len(s.cursorKey) == 0 {
		return nil, ErrUnavailable
	}
	if strings.TrimSpace(query.UserID) == "" {
		return nil, ErrUnauthorized
	}
	normalizada, err := NormalizeListQuery(query)
	if err != nil {
		return nil, err
	}
	firma := FilterSignature(normalizada)
	startSK := ""
	if normalizada.Cursor != "" {
		payload, errCursor := httputil.ValidateCursor(s.cursorKey, normalizada.Cursor, normalizada.UserID)
		if errCursor != nil {
			return nil, fmt.Errorf("%w: cursor", ErrInvalidRequest)
		}
		if payload.SortBy != firma || strings.TrimSpace(payload.LastID) == "" {
			return nil, fmt.Errorf("%w: cursor", ErrInvalidRequest)
		}
		startSK = payload.LastID
	}
	items, nextSK, err := s.store.ListContributions(ctx, normalizada, startSK, normalizada.Limit)
	if err != nil {
		return nil, normalizeStoreError(err)
	}
	page := &Page{Items: items}
	if strings.TrimSpace(nextSK) != "" {
		cursor, errCursor := httputil.GenerateCursor(s.cursorKey, normalizada.UserID, firma, nextSK, s.now().UTC(), s.cursorTTL)
		if errCursor != nil {
			return nil, ErrInternal
		}
		page.NextCursor = cursor
	}
	return page, nil
}

// Reverse ejecuta E047.
func (s *Service) Reverse(ctx context.Context, cmd ReverseCommand) (*StoredResponse, error) {
	if s == nil || s.store == nil {
		return nil, ErrUnavailable
	}
	userID := strings.TrimSpace(cmd.UserID)
	if userID == "" {
		return nil, ErrUnauthorized
	}
	id := strings.TrimSpace(cmd.ContributionID)
	if !ValidContributionID(id) {
		return nil, fmt.Errorf("%w: contributionId", ErrInvalidRequest)
	}
	key := strings.TrimSpace(cmd.IdempotencyKey)
	if key == "" {
		return nil, fmt.Errorf("%w: Idempotency-Key requerida", ErrInvalidRequest)
	}
	if len(key) < idempotencyMinBytes || len(key) > idempotencyMaxBytes {
		return nil, fmt.Errorf("%w: Idempotency-Key fuera de rango", ErrInvalidRequest)
	}
	reason, err := NormalizeReason(cmd.Reason)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	payloadHash := ReversePayloadHash(id, reason)
	record, err := s.store.LoadIdempotency(ctx, userID, OperationReverse, key)
	if err != nil {
		return nil, normalizeStoreError(err)
	}
	if record != nil && recordReusable(record, now) {
		if record.PayloadHash != payloadHash {
			return nil, ErrConflict
		}
		if record.HTTPStatus < 100 || record.HTTPStatus > 599 || len(record.ResponseBody) == 0 {
			return nil, ErrInternal
		}
		return &StoredResponse{Status: record.HTTPStatus, Body: record.ResponseBody, Location: record.Location, Replayed: true}, nil
	}

	previous, err := s.store.LoadContribution(ctx, userID, id)
	if err != nil {
		return nil, normalizeStoreError(err)
	}
	if previous == nil {
		return nil, ErrNotFound
	}
	if previous.Status != StatusActive {
		return nil, ErrAlreadyReversed
	}
	next := *previous
	next.Status = StatusReversed
	next.Version = previous.Version + 1
	next.ReversedAt = now
	next.ReverseReason = reason
	next.UpdatedAt = now
	body, err := next.Body()
	if err != nil {
		return nil, ErrInternal
	}
	result, err := s.store.ReverseContribution(ctx, ReverseWrite{
		UserID:         userID,
		Previous:       *previous,
		Next:           next,
		IdempotencyKey: key,
		Operation:      OperationReverse,
		PayloadHash:    payloadHash,
		ResponseBody:   body,
		Location:       "/api/v1/goal-contributions/" + id,
		Now:            now,
		ExpiresAt:      now.Add(IdempotencyTTL),
	})
	if err != nil {
		return nil, normalizeStoreError(err)
	}
	return result, nil
}

func (s *Service) loadGoals(ctx context.Context, userID string, allocations []Allocation) (map[string]GoalSnapshot, error) {
	goals := make(map[string]GoalSnapshot, len(allocations))
	for _, a := range allocations {
		if _, ya := goals[a.GoalID]; ya {
			continue
		}
		goal, err := s.store.LoadGoal(ctx, userID, a.GoalID)
		if err != nil {
			return nil, normalizeStoreError(err)
		}
		if goal == nil {
			return nil, ErrGoalNotFound
		}
		if !goal.Active() {
			return nil, ErrGoalInactive
		}
		goals[a.GoalID] = *goal
	}
	return goals, nil
}

func recordReusable(record *IdempotencyRecord, now time.Time) bool {
	if record == nil || record.ExpiresAt.IsZero() {
		return false
	}
	return now.Before(record.ExpiresAt)
}

func normalizeStoreError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrUnauthorized),
		errors.Is(err, ErrInvalidRequest),
		errors.Is(err, ErrNotFound),
		errors.Is(err, ErrGoalNotFound),
		errors.Is(err, ErrConflict),
		errors.Is(err, ErrAlreadyReversed),
		errors.Is(err, ErrGoalInactive),
		errors.Is(err, ErrCapacityExceeded),
		errors.Is(err, ErrUnavailable),
		errors.Is(err, ErrInternal),
		errors.Is(err, ErrIDCollision):
		return err
	default:
		return ErrUnavailable
	}
}
