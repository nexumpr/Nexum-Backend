package dynamo

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	appAccount "github.com/nexumpr/Nexum-Backend/internal/application/account"
	domainDeletion "github.com/nexumpr/Nexum-Backend/internal/domain/deletion"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
)

// DeletionAPI es el subconjunto del cliente DynamoDB que usa E014. Se declara como interfaz
// para probar la construcción exacta de transacciones y condiciones con dobles; no incluye
// `Scan` ni `BatchWriteItem`, que están prohibidos en este flujo.
type DeletionAPI interface {
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	TransactWriteItems(ctx context.Context, params *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error)
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
	DeleteItem(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
}

// DeletionStore implementa:
//   - appAccount.DeletionStore (solicitud de eliminación, transacción de 3 ítems);
//   - appAccount.PurgeStore (descubrimiento, fencing y purga por fases).
//
// Vive en la tabla única existente y sólo usa prefijos ya autorizados por IAM
// (`USER#*`, `EMAIL#*`, `REFRESH#*`, `QUEUE#*`). No crea tablas, índices ni particiones y
// nunca ejecuta `Scan` ni `BatchWriteItem`.
type DeletionStore struct {
	client DeletionAPI
	table  string
}

// NewDeletionStore construye el adaptador. Sin cliente o tabla, todas las operaciones fallan
// cerradas: nunca se sustituyen por memoria local.
func NewDeletionStore(client DeletionAPI, table string) *DeletionStore {
	return &DeletionStore{client: client, table: strings.TrimSpace(table)}
}

func (s *DeletionStore) disponible() bool {
	return s != nil && s.client != nil && s.table != ""
}

// ---------------------------------------------------------------------------------------
// Solicitud de eliminación (E014 HTTP)
// ---------------------------------------------------------------------------------------

// LoadDeletionProfile lee `USER#<id>/PROFILE` con consistencia fuerte.
func (s *DeletionStore) LoadDeletionProfile(ctx context.Context, userID string) (*appAccount.DeletionProfile, error) {
	if !s.disponible() {
		return nil, appAccount.ErrDeletionUnavailable
	}
	if strings.TrimSpace(userID) == "" {
		return nil, nil
	}
	pk, sk := domainDynamo.UserKeys(userID)
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(pk),
			"SK": stringAttr(sk),
		},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return nil, appAccount.ErrDeletionUnavailable
	}
	if out == nil || len(out.Item) == 0 {
		return nil, nil
	}
	if entity := stringAttribute(out.Item["entityType"]); entity != "" && entity != "USER_PROFILE" {
		return nil, nil
	}
	perfil := &appAccount.DeletionProfile{
		UserID:       stringAttribute(out.Item["userId"]),
		Status:       stringAttribute(out.Item["status"]),
		PasswordHash: stringAttribute(out.Item["passwordHash"]),
		Email:        stringAttribute(out.Item["email"]),
	}
	if perfil.UserID == "" {
		perfil.UserID = userID
	}
	if version, ok := attributeInt(out.Item["version"]); ok {
		perfil.Version = version
		perfil.HasVersion = true
	}
	if epoch, ok := attributeInt(out.Item["sessionEpoch"]); ok {
		perfil.SessionEpoch = epoch
	}
	return perfil, nil
}

// ScheduleDeletion ejecuta la transacción de 3 ítems del diseño §2. Cualquier fallo de
// condición cancela la transacción completa: no queda un perfil a medias ni un trabajo sin
// cola.
func (s *DeletionStore) ScheduleDeletion(ctx context.Context, app appAccount.DeletionApplication) error {
	if !s.disponible() {
		return appAccount.ErrDeletionUnavailable
	}
	userID := strings.TrimSpace(app.UserID)
	requestID := strings.TrimSpace(app.RequestID)
	if userID == "" || requestID == "" {
		return appAccount.ErrDeletionUnavailable
	}

	now := app.Now.UTC()
	nowISO := now.Format(time.RFC3339)
	nowEpoch := now.Unix()

	userPK, userSK := domainDynamo.UserKeys(userID)
	controlPK, controlSK := domainDeletion.UserControlKeys(userID, requestID)
	queuePK, queueSK := domainDeletion.QueueKeys(requestID)

	// 1. Perfil: ACTIVE -> DELETION_PENDING con época y versión incrementadas. Las
	// condiciones exigen el estado y los valores leídos: si otro proceso cambió el perfil
	// entre la lectura y la escritura, se cancela todo.
	condicionPerfil := "attribute_exists(PK) AND #status = :active AND #version = :expected AND (attribute_not_exists(#epoch) OR #epoch = :expectedEpoch)"
	actualizacionPerfil := "SET #status = :pending, deletedAt = :nowISO, sessionEpoch = :nextEpoch ADD #version :one"
	valoresPerfil := map[string]types.AttributeValue{
		":active":        stringAttr(appAccount.ProfileStatusActive),
		":pending":       stringAttr(domainDeletion.StatusDeletionPending),
		":nowISO":        stringAttr(nowISO),
		":expected":      numberAttr(app.ExpectedVersion),
		":expectedEpoch": numberAttr(app.ExpectedSessionEpoch),
		":nextEpoch":     numberAttr(app.ExpectedSessionEpoch + 1),
		":one":           numberAttr(1),
	}
	if !app.HasVersion {
		// Perfil legado sin `version`: no existe rotador previo que pueda perder un CAS, así
		// que se exige su ausencia y se inicializa a 1. `:expected` no se declara: DynamoDB
		// rechaza valores de expresión no utilizados.
		condicionPerfil = "attribute_exists(PK) AND #status = :active AND attribute_not_exists(#version) AND (attribute_not_exists(#epoch) OR #epoch = :expectedEpoch)"
		actualizacionPerfil = "SET #status = :pending, deletedAt = :nowISO, sessionEpoch = :nextEpoch, #version = :one"
		delete(valoresPerfil, ":expected")
	}

	perfil := types.TransactWriteItem{Update: &types.Update{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(userPK),
			"SK": stringAttr(userSK),
		},
		UpdateExpression:    aws.String(actualizacionPerfil),
		ConditionExpression: aws.String(condicionPerfil),
		ExpressionAttributeNames: map[string]string{
			"#status":  "status",
			"#version": "version",
			"#epoch":   "sessionEpoch",
		},
		ExpressionAttributeValues: valoresPerfil,
	}}

	// 2. Control de progreso: fuente de verdad, con `state=PENDING` y `phase=COLLECT`.
	control := map[string]types.AttributeValue{
		"PK":             stringAttr(controlPK),
		"SK":             stringAttr(controlSK),
		"entityType":     stringAttr(domainDeletion.EntityControl),
		"requestId":      stringAttr(requestID),
		"userId":         stringAttr(userID),
		"state":          stringAttr(appAccount.PurgeStatePending),
		"phase":          stringAttr(appAccount.PurgePhaseCollect),
		"owner":          stringAttr(""),
		"fence":          numberAttr(0),
		"attempts":       numberAttr(0),
		"dueAt":          numberAttr(nowEpoch),
		"dueAtISO":       stringAttr(nowISO),
		"leaseUntil":     numberAttr(0),
		"deletedCount":   numberAttr(0),
		"unknownKeys":    numberAttr(0),
		"catalogVersion": stringAttr(domainDeletion.CatalogVersion),
		"version":        numberAttr(1),
		"createdAt":      stringAttr(nowISO),
	}
	if hash := strings.TrimSpace(app.EmailHash); hash != "" {
		control["emailHash"] = stringAttr(hash)
	}
	controlItem := types.TransactWriteItem{Put: &types.Put{
		TableName:           aws.String(s.table),
		Item:                control,
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	}}

	// 3. Entrada de cola: índice de vencimiento con SK estable (requestId).
	cola := map[string]types.AttributeValue{
		"PK":         stringAttr(queuePK),
		"SK":         stringAttr(queueSK),
		"entityType": stringAttr(domainDeletion.EntityQueue),
		"requestId":  stringAttr(requestID),
		"userId":     stringAttr(userID),
		"state":      stringAttr(appAccount.PurgeStatePending),
		"owner":      stringAttr(""),
		"fence":      numberAttr(0),
		"dueAt":      numberAttr(nowEpoch),
		"dueAtISO":   stringAttr(nowISO),
		"leaseUntil": numberAttr(0),
		"createdAt":  stringAttr(nowISO),
	}
	colaItem := types.TransactWriteItem{Put: &types.Put{
		TableName:           aws.String(s.table),
		Item:                cola,
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	}}

	_, err := s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{perfil, controlItem, colaItem},
	})
	if err == nil {
		return nil
	}
	if isConditionalCancel(err) {
		// El perfil dejó de estar ACTIVE, cambió de versión o cambió la época; o el
		// `requestId` colisionó. Nada se escribió: es un conflicto, no un fallo de servicio.
		return appAccount.ErrDeletionConflict
	}
	return appAccount.ErrDeletionUnavailable
}

