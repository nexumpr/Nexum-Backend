package dynamo

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
)

// MovementsAPI es el subconjunto del cliente DynamoDB que necesitan E027/E028 (y que
// reutilizarán E029/E030/E031). No incluye `Scan`, `PutItem`, `UpdateItem` ni `DeleteItem`: el
// compilador garantiza que este adaptador sólo lee por clave/partición y sólo escribe dentro de
// una transacción condicional (A-060 §2.5, AC-A-060-01).
type MovementsAPI interface {
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	TransactWriteItems(ctx context.Context, params *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error)
}

const (
	// movementEntityType identifica el ítem principal de un movimiento.
	movementEntityType = "USER_MOVEMENT"
	// movementPointerEntityType identifica el localizador O(1) por identificador.
	movementPointerEntityType = "MOVEMENT_LOCATOR"
	// movementIdempotencyEntityType identifica el registro de idempotencia.
	movementIdempotencyEntityType = "IDEMPOTENCY_RECORD"

	// movementPointerPrefix es la SK del localizador. Deliberadamente NO empieza por
	// `MOVEMENT#`, de modo que `begins_with(SK, "MOVEMENT#")` no lo incluye y el listado por
	// partición nunca devuelve punteros.
	//
	// No se añade a `internal/domain/dynamo/keys.go` porque ese archivo es propiedad del
	// orquestador durante la oleada; la forma es la del diseño aprobado.
	movementPointerPrefix = "MOVEMENTPTR#"

	// maxPaginasListado acota cuántas páginas de DynamoDB se evalúan en una sola petición de
	// E027 cuando hay filtro. Sin filtro basta una. Si se alcanza el tope se devuelve un cursor
	// desde la ÚLTIMA CLAVE EVALUADA, de modo que no se pierde ni se duplica ningún elemento.
	maxPaginasListado = 4
)

// MovementsStore implementa `movements.Store` sobre la tabla única existente.
//
// Patrón físico (A-060 §2.3, diseño de la Oleada 2):
//
//	principal    PK = USER#<userId>   SK = MOVEMENT#<occurredAt ordenable>#<movementId>
//	puntero      PK = USER#<userId>   SK = MOVEMENTPTR#<movementId>   (atributo targetSK)
//	idempotencia PK = USER#<userId>   SK = IDEM#E028_CREATE_MOVEMENT#<sha256(operación:clave)>
//
// No crea tablas, índices ni particiones nuevas: todas las claves viven en `USER#<userId>`, que
// la política IAM vigente ya autoriza (`LeadingKeys` `USER#*`) y que la purga de E014 recorre.
type MovementsStore struct {
	client MovementsAPI
	table  string
}

// NewMovementsStore construye el adaptador. Sin cliente o sin tabla, todas las operaciones
// fallan cerradas con ErrUnavailable: nunca se sustituyen por memoria local.
func NewMovementsStore(client MovementsAPI, table string) *MovementsStore {
	return &MovementsStore{client: client, table: strings.TrimSpace(table)}
}

// MovementPointerKeys devuelve la PK/SK del localizador O(1) de un movimiento. Se exporta para
// que E029/E030/E031 y las pruebas usen exactamente la misma forma.
func MovementPointerKeys(userID, movementID string) (pk, sk string) {
	return domainDynamo.PrefixUser + strings.TrimSpace(userID), movementPointerPrefix + strings.TrimSpace(movementID)
}

