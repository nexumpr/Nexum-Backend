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

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
)

// Limites de TransactWriteItems documentados por AWS: 100 acciones y 4 MB por
// transaccion. El alta de E001 gasta seis acciones fijas, de modo que el maximo de
// compromisos es 94. Los atributos de cada fila estan acotados por el contrato
// (concepto <= 100 runes), por lo que el peor caso queda muy por debajo de 4 MB; el
// limite de bytes se comprueba igualmente para fallar cerrado.
const (
	// MaxTransactionItems es el maximo de acciones por transaccion de DynamoDB.
	MaxTransactionItems = 100

	// fixedRegistrationItems son las acciones que no dependen de la lista de gastos:
	// reserva de email, perfil, ingreso, consentimiento, onboarding e idempotencia.
	fixedRegistrationItems = 6

	// maxTransactionBytes deja margen sobre los 4 MB de AWS para no depender del
	// redondeo interno del servicio.
	maxTransactionBytes = 3_500_000
)

// DynamoAPI es el subconjunto del cliente DynamoDB que usa el registro atómico de E001.
// Se declara como interfaz para poder probar la construccion de la transaccion con dobles.
type DynamoAPI interface {
	TransactWriteItems(ctx context.Context, params *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error)
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
}

// RegistrationStore implementa appIdentity.RegisterStore con TransactWriteItems real.
// No usa GetItem+PutItem para comprobar unicidad: la reserva de email, el perfil, el
// ingreso, los compromisos, el consentimiento, el onboarding y la idempotencia se
// escriben o se cancelan juntos.
type RegistrationStore struct {
	client DynamoAPI
	table  string
	now    func() time.Time
}

// NewRegistrationStore construye el almacen real. Si client o table faltan, todas las
// operaciones fallan cerradas con ErrStoreUnavailable (nunca se usa memoria en produccion).
func NewRegistrationStore(client DynamoAPI, table string) *RegistrationStore {
	return &RegistrationStore{
		client: client,
		table:  strings.TrimSpace(table),
		now:    func() time.Time { return time.Now().UTC() },
	}
}

// WithClock permite inyectar un reloj determinista en pruebas.
func (s *RegistrationStore) WithClock(now func() time.Time) *RegistrationStore {
	if now != nil {
		s.now = now
	}
	return s
}

type registrationWrite struct {
	idempotencyKey string
	operation      string
	payloadHash    string
	record         appIdentity.RegistrationRecord
	response       appIdentity.StoredResponse
	ttl            time.Duration
}

// itemKind identifica el proposito de cada accion de la transaccion para traducir los
// motivos de cancelacion de DynamoDB sin depender de indices fijos.
type itemKind int

const (
	kindEmailReservation itemKind = iota
	kindProfile
	kindIncome
	kindConsent
	kindOnboarding
	kindRecurring
	kindIdempotency
)

// CreateOrReplay ejecuta la transaccion atomica de E001.
func (s *RegistrationStore) CreateOrReplay(
	ctx context.Context,
	idempotencyKey, operation, payloadHash string,
	record appIdentity.RegistrationRecord,
	response appIdentity.StoredResponse,
	ttl time.Duration,
) (*appIdentity.StoredResponse, error) {
	return s.createOrReplay(ctx, registrationWrite{
		idempotencyKey: idempotencyKey,
		operation:      operation,
		payloadHash:    payloadHash,
		record:         record,
		response:       response,
		ttl:            ttl,
	}, true)
}

