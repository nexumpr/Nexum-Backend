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

// fakeRefreshDynamo captura las transacciones reales que construiría E003.
type fakeRefreshDynamo struct {
	transactInputs []*dynamodb.TransactWriteItemsInput
	transactErrs   []error
	getInputs      []*dynamodb.GetItemInput
	getOutputs     []*dynamodb.GetItemOutput
	getErr         error
}

func (f *fakeRefreshDynamo) TransactWriteItems(_ context.Context, params *dynamodb.TransactWriteItemsInput, _ ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	f.transactInputs = append(f.transactInputs, params)
	if len(f.transactErrs) > 0 {
		err := f.transactErrs[0]
		f.transactErrs = f.transactErrs[1:]
		if err != nil {
			return nil, err
		}
	}
	return &dynamodb.TransactWriteItemsOutput{}, nil
}

func (f *fakeRefreshDynamo) GetItem(_ context.Context, params *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	f.getInputs = append(f.getInputs, params)
	if f.getErr != nil {
		return nil, f.getErr
	}
	if len(f.getOutputs) == 0 {
		return &dynamodb.GetItemOutput{}, nil
	}
	salida := f.getOutputs[0]
	f.getOutputs = f.getOutputs[1:]
	return salida, nil
}

const (
	refreshTable    = "NEXUM_CORE_H08I"
	refreshUser     = "47622cbc-36ec-4b30-9b8a-d9c26833a395"
	refreshSesion   = "018f90ef-08e5-7a68-b09f-0000000000a1"
	refreshFamilia  = "018f90ef-08e5-7a68-b09f-0000000000b2"
	refreshDigest   = "1111111111111111111111111111111111111111111111111111111111111111"
	refreshNuevo    = "2222222222222222222222222222222222222222222222222222222222222222"
	refreshOtraUser = "otro-usuario"
)

var refreshAhora = time.Date(2026, 11, 12, 21, 0, 0, 0, time.UTC)

func rotacionFixture() appIdentity.RefreshRotation {
	return appIdentity.RefreshRotation{
		UserID:          refreshUser,
		SessionID:       refreshSesion,
		FamilyID:        refreshFamilia,
		OldDigest:       refreshDigest,
		NewDigest:       refreshNuevo,
		NewAccessJTI:    "jti-nuevo",
		ExpectedVersion: 4,
		NewVersion:      5,
		ExpiresAt:       refreshAhora.Add(20 * 24 * time.Hour),
		Now:             refreshAhora,
	}
}

