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

	// movementDeletedByUser es el único valor admitido en `deletedBy` del tombstone de E031.
	// Es un literal fijo: nunca se persiste sesión, token ni correo (R-31-04).
	movementDeletedByUser = "USER"

	// filtroNoBorrado es la condición de exclusión de tombstones que comparten E027 y E051.
	// La ausencia del atributo significa «no borrado»: cubre todos los movimientos históricos
	// sin backfill (DECISION_E031_BORRADO §5).
	filtroNoBorrado = "(attribute_not_exists(#deleted) OR #deleted = :false)"
)

// deletedFilterNames son los alias que exige `filtroNoBorrado`.
func deletedFilterNames() map[string]string {
	return map[string]string{"#deleted": "deleted"}
}

// deletedFilterValues añade los valores que exige `filtroNoBorrado`.
func deletedFilterValues(valores map[string]types.AttributeValue) map[string]types.AttributeValue {
	if valores == nil {
		valores = map[string]types.AttributeValue{}
	}
	valores[":false"] = &types.AttributeValueMemberBOOL{Value: false}
	return valores
}

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
	movementItem, _, sk := movementRecord(userID, movimiento, ahora, ahora)
	ptrPK, ptrSK := MovementPointerKeys(userID, movimiento.ID)
	ahoraISO := ahora.Format(time.RFC3339)

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
	// El registro de idempotencia es la autoridad de la carrera: si existe, otra peticion
	// gano y esta debe recibir el replay (o el 409 si el payload difiere). No se depende
	// del orden ni del indice de CancellationReasons: DynamoDB los reporta por elemento y
	// el resultado debe ser identico con cualquier orden.
	existenteCarrera, errLectura := s.leerIdempotencia(ctx, idemPK, idemSK, userID)
	if errLectura != nil {
		return nil, errLectura
	}
	if existenteCarrera != nil {
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
//
// El filtro de borrado (`attribute_not_exists(deleted) OR deleted = false`) se aplica SIEMPRE:
// los tombstones de E031 no se listan ni con filtros `type`/`from`/`to` ni en la última página.
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

	var filtros []string
	nombres := deletedFilterNames()
	valores = deletedFilterValues(valores)
	filtros = append(filtros, filtroNoBorrado)
	if query.Type != "" {
		filtros = append(filtros, "#type = :type")
		nombres["#type"] = "type"
		valores[":type"] = stringAttr(query.Type)
	}
	filtro := strings.Join(filtros, " AND ")

	// Sin filtro de tipo, la condición de borrado ya obliga a evaluar más de una fila por página:
	// se pide siempre una fila de más para saber, sin una consulta extra, si hay página siguiente.
	// El `Limit` cuenta ítems EVALUADOS, así que se itera hasta llenar la página y, al alcanzar el
	// tope de páginas, se devuelve un cursor desde la última clave evaluada.
	pedido := limit + 1

	filas := make([]filaMovimiento, 0, limit+1)
	desde := strings.TrimSpace(startSK)
	continuacion := ""
	for pagina := 0; pagina < maxPaginasListado; pagina++ {
		entrada := &dynamodb.QueryInput{
			TableName:                 aws.String(s.table),
			KeyConditionExpression:    aws.String(condicion),
			FilterExpression:          aws.String(filtro),
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
			if !ok || movimiento.Deleted {
				// Ítem con otra entidad, sin los atributos mínimos o tombstone de E031: no se
				// inventa un movimiento. El cursor se calcula sobre posiciones físicas, no sobre
				// estas filas, así que no se saltan datos.
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

	creado, _ := attributeISOTime(item["createdAt"])

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
		CreatedAt:          creado,
		// `deleted` ausente = no borrado: así se leen todos los movimientos históricos escritos
		// antes de E031, sin migración ni backfill.
		Deleted: boolAttribute(item["deleted"]),
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

// movementRecord construye el ítem principal completo y devuelve también su PK y su SK, para que
// el llamador referencie la posición física sin recalcularla. Es la ÚNICA definición del ítem:
// el alta y el movimiento de E030 escriben exactamente los mismos atributos.
func movementRecord(userID string, m appMovements.Movement, creado, actualizado time.Time) (map[string]types.AttributeValue, string, string) {
	pk, sk, gsi1pk, gsi1sk := domainDynamo.MovementKeys(userID, m.ID, m.Type, m.OccurredAt)
	item := map[string]types.AttributeValue{
		"PK":          stringAttr(pk),
		"SK":          stringAttr(sk),
		"entityType":  stringAttr(movementEntityType),
		"userId":      stringAttr(userID),
		"movementId":  stringAttr(m.ID),
		"type":        stringAttr(m.Type),
		"amountCents": numberAttr(m.AmountCents),
		"currency":    stringAttr(m.Currency),
		"categoryId":  stringAttr(m.CategoryID),
		"description": stringAttr(m.Description),
		"occurredAt":  stringAttr(appMovements.SortableTime(m.OccurredAt)),
		"version":     numberAttr(m.Version),
		"createdAt":   stringAttr(creado.UTC().Format(time.RFC3339)),
		"updatedAt":   stringAttr(actualizado.UTC().Format(time.RFC3339)),
		"GSI1PK":      stringAttr(gsi1pk),
		"GSI1SK":      stringAttr(gsi1sk),
	}
	if v := strings.TrimSpace(m.ContributionID); v != "" {
		item["contributionId"] = stringAttr(v)
	}
	if v := strings.TrimSpace(m.RecurringPaymentID); v != "" {
		item["recurringPaymentId"] = stringAttr(v)
	}
	return item, pk, sk
}

// getItem lee una clave exacta con consistencia fuerte. Devuelve (nil, nil) cuando no existe.
func (s *MovementsStore) getItem(ctx context.Context, pk, sk string) (map[string]types.AttributeValue, error) {
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(pk),
			"SK": stringAttr(sk),
		},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return nil, appMovements.ErrUnavailable
	}
	if out == nil || len(out.Item) == 0 {
		return nil, nil
	}
	return out.Item, nil
}

// LoadMovement resuelve un movimiento propio con dos lecturas exactas (E029):
//
//	USER#<userId>/MOVEMENTPTR#<movementId>  ->  targetSK
//	USER#<userId>/<targetSK>                ->  movimiento
//
// Es O(1) y nunca enumera la partición. Devuelve (nil, nil) cuando el localizador no existe, la
// clave la ocupa otra entidad, el titular del ítem no coincide o el principal desapareció: todos
// esos casos son indistinguibles para el cliente (404) y no revelan existencia ajena.
func (s *MovementsStore) LoadMovement(ctx context.Context, userID, movementID string) (*appMovements.Movement, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, appMovements.ErrUnavailable
	}
	titular := strings.TrimSpace(userID)
	id := strings.TrimSpace(movementID)
	if titular == "" || id == "" {
		return nil, appMovements.ErrUnavailable
	}

	ptrPK, ptrSK := MovementPointerKeys(titular, id)
	puntero, err := s.getItem(ctx, ptrPK, ptrSK)
	if err != nil {
		return nil, err
	}
	if puntero == nil {
		return nil, nil
	}
	if entidad := stringAttribute(puntero["entityType"]); entidad != "" && entidad != movementPointerEntityType {
		return nil, nil
	}
	if dueno := stringAttribute(puntero["userId"]); dueno != "" && dueno != titular {
		return nil, nil
	}
	objetivo := strings.TrimSpace(stringAttribute(puntero["targetSK"]))
	if !strings.HasPrefix(objetivo, domainDynamo.PrefixMovement) {
		// Localizador sin destino utilizable: no se adivina la posición del ítem.
		return nil, nil
	}

	principal, err := s.getItem(ctx, domainDynamo.PrefixUser+titular, objetivo)
	if err != nil {
		return nil, err
	}
	if principal == nil {
		return nil, nil
	}
	movimiento, ok := movimientoDesdeItem(principal, titular)
	if !ok || movimiento.ID != id {
		return nil, nil
	}
	return &movimiento, nil
}

// ReplaceMovement aplica la modificación de E030 con compare-and-swap por versión.
//
// Dos caminos, ambos en UNA transacción (nunca hay estado intermedio visible):
//
//   - la SK no cambia: un único `Update` condicionado a `#version = :expected` que fija
//     `categoryId`, `description`, `occurredAt`, `updatedAt` y la versión siguiente;
//   - la SK cambia (`occurredAt` es parte de la clave): `Put` del ítem nuevo con
//     `attribute_not_exists(PK)`, `Delete` del ítem viejo condicionado a la versión esperada y
//     `Update` del puntero condicionado a que siga apuntando a la SK vieja. Si otro escritor movió
//     el ítem, la transacción se cancela entera.
//
// Una cancelación por condición se traduce en ErrConflict: el servicio relee y decide entre 412,
// 404 y 409 sin haber escrito nada.
func (s *MovementsStore) ReplaceMovement(ctx context.Context, write appMovements.ReplaceWrite) (*appMovements.Movement, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, appMovements.ErrUnavailable
	}
	titular := strings.TrimSpace(write.UserID)
	id := strings.TrimSpace(write.MovementID)
	if titular == "" || id == "" || write.ExpectedVersion < 1 {
		return nil, appMovements.ErrUnavailable
	}
	anterior := write.Previous
	siguiente := write.Next
	if siguiente.ID != id {
		// Invariante del caso de uso: el identificador no cambia en una edición.
		return nil, appMovements.ErrUnavailable
	}
	if siguiente.Version != write.ExpectedVersion+1 {
		// La versión esperada por el cliente ya no es la vigente: es un conflicto de
		// versión (412), no un fallo de dependencia.
		return nil, appMovements.ErrConflict
	}

	ahora := write.Now.UTC()
	creado := anterior.CreatedAt
	if creado.IsZero() {
		creado = ahora
	}
	item, _, skNueva := movementRecord(titular, siguiente, creado, ahora)
	pk := domainDynamo.PrefixUser + titular
	_, skVieja, _, _ := domainDynamo.MovementKeys(titular, id, anterior.Type, anterior.OccurredAt)
	ptrPK, ptrSK := MovementPointerKeys(titular, id)
	ahoraISO := ahora.Format(time.RFC3339)

	var items []types.TransactWriteItem
	if skVieja == skNueva {
		items = []types.TransactWriteItem{{Update: &types.Update{
			TableName: aws.String(s.table),
			Key: map[string]types.AttributeValue{
				"PK": stringAttr(pk),
				"SK": stringAttr(skVieja),
			},
			UpdateExpression:    aws.String("SET categoryId = :categoria, description = :descripcion, occurredAt = :ocurrido, updatedAt = :actualizado, #version = :siguiente"),
			ConditionExpression: aws.String("attribute_exists(PK) AND #version = :esperada AND " + filtroNoBorrado),
			ExpressionAttributeNames: map[string]string{
				"#version": "version",
				"#deleted": "deleted",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":categoria":   stringAttr(siguiente.CategoryID),
				":descripcion": stringAttr(siguiente.Description),
				":ocurrido":    stringAttr(appMovements.SortableTime(siguiente.OccurredAt)),
				":actualizado": stringAttr(ahoraISO),
				":siguiente":   numberAttr(siguiente.Version),
				":esperada":    numberAttr(write.ExpectedVersion),
				":false":       &types.AttributeValueMemberBOOL{Value: false},
			},
		}}}
	} else {
		items = []types.TransactWriteItem{
			{Put: &types.Put{
				TableName:           aws.String(s.table),
				Item:                item,
				ConditionExpression: aws.String("attribute_not_exists(PK)"),
			}},
			{Delete: &types.Delete{
				TableName: aws.String(s.table),
				Key: map[string]types.AttributeValue{
					"PK": stringAttr(pk),
					"SK": stringAttr(skVieja),
				},
				ConditionExpression: aws.String("attribute_exists(PK) AND #version = :esperada AND " + filtroNoBorrado),
				ExpressionAttributeNames: map[string]string{
					"#version": "version",
					"#deleted": "deleted",
				},
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":esperada": numberAttr(write.ExpectedVersion),
					":false":    &types.AttributeValueMemberBOOL{Value: false},
				},
			}},
			{Update: &types.Update{
				TableName: aws.String(s.table),
				Key: map[string]types.AttributeValue{
					"PK": stringAttr(ptrPK),
					"SK": stringAttr(ptrSK),
				},
				UpdateExpression:    aws.String("SET targetSK = :nueva, updatedAt = :actualizado"),
				ConditionExpression: aws.String("attribute_exists(PK) AND targetSK = :vieja AND " + filtroNoBorrado),
				ExpressionAttributeNames: map[string]string{
					"#deleted": "deleted",
				},
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":nueva":       stringAttr(skNueva),
					":vieja":       stringAttr(skVieja),
					":actualizado": stringAttr(ahoraISO),
					":false":       &types.AttributeValueMemberBOOL{Value: false},
				},
			}},
		}
	}

	if _, err := s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: items}); err != nil {
		// La ruta sin movimiento usa UpdateItem, que devuelve ConditionalCheckFailedException
		// (no TransactionCanceledException). Sin este mapeo, una versión obsoleta se
		// traduciría a 503 en lugar de 412.
		var condicional *types.ConditionalCheckFailedException
		if errors.As(err, &condicional) {
			return nil, appMovements.ErrConflict
		}
		var cancelada *types.TransactionCanceledException
		if !errors.As(err, &cancelada) {
			return nil, appMovements.ErrUnavailable
		}
		if isConditionalCancel(err) {
			return nil, appMovements.ErrConflict
		}
		return nil, appMovements.ErrUnavailable
	}

	resultado := siguiente
	return &resultado, nil
}