// CreateMovement ejecuta el alta de E028 en UNA transacción condicional de tres acciones:
//
//  1. ítem principal, con `attribute_not_exists(PK)` (la clave incluye el identificador: una
//     colisión sería un fallo de generación, no una escritura duplicada);
//  2. puntero por identificador, con `attribute_not_exists(PK)`;
//  3. registro de idempotencia, con `attribute_not_exists(PK) OR expiresAt < :now`.
//
// Antes de escribir se lee el registro con consistencia fuerte: si existe y el `payloadHash`
// coincide se devuelve la respuesta ORIGINAL sin volver a escribir (replay), y si difiere se
// devuelve ErrConflict (409). El cuerpo se guarda ya serializado, de modo que el replay es byte
// a byte idéntico y no puede divergir del 201 original.
func (s *MovementsStore) CreateMovement(ctx context.Context, write appMovements.CreateWrite) (*appMovements.CreateResult, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, appMovements.ErrUnavailable
	}
	userID := strings.TrimSpace(write.UserID)
	movimiento := write.Movement
	if userID == "" || strings.TrimSpace(movimiento.ID) == "" || write.PayloadHash == "" ||
		len(write.ResponseBody) == 0 || write.Status < 100 || write.Status > 599 {
		return nil, appMovements.ErrUnavailable
	}
	operacion := strings.TrimSpace(write.Operation)
	if operacion == "" {
		operacion = appMovements.OperationCreateMovement
	}
	ahora := write.Now.UTC()

	idemPK, idemSK := domainDynamo.UserIdempotencyKeys(userID, operacion, write.IdempotencyKey)

	// 1. Lectura consistente del registro de idempotencia.
	existente, err := s.leerIdempotencia(ctx, idemPK, idemSK, userID)
	if err != nil {
		return nil, err
	}
	if existente != nil && !existente.expirado(ahora) {
		return existente.replay(write.PayloadHash)
	}

	// 2. Transacción atómica: recurso + registro. Nunca se confirma uno sin el otro.
	pk, sk, gsi1pk, gsi1sk := domainDynamo.MovementKeys(userID, movimiento.ID, movimiento.Type, movimiento.OccurredAt)
	ptrPK, ptrSK := MovementPointerKeys(userID, movimiento.ID)
	ahoraISO := ahora.Format(time.RFC3339)

	movementItem := map[string]types.AttributeValue{
		"PK":          stringAttr(pk),
		"SK":          stringAttr(sk),
		"entityType":  stringAttr(movementEntityType),
		"userId":      stringAttr(userID),
		"movementId":  stringAttr(movimiento.ID),
		"type":        stringAttr(movimiento.Type),
		"amountCents": numberAttr(movimiento.AmountCents),
		"currency":    stringAttr(movimiento.Currency),
		"categoryId":  stringAttr(movimiento.CategoryID),
		"description": stringAttr(movimiento.Description),
		"occurredAt":  stringAttr(appMovements.SortableTime(movimiento.OccurredAt)),
		"version":     numberAttr(movimiento.Version),
		"createdAt":   stringAttr(ahoraISO),
		"updatedAt":   stringAttr(ahoraISO),
		"GSI1PK":      stringAttr(gsi1pk),
		"GSI1SK":      stringAttr(gsi1sk),
	}
	if v := strings.TrimSpace(movimiento.ContributionID); v != "" {
		movementItem["contributionId"] = stringAttr(v)
	}
	if v := strings.TrimSpace(movimiento.RecurringPaymentID); v != "" {
		movementItem["recurringPaymentId"] = stringAttr(v)
	}

	pointerItem := map[string]types.AttributeValue{
		"PK":         stringAttr(ptrPK),
		"SK":         stringAttr(ptrSK),
		"entityType": stringAttr(movementPointerEntityType),
		"userId":     stringAttr(userID),
		"movementId": stringAttr(movimiento.ID),
		"targetSK":   stringAttr(sk),
		"createdAt":  stringAttr(ahoraISO),
		"updatedAt":  stringAttr(ahoraISO),
	}

	idemItem := map[string]types.AttributeValue{
		"PK":           stringAttr(idemPK),
		"SK":           stringAttr(idemSK),
		"entityType":   stringAttr(movementIdempotencyEntityType),
		"userId":       stringAttr(userID),
		"operation":    stringAttr(operacion),
		"payloadHash":  stringAttr(write.PayloadHash),
		"httpStatus":   numberAttr(int64(write.Status)),
		"responseBody": stringAttr(string(write.ResponseBody)),
		"location":     stringAttr(write.Location),
		"resourceId":   stringAttr(movimiento.ID),
		"createdAt":    stringAttr(ahoraISO),
		"expiresAt":    numberAttr(write.ExpiresAt.UTC().Unix()),
	}

	items := []types.TransactWriteItem{
		{Put: &types.Put{
			TableName:           aws.String(s.table),
			Item:                movementItem,
			ConditionExpression: aws.String("attribute_not_exists(PK)"),
		}},
		{Put: &types.Put{
			TableName:           aws.String(s.table),
			Item:                pointerItem,
			ConditionExpression: aws.String("attribute_not_exists(PK)"),
		}},
		{Put: &types.Put{
			TableName:           aws.String(s.table),
			Item:                idemItem,
			ConditionExpression: aws.String("(attribute_not_exists(PK) OR expiresAt < :now)"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":now": numberAttr(ahora.Unix()),
			},
		}},
	}

	_, err = s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: items})
	if err == nil {
		return &appMovements.CreateResult{
			Status:   write.Status,
			Body:     append([]byte(nil), write.ResponseBody...),
			Location: write.Location,
		}, nil
	}

	var cancelada *types.TransactionCanceledException
	if !errors.As(err, &cancelada) {
		return nil, appMovements.ErrUnavailable
	}
	if transactionItemFailed(err, 2) {
		// El registro de idempotencia apareció entre la lectura y la transacción: lo escrito
		// por el ganador es la autoridad y NO se ha creado un segundo movimiento.
		return s.resolverConflictoIdempotencia(ctx, idemPK, idemSK, userID, write)
	}
	if transactionItemFailed(err, 0) || transactionItemFailed(err, 1) {
		return nil, appMovements.ErrMovementIDCollision
	}
	// Cancelación por otra causa (conflicto de transacción, throttling): nada se escribió.
	return nil, appMovements.ErrUnavailable
}

