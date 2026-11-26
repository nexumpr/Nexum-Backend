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
	// legacyNotificationEntityType es el entityType con el que el worker del
	// outbox escribio avisos antes de unificar la constante. Se sigue leyendo
	// para no ocultar notificaciones ya entregadas; nunca se escribe.
	legacyNotificationEntityType = "USER_NOTIFICATION"
	ReadAllEntityType            = "READALL"

	// ReadAllQueueEntityType y ReadAllPointerEntityType identifican el ítem
	// global de la cola de E063 y el puntero dentro de la partición del
	// titular (que E014 borra con el resto de la partición).
	ReadAllQueueEntityType   = "READALL_QUEUE"
	ReadAllPointerEntityType = "READALL_PTR"

	// NotificationSKPrefix y ReadAllSKPrefix son los prefijos de SK de la tabla
	// única. La comunidad `USER#<sub>` se comparte; los prefijos no.
	NotificationSKPrefix   = "NOTIFICATION#"
	ReadAllSKPrefix        = "READALL#"
	ReadAllPointerSKPrefix = "READALLPTR#"

	// ReadAllQueuePK y ReadAllQueueSKPrefix forman la cola global y acotada de
	// E063: `QUEUE#READALL` / `REQ#<createdAt>#<requestId>`. Nunca se consulta
	// con Scan: es un Query de una sola partición.
	ReadAllQueuePK       = "QUEUE#READALL"
	ReadAllQueueSKPrefix = "REQ#"

	// NotificationsReadAllBatch acota cada Query del fan-out de E063.
	NotificationsReadAllBatch int32 = 25

	// readAllDueMaxPaginas acota cuántas páginas de la cola evalúa un solo
	// ciclo para no quedarse mirando siempre los mismos ítems con lease.
	readAllDueMaxPaginas = 3
)

// readAllQueueTimeLayout es RFC3339Nano con ancho fijo: la SK de la cola debe
// ordenar lexicográficamente igual que el tiempo y no depender de si los
// nanosegundos son cero.
const readAllQueueTimeLayout = "2006-01-02T15:04:05.000000000Z"

// NotificationsAPI es el subconjunto del cliente DynamoDB que necesitan
// E061/E062/E063. No incluye Scan: el compilador impide un barrido global.
type NotificationsAPI interface {
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
	TransactWriteItems(ctx context.Context, params *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error)
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

func readAllPointerSortKey(requestID string) string {
	return ReadAllPointerSKPrefix + strings.TrimSpace(requestID)
}

// readAllQueueSortKey usa un ancho fijo para que el orden lexicográfico de la
// SK coincida con el orden temporal y no dependa de los nanosegundos.
func readAllQueueSortKey(createdAt time.Time, requestID string) string {
	return ReadAllQueueSKPrefix + createdAt.UTC().Format(readAllQueueTimeLayout) + "#" + strings.TrimSpace(requestID)
}

func readAllQueueKey(job appNotifications.ReadAllJob) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"PK": stringAttr(ReadAllQueuePK),
		"SK": stringAttr(readAllQueueSortKey(job.CreatedAt, job.RequestID)),
	}
}

func epochAttr(value time.Time) types.AttributeValue { return numberAttr(value.UTC().Unix()) }

// epochTimeAttribute lee una marca almacenada como Number (epoch seconds).
func epochTimeAttribute(value types.AttributeValue) (time.Time, bool) {
	seconds, ok := attributeInt(value)
	if !ok || seconds <= 0 {
		return time.Time{}, false
	}
	return time.Unix(seconds, 0).UTC(), true
}

