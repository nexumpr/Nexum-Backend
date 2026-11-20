package dynamo

// Adaptador de persistencia del pago de una regla recurrente (E037):
// `POST /api/v1/recurring-payments/{recurringPaymentId}/payments`.
//
// Patrón físico (todo dentro de la partición del titular que la política IAM vigente ya
// autoriza con `LeadingKeys` `USER#*`; no crea tabla, índice ni permiso nuevo):
//
//	movimiento   PK = USER#<userId>   SK = MOVEMENT#<occurredAt ordenable>#<movementId>
//	puntero      PK = USER#<userId>   SK = MOVEMENTPTR#<movementId>
//	localizador  PK = USER#<userId>   SK = RECURRINGPAY#<ruleId>#<YYYY-MM-DD>   (fecha civil local)
//	idempotencia PK = USER#<userId>   SK = IDEM#E037_PAY_RECURRING#<sha256(operación:clave)>
//
// El movimiento y su puntero se materializan con las MISMAS funciones que E028/E029
// (`movementRecord`, `MovementPointerKeys`), de modo que E027/E029 devuelven exactamente el
// mismo recurso y el vínculo `recurringPaymentId` es visible sin traducciones adicionales.
//
// No hay `Scan`, ni `PutItem`, ni `UpdateItem`, ni `DeleteItem`: la interfaz del cliente declara
// únicamente `GetItem` y `TransactWriteItems`, y el compilador impide salir de ahí.

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
	appRecurring "github.com/nexumpr/Nexum-Backend/internal/application/recurring"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
	domainRecurring "github.com/nexumpr/Nexum-Backend/internal/domain/recurring"
)

// RecurringPaymentAPI es el subconjunto del cliente DynamoDB que usa E037.
//
// Se declara aquí —y no se reutiliza `RecurringAPI`— para que el compilador impida `Query`,
// `Scan` y cualquier escritura no transaccional en este dominio.
type RecurringPaymentAPI interface {
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	TransactWriteItems(ctx context.Context, params *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error)
}

const (
	// recurringPaymentLocatorPrefix es la SK del localizador de ocurrencia. Deliberadamente NO
	// empieza por `RECURRING#`, de modo que el listado de E032 no lo confunda con una regla.
	//
	// No se añade a `internal/domain/dynamo/keys.go` porque ese archivo es propiedad del
	// orquestador durante la oleada; la forma es la del diseño aprobado (RFC-022 §5.5).
	recurringPaymentLocatorPrefix = "RECURRINGPAY#"

	// recurringPaymentLocatorEntityType identifica el localizador de ocurrencia.
	recurringPaymentLocatorEntityType = "RECURRING_PAYMENT_OCCURRENCE"

	// recurringPaymentIdemEntityType identifica el registro de idempotencia. Es el mismo valor
	// que ya escriben E028 y E033: la entidad no cambia, sólo la operación del SK.
	recurringPaymentIdemEntityType = "IDEMPOTENCY_RECORD"
)

// RecurringPaymentLocatorKeys devuelve la PK/SK del localizador que bloquea un segundo pago de
// la misma regla en la misma fecha civil local. Se exporta para que las pruebas y un futuro
// lector de ocurrencias usen exactamente la misma forma.
func RecurringPaymentLocatorKeys(userID, ruleID, localDate string) (pk, sk string) {
	return domainDynamo.PrefixUser + strings.TrimSpace(userID),
		recurringPaymentLocatorPrefix + strings.TrimSpace(ruleID) + "#" + strings.TrimSpace(localDate)
}

// RecurringPaymentStore implementa `appRecurring.PaymentStore` sobre la tabla única existente.
type RecurringPaymentStore struct {
	client RecurringPaymentAPI
	table  string
	now    func() time.Time
}

