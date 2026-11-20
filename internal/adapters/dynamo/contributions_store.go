package dynamo

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	appContrib "github.com/nexumpr/Nexum-Backend/internal/application/contributions"
	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
)

// ContributionsAPI es el subconjunto del cliente DynamoDB usado por E043–E047. No incluye
// `Scan`: el compilador impide enumerar la tabla.
type ContributionsAPI interface {
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	TransactWriteItems(ctx context.Context, params *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error)
}

const (
	contributionEntityType        = "CONTRIBUTION"
	contributionPointerEntityType = "CONTRIBUTION_LOCATOR"
	contributionPointerPrefix     = "CONTRIBUTIONPTR#"
	contributionPrefix            = "CONTRIBUTION#"
	contributionStatusActive      = appContrib.StatusActive
	contributionStatusReversed    = appContrib.StatusReversed

	// contributionListMaxPages acota las páginas internas de E045 cuando hay filtros. Si se
	// agota con más datos y sin filas, se devuelve la última clave evaluada como cursor.
	contributionListMaxPages = 4
)

// ContributionsStore implementa `contributions.Store` sobre la tabla única.
//
//	aportación  USER#<id> / CONTRIBUTION#<occurredAt>#<id>
//	puntero     USER#<id> / CONTRIBUTIONPTR#<id>   (targetSK)
//	movimiento  USER#<id> / MOVEMENT#<occurredAt>#<id>
//	meta        USER#<id> / GOAL#<id>
//	idem        USER#<id> / IDEM#<operación>#<sha256>
type ContributionsStore struct {
	client ContributionsAPI
	table  string
}

// NewContributionsStore construye el adaptador. Sin cliente o tabla falla cerrado.
func NewContributionsStore(client ContributionsAPI, table string) *ContributionsStore {
	return &ContributionsStore{client: client, table: strings.TrimSpace(table)}
}

// ContributionPointerKeys devuelve la PK/SK del localizador O(1).
func ContributionPointerKeys(userID, contributionID string) (pk, sk string) {
	return domainDynamo.PrefixUser + strings.TrimSpace(userID), contributionPointerPrefix + strings.TrimSpace(contributionID)
}

// contributionKeys devuelve la PK/SK del ítem principal.
func contributionKeys(userID, contributionID string, occurredAt time.Time) (pk, sk string) {
	return domainDynamo.PrefixUser + strings.TrimSpace(userID), contributionPrefix + appMovements.SortableTime(occurredAt) + "#" + strings.TrimSpace(contributionID)
}

func (s *ContributionsStore) LoadIdempotency(ctx context.Context, userID, operation, key string) (*appContrib.IdempotencyRecord, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, appContrib.ErrUnavailable
	}
	pk, sk := domainDynamo.UserIdempotencyKeys(userID, operation, key)
	item, err := s.getItem(ctx, pk, sk)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, nil
	}
	if entidad := stringAttribute(item["entityType"]); entidad != "" && entidad != movementIdempotencyEntityType {
		return nil, nil
	}
	if titular := stringAttribute(item["userId"]); titular != "" && titular != userID {
		return nil, nil
	}
	registro := &appContrib.IdempotencyRecord{
		PayloadHash:  stringAttribute(item["payloadHash"]),
		ResponseBody: []byte(stringAttribute(item["responseBody"])),
		Location:     stringAttribute(item["location"]),
	}
	if estado, ok := attributeInt(item["httpStatus"]); ok {
		registro.HTTPStatus = int(estado)
	}
	if expira, ok := attributeInt(item["expiresAt"]); ok && expira > 0 {
		registro.ExpiresAt = time.Unix(expira, 0).UTC()
	}
	return registro, nil
}