// resolverConflictoIdempotencia relee el registro tras una cancelación por su condición.
//
// Mismo contenido -> replay idéntico; contenido distinto -> 409; registro ausente (carrera en la
// que el ganador aún no es visible) -> 409 sin duplicar. Nunca se escribe dos veces.
func (s *MovementsStore) resolverConflictoIdempotencia(ctx context.Context, pk, sk, userID string, write appMovements.CreateWrite) (*appMovements.CreateResult, error) {
	existente, err := s.leerIdempotencia(ctx, pk, sk, userID)
	if err != nil {
		return nil, err
	}
	if existente == nil {
		return nil, appMovements.ErrConflict
	}
	if existente.expirado(write.Now.UTC()) {
		return nil, appMovements.ErrConflict
	}
	return existente.replay(write.PayloadHash)
}

// registroIdempotencia es la lectura mínima del registro persistido.
type registroIdempotencia struct {
	payloadHash string
	status      int
	cuerpo      string
	ubicacion   string
	expiraEn    int64
	tieneExpira bool
}

// expirado indica si el vencimiento lógico ya pasó. Un registro sin `expiresAt` se considera
// vigente: nunca se reutiliza una clave por un atributo ausente.
func (r *registroIdempotencia) expirado(ahora time.Time) bool {
	if r == nil || !r.tieneExpira {
		return false
	}
	return ahora.Unix() >= r.expiraEn
}

// replay reproduce la respuesta original. Un `payloadHash` distinto es un conflicto (409): la
// misma clave no puede describir dos operaciones.
func (r *registroIdempotencia) replay(payloadHash string) (*appMovements.CreateResult, error) {
	if r == nil {
		return nil, appMovements.ErrInternal
	}
	if r.payloadHash != payloadHash {
		return nil, appMovements.ErrConflict
	}
	if r.status < 100 || r.status > 599 || r.cuerpo == "" {
		return nil, appMovements.ErrInternal
	}
	return &appMovements.CreateResult{
		Status:   r.status,
		Body:     []byte(r.cuerpo),
		Location: r.ubicacion,
		Replayed: true,
	}, nil
}

// leerIdempotencia lee el registro con consistencia fuerte. Devuelve (nil, nil) cuando la clave
// no existe o la ocupa otra entidad: ausencia de registro no es un error.
func (s *MovementsStore) leerIdempotencia(ctx context.Context, pk, sk, userID string) (*registroIdempotencia, error) {
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(pk),
			"SK": stringAttr(sk),
		},
		// La decisión replay/crear no puede basarse en una réplica retrasada.
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return nil, appMovements.ErrUnavailable
	}
	if out == nil || len(out.Item) == 0 {
		return nil, nil
	}
	if entidad := stringAttribute(out.Item["entityType"]); entidad != "" && entidad != movementIdempotencyEntityType {
		return nil, nil
	}
	if titular := stringAttribute(out.Item["userId"]); titular != "" && titular != userID {
		// Defensa en profundidad: la PK ya aísla por usuario.
		return nil, nil
	}
	return idempotenciaDesdeItem(out.Item), nil
}

// idempotenciaDesdeItem proyecta el ítem leído. `httpStatus` y `expiresAt` se aceptan como
// Number o como texto porque ambos aparecen en la tabla histórica.
func idempotenciaDesdeItem(item map[string]types.AttributeValue) *registroIdempotencia {
	registro := &registroIdempotencia{
		payloadHash: stringAttribute(item["payloadHash"]),
		cuerpo:      stringAttribute(item["responseBody"]),
		ubicacion:   stringAttribute(item["location"]),
	}
	if estado, err := strconv.Atoi(strings.TrimSpace(stringAttribute(item["httpStatus"]))); err == nil {
		registro.status = estado
	}
	if expira, err := strconv.ParseInt(strings.TrimSpace(stringAttribute(item["expiresAt"])), 10, 64); err == nil {
		registro.expiraEn = expira
		registro.tieneExpira = true
	}
	return registro
}