func TestRotateRefreshConstruyeTransaccionCASDeCuatroAcciones(t *testing.T) {
	fake := &fakeRefreshDynamo{}
	store := dynamoAdapter.NewRefreshStore(fake, refreshTable)

	if err := store.RotateRefresh(context.Background(), rotacionFixture()); err != nil {
		t.Fatalf("rotacion fallo: %v", err)
	}
	if len(fake.transactInputs) != 1 {
		t.Fatalf("se esperaba una transaccion, hay %d", len(fake.transactInputs))
	}
	items := fake.transactInputs[0].TransactItems
	if len(items) != 4 {
		t.Fatalf("la rotacion debe ser una transaccion de 4 acciones, hay %d", len(items))
	}

	// 1. Consumo del localizador presentado: un solo uso, compatible E002 y E003.
	if items[0].Update == nil {
		t.Fatal("la primera accion debe consumir el localizador presentado")
	}
	consumo := items[0].Update
	if got := pkOf(t, consumo.Key); got != "REFRESH#"+refreshDigest {
		t.Fatalf("PK del localizador consumido inesperada: %s", got)
	}
	if got := skOf(t, consumo.Key); got != "SESSION" {
		t.Fatalf("SK del localizador consumido inesperada: %s", got)
	}
	condicion := aws.ToString(consumo.ConditionExpression)
	// Compatibilidad: E002 no escribe `used`; E003 lo escribe como false. La condicion
	// debe aceptar ambos y rechazar solo `used=true`.
	if !strings.Contains(condicion, "attribute_not_exists(#used)") || !strings.Contains(condicion, "#used = :false") {
		t.Fatalf("la condicion no admite localizadores de E002 y de E003: %s", condicion)
	}
	if !strings.Contains(condicion, "#status = :active") {
		t.Fatalf("la condicion debe exigir el localizador ACTIVE: %s", condicion)
	}
	if !strings.Contains(aws.ToString(consumo.UpdateExpression), "#used = :used") {
		t.Fatalf("el consumo debe marcar used: %s", aws.ToString(consumo.UpdateExpression))
	}
	if valor := stringAttrValue(t, consumo.ExpressionAttributeValues, ":used"); valor != "true" {
		t.Fatalf(":used debe ser true, es %q", valor)
	}
	if valor := stringAttrValue(t, consumo.ExpressionAttributeValues, ":false"); valor != "false" {
		t.Fatalf(":false debe ser BOOL false, es %q", valor)
	}
	if _, ok := consumo.ExpressionAttributeValues[":false"].(*types.AttributeValueMemberBOOL); !ok {
		t.Fatal(":false debe declararse como BOOL y no como texto")
	}

	// 2. Sesión autoritativa: CAS por versión y digest anterior.
	if items[1].Update == nil {
		t.Fatal("la segunda accion debe actualizar la sesion")
	}
	sesion := items[1].Update
	if got := pkOf(t, sesion.Key); got != "USER#"+refreshUser {
		t.Fatalf("PK de sesion inesperada: %s", got)
	}
	if got := skOf(t, sesion.Key); got != "SESSION#"+refreshSesion {
		t.Fatalf("SK de sesion inesperada: %s", got)
	}
	condSesion := aws.ToString(sesion.ConditionExpression)
	for _, fragmento := range []string{"#version = :expected", "refreshHash = :oldHash", "#status = :active"} {
		if !strings.Contains(condSesion, fragmento) {
			t.Fatalf("la condicion de sesion no incluye %q: %s", fragmento, condSesion)
		}
	}
	if valor := stringAttrValue(t, sesion.ExpressionAttributeValues, ":expected"); valor != "4" {
		t.Fatalf(":expected debe ser la version leida (4), es %q", valor)
	}
	if valor := stringAttrValue(t, sesion.ExpressionAttributeValues, ":newVersion"); valor != "5" {
		t.Fatalf(":newVersion debe ser 5, es %q", valor)
	}
	if !strings.Contains(aws.ToString(sesion.UpdateExpression), "refreshHash = :newHash") {
		t.Fatal("la sesion debe apuntar al digest nuevo")
	}

	// 3. La cuenta propietaria debe seguir ACTIVE dentro de la misma transacción.
	if items[2].ConditionCheck == nil {
		t.Fatal("la tercera accion debe comprobar el perfil ACTIVE")
	}
	cuenta := items[2].ConditionCheck
	if got := pkOf(t, cuenta.Key); got != "USER#"+refreshUser {
		t.Fatalf("PK de la comprobacion de cuenta inesperada: %s", got)
	}
	if got := skOf(t, cuenta.Key); got != "PROFILE" {
		t.Fatalf("SK de la comprobacion de cuenta inesperada: %s", got)
	}
	condCuenta := aws.ToString(cuenta.ConditionExpression)
	if !strings.Contains(condCuenta, "attribute_exists(PK)") || !strings.Contains(condCuenta, "#status = :active") {
		t.Fatalf("la cuenta debe existir y estar ACTIVE: %s", condCuenta)
	}

	// 4. Localizador nuevo con TTL heredado de la sesión (no se extiende la familia).
	if items[3].Put == nil {
		t.Fatal("la cuarta accion debe crear el localizador nuevo")
	}
	nuevo := items[3].Put
	if aws.ToString(nuevo.ConditionExpression) != "attribute_not_exists(PK)" {
		t.Fatalf("el localizador nuevo exige ineditos: %s", aws.ToString(nuevo.ConditionExpression))
	}
	if got := stringAttrValueMap(nuevo.Item, "PK"); got != "REFRESH#"+refreshNuevo {
		t.Fatalf("PK del localizador nuevo inesperada: %s", got)
	}
	ttl := stringAttrValueMap(nuevo.Item, "expiresAt")
	if ttl != strconv.FormatInt(refreshAhora.Add(20*24*time.Hour).Unix(), 10) {
		t.Fatalf("expiresAt debe ser Number con la expiracion absoluta: %s", ttl)
	}
	if usado := stringAttrValueMap(nuevo.Item, "used"); usado != "false" {
		t.Fatalf("el localizador nuevo debe nacer sin usar: %s", usado)
	}
	if estado := stringAttrValueMap(nuevo.Item, "status"); estado != "ACTIVE" {
		t.Fatalf("el localizador nuevo debe nacer ACTIVE: %s", estado)
	}
}