// ---------------------------------------------------------------------------------------
// Worker de purga
// ---------------------------------------------------------------------------------------

// Discover consulta `QUEUE#DELETION` con consistencia fuerte y la FilterExpression del
// diseño §3. `Limit` se aplica ANTES del filtro en DynamoDB, así que una página puede
// devolver cero coincidencias y aun así tener continuación: se devuelve la `SK` de
// `LastEvaluatedKey` para que el worker siga paginando sin detenerse.
func (s *DeletionStore) Discover(ctx context.Context, startAfter string, now time.Time, pageSize int) ([]appAccount.PurgeCandidate, string, error) {
	if !s.disponible() {
		return nil, "", appAccount.ErrDeletionUnavailable
	}
	if pageSize <= 0 {
		pageSize = 50
	}
	pk := domainDeletion.PartitionQueueDeletion

	entrada := &dynamodb.QueryInput{
		TableName:              aws.String(s.table),
		KeyConditionExpression: aws.String("PK = :q"),
		FilterExpression: aws.String(
			"((#state = :pending AND #due <= :now)" +
				" OR (#state = :retry AND #due <= :now)" +
				" OR (#state = :slow AND #due <= :now)" +
				" OR (#state = :running AND #lease < :now))"),
		ExpressionAttributeNames: map[string]string{
			"#state": "state",
			"#due":   "dueAt",
			"#lease": "leaseUntil",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":q":       stringAttr(pk),
			":pending": stringAttr(appAccount.PurgeStatePending),
			":retry":   stringAttr(appAccount.PurgeStateRetry),
			":slow":    stringAttr(appAccount.PurgeStateSlowRetry),
			":running": stringAttr(appAccount.PurgeStateRunning),
			":now":     numberAttr(now.UTC().Unix()),
		},
		ConsistentRead: aws.Bool(true),
		Limit:          aws.Int32(int32(pageSize)),
	}
	if strings.TrimSpace(startAfter) != "" {
		entrada.ExclusiveStartKey = map[string]types.AttributeValue{
			"PK": stringAttr(pk),
			"SK": stringAttr(startAfter),
		}
	}

	out, err := s.client.Query(ctx, entrada)
	if err != nil {
		return nil, "", appAccount.ErrDeletionUnavailable
	}
	if out == nil {
		return nil, "", nil
	}
	candidatos := make([]appAccount.PurgeCandidate, 0, len(out.Items))
	for _, item := range out.Items {
		candidato := appAccount.PurgeCandidate{
			UserID:    stringAttribute(item["userId"]),
			RequestID: stringAttribute(item["requestId"]),
			State:     stringAttribute(item["state"]),
		}
		if strings.TrimSpace(candidato.RequestID) == "" {
			candidato.RequestID = stringAttribute(item["SK"])
		}
		if candidato.UserID == "" || candidato.RequestID == "" {
			continue
		}
		if segundos, ok := attributeInt(item["dueAt"]); ok && segundos > 0 {
			candidato.DueAt = time.Unix(segundos, 0).UTC()
		}
		candidatos = append(candidatos, candidato)
	}
	siguiente := ""
	if len(out.LastEvaluatedKey) > 0 {
		siguiente = stringAttribute(out.LastEvaluatedKey["SK"])
	}
	return candidatos, siguiente, nil
}