// limiteColaNotificaciones acota el Limit de un Query antes de int32.
func limiteColaNotificaciones(limit int) int32 {
	if limit < 1 {
		return 1
	}
	if limit > appNotifications.MaxLimit {
		return int32(appNotifications.MaxLimit)
	}
	return int32(limit)
}

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
		":legacy":       stringAttr(legacyNotificationEntityType),
	}
	// Se aceptan ambos valores: el canonico y el que escribio el worker antes
	// de unificar la constante. El parentesis externo mantiene la precedencia
	// cuando se agregan filtros con AND.
	filtros := []string{"(#entityType = :notificacion OR #entityType = :legacy)"}
	if isRead != nil {
		nombres["#isRead"] = "isRead"
		valores[":isRead"] = boolAttr(*isRead)
		filtros = append(filtros, "#isRead = :isRead")
	}
	if tipo != "" {
		nombres["#type"] = "type"
		valores[":type"] = stringAttr(string(tipo))
		expresion := "#type = :type"
		if alias := appNotifications.TypeFilterAliases(tipo); len(alias) > 0 {
			// El filtro `system` debe alcanzar los avisos históricos escritos como
			// `reminder`. Se resuelve en la FilterExpression para que el filtro se
			// aplique antes de paginar y no después de recortar la página. Los alias
			// se declaran uno a uno: un nombre dinámico no aporta y complica la lectura.
			partes := []string{"#type = :type"}
			for i, valor := range alias {
				nombres[typeAliasName(i)] = "type"
				valores[typeAliasValue(i)] = stringAttr(string(valor))
				partes = append(partes, typeAliasName(i)+" = "+typeAliasValue(i))
			}
			expresion = "(" + strings.Join(partes, " OR ") + ")"
		}
		filtros = append(filtros, expresion)
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
		entrada.Limit = aws.Int32(limiteConsultaNotificaciones(limit))
	}
	return s.client.Query(ctx, entrada)
}

// typeAliasName y typeAliasValue devuelven los marcadores de un alias de `type`.
// El conjunto es cerrado (hoy sólo `reminder` para `system`): no hace falta
// generarlos en tiempo de ejecución.
func typeAliasName(i int) string {
	if i == 0 {
		return "#typeAlias0"
	}
	return "#typeAlias1"
}

func typeAliasValue(i int) string {
	if i == 0 {
		return ":typeAlias0"
	}
	return ":typeAlias1"
}

// limiteConsultaNotificaciones acota el `Limit` de DynamoDB al rango contractual
// (1..MaxLimit) antes de convertirlo a int32. Evita el desbordamiento G115 y un
// valor fuera de contrato si un llamador interno se equivoca.
func limiteConsultaNotificaciones(limit int) int32 {
	if limit < 1 {
		return 1
	}
	if limit > appNotifications.MaxLimit {
		return int32(appNotifications.MaxLimit)
	}
	return int32(limit)
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

// BeginReadAll hace la admisión de E063: escribe en una sola transacción el
// registro READALL, el ítem de la cola global `QUEUE#READALL` y el puntero
// `USER#<sub>/READALLPTR#<requestId>` que E014 borra con la partición.
//
// No procesa notificaciones: el 202 sólo confirma que el trabajo quedó
// persistido. Si el requestId ya existe, la transacción se cancela y se
// devuelve el registro vigente (idempotencia); si está pendiente y perdió su
// ítem de cola, se repara con escrituras condicionales.
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
	creado := now.UTC()
	registro := map[string]types.AttributeValue{
		"PK":             stringAttr(pk),
		"SK":             stringAttr(readAllSortKey(requestID)),
		"entityType":     stringAttr(ReadAllEntityType),
		"userId":         stringAttr(userID),
		"requestId":      stringAttr(requestID),
		"status":         stringAttr(appNotifications.StatusPending),
		"createdAt":      stringAttr(creado.Format(time.RFC3339Nano)),
		"processedCount": numberAttr(0),
		"initialUnread":  numberAttr(inicial),
		"fence":          numberAttr(0),
	}
	cola := map[string]types.AttributeValue{
		"PK":         stringAttr(ReadAllQueuePK),
		"SK":         stringAttr(readAllQueueSortKey(creado, requestID)),
		"entityType": stringAttr(ReadAllQueueEntityType),
		"userId":     stringAttr(userID),
		"requestId":  stringAttr(requestID),
		"createdAt":  epochAttr(creado),
		"fence":      numberAttr(0),
		"attempts":   numberAttr(0),
	}
	puntero := map[string]types.AttributeValue{
		"PK":         stringAttr(pk),
		"SK":         stringAttr(readAllPointerSortKey(requestID)),
		"entityType": stringAttr(ReadAllPointerEntityType),
		"requestId":  stringAttr(requestID),
		"skGlobal":   stringAttr(readAllQueueSortKey(creado, requestID)),
		"createdAt":  stringAttr(creado.Format(time.RFC3339Nano)),
	}

	acciones := []types.TransactWriteItem{
		{Put: &types.Put{TableName: aws.String(s.table), Item: registro, ConditionExpression: aws.String("attribute_not_exists(PK)")}},
		{Put: &types.Put{TableName: aws.String(s.table), Item: cola, ConditionExpression: aws.String("attribute_not_exists(PK)")}},
		{Put: &types.Put{TableName: aws.String(s.table), Item: puntero, ConditionExpression: aws.String("attribute_not_exists(PK)")}},
	}
	_, err := s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: acciones})
	if err == nil {
		return readAllFromItem(registro)
	}
	var cancelada *types.TransactionCanceledException
	if !errors.As(err, &cancelada) {
		return nil, appNotifications.ErrUnavailable
	}

	// El registro ya existía: misma petición repetida. Se devuelve el vigente y
	// se repara la cola si quedó pendiente sin ítem (registros heredados).
	estado, errLeer := s.LoadReadAll(ctx, userID, requestID)
	if errLeer != nil {
		return nil, errLeer
	}
	if estado.Status == appNotifications.StatusPending {
		s.ensureReadAllQueued(ctx, userID, estado)
	}
	return estado, nil
}