func (s *RegistrationStore) createOrReplay(
	ctx context.Context,
	write registrationWrite,
	allowExpiredRetry bool,
) (*appIdentity.StoredResponse, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, appIdentity.ErrStoreUnavailable
	}
	if write.ttl <= 0 {
		write.ttl = appIdentity.RegisterIdempotencyTTL
	}

	if len(write.record.RecurringExpenses) > MaxTransactionItems-fixedRegistrationItems {
		return nil, appIdentity.ErrTransactionTooLarge
	}

	now := s.now().UTC()
	expiresAt := now.Add(write.ttl).Unix()
	createdAt := write.record.CreatedAt.UTC().Format(time.RFC3339)
	updatedAt := now.Format(time.RFC3339)
	acceptedAt := write.record.ConsentAcceptedAt.UTC().Format(time.RFC3339)

	emailPK, emailSK := domainDynamo.EmailReservationKeys(write.record.Email)
	userPK, userSK := domainDynamo.UserKeys(write.record.UserID)
	incomePK, incomeSK := domainDynamo.IncomeKeys(write.record.UserID)
	consentPK, consentSK := domainDynamo.ConsentKeys(write.record.UserID, write.record.ConsentNoticeVersion)
	onboardingPK, onboardingSK := domainDynamo.OnboardingKeys(write.record.UserID)
	idemPK, idemSK := domainDynamo.IdempotencyKeys(write.operation, write.idempotencyKey)

	emailItem := map[string]types.AttributeValue{
		"PK":         stringAttr(emailPK),
		"SK":         stringAttr(emailSK),
		"entityType": stringAttr("EMAIL_RESERVATION"),
		"userId":     stringAttr(write.record.UserID),
		"emailHash":  stringAttr(strings.TrimPrefix(emailPK, domainDynamo.PrefixEmail)),
		"createdAt":  stringAttr(createdAt),
		"version":    numberAttr(1),
	}

	userItem := map[string]types.AttributeValue{
		"PK":         stringAttr(userPK),
		"SK":         stringAttr(userSK),
		"entityType": stringAttr("USER_PROFILE"),
		"userId":     stringAttr(write.record.UserID),
		"email":      stringAttr(write.record.Email),
		// `displayName` es el atributo canonico historico; el contrato V6.0.2 lo
		// expone como `name`. No se duplica el valor en el item.
		"displayName":   stringAttr(write.record.Name),
		"passwordHash":  stringAttr(write.record.PasswordHash),
		"emailVerified": boolAttr(write.record.EmailVerified),
		// Campos heredados de H06: se conservan en PROFILE para que los perfiles
		// antiguos y los nuevos tengan la misma forma. El registro versionado y
		// autoritativo del consentimiento es el item CONSENT#<version>.
		"privacyNoticeVersion": stringAttr(write.record.ConsentNoticeVersion),
		"privacyAcceptedAt":    stringAttr(acceptedAt),
		"status":               stringAttr("ACTIVE"),
		"createdAt":            stringAttr(createdAt),
		"updatedAt":            stringAttr(updatedAt),
		"version":              numberAttr(1),
	}

	incomeItem := map[string]types.AttributeValue{
		"PK":                     stringAttr(incomePK),
		"SK":                     stringAttr(incomeSK),
		"entityType":             stringAttr("USER_INCOME"),
		"userId":                 stringAttr(write.record.UserID),
		"currency":               stringAttr(appIdentity.RegisterCurrency),
		"amountCents":            numberAttr(write.record.Income.AmountCents),
		"frequency":              stringAttr(write.record.Income.Frequency),
		"monthlyEquivalentCents": numberAttr(write.record.Income.MonthlyEquivalentCents),
		"createdAt":              stringAttr(createdAt),
		"updatedAt":              stringAttr(updatedAt),
		"version":                numberAttr(1),
	}

	consentItem := map[string]types.AttributeValue{
		"PK":               stringAttr(consentPK),
		"SK":               stringAttr(consentSK),
		"entityType":       stringAttr("USER_CONSENT"),
		"userId":           stringAttr(write.record.UserID),
		"noticeVersion":    stringAttr(write.record.ConsentNoticeVersion),
		"acceptedAt":       stringAttr(acceptedAt),
		"acceptedByServer": boolAttr(true),
		"createdAt":        stringAttr(createdAt),
		"version":          numberAttr(1),
	}

	onboardingItem := map[string]types.AttributeValue{
		"PK":            stringAttr(onboardingPK),
		"SK":            stringAttr(onboardingSK),
		"entityType":    stringAttr("USER_ONBOARDING"),
		"userId":        stringAttr(write.record.UserID),
		"microphone":    stringAttr(write.record.OnboardingMicrophone),
		"notifications": stringAttr(write.record.OnboardingNotifications),
		"configured":    boolAttr(write.record.OnboardingConfigured),
		"createdAt":     stringAttr(createdAt),
		"updatedAt":     stringAttr(updatedAt),
		"version":       numberAttr(1),
	}

	idemItem := map[string]types.AttributeValue{
		"PK":             stringAttr(idemPK),
		"SK":             stringAttr(idemSK),
		"entityType":     stringAttr("IDEMPOTENCY_RECORD"),
		"operation":      stringAttr(write.operation),
		"payloadHash":    stringAttr(write.payloadHash),
		"responseStatus": numberAttr(int64(write.response.Status)),
		"responseBody":   stringAttr(string(write.response.Body)),
		"location":       stringAttr(write.response.Location),
		"createdAt":      stringAttr(now.Format(time.RFC3339)),
		"expiresAt":      numberAttr(expiresAt),
	}
	// Verificador Argon2id del solicitante original: permite probar en el replay que
	// la contrasena presentada es la misma sin guardar la contrasena ni un digest
	// simple. Es el mismo valor que ya contiene PROFILE.passwordHash.
	if write.response.PasswordVerifier != "" {
		idemItem["passwordVerifier"] = stringAttr(write.response.PasswordVerifier)
	}

	items := make([]types.TransactWriteItem, 0, fixedRegistrationItems+len(write.record.RecurringExpenses))
	kinds := make([]itemKind, 0, cap(items))

	items = append(items, types.TransactWriteItem{Put: &types.Put{
		TableName:           aws.String(s.table),
		Item:                emailItem,
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	}})
	kinds = append(kinds, kindEmailReservation)

	items = append(items, types.TransactWriteItem{Put: &types.Put{
		TableName:           aws.String(s.table),
		Item:                userItem,
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	}})
	kinds = append(kinds, kindProfile)

	items = append(items, types.TransactWriteItem{Put: &types.Put{
		TableName:           aws.String(s.table),
		Item:                incomeItem,
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	}})
	kinds = append(kinds, kindIncome)

	items = append(items, types.TransactWriteItem{Put: &types.Put{
		TableName:           aws.String(s.table),
		Item:                consentItem,
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	}})
	kinds = append(kinds, kindConsent)

	items = append(items, types.TransactWriteItem{Put: &types.Put{
		TableName:           aws.String(s.table),
		Item:                onboardingItem,
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	}})
	kinds = append(kinds, kindOnboarding)

	for _, expense := range write.record.RecurringExpenses {
		recurringPK, recurringSK := domainDynamo.RecurringKeys(write.record.UserID, expense.ID)
		items = append(items, types.TransactWriteItem{Put: &types.Put{
			TableName: aws.String(s.table),
			Item: map[string]types.AttributeValue{
				"PK":          stringAttr(recurringPK),
				"SK":          stringAttr(recurringSK),
				"entityType":  stringAttr("RECURRING_PAYMENT"),
				"userId":      stringAttr(write.record.UserID),
				"recurringId": stringAttr(expense.ID),
				"concept":     stringAttr(expense.Concept),
				"currency":    stringAttr(appIdentity.RegisterCurrency),
				"amountCents": numberAttr(expense.AmountCents),
				"frequency":   stringAttr(expense.Frequency),
				"category":    stringAttr(expense.Category),
				"expenseType": stringAttr(expense.Type),
				"createdAt":   stringAttr(createdAt),
				"updatedAt":   stringAttr(updatedAt),
				"version":     numberAttr(1),
			},
			ConditionExpression: aws.String("attribute_not_exists(PK)"),
		}})
		kinds = append(kinds, kindRecurring)
	}

	items = append(items, types.TransactWriteItem{Put: &types.Put{
		TableName:                 aws.String(s.table),
		Item:                      idemItem,
		ConditionExpression:       aws.String("attribute_not_exists(PK) OR expiresAt < :now"),
		ExpressionAttributeValues: map[string]types.AttributeValue{":now": numberAttr(now.Unix())},
	}})
	kinds = append(kinds, kindIdempotency)

	if len(items) > MaxTransactionItems {
		return nil, appIdentity.ErrTransactionTooLarge
	}
	if estimateBytes(items) > maxTransactionBytes {
		return nil, appIdentity.ErrTransactionTooLarge
	}

	input := &dynamodb.TransactWriteItemsInput{TransactItems: items}

	_, err := s.client.TransactWriteItems(ctx, input)
	if err == nil {
		return nil, nil
	}

	var canceled *types.TransactionCanceledException
	if errors.As(err, &canceled) {
		if index := failureIndex(canceled.CancellationReasons, kinds, kindIdempotency); index >= 0 {
			return s.replayOrConflict(ctx, write, idemPK, idemSK, allowExpiredRetry)
		}
		if failureIndex(canceled.CancellationReasons, kinds, kindEmailReservation) >= 0 {
			return nil, appIdentity.ErrEmailAlreadyRegistered
		}
		// Perfil, ingreso, consentimiento, onboarding o compromisos: conflicto de
		// estado inesperado. No se confirma ninguna escritura parcial.
		if hasConditionalFailure(canceled.CancellationReasons, kinds) {
			return nil, appIdentity.ErrRegistrationInternal
		}
		// TransactionConflict, throttling u otras causas: no se confirma escritura parcial.
		return nil, appIdentity.ErrStoreUnavailable
	}

	var notFound *types.ResourceNotFoundException
	if errors.As(err, &notFound) {
		return nil, appIdentity.ErrStoreUnavailable
	}

	return nil, appIdentity.ErrStoreUnavailable
}