// Claim toma el trabajo de forma atómica: UNA transacción incrementa `fence` en el control
// y en la entrada de cola (quedan siempre iguales) y sólo se aplica si el estado y el
// vencimiento lo permiten. Devuelve el control recién reclamado.
func (s *DeletionStore) Claim(ctx context.Context, candidato appAccount.PurgeCandidate, owner string, now time.Time, lease time.Duration) (*appAccount.PurgeControl, appAccount.ClaimOutcome, error) {
	if !s.disponible() {
		return nil, appAccount.ClaimNotEligible, appAccount.ErrDeletionUnavailable
	}
	userID := strings.TrimSpace(candidato.UserID)
	requestID := strings.TrimSpace(candidato.RequestID)
	if userID == "" || requestID == "" {
		return nil, appAccount.ClaimNotEligible, appAccount.ErrDeletionUnavailable
	}

	controlPK, controlSK := domainDeletion.UserControlKeys(userID, requestID)
	queuePK, queueSK := domainDeletion.QueueKeys(requestID)
	nowUTC := now.UTC()
	until := nowUTC.Add(lease).Unix()

	condicion := "((#state = :pending AND #due <= :now)" +
		" OR (#state = :running AND #lease < :now)" +
		" OR (#state = :retry AND #due <= :now)" +
		" OR (#state = :slow AND #due <= :now))"

	valoresControl := map[string]types.AttributeValue{
		":pending": stringAttr(appAccount.PurgeStatePending),
		":running": stringAttr(appAccount.PurgeStateRunning),
		":retry":   stringAttr(appAccount.PurgeStateRetry),
		":slow":    stringAttr(appAccount.PurgeStateSlowRetry),
		":now":     numberAttr(nowUTC.Unix()),
		":owner":   stringAttr(owner),
		":until":   numberAttr(until),
		":collect": stringAttr(appAccount.PurgePhaseCollect),
		":one":     numberAttr(1),
	}

	transaccion := func(variables map[string]types.AttributeValue) []types.TransactWriteItem {
		return []types.TransactWriteItem{
			{Update: &types.Update{
				TableName: aws.String(s.table),
				Key: map[string]types.AttributeValue{
					"PK": stringAttr(controlPK),
					"SK": stringAttr(controlSK),
				},
				UpdateExpression: aws.String(
					"SET #state = :running, #owner = :owner, #lease = :until, #phase = if_not_exists(#phase, :collect) ADD #fence :one, #attempts :one"),
				ConditionExpression: aws.String(condicion),
				ExpressionAttributeNames: map[string]string{
					"#state":    "state",
					"#due":      "dueAt",
					"#owner":    "owner",
					"#lease":    "leaseUntil",
					"#phase":    "phase",
					"#fence":    "fence",
					"#attempts": "attempts",
				},
				ExpressionAttributeValues: variables,
			}},
			{Update: &types.Update{
				TableName: aws.String(s.table),
				Key: map[string]types.AttributeValue{
					"PK": stringAttr(queuePK),
					"SK": stringAttr(queueSK),
				},
				// La cola debe existir y su `fence` avanza junto al del control en la MISMA
				// transacción: a partir de aquí toda escritura del worker exige owner+fence.
				UpdateExpression:    aws.String("SET #state = :running, #owner = :owner, #lease = :until ADD #fence :one"),
				ConditionExpression: aws.String("attribute_exists(PK)"),
				ExpressionAttributeNames: map[string]string{
					"#state": "state",
					"#owner": "owner",
					"#lease": "leaseUntil",
					"#fence": "fence",
				},
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":running": stringAttr(appAccount.PurgeStateRunning),
					":owner":   stringAttr(owner),
					":until":   numberAttr(until),
					":one":     numberAttr(1),
				},
			}},
		}
	}

	_, err := s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: transaccion(valoresControl)})
	if err != nil {
		if !isConditionalCancel(err) {
			return nil, appAccount.ClaimNotEligible, appAccount.ErrDeletionUnavailable
		}
		control, errLectura := s.loadControl(ctx, controlPK, controlSK)
		if errLectura != nil {
			return nil, appAccount.ClaimNotEligible, appAccount.ErrDeletionUnavailable
		}
		if control == nil {
			return nil, appAccount.ClaimNotEligible, nil
		}
		if control.State == appAccount.PurgeStatePurged {
			return nil, appAccount.ClaimAlreadyPurged, nil
		}
		// La cola es sólo un índice de vencimiento y puede repararse desde el control. Si la
		// entrada faltaba (p. ej. borrada por error), la transacción se canceló porque exige
		// `attribute_exists(PK)` en la cola; se recrea y se reintenta UNA vez.
		reparada, errReparar := s.repararCola(ctx, control, nowUTC)
		if errReparar != nil {
			return nil, appAccount.ClaimNotEligible, appAccount.ErrDeletionUnavailable
		}
		if !reparada {
			return nil, appAccount.ClaimNotEligible, nil
		}
		if _, errReintento := s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: transaccion(valoresControl)}); errReintento != nil {
			if isConditionalCancel(errReintento) {
				return nil, appAccount.ClaimNotEligible, nil
			}
			return nil, appAccount.ClaimNotEligible, appAccount.ErrDeletionUnavailable
		}
	}

	control, err := s.loadControl(ctx, controlPK, controlSK)
	if err != nil {
		return nil, appAccount.ClaimNotEligible, appAccount.ErrDeletionUnavailable
	}
	if control == nil {
		return nil, appAccount.ClaimNotEligible, appAccount.ErrDeletionUnavailable
	}
	return control, appAccount.ClaimClaimed, nil
}

// repararCola recrea la entrada de cola desde el control cuando falta. Es idempotente y no
// toca una entrada existente: `attribute_not_exists(PK)` falla y se informa (false, nil).
func (s *DeletionStore) repararCola(ctx context.Context, control *appAccount.PurgeControl, now time.Time) (bool, error) {
	if control == nil {
		return false, nil
	}
	queuePK, queueSK := domainDeletion.QueueKeys(control.RequestID)
	if strings.TrimSpace(queueSK) == "" {
		return false, nil
	}
	// La entrada reparada refleja el estado REAL del control: si está RUNNING con lease
	// vigente, la fila no debe aparecer como elegible; si está RETRY con `dueAt` futuro,
	// tampoco. Así la reparación no altera el orden de vencimientos.
	due := control.DueAt
	if due.IsZero() {
		due = now.UTC()
	}
	lease := control.LeaseUntil
	leaseEpoch := int64(0)
	if !lease.IsZero() {
		leaseEpoch = lease.UTC().Unix()
	}
	item := map[string]types.AttributeValue{
		"PK":         stringAttr(queuePK),
		"SK":         stringAttr(queueSK),
		"entityType": stringAttr(domainDeletion.EntityQueue),
		"requestId":  stringAttr(control.RequestID),
		"userId":     stringAttr(control.UserID),
		"state":      stringAttr(control.State),
		"owner":      stringAttr(control.Owner),
		"fence":      numberAttr(control.Fence),
		"dueAt":      numberAttr(due.UTC().Unix()),
		"dueAtISO":   stringAttr(due.UTC().Format(time.RFC3339)),
		"leaseUntil": numberAttr(leaseEpoch),
		"createdAt":  stringAttr(now.UTC().Format(time.RFC3339)),
	}
	_, err := s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(s.table),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	})
	if err == nil {
		return true, nil
	}
	if isConditionalCancel(err) || isDeletionConditionalFailure(err) {
		return false, nil
	}
	return false, appAccount.ErrDeletionUnavailable
}

