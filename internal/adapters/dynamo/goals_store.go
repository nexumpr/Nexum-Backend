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
	"github.com/nexumpr/Nexum-Backend/internal/domain/money"
)

// GoalsAPI es el subconjunto del cliente DynamoDB que usan los endpoints de metas. Se declara
// como interfaz propia (y no se reutiliza `DynamoAPI`) para que el compilador impida `Scan`:
// las metas se resuelven siempre por clave exacta o `Query` en la partición del titular.
type GoalsAPI interface {
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
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

	// goalsListMaxPages acota las páginas internas de E038. Si se agota con más datos y sin
	// ninguna fila que devolver, la petición falla cerrada (503) en lugar de presentar una
	// página vacía que el cliente interpretaría como «no hay metas». Si ya hay filas, se
	// devuelve la página parcial con cursor: no se pierde ni se duplica nada.
	goalsListMaxPages = 20

	// goalsAggregatePageSize y goalsAggregateMaxPages acotan el recorrido completo de los
	// totales de E038. Agotar el tope es un fallo cerrado (503): nunca se devuelven totales
	// parciales como si fueran exactos.
	goalsAggregatePageSize = 100
	goalsAggregateMaxPages = 50

	// goalsAggregateMaxItems es el máximo de metas activas sumadas. Por encima, 503.
	goalsAggregateMaxItems = 5000
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

// goalFromItem mapea un item de la tabla a la meta de dominio.
//
// Devuelve (nil, nil) cuando el item pertenece a otra entidad: no es un error, simplemente no
// hay meta en esa clave. Cualquier defecto de forma en un item que SÍ declara ser una meta se
// devuelve como ErrInconsistent: nunca se emite al cliente un valor fuera del contrato ni un
// importe inventado.
func goalFromItem(item map[string]types.AttributeValue) (*domainGoal.Goal, error) {
	if len(item) == 0 {
		return nil, nil
	}
	if entity := stringAttribute(item["entityType"]); entity != goalsEntityType {
		return nil, nil
	}

	goalID := stringAttribute(item["goalId"])
	if goalID == "" {
		goalID = strings.TrimPrefix(stringAttribute(item["SK"]), domainDynamo.PrefixGoal)
	}
	userID := stringAttribute(item["userId"])
	if goalID == "" || userID == "" {
		return nil, appGoals.ErrInconsistent
	}

	targetCents, okTarget := attributeInt(item["targetAmountCents"])
	savedCents, okSaved := attributeInt(item["savedAmountCents"])
	openingCents, okOpening := attributeInt(item["openingSavedCents"])
	version, okVersion := attributeInt(item["version"])
	if !okTarget || !okSaved || !okOpening || !okVersion {
		return nil, appGoals.ErrInconsistent
	}

	targetDate, errFecha := time.Parse(domainGoal.DateLayout, strings.TrimSpace(stringAttribute(item["targetDate"])))
	if errFecha != nil {
		return nil, appGoals.ErrInconsistent
	}
	status, errEstado := domainGoal.ParseStatus(stringAttribute(item["status"]))
	if errEstado != nil {
		return nil, appGoals.ErrInconsistent
	}
	createdAt, errCreado := parseGoalTime(stringAttribute(item["createdAt"]))
	if errCreado != nil {
		return nil, appGoals.ErrInconsistent
	}
	updatedAt, errActualizado := parseGoalTime(stringAttribute(item["updatedAt"]))
	if errActualizado != nil {
		updatedAt = createdAt
	}

	currency := stringAttribute(item["currency"])
	target, errTarget := money.New(targetCents, currency)
	if errTarget != nil {
		return nil, appGoals.ErrInconsistent
	}
	saved, errSaved := money.New(savedCents, currency)
	if errSaved != nil {
		return nil, appGoals.ErrInconsistent
	}
	opening, errOpening := money.New(openingCents, currency)
	if errOpening != nil {
		return nil, appGoals.ErrInconsistent
	}

	goal := domainGoal.Goal{
		ID:           goalID,
		UserID:       userID,
		Name:         stringAttribute(item["name"]),
		TargetAmount: target,
		SavedAmount:  saved,
		OpeningSaved: opening,
		TargetDate:   targetDate.UTC(),
		Status:       status,
		Version:      version,
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
	}
	if err := goal.Validate(); err != nil {
		return nil, appGoals.ErrInconsistent
	}
	return &goal, nil
}

// parseGoalTime interpreta las marcas RFC3339 que escribe este adaptador.
func parseGoalTime(value string) (time.Time, error) {
	return time.Parse(time.RFC3339, strings.TrimSpace(value))
}

// LoadGoal lee `USER#<userId>/GOAL#<goalId>` con consistencia fuerte (E040).
//
// Devuelve (nil, nil) cuando la clave no existe, el TTL la purgó o el item es de otra entidad:
// el llamador responde 404 homogéneo en todos esos casos. Un error real de DynamoDB se propaga
// como ErrUnavailable y nunca se convierte en «no existe».
func (s *GoalsStore) LoadGoal(ctx context.Context, userID, goalID string) (*domainGoal.Goal, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, appGoals.ErrUnavailable
	}
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(goalID) == "" {
		return nil, nil
	}

	pk, sk := domainDynamo.GoalKeys(userID, goalID)
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(pk),
			"SK": stringAttr(sk),
		},
		// La versión leída decide el ETag de E040 y el CAS de E041: no puede venir de una
		// réplica retrasada.
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return nil, appGoals.ErrUnavailable
	}
	if out == nil || len(out.Item) == 0 {
		return nil, nil
	}
	return goalFromItem(out.Item)
}

