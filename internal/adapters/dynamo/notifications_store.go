package dynamo

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	appNotifications "github.com/nexumpr/Nexum-Backend/internal/application/notifications"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
)

const (
	// NotificationEntityType y ReadAllEntityType discriminan los dos items que
	// este almacén escribe dentro de la partición del usuario.
	NotificationEntityType = "NOTIFICATION"
	ReadAllEntityType      = "READALL"

	// NotificationSKPrefix y ReadAllSKPrefix son los prefijos de SK de la tabla
	// única. La comunidad `USER#<sub>` se comparte; los prefijos no.
	NotificationSKPrefix = "NOTIFICATION#"
	ReadAllSKPrefix      = "READALL#"

	// NotificationsReadAllBatch acota cada Query del fan-out de E063.
	NotificationsReadAllBatch int32 = 25
)

// NotificationsAPI es el subconjunto del cliente DynamoDB que necesitan
// E061/E062/E063. No incluye Scan: el compilador impide un barrido global.
type NotificationsAPI interface {
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
}

// NotificationsStore implementa el puerto de aplicación sobre la tabla única.
type NotificationsStore struct {
	client NotificationsAPI
	table  string
}

// NewNotificationsStore construye el adaptador. Sin cliente o tabla, todas las
// operaciones fallan cerrado con ErrUnavailable: nunca se sirve memoria local.
func NewNotificationsStore(client NotificationsAPI, table string) *NotificationsStore {
	return &NotificationsStore{client: client, table: strings.TrimSpace(table)}
}

func (s *NotificationsStore) disponible() bool { return s != nil && s.client != nil && s.table != "" }

func notificationUserPK(userID string) string {
	return domainDynamo.PrefixUser + strings.TrimSpace(userID)
}

func notificationSortKey(createdAt time.Time, id string) string {
	return NotificationSKPrefix + createdAt.UTC().Format(time.RFC3339Nano) + "#" + strings.TrimSpace(id)
}

func readAllSortKey(requestID string) string { return ReadAllSKPrefix + strings.TrimSpace(requestID) }

func dynamoKey(k *appNotifications.NotificationKey) map[string]types.AttributeValue {
	if k == nil {
		return nil
	}
	return map[string]types.AttributeValue{"PK": stringAttr(k.PK), "SK": stringAttr(k.SK)}
}

func notificationKeyFromItem(item map[string]types.AttributeValue) *appNotifications.NotificationKey {
	if len(item) == 0 {
		return nil
	}
	return &appNotifications.NotificationKey{
		PK: stringAttribute(item["PK"]),
		SK: stringAttribute(item["SK"]),
	}
}

// queryNotifications ejecuta un Query acotado a una sola partición y prefijo
// de SK. `soloCuenta` usa Select COUNT; el llamador decide el límite.
func (s *NotificationsStore) queryNotifications(
	ctx context.Context,
	pk string,
	isRead *bool,
	tipo appNotifications.Type,
	after *appNotifications.NotificationKey,
	limit int,
	soloCuenta bool,
) (*dynamodb.QueryOutput, error) {
	nombres := map[string]string{
		"#pk": "PK", "#sk": "SK", "#entityType": "entityType",
	}
	valores := map[string]types.AttributeValue{
		":pk":           stringAttr(pk),
		":prefijo":      stringAttr(NotificationSKPrefix),
		":notificacion": stringAttr(NotificationEntityType),
	}
	filtros := []string{"#entityType = :notificacion"}
	if isRead != nil {
		nombres["#isRead"] = "isRead"
		valores[":isRead"] = boolAttr(*isRead)
		filtros = append(filtros, "#isRead = :isRead")
	}
	if tipo != "" {
		nombres["#type"] = "type"
		valores[":type"] = stringAttr(string(tipo))
		filtros = append(filtros, "#type = :type")
	}

	entrada := &dynamodb.QueryInput{
		TableName:                 aws.String(s.table),
		KeyConditionExpression:    aws.String("#pk = :pk AND begins_with(#sk, :prefijo)"),
		FilterExpression:          aws.String(strings.Join(filtros, " AND ")),
		ExpressionAttributeNames:  nombres,
		ExpressionAttributeValues: valores,
		ConsistentRead:            aws.Bool(true),
		ExclusiveStartKey:         dynamoKey(after),
	}
	if soloCuenta {
		entrada.Select = types.SelectCount
	} else if limit > 0 {
		entrada.Limit = aws.Int32(int32(limit))
	}
	return s.client.Query(ctx, entrada)
}