// ensureReadAllQueued repara la cola de un registro pendiente sin duplicar el
// ítem: ambas escrituras son condicionales y los conflictos se ignoran porque
// significan que el ítem ya estaba allí.
func (s *NotificationsStore) ensureReadAllQueued(ctx context.Context, userID string, estado *appNotifications.ReadAllState) {
	if estado == nil || estado.CreatedAt.IsZero() {
		return
	}
	cola := map[string]types.AttributeValue{
		"PK":         stringAttr(ReadAllQueuePK),
		"SK":         stringAttr(readAllQueueSortKey(estado.CreatedAt, estado.RequestID)),
		"entityType": stringAttr(ReadAllQueueEntityType),
		"userId":     stringAttr(strings.TrimSpace(userID)),
		"requestId":  stringAttr(estado.RequestID),
		"createdAt":  epochAttr(estado.CreatedAt),
		"fence":      numberAttr(estado.Fence),
		"attempts":   numberAttr(0),
	}
	puntero := map[string]types.AttributeValue{
		"PK":         stringAttr(notificationUserPK(userID)),
		"SK":         stringAttr(readAllPointerSortKey(estado.RequestID)),
		"entityType": stringAttr(ReadAllPointerEntityType),
		"requestId":  stringAttr(estado.RequestID),
		"skGlobal":   stringAttr(readAllQueueSortKey(estado.CreatedAt, estado.RequestID)),
		"createdAt":  stringAttr(estado.CreatedAt.UTC().Format(time.RFC3339Nano)),
	}
	_, _ = s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{Put: &types.Put{TableName: aws.String(s.table), Item: cola, ConditionExpression: aws.String("attribute_not_exists(PK)")}},
			{Put: &types.Put{TableName: aws.String(s.table), Item: puntero, ConditionExpression: aws.String("attribute_not_exists(PK)")}},
		},
	})
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

