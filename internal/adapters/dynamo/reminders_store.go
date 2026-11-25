package dynamo

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/nexumpr/Nexum-Backend/internal/application/reminders"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
)

// Patrón físico de E064–E067 (tabla única, sin GSI nuevo y sin Scan):
//
//	PK = USER#<sub>
//	SK = REMINDER#<RFC3339 UTC ordenable>#<reminderId>
//	SK = REMINDERPTR#<reminderId>   -> puntero con la SK completa y la versión
//
// La SK del recordatorio ordena por vencimiento y sirve de cursor opaco firmado.
// Como el identificador no forma parte de la clave de consulta, el puntero es
// obligatorio para resolver `Get`/`Update`/`Delete` con un `GetItem` exacto:
// así se evita un `Scan` y se mantiene el aislamiento por `USER#<sub>`.
//
// La idempotencia de E065 vive en la MISMA partición para no ampliar la
// política IAM:
//
//	SK = IDEM#REMINDER#<sha256 de la clave derivada por usuario>
const (
	reminderSKPrefix   = "REMINDER#"
	reminderPtrPrefix  = "REMINDERPTR#"
	reminderIdemPrefix = "IDEM#REMINDER#"
	reminderEntity     = "USER_REMINDER"
	reminderPtrEntity  = "USER_REMINDER_POINTER"
	reminderIdemEntity = "USER_REMINDER_IDEMPOTENCY"

	// Condición de CAS sobre el recordatorio vigente: existe, conserva la
	// versión esperada y no está borrado lógicamente.
	condicionReminderVigente = "attribute_exists(PK) AND #version = :expected AND (attribute_not_exists(#deleted) OR #deleted = :falso)"
	// Condición del puntero: además de la versión, la SK apuntada no cambió.
	condicionPunteroVigente = "attribute_exists(PK) AND #version = :expected AND #reminderSK = :sk"
	condicionAltaNueva      = "attribute_not_exists(PK)"
)

// RemindersAPI es el subconjunto de DynamoDB usado por E064–E067. No expone
// Scan: el compilador impide un barrido global. TransactWriteItems permite la
// creación atómica del recordatorio, su puntero y su registro de idempotencia.
type RemindersAPI interface {
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
	TransactWriteItems(ctx context.Context, params *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error)
}

// RemindersStore implementa reminders.Store sobre la tabla única existente.
type RemindersStore struct {
	client RemindersAPI
	table  string
}

// NewRemindersStore construye el adaptador. Sin cliente o tabla, todas las
// operaciones fallan cerrado con ErrUnavailable.
func NewRemindersStore(client RemindersAPI, table string) *RemindersStore {
	return &RemindersStore{client: client, table: strings.TrimSpace(table)}
}

func (s *RemindersStore) disponible() bool {
	return s != nil && s.client != nil && s.table != ""
}

func reminderPartition(userID string) string {
	return domainDynamo.PrefixUser + strings.TrimSpace(userID)
}

func reminderSortKey(dueAt time.Time, id string) string {
	return fmt.Sprintf("%s%s#%s", reminderSKPrefix, dueAt.UTC().Format("2006-01-02T15:04:05.000Z"), strings.TrimSpace(id))
}

func reminderPointerSortKey(id string) string {
	return reminderPtrPrefix + strings.TrimSpace(id)
}

func reminderIdempotencySortKey(keyHash string) string {
	return reminderIdemPrefix + strings.TrimSpace(keyHash)
}

// reminderPointer materializa la fila `REMINDERPTR#<id>`.
type reminderPointer struct {
	SortKey string
	Version int64
	Deleted bool
}