// List devuelve una página filtrada. El filtro se envía en FilterExpression y
// se vuelve a comprobar al proyectar para no fiarse de un proveedor que no lo
// aplicase. El cursor devuelto apunta al último ITEM entregado, no al final de
// la página escaneada, para no saltarse items cuando se alcanza el límite.
func (s *NotificationsStore) List(ctx context.Context, q appNotifications.ListQuery) (*appNotifications.ListResult, error) {
	if !s.disponible() {
		return nil, appNotifications.ErrUnavailable
	}
	userID := strings.TrimSpace(q.UserID)
	if userID == "" || q.Limit < 1 || q.Limit > appNotifications.MaxLimit {
		return nil, appNotifications.ErrInvalidRequest
	}
	pk := notificationUserPK(userID)
	after := q.After
	if after != nil && (after.PK != pk || after.SK == "") {
		return nil, appNotifications.ErrInvalidRequest
	}

	items := make([]appNotifications.Notification, 0, q.Limit)
	var siguiente *appNotifications.NotificationKey
	for len(items) < q.Limit {
		salida, err := s.queryNotifications(ctx, pk, q.IsRead, q.Type, after, q.Limit-len(items), false)
		if err != nil {
			return nil, appNotifications.ErrUnavailable
		}
		if salida == nil {
			return nil, appNotifications.ErrUnavailable
		}
		for _, crude := range salida.Items {
			n := notificationFromItem(crude)
			if n == nil || !cumpleFiltros(*n, q) {
				continue
			}
			items = append(items, *n)
			ultima := notificationKeyFromItem(crude)
			if ultima != nil && ultima.SK != "" {
				siguiente = ultima
			}
			if len(items) == q.Limit {
				break
			}
		}
		if len(items) == q.Limit || len(salida.LastEvaluatedKey) == 0 {
			if len(salida.LastEvaluatedKey) == 0 {
				siguiente = nil
			}
			break
		}
		after = notificationKeyFromItem(salida.LastEvaluatedKey)
		if after == nil || after.SK == "" {
			siguiente = nil
			break
		}
	}
	return &appNotifications.ListResult{Items: items, NextKey: siguiente}, nil
}

func cumpleFiltros(n appNotifications.Notification, q appNotifications.ListQuery) bool {
	if q.IsRead != nil && n.IsRead != *q.IsRead {
		return false
	}
	if q.Type != "" && n.Type != q.Type {
		return false
	}
	return true
}

// CountUnread cuenta TODAS las no leídas del usuario con Query/Select COUNT,
// paginando el propio Query. Nunca usa el resultado de List ni un Scan.
func (s *NotificationsStore) CountUnread(ctx context.Context, userID string) (int64, error) {
	if !s.disponible() {
		return 0, appNotifications.ErrUnavailable
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return 0, appNotifications.ErrUnauthorized
	}
	pk := notificationUserPK(userID)
	soloNoLeidas := false
	var total int64
	var after *appNotifications.NotificationKey
	for {
		salida, err := s.queryNotifications(ctx, pk, &soloNoLeidas, "", after, 0, true)
		if err != nil {
			return 0, appNotifications.ErrUnavailable
		}
		if salida == nil {
			return 0, appNotifications.ErrUnavailable
		}
		total += int64(salida.Count)
		if len(salida.LastEvaluatedKey) == 0 {
			return total, nil
		}
		after = notificationKeyFromItem(salida.LastEvaluatedKey)
		if after == nil || after.SK == "" {
			return 0, appNotifications.ErrUnavailable
		}
	}
}