func (s *ContributionsStore) LoadGoal(ctx context.Context, userID, goalID string) (*appContrib.GoalSnapshot, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, appContrib.ErrUnavailable
	}
	pk, sk := domainDynamo.GoalKeys(userID, goalID)
	item, err := s.getItem(ctx, pk, sk)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, nil
	}
	if entidad := stringAttribute(item["entityType"]); entidad != "" && entidad != goalsEntityType {
		return nil, nil
	}
	if titular := stringAttribute(item["userId"]); titular != "" && titular != userID {
		return nil, nil
	}
	target, okTarget := attributeInt(item["targetAmountCents"])
	saved, okSaved := attributeInt(item["savedAmountCents"])
	if !okTarget || !okSaved {
		return nil, appContrib.ErrUnavailable
	}
	version, _ := attributeInt(item["version"])
	active, _ := attributeInt(item["activeContributions"])
	deleted, _ := boolAttributeOK(item["deleted"])
	return &appContrib.GoalSnapshot{
		ID:                  goalID,
		TargetCents:         target,
		SavedCents:          saved,
		Status:              stringAttribute(item["status"]),
		Deleted:             deleted,
		ActiveContributions: active,
		Version:             version,
	}, nil
}

// LoadTimezone lee la zona IANA de las preferencias. Una preferencia ausente o inválida no es
// un error: el servicio aplica el valor por defecto documentado.
func (s *ContributionsStore) LoadTimezone(ctx context.Context, userID string) (string, error) {
	if s == nil || s.client == nil || s.table == "" {
		return "", appContrib.ErrUnavailable
	}
	pk, sk := domainDynamo.PrefixUser+strings.TrimSpace(userID), "PREFERENCES"
	item, err := s.getItem(ctx, pk, sk)
	if err != nil {
		return "", err
	}
	if item == nil {
		return "", nil
	}
	if tz := strings.TrimSpace(stringAttribute(item["timezone"])); tz != "" {
		return tz, nil
	}
	return strings.TrimSpace(stringAttribute(item["timeZone"])), nil
}

