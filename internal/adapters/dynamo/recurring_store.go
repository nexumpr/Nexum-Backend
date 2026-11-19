package dynamo

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	appRecurring "github.com/nexumpr/Nexum-Backend/internal/application/recurring"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
	domainRecurring "github.com/nexumpr/Nexum-Backend/internal/domain/recurring"
)

// RecurringAPI es el subconjunto del cliente DynamoDB que usan E032–E036.
//
// Se declara aquí —y no se reutiliza `DynamoAPI`— para que el compilador impida las
// operaciones que este dominio no debe usar: no hay `Scan`, no hay `BatchWriteItem` y no hay
// borrado físico de reglas. E036 desactiva; nunca elimina el ítem.
type RecurringAPI interface {
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
	TransactWriteItems(ctx context.Context, params *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error)
}

const (
	// recurringEntityType identifica la regla recurrente. Es el MISMO valor que ya escribe
	// E001 en el alta para los compromisos fijos: no se crea una segunda entidad con la misma
	// clave. Los ítems heredados del alta se proyectan al contrato vigente sin inventar datos
	// (D-E032-01); ver `recurringFromItem`.
	recurringEntityType = "RECURRING_PAYMENT"

	// recurringIdemEntityType identifica el registro de idempotencia por usuario.
	recurringIdemEntityType = "IDEMPOTENCY_RECORD"

	// recurringCreateStatus es el código que el registro de idempotencia documenta. Se declara
	// como constante para no acoplar el adaptador al paquete `net/http`.
	recurringCreateStatus int64 = 201

	// recurringUpdateAttempts acota los reintentos del compare-and-swap de E035 y E036.
	recurringUpdateAttempts = 3
)

// RecurringStore implementa `appRecurring.Store` sobre la tabla única existente.
//
// Patrón físico (A-060, MATRIZ_ACCESOS_I040):
//
//	PK = USER#<userId>   SK = RECURRING#<id>
//	PK = USER#<userId>   SK = IDEM#E033_CREATE_RECURRING#<sha256(operacion:clave)>
//
// Ambas claves viven en la partición del titular que la política IAM vigente ya autoriza
// (`LeadingKeys` `USER#*`): esta unidad no requiere tabla, índice ni permiso nuevo.
type RecurringStore struct {
	client RecurringAPI
	table  string
	now    func() time.Time
}

// NewRecurringStore construye el adaptador. Sin cliente o tabla todas las operaciones fallan
// cerradas: nunca se sustituyen por memoria local.
func NewRecurringStore(client RecurringAPI, table string) *RecurringStore {
	return &RecurringStore{
		client: client,
		table:  strings.TrimSpace(table),
		now:    func() time.Time { return time.Now().UTC() },
	}
}

// WithClock permite inyectar un reloj determinista. Sólo lo usan las pruebas y la proyección
// de la fecha de vencimiento de un ítem heredado (D-E032-01); no altera ningún dato escrito.
func (s *RecurringStore) WithClock(now func() time.Time) *RecurringStore {
	if now != nil {
		s.now = now
	}
	return s
}

// hoy devuelve la fecha civil actual del adaptador.
func (s *RecurringStore) hoy() time.Time {
	if s == nil || s.now == nil {
		return domainRecurring.DateOnly(time.Now().UTC())
	}
	return domainRecurring.DateOnly(s.now().UTC())
}

