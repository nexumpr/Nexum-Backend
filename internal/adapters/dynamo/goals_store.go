package dynamo

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	appGoals "github.com/nexumpr/Nexum-Backend/internal/application/goals"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
	domainGoal "github.com/nexumpr/Nexum-Backend/internal/domain/goal"
)

// GoalsAPI es el subconjunto del cliente DynamoDB que usan los endpoints de metas. Se declara
// como interfaz propia (y no se reutiliza `DynamoAPI`) para que el compilador impida `Scan`:
// las metas se resuelven siempre por clave exacta o `Query` en la partición del titular.
type GoalsAPI interface {
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	TransactWriteItems(ctx context.Context, params *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error)
}

const (
	// goalsEntityType identifica el item de meta dentro de la tabla única.
	goalsEntityType = "GOAL"

	// idempotencyEntityType identifica el registro de idempotencia financiera.
	idempotencyEntityType = "IDEMPOTENCY_RECORD"

	// idempotencyStatusCompleted es el único estado que E039 escribe: la transacción es
	// atómica, así que no existen registros «en curso» que otro lector pudiera interpretar.
	idempotencyStatusCompleted = "COMPLETED"

	// metaCreadaStatus es el código que se guarda y se reproduce en un replay.
	metaCreadaStatus = 201

	// indiceMeta e indiceIdempotencia son las posiciones de las dos acciones de la
	// transacción de E039: DynamoDB devuelve una razón de cancelación por acción, en orden.
	indiceMeta         = 0
	indiceIdempotencia = 1
)

// GoalsStore implementa `goals.Store` sobre la tabla única existente. No crea tablas ni
// índices y no requiere cambios de IAM: `USER#*` ya está autorizado.
//
// Patrón físico (DISEÑO_OLEADA_1 §4, A-060):
//
//	meta            USER#<userId> / GOAL#<goalId>
//	idempotencia    USER#<userId> / IDEM#<operación>#<sha256(operación:clave)>
//
// Los importes se persisten como enteros de céntimos (`targetAmountCents`, `savedAmountCents`,
// `openingSavedCents`): nunca como número decimal en coma flotante (R-030, GOV-08).
type GoalsStore struct {
	client GoalsAPI
	table  string
}

// NewGoalsStore construye el adaptador. Sin cliente o tabla todas las operaciones fallan
// cerradas con ErrUnavailable; nunca se sustituyen por memoria local.
func NewGoalsStore(client GoalsAPI, table string) *GoalsStore {
	return &GoalsStore{client: client, table: strings.TrimSpace(table)}
}

// goalItem proyecta el agregado al item de DynamoDB.
func goalItem(goal *domainGoal.Goal) map[string]types.AttributeValue {
	pk, sk := domainDynamo.GoalKeys(goal.UserID, goal.ID)
	return map[string]types.AttributeValue{
		"PK":                stringAttr(pk),
		"SK":                stringAttr(sk),
		"entityType":        stringAttr(goalsEntityType),
		"userId":            stringAttr(goal.UserID),
		"goalId":            stringAttr(goal.ID),
		"name":              stringAttr(goal.Name),
		"targetAmountCents": numberAttr(goal.TargetAmount.Amount()),
		"savedAmountCents":  numberAttr(goal.SavedAmount.Amount()),
		"openingSavedCents": numberAttr(goal.OpeningSaved.Amount()),
		"currency":          stringAttr(goal.TargetAmount.Currency()),
		"targetDate":        stringAttr(goal.FormatTargetDate()),
		"status":            stringAttr(goal.Status.String()),
		"version":           numberAttr(goal.Version),
		"createdAt":         stringAttr(goal.CreatedAt.UTC().Format(time.RFC3339)),
		"updatedAt":         stringAttr(goal.UpdatedAt.UTC().Format(time.RFC3339)),
	}
}

// idempotencyItem proyecta el registro de idempotencia. El cuerpo de la respuesta se guarda
// inmutable para que el replay sea idéntico byte a byte; el SK no contiene la clave cruda del
// cliente, sólo su SHA-256.
func idempotencyItem(write appGoals.CreateWrite, expiresAt int64, now time.Time) map[string]types.AttributeValue {
	pk, sk := domainDynamo.UserIdempotencyKeys(write.Goal.UserID, write.Operation, write.IdempotencyKey)
	return map[string]types.AttributeValue{
		"PK":           stringAttr(pk),
		"SK":           stringAttr(sk),
		"entityType":   stringAttr(idempotencyEntityType),
		"operation":    stringAttr(write.Operation),
		"userId":       stringAttr(write.Goal.UserID),
		"payloadHash":  stringAttr(write.PayloadHash),
		"status":       stringAttr(idempotencyStatusCompleted),
		"httpStatus":   numberAttr(int64(metaCreadaStatus)),
		"responseBody": stringAttr(string(write.ResponseBody)),
		"resourceId":   stringAttr(write.Goal.ID),
		"location":     stringAttr(write.Location),
		"createdAt":    stringAttr(now.UTC().Format(time.RFC3339)),
		"expiresAt":    numberAttr(expiresAt),
	}
}

