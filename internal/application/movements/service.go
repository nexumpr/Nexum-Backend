package movements

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// StatusCreated es el código de éxito de E028. Se declara aquí para que la capa de aplicación
// no dependa de `net/http` y aun así el registro de idempotencia guarde el código original.
const StatusCreated = 201

// createIDAttempts acota los reintentos ante una colisión de identificador. Una colisión de un
// UUIDv4 es estadísticamente imposible; el reintento existe para no devolver 409 por un fallo
// que no es del cliente.
const createIDAttempts = 2

// Store es el puerto de persistencia de movimientos.
//
// La superficie es deliberadamente estrecha: el adaptador de DynamoDB la implementa con
// `GetItem`, `Query` y `TransactWriteItems`, de modo que el compilador impide `Scan` y cualquier
// escritura fuera de transacción (A-060 §2.5, AC-A-060-01).
type Store interface {
	// CreateMovement crea el movimiento y su registro de idempotencia en UNA transacción. Si el
	// registro ya existía con el mismo `PayloadHash` devuelve la respuesta original con
	// `Replayed = true`; si existía con otro contenido devuelve ErrConflict.
	CreateMovement(ctx context.Context, write CreateWrite) (*CreateResult, error)
	// ListMovements consulta la partición del titular en orden ascendente por SK. Devuelve la
	// SK de continuación sólo cuando la persistencia confirma que hay más elementos.
	ListMovements(ctx context.Context, query ListQuery, startSK string, limit int) ([]Movement, string, error)
	// LoadMovement resuelve un movimiento propio por identificador mediante su localizador O(1).
	// Devuelve (nil, nil) si no existe o pertenece a otra partición: el servicio responde 404
	// homogéneo en ambos casos, sin revelar existencia.
	LoadMovement(ctx context.Context, userID, movementID string) (*Movement, error)
	// ReplaceMovement aplica una modificación con compare-and-swap por versión. Si la clave
	// cambia, mueve el ítem y actualiza el localizador en la misma transacción.
	ReplaceMovement(ctx context.Context, write ReplaceWrite) (*Movement, error)
	// DeleteMovement borra el ítem principal y su localizador. Devuelve ErrConflict si el ítem
	// cambió de posición o desapareció entre la lectura y la transacción.
	DeleteMovement(ctx context.Context, write DeleteWrite) error
}

// CreateWrite es la escritura completa del alta de E028.
type CreateWrite struct {
	UserID         string
	Movement       Movement
	IdempotencyKey string
	Operation      string
	PayloadHash    string
	// ResponseBody es el cuerpo 201 canónico. Se persiste sin re-serializar para que el replay
	// sea byte a byte idéntico al original.
	ResponseBody []byte
	Location     string
	Status       int
	Now          time.Time
	ExpiresAt    time.Time
}

// CreateResult es la respuesta de un alta: el original o su replay.
type CreateResult struct {
	Status   int
	Body     []byte
	Location string
	Replayed bool
}

// ReplaceWrite es la escritura de E030: el estado leído (`Previous`) y el resultante (`Next`),
// con la versión esperada como precondición.
type ReplaceWrite struct {
	UserID          string
	MovementID      string
	ExpectedVersion int64
	Previous        Movement
	Next            Movement
	Now             time.Time
}

// DeleteWrite es la escritura de E031: la posición leída del ítem que se va a borrar.
type DeleteWrite struct {
	UserID     string
	MovementID string
	Previous   Movement
	Now        time.Time
}

// Service contiene los casos de uso de movimientos. Es inmutable una vez construido: los
// métodos `With*` devuelven una copia, de modo que es seguro compartirlo entre peticiones.
type Service struct {
	store     Store
	cursorKey []byte
	now       func() time.Time
	newID     func() string
}

// NewService construye el servicio. Sin almacén, todas las operaciones fallan cerradas con
// ErrUnavailable: nunca se sustituye la persistencia por memoria local.
func NewService(store Store) *Service {
	return &Service{
		store:     store,
		now:       func() time.Time { return time.Now().UTC() },
		newID:     newUUIDv4,
		cursorKey: nil,
	}
}