// isDeletionConditionalFailure distingue el fallo de condición de una operación simple
// (`PutItem`/`DeleteItem`, que lanza ConditionalCheckFailedException) del fallo de una
// transacción (TransactionCanceledException). Sin esta distinción, una condición esperada
// se confundiría con una caída de DynamoDB.
func isDeletionConditionalFailure(err error) bool {
	var condicional *types.ConditionalCheckFailedException
	return errors.As(err, &condicional)
}

// Heartbeat renueva el lease con condición owner+fence. Un dueño anterior no puede latir.
func (s *DeletionStore) Heartbeat(ctx context.Context, control *appAccount.PurgeControl, owner string, fence int64, now time.Time, lease time.Duration) (bool, error) {
	if !s.disponible() || control == nil {
		return false, appAccount.ErrDeletionUnavailable
	}
	until := now.UTC().Add(lease).Unix()
	controlPK, controlSK := domainDeletion.UserControlKeys(control.UserID, control.RequestID)
	queuePK, queueSK := domainDeletion.QueueKeys(control.RequestID)

	_, err := s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{Update: &types.Update{
				TableName: aws.String(s.table),
				Key: map[string]types.AttributeValue{
					"PK": stringAttr(controlPK),
					"SK": stringAttr(controlSK),
				},
				UpdateExpression:    aws.String("SET #lease = :until"),
				ConditionExpression: aws.String("#owner = :owner AND #fence = :fence"),
				ExpressionAttributeNames: map[string]string{
					"#lease": "leaseUntil",
					"#owner": "owner",
					"#fence": "fence",
				},
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":until": numberAttr(until),
					":owner": stringAttr(owner),
					":fence": numberAttr(fence),
				},
			}},
			{Update: &types.Update{
				TableName: aws.String(s.table),
				Key: map[string]types.AttributeValue{
					"PK": stringAttr(queuePK),
					"SK": stringAttr(queueSK),
				},
				UpdateExpression:    aws.String("SET #lease = :until"),
				ConditionExpression: aws.String("#owner = :owner AND #fence = :fence"),
				ExpressionAttributeNames: map[string]string{
					"#lease": "leaseUntil",
					"#owner": "owner",
					"#fence": "fence",
				},
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":until": numberAttr(until),
					":owner": stringAttr(owner),
					":fence": numberAttr(fence),
				},
			}},
		},
	})
	if err == nil {
		return true, nil
	}
	if isConditionalCancel(err) {
		return false, nil
	}
	return false, appAccount.ErrDeletionUnavailable
}

// HoldsLease relee el control con consistencia fuerte y exige owner, fence y lease vigentes.
func (s *DeletionStore) HoldsLease(ctx context.Context, control *appAccount.PurgeControl, owner string, fence int64, now time.Time) (bool, error) {
	if !s.disponible() || control == nil {
		return false, appAccount.ErrDeletionUnavailable
	}
	controlPK, controlSK := domainDeletion.UserControlKeys(control.UserID, control.RequestID)
	actual, err := s.loadControl(ctx, controlPK, controlSK)
	if err != nil {
		return false, err
	}
	if actual == nil {
		return false, nil
	}
	if actual.Owner != owner || actual.Fence != fence {
		return false, nil
	}
	if !actual.LeaseUntil.After(now.UTC()) {
		return false, nil
	}
	return actual.State == appAccount.PurgeStateRunning, nil
}

// SaveProgress persiste el avance y renueva el lease en control y cola, con condición
// owner+fence en ambos. Devuelve false cuando el fence dejó de ser propio.
func (s *DeletionStore) SaveProgress(ctx context.Context, control *appAccount.PurgeControl, owner string, fence int64, phase, lastSK string, refreshDigests []string, deletedCount, unknownKeys int64, now time.Time, lease time.Duration) (bool, error) {
	if !s.disponible() || control == nil {
		return false, appAccount.ErrDeletionUnavailable
	}
	controlPK, controlSK := domainDeletion.UserControlKeys(control.UserID, control.RequestID)
	queuePK, queueSK := domainDeletion.QueueKeys(control.RequestID)
	until := now.UTC().Add(lease).Unix()

	actualizacion := "SET #phase = :phase, lastSK = :lastSK, #deleted = :deleted, unknownKeys = :unknown, #lease = :until"
	nombres := map[string]string{
		"#phase":   "phase",
		"#deleted": "deletedCount",
		"#lease":   "leaseUntil",
		"#owner":   "owner",
		"#fence":   "fence",
	}
	valores := map[string]types.AttributeValue{
		":phase":   stringAttr(phase),
		":lastSK":  stringAttr(lastSK),
		":deleted": numberAttr(deletedCount),
		":unknown": numberAttr(unknownKeys),
		":until":   numberAttr(until),
		":owner":   stringAttr(owner),
		":fence":   numberAttr(fence),
	}
	unidos := strings.Join(refreshDigests, ",")
	if unidos == "" {
		// Un conjunto/String vacío no aporta nada y `REMOVE` evita dejar un valor obsoleto.
		actualizacion += " REMOVE refreshDigests"
	} else {
		actualizacion += ", refreshDigests = :digests"
		valores[":digests"] = stringAttr(unidos)
	}

	_, err := s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{Update: &types.Update{
				TableName: aws.String(s.table),
				Key: map[string]types.AttributeValue{
					"PK": stringAttr(controlPK),
					"SK": stringAttr(controlSK),
				},
				UpdateExpression:          aws.String(actualizacion),
				ConditionExpression:       aws.String("#owner = :owner AND #fence = :fence"),
				ExpressionAttributeNames:  nombres,
				ExpressionAttributeValues: valores,
			}},
			{Update: &types.Update{
				TableName: aws.String(s.table),
				Key: map[string]types.AttributeValue{
					"PK": stringAttr(queuePK),
					"SK": stringAttr(queueSK),
				},
				UpdateExpression:    aws.String("SET #lease = :until"),
				ConditionExpression: aws.String("#owner = :owner AND #fence = :fence"),
				ExpressionAttributeNames: map[string]string{
					"#lease": "leaseUntil",
					"#owner": "owner",
					"#fence": "fence",
				},
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":until": numberAttr(until),
					":owner": stringAttr(owner),
					":fence": numberAttr(fence),
				},
			}},
		},
	})
	if err == nil {
		return true, nil
	}
	if isConditionalCancel(err) {
		return false, nil
	}
	return false, appAccount.ErrDeletionUnavailable
}