// LoadIdempotency lee el registro de idempotencia con consistencia fuerte.
//
// Devuelve (nil, nil) cuando la clave no existe, la ocupa otra entidad o el registro no está
// completado: en esos casos no hay respuesta reproducible que devolver y el llamador decide.
// Un error real de DynamoDB se propaga como ErrUnavailable y nunca se convierte en «no hay
// registro», que provocaría una escritura duplicada.
func (s *GoalsStore) LoadIdempotency(ctx context.Context, userID, operation, idempotencyKey string) (*appGoals.IdempotencyRecord, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, appGoals.ErrUnavailable
	}
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(operation) == "" || strings.TrimSpace(idempotencyKey) == "" {
		return nil, appGoals.ErrUnavailable
	}

	pk, sk := domainDynamo.UserIdempotencyKeys(userID, operation, idempotencyKey)
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(pk),
			"SK": stringAttr(sk),
		},
		// La decisión replay/409 no puede basarse en una réplica retrasada.
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return nil, appGoals.ErrUnavailable
	}
	if out == nil || len(out.Item) == 0 {
		return nil, nil
	}
	item := out.Item
	if entity := stringAttribute(item["entityType"]); entity != idempotencyEntityType {
		return nil, nil
	}
	if status := stringAttribute(item["status"]); status != idempotencyStatusCompleted {
		return nil, nil
	}

	payloadHash := stringAttribute(item["payloadHash"])
	if payloadHash == "" {
		return nil, appGoals.ErrInconsistent
	}
	httpStatus, ok := attributeInt(item["httpStatus"])
	if !ok || httpStatus < 100 || httpStatus > 599 {
		return nil, appGoals.ErrInconsistent
	}
	body := stringAttribute(item["responseBody"])
	if body == "" {
		return nil, appGoals.ErrInconsistent
	}

	record := &appGoals.IdempotencyRecord{
		PayloadHash:  payloadHash,
		Status:       idempotencyStatusCompleted,
		HTTPStatus:   int(httpStatus),
		ResponseBody: []byte(body),
		ResourceID:   stringAttribute(item["resourceId"]),
		Location:     stringAttribute(item["location"]),
	}
	if expiresAt, ok := attributeInt(item["expiresAt"]); ok && expiresAt > 0 {
		record.ExpiresAt = time.Unix(expiresAt, 0).UTC()
	}
	return record, nil
}

// CreateGoal ejecuta la transacción de E039 y, si la condición del registro de idempotencia
// falla, resuelve el replay o el conflicto.
func (s *GoalsStore) CreateGoal(ctx context.Context, write appGoals.CreateWrite) (*appGoals.StoredResponse, error) {
	return s.createGoal(ctx, write, true)
}