// AdvanceReadAll guarda el cursor y renueva el lease del trabajo en una sola
// transacción: actualiza el ítem de cola (condición de fence/dueño/lease) y el
// registro READALL (condición de estado y fence monótono). Un worker cuyo lease
// venció o fue reclamado por otro no puede avanzar.
func (s *NotificationsStore) AdvanceReadAll(ctx context.Context, userID, expectedLeaseOwner string, next appNotifications.ReadAllState) error {
	if !s.disponible() {
		return appNotifications.ErrUnavailable
	}
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(next.RequestID) == "" {
		return appNotifications.ErrInvalidRequest
	}
	if next.CreatedAt.IsZero() {
		return appNotifications.ErrInvalidRequest
	}
	if expectedLeaseOwner != "" && next.LeaseOwner != expectedLeaseOwner {
		// El llamador afirma ser el dueño pero el estado que intenta persistir
		// pertenece a otro: es una carrera de lease, no una solicitud mal formada.
		return appNotifications.ErrConflict
	}
	cursorSK := ""
	if next.Cursor != nil {
		cursorSK = next.Cursor.SK
	}
	ahora := time.Now().UTC()

	renovarCola := types.TransactWriteItem{Update: &types.Update{
		TableName:        aws.String(s.table),
		Key:              readAllQueueKey(appNotifications.ReadAllJob{RequestID: next.RequestID, CreatedAt: next.CreatedAt}),
		UpdateExpression: aws.String("SET #leaseOwner = :owner, #leaseUntil = :until, #updatedAt = :updatedAt"),
		// La autoridad es fence+owner: si otro worker reclamó, el fence cambió y la
		// condición falla. No se compara con el reloj del proceso, que puede diferir
		// del reloj del llamador y romper avances legítimos.
		ConditionExpression: aws.String("#fence = :fence AND #leaseOwner = :owner"),
		ExpressionAttributeNames: map[string]string{
			"#leaseOwner": "leaseOwner", "#leaseUntil": "leaseUntil", "#updatedAt": "updatedAt", "#fence": "fence",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":owner":     stringAttr(next.LeaseOwner),
			":until":     epochAttr(next.LeaseUntil),
			":updatedAt": numberAttr(ahora.Unix()),
			":fence":     numberAttr(next.Fence),
		},
	}}
	avanzar := types.TransactWriteItem{Update: &types.Update{
		TableName: aws.String(s.table),
		Key: dynamoKey(&appNotifications.NotificationKey{
			PK: notificationUserPK(userID), SK: readAllSortKey(next.RequestID),
		}),
		UpdateExpression: aws.String("SET #status = :status, #processedCount = :count, #cursor = :cursor, " +
			"#leaseOwner = :owner, #leaseUntil = :until, #fence = :fence, #updatedAt = :updatedAt"),
		ConditionExpression: aws.String("#status = :pending AND (attribute_not_exists(#fence) OR #fence <= :fence)"),
		ExpressionAttributeNames: map[string]string{
			"#status": "status", "#processedCount": "processedCount", "#cursor": "cursor",
			"#leaseOwner": "leaseOwner", "#leaseUntil": "leaseUntil", "#fence": "fence", "#updatedAt": "updatedAt",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":status":    stringAttr(appNotifications.StatusPending),
			":pending":   stringAttr(appNotifications.StatusPending),
			":count":     numberAttr(next.ProcessedCount),
			":cursor":    stringAttr(cursorSK),
			":owner":     stringAttr(next.LeaseOwner),
			":until":     epochAttr(next.LeaseUntil),
			":fence":     numberAttr(next.Fence),
			":updatedAt": numberAttr(ahora.Unix()),
		},
	}}
	_, err := s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{renovarCola, avanzar},
	})
	return traducirTransaccion(err)
}

// CompleteReadAll cierra el registro y borra el ítem de cola y el puntero en la
// misma transacción. Si otro worker ya cerró el registro se devuelve conflicto
// (el worker lo trata como trabajo ajeno, nunca como fallo transitorio).
func (s *NotificationsStore) CompleteReadAll(ctx context.Context, userID, requestID, expectedLeaseOwner string, fence, processed int64, now time.Time) error {
	if !s.disponible() {
		return appNotifications.ErrUnavailable
	}
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(requestID) == "" {
		return appNotifications.ErrInvalidRequest
	}
	estado, err := s.LoadReadAll(ctx, userID, requestID)
	if err != nil {
		return err
	}
	if estado.Status == appNotifications.StatusCompleted {
		return nil // idempotente: ya estaba cerrado
	}
	job := appNotifications.ReadAllJob{
		UserID: strings.TrimSpace(userID), RequestID: strings.TrimSpace(requestID),
		CreatedAt: estado.CreatedAt, LeaseOwner: expectedLeaseOwner, Fence: fence,
	}
	cerrar := types.TransactWriteItem{Update: &types.Update{
		TableName: aws.String(s.table),
		Key: dynamoKey(&appNotifications.NotificationKey{
			PK: notificationUserPK(userID), SK: readAllSortKey(requestID),
		}),
		UpdateExpression: aws.String("SET #status = :status, #completedAt = :completedAt, " +
			"#processedCount = :count, #fence = :fence, #updatedAt = :updatedAt " +
			"REMOVE #cursor, #leaseOwner, #leaseUntil"),
		ConditionExpression: aws.String("#status = :pending AND (attribute_not_exists(#fence) OR #fence <= :fence)"),
		ExpressionAttributeNames: map[string]string{
			"#status": "status", "#completedAt": "completedAt", "#processedCount": "processedCount",
			"#cursor": "cursor", "#leaseOwner": "leaseOwner", "#leaseUntil": "leaseUntil",
			"#fence": "fence", "#updatedAt": "updatedAt",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":status":      stringAttr(appNotifications.StatusCompleted),
			":pending":     stringAttr(appNotifications.StatusPending),
			":completedAt": stringAttr(now.UTC().Format(time.RFC3339Nano)),
			":count":       numberAttr(processed),
			":fence":       numberAttr(fence),
			":updatedAt":   numberAttr(now.UTC().Unix()),
		},
	}}
	_, errTxn := s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{cerrar, s.borrarColaReadAll(job), s.borrarPunteroReadAll(userID, job.RequestID)},
	})
	return traducirTransaccion(errTxn)
}