// Create escribe la regla y el registro de idempotencia en UNA transacción (E033).
//
// Condiciones: `attribute_not_exists(PK)` sobre el recurso y sobre el registro de
// idempotencia (con la salvedad de un registro vencido, que puede reutilizarse). Si el
// registro ya existe con el mismo `payloadHash`, se devuelve el cuerpo almacenado
// (`replayed=true`) sin crear nada; con otro `payloadHash`, ErrConflict (409).
func (s *RecurringStore) Create(ctx context.Context, write appRecurring.CreateWrite) (string, []byte, bool, error) {
	if s == nil || s.client == nil || s.table == "" {
		return "", nil, false, appRecurring.ErrUnavailable
	}
	userID := strings.TrimSpace(write.UserID)
	if userID == "" || strings.TrimSpace(write.Payment.ID) == "" || len(write.Body) == 0 {
		return "", nil, false, appRecurring.ErrUnavailable
	}
	if strings.TrimSpace(write.Operation) == "" || strings.TrimSpace(write.PayloadHash) == "" {
		return "", nil, false, appRecurring.ErrUnavailable
	}

	ttl := write.TTL
	if ttl <= 0 {
		ttl = appRecurring.IdempotencyTTL
	}
	now := write.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	creadoISO := now.Format(time.RFC3339)

	pk, sk := domainDynamo.RecurringKeys(userID, write.Payment.ID)
	idemPK, idemSK := domainDynamo.UserIdempotencyKeys(userID, write.Operation, write.IdempotencyKey)

	recurso := recurringItem(pk, sk, userID, write.Payment, creadoISO)

	idempotencia := map[string]types.AttributeValue{
		"PK":             stringAttr(idemPK),
		"SK":             stringAttr(idemSK),
		"entityType":     stringAttr(recurringIdemEntityType),
		"operation":      stringAttr(write.Operation),
		"payloadHash":    stringAttr(write.PayloadHash),
		"responseStatus": numberAttr(recurringCreateStatus),
		"responseBody":   stringAttr(string(write.Body)),
		"resourceId":     stringAttr(write.Payment.ID),
		"location":       stringAttr("/api/v1/recurring-payments/" + write.Payment.ID),
		"createdAt":      stringAttr(creadoISO),
		"expiresAt":      numberAttr(now.Add(ttl).Unix()),
	}

	entrada := &dynamodb.TransactWriteItemsInput{TransactItems: []types.TransactWriteItem{
		{Put: &types.Put{
			TableName:           aws.String(s.table),
			Item:                recurso,
			ConditionExpression: aws.String("attribute_not_exists(PK)"),
		}},
		{Put: &types.Put{
			TableName:                 aws.String(s.table),
			Item:                      idempotencia,
			ConditionExpression:       aws.String("attribute_not_exists(PK) OR expiresAt < :now"),
			ExpressionAttributeValues: map[string]types.AttributeValue{":now": numberAttr(now.Unix())},
		}},
	}}

	_, err := s.client.TransactWriteItems(ctx, entrada)
	if err == nil {
		return write.Payment.ID, write.Body, false, nil
	}
	if !isConditionalCancel(err) {
		return "", nil, false, appRecurring.ErrUnavailable
	}

	razones := cancellationReasons(err)
	if len(razones) > 1 && razonCondicional(razones[1]) {
		// La clave de idempotencia ya estaba tomada: replay o conflicto.
		return s.replayOrConflict(ctx, write, idemPK, idemSK, now)
	}
	if len(razones) > 0 && razonCondicional(razones[0]) {
		// Colisión del identificador generado: no se sobrescribe nada.
		return "", nil, false, appRecurring.ErrConflict
	}
	return "", nil, false, appRecurring.ErrUnavailable
}

// replayOrConflict decide entre devolver la respuesta original o un 409.
func (s *RecurringStore) replayOrConflict(
	ctx context.Context,
	write appRecurring.CreateWrite,
	idemPK, idemSK string,
	now time.Time,
) (string, []byte, bool, error) {
	salida, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(idemPK),
			"SK": stringAttr(idemSK),
		},
		// Lectura consistente: el replay no puede basarse en una réplica retrasada que aún no
		// ve el registro ya confirmado.
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return "", nil, false, appRecurring.ErrUnavailable
	}
	if salida == nil || len(salida.Item) == 0 {
		// Carrera sin registro visible: no se inventa una respuesta ni se duplica el recurso.
		return "", nil, false, appRecurring.ErrConflict
	}

	if almacenado := stringAttribute(salida.Item["payloadHash"]); almacenado != write.PayloadHash {
		return "", nil, false, appRecurring.ErrConflict
	}
	if expiresAt, ok := attributeInt(salida.Item["expiresAt"]); ok && expiresAt > 0 && now.Unix() >= expiresAt {
		return "", nil, false, appRecurring.ErrConflict
	}

	id := strings.TrimSpace(stringAttribute(salida.Item["resourceId"]))
	cuerpo := stringAttribute(salida.Item["responseBody"])
	if id == "" || cuerpo == "" {
		return "", nil, false, appRecurring.ErrUnavailable
	}
	return id, []byte(cuerpo), true, nil
}