// NewRecurringPaymentStore construye el adaptador. Sin cliente o sin tabla, todas las
// operaciones fallan cerradas con ErrUnavailable: nunca se sustituyen por memoria local.
func NewRecurringPaymentStore(client RecurringPaymentAPI, table string) *RecurringPaymentStore {
	return &RecurringPaymentStore{
		client: client,
		table:  strings.TrimSpace(table),
		now:    func() time.Time { return time.Now().UTC() },
	}
}

// WithClock permite inyectar un reloj determinista (proyección de ítems heredados en las
// pruebas); no altera ningún dato escrito.
func (s *RecurringPaymentStore) WithClock(now func() time.Time) *RecurringPaymentStore {
	if now != nil {
		s.now = now
	}
	return s
}

// fechaHoy devuelve la fecha civil usada para proyectar reglas heredadas del alta (D-E032-01).
func (s *RecurringPaymentStore) fechaHoy() time.Time {
	if s == nil || s.now == nil {
		return domainRecurring.DateOnly(time.Now().UTC())
	}
	return domainRecurring.DateOnly(s.now().UTC())
}

// LoadRule lee `USER#<id>/RECURRING#<id>` con consistencia fuerte y la proyecta al contrato
// vigente. Devuelve (nil, nil) cuando la clave no existe o el ítem no es representable: en
// ambos casos el servicio responde 404 sin revelar si el recurso pertenece a otra cuenta.
func (s *RecurringPaymentStore) LoadRule(ctx context.Context, userID, ruleID string) (*appRecurring.Payment, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, appRecurring.ErrUnavailable
	}
	titular := strings.TrimSpace(userID)
	id := strings.TrimSpace(ruleID)
	if titular == "" || id == "" {
		return nil, appRecurring.ErrUnavailable
	}

	pk, sk := domainDynamo.RecurringKeys(titular, id)
	item, err := s.leerItem(ctx, pk, sk)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, nil
	}
	regla, ok := recurringFromItem(item, s.fechaHoy())
	if !ok {
		return nil, nil
	}
	return regla, nil
}

// LoadTimezone lee la zona IANA de `USER#<id>/PREFERENCES` con consistencia fuerte.
//
// Devuelve cadena vacía —sin error— cuando el titular no tiene preferencias, cuando la clave la
// ocupa otra entidad o cuando el ítem no declara zona: el servicio aplica entonces la zona por
// defecto del Gate Oleada 3 (D-W3-08). Un fallo de lectura sí es un error: sin zona no se puede
// fijar el día civil con seguridad y la operación falla cerrada.
func (s *RecurringPaymentStore) LoadTimezone(ctx context.Context, userID string) (string, error) {
	if s == nil || s.client == nil || s.table == "" {
		return "", appRecurring.ErrUnavailable
	}
	titular := strings.TrimSpace(userID)
	if titular == "" {
		return "", appRecurring.ErrUnavailable
	}

	item, err := s.leerItem(ctx, domainDynamo.PrefixUser+titular, preferencesSortKey)
	if err != nil {
		return "", err
	}
	if item == nil {
		return "", nil
	}
	if entidad := stringAttribute(item["entityType"]); entidad != "" && entidad != preferencesEntityType {
		// La clave la ocupa otra entidad: no se interpreta como preferencias del titular.
		return "", nil
	}
	zona := strings.TrimSpace(stringAttribute(item["timezone"]))
	if zona == "" {
		// Compatibilidad con un ítem que sólo guardara el alias A8.
		zona = strings.TrimSpace(stringAttribute(item["timeZone"]))
	}
	return zona, nil
}