func TestRotateRefreshFalloDeCondicionEsConflicto(t *testing.T) {
	fake := &fakeRefreshDynamo{transactErrs: []error{transaccionCancelada("ConditionalCheckFailed")}}
	store := dynamoAdapter.NewRefreshStore(fake, refreshTable)

	err := store.RotateRefresh(context.Background(), rotacionFixture())
	if !errors.Is(err, appIdentity.ErrRefreshConflict) {
		t.Fatalf("se esperaba ErrRefreshConflict, hay %v", err)
	}
}

func TestRotateRefreshFalloDeServicioNoEsConflicto(t *testing.T) {
	fake := &fakeRefreshDynamo{transactErrs: []error{transaccionCancelada("TransactionConflict")}}
	store := dynamoAdapter.NewRefreshStore(fake, refreshTable)

	err := store.RotateRefresh(context.Background(), rotacionFixture())
	if !errors.Is(err, appIdentity.ErrRefreshUnavailable) {
		t.Fatalf("un conflicto de transaccion no es un CAS perdido: %v", err)
	}

	fakeErr := &fakeRefreshDynamo{transactErrs: []error{errors.New("network error")}}
	store = dynamoAdapter.NewRefreshStore(fakeErr, refreshTable)
	if err := store.RotateRefresh(context.Background(), rotacionFixture()); !errors.Is(err, appIdentity.ErrRefreshUnavailable) {
		t.Fatalf("un fallo de red debe ser 503, hay %v", err)
	}
}

func TestRotateRefreshRechazaDigestsInvalidos(t *testing.T) {
	fake := &fakeRefreshDynamo{}
	store := dynamoAdapter.NewRefreshStore(fake, refreshTable)

	rota := rotacionFixture()
	rota.NewDigest = rota.OldDigest
	if err := store.RotateRefresh(context.Background(), rota); !errors.Is(err, appIdentity.ErrRefreshUnavailable) {
		t.Fatalf("un digest repetido debe fallar cerrado, hay %v", err)
	}
	if len(fake.transactInputs) != 0 {
		t.Fatal("no debe emitirse transaccion con digests invalidos")
	}
}

func TestLocateRefreshLeeConConsistenciaFuerte(t *testing.T) {
	item := map[string]types.AttributeValue{
		"PK":        strAttr("REFRESH#" + refreshDigest),
		"SK":        strAttr("SESSION"),
		"userId":    strAttr(refreshUser),
		"sessionId": strAttr(refreshSesion),
		"familyId":  strAttr(refreshFamilia),
		"status":    strAttr("ACTIVE"),
		"used":      &types.AttributeValueMemberBOOL{Value: true},
		"expiresAt": numAttr(refreshAhora.Add(time.Hour).Unix()),
	}
	fake := &fakeRefreshDynamo{getOutputs: []*dynamodb.GetItemOutput{{Item: item}}}
	store := dynamoAdapter.NewRefreshStore(fake, refreshTable)

	locator, err := store.LocateRefresh(context.Background(), refreshDigest)
	if err != nil || locator == nil {
		t.Fatalf("localizador no resuelto: %v", err)
	}
	if !aws.ToBool(fake.getInputs[0].ConsistentRead) {
		t.Fatal("la lectura del localizador debe ser consistente")
	}
	if !locator.Used || locator.Status != "ACTIVE" || locator.UserID != refreshUser || locator.SessionID != refreshSesion {
		t.Fatalf("localizador mal interpretado: %+v", locator)
	}
	if !locator.HasExpiry || !locator.ExpiresAt.Equal(refreshAhora.Add(time.Hour)) {
		t.Fatalf("expiracion mal interpretada: %+v", locator)
	}
}