// WithCursorKey fija la clave HMAC que firma los cursores de E027. Sin clave, el listado falla
// cerrado (503) en lugar de emitir cursores sin firmar.
func (s *Service) WithCursorKey(key []byte) *Service {
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

// WithIDGenerator inyecta un generador de identificadores determinista en pruebas.
func (s *Service) WithIDGenerator(gen func() string) *Service {
	if gen == nil {
		return s
	}
	copia := *s
	copia.newID = gen
	return &copia
}

// Now devuelve la hora del servicio en UTC.
func (s *Service) Now() time.Time { return s.now().UTC() }

// newUUIDv4 genera un identificador RFC 4122 versión 4 con `crypto/rand`.
func newUUIDv4() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// Create ejecuta `POST /api/v1/movements` (E028).
//
// La identidad llega como argumento desde el `sub` verificado: el comando no puede elegir el
// titular. Una misma Idempotency-Key con el mismo contenido reproduce el 201 original con
// `Replayed = true`; con contenido distinto devuelve ErrConflict (409).
func (s *Service) Create(ctx context.Context, userID string, cmd CreateCommand) (*CreateResult, error) {
	if s == nil || s.store == nil {
		return nil, ErrUnavailable
	}
	titular := strings.TrimSpace(userID)
	if titular == "" {
		return nil, ErrUnauthorized
	}
	if s.newID == nil {
		return nil, ErrInternal
	}

	ahora := s.now().UTC()
	normalizado, err := NormalizeCreate(cmd, ahora)
	if err != nil {
		return nil, err
	}

	ocurrido, ok := ParseStoredOccurredAt(normalizado.OccurredAt)
	if !ok {
		return nil, ErrInternal
	}

	payloadHash := PayloadHash(normalizado)

	for intento := 0; intento < createIDAttempts; intento++ {
		id := strings.TrimSpace(s.newID())
		if id == "" {
			return nil, ErrInternal
		}
		movimiento := Movement{
			ID:          id,
			Type:        normalizado.Type,
			AmountCents: AmountCents(normalizado.Amount),
			Currency:    normalizado.Currency,
			CategoryID:  normalizado.CategoryID,
			Description: normalizado.Description,
			OccurredAt:  ocurrido,
			Version:     1,
		}
		cuerpo, err := MarshalMovement(movimiento)
		if err != nil {
			return nil, ErrInternal
		}
		resultado, err := s.store.CreateMovement(ctx, CreateWrite{
			UserID:         titular,
			Movement:       movimiento,
			IdempotencyKey: normalizado.IdempotencyKey,
			Operation:      OperationCreateMovement,
			PayloadHash:    payloadHash,
			ResponseBody:   cuerpo,
			Location:       "/api/v1/movements/" + id,
			Status:         StatusCreated,
			Now:            ahora,
			ExpiresAt:      ahora.Add(IdempotencyTTL),
		})
		if errors.Is(err, ErrMovementIDCollision) {
			// No es un conflicto del cliente: se reintenta con otro identificador.
			continue
		}
		return resultado, err
	}
	return nil, ErrInternal
}

// List ejecuta `GET /api/v1/movements` (E027).
//
// El cursor es opaco y va firmado con HMAC: liga la identidad del titular y la firma de los
// filtros, de modo que reutilizar un cursor con otro `type`/`from`/`to` devuelve 400 en lugar de
// mezclar páginas de conjuntos distintos (E027 T07).
func (s *Service) List(ctx context.Context, query ListQuery) (*Page, error) {
	if s == nil || s.store == nil {
		return nil, ErrUnavailable
	}
	if len(s.cursorKey) == 0 {
		// Sin clave no se pueden firmar ni verificar cursores: fail-closed.
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

	items, nextSK, err := s.store.ListMovements(ctx, normalizada, startSK, normalizada.Limit)
	if err != nil {
		return nil, err
	}

	pagina := &Page{Items: items}
	if strings.TrimSpace(nextSK) != "" {
		cursor, errCursor := httputil.GenerateCursor(s.cursorKey, normalizada.UserID, firma, nextSK, s.now().UTC(), httputil.DefaultCursorTTL)
		if errCursor != nil {
			return nil, ErrInternal
		}
		pagina.NextCursor = cursor
	}
	return pagina, nil
}

// NormalizeListQuery aplica los límites del contrato a la consulta de E027.
//
// Un `limit` fuera de 1..100, un `cursor` con longitud fuera de 8..2048, un `type` fuera del
// enum o un rango `from > to` son 400: no se recortan en silencio.
func NormalizeListQuery(q ListQuery) (ListQuery, error) {
	if q.Limit == 0 {
		q.Limit = DefaultLimit
	}
	if q.Limit < 1 || q.Limit > MaxLimit {
		return ListQuery{}, fmt.Errorf("%w: limit", ErrInvalidRequest)
	}

	q.Cursor = strings.TrimSpace(q.Cursor)
	if q.Cursor != "" && (len(q.Cursor) < CursorMinLength || len(q.Cursor) > CursorMaxLength) {
		return ListQuery{}, fmt.Errorf("%w: cursor", ErrInvalidRequest)
	}

	if strings.TrimSpace(q.Type) != "" {
		tipo, err := NormalizeType(q.Type)
		if err != nil {
			// Un valor fuera del enum en un parámetro de consulta es una forma inválida (400),
			// no una regla de negocio del recurso (422). No se propaga el *FieldViolation.
			return ListQuery{}, fmt.Errorf("%w: type", ErrInvalidRequest)
		}
		q.Type = tipo
	} else {
		q.Type = ""
	}

	if q.HasFrom {
		q.From = q.From.UTC().Truncate(time.Millisecond)
	}
	if q.HasTo {
		q.To = q.To.UTC().Truncate(time.Millisecond)
	}
	if q.HasFrom && q.HasTo && q.From.After(q.To) {
		return ListQuery{}, fmt.Errorf("%w: from", ErrInvalidRequest)
	}
	return q, nil
}

// FilterSignature describe el conjunto exacto que pagina un cursor. Se guarda en el campo
// `SortBy` del cursor firmado y se compara al reanudar.
func FilterSignature(q ListQuery) string {
	desde, hasta := "", ""
	if q.HasFrom {
		desde = SortableTime(q.From)
	}
	if q.HasTo {
		hasta = SortableTime(q.To)
	}
	return fmt.Sprintf("type=%s;from=%s;to=%s", q.Type, desde, hasta)
}

// deleteAttempts acota los reintentos de E031 cuando el ítem se movió entre la lectura y el
// borrado (un PATCH concurrente que cambia `occurredAt`).
const deleteAttempts = 2

// Get ejecuta `GET /api/v1/movements/{movementId}` (E029).
//
// El identificador se valida antes de tocar la persistencia: un uuid mal formado es 400 y no un
// 404, de modo que un error de cliente no se confunde con un recurso inexistente. Un movimiento
// de otro titular vive en otra partición y nunca se localiza: la respuesta es 404 idéntica a la
// de un identificador inexistente.
func (s *Service) Get(ctx context.Context, userID, movementID string) (*Movement, error) {
	if s == nil || s.store == nil {
		return nil, ErrUnavailable
	}
	titular := strings.TrimSpace(userID)
	if titular == "" {
		return nil, ErrUnauthorized
	}
	id := strings.TrimSpace(movementID)
	if !ValidMovementID(id) {
		return nil, fmt.Errorf("%w: movementId", ErrInvalidRequest)
	}

	movimiento, err := s.store.LoadMovement(ctx, titular, id)
	if err != nil {
		return nil, err
	}
	if movimiento == nil {
		return nil, ErrNotFound
	}
	return movimiento, nil
}

// Patch ejecuta `PATCH /api/v1/movements/{movementId}` (E030).
//
// Reglas aplicadas: `If-Match` ya validado por el transporte; sólo `categoryId`, `description` y
// `occurredAt` son editables; `type`, `amount`, `currency`, `contributionId` y
// `recurringPaymentId` no lo son, de modo que no puede alterarse un vínculo ni un importe. Una
// petición sin campos aplicables es un no-op que devuelve el estado vigente sin subir la versión.
func (s *Service) Patch(ctx context.Context, userID, movementID string, expectedVersion int64, patch Patch) (*Movement, error) {
	if s == nil || s.store == nil {
		return nil, ErrUnavailable
	}
	titular := strings.TrimSpace(userID)
	if titular == "" {
		return nil, ErrUnauthorized
	}
	id := strings.TrimSpace(movementID)
	if !ValidMovementID(id) {
		return nil, fmt.Errorf("%w: movementId", ErrInvalidRequest)
	}
	if expectedVersion < 1 {
		return nil, fmt.Errorf("%w: If-Match", ErrInvalidRequest)
	}

	actual, err := s.store.LoadMovement(ctx, titular, id)
	if err != nil {
		return nil, err
	}
	if actual == nil {
		return nil, ErrNotFound
	}
	// T-1 (RFC-022 v1.2.1 §10): un movimiento vinculado a una aportación es historia económica
	// de esa aportación. Permitir cambiar categoría, descripción o fecha haría divergir el libro
	// de la aportación y de las metas. La operación se rechaza con 409 sin escribir nada; la
	// reversión explícita corresponde a E047. Los movimientos independientes no cambian.
	if strings.TrimSpace(actual.ContributionID) != "" {
		return nil, ErrContributionLinked
	}
	if actual.Version != expectedVersion {
		return nil, ErrVersionConflict
	}

	ahora := s.now().UTC()
	siguiente := *actual
	cambios := 0

	if patch.CategoryID != nil {
		categoria, err := NormalizeCategoryID(*patch.CategoryID)
		if err != nil {
			return nil, err
		}
		if categoria != siguiente.CategoryID {
			siguiente.CategoryID = categoria
			cambios++
		}
	}
	if patch.Description != nil {
		descripcion, err := NormalizeDescription(*patch.Description)
		if err != nil {
			return nil, err
		}
		if descripcion != siguiente.Description {
			siguiente.Description = descripcion
			cambios++
		}
	}
	if patch.OccurredAt != nil {
		ocurrido, err := NormalizeOccurredAt(*patch.OccurredAt, ahora)
		if err != nil {
			return nil, err
		}
		if !ocurrido.Equal(siguiente.OccurredAt) {
			siguiente.OccurredAt = ocurrido
			cambios++
		}
	}

	if cambios == 0 {
		// Nada que escribir: se devuelve el estado vigente y el ETag no cambia.
		return actual, nil
	}

	siguiente.Version = actual.Version + 1
	actualizado, err := s.store.ReplaceMovement(ctx, ReplaceWrite{
		UserID:          titular,
		MovementID:      id,
		ExpectedVersion: expectedVersion,
		Previous:        *actual,
		Next:            siguiente,
		Now:             ahora,
	})
	if err != nil {
		return s.resolverConflictoReemplazo(ctx, titular, id, expectedVersion, err)
	}
	if actualizado == nil {
		return nil, ErrInternal
	}
	return actualizado, nil
}

// resolverConflictoReemplazo distingue, tras una cancelación por condición, entre versión
// desactualizada (412), recurso desaparecido (404) y carrera sin cambio de versión (409). No se
// escribe nada en ninguno de los tres casos.
func (s *Service) resolverConflictoReemplazo(ctx context.Context, userID, movementID string, esperada int64, err error) (*Movement, error) {
	if !errors.Is(err, ErrConflict) {
		return nil, err
	}
	actual, errLectura := s.store.LoadMovement(ctx, userID, movementID)
	if errLectura != nil {
		return nil, errLectura
	}
	if actual == nil {
		return nil, ErrNotFound
	}
	if actual.Version != esperada {
		return nil, ErrVersionConflict
	}
	return nil, ErrConflict
}

// Delete ejecuta `DELETE /api/v1/movements/{movementId}` (E031).
//
// Un movimiento ligado a una aportación confirmada (`contributionId` no nulo) no se borra: la
// respuesta es 409 y la reversa corresponde a la operación de metas, no a este endpoint. Si el
// ítem se movió entre la lectura y el borrado (un PATCH concurrente), se relee y se reintenta una
// vez con la posición nueva.
func (s *Service) Delete(ctx context.Context, userID, movementID string) error {
	if s == nil || s.store == nil {
		return ErrUnavailable
	}
	titular := strings.TrimSpace(userID)
	if titular == "" {
		return ErrUnauthorized
	}
	id := strings.TrimSpace(movementID)
	if !ValidMovementID(id) {
		return fmt.Errorf("%w: movementId", ErrInvalidRequest)
	}

	ahora := s.now().UTC()
	for intento := 0; intento < deleteAttempts; intento++ {
		actual, err := s.store.LoadMovement(ctx, titular, id)
		if err != nil {
			return err
		}
		if actual == nil {
			return ErrNotFound
		}
		if strings.TrimSpace(actual.ContributionID) != "" {
			return ErrContributionLinked
		}
		errBorrado := s.store.DeleteMovement(ctx, DeleteWrite{
			UserID:     titular,
			MovementID: id,
			Previous:   *actual,
			Now:        ahora,
		})
		if errBorrado == nil {
			return nil
		}
		if !errors.Is(errBorrado, ErrConflict) {
			return errBorrado
		}
		// El ítem se movió o cambió: se relee y se decide en la siguiente vuelta.
	}
	return ErrConflict
}