// List consulta la partición del titular por prefijo `RECURRING#` (E032).
//
// La condición de clave es `PK = :pk AND begins_with(SK, :prefix)`; los filtros `active` y
// `type` se aplican en el servidor con `FilterExpression`, nunca descargando la partición
// completa al cliente ni usando `Scan`. La paginación usa `LastEvaluatedKey`, de modo que no
// hay duplicados entre páginas. Con filtros, DynamoDB evalúa `limit` ítems y puede devolver
// menos coincidencias: la página puede ser más corta, pero el cursor siempre avanza.
func (s *RecurringStore) List(ctx context.Context, userID, startSK string, limit int, filter appRecurring.Filter) ([]appRecurring.Payment, string, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, "", appRecurring.ErrUnavailable
	}
	limpioUsuario := strings.TrimSpace(userID)
	if limpioUsuario == "" {
		return nil, "", appRecurring.ErrUnavailable
	}
	if limit <= 0 || limit > appRecurring.MaxLimit {
		return nil, "", appRecurring.ErrUnavailable
	}

	pk := domainDynamo.PrefixUser + limpioUsuario
	valores := map[string]types.AttributeValue{
		":pk":     stringAttr(pk),
		":prefix": stringAttr(domainDynamo.PrefixRecurring),
	}
	nombres := map[string]string{}
	condiciones := make([]string, 0, 2)

	if filter.Active != nil {
		nombres["#active"] = "active"
		condiciones = append(condiciones, "#active = :active")
		valores[":active"] = boolAttr(*filter.Active)
	}
	if filter.Type != "" {
		// `type` es palabra reservada de DynamoDB: siempre por alias.
		nombres["#type"] = "type"
		condiciones = append(condiciones, "#type = :type")
		valores[":type"] = stringAttr(filter.Type)
	}

	entrada := &dynamodb.QueryInput{
		TableName:                 aws.String(s.table),
		KeyConditionExpression:    aws.String("PK = :pk AND begins_with(SK, :prefix)"),
		ExpressionAttributeValues: valores,
		// Lectura consistente: una regla recién creada por otra instancia debe aparecer.
		ConsistentRead:   aws.Bool(true),
		ScanIndexForward: aws.Bool(true),
		Limit:            aws.Int32(int32(limit)),
	}
	if len(nombres) > 0 {
		entrada.ExpressionAttributeNames = nombres
		entrada.FilterExpression = aws.String(strings.Join(condiciones, " AND "))
	}
	if startSK != "" {
		entrada.ExclusiveStartKey = map[string]types.AttributeValue{
			"PK": stringAttr(pk),
			"SK": stringAttr(startSK),
		}
	}

	salida, err := s.client.Query(ctx, entrada)
	if err != nil {
		return nil, "", appRecurring.ErrUnavailable
	}
	if salida == nil {
		return nil, "", nil
	}

	items := make([]appRecurring.Payment, 0, len(salida.Items))
	hoy := s.hoy()
	for _, item := range salida.Items {
		pago, ok := recurringFromItem(item, hoy)
		if !ok {
			// Otra entidad o ítem sin mínimos irrenunciables: no se inventa nada. El cursor ya
			// se calculó sobre la posición física.
			continue
		}
		items = append(items, *pago)
	}

	nextSK := ""
	if len(salida.LastEvaluatedKey) > 0 {
		nextSK = stringAttribute(salida.LastEvaluatedKey["SK"])
	}
	return items, nextSK, nil
}