// ListUserGoals consulta la partición del titular con
// `PK = :pk AND begins_with(SK, "GOAL#")`, orden ascendente por `SK` y paginación por
// `ExclusiveStartKey` (E038). El filtro de estado se aplica en la propia consulta.
//
// DynamoDB aplica `Limit` a los ítems LEÍDOS antes del filtro, de modo que una página puede
// llegar vacía con `LastEvaluatedKey`. El bucle continúa hasta completar la página o agotar la
// partición; si se alcanza el tope de páginas sin ninguna fila y con más datos, se falla cerrado
// (503) en lugar de devolver una página vacía que parecería «fin de la colección».
func (s *GoalsStore) ListUserGoals(ctx context.Context, query appGoals.ListStoreQuery) ([]appGoals.GoalRow, string, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, "", appGoals.ErrUnavailable
	}
	if strings.TrimSpace(query.UserID) == "" || query.Limit < 1 || query.Limit > appGoals.MaxListLimit {
		return nil, "", appGoals.ErrUnavailable
	}

	pk := domainDynamo.PrefixUser + query.UserID
	var start map[string]types.AttributeValue
	if query.StartSK != "" {
		start = map[string]types.AttributeValue{
			"PK": stringAttr(pk),
			"SK": stringAttr(query.StartSK),
		}
	}

	recogidos := make([]appGoals.GoalRow, 0, query.Limit+1)
	for pagina := 0; pagina < goalsListMaxPages; pagina++ {
		restantes := query.Limit + 1 - len(recogidos)
		if restantes <= 0 {
			break
		}
		valores := map[string]types.AttributeValue{
			":pk":     stringAttr(pk),
			":prefix": stringAttr(domainDynamo.PrefixGoal),
		}
		input := &dynamodb.QueryInput{
			TableName:              aws.String(s.table),
			KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :prefix)"),
			// Lectura consistente: la lista debe reflejar un alta ya confirmada por otra
			// instancia.
			ConsistentRead:            aws.Bool(true),
			ScanIndexForward:          aws.Bool(true),
			Limit:                     aws.Int32(int32(restantes)),
			ExpressionAttributeValues: valores,
		}
		if query.HasStatus {
			input.FilterExpression = aws.String("#status = :status")
			input.ExpressionAttributeNames = map[string]string{"#status": "status"}
			valores[":status"] = stringAttr(query.Status.String())
		}
		if start != nil {
			input.ExclusiveStartKey = start
		}

		out, err := s.client.Query(ctx, input)
		if err != nil {
			return nil, "", appGoals.ErrUnavailable
		}
		if out == nil {
			break
		}
		for _, item := range out.Items {
			if len(recogidos) == query.Limit+1 {
				break
			}
			goal, errMapeo := goalFromItem(item)
			if errMapeo != nil {
				return nil, "", errMapeo
			}
			if goal == nil {
				continue
			}
			if query.HasStatus && goal.Status != query.Status {
				// Defensa en profundidad: el filtro ya se aplicó en la consulta.
				continue
			}
			recogidos = append(recogidos, appGoals.GoalRow{Goal: *goal, SortKey: stringAttribute(item["SK"])})
		}
		if len(recogidos) >= query.Limit+1 {
			break
		}
		if len(out.LastEvaluatedKey) == 0 {
			start = nil
			break
		}
		start = out.LastEvaluatedKey
	}

	if len(recogidos) == 0 && start != nil {
		// Se agotó el tope de páginas sin una sola fila y quedan datos: no se puede construir
		// un cursor válido, así que se falla cerrado en lugar de aparentar fin de colección.
		return nil, "", appGoals.ErrUnavailable
	}

	hayMas := start != nil
	if len(recogidos) > query.Limit {
		recogidos = recogidos[:query.Limit]
		hayMas = true
	}
	nextSK := ""
	if hayMas && len(recogidos) > 0 {
		nextSK = recogidos[len(recogidos)-1].SortKey
	}
	return recogidos, nextSK, nil
}