func TestLocateRefreshInexistenteDevuelveNil(t *testing.T) {
	fake := &fakeRefreshDynamo{getOutputs: []*dynamodb.GetItemOutput{{}}}
	store := dynamoAdapter.NewRefreshStore(fake, refreshTable)

	locator, err := store.LocateRefresh(context.Background(), refreshDigest)
	if err != nil || locator != nil {
		t.Fatalf("un localizador inexistente debe ser nil, nil: %v %+v", err, locator)
	}

	incompleto := map[string]types.AttributeValue{
		"PK": strAttr("REFRESH#" + refreshDigest),
		"SK": strAttr("SESSION"),
	}
	incoherente := &fakeRefreshDynamo{getOutputs: []*dynamodb.GetItemOutput{{Item: incompleto}}}
	if locator, err := dynamoAdapter.NewRefreshStore(incoherente, refreshTable).
		LocateRefresh(context.Background(), refreshDigest); err != nil || locator != nil {
		t.Fatalf("un localizador sin identidad no puede autenticar: %v %+v", err, locator)
	}
}

func TestLocateRefreshErrorEsIndisponibilidad(t *testing.T) {
	fake := &fakeRefreshDynamo{getErr: errors.New("throttled")}
	store := dynamoAdapter.NewRefreshStore(fake, refreshTable)

	if _, err := store.LocateRefresh(context.Background(), refreshDigest); !errors.Is(err, appIdentity.ErrRefreshUnavailable) {
		t.Fatalf("se esperaba ErrRefreshUnavailable, hay %v", err)
	}
}

func TestLoadSessionInterpretaVersionYExpiracion(t *testing.T) {
	item := map[string]types.AttributeValue{
		"PK":          strAttr("USER#" + refreshUser),
		"SK":          strAttr("SESSION#" + refreshSesion),
		"entityType":  strAttr("USER_SESSION"),
		"userId":      strAttr(refreshUser),
		"sessionId":   strAttr(refreshSesion),
		"familyId":    strAttr(refreshFamilia),
		"refreshHash": strAttr(refreshDigest),
		"accessJti":   strAttr("jti"),
		"status":      strAttr("ACTIVE"),
		"deviceId":    strAttr("installation-6a4f1d8c"),
		"platform":    strAttr("android"),
		"createdAt":   strAttr(refreshAhora.Add(-2 * time.Hour).Format(time.RFC3339)),
		"lastUsedAt":  strAttr(refreshAhora.Add(-time.Hour).Format(time.RFC3339)),
		"expiresAt":   numAttr(refreshAhora.Add(24 * time.Hour).Unix()),
		"version":     numAttr(7),
	}
	fake := &fakeRefreshDynamo{getOutputs: []*dynamodb.GetItemOutput{{Item: item}}}
	store := dynamoAdapter.NewRefreshStore(fake, refreshTable)

	session, err := store.LoadSession(context.Background(), refreshUser, refreshSesion)
	if err != nil || session == nil {
		t.Fatalf("sesion no resuelta: %v", err)
	}
	if !aws.ToBool(fake.getInputs[0].ConsistentRead) {
		t.Fatal("la sesion autoritativa debe leerse con consistencia fuerte")
	}
	if session.Version != 7 {
		t.Fatalf("version mal interpretada: %d", session.Version)
	}
	if session.RefreshHash != refreshDigest || session.DeviceID != "installation-6a4f1d8c" {
		t.Fatalf("sesion mal interpretada: %+v", session)
	}
	if !session.HasExpiry || !session.ExpiresAt.Equal(refreshAhora.Add(24*time.Hour)) {
		t.Fatalf("expiracion mal interpretada: %+v", session)
	}
	if !session.CreatedAt.Equal(refreshAhora.Add(-2*time.Hour)) || !session.LastUsedAt.Equal(refreshAhora.Add(-time.Hour)) {
		t.Fatalf("marcas temporales mal interpretadas: %+v", session)
	}
}

