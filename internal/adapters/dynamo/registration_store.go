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

// DynamoAPI es el subconjunto del cliente DynamoDB que usa el registro atómico de E001.
// Se declara como interfaz para poder probar la construccion de la transaccion con dobles.
type DynamoAPI interface {
	TransactWriteItems(ctx context.Context, params *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error)
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
}

// RegistrationStore implementa appIdentity.RegisterStore con TransactWriteItems real.
// No usa GetItem+PutItem para comprobar unicidad: la reserva de email, el perfil y la
// idempotencia se escriben o se cancelan juntos.
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

	now := s.now().UTC()
	expiresAt := now.Add(write.ttl).Unix()
	createdAt := write.record.CreatedAt.UTC().Format(time.RFC3339)
	updatedAt := now.Format(time.RFC3339)
	acceptedAt := write.record.PrivacyAcceptedAt.UTC().Format(time.RFC3339)

	emailPK, emailSK := domainDynamo.EmailReservationKeys(write.record.Email)
	userPK, userSK := domainDynamo.UserKeys(write.record.UserID)
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
		"PK":                   stringAttr(userPK),
		"SK":                   stringAttr(userSK),
		"entityType":           stringAttr("USER_PROFILE"),
		"userId":               stringAttr(write.record.UserID),
		"email":                stringAttr(write.record.Email),
		"displayName":          stringAttr(write.record.DisplayName),
		"passwordHash":         stringAttr(write.record.PasswordHash),
		"emailVerified":        boolAttr(write.record.EmailVerified),
		"privacyNoticeVersion": stringAttr(write.record.PrivacyNoticeVersion),
		"privacyAcceptedAt":    stringAttr(acceptedAt),
		"status":               stringAttr("ACTIVE"),
		"createdAt":            stringAttr(createdAt),
		"updatedAt":            stringAttr(updatedAt),
		"version":              numberAttr(1),
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

	input := &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{
				Put: &types.Put{
					TableName:           aws.String(s.table),
					Item:                emailItem,
					ConditionExpression: aws.String("attribute_not_exists(PK)"),
				},
			},
			{
				Put: &types.Put{
					TableName:           aws.String(s.table),
					Item:                userItem,
					ConditionExpression: aws.String("attribute_not_exists(PK)"),
				},
			},
			{
				Put: &types.Put{
					TableName:                 aws.String(s.table),
					Item:                      idemItem,
					ConditionExpression:       aws.String("attribute_not_exists(PK) OR expiresAt < :now"),
					ExpressionAttributeValues: map[string]types.AttributeValue{":now": numberAttr(now.Unix())},
				},
			},
		},
	}

	_, err := s.client.TransactWriteItems(ctx, input)
	if err == nil {
		return nil, nil
	}

	var canceled *types.TransactionCanceledException
	if errors.As(err, &canceled) {
		if conditionalFailure(canceled.CancellationReasons, 2) {
			return s.replayOrConflict(ctx, write, idemPK, idemSK, allowExpiredRetry)
		}
		if conditionalFailure(canceled.CancellationReasons, 0) {
			return nil, appIdentity.ErrEmailAlreadyRegistered
		}
		if conditionalFailure(canceled.CancellationReasons, 1) {
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
		Status:   status,
		Body:     []byte(body),
		Location: stringAttribute(out.Item["location"]),
		Replayed: true,
	}, nil
}

func conditionalFailure(reasons []types.CancellationReason, index int) bool {
	if index < 0 || index >= len(reasons) {
		return false
	}
	code := reasons[index].Code
	return code != nil && *code == "ConditionalCheckFailed"
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
