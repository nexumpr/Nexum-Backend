package contributions

import (
	"context"
	"time"

	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
)

// Store es el puerto de persistencia. El adaptador real es
// `internal/adapters/dynamo.ContributionsStore`; las pruebas usan dobles con estado.
//
// Contrato del adaptador:
//   - Ninguna operación usa `Scan`: todo se resuelve por `GetItem` o `Query` en `USER#<id>`.
//   - Un fallo real de DynamoDB se propaga como ErrUnavailable y NUNCA se convierte en
//     «no existe».
//   - `CreateContribution` escribe aportación, puntero, movimiento, puntero del movimiento,
//     metas e idempotencia en UNA transacción; o se confirman todos o ninguno.
//   - `ReverseContribution` revierte aportación y metas en UNA transacción condicionada.
type Store interface {
	LoadIdempotency(ctx context.Context, userID, operation, idempotencyKey string) (*IdempotencyRecord, error)
	LoadGoal(ctx context.Context, userID, goalID string) (*GoalSnapshot, error)
	CreateContribution(ctx context.Context, write CreateWrite) (*StoredResponse, error)
	ListContributions(ctx context.Context, query ListQuery, startSK string, limit int) ([]Contribution, string, error)
	LoadContribution(ctx context.Context, userID, contributionID string) (*Contribution, error)
	ReverseContribution(ctx context.Context, write ReverseWrite) (*StoredResponse, error)
}

// IdempotencyRecord es el registro persistido de una operación idempotente.
type IdempotencyRecord struct {
	PayloadHash  string
	HTTPStatus   int
	ResponseBody []byte
	Location     string
	ExpiresAt    time.Time
}

// StoredResponse es la respuesta HTTP reproducible byte a byte.
type StoredResponse struct {
	Status   int
	Body     []byte
	Location string
	Replayed bool
}

// CreateWrite agrupa la escritura atómica de E044.
type CreateWrite struct {
	UserID       string
	Contribution Contribution
	Movement     appMovements.Movement
	// Goals es la lectura consistente previa de cada meta. El adaptador usa `TargetCents` y
	// `Version` para construir la condición atómica de capacidad y detectar cambios
	// concurrentes del objetivo.
	Goals          map[string]GoalSnapshot
	IdempotencyKey string
	Operation      string
	PayloadHash    string
	ResponseBody   []byte
	Location       string
	Now            time.Time
	ExpiresAt      time.Time
}

// ReverseWrite agrupa la escritura atómica de E047.
type ReverseWrite struct {
	UserID         string
	Previous       Contribution
	Next           Contribution
	IdempotencyKey string
	Operation      string
	PayloadHash    string
	ResponseBody   []byte
	Location       string
	Now            time.Time
	ExpiresAt      time.Time
}