// resolveNotificationKey localiza la SK exacta dentro de la partición del
// titular mediante Query por prefijo. Un item ajeno nunca casará con esta PK.
func (s *NotificationsStore) resolveNotificationKey(ctx context.Context, userID, notificationID string) (*appNotifications.NotificationKey, error) {
	pk := notificationUserPK(userID)
	var after *appNotifications.NotificationKey
	for {
		entrada := &dynamodb.QueryInput{
			TableName:              aws.String(s.table),
			KeyConditionExpression: aws.String("#pk = :pk AND begins_with(#sk, :prefijo)"),
			FilterExpression:       aws.String("#id = :id"),
			ExpressionAttributeNames: map[string]string{
				"#pk": "PK", "#sk": "SK", "#id": "id",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":pk":      stringAttr(pk),
				":prefijo": stringAttr(NotificationSKPrefix),
				":id":      stringAttr(notificationID),
			},
			ConsistentRead:    aws.Bool(true),
			ExclusiveStartKey: dynamoKey(after),
		}
		salida, err := s.client.Query(ctx, entrada)
		if err != nil || salida == nil {
			return nil, appNotifications.ErrUnavailable
		}
		for _, crude := range salida.Items {
			if notificationFromItem(crude) == nil {
				continue
			}
			if clave := notificationKeyFromItem(crude); clave != nil && clave.SK != "" {
				return clave, nil
			}
		}
		if len(salida.LastEvaluatedKey) == 0 {
			return nil, appNotifications.ErrNotFound
		}
		after = notificationKeyFromItem(salida.LastEvaluatedKey)
	}
}

// MarkRead hace la transición atómica `isRead = false` -> `true` con
// ReturnValues ALL_NEW. Si otra petición ganó la carrera, se relee el item: un
// item ya leído se devuelve tal cual (idempotencia) sin contarlo dos veces.
func (s *NotificationsStore) MarkRead(ctx context.Context, userID, notificationID string, now time.Time) (*appNotifications.Notification, error) {
	if !s.disponible() {
		return nil, appNotifications.ErrUnavailable
	}
	userID = strings.TrimSpace(userID)
	notificationID = strings.TrimSpace(notificationID)
	if userID == "" {
		return nil, appNotifications.ErrUnauthorized
	}
	if notificationID == "" {
		return nil, appNotifications.ErrInvalidRequest
	}
	clave, err := s.resolveNotificationKey(ctx, userID, notificationID)
	if err != nil {
		return nil, err
	}
	entrada := &dynamodb.UpdateItemInput{
		TableName:           aws.String(s.table),
		Key:                 dynamoKey(clave),
		UpdateExpression:    aws.String("SET #isRead = :verdadero, #readAt = :readAt"),
		ConditionExpression: aws.String("#isRead = :falso"),
		ExpressionAttributeNames: map[string]string{
			"#isRead": "isRead", "#readAt": "readAt",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":verdadero": boolAttr(true),
			":falso":     boolAttr(false),
			":readAt":    stringAttr(now.UTC().Format(time.RFC3339Nano)),
		},
		ReturnValues: types.ReturnValueAllNew,
	}
	salida, err := s.client.UpdateItem(ctx, entrada)
	if err == nil {
		if salida == nil {
			return nil, appNotifications.ErrUnavailable
		}
		n := notificationFromItem(salida.Attributes)
		if n == nil {
			return nil, appNotifications.ErrUnavailable
		}
		return n, nil
	}
	var condicional *types.ConditionalCheckFailedException
	if !errors.As(err, &condicional) {
		return nil, appNotifications.ErrUnavailable
	}
	actual, errLeer := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      aws.String(s.table),
		Key:            dynamoKey(clave),
		ConsistentRead: aws.Bool(true),
	})
	if errLeer != nil || actual == nil {
		return nil, appNotifications.ErrUnavailable
	}
	n := notificationFromItem(actual.Item)
	if n == nil {
		return nil, appNotifications.ErrNotFound
	}
	if !n.IsRead {
		return nil, appNotifications.ErrConflict
	}
	return n, nil
}

// BeginReadAll persiste el registro READALL ANTES de devolver 202. Si el
// requestId ya existe se reanuda; si está completado se devuelve tal cual.
func (s *NotificationsStore) BeginReadAll(ctx context.Context, userID, requestID string, now time.Time) (*appNotifications.ReadAllState, error) {
	if !s.disponible() {
		return nil, appNotifications.ErrUnavailable
	}
	userID = strings.TrimSpace(userID)
	requestID = strings.TrimSpace(requestID)
	if userID == "" {
		return nil, appNotifications.ErrUnauthorized
	}
	if requestID == "" {
		return nil, appNotifications.ErrInvalidRequest
	}
	pk := notificationUserPK(userID)
	inicial, errConteo := s.CountUnread(ctx, userID)
	if errConteo != nil {
		return nil, errConteo
	}
	item := map[string]types.AttributeValue{
		"PK":             stringAttr(pk),
		"SK":             stringAttr(readAllSortKey(requestID)),
		"entityType":     stringAttr(ReadAllEntityType),
		"userId":         stringAttr(userID),
		"requestId":      stringAttr(requestID),
		"status":         stringAttr(appNotifications.StatusPending),
		"createdAt":      stringAttr(now.UTC().Format(time.RFC3339Nano)),
		"processedCount": numberAttr(0),
		"initialUnread":  numberAttr(inicial),
	}
	_, err := s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(s.table),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	})
	if err == nil {
		return readAllFromItem(item)
	}
	var condicional *types.ConditionalCheckFailedException
	if !errors.As(err, &condicional) {
		return nil, appNotifications.ErrUnavailable
	}
	return s.LoadReadAll(ctx, userID, requestID)
}

