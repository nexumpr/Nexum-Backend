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

const folioIdem = "0123456789abcdef"

const huellaSintetica = "$argon2id$v=19$m=65536,t=3,p=2$c2FsdA$aGFzaA"

func expenseFixture(index int) appIdentity.RegistrationExpense {
	return appIdentity.RegistrationExpense{
		ID:          "018f90ef-08e5-4a68-b09f-00000000010" + strconv.Itoa(index),
		Concept:     "Compromiso " + strconv.Itoa(index),
		AmountCents: int64(69900 + index),
		Frequency:   "monthly",
		Category:    "Servicios",
		Type:        "fixed",
	}
}

func registrationRecord(expenses ...appIdentity.RegistrationExpense) appIdentity.RegistrationRecord {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	return appIdentity.RegistrationRecord{
		UserID:       "018f90ef-08e5-4a68-b09f-000000000001",
		Email:        "usuario@example.invalid",
		Name:         "Usuario Ejemplo",
		PasswordHash: huellaSintetica,
		Income: appIdentity.RegistrationIncome{
			AmountCents:            1200000,
			Frequency:              "monthly",
			MonthlyEquivalentCents: 1200000,
		},
		RecurringExpenses:       expenses,
		ConsentNoticeVersion:    "2026-11",
		ConsentAcceptedAt:       now,
		EmailVerified:           false,
		Version:                 1,
		CreatedAt:               now,
		UpdatedAt:               now,
		OnboardingMicrophone:    "pending",
		OnboardingNotifications: "pending",
		OnboardingConfigured:    false,
		ClientLocale:            "es-MX",
		ClientTimeZone:          "America/Mexico_City",
	}
}

func storedResponse() appIdentity.StoredResponse {
	return appIdentity.StoredResponse{
		Status:           201,
		Body:             []byte(`{"user":{"id":"018f90ef-08e5-4a68-b09f-000000000001"}}`),
		Location:         "/api/v1/users/me",
		PasswordVerifier: huellaSintetica,
	}
}

