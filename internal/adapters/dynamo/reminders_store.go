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
//
// La SK ordena por vencimiento y sirve de cursor opaco firmado. La idempotencia
// de E065 vive en la MISMA partición para no ampliar la política IAM:
//
//	SK = IDEM#REMINDER#<sha256 de la clave derivada por usuario>
const (
	reminderSKPrefix   = "REMINDER#"
	reminderIdemPrefix = "IDEM#REMINDER#"
	reminderEntity     = "USER_REMINDER"
	reminderIdemEntity = "USER_REMINDER_IDEMPOTENCY"
)

// RemindersAPI es el subconjunto de DynamoDB usado por E064–E067. No expone
// Scan: el compilador impide un barrido global. TransactWriteItems permite la
// creación atómica del recordatorio y su registro de idempotencia.
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

func reminderIdempotencySortKey(keyHash string) string {
	return reminderIdemPrefix + strings.TrimSpace(keyHash)
}

// List consulta únicamente la partición del titular, ordenada por vencimiento.
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
		for _, item := range page.Items {
			if stringAttribute(item["entityType"]) != reminderEntity {
				continue
			}
			reminder, err := reminderDesdeItem(item)
			if err != nil {
				return nil, reminders.ErrUnavailable
			}
			resultado.Items = append(resultado.Items, reminder)
			if len(resultado.Items) >= limite {
				break
			}
		}
		if len(resultado.Items) >= limite {
			if len(page.Items) > 0 {
				last := page.Items[len(page.Items)-1]
				resultado.NextKey = &reminders.Key{PK: pk, SK: stringAttribute(last["SK"])}
			}
			return resultado, nil
		}
		if len(page.LastEvaluatedKey) == 0 {
			return resultado, nil
		}
		input.ExclusiveStartKey = page.LastEvaluatedKey
	}
}

// Get lee un recordatorio propio con consistencia fuerte.
func (s *RemindersStore) Get(ctx context.Context, userID, id string) (*reminders.Reminder, error) {
	if !s.disponible() {
		return nil, reminders.ErrUnavailable
	}
	userID = strings.TrimSpace(userID)
	id = strings.TrimSpace(id)
	if userID == "" || id == "" {
		return nil, reminders.ErrInvalidRequest
	}
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      aws.String(s.table),
		ConsistentRead: aws.Bool(true),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(reminderPartition(userID)),
			"SK": stringAttr(id),
		},
	})
	if err != nil {
		return nil, reminders.ErrUnavailable
	}
	if out == nil || len(out.Item) == 0 || stringAttribute(out.Item["entityType"]) != reminderEntity {
		return nil, nil
	}
	reminder, err := reminderDesdeItem(out.Item)
	if err != nil {
		return nil, err
	}
	return &reminder, nil
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

// Create escribe el recordatorio y su idempotencia en una sola transacción:
// dos instancias con la misma clave no pueden crear dos recursos.
func (s *RemindersStore) Create(ctx context.Context, input reminders.CreateInput) error {
	if !s.disponible() {
		return reminders.ErrUnavailable
	}
	dueAt, err := time.Parse(time.RFC3339, strings.TrimSpace(input.DueAt))
	if err != nil {
		return reminders.ErrInvalidRequest
	}
	pk := reminderPartition(input.UserID)
	sk := reminderSortKey(dueAt, input.ID)
	ahora := input.Now.UTC()
	if ahora.IsZero() {
		ahora = time.Now().UTC()
	}
	item := map[string]types.AttributeValue{
		"PK":         stringAttr(pk),
		"SK":         stringAttr(sk),
		"entityType": stringAttr(reminderEntity),
		"reminderID": stringAttr(input.ID),
		"userID":     stringAttr(strings.TrimSpace(input.UserID)),
		"title":      stringAttr(input.Title),
		"dueAt":      stringAttr(input.DueAt),
		"timezone":   stringAttr(input.Timezone),
		"active":     boolAttr(true),
		"deleted":    boolAttr(false),
		"version":    numberAttr(1),
		"frequency":  stringAttr(reminders.FrecuenciaString(input.Frequency)),
		"createdAt":  stringAttr(ahora.Format(time.RFC3339Nano)),
		"updatedAt":  stringAttr(ahora.Format(time.RFC3339Nano)),
	}
	idem := map[string]types.AttributeValue{
		"PK":          stringAttr(pk),
		"SK":          stringAttr(reminderIdempotencySortKey(input.KeyHash)),
		"entityType":  stringAttr(reminderIdemEntity),
		"reminderID":  stringAttr(input.ID),
		"payloadHash": stringAttr(input.PayloadHash),
		"createdAt":   stringAttr(ahora.Format(time.RFC3339Nano)),
	}
	_, err = s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{Put: &types.Put{TableName: aws.String(s.table), Item: item, ConditionExpression: aws.String("attribute_not_exists(PK)")}},
			{Put: &types.Put{TableName: aws.String(s.table), Item: idem, ConditionExpression: aws.String("attribute_not_exists(PK)")}},
		},
	})
	if err == nil {
		return nil
	}
	var condicional *types.ConditionalCheckFailedException
	if errors.As(err, &condicional) {
		return reminders.ErrConflict
	}
	return reminders.ErrUnavailable
}