// ScheduleRetry reprograma el trabajo en una transacción: control y cola comparten estado,
// `dueAt` y la cesión de propiedad. La SK de la cola es el `requestId`, de modo que se
// actualiza el atributo de la MISMA entrada, sin duplicados.
func (s *DeletionStore) ScheduleRetry(ctx context.Context, control *appAccount.PurgeControl, owner string, fence int64, state string, dueAt time.Time, deletedCount int64, now time.Time) (bool, error) {
	if !s.disponible() || control == nil {
		return false, appAccount.ErrDeletionUnavailable
	}
	controlPK, controlSK := domainDeletion.UserControlKeys(control.UserID, control.RequestID)
	queuePK, queueSK := domainDeletion.QueueKeys(control.RequestID)
	due := dueAt.UTC()
	dueISO := due.Format(time.RFC3339)

	condicion := "#owner = :owner AND #fence = :fence"

	_, err := s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{Update: &types.Update{
				TableName: aws.String(s.table),
				Key: map[string]types.AttributeValue{
					"PK": stringAttr(controlPK),
					"SK": stringAttr(controlSK),
				},
				UpdateExpression:    aws.String("SET #state = :state, dueAt = :due, dueAtISO = :dueISO, #deleted = :deleted, #owner = :empty, leaseUntil = :zero"),
				ConditionExpression: aws.String(condicion),
				ExpressionAttributeNames: map[string]string{
					"#state":   "state",
					"#owner":   "owner",
					"#fence":   "fence",
					"#deleted": "deletedCount",
				},
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":state":   stringAttr(state),
					":due":     numberAttr(due.Unix()),
					":dueISO":  stringAttr(dueISO),
					":deleted": numberAttr(deletedCount),
					":empty":   stringAttr(""),
					":zero":    numberAttr(0),
					":owner":   stringAttr(owner),
					":fence":   numberAttr(fence),
				},
			}},
			{Update: &types.Update{
				TableName: aws.String(s.table),
				Key: map[string]types.AttributeValue{
					"PK": stringAttr(queuePK),
					"SK": stringAttr(queueSK),
				},
				UpdateExpression:    aws.String("SET #state = :state, dueAt = :due, dueAtISO = :dueISO, #owner = :empty, leaseUntil = :zero"),
				ConditionExpression: aws.String(condicion),
				ExpressionAttributeNames: map[string]string{
					"#state": "state",
					"#owner": "owner",
					"#fence": "fence",
				},
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":state":  stringAttr(state),
					":due":    numberAttr(due.Unix()),
					":dueISO": stringAttr(dueISO),
					":empty":  stringAttr(""),
					":zero":   numberAttr(0),
					":owner":  stringAttr(owner),
					":fence":  numberAttr(fence),
				},
			}},
		},
	})
	if err == nil {
		control.State = state
		control.DueAt = due
		control.Owner = ""
		control.DeletedCount = deletedCount
		return true, nil
	}
	if isConditionalCancel(err) {
		return false, nil
	}
	return false, appAccount.ErrDeletionUnavailable
}

// Complete escribe el tombstone mínimo y borra la entrada de cola en la MISMA transacción,
// con condición owner+fence en ambos ítems. Elimina del control todo dato personal
// localizable: emailHash, refreshDigests, lastSK, owner, unknownKeys, userId y cursores.
func (s *DeletionStore) Complete(ctx context.Context, control *appAccount.PurgeControl, owner string, fence int64, deletedCount int64, now time.Time) (bool, error) {
	if !s.disponible() || control == nil {
		return false, appAccount.ErrDeletionUnavailable
	}
	controlPK, controlSK := domainDeletion.UserControlKeys(control.UserID, control.RequestID)
	queuePK, queueSK := domainDeletion.QueueKeys(control.RequestID)
	nowUTC := now.UTC()
	nowISO := nowUTC.Format(time.RFC3339)
	expira := nowUTC.Add(domainDeletion.TombstoneRetention).Unix()

	_, err := s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{Update: &types.Update{
				TableName: aws.String(s.table),
				Key: map[string]types.AttributeValue{
					"PK": stringAttr(controlPK),
					"SK": stringAttr(controlSK),
				},
				UpdateExpression: aws.String(
					"SET #state = :purged, completedAt = :now, #deleted = :deleted, #version = :one, #catalog = if_not_exists(#catalog, :catalog), expiresAt = :expires " +
						"REMOVE emailHash, refreshDigests, lastSK, #owner, unknownKeys, leaseUntil, dueAt, dueAtISO, #attempts, #fence, #phase, userId, createdAt"),
				ConditionExpression: aws.String("#owner = :owner AND #fence = :fence"),
				ExpressionAttributeNames: map[string]string{
					"#state":    "state",
					"#owner":    "owner",
					"#fence":    "fence",
					"#attempts": "attempts",
					"#phase":    "phase",
					"#deleted":  "deletedCount",
					"#version":  "version",
					"#catalog":  "catalogVersion",
				},
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":purged":  stringAttr(appAccount.PurgeStatePurged),
					":now":     stringAttr(nowISO),
					":deleted": numberAttr(deletedCount),
					":one":     numberAttr(1),
					":catalog": stringAttr(domainDeletion.CatalogVersion),
					":expires": numberAttr(expira),
					":owner":   stringAttr(owner),
					":fence":   numberAttr(fence),
				},
			}},
			{Delete: &types.Delete{
				TableName: aws.String(s.table),
				Key: map[string]types.AttributeValue{
					"PK": stringAttr(queuePK),
					"SK": stringAttr(queueSK),
				},
				ConditionExpression: aws.String("#owner = :owner AND #fence = :fence"),
				ExpressionAttributeNames: map[string]string{
					"#owner": "owner",
					"#fence": "fence",
				},
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":owner": stringAttr(owner),
					":fence": numberAttr(fence),
				},
			}},
		},
	})
	if err == nil {
		return true, nil
	}
	if isConditionalCancel(err) {
		return false, nil
	}
	return false, appAccount.ErrDeletionUnavailable
}