func TestLoadSessionRechazaOtraEntidad(t *testing.T) {
	item := map[string]types.AttributeValue{
		"PK":         strAttr("USER#" + refreshUser),
		"SK":         strAttr("SESSION#" + refreshSesion),
		"entityType": strAttr("REFRESH_LOOKUP"),
		"userId":     strAttr(refreshOtraUser),
		"sessionId":  strAttr(refreshSesion),
	}
	fake := &fakeRefreshDynamo{getOutputs: []*dynamodb.GetItemOutput{{Item: item}}}
	store := dynamoAdapter.NewRefreshStore(fake, refreshTable)

	if session, err := store.LoadSession(context.Background(), refreshUser, refreshSesion); err != nil || session != nil {
		t.Fatalf("otra entidad no puede actuar como sesion: %v %+v", err, session)
	}
}

func TestLoadSessionSinIdentidadNoAutentica(t *testing.T) {
	store := dynamoAdapter.NewRefreshStore(&fakeRefreshDynamo{}, refreshTable)
	if session, err := store.LoadSession(context.Background(), "", ""); err != nil || session != nil {
		t.Fatalf("sin identidad no se consulta ni autentica: %v %+v", err, session)
	}
}

func TestRevokeFamilyRevocaSesionYLocalizadores(t *testing.T) {
	fake := &fakeRefreshDynamo{}
	store := dynamoAdapter.NewRefreshStore(fake, refreshTable)

	err := store.RevokeFamily(context.Background(), appIdentity.RefreshRevocation{
		UserID:          refreshUser,
		SessionID:       refreshSesion,
		FamilyID:        refreshFamilia,
		PresentedDigest: refreshDigest,
		CurrentDigest:   refreshNuevo,
		Now:             refreshAhora,
	})
	if err != nil {
		t.Fatalf("revocacion fallo: %v", err)
	}
	items := fake.transactInputs[0].TransactItems
	if len(items) != 3 {
		t.Fatalf("se esperaban sesion + 2 localizadores, hay %d", len(items))
	}
	if got := pkOf(t, items[0].Update.Key); got != "USER#"+refreshUser {
		t.Fatalf("la sesion debe revocarse primero: %s", got)
	}
	if valor := stringAttrValue(t, items[0].Update.ExpressionAttributeValues, ":revoked"); valor != "REVOKED" {
		t.Fatalf("estado de revocacion inesperado: %q", valor)
	}
	if valor := stringAttrValue(t, items[0].Update.ExpressionAttributeValues, ":reason"); valor != appIdentity.RevokeReasonReplay {
		t.Fatalf("motivo de revocacion inesperado: %q", valor)
	}
	digests := []string{
		pkOf(t, items[1].Update.Key),
		pkOf(t, items[2].Update.Key),
	}
	if digests[0] != "REFRESH#"+refreshDigest || digests[1] != "REFRESH#"+refreshNuevo {
		t.Fatalf("localizadores revocados inesperados: %v", digests)
	}
}