func (s *GoalsStore) createGoal(ctx context.Context, write appGoals.CreateWrite, permitirReintento bool) (*appGoals.StoredResponse, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, appGoals.ErrUnavailable
	}
	if write.Goal == nil {
		return nil, appGoals.ErrUnavailable
	}
	if strings.TrimSpace(write.Operation) == "" || strings.TrimSpace(write.IdempotencyKey) == "" || strings.TrimSpace(write.PayloadHash) == "" {
		return nil, appGoals.ErrInvalidRequest
	}
	if len(write.ResponseBody) == 0 {
		return nil, appGoals.ErrInternal
	}

	now := write.Now.UTC()
	ttl := write.TTL
	if ttl <= 0 {
		ttl = appGoals.IdempotencyTTL
	}
	expiresAt := now.Add(ttl).Unix()

	// Dos acciones en UNA transacción: la meta y el registro de idempotencia. La condición
	// del registro permite reutilizar la clave una vez vencido su vencimiento lógico; la de
	// la meta impide sobrescribir un item existente (colisión de identificador).
	transaccion := []types.TransactWriteItem{
		{Put: &types.Put{
			TableName:           aws.String(s.table),
			Item:                goalItem(write.Goal),
			ConditionExpression: aws.String("attribute_not_exists(PK)"),
		}},
		{Put: &types.Put{
			TableName:                 aws.String(s.table),
			Item:                      idempotencyItem(write, expiresAt, now),
			ConditionExpression:       aws.String("(attribute_not_exists(PK) OR expiresAt < :now)"),
			ExpressionAttributeValues: map[string]types.AttributeValue{":now": numberAttr(now.Unix())},
		}},
	}

	_, err := s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: transaccion})
	if err == nil {
		return &appGoals.StoredResponse{
			Status:     metaCreadaStatus,
			Body:       write.ResponseBody,
			Location:   write.Location,
			ResourceID: write.Goal.ID,
		}, nil
	}

	var cancelada *types.TransactionCanceledException
	if !errors.As(err, &cancelada) {
		// Throttling, red u otro fallo: no se confirma ninguna escritura parcial.
		return nil, appGoals.ErrUnavailable
	}
	if falloDeCondicionEn(cancelada, indiceIdempotencia) {
		return s.resolveReplayOrConflict(ctx, write, permitirReintento)
	}
	if falloDeCondicionEn(cancelada, indiceMeta) {
		// La condición de la meta puede fallar en un replay ya confirmado (el item existe) o
		// en una colisión real del identificador generado. Se relee el registro: si hay uno
		// vigente con el mismo payload es un replay; si no, es un fallo interno. No se
		// reintenta, porque el identificador volvería a colisionar.
		return s.resolveReplayOnly(ctx, write)
	}
	return nil, appGoals.ErrUnavailable
}

// replayFromRecord reproduce una respuesta almacenada. Devuelve (nil, false, nil) cuando el
// registro no existe o venció lógicamente y, por tanto, no hay nada que reproducir.
func replayFromRecord(record *appGoals.IdempotencyRecord, write appGoals.CreateWrite) (*appGoals.StoredResponse, bool, error) {
	if record == nil || record.ExpiresAt.IsZero() {
		return nil, false, nil
	}
	if !write.Now.UTC().Before(record.ExpiresAt) {
		return nil, false, nil
	}
	if record.PayloadHash != write.PayloadHash {
		return nil, false, appGoals.ErrConflict
	}
	if record.HTTPStatus < 100 || record.HTTPStatus > 599 || len(record.ResponseBody) == 0 {
		return nil, false, appGoals.ErrInternal
	}
	return &appGoals.StoredResponse{
		Status:     record.HTTPStatus,
		Body:       record.ResponseBody,
		Location:   record.Location,
		ResourceID: record.ResourceID,
		Replayed:   true,
	}, true, nil
}

// resolveReplayOrConflict relee el registro tras una cancelación por condición.
//
//   - Mismo payloadHash y registro vigente  -> replay con el cuerpo y código originales.
//   - payloadHash distinto                   -> 409 sin escribir nada.
//   - Sin registro (carrera con el TTL)      -> un único reintento de la transacción; si
//     vuelve a fallar, 409 sin duplicar.
func (s *GoalsStore) resolveReplayOrConflict(ctx context.Context, write appGoals.CreateWrite, permitirReintento bool) (*appGoals.StoredResponse, error) {
	record, err := s.LoadIdempotency(ctx, write.Goal.UserID, write.Operation, write.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	replay, ok, errReplay := replayFromRecord(record, write)
	if errReplay != nil {
		return nil, errReplay
	}
	if ok {
		return replay, nil
	}

	if permitirReintento {
		return s.createGoal(ctx, write, false)
	}
	return nil, appGoals.ErrConflict
}

// resolveReplayOnly relee el registro cuando sólo falló la condición de la meta. Sin registro
// vigente, la causa es una colisión del identificador generado: fallo interno, nunca un
// conflicto del cliente.
func (s *GoalsStore) resolveReplayOnly(ctx context.Context, write appGoals.CreateWrite) (*appGoals.StoredResponse, error) {
	record, err := s.LoadIdempotency(ctx, write.Goal.UserID, write.Operation, write.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	replay, ok, errReplay := replayFromRecord(record, write)
	if errReplay != nil {
		return nil, errReplay
	}
	if ok {
		return replay, nil
	}
	return nil, appGoals.ErrInternal
}

// falloDeCondicionEn indica si la acción `indice` de la transacción falló por condición.
func falloDeCondicionEn(err *types.TransactionCanceledException, indice int) bool {
	if err == nil || indice < 0 || indice >= len(err.CancellationReasons) {
		return false
	}
	razon := err.CancellationReasons[indice]
	return razon.Code != nil && *razon.Code == "ConditionalCheckFailed"
}