// Update aplica un CAS por versión. `frequency` con valor nil elimina el
// atributo (recurrencia cancelada); ausente lo preserva.
func (s *RemindersStore) Update(ctx context.Context, input reminders.UpdateInput) (*reminders.Reminder, error) {
	if !s.disponible() {
		return nil, reminders.ErrUnavailable
	}
	names := map[string]string{
		"#pk":      "PK",
		"#version": "version",
		"#borrado": "deleted",
		"#updated": "updatedAt",
	}
	values := map[string]types.AttributeValue{
		":expected": numberAttr(input.ExpectedVersion),
		":falso":    boolAttr(false),
		":updated":  stringAttr(input.Now.UTC().Format(time.RFC3339Nano)),
	}
	sets := []string{"#version = #version + :uno", "#updated = :updated"}
	values[":uno"] = numberAttr(1)
	removals := []string{}
	if input.Title != nil {
		names["#title"] = "title"
		values[":title"] = stringAttr(*input.Title)
		sets = append(sets, "#title = :title")
	}
	if input.DueAt != nil {
		dueAt, err := time.Parse(time.RFC3339, strings.TrimSpace(*input.DueAt))
		if err != nil {
			return nil, reminders.ErrInvalidRequest
		}
		names["#dueAt"] = "dueAt"
		names["#sk"] = "SK"
		values[":dueAt"] = stringAttr(*input.DueAt)
		values[":sk"] = stringAttr(reminderSortKey(dueAt, input.ID))
		sets = append(sets, "#dueAt = :dueAt", "#sk = :sk")
	}
	if input.Active != nil {
		names["#active"] = "active"
		values[":active"] = boolAttr(*input.Active)
		sets = append(sets, "#active = :active")
	}
	if input.Frequency.Set {
		names["#frequency"] = "frequency"
		if input.Frequency.Value == nil {
			removals = append(removals, "#frequency")
		} else {
			values[":frequency"] = stringAttr(string(*input.Frequency.Value))
			sets = append(sets, "#frequency = :frequency")
		}
	}
	expression := "SET " + strings.Join(sets, ", ")
	if len(removals) > 0 {
		expression += " REMOVE " + strings.Join(removals, ", ")
	}
	_, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(reminderPartition(input.UserID)),
			"SK": stringAttr(input.ID),
		},
		UpdateExpression:          aws.String(expression),
		ConditionExpression:       aws.String("attribute_exists(#pk) AND #version = :expected AND (attribute_not_exists(#borrado) OR #borrado = :falso)"),
		ExpressionAttributeNames:  names,
		ExpressionAttributeValues: values,
	})
	if err != nil {
		var condicional *types.ConditionalCheckFailedException
		if errors.As(err, &condicional) {
			return nil, reminders.ErrConflict
		}
		return nil, reminders.ErrUnavailable
	}
	return s.Get(ctx, input.UserID, input.ID)
}

// Delete marca el registro como borrado y cancela disparos futuros. Es
// idempotente: si ya estaba borrado se devuelve ErrNotFound y el servicio lo
// traduce a 204.
func (s *RemindersStore) Delete(ctx context.Context, userID, id string, expectedVersion int64) error {
	if !s.disponible() {
		return reminders.ErrUnavailable
	}
	_, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(reminderPartition(userID)),
			"SK": stringAttr(id),
		},
		UpdateExpression:    aws.String("SET #deleted = :verdadero, #active = :falso, #version = #version + :uno, #updated = :updated"),
		ConditionExpression: aws.String("attribute_exists(#pk) AND #version = :expected AND (attribute_not_exists(#deleted) OR #deleted = :falso)"),
		ExpressionAttributeNames: map[string]string{
			"#pk":      "PK",
			"#deleted": "deleted",
			"#active":  "active",
			"#version": "version",
			"#updated": "updatedAt",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":verdadero": boolAttr(true),
			":falso":     boolAttr(false),
			":uno":       numberAttr(1),
			":expected":  numberAttr(expectedVersion),
			":updated":   stringAttr(time.Now().UTC().Format(time.RFC3339Nano)),
		},
	})
	if err == nil {
		return nil
	}
	var condicional *types.ConditionalCheckFailedException
	if !errors.As(err, &condicional) {
		return reminders.ErrUnavailable
	}
	actual, errLeer := s.Get(ctx, userID, id)
	if errLeer != nil {
		return errLeer
	}
	if actual == nil || actual.Deleted {
		return reminders.ErrNotFound
	}
	return reminders.ErrConflict
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