// LoadReadAll lee el registro con consistencia fuerte.
func (s *NotificationsStore) LoadReadAll(ctx context.Context, userID, requestID string) (*appNotifications.ReadAllState, error) {
	if !s.disponible() {
		return nil, appNotifications.ErrUnavailable
	}
	userID = strings.TrimSpace(userID)
	requestID = strings.TrimSpace(requestID)
	if userID == "" || requestID == "" {
		return nil, appNotifications.ErrInvalidRequest
	}
	salida, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      aws.String(s.table),
		Key:            dynamoKey(&appNotifications.NotificationKey{PK: notificationUserPK(userID), SK: readAllSortKey(requestID)}),
		ConsistentRead: aws.Bool(true),
	})
	if err != nil || salida == nil {
		return nil, appNotifications.ErrUnavailable
	}
	if len(salida.Item) == 0 {
		return nil, appNotifications.ErrNotFound
	}
	return readAllFromItem(salida.Item)
}

// AdvanceReadAll guarda el cursor (LastEvaluatedKey) y renueva el lease. La
// condición impide que dos nodos con dueños distintos avancen el mismo request.
func (s *NotificationsStore) AdvanceReadAll(ctx context.Context, userID, expectedLeaseOwner string, next appNotifications.ReadAllState) error {
	if !s.disponible() {
		return appNotifications.ErrUnavailable
	}
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(next.RequestID) == "" {
		return appNotifications.ErrInvalidRequest
	}
	cursorSK := ""
	if next.Cursor != nil {
		cursorSK = next.Cursor.SK
	}
	_, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.table),
		Key: dynamoKey(&appNotifications.NotificationKey{
			PK: notificationUserPK(userID), SK: readAllSortKey(next.RequestID),
		}),
		UpdateExpression: aws.String("SET #status = :status, #processedCount = :count, #cursor = :cursor, " +
			"#leaseOwner = :owner, #leaseUntil = :until, #updatedAt = :updatedAt"),
		ConditionExpression: aws.String("#status = :pending AND " +
			"(attribute_not_exists(#leaseOwner) OR #leaseOwner = :expected OR #leaseUntil <= :now)"),
		ExpressionAttributeNames: map[string]string{
			"#status": "status", "#processedCount": "processedCount", "#cursor": "cursor",
			"#leaseOwner": "leaseOwner", "#leaseUntil": "leaseUntil", "#updatedAt": "updatedAt",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":status":    stringAttr(appNotifications.StatusPending),
			":count":     numberAttr(next.ProcessedCount),
			":cursor":    stringAttr(cursorSK),
			":owner":     stringAttr(next.LeaseOwner),
			":until":     stringAttr(next.LeaseUntil.UTC().Format(time.RFC3339Nano)),
			":updatedAt": stringAttr(time.Now().UTC().Format(time.RFC3339Nano)),
			":pending":   stringAttr(appNotifications.StatusPending),
			":expected":  stringAttr(expectedLeaseOwner),
			":now":       stringAttr(time.Now().UTC().Format(time.RFC3339Nano)),
		},
	})
	return traducirCondicional(err)
}