func (s *ContributionsStore) CreateContribution(ctx context.Context, write appContrib.CreateWrite) (*appContrib.StoredResponse, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, appContrib.ErrUnavailable
	}
	userID := strings.TrimSpace(write.UserID)
	if userID == "" || strings.TrimSpace(write.Contribution.ID) == "" || write.PayloadHash == "" ||
		len(write.ResponseBody) == 0 || write.Operation == "" {
		return nil, appContrib.ErrUnavailable
	}
	now := write.Now.UTC()
	idemPK, idemSK := domainDynamo.UserIdempotencyKeys(userID, write.Operation, write.IdempotencyKey)
	existing, err := s.LoadIdempotency(ctx, userID, write.Operation, write.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		if !existing.ExpiresAt.IsZero() && now.Before(existing.ExpiresAt) {
			if existing.PayloadHash != write.PayloadHash {
				return nil, appContrib.ErrConflict
			}
			if existing.HTTPStatus < 100 || existing.HTTPStatus > 599 || len(existing.ResponseBody) == 0 {
				return nil, appContrib.ErrInternal
			}
			return &appContrib.StoredResponse{Status: existing.HTTPStatus, Body: existing.ResponseBody, Location: existing.Location, Replayed: true}, nil
		}
	}

	mainPK, mainSK := contributionKeys(userID, write.Contribution.ID, write.Contribution.OccurredAt)
	ptrPK, ptrSK := ContributionPointerKeys(userID, write.Contribution.ID)
	item := contributionItem(write.Contribution, mainPK, mainSK, now)
	pointer := map[string]types.AttributeValue{
		"PK":         stringAttr(ptrPK),
		"SK":         stringAttr(ptrSK),
		"entityType": stringAttr(contributionPointerEntityType),
		"userId":     stringAttr(userID),
		"contributionId": stringAttr(write.Contribution.ID),
		"targetSK":   stringAttr(mainSK),
		"createdAt":  stringAttr(now.Format(time.RFC3339)),
		"updatedAt":  stringAttr(now.Format(time.RFC3339)),
	}
	movementItem, _, movementSK := movementRecord(userID, write.Movement, now, now)
	movementPK, movementPointerSK := MovementPointerKeys(userID, write.Movement.ID)
	movementPointer := map[string]types.AttributeValue{
		"PK":         stringAttr(movementPK),
		"SK":         stringAttr(movementPointerSK),
		"entityType": stringAttr(movementPointerEntityType),
		"userId":     stringAttr(userID),
		"movementId": stringAttr(write.Movement.ID),
		"targetSK":   stringAttr(movementSK),
		"createdAt":  stringAttr(now.Format(time.RFC3339)),
		"updatedAt":  stringAttr(now.Format(time.RFC3339)),
	}
	idem := map[string]types.AttributeValue{
		"PK":           stringAttr(idemPK),
		"SK":           stringAttr(idemSK),
		"entityType":   stringAttr(movementIdempotencyEntityType),
		"userId":       stringAttr(userID),
		"operation":    stringAttr(write.Operation),
		"payloadHash":  stringAttr(write.PayloadHash),
		"httpStatus":   numberAttr(201),
		"responseBody": stringAttr(string(write.ResponseBody)),
		"location":     stringAttr(write.Location),
		"resourceId":   stringAttr(write.Contribution.ID),
		"createdAt":    stringAttr(now.Format(time.RFC3339)),
		"expiresAt":    numberAttr(write.ExpiresAt.UTC().Unix()),
	}

	transact := []types.TransactWriteItem{
		{Put: &types.Put{TableName: aws.String(s.table), Item: item, ConditionExpression: aws.String("attribute_not_exists(PK)")}},
		{Put: &types.Put{TableName: aws.String(s.table), Item: pointer, ConditionExpression: aws.String("attribute_not_exists(PK)")}},
		{Put: &types.Put{TableName: aws.String(s.table), Item: movementItem, ConditionExpression: aws.String("attribute_not_exists(PK)")}},
		{Put: &types.Put{TableName: aws.String(s.table), Item: movementPointer, ConditionExpression: aws.String("attribute_not_exists(PK)")}},
		{Put: &types.Put{TableName: aws.String(s.table), Item: idem, ConditionExpression: aws.String("(attribute_not_exists(PK) OR expiresAt < :now)"), ExpressionAttributeValues: map[string]types.AttributeValue{":now": numberAttr(now.Unix())}}},
	}
	for _, alloc := range write.Contribution.Allocations {
		goal, ok := write.Goals[alloc.GoalID]
		if !ok {
			return nil, appContrib.ErrGoalNotFound
		}
		goalPK, goalSK := domainDynamo.GoalKeys(userID, alloc.GoalID)
		transact = append(transact, types.TransactWriteItem{Update: &types.Update{
			TableName: aws.String(s.table),
			Key:       map[string]types.AttributeValue{"PK": stringAttr(goalPK), "SK": stringAttr(goalSK)},
			UpdateExpression: aws.String("SET #saved = #saved + :alloc, #activeContrib = if_not_exists(#activeContrib, :zero) + :one, #updatedAt = :updatedAt ADD #version :one"),
			ConditionExpression: aws.String("attribute_exists(PK) AND #status = :active AND (attribute_not_exists(#deleted) OR #deleted = :false) AND #saved <= :maxSaved AND #version = :goalVersion"),
			ExpressionAttributeNames: map[string]string{
				"#saved":           "savedAmountCents",
				"#activeContrib":   "activeContributions",
				"#updatedAt":       "updatedAt",
				"#version":         "version",
				"#status":          "status",
				"#deleted":         "deleted",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":alloc":    numberAttr(alloc.AmountCents),
				":zero":     numberAttr(0),
				":one":      numberAttr(1),
				":updatedAt": stringAttr(now.Format(time.RFC3339)),
				":active":   stringAttr(contributionStatusActive),
				":false":    &types.AttributeValueMemberBOOL{Value: false},
				":maxSaved": numberAttr(goal.TargetCents - alloc.AmountCents),
				":goalVersion": numberAttr(goal.Version),
			},
		}})
	}

	_, err = s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: transact})
	if err == nil {
		return &appContrib.StoredResponse{Status: 201, Body: write.ResponseBody, Location: write.Location}, nil
	}
	var cancelada *types.TransactionCanceledException
	if !errors.As(err, &cancelada) {
		return nil, appContrib.ErrUnavailable
	}
	// El registro de idempotencia es la autoridad de la carrera.
	existing, errLectura := s.LoadIdempotency(ctx, userID, write.Operation, write.IdempotencyKey)
	if errLectura != nil {
		return nil, errLectura
	}
	if existing != nil {
		if existing.PayloadHash != write.PayloadHash {
			return nil, appContrib.ErrConflict
		}
		if existing.HTTPStatus < 100 || existing.HTTPStatus > 599 || len(existing.ResponseBody) == 0 {
			return nil, appContrib.ErrInternal
		}
		return &appContrib.StoredResponse{Status: existing.HTTPStatus, Body: existing.ResponseBody, Location: existing.Location, Replayed: true}, nil
	}
	// Identifica la causa real releyendo las metas: no se supone por el índice de la razón.
	for _, alloc := range write.Contribution.Allocations {
		goal, errGoal := s.LoadGoal(ctx, userID, alloc.GoalID)
		if errGoal != nil {
			return nil, errGoal
		}
		if goal == nil {
			return nil, appContrib.ErrGoalNotFound
		}
		if !goal.Active() {
			return nil, appContrib.ErrGoalInactive
		}
		if alloc.AmountCents > goal.RemainingCapacityCents() {
			return nil, appContrib.ErrCapacityExceeded
		}
	}
	if transactionItemFailed(cancelada, 0) || transactionItemFailed(cancelada, 1) ||
		transactionItemFailed(cancelada, 2) || transactionItemFailed(cancelada, 3) {
		return nil, appContrib.ErrIDCollision
	}
	return nil, appContrib.ErrUnavailable
}

