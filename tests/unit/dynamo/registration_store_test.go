package dynamo_test

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
)

type fakeDynamo struct {
	transactErrs  []error
	transactCalls int
	transactInput *dynamodb.TransactWriteItemsInput

	getCalls  int
	getErr    error
	getInput  *dynamodb.GetItemInput
	getOutput *dynamodb.GetItemOutput
}

func (f *fakeDynamo) TransactWriteItems(_ context.Context, params *dynamodb.TransactWriteItemsInput, _ ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	f.transactCalls++
	f.transactInput = params
	if len(f.transactErrs) > 0 {
		err := f.transactErrs[0]
		f.transactErrs = f.transactErrs[1:]
		if err != nil {
			return nil, err
		}
	}
	return &dynamodb.TransactWriteItemsOutput{}, nil
}

func (f *fakeDynamo) GetItem(_ context.Context, params *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	f.getCalls++
	f.getInput = params
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.getOutput, nil
}

func strAttr(value string) types.AttributeValue {
	return &types.AttributeValueMemberS{Value: value}
}

func numAttr(value int64) types.AttributeValue {
	return &types.AttributeValueMemberN{Value: strconv.FormatInt(value, 10)}
}

func registrationRecord() appIdentity.RegistrationRecord {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	return appIdentity.RegistrationRecord{
		UserID:               "018f90ef-08e5-4a68-b09f-000000000001",
		Email:                "usuario@example.invalid",
		DisplayName:          "Usuario Ejemplo",
		PasswordHash:         "$argon2id$v=19$m=65536,t=3,p=2$c2FsdA$aGFzaA",
		PrivacyNoticeVersion: "2026-11",
		PrivacyAcceptedAt:    now,
		EmailVerified:        false,
		Version:              1,
		CreatedAt:            now,
	}
}

func storedResponse() appIdentity.StoredResponse {
	return appIdentity.StoredResponse{
		Status:   201,
		Body:     []byte(`{"id":"018f90ef-08e5-4a68-b09f-000000000001"}`),
		Location: "/api/v1/users/me",
	}
}

func cancellationAt(indices ...int) *types.TransactionCanceledException {
	reasons := make([]types.CancellationReason, 3)
	for i := range reasons {
		reasons[i] = types.CancellationReason{Code: aws.String("None")}
	}
	for _, index := range indices {
		if index >= 0 && index < len(reasons) {
			reasons[index] = types.CancellationReason{Code: aws.String("ConditionalCheckFailed")}
		}
	}
	return &types.TransactionCanceledException{CancellationReasons: reasons}
}

func TestRegistrationStoreBuildsAtomicTransaction(t *testing.T) {
	fake := &fakeDynamo{}
	store := dynamoAdapter.NewRegistrationStore(fake, "NEXUM_CORE_TEST")

	replay, err := store.CreateOrReplay(
		context.Background(),
		"0123456789abcdef",
		appIdentity.RegisterOperation,
		"payload-hash",
		registrationRecord(),
		storedResponse(),
		time.Hour,
	)
	if err != nil {
		t.Fatalf("transaccion fallo: %v", err)
	}
	if replay != nil {
		t.Fatal("la primera escritura no debe ser replay")
	}
	if fake.transactCalls != 1 {
		t.Fatalf("se esperaba 1 transaccion, obtenidas %d", fake.transactCalls)
	}
	if fake.transactInput == nil || len(fake.transactInput.TransactItems) != 3 {
		t.Fatalf("se esperaban 3 acciones atomicas: %+v", fake.transactInput)
	}

	emailPut := fake.transactInput.TransactItems[0].Put
	if emailPut == nil || aws.ToString(emailPut.ConditionExpression) != "attribute_not_exists(PK)" {
		t.Fatalf("reserva de email sin condicion: %+v", emailPut)
	}
	emailPK := emailPut.Item["PK"].(*types.AttributeValueMemberS).Value
	if !strings.HasPrefix(emailPK, "EMAIL#") {
		t.Fatalf("PK de email inesperada: %q", emailPK)
	}

	userPut := fake.transactInput.TransactItems[1].Put
	if userPut == nil || aws.ToString(userPut.ConditionExpression) != "attribute_not_exists(PK)" {
		t.Fatalf("perfil sin condicion: %+v", userPut)
	}
	userPK := userPut.Item["PK"].(*types.AttributeValueMemberS).Value
	if !strings.HasPrefix(userPK, "USER#") {
		t.Fatalf("PK de usuario inesperada: %q", userPK)
	}
	if userPut.Item["emailVerified"].(*types.AttributeValueMemberBOOL).Value {
		t.Fatal("emailVerified persistido debe ser false")
	}

	idemPut := fake.transactInput.TransactItems[2].Put
	if idemPut == nil {
		t.Fatal("falta accion de idempotencia")
	}
	if !strings.Contains(aws.ToString(idemPut.ConditionExpression), "expiresAt < :now") {
		t.Fatalf("condicion de idempotencia inesperada: %q", aws.ToString(idemPut.ConditionExpression))
	}
	idemPK := idemPut.Item["PK"].(*types.AttributeValueMemberS).Value
	if !strings.HasPrefix(idemPK, "IDEM#") {
		t.Fatalf("PK de idempotencia inesperada: %q", idemPK)
	}
	if _, ok := idemPut.ExpressionAttributeValues[":now"]; !ok {
		t.Fatal("falta valor :now de la condicion de idempotencia")
	}
	if strings.Contains(idemPK, "0123456789abcdef") {
		t.Fatal("la Idempotency-Key cruda no debe persistirse en la PK")
	}
}