// pointerFor lee el puntero propio con consistencia fuerte. Devuelve nil cuando
// no existe, de modo que el recurso es indistinguible de uno ajeno.
func (s *RemindersStore) pointerFor(ctx context.Context, userID, id string) (*reminderPointer, error) {
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      aws.String(s.table),
		ConsistentRead: aws.Bool(true),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(reminderPartition(userID)),
			"SK": stringAttr(reminderPointerSortKey(id)),
		},
	})
	if err != nil {
		return nil, reminders.ErrUnavailable
	}
	if out == nil || len(out.Item) == 0 || stringAttribute(out.Item["entityType"]) != reminderPtrEntity {
		return nil, nil
	}
	sortKey := strings.TrimSpace(stringAttribute(out.Item["reminderSK"]))
	if sortKey == "" {
		return nil, nil
	}
	version, err := strconv.ParseInt(stringAttribute(out.Item["version"]), 10, 64)
	if err != nil || version < 1 {
		// Un puntero corrupto no puede resolverse sin Scan: falla cerrado.
		return nil, reminders.ErrUnavailable
	}
	return &reminderPointer{
		SortKey: sortKey,
		Version: version,
		Deleted: boolAttribute(out.Item["deleted"]),
	}, nil
}

// reminderItemAt lee el ítem exacto apuntado. Devuelve el mapa crudo (para
// conservar atributos como createdAt al mover la SK) y la entidad parseada.
func (s *RemindersStore) reminderItemAt(ctx context.Context, userID, sortKey string) (map[string]types.AttributeValue, *reminders.Reminder, error) {
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      aws.String(s.table),
		ConsistentRead: aws.Bool(true),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(reminderPartition(userID)),
			"SK": stringAttr(sortKey),
		},
	})
	if err != nil {
		return nil, nil, reminders.ErrUnavailable
	}
	if out == nil || len(out.Item) == 0 || stringAttribute(out.Item["entityType"]) != reminderEntity {
		return nil, nil, nil
	}
	reminder, err := reminderDesdeItem(out.Item)
	if err != nil {
		return nil, nil, err
	}
	return out.Item, &reminder, nil
}

// List consulta únicamente la partición del titular, ordenada por vencimiento.
// `NextKey` es siempre la SK del último elemento realmente devuelto, nunca la
// del último ítem crudo de la página: así el cursor no salta elementos cuando
// la página termina en un ítem filtrado o ajeno al tipo.
func (s *RemindersStore) List(ctx context.Context, query reminders.ListQuery) (*reminders.ListResult, error) {
	if !s.disponible() {
		return nil, reminders.ErrUnavailable
	}
	userID := strings.TrimSpace(query.UserID)
	if userID == "" {
		return nil, reminders.ErrUnauthorized
	}
	pk := reminderPartition(userID)
	if query.After != nil && query.After.PK != pk {
		return nil, reminders.ErrInvalidRequest
	}
	limite := query.Limit
	if limite <= 0 {
		limite = reminders.DefaultLimit
	}

	expressionNames := map[string]string{
		"#pk":      "PK",
		"#sk":      "SK",
		"#borrado": "deleted",
	}
	expressionValues := map[string]types.AttributeValue{
		":pk":      stringAttr(pk),
		":prefijo": stringAttr(reminderSKPrefix),
		":falso":   boolAttr(false),
	}
	parts := []string{"#borrado = :falso"}
	if query.Active != nil {
		parts = append(parts, "#activo = :activo")
		expressionNames["#activo"] = "active"
		expressionValues[":activo"] = boolAttr(*query.Active)
	}

	input := &dynamodb.QueryInput{
		TableName:                 aws.String(s.table),
		ConsistentRead:            aws.Bool(true),
		KeyConditionExpression:    aws.String("#pk = :pk AND begins_with(#sk, :prefijo)"),
		FilterExpression:          aws.String(strings.Join(parts, " AND ")),
		ExpressionAttributeNames:  expressionNames,
		ExpressionAttributeValues: expressionValues,
	}
	if query.After != nil {
		input.ExclusiveStartKey = map[string]types.AttributeValue{
			"PK": stringAttr(query.After.PK),
			"SK": stringAttr(query.After.SK),
		}
	}

	resultado := &reminders.ListResult{Items: []reminders.Reminder{}}
	for {
		page, err := s.client.Query(ctx, input)
		if err != nil {
			return nil, reminders.ErrUnavailable
		}
		for i, item := range page.Items {
			if stringAttribute(item["entityType"]) != reminderEntity {
				continue
			}
			reminder, err := reminderDesdeItem(item)
			if err != nil {
				return nil, reminders.ErrUnavailable
			}
			resultado.Items = append(resultado.Items, reminder)
			if len(resultado.Items) < limite {
				continue
			}
			// Sólo se emite cursor si de verdad quedan ítems por evaluar; el
			// cursor apunta al último recordatorio entregado.
			hayResto := i+1 < len(page.Items) || len(page.LastEvaluatedKey) > 0
			if hayResto {
				resultado.NextKey = &reminders.Key{PK: pk, SK: stringAttribute(item["SK"])}
			}
			return resultado, nil
		}
		if len(page.LastEvaluatedKey) == 0 {
			return resultado, nil
		}
		input.ExclusiveStartKey = page.LastEvaluatedKey
	}
}