// goalMaxSaved ya no se usa: la condición se construye con el objetivo y la versión leídos de
// forma consistente por el servicio. Se conserva la nota para que nadie reintroduzca un tope
// fijo que anularía la comprobación de capacidad.

func (s *ContributionsStore) ListContributions(ctx context.Context, query appContrib.ListQuery, startSK string, limit int) ([]appContrib.Contribution, string, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, "", appContrib.ErrUnavailable
	}
	if limit < 1 {
		return nil, "", appContrib.ErrInvalidRequest
	}
	pk := domainDynamo.PrefixUser + strings.TrimSpace(query.UserID)
	valores := map[string]types.AttributeValue{":pk": stringAttr(pk)}
	nombres := map[string]string{"#sk": "SK"}
	var condiciones []string
	if query.HasFrom || query.HasTo {
		desde := "0000"
		hasta := "9999"
		if query.HasFrom {
			desde = appMovements.SortableTime(query.From)
		}
		if query.HasTo {
			hasta = appMovements.SortableTime(query.To)
		}
		condiciones = append(condiciones, "#sk BETWEEN :from AND :to")
		valores[":from"] = stringAttr(contributionPrefix + desde)
		valores[":to"] = stringAttr(contributionPrefix + hasta + "~")
	} else {
		condiciones = append(condiciones, "begins_with(#sk, :prefix)")
		valores[":prefix"] = stringAttr(contributionPrefix)
	}
	if query.GoalID != "" {
		condiciones = append(condiciones, "contains(goalIds, :goal)")
		valores[":goal"] = stringAttr(query.GoalID)
	}
	if query.Status != "" {
		condiciones = append(condiciones, "#status = :status")
		nombres["#status"] = "status"
		valores[":status"] = stringAttr(query.Status)
	}
	filtro := ""
	if len(condiciones) > 1 {
		filtro = strings.Join(condiciones[1:], " AND ")
	}

	items := make([]appContrib.Contribution, 0, limit)
	lastSK := ""
	for pagina := 0; pagina < contributionListMaxPages && len(items) < limit; pagina++ {
		entrada := &dynamodb.QueryInput{
			TableName:                 aws.String(s.table),
			KeyConditionExpression:    aws.String("PK = :pk AND " + condiciones[0]),
			ExpressionAttributeNames:  nombres,
			ExpressionAttributeValues: valores,
			ScanIndexForward:          aws.Bool(false),
			Limit:                     aws.Int32(int32(limit)),
		}
		if filtro != "" {
			entrada.FilterExpression = aws.String(filtro)
		}
		if startSK != "" {
			entrada.ExclusiveStartKey = map[string]types.AttributeValue{"PK": stringAttr(pk), "SK": stringAttr(startSK)}
		}
		out, err := s.client.Query(ctx, entrada)
		if err != nil {
			return nil, "", appContrib.ErrUnavailable
		}
		consumidos := 0
		for _, item := range out.Items {
			c, ok := contributionFromItem(item, query.UserID)
			if !ok {
				consumidos++
				continue
			}
			items = append(items, c)
			lastSK = stringAttribute(item["SK"])
			consumidos++
			if len(items) >= limit {
				break
			}
		}
		if len(items) >= limit {
			if consumidos < len(out.Items) || len(out.LastEvaluatedKey) > 0 {
				return items, lastSK, nil
			}
			return items, "", nil
		}
		if len(out.LastEvaluatedKey) == 0 {
			return items, "", nil
		}
		startSK = stringAttribute(out.LastEvaluatedKey["SK"])
	}
	if len(items) > 0 {
		return items, lastSK, nil
	}
	return items, startSK, nil
}