// ClearQueueEntry elimina una entrada de cola huérfana (control ya PURGED). Es idempotente.
func (s *DeletionStore) ClearQueueEntry(ctx context.Context, requestID string) error {
	if !s.disponible() {
		return appAccount.ErrDeletionUnavailable
	}
	pk, sk := domainDeletion.QueueKeys(requestID)
	if strings.TrimSpace(sk) == "" {
		return nil
	}
	_, err := s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(pk),
			"SK": stringAttr(sk),
		},
	})
	if err != nil {
		return appAccount.ErrDeletionUnavailable
	}
	return nil
}

// ListPartition recorre la partición del titular con consistencia fuerte, orden ascendente
// por SK y `Limit`. El llamador usa la última SK para continuar.
func (s *DeletionStore) ListPartition(ctx context.Context, userID, startSK string, limit int) ([]appAccount.PartitionItem, error) {
	if !s.disponible() {
		return nil, appAccount.ErrDeletionUnavailable
	}
	if limit <= 0 {
		return nil, appAccount.ErrDeletionUnavailable
	}
	pk := domainDeletion.UserPartition(userID)
	entrada := &dynamodb.QueryInput{
		TableName:              aws.String(s.table),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": stringAttr(pk),
		},
		ConsistentRead:   aws.Bool(true),
		ScanIndexForward: aws.Bool(true),
		Limit:            aws.Int32(limiteDynamo(limit)),
	}
	if strings.TrimSpace(startSK) != "" {
		entrada.ExclusiveStartKey = map[string]types.AttributeValue{
			"PK": stringAttr(pk),
			"SK": stringAttr(startSK),
		}
	}
	out, err := s.client.Query(ctx, entrada)
	if err != nil {
		return nil, appAccount.ErrDeletionUnavailable
	}
	if out == nil {
		return nil, nil
	}
	items := make([]appAccount.PartitionItem, 0, len(out.Items))
	for _, item := range out.Items {
		items = append(items, appAccount.PartitionItem{
			SK:          stringAttribute(item["SK"]),
			EntityType:  stringAttribute(item["entityType"]),
			RefreshHash: stringAttribute(item["refreshHash"]),
			Family:      stringAttribute(item["family"]),
			GlobalSK:    stringAttribute(item["skGlobal"]),
		})
	}
	return items, nil
}

// DeletePartitionItem borra un ítem de la partición del titular. El llamador garantiza que
// la SK no pertenece al control (`DELETION#`); el adapter lo vuelve a comprobar como
// defensa en profundidad.
func (s *DeletionStore) DeletePartitionItem(ctx context.Context, userID, sk string) error {
	if !s.disponible() {
		return appAccount.ErrDeletionUnavailable
	}
	if domainDeletion.IsControlSK(sk) {
		// Nunca se borra el registro de progreso.
		return nil
	}
	pk := domainDeletion.UserPartition(userID)
	_, err := s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(pk),
			"SK": stringAttr(sk),
		},
	})
	if err != nil {
		return appAccount.ErrDeletionUnavailable
	}
	return nil
}

// DeleteGlobalItem borra el ítem global al que apunta un puntero del titular, sin `Scan`:
// la clave llega del puntero (`skGlobal`) y la partición del prefijo del puntero o, en los
// punteros de token, del propio `skGlobal` (PK de la familia).
//
// Controles:
//   - Sólo se aceptan las particiones del servicio declaradas en `domain/deletion`
//     (outbox, scheduler, cola READALL y las particiones de token `RESET#<sha256>` /
//     `ONE_USE#<sha256>`).
//   - Se lee el ítem antes de borrar para comprobar el titular: un puntero corrupto no
//     puede borrar datos de otra cuenta (`GlobalItemForeign`).
//   - Un envío en curso (atributo `owner`/`leaseOwner` con `leaseUntil` vigente) bloquea el
//     borrado (`GlobalItemBusy`) y el trabajo se reintenta cuando el lease vence.
//   - El borrado es condicional: exige que el ítem siga existiendo y que `owner`/
//     `leaseOwner`/`leaseUntil` sean los leídos, de modo que una reclamación concurrente
//     entre la lectura y el borrado no destruya un envío recién reclamado
//     (`GlobalItemBusy` y reintento). DynamoDB no admite dos acciones sobre el mismo ítem
//     en una transacción, así que no se puede «marcar revocado» y borrar de forma atómica.
func (s *DeletionStore) DeleteGlobalItem(ctx context.Context, partition, sk, userID string, now time.Time) (appAccount.GlobalDeleteResult, error) {
	if !s.disponible() {
		return appAccount.GlobalItemBusy, appAccount.ErrDeletionUnavailable
	}
	partition = strings.TrimSpace(partition)
	sk = strings.TrimSpace(sk)
	userID = strings.TrimSpace(userID)
	if !domainDeletion.IsGlobalPartition(partition) || sk == "" || userID == "" {
		return appAccount.GlobalItemBusy, appAccount.ErrDeletionUnavailable
	}

	actual, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(partition),
			"SK": stringAttr(sk),
		},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return appAccount.GlobalItemBusy, appAccount.ErrDeletionUnavailable
	}
	if actual == nil || len(actual.Item) == 0 {
		return appAccount.GlobalItemDeleted, nil
	}
	titular, titularCoherente := titularDeItem(actual.Item)
	if !titularCoherente || (titular != "" && titular != userID) {
		return appAccount.GlobalItemForeign, nil
	}
	if enEnvio(actual.Item, now) {
		return appAccount.GlobalItemBusy, nil
	}

	// Tras el tombstone ninguna reclamación es legítima: se borra el ítem.
	//
	// El borrado es CONDICIONAL (nunca un borrado a ciegas por clave): exige que el ítem
	// siga existiendo y que `owner`/`leaseOwner` y `leaseUntil` sean EXACTAMENTE los
	// leídos. Si otro worker lo reclamó entre la lectura y el borrado, la condición falla
	// y el trabajo se reintenta: no se borra un ítem bajo un envío recién reclamado. Un
	// lease vencido en la lectura no bloquea (ya se comprobó con `enEnvio`), pero su valor
	// exacto forma parte de la condición para detectar la reclamación concurrente.
	condicion, nombres, valores := condicionBorradoGlobal(actual.Item)
	_, err = s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(partition),
			"SK": stringAttr(sk),
		},
		ConditionExpression:       aws.String(condicion),
		ExpressionAttributeNames:  nombres,
		ExpressionAttributeValues: valores,
	})
	if err == nil {
		return appAccount.GlobalItemDeleted, nil
	}
	var condicional *types.ConditionalCheckFailedException
	if errors.As(err, &condicional) {
		// Otro worker reclamó el ítem entre la lectura y el borrado, o ya no existe.
		return appAccount.GlobalItemBusy, nil
	}
	return appAccount.GlobalItemBusy, appAccount.ErrDeletionUnavailable
}