// DeleteMovement escribe el TOMBSTONE lógico de E031 (DECISION_E031_BORRADO §3.2) en UNA
// transacción de dos acciones:
//
//  1. `Update` del ítem principal: marca `deleted=true`, `deletedAt`, `deletedBy` y `updatedAt`,
//     incrementa `version` y RETIRA el texto libre (`description`) y la proyección en `GSI1`
//     (`GSI1PK`/`GSI1SK`), de modo que el movimiento borrado desaparece del índice sin `Scan`.
//     La condición exige la versión leída (`ExpectedVersion`) y que el ítem NO esté ya borrado:
//     un PATCH concurrente o un segundo borrado cancelan la transacción.
//  2. `Update` del localizador `MOVEMENTPTR#<id>`: se CONSERVA el puntero (autorización
//     `APRUEBO_E031_IDEMPOTENTE_204_CON_PUNTERO`) y sólo se marca `deleted=true` y `updatedAt`,
//     manteniendo `targetSK`. Un segundo DELETE debe poder resolver el mismo movimiento y
//     devolver 204 idempotente sin volver a escribir.
//
// Ninguna expresión declara valores que no use: DynamoDB rechaza con ValidationException
// cualquier `ExpressionAttributeValues` sobrante (defecto que ya rompió E003/E042).
func (s *MovementsStore) DeleteMovement(ctx context.Context, write appMovements.DeleteWrite) error {
	if s == nil || s.client == nil || s.table == "" {
		return appMovements.ErrUnavailable
	}
	titular := strings.TrimSpace(write.UserID)
	id := strings.TrimSpace(write.MovementID)
	if titular == "" || id == "" {
		return appMovements.ErrUnavailable
	}

	pk := domainDynamo.PrefixUser + titular
	_, sk, _, _ := domainDynamo.MovementKeys(titular, id, write.Previous.Type, write.Previous.OccurredAt)
	ptrPK, ptrSK := MovementPointerKeys(titular, id)
	ahora := write.Now.UTC()
	ahoraISO := ahora.Format(time.RFC3339)
	esperada := write.ExpectedVersion
	if esperada < 1 {
		// Sin versión leída no hay CAS posible: la escritura falla cerrada en lugar de inventar
		// una precondición que podría sobrescribir un cambio concurrente.
		return appMovements.ErrUnavailable
	}

	principal := func() types.TransactWriteItem {
		return types.TransactWriteItem{Update: &types.Update{
			TableName: aws.String(s.table),
			Key:       map[string]types.AttributeValue{"PK": stringAttr(pk), "SK": stringAttr(sk)},
			UpdateExpression: aws.String("SET #deleted = :true, #deletedAt = :now, #deletedBy = :actor, #updatedAt = :now " +
				"REMOVE #description, #gsi1pk, #gsi1sk ADD #version :one"),
			ConditionExpression: aws.String("attribute_exists(PK) AND #version = :expected AND " + filtroNoBorrado),
			ExpressionAttributeNames: map[string]string{
				"#deleted":     "deleted",
				"#deletedAt":   "deletedAt",
				"#deletedBy":   "deletedBy",
				"#updatedAt":   "updatedAt",
				"#version":     "version",
				"#description": "description",
				"#gsi1pk":      "GSI1PK",
				"#gsi1sk":      "GSI1SK",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":true":     &types.AttributeValueMemberBOOL{Value: true},
				":false":    &types.AttributeValueMemberBOOL{Value: false},
				":now":      stringAttr(ahoraISO),
				":actor":    stringAttr(movementDeletedByUser),
				":expected": numberAttr(esperada),
				":one":      numberAttr(1),
			},
		}}
	}
	puntero := types.TransactWriteItem{Update: &types.Update{
		TableName:           aws.String(s.table),
		Key:                 map[string]types.AttributeValue{"PK": stringAttr(ptrPK), "SK": stringAttr(ptrSK)},
		UpdateExpression:    aws.String("SET #deleted = :true, #updatedAt = :now"),
		ConditionExpression: aws.String("attribute_exists(PK)"),
		ExpressionAttributeNames: map[string]string{
			"#deleted":   "deleted",
			"#updatedAt": "updatedAt",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":true": &types.AttributeValueMemberBOOL{Value: true},
			":now":  stringAttr(ahoraISO),
		},
	}}

	_, err := s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{principal(), puntero},
	})
	if err == nil {
		return nil
	}
	var cancelada *types.TransactionCanceledException
	if !errors.As(err, &cancelada) {
		return appMovements.ErrUnavailable
	}
	if transactionItemFailed(err, 0) {
		// La versión cambió (PATCH concurrente) o el ítem ya era un tombstone. El servicio relee
		// y decide: 204 si ya estaba borrado, 404 si desapareció, 409 si hay conflicto real.
		return appMovements.ErrConflict
	}
	if !transactionItemFailed(err, 1) {
		return appMovements.ErrUnavailable
	}
	// Sólo falló el localizador (ausente o movido): la autoridad es el ítem principal. Se repite
	// la escritura del tombstone SIN el puntero; nunca se revierte un tombstone ya aplicado.
	if _, errRetry := s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{principal()},
	}); errRetry != nil {
		if isConditionalCancel(errRetry) {
			return appMovements.ErrConflict
		}
		return appMovements.ErrUnavailable
	}
	return nil
}