// Pay ejecuta el pago de E037 en UNA transacción condicional de cinco acciones:
//
//  1. movimiento `MOVEMENT#<occurredAt>#<id>`, con `attribute_not_exists(PK)`;
//  2. puntero `MOVEMENTPTR#<id>`, con `attribute_not_exists(PK)`;
//  3. localizador `RECURRINGPAY#<ruleId>#<fecha local>`, con `attribute_not_exists(PK)`: es el
//     control de duplicado por día civil que gana aunque la Idempotency-Key sea distinta;
//  4. registro de idempotencia, con `attribute_not_exists(PK) OR expiresAt < :now`;
//  5. `ConditionCheck` de la regla: debe existir en la MISMA partición y estar activa (un ítem
//     heredado del alta, sin atributo `active`, se considera activo, D-E032-01).
//
// El registro de idempotencia se lee antes de escribir: si ya existe con el mismo `payloadHash`
// se devuelve la respuesta ORIGINAL sin escribir nada (replay) y con otro contenido se devuelve
// conflicto. Tras una cancelación, la resolución es determinista y NO depende del orden ni del
// índice de `CancellationReasons`: idempotencia → localizador → estado de la regla → colisión del
// identificador. Nunca se confirma una parte sin las demás.
func (s *RecurringPaymentStore) Pay(ctx context.Context, write appRecurring.PayWrite) (*appRecurring.PayResult, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, appRecurring.ErrUnavailable
	}

	userID := strings.TrimSpace(write.UserID)
	ruleID := strings.TrimSpace(write.RuleID)
	localDate := strings.TrimSpace(write.LocalDate)
	movimiento := write.Movement
	if userID == "" || ruleID == "" || localDate == "" {
		return nil, appRecurring.ErrUnavailable
	}
	if strings.TrimSpace(movimiento.ID) == "" || write.PayloadHash == "" || len(write.ResponseBody) == 0 {
		return nil, appRecurring.ErrUnavailable
	}
	if write.Status < 100 || write.Status > 599 {
		return nil, appRecurring.ErrUnavailable
	}
	if strings.TrimSpace(movimiento.RecurringPaymentID) != ruleID {
		// Invariante del caso de uso: el movimiento derivado siempre queda vinculado a su regla.
		return nil, appRecurring.ErrUnavailable
	}

	operacion := strings.TrimSpace(write.Operation)
	if operacion == "" {
		operacion = appRecurring.OperationPayRecurring
	}
	ahora := write.Now.UTC()
	if ahora.IsZero() {
		ahora = time.Now().UTC()
	}
	expira := write.ExpiresAt.UTC()
	if expira.IsZero() {
		expira = ahora.Add(appRecurring.IdempotencyTTL)
	}

	idemPK, idemSK := domainDynamo.UserIdempotencyKeys(userID, operacion, write.IdempotencyKey)
	locPK, locSK := RecurringPaymentLocatorKeys(userID, ruleID, localDate)

	// 1. Replay rápido con lectura consistente: una réplica retrasada no puede decidir aquí.
	existente, errLectura := s.leerIdempotenciaPago(ctx, idemPK, idemSK, userID)
	if errLectura != nil {
		return nil, errLectura
	}
	if existente != nil && !existente.expirado(ahora) {
		return replayPay(existente, write.PayloadHash)
	}

	// 2. Transacción atómica: movimiento + puntero + localizador + idempotencia + regla activa.
	itemMovimiento, _, skMovimiento := movementRecord(userID, movimiento, ahora, ahora)
	ptrPK, ptrSK := MovementPointerKeys(userID, movimiento.ID)
	ahoraISO := ahora.Format(time.RFC3339)

	puntero := map[string]types.AttributeValue{
		"PK":         stringAttr(ptrPK),
		"SK":         stringAttr(ptrSK),
		"entityType": stringAttr(movementPointerEntityType),
		"userId":     stringAttr(userID),
		"movementId": stringAttr(movimiento.ID),
		"targetSK":   stringAttr(skMovimiento),
		"createdAt":  stringAttr(ahoraISO),
		"updatedAt":  stringAttr(ahoraISO),
	}

	localizador := map[string]types.AttributeValue{
		"PK":          stringAttr(locPK),
		"SK":          stringAttr(locSK),
		"entityType":  stringAttr(recurringPaymentLocatorEntityType),
		"userId":      stringAttr(userID),
		"recurringId": stringAttr(ruleID),
		"localDate":   stringAttr(localDate),
		"movementId":  stringAttr(movimiento.ID),
		"movementSK":  stringAttr(skMovimiento),
		"occurredAt":  stringAttr(appMovements.SortableTime(movimiento.OccurredAt)),
		"createdAt":   stringAttr(ahoraISO),
	}

	idempotencia := map[string]types.AttributeValue{
		"PK":           stringAttr(idemPK),
		"SK":           stringAttr(idemSK),
		"entityType":   stringAttr(recurringPaymentIdemEntityType),
		"userId":       stringAttr(userID),
		"operation":    stringAttr(operacion),
		"payloadHash":  stringAttr(write.PayloadHash),
		"httpStatus":   numberAttr(int64(write.Status)),
		"responseBody": stringAttr(string(write.ResponseBody)),
		"location":     stringAttr("/api/v1/movements/" + movimiento.ID),
		"resourceId":   stringAttr(movimiento.ID),
		"recurringId":  stringAttr(ruleID),
		"localDate":    stringAttr(localDate),
		"createdAt":    stringAttr(ahoraISO),
		"expiresAt":    numberAttr(expira.Unix()),
	}

	reglaPK, reglaSK := domainDynamo.RecurringKeys(userID, ruleID)

	items := []types.TransactWriteItem{
		{Put: &types.Put{
			TableName:           aws.String(s.table),
			Item:                itemMovimiento,
			ConditionExpression: aws.String("attribute_not_exists(PK)"),
		}},
		{Put: &types.Put{
			TableName:           aws.String(s.table),
			Item:                puntero,
			ConditionExpression: aws.String("attribute_not_exists(PK)"),
		}},
		{Put: &types.Put{
			TableName:           aws.String(s.table),
			Item:                localizador,
			ConditionExpression: aws.String("attribute_not_exists(PK)"),
		}},
		{Put: &types.Put{
			TableName:                 aws.String(s.table),
			Item:                      idempotencia,
			ConditionExpression:       aws.String("attribute_not_exists(PK) OR expiresAt < :now"),
			ExpressionAttributeValues: map[string]types.AttributeValue{":now": numberAttr(ahora.Unix())},
		}},
		{ConditionCheck: &types.ConditionCheck{
			TableName: aws.String(s.table),
			Key: map[string]types.AttributeValue{
				"PK": stringAttr(reglaPK),
				"SK": stringAttr(reglaSK),
			},
			// La regla debe existir en la partición del titular y estar activa. Un ítem heredado
			// del alta no tiene `active`: se considera activo, igual que en E032/E034.
			ConditionExpression:      aws.String("attribute_exists(PK) AND (attribute_not_exists(#activo) OR #activo = :activo)"),
			ExpressionAttributeNames: map[string]string{"#activo": "active"},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":activo": boolAttr(true),
			},
		}},
	}

	_, err := s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: items})
	if err == nil {
		return &appRecurring.PayResult{
			Status: write.Status,
			Body:   append([]byte(nil), write.ResponseBody...),
		}, nil
	}

	var cancelada *types.TransactionCanceledException
	if !errors.As(err, &cancelada) {
		// Throttling, conflicto de transacción o fallo de servicio: nada se escribió.
		return nil, appRecurring.ErrUnavailable
	}
	return s.resolverCancelacion(ctx, userID, ruleID, localDate, idemPK, idemSK, ahora, write)
}