// condicionBorradoGlobal construye la condición del borrado de un ítem global a partir del
// ítem LEÍDO. Siempre exige `attribute_exists(PK)`; añade la igualdad de `owner` y
// `leaseOwner` cuando el ítem los declara y la de `leaseUntil` con el valor exacto, sea
// Number (epoch, cola READALL) o String (RFC3339, outbox). Devuelve nombres y valores vacíos
// (nil) cuando no hacen falta, para no enviar mapas innecesarios.
func condicionBorradoGlobal(item map[string]types.AttributeValue) (string, map[string]string, map[string]types.AttributeValue) {
	condicion := "attribute_exists(PK)"
	nombres := map[string]string{}
	valores := map[string]types.AttributeValue{}
	if propietario := strings.TrimSpace(stringAttribute(item["owner"])); propietario != "" {
		condicion += " AND #owner = :owner"
		nombres["#owner"] = "owner"
		valores[":owner"] = stringAttr(propietario)
	}
	if arrendatario := strings.TrimSpace(stringAttribute(item["leaseOwner"])); arrendatario != "" {
		condicion += " AND #leaseOwner = :leaseOwner"
		nombres["#leaseOwner"] = "leaseOwner"
		valores[":leaseOwner"] = stringAttr(arrendatario)
	}
	if arrendamiento, ok := item["leaseUntil"]; ok && arrendamiento != nil {
		condicion += " AND #leaseUntil = :leaseUntil"
		nombres["#leaseUntil"] = "leaseUntil"
		valores[":leaseUntil"] = arrendamiento
	}
	if len(nombres) == 0 {
		return condicion, nil, nil
	}
	return condicion, nombres, valores
}

// titularDeItem devuelve el titular de un ítem global y si la lectura es fiable. El outbox y
// la cola READALL usan `userID`; los ítems de token de E005/E006 y E010/E011 usan `userId`. Sin
// esta segunda forma, la comprobación de titular de un puntero de token quedaría vacía y un
// puntero manipulado podría borrar el token de otra cuenta.
//
// Si el ítem declara AMBAS formas con valores distintos, la lectura es contradictoria y se
// informa `coherente=false`: la purga no borra ese ítem (falla cerrado) y el puntero se retira
// contabilizándolo como clave desconocida. Elegir una de las dos formas permitiría que un ítem
// manipulado —`userID` del titular con `userId` de un tercero— borrara datos ajenos.
func titularDeItem(item map[string]types.AttributeValue) (string, bool) {
	conMayusculas := strings.TrimSpace(stringAttribute(item["userID"]))
	conMinusculas := strings.TrimSpace(stringAttribute(item["userId"]))
	if conMayusculas != "" && conMinusculas != "" {
		if conMayusculas != conMinusculas {
			return "", false
		}
		return conMayusculas, true
	}
	if conMayusculas != "" {
		return conMayusculas, true
	}
	return conMinusculas, true
}

// DeleteGlobalPointer borra el puntero del titular sólo si sigue apuntando a la referencia
// global leída. La condición impide que una reprogramación concurrente (el outbox mueve el
// ítem global y actualiza `skGlobal` en la MISMA transacción) deje el ítem global nuevo sin
// puntero: si el puntero cambió, se informa `false` y el worker reintenta la página.
//
// Con `expectedGlobalRef` vacío (puntero corrupto sin referencia) el borrado no necesita
// condición sobre `skGlobal`: sólo se protege el control de progreso.
func (s *DeletionStore) DeleteGlobalPointer(ctx context.Context, userID, sk, expectedGlobalRef string) (bool, error) {
	if !s.disponible() {
		return false, appAccount.ErrDeletionUnavailable
	}
	if domainDeletion.IsControlSK(sk) {
		// Nunca se borra el registro de progreso.
		return true, nil
	}
	entrada := &dynamodb.DeleteItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(domainDeletion.UserPartition(userID)),
			"SK": stringAttr(sk),
		},
	}
	if esperado := strings.TrimSpace(expectedGlobalRef); esperado != "" {
		entrada.ConditionExpression = aws.String("attribute_exists(PK) AND #skGlobal = :skGlobal")
		entrada.ExpressionAttributeNames = map[string]string{"#skGlobal": "skGlobal"}
		entrada.ExpressionAttributeValues = map[string]types.AttributeValue{":skGlobal": stringAttr(esperado)}
	}
	_, err := s.client.DeleteItem(ctx, entrada)
	if err == nil {
		return true, nil
	}
	var condicional *types.ConditionalCheckFailedException
	if errors.As(err, &condicional) {
		// El puntero se movió (o ya no existe): no se borra a ciegas; se reintenta.
		return false, nil
	}
	return false, appAccount.ErrDeletionUnavailable
}

// enEnvio indica si el ítem global tiene un lease de envío vigente. Cubre el outbox
// (`owner` + `leaseUntil` RFC3339) y la cola READALL (`leaseOwner` + `leaseUntil` epoch).
func enEnvio(item map[string]types.AttributeValue, now time.Time) bool {
	propietario := strings.TrimSpace(stringAttribute(item["owner"]))
	if propietario == "" {
		propietario = strings.TrimSpace(stringAttribute(item["leaseOwner"]))
	}
	if propietario == "" {
		return false
	}
	if epoch, ok := attributeInt(item["leaseUntil"]); ok {
		return time.Unix(epoch, 0).After(now.UTC())
	}
	lease, errLease := time.Parse(time.RFC3339Nano, strings.TrimSpace(stringAttribute(item["leaseUntil"])))
	if errLease != nil {
		return false
	}
	return lease.After(now.UTC())
}