// DueReadAll devuelve trabajos sin lease o con lease vencido. Es un Query de
// una sola partición con filtro: nunca un Scan, y acotado a tres páginas por
// ciclo para no mirar siempre los mismos ítems reclamados.
func (s *NotificationsStore) DueReadAll(ctx context.Context, now time.Time, limit int) ([]appNotifications.ReadAllJob, error) {
	if !s.disponible() {
		return nil, appNotifications.ErrUnavailable
	}
	if limit < 1 || limit > appNotifications.MaxLimit {
		return nil, appNotifications.ErrInvalidRequest
	}
	trabajos := make([]appNotifications.ReadAllJob, 0, limit)
	var despues map[string]types.AttributeValue
	for pagina := 0; pagina < readAllDueMaxPaginas && len(trabajos) < limit; pagina++ {
		entrada := &dynamodb.QueryInput{
			TableName:              aws.String(s.table),
			KeyConditionExpression: aws.String("#pk = :pk AND begins_with(#sk, :prefijo)"),
			FilterExpression:       aws.String("attribute_not_exists(#leaseUntil) OR #leaseUntil <= :now"),
			ExpressionAttributeNames: map[string]string{
				"#pk": "PK", "#sk": "SK", "#leaseUntil": "leaseUntil",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":pk":      stringAttr(ReadAllQueuePK),
				":prefijo": stringAttr(ReadAllQueueSKPrefix),
				":now":     numberAttr(now.UTC().Unix()),
			},
			ConsistentRead:    aws.Bool(true),
			Limit:             aws.Int32(limiteColaNotificaciones(limit)),
			ExclusiveStartKey: despues,
		}
		salida, err := s.client.Query(ctx, entrada)
		if err != nil || salida == nil {
			return nil, appNotifications.ErrUnavailable
		}
		for _, item := range salida.Items {
			trabajo, ok := readAllJobFromItem(item)
			if ok {
				trabajos = append(trabajos, trabajo)
			}
		}
		if len(salida.LastEvaluatedKey) == 0 {
			break
		}
		despues = salida.LastEvaluatedKey
	}
	return trabajos, nil
}