// Get lee `USER#<id>/RECURRING#<id>` con consistencia fuerte (E034). Devuelve (nil, nil)
// cuando la clave no existe o cuando el ítem no es representable en el contrato vigente.
func (s *RecurringStore) Get(ctx context.Context, userID, paymentID string) (*appRecurring.Payment, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, appRecurring.ErrUnavailable
	}
	limpioUsuario := strings.TrimSpace(userID)
	limpioID := strings.TrimSpace(paymentID)
	if limpioUsuario == "" || limpioID == "" {
		return nil, appRecurring.ErrUnavailable
	}

	pk, sk := domainDynamo.RecurringKeys(limpioUsuario, limpioID)
	salida, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(pk),
			"SK": stringAttr(sk),
		},
		// La versión leída decide el 412 de E035 y la idempotencia de E036.
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return nil, appRecurring.ErrUnavailable
	}
	if salida == nil || len(salida.Item) == 0 {
		return nil, nil
	}
	pago, ok := recurringFromItem(salida.Item, s.hoy())
	if !ok {
		return nil, nil
	}
	return pago, nil
}

// Update aplica una modificación con compare-and-swap por `version` (E035).
//
// Una escritura condicional por intento:
//
//	SET name, amountCents, nextDueDate, active, type, startDate, updatedAt
//	[REMOVE dueDay | SET dueDay]
//	ADD version :one
//	Condición: attribute_exists(PK) AND #version = :expected
//
// Si la condición falla se relee con consistencia fuerte: una versión distinta de la esperada
// es 412 y no se escribe nada; si el ítem sigue en la versión esperada se reintenta el CAS y,
// agotados los intentos, se devuelve 409. Cualquier otro error falla cerrado.
func (s *RecurringStore) Update(ctx context.Context, write appRecurring.UpdateWrite) (*appRecurring.Payment, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, appRecurring.ErrUnavailable
	}
	userID := strings.TrimSpace(write.UserID)
	paymentID := strings.TrimSpace(write.PaymentID)
	if userID == "" || paymentID == "" || write.ExpectedVersion < 1 {
		return nil, appRecurring.ErrUnavailable
	}
	now := write.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}

	pk, sk := domainDynamo.RecurringKeys(userID, paymentID)
	esperada := write.ExpectedVersion

	for intento := 0; intento < recurringUpdateAttempts; intento++ {
		actualizado, err := s.updateOnce(ctx, pk, sk, write.Payment, esperada, now)
		if err == nil {
			return actualizado, nil
		}
		if !errors.Is(err, errRecurringConditional) {
			return nil, appRecurring.ErrUnavailable
		}

		actual, errLectura := s.Get(ctx, userID, paymentID)
		if errLectura != nil {
			return nil, appRecurring.ErrUnavailable
		}
		if actual == nil {
			return nil, appRecurring.ErrNotFound
		}
		if actual.Version != esperada {
			return nil, appRecurring.ErrVersionConflict
		}
	}
	return nil, appRecurring.ErrConflict
}

// errRecurringConditional marca un fallo de condición recuperable dentro del bucle de CAS.
var errRecurringConditional = errors.New("condicion de actualizacion no cumplida")

// updateOnce ejecuta una escritura condicional y devuelve el ítem actualizado.
func (s *RecurringStore) updateOnce(
	ctx context.Context,
	pk, sk string,
	pago appRecurring.Payment,
	esperada int64,
	now time.Time,
) (*appRecurring.Payment, error) {
	expresion := "SET #name = :name, amountCents = :amountCents, nextDueDate = :nextDueDate, " +
		"#active = :active, #type = :type, startDate = :startDate, updatedAt = :updatedAt"
	valores := map[string]types.AttributeValue{
		":name":        stringAttr(pago.Name),
		":amountCents": numberAttr(pago.AmountCents),
		":nextDueDate": stringAttr(domainRecurring.FormatDate(pago.NextDueDate)),
		":active":      boolAttr(pago.Active),
		":type":        stringAttr(pago.Type),
		":startDate":   stringAttr(domainRecurring.FormatDate(pago.StartDate)),
		":updatedAt":   stringAttr(now.Format(time.RFC3339)),
		":expected":    numberAttr(esperada),
		":one":         numberAttr(1),
	}
	nombres := map[string]string{
		"#name":    "name",
		"#active":  "active",
		"#type":    "type",
		"#version": "version",
	}
	if pago.DueDay != nil {
		expresion += ", dueDay = :dueDay"
		valores[":dueDay"] = numberAttr(int64(*pago.DueDay))
	} else {
		// Un `dueDay` ausente borra el atributo: no se conserva un día fijo que el cliente
		// acaba de eliminar.
		expresion += " REMOVE dueDay"
	}
	expresion += " ADD #version :one"

	salida, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(pk),
			"SK": stringAttr(sk),
		},
		UpdateExpression:          aws.String(expresion),
		ConditionExpression:       aws.String("attribute_exists(PK) AND #version = :expected"),
		ExpressionAttributeNames:  nombres,
		ExpressionAttributeValues: valores,
		ReturnValues:              types.ReturnValueAllNew,
	})
	if err != nil {
		if isConditionalCheckFailed(err) {
			return nil, errRecurringConditional
		}
		return nil, appRecurring.ErrUnavailable
	}
	if salida == nil {
		return nil, appRecurring.ErrUnavailable
	}
	actualizado, ok := recurringFromItem(salida.Attributes, s.hoy())
	if !ok {
		return nil, appRecurring.ErrUnavailable
	}
	return actualizado, nil
}