// cancellationAtTotal construye tantos motivos como acciones tiene la transaccion y
// marca como fallidos los indices indicados.
func cancellationAtTotal(total int, indices ...int) *types.TransactionCanceledException {
	reasons := make([]types.CancellationReason, total)
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

// Indices fijos del layout: 0 email, 1 perfil, 2 ingreso, 3 consentimiento,
// 4 onboarding, 5..(5+N-1) compromisos, ultimo idempotencia.
const (
	idxEmail      = 0
	idxProfile    = 1
	idxIncome     = 2
	idxConsent    = 3
	idxOnboarding = 4
	idxFirstRecur = 5
)

func idxIdem(total int) int { return total - 1 }

func totalItems(expenses int) int { return 6 + expenses }

func itemPK(t *testing.T, input *dynamodb.TransactWriteItemsInput, index int) string {
	t.Helper()
	if input == nil || index >= len(input.TransactItems) || input.TransactItems[index].Put == nil {
		t.Fatalf("no existe la accion %d de la transaccion", index)
	}
	value, ok := input.TransactItems[index].Put.Item["PK"]
	if !ok {
		t.Fatalf("la accion %d no tiene PK", index)
	}
	return value.(*types.AttributeValueMemberS).Value
}

func itemSK(t *testing.T, input *dynamodb.TransactWriteItemsInput, index int) string {
	t.Helper()
	if input == nil || index >= len(input.TransactItems) || input.TransactItems[index].Put == nil {
		t.Fatalf("no existe la accion %d de la transaccion", index)
	}
	value, ok := input.TransactItems[index].Put.Item["SK"]
	if !ok {
		t.Fatalf("la accion %d no tiene SK", index)
	}
	return value.(*types.AttributeValueMemberS).Value
}

func TestRegistrationStoreEscribeLasSeisAccionesFijas(t *testing.T) {
	fake := &fakeDynamo{}
	store := dynamoAdapter.NewRegistrationStore(fake, "NEXUM_CORE_TEST")

	replay, err := store.CreateOrReplay(
		context.Background(), folioIdem, appIdentity.RegisterOperation,
		"payload-hash", registrationRecord(), storedResponse(), time.Hour,
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
	if fake.transactInput == nil || len(fake.transactInput.TransactItems) != totalItems(0) {
		t.Fatalf("se esperaban %d acciones atomicas: %+v", totalItems(0), fake.transactInput)
	}

	if pk := itemPK(t, fake.transactInput, idxEmail); !strings.HasPrefix(pk, "EMAIL#") {
		t.Fatalf("PK de email inesperada: %q", pk)
	}
	if pk := itemPK(t, fake.transactInput, idxProfile); !strings.HasPrefix(pk, "USER#") {
		t.Fatalf("PK de perfil inesperada: %q", pk)
	}
	if sk := itemSK(t, fake.transactInput, idxProfile); sk != "PROFILE" {
		t.Fatalf("SK de perfil inesperada: %q", sk)
	}
	if sk := itemSK(t, fake.transactInput, idxIncome); sk != "INCOME" {
		t.Fatalf("SK de ingreso inesperada: %q", sk)
	}
	if sk := itemSK(t, fake.transactInput, idxConsent); sk != "CONSENT#2026-11" {
		t.Fatalf("SK de consentimiento inesperada: %q", sk)
	}
	if sk := itemSK(t, fake.transactInput, idxOnboarding); sk != "ONBOARDING" {
		t.Fatalf("SK de onboarding inesperada: %q", sk)
	}
	if pk := itemPK(t, fake.transactInput, idxIdem(totalItems(0))); !strings.HasPrefix(pk, "IDEM#") {
		t.Fatalf("PK de idempotencia inesperada: %q", pk)
	}
}

func TestRegistrationStoreContenidoDeIngresoYConsentimiento(t *testing.T) {
	fake := &fakeDynamo{}
	store := dynamoAdapter.NewRegistrationStore(fake, "NEXUM_CORE_TEST")

	if _, err := store.CreateOrReplay(
		context.Background(), folioIdem, appIdentity.RegisterOperation,
		"payload-hash", registrationRecord(), storedResponse(), time.Hour,
	); err != nil {
		t.Fatalf("transaccion fallo: %v", err)
	}

	income := fake.transactInput.TransactItems[idxIncome].Put.Item
	if got := income["amountCents"].(*types.AttributeValueMemberN).Value; got != "1200000" {
		t.Fatalf("amountCents inesperado: %q", got)
	}
	if got := income["frequency"].(*types.AttributeValueMemberS).Value; got != "monthly" {
		t.Fatalf("frequency inesperada: %q", got)
	}
	if got := income["monthlyEquivalentCents"].(*types.AttributeValueMemberN).Value; got != "1200000" {
		t.Fatalf("monthlyEquivalentCents inesperado: %q", got)
	}
	if got := income["currency"].(*types.AttributeValueMemberS).Value; got != "MXN" {
		t.Fatalf("currency inesperada: %q", got)
	}

	consent := fake.transactInput.TransactItems[idxConsent].Put.Item
	if got := consent["noticeVersion"].(*types.AttributeValueMemberS).Value; got != "2026-11" {
		t.Fatalf("noticeVersion inesperada: %q", got)
	}
	if !consent["acceptedByServer"].(*types.AttributeValueMemberBOOL).Value {
		t.Fatal("acceptedByServer debe persistirse como true")
	}
	if got := consent["acceptedAt"].(*types.AttributeValueMemberS).Value; got != "2026-11-10T12:00:00Z" {
		t.Fatalf("acceptedAt inesperado: %q", got)
	}

	onboarding := fake.transactInput.TransactItems[idxOnboarding].Put.Item
	if got := onboarding["microphone"].(*types.AttributeValueMemberS).Value; got != "pending" {
		t.Fatalf("microphone inesperado: %q", got)
	}
	if onboarding["configured"].(*types.AttributeValueMemberBOOL).Value {
		t.Fatal("configured debe persistirse como false")
	}

	profile := fake.transactInput.TransactItems[idxProfile].Put.Item
	if profile["emailVerified"].(*types.AttributeValueMemberBOOL).Value {
		t.Fatal("emailVerified persistido debe ser false")
	}
	if got := profile["displayName"].(*types.AttributeValueMemberS).Value; got != "Usuario Ejemplo" {
		t.Fatalf("displayName inesperado: %q", got)
	}
	if _, ok := profile["privacyNoticeVersion"]; !ok {
		t.Fatal("PROFILE conserva privacyNoticeVersion por compatibilidad")
	}
}

func TestRegistrationStorePersisteTodosLosCompromisos(t *testing.T) {
	fake := &fakeDynamo{}
	store := dynamoAdapter.NewRegistrationStore(fake, "NEXUM_CORE_TEST")

	expenses := []appIdentity.RegistrationExpense{expenseFixture(1), expenseFixture(2), expenseFixture(3)}
	if _, err := store.CreateOrReplay(
		context.Background(), folioIdem, appIdentity.RegisterOperation,
		"payload-hash", registrationRecord(expenses...), storedResponse(), time.Hour,
	); err != nil {
		t.Fatalf("transaccion fallo: %v", err)
	}

	if len(fake.transactInput.TransactItems) != totalItems(3) {
		t.Fatalf("se esperaban %d acciones, hay %d", totalItems(3), len(fake.transactInput.TransactItems))
	}
	for i, expense := range expenses {
		index := idxFirstRecur + i
		if sk := itemSK(t, fake.transactInput, index); sk != "RECURRING#"+expense.ID {
			t.Fatalf("SK de compromiso %d inesperada: %q", i, sk)
		}
		item := fake.transactInput.TransactItems[index].Put.Item
		if got := item["concept"].(*types.AttributeValueMemberS).Value; got != expense.Concept {
			t.Fatalf("concepto %d inesperado: %q", i, got)
		}
		if got := item["amountCents"].(*types.AttributeValueMemberN).Value; got != strconv.FormatInt(expense.AmountCents, 10) {
			t.Fatalf("importe %d inesperado: %q", i, got)
		}
		if got := item["expenseType"].(*types.AttributeValueMemberS).Value; got != "fixed" {
			t.Fatalf("tipo %d inesperado: %q", i, got)
		}
		if got := item["category"].(*types.AttributeValueMemberS).Value; got != "Servicios" {
			t.Fatalf("categoria %d inesperada: %q", i, got)
		}
	}

	// Solo se crean compromisos programados: nunca un MOVEMENT de gasto ejecutado.
	for _, item := range fake.transactInput.TransactItems {
		if item.Put == nil {
			continue
		}
		if sk, ok := item.Put.Item["SK"].(*types.AttributeValueMemberS); ok && strings.HasPrefix(sk.Value, "MOVEMENT#") {
			t.Fatalf("el registro inicial no debe crear movimientos: %q", sk.Value)
		}
	}
}

func TestRegistrationStoreRechazaExcesoDeCompromisos(t *testing.T) {
	fake := &fakeDynamo{}
	store := dynamoAdapter.NewRegistrationStore(fake, "NEXUM_CORE_TEST")

	expenses := make([]appIdentity.RegistrationExpense, 0, 95)
	for i := 0; i < 95; i++ {
		expenses = append(expenses, expenseFixture(i))
	}
	_, err := store.CreateOrReplay(
		context.Background(), folioIdem, appIdentity.RegisterOperation,
		"payload-hash", registrationRecord(expenses...), storedResponse(), time.Hour,
	)
	if !errors.Is(err, appIdentity.ErrTransactionTooLarge) {
		t.Fatalf("se esperaba ErrTransactionTooLarge, se obtuvo %v", err)
	}
	if fake.transactCalls != 0 {
		t.Fatalf("no debe emitirse ninguna transaccion: %d", fake.transactCalls)
	}
}

func TestRegistrationStoreAceptaElMaximoDeCompromisos(t *testing.T) {
	fake := &fakeDynamo{}
	store := dynamoAdapter.NewRegistrationStore(fake, "NEXUM_CORE_TEST")

	expenses := make([]appIdentity.RegistrationExpense, 0, 94)
	for i := 0; i < 94; i++ {
		expenses = append(expenses, expenseFixture(i))
	}
	if _, err := store.CreateOrReplay(
		context.Background(), folioIdem, appIdentity.RegisterOperation,
		"payload-hash", registrationRecord(expenses...), storedResponse(), time.Hour,
	); err != nil {
		t.Fatalf("94 compromisos deben caber: %v", err)
	}
	if len(fake.transactInput.TransactItems) != 100 {
		t.Fatalf("la transaccion debe tener exactamente 100 acciones, tiene %d", len(fake.transactInput.TransactItems))
	}
}

func TestRegistrationStoreIdempotenciaConVerificador(t *testing.T) {
	fake := &fakeDynamo{}
	store := dynamoAdapter.NewRegistrationStore(fake, "NEXUM_CORE_TEST")

	if _, err := store.CreateOrReplay(
		context.Background(), folioIdem, appIdentity.RegisterOperation,
		"payload-hash", registrationRecord(), storedResponse(), time.Hour,
	); err != nil {
		t.Fatalf("transaccion fallo: %v", err)
	}

	idemPut := fake.transactInput.TransactItems[idxIdem(totalItems(0))].Put
	if idemPut == nil {
		t.Fatal("falta accion de idempotencia")
	}
	if !strings.Contains(aws.ToString(idemPut.ConditionExpression), "expiresAt < :now") {
		t.Fatalf("condicion de idempotencia inesperada: %q", aws.ToString(idemPut.ConditionExpression))
	}
	if _, ok := idemPut.ExpressionAttributeValues[":now"]; !ok {
		t.Fatal("falta valor :now de la condicion de idempotencia")
	}
	if got := idemPut.Item["passwordVerifier"].(*types.AttributeValueMemberS).Value; got != huellaSintetica {
		t.Fatalf("passwordVerifier inesperado: %q", got)
	}
	idemPK := idemPut.Item["PK"].(*types.AttributeValueMemberS).Value
	if strings.Contains(idemPK, folioIdem) {
		t.Fatal("la Idempotency-Key cruda no debe persistirse en la PK")
	}
}

func TestRegistrationStoreReplayDevuelveVerificador(t *testing.T) {
	fake := &fakeDynamo{
		transactErrs: []error{cancellationAtTotal(totalItems(0), idxIdem(totalItems(0)))},
		getOutput: &dynamodb.GetItemOutput{Item: map[string]types.AttributeValue{
			"payloadHash":      strAttr("payload-hash"),
			"expiresAt":        numAttr(time.Now().Add(time.Hour).Unix()),
			"responseStatus":   numAttr(201),
			"responseBody":     strAttr(`{"user":{"id":"018f90ef-08e5-4a68-b09f-000000000001"}}`),
			"location":         strAttr("/api/v1/users/me"),
			"passwordVerifier": strAttr(huellaSintetica),
		}},
	}
	store := dynamoAdapter.NewRegistrationStore(fake, "NEXUM_CORE_TEST")

	result, err := store.CreateOrReplay(
		context.Background(), folioIdem, appIdentity.RegisterOperation,
		"payload-hash", registrationRecord(), storedResponse(), time.Hour,
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
	if result.PasswordVerifier != huellaSintetica {
		t.Fatalf("el replay debe devolver el verificador Argon2id: %q", result.PasswordVerifier)
	}
	if fake.getInput == nil || fake.getInput.ConsistentRead == nil || !*fake.getInput.ConsistentRead {
		t.Fatal("GetItem de idempotencia debe usar ConsistentRead=true")
	}
}

func TestRegistrationStoreConflictoDePayload(t *testing.T) {
	fake := &fakeDynamo{
		transactErrs: []error{cancellationAtTotal(totalItems(0), idxIdem(totalItems(0)))},
		getOutput: &dynamodb.GetItemOutput{Item: map[string]types.AttributeValue{
			"payloadHash":    strAttr("otro-payload"),
			"expiresAt":      numAttr(time.Now().Add(time.Hour).Unix()),
			"responseStatus": numAttr(201),
			"responseBody":   strAttr(`{"user":{"id":"otro"}}`),
		}},
	}
	store := dynamoAdapter.NewRegistrationStore(fake, "NEXUM_CORE_TEST")

	_, err := store.CreateOrReplay(
		context.Background(), folioIdem, appIdentity.RegisterOperation,
		"payload-hash", registrationRecord(), storedResponse(), time.Hour,
	)
	if !errors.Is(err, appIdentity.ErrIdempotencyConflict) {
		t.Fatalf("se esperaba ErrIdempotencyConflict, se obtuvo %v", err)
	}
}

func TestRegistrationStoreEmailDuplicado(t *testing.T) {
	fake := &fakeDynamo{transactErrs: []error{cancellationAtTotal(totalItems(0), idxEmail)}}
	store := dynamoAdapter.NewRegistrationStore(fake, "NEXUM_CORE_TEST")

	_, err := store.CreateOrReplay(
		context.Background(), folioIdem, appIdentity.RegisterOperation,
		"payload-hash", registrationRecord(), storedResponse(), time.Hour,
	)
	if !errors.Is(err, appIdentity.ErrEmailAlreadyRegistered) {
		t.Fatalf("se esperaba ErrEmailAlreadyRegistered, se obtuvo %v", err)
	}
}

func TestRegistrationStoreConflictoDePerfilEsInterno(t *testing.T) {
	for _, index := range []int{idxProfile, idxIncome, idxConsent, idxOnboarding} {
		t.Run(strconv.Itoa(index), func(t *testing.T) {
			fake := &fakeDynamo{transactErrs: []error{cancellationAtTotal(totalItems(0), index)}}
			store := dynamoAdapter.NewRegistrationStore(fake, "NEXUM_CORE_TEST")
			_, err := store.CreateOrReplay(
				context.Background(), folioIdem, appIdentity.RegisterOperation,
				"payload-hash", registrationRecord(), storedResponse(), time.Hour,
			)
			if !errors.Is(err, appIdentity.ErrRegistrationInternal) {
				t.Fatalf("se esperaba ErrRegistrationInternal, se obtuvo %v", err)
			}
		})
	}
}

// Atomicidad: si una sola fila de compromiso falla, la transaccion completa se cancela
// y no se escribe nada. No existe camino de escritura parcial.
func TestRegistrationStoreFalloDeUnaFilaCancelaTodo(t *testing.T) {
	expenses := []appIdentity.RegistrationExpense{expenseFixture(1), expenseFixture(2)}
	fake := &fakeDynamo{
		transactErrs: []error{cancellationAtTotal(totalItems(2), idxFirstRecur+1)},
	}
	store := dynamoAdapter.NewRegistrationStore(fake, "NEXUM_CORE_TEST")

	_, err := store.CreateOrReplay(
		context.Background(), folioIdem, appIdentity.RegisterOperation,
		"payload-hash", registrationRecord(expenses...), storedResponse(), time.Hour,
	)
	if !errors.Is(err, appIdentity.ErrRegistrationInternal) {
		t.Fatalf("se esperaba ErrRegistrationInternal, se obtuvo %v", err)
	}
	if fake.transactCalls != 1 {
		t.Fatalf("se esperaba una unica transaccion, hubo %d", fake.transactCalls)
	}
	if fake.getCalls != 0 {
		t.Fatalf("un fallo de fila no debe leerse como replay: %d lecturas", fake.getCalls)
	}
}

func TestRegistrationStoreNoDisponible(t *testing.T) {
	casos := []struct {
		name string
		err  error
	}{
		{"tabla inexistente", &types.ResourceNotFoundException{}},
		{"conflicto de transaccion", &types.TransactionCanceledException{CancellationReasons: []types.CancellationReason{
			{Code: aws.String("TransactionConflict")},
			{Code: aws.String("TransactionConflict")},
			{Code: aws.String("TransactionConflict")},
			{Code: aws.String("TransactionConflict")},
			{Code: aws.String("TransactionConflict")},
			{Code: aws.String("TransactionConflict")},
		}}},
		{"throttling", errors.New("throttled")},
	}
	for _, caso := range casos {
		t.Run(caso.name, func(t *testing.T) {
			fake := &fakeDynamo{transactErrs: []error{caso.err}}
			store := dynamoAdapter.NewRegistrationStore(fake, "NEXUM_CORE_TEST")
			_, err := store.CreateOrReplay(
				context.Background(), folioIdem, appIdentity.RegisterOperation,
				"payload-hash", registrationRecord(), storedResponse(), time.Hour,
			)
			if !errors.Is(err, appIdentity.ErrStoreUnavailable) {
				t.Fatalf("se esperaba ErrStoreUnavailable, se obtuvo %v", err)
			}
		})
	}
}

func TestRegistrationStoreIdempotenciaExpiradaReintentaUnaVez(t *testing.T) {
	fake := &fakeDynamo{
		transactErrs: []error{cancellationAtTotal(totalItems(0), idxIdem(totalItems(0))), nil},
		getOutput: &dynamodb.GetItemOutput{Item: map[string]types.AttributeValue{
			"payloadHash":    strAttr("payload-hash"),
			"expiresAt":      numAttr(time.Now().Add(-time.Hour).Unix()),
			"responseStatus": numAttr(201),
			"responseBody":   strAttr(`{"user":{"id":"viejo"}}`),
		}},
	}
	store := dynamoAdapter.NewRegistrationStore(fake, "NEXUM_CORE_TEST")

	result, err := store.CreateOrReplay(
		context.Background(), folioIdem, appIdentity.RegisterOperation,
		"payload-hash", registrationRecord(), storedResponse(), time.Hour,
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

func TestRegistrationStoreGetItemFallaEsNoDisponible(t *testing.T) {
	fake := &fakeDynamo{
		transactErrs: []error{cancellationAtTotal(totalItems(0), idxIdem(totalItems(0)))},
		getErr:       errors.New("red caida"),
	}
	store := dynamoAdapter.NewRegistrationStore(fake, "NEXUM_CORE_TEST")

	_, err := store.CreateOrReplay(
		context.Background(), folioIdem, appIdentity.RegisterOperation,
		"payload-hash", registrationRecord(), storedResponse(), time.Hour,
	)
	if !errors.Is(err, appIdentity.ErrStoreUnavailable) {
		t.Fatalf("se esperaba ErrStoreUnavailable, se obtuvo %v", err)
	}
}

func TestRegistrationStoreSinClienteOTablaFallaCerrado(t *testing.T) {
	casos := []*dynamoAdapter.RegistrationStore{
		dynamoAdapter.NewRegistrationStore(nil, "NEXUM_CORE_TEST"),
		dynamoAdapter.NewRegistrationStore(&fakeDynamo{}, ""),
	}
	for _, store := range casos {
		_, err := store.CreateOrReplay(
			context.Background(), folioIdem, appIdentity.RegisterOperation,
			"payload-hash", registrationRecord(), storedResponse(), time.Hour,
		)
		if !errors.Is(err, appIdentity.ErrStoreUnavailable) {
			t.Fatalf("se esperaba ErrStoreUnavailable, se obtuvo %v", err)
		}
	}
}

// Las claves usadas deben caber en la politica IAM vigente (USER#, EMAIL#, IDEM#).
func TestRegistrationStoreUsaParticionesAutorizadas(t *testing.T) {
	fake := &fakeDynamo{}
	store := dynamoAdapter.NewRegistrationStore(fake, "NEXUM_CORE_TEST")

	expenses := []appIdentity.RegistrationExpense{expenseFixture(1)}
	if _, err := store.CreateOrReplay(
		context.Background(), folioIdem, appIdentity.RegisterOperation,
		"payload-hash", registrationRecord(expenses...), storedResponse(), time.Hour,
	); err != nil {
		t.Fatalf("transaccion fallo: %v", err)
	}

	permitidos := []string{"USER#", "EMAIL#", "IDEM#"}
	for i := range fake.transactInput.TransactItems {
		pk := itemPK(t, fake.transactInput, i)
		ok := false
		for _, prefijo := range permitidos {
			if strings.HasPrefix(pk, prefijo) {
				ok = true
				break
			}
		}
		if !ok {
			t.Fatalf("particion no autorizada por IAM: %q", pk)
		}
	}
}