// Get resuelve el puntero y lee el recordatorio propio con consistencia fuerte.
func (s *RemindersStore) Get(ctx context.Context, userID, id string) (*reminders.Reminder, error) {
	if !s.disponible() {
		return nil, reminders.ErrUnavailable
	}
	userID = strings.TrimSpace(userID)
	id = strings.TrimSpace(id)
	if userID == "" || id == "" {
		return nil, reminders.ErrInvalidRequest
	}
	pointer, err := s.pointerFor(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if pointer == nil {
		return nil, nil
	}
	_, reminder, err := s.reminderItemAt(ctx, userID, pointer.SortKey)
	if err != nil {
		return nil, err
	}
	return reminder, nil
}

// FindIdempotency devuelve el registro previo de E065, si existe.
func (s *RemindersStore) FindIdempotency(ctx context.Context, userID, keyHash string) (*reminders.IdempotencyRecord, error) {
	if !s.disponible() {
		return nil, reminders.ErrUnavailable
	}
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      aws.String(s.table),
		ConsistentRead: aws.Bool(true),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(reminderPartition(userID)),
			"SK": stringAttr(reminderIdempotencySortKey(keyHash)),
		},
	})
	if err != nil {
		return nil, reminders.ErrUnavailable
	}
	if out == nil || len(out.Item) == 0 || stringAttribute(out.Item["entityType"]) != reminderIdemEntity {
		return nil, nil
	}
	return &reminders.IdempotencyRecord{
		ReminderID:  stringAttribute(out.Item["reminderID"]),
		PayloadHash: stringAttribute(out.Item["payloadHash"]),
	}, nil
}

// Create escribe recordatorio, puntero e idempotencia en una sola transacción:
// dos instancias con la misma clave no pueden crear dos recursos y ningún
// registro queda sin su puntero de resolución.
func (s *RemindersStore) Create(ctx context.Context, input reminders.CreateInput) error {
	if !s.disponible() {
		return reminders.ErrUnavailable
	}
	dueAt, err := time.Parse(time.RFC3339, strings.TrimSpace(input.DueAt))
	if err != nil {
		return reminders.ErrInvalidRequest
	}
	dueAtTexto := dueAt.UTC().Truncate(time.Second).Format(time.RFC3339)
	userID := strings.TrimSpace(input.UserID)
	id := strings.TrimSpace(input.ID)
	if userID == "" || id == "" {
		return reminders.ErrInvalidRequest
	}
	pk := reminderPartition(userID)
	sk := reminderSortKey(dueAt, id)
	ahora := input.Now.UTC()
	if ahora.IsZero() {
		ahora = time.Now().UTC()
	}
	item := map[string]types.AttributeValue{
		"PK":         stringAttr(pk),
		"SK":         stringAttr(sk),
		"entityType": stringAttr(reminderEntity),
		"reminderID": stringAttr(id),
		"userID":     stringAttr(userID),
		"title":      stringAttr(input.Title),
		"dueAt":      stringAttr(dueAtTexto),
		"timezone":   stringAttr(input.Timezone),
		"active":     boolAttr(true),
		"deleted":    boolAttr(false),
		"version":    numberAttr(1),
		"frequency":  stringAttr(reminders.FrecuenciaString(input.Frequency)),
		"createdAt":  stringAttr(ahora.Format(time.RFC3339Nano)),
		"updatedAt":  stringAttr(ahora.Format(time.RFC3339Nano)),
	}
	puntero := map[string]types.AttributeValue{
		"PK":         stringAttr(pk),
		"SK":         stringAttr(reminderPointerSortKey(id)),
		"entityType": stringAttr(reminderPtrEntity),
		"reminderID": stringAttr(id),
		"reminderSK": stringAttr(sk),
		"deleted":    boolAttr(false),
		"version":    numberAttr(1),
		"createdAt":  stringAttr(ahora.Format(time.RFC3339Nano)),
		"updatedAt":  stringAttr(ahora.Format(time.RFC3339Nano)),
	}
	idem := map[string]types.AttributeValue{
		"PK":          stringAttr(pk),
		"SK":          stringAttr(reminderIdempotencySortKey(input.KeyHash)),
		"entityType":  stringAttr(reminderIdemEntity),
		"reminderID":  stringAttr(id),
		"payloadHash": stringAttr(input.PayloadHash),
		"createdAt":   stringAttr(ahora.Format(time.RFC3339Nano)),
	}
	_, err = s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{Put: &types.Put{TableName: aws.String(s.table), Item: item, ConditionExpression: aws.String(condicionAltaNueva)}},
			{Put: &types.Put{TableName: aws.String(s.table), Item: puntero, ConditionExpression: aws.String(condicionAltaNueva)}},
			{Put: &types.Put{TableName: aws.String(s.table), Item: idem, ConditionExpression: aws.String(condicionAltaNueva)}},
		},
	})
	if err == nil {
		return nil
	}
	if esCancelacionPorCondicion(err) {
		return reminders.ErrConflict
	}
	return reminders.ErrUnavailable
}