func (s *ContributionsStore) LoadContribution(ctx context.Context, userID, contributionID string) (*appContrib.Contribution, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, appContrib.ErrUnavailable
	}
	ptrPK, ptrSK := ContributionPointerKeys(userID, contributionID)
	pointer, err := s.getItem(ctx, ptrPK, ptrSK)
	if err != nil {
		return nil, err
	}
	if pointer == nil {
		return nil, nil
	}
	if entidad := stringAttribute(pointer["entityType"]); entidad != "" && entidad != contributionPointerEntityType {
		return nil, nil
	}
	targetSK := stringAttribute(pointer["targetSK"])
	if targetSK == "" {
		return nil, nil
	}
	item, err := s.getItem(ctx, ptrPK, targetSK)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, nil
	}
	c, ok := contributionFromItem(item, userID)
	if !ok {
		return nil, nil
	}
	return &c, nil
}

func (s *ContributionsStore) ReverseContribution(ctx context.Context, write appContrib.ReverseWrite) (*appContrib.StoredResponse, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, appContrib.ErrUnavailable
	}
	userID := strings.TrimSpace(write.UserID)
	prev := write.Previous
	next := write.Next
	if userID == "" || prev.ID == "" || next.ID != prev.ID {
		return nil, appContrib.ErrUnavailable
	}
	now := write.Now.UTC()
	idemPK, idemSK := domainDynamo.UserIdempotencyKeys(userID, write.Operation, write.IdempotencyKey)
	existing, err := s.LoadIdempotency(ctx, userID, write.Operation, write.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		if !existing.ExpiresAt.IsZero() && now.Before(existing.ExpiresAt) {
			if existing.PayloadHash != write.PayloadHash {
				return nil, appContrib.ErrConflict
			}
			if existing.HTTPStatus < 100 || existing.HTTPStatus > 599 || len(existing.ResponseBody) == 0 {
				return nil, appContrib.ErrInternal
			}
			return &appContrib.StoredResponse{Status: existing.HTTPStatus, Body: existing.ResponseBody, Location: existing.Location, Replayed: true}, nil
		}
	}
	pk, sk := contributionKeys(userID, prev.ID, prev.OccurredAt)
	transact := []types.TransactWriteItem{{Update: &types.Update{
		TableName: aws.String(s.table),
		Key:       map[string]types.AttributeValue{"PK": stringAttr(pk), "SK": stringAttr(sk)},
		UpdateExpression: aws.String("SET #status = :reversed, #reversedAt = :now, #reverseReason = :reason, #updatedAt = :now ADD #version :one"),
		ConditionExpression: aws.String("#status = :active AND #version = :expected"),
		ExpressionAttributeNames: map[string]string{
			"#status":        "status",
			"#reversedAt":    "reversedAt",
			"#reverseReason": "reverseReason",
			"#updatedAt":     "updatedAt",
			"#version":       "version",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":reversed": stringAttr(contributionStatusReversed),
			":now":      stringAttr(now.Format(time.RFC3339)),
			":reason":   stringAttr(write.Next.ReverseReason),
			":active":   stringAttr(contributionStatusActive),
			":expected": numberAttr(prev.Version),
		},
	}}}
	for _, alloc := range prev.Allocations {
		goalPK, goalSK := domainDynamo.GoalKeys(userID, alloc.GoalID)
		transact = append(transact, types.TransactWriteItem{Update: &types.Update{
			TableName: aws.String(s.table),
			Key:       map[string]types.AttributeValue{"PK": stringAttr(goalPK), "SK": stringAttr(goalSK)},
			UpdateExpression: aws.String("SET #saved = #saved - :alloc, #activeContrib = #activeContrib - :one, #updatedAt = :now ADD #version :one"),
			ConditionExpression: aws.String("attribute_exists(PK) AND #status = :active AND (attribute_not_exists(#deleted) OR #deleted = :false) AND #saved >= :alloc AND #activeContrib >= :one"),
			ExpressionAttributeNames: map[string]string{
				"#saved":         "savedAmountCents",
				"#activeContrib": "activeContributions",
				"#updatedAt":     "updatedAt",
				"#version":       "version",
				"#status":        "status",
				"#deleted":       "deleted",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":alloc": numberAttr(alloc.AmountCents),
				":one":   numberAttr(1),
				":now":   stringAttr(now.Format(time.RFC3339)),
				":active": stringAttr(contributionStatusActive),
				":false":  &types.AttributeValueMemberBOOL{Value: false},
			},
		}})
	}
	transact = append(transact, types.TransactWriteItem{Put: &types.Put{
		TableName: aws.String(s.table),
		Item: map[string]types.AttributeValue{
			"PK":           stringAttr(idemPK),
			"SK":           stringAttr(idemSK),
			"entityType":   stringAttr(movementIdempotencyEntityType),
			"userId":       stringAttr(userID),
			"operation":    stringAttr(write.Operation),
			"payloadHash":  stringAttr(write.PayloadHash),
			"httpStatus":   numberAttr(200),
			"responseBody": stringAttr(string(write.ResponseBody)),
			"location":     stringAttr(write.Location),
			"resourceId":   stringAttr(prev.ID),
			"createdAt":    stringAttr(now.Format(time.RFC3339)),
			"expiresAt":    numberAttr(write.ExpiresAt.UTC().Unix()),
		},
		ConditionExpression: aws.String("(attribute_not_exists(PK) OR expiresAt < :now)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{":now": numberAttr(now.Unix())},
	}})

	_, err = s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: transact})
	if err == nil {
		return &appContrib.StoredResponse{Status: 200, Body: write.ResponseBody, Location: write.Location}, nil
	}
	var cancelada *types.TransactionCanceledException
	if !errors.As(err, &cancelada) {
		return nil, appContrib.ErrUnavailable
	}
	existing, errLectura := s.LoadIdempotency(ctx, userID, write.Operation, write.IdempotencyKey)
	if errLectura != nil {
		return nil, errLectura
	}
	if existing != nil {
		if existing.PayloadHash != write.PayloadHash {
			return nil, appContrib.ErrConflict
		}
		if existing.HTTPStatus < 100 || existing.HTTPStatus > 599 || len(existing.ResponseBody) == 0 {
			return nil, appContrib.ErrInternal
		}
		return &appContrib.StoredResponse{Status: existing.HTTPStatus, Body: existing.ResponseBody, Location: existing.Location, Replayed: true}, nil
	}
	actual, errCarga := s.LoadContribution(ctx, userID, prev.ID)
	if errCarga != nil {
		return nil, errCarga
	}
	if actual == nil {
		return nil, appContrib.ErrNotFound
	}
	if actual.Status != contributionStatusActive {
		return nil, appContrib.ErrAlreadyReversed
	}
	return nil, appContrib.ErrConflict
}

