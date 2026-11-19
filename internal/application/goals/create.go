package goals

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	domainGoal "github.com/nexumpr/Nexum-Backend/internal/domain/goal"
	domainIdentity "github.com/nexumpr/Nexum-Backend/internal/domain/identity"
)

// defaultNewID genera el identificador de la meta con el mismo generador UUID v4 del alta de
// usuarios: sin dependencias externas y con `crypto/rand`.
func defaultNewID() (string, error) {
	return domainIdentity.NewID()
}

// CreateCommand es la entrada ya convertida por el transporte: importes en céntimos y fecha en
// formato `YYYY-MM-DD`. La aplicación vuelve a validar todo (defensa en profundidad).
type CreateCommand struct {
	UserID            string
	IdempotencyKey    string
	Name              string
	TargetAmountCents int64
	InitialSavedCents int64
	Currency          string
	TargetDate        string
}

// Create ejecuta E039 (`POST /api/v1/goals`).
//
// Secuencia: validar y normalizar → calcular el hash del payload → lectura consistente del
// registro de idempotencia (replay sin escribir si ya está completado) → generar el
// identificador → construir y validar el agregado → serializar la respuesta → transacción
// única con la meta y el registro.
//
// El replay devuelve el mismo código y el mismo cuerpo que la primera respuesta; el
// transporte añade `X-Idempotent-Replay: true`. Una clave reutilizada con otro payload es un
// 409 y no crea nada.
func (s *Service) Create(ctx context.Context, cmd CreateCommand) (*StoredResponse, error) {
	if s == nil || s.store == nil || s.newID == nil || s.now == nil {
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
	name, errNombre := domainGoal.NormalizeName(cmd.Name)
	if errNombre != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRequest, errNombre)
	}
	if strings.TrimSpace(cmd.Currency) != domainGoal.CurrencyMXN {
		return nil, fmt.Errorf("%w: currency debe ser MXN", ErrInvalidRequest)
	}
	targetDate, errFecha := time.Parse(domainGoal.DateLayout, strings.TrimSpace(cmd.TargetDate))
	if errFecha != nil {
		return nil, fmt.Errorf("%w: targetDate invalida", ErrInvalidRequest)
	}
	if errMonto := domainGoal.ValidateTargetAmountCents(cmd.TargetAmountCents); errMonto != nil {
		if errors.Is(errMonto, domainGoal.ErrTargetNotPositive) {
			return nil, ErrTargetNotPositive
		}
		return nil, fmt.Errorf("%w: %v", ErrInvalidRequest, errMonto)
	}
	opening := cmd.InitialSavedCents
	if opening < 0 {
		return nil, fmt.Errorf("%w: initialSavedAmount negativo", ErrInvalidRequest)
	}
	if errApertura := domainGoal.ValidateOpeningCents(opening, cmd.TargetAmountCents); errApertura != nil {
		return nil, ErrOpeningExceedsTarget
	}

	payloadHash, errHash := hashCreatePayload(
		name,
		cmd.TargetAmountCents,
		opening,
		domainGoal.CurrencyMXN,
		targetDate.Format(domainGoal.DateLayout),
	)
	if errHash != nil {
		return nil, errHash
	}

	now := s.now().UTC()

	// Lectura consistente previa: un replay no debe ni generar un identificador nuevo ni
	// intentar la transacción. Si el registro venció lógicamente se ignora y se escribe de
	// nuevo; el `expiresAt < :now` de la condición permite reutilizar la clave.
	record, errLectura := s.store.LoadIdempotency(ctx, userID, OperationCreateGoal, key)
	if errLectura != nil {
		return nil, normalizeStoreError(errLectura)
	}
	if record != nil && recordReusable(record, now) {
		if record.PayloadHash != payloadHash {
			return nil, ErrConflict
		}
		if record.HTTPStatus < 100 || record.HTTPStatus > 599 || len(record.ResponseBody) == 0 {
			return nil, ErrInternal
		}
		return &StoredResponse{
			Status:     record.HTTPStatus,
			Body:       record.ResponseBody,
			Location:   record.Location,
			ResourceID: record.ResourceID,
			Replayed:   true,
		}, nil
	}

	goalID, errID := s.newID()
	if errID != nil {
		return nil, ErrInternal
	}
	goal, errAgregado := domainGoal.New(userID, goalID, name, cmd.TargetAmountCents, opening, targetDate, now)
	if errAgregado != nil {
		return nil, mapCreateDomainError(errAgregado)
	}
	body, errSerial := marshalContract(&goal)
	if errSerial != nil {
		return nil, errSerial
	}

	return respuestaLectura(s.store.CreateGoal(ctx, CreateWrite{
		Goal:           &goal,
		Operation:      OperationCreateGoal,
		IdempotencyKey: key,
		PayloadHash:    payloadHash,
		ResponseBody:   body,
		Location:       s.location(goal.ID),
		TTL:            IdempotencyTTL,
		Now:            now,
	}))
}

// respuestaLectura normaliza el resultado de la persistencia: un error no clasificado se
// traduce a ErrUnavailable (503) en lugar de filtrarse como 500. Así una caída de DynamoDB
// nunca se presenta como un fallo interno del endpoint.
func respuestaLectura(respuesta *StoredResponse, err error) (*StoredResponse, error) {
	if err == nil {
		return respuesta, nil
	}
	return nil, normalizeStoreError(err)
}

// normalizeStoreError conserva los errores de aplicación que el adaptador ya tradujo y
// convierte cualquier otro en ErrUnavailable.
func normalizeStoreError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrUnauthorized),
		errors.Is(err, ErrInvalidRequest),
		errors.Is(err, ErrNotFound),
		errors.Is(err, ErrConflict),
		errors.Is(err, ErrVersionConflict),
		errors.Is(err, ErrTargetNotPositive),
		errors.Is(err, ErrOpeningExceedsTarget),
		errors.Is(err, ErrTargetBelowSaved),
		errors.Is(err, ErrInvalidTransition),
		errors.Is(err, ErrInconsistent),
		errors.Is(err, ErrInternal),
		errors.Is(err, ErrUnavailable):
		return err
	default:
		return ErrUnavailable
	}
}

// recordReusable indica si el registro sigue vigente en el reloj del servicio. Un registro sin
// `expiresAt` se considera no reutilizable: el contrato del adaptador siempre lo escribe.
func recordReusable(record *IdempotencyRecord, now time.Time) bool {
	if record == nil || record.ExpiresAt.IsZero() {
		return false
	}
	return now.Before(record.ExpiresAt)
}

// mapCreateDomainError traduce los invariantes del agregado a los errores de aplicación. En la
// práctica sólo puede fallar por una inconsistencia interna: la entrada ya se validó arriba.
func mapCreateDomainError(err error) error {
	switch {
	case errors.Is(err, domainGoal.ErrTargetNotPositive):
		return ErrTargetNotPositive
	case errors.Is(err, domainGoal.ErrOpeningOutOfRange):
		return ErrOpeningExceedsTarget
	case errors.Is(err, domainGoal.ErrNameInvalid),
		errors.Is(err, domainGoal.ErrCurrencyInvalid),
		errors.Is(err, domainGoal.ErrAmountOutOfRange),
		errors.Is(err, domainGoal.ErrTargetDateInvalid):
		return fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	default:
		return ErrInternal
	}
}