func TestRegistrationStoreReplaysStoredResponse(t *testing.T) {
	fake := &fakeDynamo{
		transactErrs: []error{cancellationAt(2)},
		getOutput: &dynamodb.GetItemOutput{Item: map[string]types.AttributeValue{
			"payloadHash":    strAttr("payload-hash"),
			"expiresAt":      numAttr(time.Now().Add(time.Hour).Unix()),
			"responseStatus": numAttr(201),
			"responseBody":   strAttr(`{"id":"018f90ef-08e5-4a68-b09f-000000000001"}`),
			"location":       strAttr("/api/v1/users/me"),
		}},
	}
	store := dynamoAdapter.NewRegistrationStore(fake, "NEXUM_CORE_TEST")

	result, err := store.CreateOrReplay(
		context.Background(),
		"0123456789abcdef",
		appIdentity.RegisterOperation,
		"payload-hash",
		registrationRecord(),
		storedResponse(),
		time.Hour,
	)
	if err != nil {
		t.Fatalf("replay fallo: %v", err)
	}
	if result == nil || !result.Replayed {
		t.Fatalf("se esperaba replay: %+v", result)
	}
	if result.Status != 201 || result.Location != "/api/v1/users/me" {
		t.Fatalf("respuesta replay inesperada: %+v", result)
	}
	if fake.getInput == nil || fake.getInput.ConsistentRead == nil || !*fake.getInput.ConsistentRead {
		t.Fatal("GetItem de idempotencia debe usar ConsistentRead=true")
	}
}

func TestRegistrationStoreIdempotencyConflict(t *testing.T) {
	fake := &fakeDynamo{
		transactErrs: []error{cancellationAt(2)},
		getOutput: &dynamodb.GetItemOutput{Item: map[string]types.AttributeValue{
			"payloadHash":    strAttr("otro-payload"),
			"expiresAt":      numAttr(time.Now().Add(time.Hour).Unix()),
			"responseStatus": numAttr(201),
			"responseBody":   strAttr(`{"id":"otro"}`),
		}},
	}
	store := dynamoAdapter.NewRegistrationStore(fake, "NEXUM_CORE_TEST")

	_, err := store.CreateOrReplay(
		context.Background(),
		"0123456789abcdef",
		appIdentity.RegisterOperation,
		"payload-hash",
		registrationRecord(),
		storedResponse(),
		time.Hour,
	)
	if !errors.Is(err, appIdentity.ErrIdempotencyConflict) {
		t.Fatalf("se esperaba ErrIdempotencyConflict, obtenido %v", err)
	}
}

func TestRegistrationStoreEmailConflict(t *testing.T) {
	fake := &fakeDynamo{transactErrs: []error{cancellationAt(0)}}
	store := dynamoAdapter.NewRegistrationStore(fake, "NEXUM_CORE_TEST")

	_, err := store.CreateOrReplay(
		context.Background(),
		"0123456789abcdef",
		appIdentity.RegisterOperation,
		"payload-hash",
		registrationRecord(),
		storedResponse(),
		time.Hour,
	)
	if !errors.Is(err, appIdentity.ErrEmailAlreadyRegistered) {
		t.Fatalf("se esperaba ErrEmailAlreadyRegistered, obtenido %v", err)
	}
}