func (s *ContributionsStore) getItem(ctx context.Context, pk, sk string) (map[string]types.AttributeValue, error) {
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      aws.String(s.table),
		Key:            map[string]types.AttributeValue{"PK": stringAttr(pk), "SK": stringAttr(sk)},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return nil, appContrib.ErrUnavailable
	}
	if out == nil || len(out.Item) == 0 {
		return nil, nil
	}
	return out.Item, nil
}

type allocationJSON struct {
	GoalID      string `json:"goalId"`
	AmountCents int64  `json:"amountCents"`
}

func contributionItem(c appContrib.Contribution, pk, sk string, now time.Time) map[string]types.AttributeValue {
	allocations := make([]allocationJSON, 0, len(c.Allocations))
	goalIDs := make([]string, 0, len(c.Allocations))
	for _, a := range c.Allocations {
		allocations = append(allocations, allocationJSON{GoalID: a.GoalID, AmountCents: a.AmountCents})
		goalIDs = append(goalIDs, a.GoalID)
	}
	raw, _ := json.Marshal(allocations)
	item := map[string]types.AttributeValue{
		"PK":              stringAttr(pk),
		"SK":              stringAttr(sk),
		"entityType":      stringAttr(contributionEntityType),
		"userId":          stringAttr(c.UserID),
		"contributionId":  stringAttr(c.ID),
		"amountCents":     numberAttr(c.AmountCents),
		"currency":        stringAttr(c.Currency),
		"allocationsJSON": stringAttr(string(raw)),
		"occurredAt":      stringAttr(appMovements.SortableTime(c.OccurredAt)),
		"status":          stringAttr(c.Status),
		"version":         numberAttr(c.Version),
		"createdAt":       stringAttr(now.Format(time.RFC3339)),
		"updatedAt":       stringAttr(now.Format(time.RFC3339)),
	}
	if len(goalIDs) > 0 {
		item["goalIds"] = &types.AttributeValueMemberSS{Value: goalIDs}
	}
	return item
}