// failureIndex devuelve el indice del primer fallo condicional del tipo indicado.
func failureIndex(reasons []types.CancellationReason, kinds []itemKind, kind itemKind) int {
	for i, reason := range reasons {
		if i >= len(kinds) || kinds[i] != kind {
			continue
		}
		if reason.Code != nil && *reason.Code == "ConditionalCheckFailed" {
			return i
		}
	}
	return -1
}

// hasConditionalFailure indica si alguna accion fallo por condicion, sin importar cual.
func hasConditionalFailure(reasons []types.CancellationReason, kinds []itemKind) bool {
	for i, reason := range reasons {
		if i >= len(kinds) {
			break
		}
		if reason.Code != nil && *reason.Code == "ConditionalCheckFailed" {
			return true
		}
	}
	return false
}

// estimateBytes aproxima el tamano de la transaccion sumando nombres y valores de
// atributo, mas una sobrecarga fija por accion.
func estimateBytes(items []types.TransactWriteItem) int {
	total := 0
	for _, item := range items {
		if item.Put == nil {
			continue
		}
		total += 100
		for name, value := range item.Put.Item {
			total += len(name)
			total += attributeSize(value)
		}
	}
	return total
}

func attributeSize(value types.AttributeValue) int {
	switch v := value.(type) {
	case *types.AttributeValueMemberS:
		return len(v.Value)
	case *types.AttributeValueMemberN:
		return len(v.Value)
	case *types.AttributeValueMemberBOOL:
		return 1
	default:
		return 32
	}
}