func TestRegistrationStoreUserConflictIsInternal(t *testing.T) {
	fake := &fakeDynamo{transactErrs: []error{cancellationAt(1)}}
	store := dynamoAdapter.NewRegistrationStore(fake, "NEXUM_CORE_TEST")

	_, err := store.CreateOrReplay(
		context.Background(),
		"0123456789abcdef",
		appIdentity.RegisterOperation,
		"payload-hash",
		registrationRecord(),
		storedResponse(),
		time.Hour,
	)
	if !errors.Is(err, appIdentity.ErrRegistrationInternal) {
		t.Fatalf("se esperaba ErrRegistrationInternal, obtenido %v", err)
	}
}

func TestRegistrationStoreUnavailable(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"tabla inexistente", &types.ResourceNotFoundException{}},
		{"conflicto de transaccion", &types.TransactionCanceledException{CancellationReasons: []types.CancellationReason{
			{Code: aws.String("TransactionConflict")},
			{Code: aws.String("TransactionConflict")},
			{Code: aws.String("TransactionConflict")},
		}}},
		{"throttling", errors.New("throttled")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeDynamo{transactErrs: []error{tc.err}}
			store := dynamoAdapter.NewRegistrationStore(fake, "NEXUM_CORE_TEST")
			_, err := store.CreateOrReplay(
				context.Background(),
				"0123456789abcdef",
				appIdentity.RegisterOperation,
				"payload-hash",
				registrationRecord(),
				storedResponse(),
				time.Hour,
			)
			if !errors.Is(err, appIdentity.ErrStoreUnavailable) {
				t.Fatalf("se esperaba ErrStoreUnavailable, obtenido %v", err)
			}
		})
	}
}

func TestRegistrationStoreExpiredIdempotencyRetriesOnce(t *testing.T) {
	fake := &fakeDynamo{
		transactErrs: []error{cancellationAt(2), nil},
		getOutput: &dynamodb.GetItemOutput{Item: map[string]types.AttributeValue{
			"payloadHash":    strAttr("payload-hash"),
			"expiresAt":      numAttr(time.Now().Add(-time.Hour).Unix()),
			"responseStatus": numAttr(201),
			"responseBody":   strAttr(`{"id":"viejo"}`),
		}},
	}
	store := dynamoAdapter.NewRegistrationStore(fake, "NEXUM_CORE_TEST")

	result, err := store.CreateOrReplay(
		context.Background(),
		"0123456789abcdef",
		appIdentity.RegisterOperation,
		"payload-hash",
		registrationRecord(),
		storedResponse(),
		time.Hour,
	)
	if err != nil {
		t.Fatalf("reintento fallo: %v", err)
	}
	if result != nil {
		t.Fatalf("el reintento exitoso no debe ser replay: %+v", result)
	}
	if fake.transactCalls != 2 {
		t.Fatalf("se esperaban 2 transacciones, obtenidas %d", fake.transactCalls)
	}
}

func TestRegistrationStoreGetItemFailureIsUnavailable(t *testing.T) {
	fake := &fakeDynamo{
		transactErrs: []error{cancellationAt(2)},
		getErr:       errors.New("red caida"),
	}
	store := dynamoAdapter.NewRegistrationStore(fake, "NEXUM_CORE_TEST")

	_, err := store.CreateOrReplay(
		context.Background(),
		"0123456789abcdef",
		appIdentity.RegisterOperation,
		"payload-hash",
		registrationRecord(),
		storedResponse(),
		time.Hour,
	)
	if !errors.Is(err, appIdentity.ErrStoreUnavailable) {
		t.Fatalf("se esperaba ErrStoreUnavailable, obtenido %v", err)
	}
}

func TestRegistrationStoreWithoutClientOrTableFailsClosed(t *testing.T) {
	cases := []*dynamoAdapter.RegistrationStore{
		dynamoAdapter.NewRegistrationStore(nil, "NEXUM_CORE_TEST"),
		dynamoAdapter.NewRegistrationStore(&fakeDynamo{}, ""),
	}
	for _, store := range cases {
		_, err := store.CreateOrReplay(
			context.Background(),
			"0123456789abcdef",
			appIdentity.RegisterOperation,
			"payload-hash",
			registrationRecord(),
			storedResponse(),
			time.Hour,
		)
		if !errors.Is(err, appIdentity.ErrStoreUnavailable) {
			t.Fatalf("se esperaba ErrStoreUnavailable, obtenido %v", err)
		}
	}
}