// resolverCancelacion decide el resultado de una transacción cancelada sin depender del índice de
// `CancellationReasons`. Orden: idempotencia (autoridad de la carrera) → localizador de la fecha
// local (duplicado) → estado de la regla (404/409) → colisión del identificador (reintento).
func (s *RecurringPaymentStore) resolverCancelacion(
	ctx context.Context,
	userID, ruleID, localDate, idemPK, idemSK string,
	ahora time.Time,
	write appRecurring.PayWrite,
) (*appRecurring.PayResult, error) {
	existente, err := s.leerIdempotenciaPago(ctx, idemPK, idemSK, userID)
	if err != nil {
		return nil, err
	}
	if existente != nil {
		if existente.expirado(ahora) {
			// Registro vencido que la condición dejó reutilizar pero ya describe otra operación.
			return nil, appRecurring.ErrConflict
		}
		return replayPay(existente, write.PayloadHash)
	}

	locPK, locSK := RecurringPaymentLocatorKeys(userID, ruleID, localDate)
	ocupado, err := s.existe(ctx, locPK, locSK)
	if err != nil {
		return nil, err
	}
	if ocupado {
		return nil, appRecurring.ErrDuplicatePayment
	}

	regla, err := s.LoadRule(ctx, userID, ruleID)
	if err != nil {
		return nil, err
	}
	if regla == nil {
		return nil, appRecurring.ErrNotFound
	}
	if !regla.Active {
		return nil, appRecurring.ErrRuleInactive
	}

	ptrPK, ptrSK := MovementPointerKeys(userID, write.Movement.ID)
	ocupado, err = s.existe(ctx, ptrPK, ptrSK)
	if err != nil {
		return nil, err
	}
	if ocupado {
		// Colisión del identificador generado: el servicio reintenta con otro.
		return nil, appRecurring.ErrMovementIDCollision
	}
	return nil, appRecurring.ErrUnavailable
}