func contributionFromItem(item map[string]types.AttributeValue, userID string) (appContrib.Contribution, bool) {
	if len(item) == 0 {
		return appContrib.Contribution{}, false
	}
	if entidad := stringAttribute(item["entityType"]); entidad != "" && entidad != contributionEntityType {
		return appContrib.Contribution{}, false
	}
	if titular := stringAttribute(item["userId"]); titular != "" && titular != userID {
		return appContrib.Contribution{}, false
	}
	id := stringAttribute(item["contributionId"])
	amount, okAmount := attributeInt(item["amountCents"])
	if id == "" || !okAmount {
		return appContrib.Contribution{}, false
	}
	status := stringAttribute(item["status"])
	if status != contributionStatusActive && status != contributionStatusReversed {
		return appContrib.Contribution{}, false
	}
	version, okVersion := attributeInt(item["version"])
	if !okVersion || version < 1 {
		version = 1
	}
	occurred, okOccurred := appMovements.ParseStoredOccurredAt(stringAttribute(item["occurredAt"]))
	if !okOccurred {
		return appContrib.Contribution{}, false
	}
	var raw []allocationJSON
	if err := json.Unmarshal([]byte(stringAttribute(item["allocationsJSON"])), &raw); err != nil {
		return appContrib.Contribution{}, false
	}
	allocations := make([]appContrib.Allocation, 0, len(raw))
	for _, a := range raw {
		if a.GoalID == "" || a.AmountCents <= 0 {
			return appContrib.Contribution{}, false
		}
		allocations = append(allocations, appContrib.Allocation{GoalID: a.GoalID, AmountCents: a.AmountCents})
	}
	created, _ := attributeISOTime(item["createdAt"])
	updated, _ := attributeISOTime(item["updatedAt"])
	reversed, _ := attributeISOTime(item["reversedAt"])
	return appContrib.Contribution{
		ID:            id,
		UserID:        userID,
		AmountCents:   amount,
		Currency:      stringAttribute(item["currency"]),
		Allocations:   allocations,
		OccurredAt:    occurred,
		Status:        status,
		Version:       version,
		CreatedAt:     created,
		UpdatedAt:     updated,
		ReversedAt:    reversed,
		ReverseReason: stringAttribute(item["reverseReason"]),
	}, true
}