// ClaimReadAll reclama el trabajo con un UpdateItem condicional: sólo un worker
// puede pasar de «sin lease» o «lease vencido» a dueño con fence incrementado.
func (s *NotificationsStore) ClaimReadAll(ctx context.Context, job appNotifications.ReadAllJob, owner string, now time.Time, lease time.Duration) (appNotifications.ReadAllJob, bool, error) {
	if !s.disponible() {
		return appNotifications.ReadAllJob{}, false, appNotifications.ErrUnavailable
	}
	job.UserID = strings.TrimSpace(job.UserID)
	job.RequestID = strings.TrimSpace(job.RequestID)
	owner = strings.TrimSpace(owner)
	if job.UserID == "" || job.RequestID == "" || owner == "" {
		return appNotifications.ReadAllJob{}, false, appNotifications.ErrInvalidRequest
	}
	if lease <= 0 {
		return appNotifications.ReadAllJob{}, false, appNotifications.ErrInvalidRequest
	}
	salida, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.table),
		Key:       readAllQueueKey(job),
		UpdateExpression: aws.String("SET #leaseOwner = :owner, #leaseUntil = :until, #updatedAt = :updatedAt, " +
			"#attempts = if_not_exists(#attempts, :zero) + :one, #fence = if_not_exists(#fence, :zero) + :one"),
		ConditionExpression: aws.String("attribute_exists(PK) AND (attribute_not_exists(#leaseUntil) OR #leaseUntil <= :now)"),
		ExpressionAttributeNames: map[string]string{
			"#leaseOwner": "leaseOwner", "#leaseUntil": "leaseUntil", "#updatedAt": "updatedAt",
			"#attempts": "attempts", "#fence": "fence",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":owner":     stringAttr(owner),
			":until":     epochAttr(now.UTC().Add(lease)),
			":updatedAt": numberAttr(now.UTC().Unix()),
			":zero":      numberAttr(0),
			":one":       numberAttr(1),
			":now":       numberAttr(now.UTC().Unix()),
		},
		ReturnValues: types.ReturnValueAllNew,
	})
	if err != nil {
		var condicional *types.ConditionalCheckFailedException
		if errors.As(err, &condicional) {
			return appNotifications.ReadAllJob{}, false, nil // otro worker lo tomó
		}
		return appNotifications.ReadAllJob{}, false, appNotifications.ErrUnavailable
	}
	if salida == nil || len(salida.Attributes) == 0 {
		return appNotifications.ReadAllJob{}, false, appNotifications.ErrUnavailable
	}
	reclamado, ok := readAllJobFromItem(salida.Attributes)
	if !ok {
		return appNotifications.ReadAllJob{}, false, appNotifications.ErrUnavailable
	}
	return reclamado, true, nil
}

// ReleaseReadAll libera el lease y programa el siguiente intento. El ítem sigue
// en la cola: nunca se descarta trabajo sin ejecutarlo.
func (s *NotificationsStore) ReleaseReadAll(ctx context.Context, job appNotifications.ReadAllJob, now, retryAt time.Time) error {
	if !s.disponible() {
		return appNotifications.ErrUnavailable
	}
	if strings.TrimSpace(job.UserID) == "" || strings.TrimSpace(job.RequestID) == "" {
		return appNotifications.ErrInvalidRequest
	}
	_, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:           aws.String(s.table),
		Key:                 readAllQueueKey(job),
		UpdateExpression:    aws.String("SET #leaseOwner = :vacio, #leaseUntil = :retry, #updatedAt = :now"),
		ConditionExpression: aws.String("#fence = :fence AND #leaseOwner = :owner"),
		ExpressionAttributeNames: map[string]string{
			"#leaseOwner": "leaseOwner", "#leaseUntil": "leaseUntil", "#updatedAt": "updatedAt", "#fence": "fence",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":vacio": stringAttr(""),
			":retry": epochAttr(retryAt),
			":now":   numberAttr(now.UTC().Unix()),
			":fence": numberAttr(job.Fence),
			":owner": stringAttr(job.LeaseOwner),
		},
	})
	return traducirTransaccion(err)
}

// FinishReadAll borra el ítem de cola (condicionado al fence/dueño) y el
// puntero del titular. Se usa cuando el registro ya estaba completado o había
// desaparecido: el trabajo no debe quedar en la cola.
func (s *NotificationsStore) FinishReadAll(ctx context.Context, job appNotifications.ReadAllJob, _ time.Time) error {
	if !s.disponible() {
		return appNotifications.ErrUnavailable
	}
	if strings.TrimSpace(job.UserID) == "" || strings.TrimSpace(job.RequestID) == "" {
		return appNotifications.ErrInvalidRequest
	}
	_, err := s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			s.borrarColaReadAll(job),
			s.borrarPunteroReadAll(job.UserID, job.RequestID),
		},
	})
	return traducirTransaccion(err)
}

// borrarColaReadAll borra el ítem de cola por clave. El trabajo es obsoleto
// (el registro READALL desapareció o ya se completó): borrar es idempotente y no
// debe competir con el dueño vigente, que también encontrará el registro ausente.
func (s *NotificationsStore) borrarColaReadAll(job appNotifications.ReadAllJob) types.TransactWriteItem {
	return types.TransactWriteItem{Delete: &types.Delete{
		TableName: aws.String(s.table),
		Key:       readAllQueueKey(job),
	}}
}