// ListPasswordResetQueue consulta `QUEUE#PWRESET` filtrando por `userId`. El `Limit` se
// aplica antes del filtro, así que se devuelve la clave de continuación aunque la página no
// aporte coincidencias.
func (s *DeletionStore) ListPasswordResetQueue(ctx context.Context, userID, startSK string, limit int) ([]appAccount.RawItem, string, error) {
	if !s.disponible() {
		return nil, "", appAccount.ErrDeletionUnavailable
	}
	if limit <= 0 {
		limit = 25
	}
	entrada := &dynamodb.QueryInput{
		TableName:              aws.String(s.table),
		KeyConditionExpression: aws.String("PK = :pk"),
		FilterExpression:       aws.String("userId = :user"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":   stringAttr(domainDeletion.PartitionQueueRecuperacion),
			":user": stringAttr(userID),
		},
		ConsistentRead: aws.Bool(true),
		Limit:          aws.Int32(limiteDynamo(limit)),
	}
	if strings.TrimSpace(startSK) != "" {
		entrada.ExclusiveStartKey = map[string]types.AttributeValue{
			"PK": stringAttr(domainDeletion.PartitionQueueRecuperacion),
			"SK": stringAttr(startSK),
		}
	}
	out, err := s.client.Query(ctx, entrada)
	if err != nil {
		return nil, "", appAccount.ErrDeletionUnavailable
	}
	if out == nil {
		return nil, "", nil
	}
	items := make([]appAccount.RawItem, 0, len(out.Items))
	for _, item := range out.Items {
		items = append(items, appAccount.RawItem{
			PK: stringAttribute(item["PK"]),
			SK: stringAttribute(item["SK"]),
		})
	}
	siguiente := ""
	if len(out.LastEvaluatedKey) > 0 {
		siguiente = stringAttribute(out.LastEvaluatedKey["SK"])
	}
	return items, siguiente, nil
}

// DeleteRawItem borra un ítem por su clave completa. Es idempotente.
func (s *DeletionStore) DeleteRawItem(ctx context.Context, pk, sk string) error {
	if !s.disponible() {
		return appAccount.ErrDeletionUnavailable
	}
	if strings.TrimSpace(pk) == "" || strings.TrimSpace(sk) == "" {
		return appAccount.ErrDeletionUnavailable
	}
	_, err := s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(pk),
			"SK": stringAttr(sk),
		},
	})
	if err != nil {
		return appAccount.ErrDeletionUnavailable
	}
	return nil
}

// ExistsItem comprueba con consistencia fuerte si una clave sigue existiendo.
func (s *DeletionStore) ExistsItem(ctx context.Context, pk, sk string) (bool, error) {
	if !s.disponible() {
		return false, appAccount.ErrDeletionUnavailable
	}
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(pk),
			"SK": stringAttr(sk),
		},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return false, appAccount.ErrDeletionUnavailable
	}
	return out != nil && len(out.Item) > 0, nil
}

// loadControl lee el control con consistencia fuerte y lo reduce a la fila de aplicación.
func (s *DeletionStore) loadControl(ctx context.Context, pk, sk string) (*appAccount.PurgeControl, error) {
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(pk),
			"SK": stringAttr(sk),
		},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return nil, appAccount.ErrDeletionUnavailable
	}
	if out == nil || len(out.Item) == 0 {
		return nil, nil
	}
	return controlFromItem(out.Item), nil
}

// controlFromItem reduce el ítem de control. Los digests se guardan como texto separado por
// comas (son hex, sin comas posibles) para evitar conjuntos vacíos, que DynamoDB rechaza.
func controlFromItem(item map[string]types.AttributeValue) *appAccount.PurgeControl {
	control := &appAccount.PurgeControl{
		UserID:         stringAttribute(item["userId"]),
		RequestID:      stringAttribute(item["requestId"]),
		State:          stringAttribute(item["state"]),
		Phase:          stringAttribute(item["phase"]),
		Owner:          stringAttribute(item["owner"]),
		EmailHash:      stringAttribute(item["emailHash"]),
		CatalogVersion: stringAttribute(item["catalogVersion"]),
		LastSK:         stringAttribute(item["lastSK"]),
	}
	if sk := stringAttribute(item["SK"]); control.RequestID == "" && strings.HasPrefix(sk, domainDeletion.PrefixDeletion) {
		control.RequestID = strings.TrimPrefix(sk, domainDeletion.PrefixDeletion)
	}
	if pk := stringAttribute(item["PK"]); control.UserID == "" && strings.HasPrefix(pk, domainDynamo.PrefixUser) {
		control.UserID = strings.TrimPrefix(pk, domainDynamo.PrefixUser)
	}
	if fence, ok := attributeInt(item["fence"]); ok {
		control.Fence = fence
	}
	if attempts, ok := attributeInt(item["attempts"]); ok {
		control.Attempts = attempts
	}
	if deleted, ok := attributeInt(item["deletedCount"]); ok {
		control.DeletedCount = deleted
	}
	if unknown, ok := attributeInt(item["unknownKeys"]); ok {
		control.UnknownKeys = unknown
	}
	if due, ok := attributeInt(item["dueAt"]); ok && due > 0 {
		control.DueAt = time.Unix(due, 0).UTC()
	}
	if lease, ok := attributeInt(item["leaseUntil"]); ok && lease > 0 {
		control.LeaseUntil = time.Unix(lease, 0).UTC()
	}
	if digests := stringAttribute(item["refreshDigests"]); digests != "" {
		for _, digest := range strings.Split(digests, ",") {
			digest = strings.TrimSpace(digest)
			if digest != "" {
				control.RefreshDigests = append(control.RefreshDigests, digest)
			}
		}
	}
	return control
}

// limiteDynamo acota el tamano de pagina al rango valido de DynamoDB (1..1000) antes
// de convertirlo a int32: evita el desbordamiento que gosec marca como G115.
func limiteDynamo(limit int) int32 {
	if limit <= 0 {
		return 1
	}
	if limit > 1000 {
		return 1000
	}
	return int32(limit)
}