func TestRevokeFamilyReintentaSoloLaSesion(t *testing.T) {
	fake := &fakeRefreshDynamo{transactErrs: []error{transaccionCancelada("ConditionalCheckFailed")}}
	store := dynamoAdapter.NewRefreshStore(fake, refreshTable)

	err := store.RevokeFamily(context.Background(), appIdentity.RefreshRevocation{
		UserID:          refreshUser,
		SessionID:       refreshSesion,
		PresentedDigest: refreshDigest,
		Now:             refreshAhora,
	})
	if err != nil {
		t.Fatalf("la revocacion debe reintentarse solo con la sesion: %v", err)
	}
	if len(fake.transactInputs) != 2 {
		t.Fatalf("se esperaban dos intentos, hay %d", len(fake.transactInputs))
	}
	if len(fake.transactInputs[1].TransactItems) != 1 {
		t.Fatalf("el reintento debe llevar solo la sesion: %d", len(fake.transactInputs[1].TransactItems))
	}
	if got := pkOf(t, fake.transactInputs[1].TransactItems[0].Update.Key); got != "USER#"+refreshUser {
		t.Fatalf("el reintento no revoca la sesion: %s", got)
	}
}

func TestRevokeFamilyFallaCerradoSiNoConfirma(t *testing.T) {
	fake := &fakeRefreshDynamo{transactErrs: []error{
		transaccionCancelada("ConditionalCheckFailed"),
		transaccionCancelada("ConditionalCheckFailed"),
	}}
	store := dynamoAdapter.NewRefreshStore(fake, refreshTable)

	err := store.RevokeFamily(context.Background(), appIdentity.RefreshRevocation{
		UserID:    refreshUser,
		SessionID: refreshSesion,
		Now:       refreshAhora,
	})
	if !errors.Is(err, appIdentity.ErrRefreshUnavailable) {
		t.Fatalf("una revocacion no confirmada debe ser 503, hay %v", err)
	}
}

func TestRefreshStoreSinClienteFallaCerrado(t *testing.T) {
	store := dynamoAdapter.NewRefreshStore(nil, "")
	ctx := context.Background()

	if _, err := store.LocateRefresh(ctx, refreshDigest); !errors.Is(err, appIdentity.ErrRefreshUnavailable) {
		t.Fatalf("localizador sin cliente: %v", err)
	}
	if _, err := store.LoadSession(ctx, refreshUser, refreshSesion); !errors.Is(err, appIdentity.ErrRefreshUnavailable) {
		t.Fatalf("sesion sin cliente: %v", err)
	}
	if err := store.RotateRefresh(ctx, rotacionFixture()); !errors.Is(err, appIdentity.ErrRefreshUnavailable) {
		t.Fatalf("rotacion sin cliente: %v", err)
	}
	if err := store.RevokeFamily(ctx, appIdentity.RefreshRevocation{UserID: refreshUser, SessionID: refreshSesion}); !errors.Is(err, appIdentity.ErrRefreshUnavailable) {
		t.Fatalf("revocacion sin cliente: %v", err)
	}
}

func transaccionCancelada(codigo string) error {
	return &types.TransactionCanceledException{
		Message: aws.String("Transaction cancelled"),
		CancellationReasons: []types.CancellationReason{
			{Code: aws.String(codigo)},
		},
	}
}

func pkOf(t *testing.T, key map[string]types.AttributeValue) string {
	t.Helper()
	return stringAttrValueMap(key, "PK")
}

func skOf(t *testing.T, key map[string]types.AttributeValue) string {
	t.Helper()
	return stringAttrValueMap(key, "SK")
}

func stringAttrValueMap(item map[string]types.AttributeValue, nombre string) string {
	valor, ok := item[nombre]
	if !ok {
		return ""
	}
	if numero, ok := valor.(*types.AttributeValueMemberN); ok {
		return numero.Value
	}
	if booleano, ok := valor.(*types.AttributeValueMemberBOOL); ok {
		return strconv.FormatBool(booleano.Value)
	}
	texto, ok := valor.(*types.AttributeValueMemberS)
	if !ok {
		return ""
	}
	return texto.Value
}

func stringAttrValue(t *testing.T, values map[string]types.AttributeValue, nombre string) string {
	t.Helper()
	return stringAttrValueMap(values, nombre)
}