// Update aplica un CAS por versión. `frequency` con valor nil elimina el
// atributo (recurrencia cancelada); ausente lo preserva. Si cambia `dueAt`, la
// SK del recordatorio se mueve en la MISMA transacción que actualiza el
// puntero, de modo que nunca queda un puntero apuntando a una SK eliminada.
func (s *RemindersStore) Update(ctx context.Context, input reminders.UpdateInput) (*reminders.Reminder, error) {
	if !s.disponible() {
		return nil, reminders.ErrUnavailable
	}
	userID := strings.TrimSpace(input.UserID)
	id := strings.TrimSpace(input.ID)
	if userID == "" || id == "" || input.ExpectedVersion < 1 {
		return nil, reminders.ErrInvalidRequest
	}

	pointer, err := s.pointerFor(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if pointer == nil || pointer.Deleted {
		return nil, reminders.ErrNotFound
	}
	raw, actual, err := s.reminderItemAt(ctx, userID, pointer.SortKey)
	if err != nil {
		return nil, err
	}
	if actual == nil || actual.Deleted {
		return nil, reminders.ErrNotFound
	}
	if actual.Version != input.ExpectedVersion || pointer.Version != input.ExpectedVersion {
		return nil, reminders.ErrConflict
	}

	ahora := input.Now.UTC()
	if ahora.IsZero() {
		ahora = time.Now().UTC()
	}
	updatedAt := ahora.Format(time.RFC3339Nano)
	versionSiguiente := input.ExpectedVersion + 1

	nuevaSK := pointer.SortKey
	if input.DueAt != nil {
		dueAt, err := time.Parse(time.RFC3339, strings.TrimSpace(*input.DueAt))
		if err != nil {
			return nil, reminders.ErrInvalidRequest
		}
		nuevaSK = reminderSortKey(dueAt, id)
	}

	item := make(map[string]types.AttributeValue, len(raw)+2)
	for clave, valor := range raw {
		item[clave] = valor
	}
	item["SK"] = stringAttr(nuevaSK)
	item["version"] = numberAttr(versionSiguiente)
	item["updatedAt"] = stringAttr(updatedAt)
	if input.Title != nil {
		item["title"] = stringAttr(*input.Title)
	}
	if input.DueAt != nil {
		dueAt, _ := time.Parse(time.RFC3339, strings.TrimSpace(*input.DueAt))
		item["dueAt"] = stringAttr(dueAt.UTC().Truncate(time.Second).Format(time.RFC3339))
	}
	if input.Active != nil {
		item["active"] = boolAttr(*input.Active)
	}
	if input.Frequency.Set {
		if input.Frequency.Value == nil {
			delete(item, "frequency")
		} else {
			item["frequency"] = stringAttr(string(*input.Frequency.Value))
		}
	}

	acciones := make([]types.TransactWriteItem, 0, 3)
	if nuevaSK == pointer.SortKey {
		acciones = append(acciones, types.TransactWriteItem{Put: &types.Put{
			TableName:                 aws.String(s.table),
			Item:                      item,
			ConditionExpression:       aws.String(condicionReminderVigente),
			ExpressionAttributeNames:  map[string]string{"#version": "version", "#deleted": "deleted"},
			ExpressionAttributeValues: map[string]types.AttributeValue{":expected": numberAttr(input.ExpectedVersion), ":falso": boolAttr(false)},
		}})
	} else {
		acciones = append(acciones,
			types.TransactWriteItem{Put: &types.Put{
				TableName:           aws.String(s.table),
				Item:                item,
				ConditionExpression: aws.String(condicionAltaNueva),
			}},
			types.TransactWriteItem{Delete: &types.Delete{
				TableName:                 aws.String(s.table),
				Key:                       map[string]types.AttributeValue{"PK": stringAttr(reminderPartition(userID)), "SK": stringAttr(pointer.SortKey)},
				ConditionExpression:       aws.String(condicionReminderVigente),
				ExpressionAttributeNames:  map[string]string{"#version": "version", "#deleted": "deleted"},
				ExpressionAttributeValues: map[string]types.AttributeValue{":expected": numberAttr(input.ExpectedVersion), ":falso": boolAttr(false)},
			}},
		)
	}
	acciones = append(acciones, types.TransactWriteItem{Update: &types.Update{
		TableName:                 aws.String(s.table),
		Key:                       map[string]types.AttributeValue{"PK": stringAttr(reminderPartition(userID)), "SK": stringAttr(reminderPointerSortKey(id))},
		UpdateExpression:          aws.String("SET #version = :next, #updated = :updatedAt, #reminderSK = :nuevaSK"),
		ConditionExpression:       aws.String(condicionPunteroVigente),
		ExpressionAttributeNames:  map[string]string{"#version": "version", "#updated": "updatedAt", "#reminderSK": "reminderSK"},
		ExpressionAttributeValues: map[string]types.AttributeValue{":expected": numberAttr(input.ExpectedVersion), ":next": numberAttr(versionSiguiente), ":updatedAt": stringAttr(updatedAt), ":sk": stringAttr(pointer.SortKey), ":nuevaSK": stringAttr(nuevaSK)},
	}})

	if _, err := s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: acciones}); err != nil {
		if esCancelacionPorCondicion(err) {
			return nil, reminders.ErrConflict
		}
		return nil, reminders.ErrUnavailable
	}
	return s.Get(ctx, userID, id)
}