// filaMovimiento conserva la SK junto al movimiento: el cursor de E027 se construye con la clave
// física exacta, no con una reconstrucción a partir de los atributos.
type filaMovimiento struct {
	sk         string
	movimiento appMovements.Movement
}

// ListMovements consulta la partición del titular en orden ascendente por SK.
//
// La condición de clave es siempre `PK = USER#<id> AND begins_with(SK, "MOVEMENT#")`, con cotas
// opcionales de `occurredAt` (`from`/`to`) aplicadas sobre la propia SK: no hay `Scan` y no se
// consulta ninguna partición ajena.
//
// El filtro `type` se aplica con `FilterExpression` sobre la misma partición. Se hace así, y no
// con el índice GSI1, por dos motivos: la lectura es FUERTEMENTE CONSISTENTE (un movimiento
// recién creado aparece en el listado inmediatamente) y el orden por SK es único para todas las
// páginas. El coste es que una página puede requerir varias consultas; se acota con
// `maxPaginasListado` y, al alcanzarlo, se devuelve un cursor desde la última clave EVALUADA, de
// modo que la paginación sigue sin perder ni duplicar elementos.
func (s *MovementsStore) ListMovements(ctx context.Context, query appMovements.ListQuery, startSK string, limit int) ([]appMovements.Movement, string, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, "", appMovements.ErrUnavailable
	}
	userID := strings.TrimSpace(query.UserID)
	if userID == "" {
		return nil, "", appMovements.ErrUnavailable
	}
	if limit < 1 || limit > appMovements.MaxLimit {
		return nil, "", appMovements.ErrUnavailable
	}

	pk := domainDynamo.PrefixUser + userID
	condicion := "PK = :pk AND begins_with(SK, :prefijo)"
	valores := map[string]types.AttributeValue{
		":pk":      stringAttr(pk),
		":prefijo": stringAttr(domainDynamo.PrefixMovement),
	}
	if query.HasFrom {
		condicion += " AND SK >= :desde"
		valores[":desde"] = stringAttr(domainDynamo.PrefixMovement + appMovements.SortableTime(query.From))
	}
	if query.HasTo {
		// `<` contra el milisegundo siguiente: incluye todo lo ocurrido en `to` exacto sin
		// depender de un carácter centinela.
		condicion += " AND SK < :hasta"
		valores[":hasta"] = stringAttr(domainDynamo.PrefixMovement + appMovements.SortableTime(query.To.Add(time.Millisecond)))
	}

	var filtro *string
	var nombres map[string]string
	if query.Type != "" {
		filtro = aws.String("#type = :type")
		nombres = map[string]string{"#type": "type"}
		valores[":type"] = stringAttr(query.Type)
	}

	// Sin filtro se pide una fila de más: es la forma barata de saber si hay página siguiente.
	// Con filtro el `Limit` cuenta ítems EVALUADOS, así que se pide exactamente `limit` y se
	// itera hasta llenar la página.
	pedido := limit
	if filtro == nil {
		pedido = limit + 1
	}

	filas := make([]filaMovimiento, 0, limit+1)
	desde := strings.TrimSpace(startSK)
	continuacion := ""
	for pagina := 0; pagina < maxPaginasListado; pagina++ {
		entrada := &dynamodb.QueryInput{
			TableName:                 aws.String(s.table),
			KeyConditionExpression:    aws.String(condicion),
			FilterExpression:          filtro,
			ExpressionAttributeNames:  nombres,
			ExpressionAttributeValues: valores,
			// Consistencia fuerte: el listado debe reflejar lo ya confirmado por este u otro
			// nodo (E027 T05). El índice GSI1 no la admite; esta consulta sí.
			ConsistentRead:   aws.Bool(true),
			ScanIndexForward: aws.Bool(true),
			Limit:            aws.Int32(int32(pedido)),
		}
		if desde != "" {
			entrada.ExclusiveStartKey = map[string]types.AttributeValue{
				"PK": stringAttr(pk),
				"SK": stringAttr(desde),
			}
		}

		salida, err := s.client.Query(ctx, entrada)
		if err != nil {
			return nil, "", appMovements.ErrUnavailable
		}
		if salida == nil {
			break
		}
		for _, item := range salida.Items {
			movimiento, ok := movimientoDesdeItem(item, userID)
			if !ok {
				// Ítem con otra entidad o sin los atributos mínimos: no se inventa un
				// movimiento. El cursor se calcula sobre posiciones físicas, no sobre estas
				// filas, así que no se saltan datos.
				continue
			}
			filas = append(filas, filaMovimiento{sk: stringAttribute(item["SK"]), movimiento: movimiento})
		}

		if len(filas) > limit {
			filas = filas[:limit]
			return movimientosDe(filas), filas[limit-1].sk, nil
		}
		if salida.LastEvaluatedKey == nil {
			return movimientosDe(filas), "", nil
		}
		siguiente := stringAttribute(salida.LastEvaluatedKey["SK"])
		if siguiente == "" || siguiente == desde {
			// Sin avance no se puede garantizar progreso: se termina la página en lugar de
			// emitir un cursor que devolvería lo mismo indefinidamente.
			return movimientosDe(filas), "", nil
		}
		desde = siguiente
		continuacion = siguiente
	}

	return movimientosDe(filas), continuacion, nil
}

