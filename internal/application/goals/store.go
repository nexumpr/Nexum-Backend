package goals

import (
	"context"
	"time"

	domainGoal "github.com/nexumpr/Nexum-Backend/internal/domain/goal"
	"github.com/nexumpr/Nexum-Backend/internal/domain/money"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
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

	// LoadGoal lee una meta del titular con consistencia fuerte. Devuelve (nil, nil) cuando la
	// clave no existe o la ocupa otra entidad: el servicio responde 404 en ambos casos para no
	// enumerar recursos ajenos.
	LoadGoal(ctx context.Context, userID, goalID string) (*domainGoal.Goal, error)

	// ListUserGoals devuelve una página de metas del titular y la `SK` desde la que continuar
	// (vacía cuando no hay más). `HasStatus` activa el filtro por estado en la propia consulta.
	ListUserGoals(ctx context.Context, query ListStoreQuery) ([]GoalRow, string, error)

	// AggregateUserGoals recorre TODA la colección de metas del titular y devuelve el número
	// de metas activas y la suma de su ahorro. El resultado no depende de la página solicitada
	// (A8 de RFC-016).
	AggregateUserGoals(ctx context.Context, userID string) (Totals, error)

	// UpdateGoal aplica una modificación con compare-and-swap por versión (E041). Debe
	// imponer además, de forma atómica, que el nuevo monto objetivo no quede por debajo del
	// ahorro acumulado. Un fallo de la condición se traduce en ErrVersionConflict (412),
	// ErrTargetBelowSaved (422) o ErrConflict (409) tras agotar los reintentos.
	UpdateGoal(ctx context.Context, write UpdateWrite) (*domainGoal.Goal, error)

	// DeleteGoal marca la meta como borrada (E042, RFC-022 §6 alternativa B) con una escritura
	// condicional: `deleted=true`, `status=archived`, `savedAmount=openingSaved` y `version+1`
	// en UNA sola operación. No revierte aportaciones y NUNCA elimina el item: preserva el
	// historial económico. Repetir el borrado sobre un tombstone es idempotente (nil), una meta
	// inexistente o ajena devuelve ErrNotFound y una meta con aportaciones vigentes devuelve
	// ErrGoalActiveContributions sin modificar nada.
	DeleteGoal(ctx context.Context, write DeleteWrite) error
}

// ListStoreQuery es la consulta paginada que recibe la persistencia.
type ListStoreQuery struct {
	UserID    string
	Status    domainGoal.Status
	HasStatus bool
	StartSK   string
	Limit     int
}

// GoalRow es una meta leída de la partición junto a su `SK` completa. La `SK` es lo único que
// se codifica en el cursor: nunca viaja el identificador de usuario en claro.
type GoalRow struct {
	Goal    domainGoal.Goal
	SortKey string
}

// Totals es el agregado de la colección completa: metas en estado `active` y suma exacta de su
// ahorro, en céntimos.
type Totals struct {
	ActiveCount int
	Saved       money.Money
}

// Límites de página del contrato de E038. Viven junto al puerto porque los comparten el
// adaptador (validación de la consulta), el caso de uso y el transporte.
const (
	// DefaultListLimit es el tamaño de página por defecto del contrato (25).
	DefaultListLimit = httputil.DefaultPageLimit

	// MaxListLimit es el tamaño de página máximo admitido por el contrato (100).
	MaxListLimit = httputil.MaxPageLimit
)

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

// DeleteWrite es la escritura de E042 ya normalizada. No lleva la versión esperada: el
// adaptador lee el estado vigente con consistencia fuerte, decide si procede el tombstone y
// repite el compare-and-swap internamente, de modo que el contrato del puerto no obliga al
// llamador a aportar un validador que el cliente no envía en este endpoint.
type DeleteWrite struct {
	UserID string
	GoalID string
	Now    time.Time
}