// leerIdempotenciaPago lee el registro de idempotencia con consistencia fuerte. Devuelve
// (nil, nil) cuando la clave no existe o la ocupa otra entidad: la ausencia de registro no es un
// error, es la señal de que se debe crear.
func (s *RecurringPaymentStore) leerIdempotenciaPago(ctx context.Context, pk, sk, userID string) (*registroIdempotencia, error) {
	item, err := s.leerItem(ctx, pk, sk)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, nil
	}
	if entidad := stringAttribute(item["entityType"]); entidad != "" && entidad != recurringPaymentIdemEntityType {
		return nil, nil
	}
	if titular := stringAttribute(item["userId"]); titular != "" && titular != userID {
		// Defensa en profundidad: la PK ya aísla por usuario.
		return nil, nil
	}
	return idempotenciaDesdeItem(item), nil
}

// replayPay reproduce la respuesta original. Un `payloadHash` distinto es un conflicto: la misma
// clave no puede describir dos operaciones.
func replayPay(registro *registroIdempotencia, payloadHash string) (*appRecurring.PayResult, error) {
	if registro == nil {
		return nil, appRecurring.ErrUnavailable
	}
	if registro.payloadHash != payloadHash {
		return nil, appRecurring.ErrConflict
	}
	if registro.status < 100 || registro.status > 599 || registro.cuerpo == "" {
		return nil, appRecurring.ErrUnavailable
	}
	return &appRecurring.PayResult{
		Status:   registro.status,
		Body:     []byte(registro.cuerpo),
		Replayed: true,
	}, nil
}

// leerItem lee una clave exacta con consistencia fuerte. Devuelve (nil, nil) si no existe.
func (s *RecurringPaymentStore) leerItem(ctx context.Context, pk, sk string) (map[string]types.AttributeValue, error) {
	salida, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(pk),
			"SK": stringAttr(sk),
		},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return nil, appRecurring.ErrUnavailable
	}
	if salida == nil || len(salida.Item) == 0 {
		return nil, nil
	}
	return salida.Item, nil
}

// existe informa si la clave exacta está ocupada.
func (s *RecurringPaymentStore) existe(ctx context.Context, pk, sk string) (bool, error) {
	item, err := s.leerItem(ctx, pk, sk)
	if err != nil {
		return false, err
	}
	return item != nil, nil
}