// movimientosDe proyecta las filas a la lista pública.
func movimientosDe(filas []filaMovimiento) []appMovements.Movement {
	salida := make([]appMovements.Movement, 0, len(filas))
	for _, fila := range filas {
		salida = append(salida, fila.movimiento)
	}
	return salida
}

// movimientoDesdeItem interpreta un ítem como movimiento. Devuelve false si la clave la ocupa
// otra entidad o si falta un dato sin el que no puede cumplirse el contrato.
func movimientoDesdeItem(item map[string]types.AttributeValue, userID string) (appMovements.Movement, bool) {
	if len(item) == 0 {
		return appMovements.Movement{}, false
	}
	if entidad := stringAttribute(item["entityType"]); entidad != "" && entidad != movementEntityType {
		return appMovements.Movement{}, false
	}
	if titular := stringAttribute(item["userId"]); titular != "" && titular != userID {
		return appMovements.Movement{}, false
	}

	sk := stringAttribute(item["SK"])
	id := strings.TrimSpace(stringAttribute(item["movementId"]))
	if id == "" {
		id = movementIDFromSK(sk)
	}
	tipo := strings.TrimSpace(stringAttribute(item["type"]))
	centavos, okImporte := attributeInt(item["amountCents"])
	if id == "" || tipo == "" || !okImporte {
		return appMovements.Movement{}, false
	}

	ocurrido, okFecha := appMovements.ParseStoredOccurredAt(stringAttribute(item["occurredAt"]))
	if !okFecha {
		ocurrido, okFecha = occurredAtFromSK(sk)
		if !okFecha {
			return appMovements.Movement{}, false
		}
	}

	version, okVersion := attributeInt(item["version"])
	if !okVersion || version < 1 {
		// Un ítem histórico sin versión se interpreta como la versión inicial: no se inventa
		// un CAS que nunca existió ni se oculta el movimiento.
		version = 1
	}

	return appMovements.Movement{
		ID:                 id,
		Type:               tipo,
		AmountCents:        centavos,
		Currency:           stringAttribute(item["currency"]),
		CategoryID:         stringAttribute(item["categoryId"]),
		Description:        stringAttribute(item["description"]),
		OccurredAt:         ocurrido,
		Version:            version,
		ContributionID:     stringAttribute(item["contributionId"]),
		RecurringPaymentID: stringAttribute(item["recurringPaymentId"]),
	}, true
}

// movementIDFromSK extrae el identificador final de `MOVEMENT#<fecha>#<id>`.
func movementIDFromSK(sk string) string {
	resto := strings.TrimPrefix(sk, domainDynamo.PrefixMovement)
	indice := strings.LastIndex(resto, "#")
	if indice < 0 || indice == len(resto)-1 {
		return ""
	}
	return resto[indice+1:]
}

// occurredAtFromSK extrae la marca ordenable de `MOVEMENT#<fecha>#<id>`.
func occurredAtFromSK(sk string) (time.Time, bool) {
	resto := strings.TrimPrefix(sk, domainDynamo.PrefixMovement)
	indice := strings.LastIndex(resto, "#")
	if indice <= 0 {
		return time.Time{}, false
	}
	return appMovements.ParseStoredOccurredAt(resto[:indice])
}