func (s *RegistrationStore) replayOrConflict(
	ctx context.Context,
	write registrationWrite,
	idemPK, idemSK string,
	allowExpiredRetry bool,
) (*appIdentity.StoredResponse, error) {
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(idemPK),
			"SK": stringAttr(idemSK),
		},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return nil, appIdentity.ErrStoreUnavailable
	}
	if out == nil || out.Item == nil {
		return nil, appIdentity.ErrRegistrationInternal
	}

	if storedHash := stringAttribute(out.Item["payloadHash"]); storedHash != write.payloadHash {
		return nil, appIdentity.ErrIdempotencyConflict
	}

	expiresAt, err := strconv.ParseInt(stringAttribute(out.Item["expiresAt"]), 10, 64)
	if err != nil {
		return nil, appIdentity.ErrRegistrationInternal
	}
	if expiresAt > 0 && s.now().UTC().Unix() >= expiresAt {
		if allowExpiredRetry {
			// TTL aun no elimino el item: se reintenta la misma transaccion una vez.
			return s.createOrReplay(ctx, write, false)
		}
		return nil, appIdentity.ErrIdempotencyConflict
	}

	status, err := strconv.Atoi(stringAttribute(out.Item["responseStatus"]))
	if err != nil || status < 100 || status > 599 {
		return nil, appIdentity.ErrRegistrationInternal
	}

	body := stringAttribute(out.Item["responseBody"])
	if body == "" {
		return nil, appIdentity.ErrRegistrationInternal
	}

	return &appIdentity.StoredResponse{
		Status:           status,
		Body:             []byte(body),
		Location:         stringAttribute(out.Item["location"]),
		Replayed:         true,
		PasswordVerifier: stringAttribute(out.Item["passwordVerifier"]),
	}, nil
}

func stringAttr(value string) types.AttributeValue {
	return &types.AttributeValueMemberS{Value: value}
}

func numberAttr(value int64) types.AttributeValue {
	return &types.AttributeValueMemberN{Value: strconv.FormatInt(value, 10)}
}

func boolAttr(value bool) types.AttributeValue {
	return &types.AttributeValueMemberBOOL{Value: value}
}

func stringAttribute(value types.AttributeValue) string {
	switch v := value.(type) {
	case *types.AttributeValueMemberS:
		return v.Value
	case *types.AttributeValueMemberN:
		return v.Value
	default:
		return ""
	}
}