// AggregateUserGoals recorre TODA la colección de metas del titular y suma el ahorro de las que
// están `active` (A8 de RFC-016). Nunca usa `Scan`: es una `Query` paginada con filtro de estado
// en la propia consulta.
//
// El tope de páginas e ítems es un fallo cerrado (503): devolver totales parciales como si
// fueran exactos sería peor que no responder.
func (s *GoalsStore) AggregateUserGoals(ctx context.Context, userID string) (appGoals.Totals, error) {
	if s == nil || s.client == nil || s.table == "" {
		return appGoals.Totals{}, appGoals.ErrUnavailable
	}
	if strings.TrimSpace(userID) == "" {
		return appGoals.Totals{}, appGoals.ErrUnavailable
	}

	pk := domainDynamo.PrefixUser + userID
	suma := money.MustNew(0, domainGoal.CurrencyMXN)
	activas := 0
	var start map[string]types.AttributeValue

	for pagina := 0; ; pagina++ {
		if pagina >= goalsAggregateMaxPages {
			return appGoals.Totals{}, appGoals.ErrUnavailable
		}
		input := &dynamodb.QueryInput{
			TableName:              aws.String(s.table),
			KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :prefix)"),
			FilterExpression:       aws.String("#status = :active"),
			ExpressionAttributeNames: map[string]string{
				"#status": "status",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":pk":     stringAttr(pk),
				":prefix": stringAttr(domainDynamo.PrefixGoal),
				":active": stringAttr(domainGoal.StatusActive.String()),
			},
			ConsistentRead:   aws.Bool(true),
			ScanIndexForward: aws.Bool(true),
			Limit:            aws.Int32(goalsAggregatePageSize),
		}
		if start != nil {
			input.ExclusiveStartKey = start
		}

		out, err := s.client.Query(ctx, input)
		if err != nil {
			return appGoals.Totals{}, appGoals.ErrUnavailable
		}
		if out == nil {
			break
		}
		for _, item := range out.Items {
			cents, ok := attributeInt(item["savedAmountCents"])
			if !ok || cents < 0 {
				return appGoals.Totals{}, appGoals.ErrInconsistent
			}
			importe, errImporte := money.New(cents, domainGoal.CurrencyMXN)
			if errImporte != nil {
				return appGoals.Totals{}, appGoals.ErrInconsistent
			}
			suma, errImporte = suma.Add(importe)
			if errImporte != nil {
				return appGoals.Totals{}, appGoals.ErrInconsistent
			}
			activas++
			if activas > goalsAggregateMaxItems {
				return appGoals.Totals{}, appGoals.ErrUnavailable
			}
		}
		if len(out.LastEvaluatedKey) == 0 {
			break
		}
		start = out.LastEvaluatedKey
	}

	return appGoals.Totals{ActiveCount: activas, Saved: suma}, nil
}
