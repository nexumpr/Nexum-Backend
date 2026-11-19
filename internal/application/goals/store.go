package goals

import (
	"context"
	"time"

	domainGoal "github.com/nexumpr/Nexum-Backend/internal/domain/goal"
)

// Store es el puerto de persistencia de los casos de uso de metas. El adaptador real es
// `internal/adapters/dynamo.GoalsStore`; las pruebas usan dobles con estado.
//
// Contrato del adaptador:
//   - Ninguna operación usa `Scan`: todo se resuelve por clave exacta o `Query` sobre la
//     partición del titular (`PK = USER#<id>`).
//   - Un fallo real de DynamoDB se propaga como ErrUnavailable y NUNCA se convierte en
//     «no existe», que produciría un 404 falso.
//   - `CreateGoal` escribe la meta y el registro de idempotencia en UNA transacción: o se
//     confirman los dos, o no se confirma ninguno.
type Store interface {
	// LoadIdempotency devuelve el registro de idempotencia del titular, o (nil, nil) si no
	// existe, si pertenece a otra entidad o si no está completado. La expiración lógica la
	// evalúa el servicio con su propio reloj, para poder probarla de forma determinista.
	LoadIdempotency(ctx context.Context, userID, operation, idempotencyKey string) (*IdempotencyRecord, error)

	// CreateGoal crea la meta junto al registro de idempotencia. Si la condición del registro
	// falla porque otra petición ganó la carrera, devuelve el replay almacenado (mismo
	// payload) o ErrConflict (payload distinto).
	CreateGoal(ctx context.Context, write CreateWrite) (*StoredResponse, error)
}

// IdempotencyRecord es el registro persistido de una operación idempotente.
type IdempotencyRecord struct {
	PayloadHash  string
	Status       string
	HTTPStatus   int
	ResponseBody []byte
	ResourceID   string
	Location     string
	ExpiresAt    time.Time
}

// CreateWrite agrupa todo lo que la persistencia necesita para crear la meta y su registro de
// idempotencia en una sola transacción.
type CreateWrite struct {
	Goal           *domainGoal.Goal
	Operation      string
	IdempotencyKey string
	PayloadHash    string
	ResponseBody   []byte
	Location       string
	TTL            time.Duration
	Now            time.Time
}

// StoredResponse es la respuesta HTTP reproducible: el cuerpo es exactamente el que se
// guardó, de modo que un replay devuelve el mismo 201 byte a byte.
type StoredResponse struct {
	Status     int
	Body       []byte
	Location   string
	ResourceID string
	Replayed   bool
}