// CompleteReadAll cierra el registro con completedAt y libera el lease.
func (s *NotificationsStore) CompleteReadAll(ctx context.Context, userID, requestID, expectedLeaseOwner string, processed int64, now time.Time) error {
	if !s.disponible() {
		return appNotifications.ErrUnavailable
	}
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(requestID) == "" {
		return appNotifications.ErrInvalidRequest
	}
	_, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.table),
		Key: dynamoKey(&appNotifications.NotificationKey{
			PK: notificationUserPK(userID), SK: readAllSortKey(requestID),
		}),
		UpdateExpression: aws.String("SET #status = :status, #completedAt = :completedAt, " +
			"#processedCount = :count REMOVE #cursor, #leaseOwner, #leaseUntil"),
		ConditionExpression: aws.String("#status = :pending AND " +
			"(attribute_not_exists(#leaseOwner) OR #leaseOwner = :expected OR #leaseUntil <= :now)"),
		ExpressionAttributeNames: map[string]string{
			"#status": "status", "#completedAt": "completedAt", "#processedCount": "processedCount",
			"#cursor": "cursor", "#leaseOwner": "leaseOwner", "#leaseUntil": "leaseUntil",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":status":      stringAttr(appNotifications.StatusCompleted),
			":pending":     stringAttr(appNotifications.StatusPending),
			":completedAt": stringAttr(now.UTC().Format(time.RFC3339Nano)),
			":count":       numberAttr(processed),
			":expected":    stringAttr(expectedLeaseOwner),
			":now":         stringAttr(now.UTC().Format(time.RFC3339Nano)),
		},
	})
	return traducirCondicional(err)
}

func traducirCondicional(err error) error {
	if err == nil {
		return nil
	}
	var condicional *types.ConditionalCheckFailedException
	if errors.As(err, &condicional) {
		return appNotifications.ErrConflict
	}
	return appNotifications.ErrUnavailable
}

func notificationFromItem(item map[string]types.AttributeValue) *appNotifications.Notification {
	if len(item) == 0 || stringAttribute(item["entityType"]) != NotificationEntityType {
		return nil
	}
	creado, err := time.Parse(time.RFC3339Nano, stringAttribute(item["createdAt"]))
	if err != nil {
		return nil
	}
	id := stringAttribute(item["id"])
	if id == "" {
		sk := strings.TrimPrefix(stringAttribute(item["SK"]), NotificationSKPrefix)
		if i := strings.LastIndex(sk, "#"); i >= 0 {
			id = sk[i+1:]
		}
	}
	if id == "" {
		return nil
	}
	n := &appNotifications.Notification{
		ID:             id,
		Title:          stringAttribute(item["title"]),
		Message:        stringAttribute(item["message"]),
		Type:           appNotifications.Type(stringAttribute(item["type"])),
		IsRead:         boolAttribute(item["isRead"]),
		CreatedAt:      creado.UTC(),
		DeliveryStatus: stringAttribute(item["deliveryStatus"]),
	}
	if readAt, err := time.Parse(time.RFC3339Nano, stringAttribute(item["readAt"])); err == nil {
		lectura := readAt.UTC()
		n.ReadAt = &lectura
	}
	return n
}

func readAllFromItem(item map[string]types.AttributeValue) (*appNotifications.ReadAllState, error) {
	if len(item) == 0 || stringAttribute(item["entityType"]) != ReadAllEntityType {
		return nil, appNotifications.ErrNotFound
	}
	creado, err := time.Parse(time.RFC3339Nano, stringAttribute(item["createdAt"]))
	if err != nil {
		return nil, appNotifications.ErrUnavailable
	}
	estado := &appNotifications.ReadAllState{
		RequestID:  stringAttribute(item["requestId"]),
		Status:     stringAttribute(item["status"]),
		CreatedAt:  creado.UTC(),
		LeaseOwner: stringAttribute(item["leaseOwner"]),
	}
	if estado.RequestID == "" {
		sk := strings.TrimPrefix(stringAttribute(item["SK"]), ReadAllSKPrefix)
		estado.RequestID = sk
	}
	if consecuencia, ok := attributeInt(item["processedCount"]); ok && consecuencia >= 0 {
		estado.ProcessedCount = consecuencia
	}
	if inicial, ok := attributeInt(item["initialUnread"]); ok && inicial >= 0 {
		estado.InitialUnread = inicial
	}
	if cursor := stringAttribute(item["cursor"]); cursor != "" {
		estado.Cursor = &appNotifications.NotificationKey{
			PK: notificationUserPK(stringAttribute(item["userId"])),
			SK: cursor,
		}
	}
	if completado, err := time.Parse(time.RFC3339Nano, stringAttribute(item["completedAt"])); err == nil {
		final := completado.UTC()
		estado.CompletedAt = &final
	}
	if hasta, err := time.Parse(time.RFC3339Nano, stringAttribute(item["leaseUntil"])); err == nil {
		estado.LeaseUntil = hasta.UTC()
	}
	return estado, nil
}