// Deactivate marca la regla como inactiva (E036) sin borrar el ítem ni ningún pago.
//
// Es idempotente: si la regla ya está inactiva devuelve `true` sin incrementar la versión, de
// modo que repetir la petición sigue siendo 204. Devuelve `false` sólo cuando el recurso no
// existe en la partición del titular (404).
func (s *RecurringStore) Deactivate(ctx context.Context, userID, paymentID string, now time.Time) (bool, error) {
	if s == nil || s.client == nil || s.table == "" {
		return false, appRecurring.ErrUnavailable
	}
	limpioUsuario := strings.TrimSpace(userID)
	limpioID := strings.TrimSpace(paymentID)
	if limpioUsuario == "" || limpioID == "" {
		return false, appRecurring.ErrUnavailable
	}
	momento := now.UTC()
	if momento.IsZero() {
		momento = time.Now().UTC()
	}

	pk, sk := domainDynamo.RecurringKeys(limpioUsuario, limpioID)

	for intento := 0; intento < recurringUpdateAttempts; intento++ {
		_, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
			TableName: aws.String(s.table),
			Key: map[string]types.AttributeValue{
				"PK": stringAttr(pk),
				"SK": stringAttr(sk),
			},
			UpdateExpression: aws.String("SET #active = :false, updatedAt = :updatedAt ADD #version :one"),
			// Un ítem heredado del alta (D-E032-01) no tiene `active`: también se desactiva, con
			// lo que pasa a ser un ítem del contrato ya inactivo. `attribute_not_exists(#active)`
			// es parte de la condición, no un valor por defecto silencioso.
			ConditionExpression: aws.String("attribute_exists(PK) AND (attribute_not_exists(#active) OR #active = :true)"),
			ExpressionAttributeNames: map[string]string{
				"#active":  "active",
				"#version": "version",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":false":     boolAttr(false),
				":true":      boolAttr(true),
				":updatedAt": stringAttr(momento.Format(time.RFC3339)),
				":one":       numberAttr(1),
			},
		})
		if err == nil {
			return true, nil
		}
		if !isConditionalCheckFailed(err) {
			return false, appRecurring.ErrUnavailable
		}

		actual, errLectura := s.Get(ctx, limpioUsuario, limpioID)
		if errLectura != nil {
			return false, appRecurring.ErrUnavailable
		}
		if actual == nil {
			return false, nil
		}
		if !actual.Active {
			// Ya estaba inactiva: la operación es idempotente y no se reescribe nada.
			return true, nil
		}
	}
	return false, appRecurring.ErrUnavailable
}

// isConditionalCheckFailed distingue un fallo de condición de un fallo de servicio en
// `UpdateItem`.
func isConditionalCheckFailed(err error) bool {
	var condicional *types.ConditionalCheckFailedException
	return errors.As(err, &condicional)
}

// cancellationReasons extrae las razones de cancelación de una transacción.
func cancellationReasons(err error) []types.CancellationReason {
	var cancelada *types.TransactionCanceledException
	if !errors.As(err, &cancelada) {
		return nil
	}
	return cancelada.CancellationReasons
}

// razonCondicional indica si una razón concreta es un fallo de condición.
func razonCondicional(razon types.CancellationReason) bool {
	return razon.Code != nil && *razon.Code == "ConditionalCheckFailed"
}