// borrarPunteroReadAll borra el puntero del titular sin condición: es un
// índice de limpieza, no una autoridad.
func (s *NotificationsStore) borrarPunteroReadAll(userID, requestID string) types.TransactWriteItem {
	return types.TransactWriteItem{Delete: &types.Delete{
		TableName: aws.String(s.table),
		Key: dynamoKey(&appNotifications.NotificationKey{
			PK: notificationUserPK(userID), SK: readAllPointerSortKey(requestID),
		}),
	}}
}

// traducirTransaccion mapea la cancelación de una transacción a conflicto.
func traducirTransaccion(err error) error {
	if err == nil {
		return nil
	}
	var cancelada *types.TransactionCanceledException
	if errors.As(err, &cancelada) {
		return appNotifications.ErrConflict
	}
	var condicional *types.ConditionalCheckFailedException
	if errors.As(err, &condicional) {
		return appNotifications.ErrConflict
	}
	return appNotifications.ErrUnavailable
}

func notificationFromItem(item map[string]types.AttributeValue) *appNotifications.Notification {
	if len(item) == 0 {
		return nil
	}
	if entidad := stringAttribute(item["entityType"]); entidad != NotificationEntityType &&
		entidad != legacyNotificationEntityType {
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
		Type:           appNotifications.CanonicalType(appNotifications.Type(stringAttribute(item["type"]))),
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
	if fence, ok := attributeInt(item["fence"]); ok && fence >= 0 {
		estado.Fence = fence
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
	if hasta, ok := epochTimeAttribute(item["leaseUntil"]); ok {
		estado.LeaseUntil = hasta
	} else if hasta, err := time.Parse(time.RFC3339Nano, stringAttribute(item["leaseUntil"])); err == nil {
		// Los registros creados antes de la cola guardaban el lease como texto.
		estado.LeaseUntil = hasta.UTC()
	}
	return estado, nil
}

// readAllJobFromItem proyecta el ítem de la cola global. Devuelve ok=false si
// el item no pertenece a la cola o le faltan sus claves.
func readAllJobFromItem(item map[string]types.AttributeValue) (appNotifications.ReadAllJob, bool) {
	if len(item) == 0 || stringAttribute(item["PK"]) != ReadAllQueuePK {
		return appNotifications.ReadAllJob{}, false
	}
	if !strings.HasPrefix(stringAttribute(item["SK"]), ReadAllQueueSKPrefix) {
		return appNotifications.ReadAllJob{}, false
	}
	userID := strings.TrimSpace(stringAttribute(item["userId"]))
	requestID := strings.TrimSpace(stringAttribute(item["requestId"]))
	if userID == "" || requestID == "" {
		return appNotifications.ReadAllJob{}, false
	}
	trabajo := appNotifications.ReadAllJob{
		UserID:     userID,
		RequestID:  requestID,
		LeaseOwner: stringAttribute(item["leaseOwner"]),
	}
	// El instante debe provenir de la SK, que es la clave real del trabajo. `createdAt`
	// se persiste como epoch en segundos, así que reconstruir la SK desde él perdía los
	// nanosegundos y `ClaimReadAll` apuntaba a una clave distinta: el trabajo se leía
	// pero nunca se reclamaba (defecto real de E063). Se conserva el epoch solo como
	// respaldo para items antiguos sin SK válida.
	resto := strings.TrimPrefix(stringAttribute(item["SK"]), ReadAllQueueSKPrefix)
	if corte := strings.LastIndex(resto, "#"); corte > 0 {
		if creado, err := time.Parse(readAllQueueTimeLayout, resto[:corte]); err == nil {
			trabajo.CreatedAt = creado.UTC()
		}
	}
	if trabajo.CreatedAt.IsZero() {
		if creado, ok := epochTimeAttribute(item["createdAt"]); ok {
			trabajo.CreatedAt = creado
		}
	}
	if hasta, ok := epochTimeAttribute(item["leaseUntil"]); ok {
		trabajo.LeaseUntil = hasta
	}
	if fence, ok := attributeInt(item["fence"]); ok && fence >= 0 {
		trabajo.Fence = fence
	}
	if intentos, ok := attributeInt(item["attempts"]); ok && intentos > 0 && intentos < 1<<31 {
		trabajo.Attempts = int(intentos)
	}
	return trabajo, true
}