// Delete marca el registro como borrado y cancela disparos futuros en la misma
// transacción que marca el puntero. Es idempotente: si ya estaba borrado se
// devuelve ErrNotFound y el servicio lo traduce a 204.
func (s *RemindersStore) Delete(ctx context.Context, userID, id string, expectedVersion int64) error {
	if !s.disponible() {
		return reminders.ErrUnavailable
	}
	userID = strings.TrimSpace(userID)
	id = strings.TrimSpace(id)
	if userID == "" || id == "" || expectedVersion < 1 {
		return reminders.ErrInvalidRequest
	}
	pointer, err := s.pointerFor(ctx, userID, id)
	if err != nil {
		return err
	}
	if pointer == nil {
		return reminders.ErrNotFound
	}
	_, actual, err := s.reminderItemAt(ctx, userID, pointer.SortKey)
	if err != nil {
		return err
	}
	if actual == nil || actual.Deleted {
		return reminders.ErrNotFound
	}
	if actual.Version != expectedVersion || pointer.Version != expectedVersion {
		return reminders.ErrConflict
	}
	ahora := time.Now().UTC()
	updatedAt := ahora.Format(time.RFC3339Nano)
	versionSiguiente := expectedVersion + 1
	acciones := []types.TransactWriteItem{
		{Update: &types.Update{
			TableName:                 aws.String(s.table),
			Key:                       map[string]types.AttributeValue{"PK": stringAttr(reminderPartition(userID)), "SK": stringAttr(pointer.SortKey)},
			UpdateExpression:          aws.String("SET #deleted = :verdadero, #active = :falso, #version = :next, #updated = :updatedAt"),
			ConditionExpression:       aws.String(condicionReminderVigente),
			ExpressionAttributeNames:  map[string]string{"#deleted": "deleted", "#active": "active", "#version": "version", "#updated": "updatedAt"},
			ExpressionAttributeValues: map[string]types.AttributeValue{":verdadero": boolAttr(true), ":falso": boolAttr(false), ":expected": numberAttr(expectedVersion), ":next": numberAttr(versionSiguiente), ":updatedAt": stringAttr(updatedAt)},
		}},
		{Update: &types.Update{
			TableName:                 aws.String(s.table),
			Key:                       map[string]types.AttributeValue{"PK": stringAttr(reminderPartition(userID)), "SK": stringAttr(reminderPointerSortKey(id))},
			UpdateExpression:          aws.String("SET #deleted = :verdadero, #version = :next, #updated = :updatedAt"),
			ConditionExpression:       aws.String(condicionPunteroVigente),
			ExpressionAttributeNames:  map[string]string{"#deleted": "deleted", "#version": "version", "#updated": "updatedAt", "#reminderSK": "reminderSK"},
			ExpressionAttributeValues: map[string]types.AttributeValue{":verdadero": boolAttr(true), ":expected": numberAttr(expectedVersion), ":next": numberAttr(versionSiguiente), ":updatedAt": stringAttr(updatedAt), ":sk": stringAttr(pointer.SortKey)},
		}},
	}
	if _, err := s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: acciones}); err != nil {
		if !esCancelacionPorCondicion(err) {
			return reminders.ErrUnavailable
		}
		fresco, errLeer := s.Get(ctx, userID, id)
		if errLeer != nil {
			return errLeer
		}
		if fresco == nil || fresco.Deleted {
			return reminders.ErrNotFound
		}
		return reminders.ErrConflict
	}
	return nil
}

// esCancelacionPorCondicion distingue un CAS perdido de un fallo de servicio:
// sólo `ConditionalCheckFailed` cuenta como conflicto lógico.
func esCancelacionPorCondicion(err error) bool {
	var condicional *types.ConditionalCheckFailedException
	if errors.As(err, &condicional) {
		return true
	}
	var cancelada *types.TransactionCanceledException
	if errors.As(err, &cancelada) {
		for _, razon := range cancelada.CancellationReasons {
			if aws.ToString(razon.Code) == "ConditionalCheckFailed" {
				return true
			}
		}
	}
	return false
}

func reminderDesdeItem(item map[string]types.AttributeValue) (reminders.Reminder, error) {
	dueAt, err := time.Parse(time.RFC3339, stringAttribute(item["dueAt"]))
	if err != nil {
		return reminders.Reminder{}, reminders.ErrUnavailable
	}
	version, err := strconv.ParseInt(stringAttribute(item["version"]), 10, 64)
	if err != nil {
		return reminders.Reminder{}, reminders.ErrUnavailable
	}
	reminder := reminders.Reminder{
		ID:       stringAttribute(item["reminderID"]),
		Title:    stringAttribute(item["title"]),
		DueAt:    dueAt,
		Timezone: stringAttribute(item["timezone"]),
		Active:   boolAttribute(item["active"]),
		Version:  version,
		Deleted:  boolAttribute(item["deleted"]),
	}
	if raw := strings.TrimSpace(stringAttribute(item["frequency"])); raw != "" {
		frecuencia, ok := reminders.ParseFrequency(raw)
		if !ok {
			return reminders.Reminder{}, reminders.ErrUnavailable
		}
		reminder.Frequency = &frecuencia
	}
	return reminder, nil
}
