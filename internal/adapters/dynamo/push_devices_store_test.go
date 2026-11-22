package dynamo

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	appPush "github.com/nexumpr/Nexum-Backend/internal/application/pushdevices"
)

var e059AdaptadorAhora = time.Date(2026, 11, 22, 12, 30, 0, 0, time.UTC)

type e059FakeClient struct {
	getSalida     *dynamodb.GetItemOutput
	getErr        error
	querySalida   *dynamodb.QueryOutput
	queryErr      error
	updateErr     error
	transactErr   error
	ultimoGet     *dynamodb.GetItemInput
	ultimoQuery   *dynamodb.QueryInput
	ultimoUpdate  *dynamodb.UpdateItemInput
	ultimaTransac *dynamodb.TransactWriteItemsInput
}

func (f *e059FakeClient) GetItem(_ context.Context, params *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	f.ultimoGet = params
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.getSalida, nil
}

func (f *e059FakeClient) Query(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	f.ultimoQuery = params
	if f.queryErr != nil {
		return nil, f.queryErr
	}
	return f.querySalida, nil
}

func (f *e059FakeClient) UpdateItem(_ context.Context, params *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	f.ultimoUpdate = params
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	return &dynamodb.UpdateItemOutput{}, nil
}

func (f *e059FakeClient) TransactWriteItems(_ context.Context, params *dynamodb.TransactWriteItemsInput, _ ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	f.ultimaTransac = params
	if f.transactErr != nil {
		return nil, f.transactErr
	}
	return &dynamodb.TransactWriteItemsOutput{}, nil
}

func itemDispositivoE059(id, proveedor, etiqueta, huella string, activo bool) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"PK":               stringAttr("USER#user-a"),
		"SK":               stringAttr(pushDeviceSKPrefix + id),
		"entityType":       stringAttr(pushDeviceEntityType),
		"provider":         stringAttr(proveedor),
		"deviceLabel":      stringAttr(etiqueta),
		"registeredAt":     stringAttr(e059AdaptadorAhora.Format(time.RFC3339Nano)),
		"tokenCipher":      stringAttr("cifrado-base64"),
		"tokenFingerprint": stringAttr(huella),
		"active":           boolAttr(activo),
		"version":          numberAttr(1),
	}
}

func escrituraE059(huellaAnterior string) appPush.RegisterWrite {
	return appPush.RegisterWrite{
		UserID:         "user-a",
		IdempotencyKey: "clave-idempotencia-0001",
		PayloadHash:    strings.Repeat("a", 64),
		Device: appPush.Device{
			ID:               "018f90ef-08e5-7a68-b09f-2b67a47c92f0",
			Provider:         appPush.ProviderFCM,
			DeviceLabel:      "Android prueba",
			RegisteredAt:     e059AdaptadorAhora,
			Active:           true,
			TokenCipher:      []byte("sobre-aes-gcm-sintetico"),
			TokenFingerprint: strings.Repeat("b", 64),
		},
		PreviousFingerprint: huellaAnterior,
		ResponseJSON:        []byte(`{"id":"018f90ef-08e5-7a68-b09f-2b67a47c92f0","provider":"fcm","deviceLabel":"Android prueba","registeredAt":"2026-11-22T12:30:00Z"}`),
		Now:                 e059AdaptadorAhora,
		TTL:                 24 * time.Hour,
	}
}

func TestE059AdaptadorTransaccionCondicional(t *testing.T) {
	cliente := &e059FakeClient{}
	if err := NewPushDevicesStore(cliente, "nexum-table").Register(context.Background(), escrituraE059("")); err != nil {
		t.Fatalf("registro: %v", err)
	}
	if cliente.ultimaTransac == nil || len(cliente.ultimaTransac.TransactItems) != 2 {
		t.Fatalf(" TransactWriteItems no construido: %+v", cliente.ultimaTransac)
	}
	dispositivo := cliente.ultimaTransac.TransactItems[0].Put
	idem := cliente.ultimaTransac.TransactItems[1].Put
	if dispositivo == nil || idem == nil {
		t.Fatal("la transacción debe tener dos Put")
	}
	if got := *dispositivo.ConditionExpression; got != "attribute_not_exists(PK)" {
		t.Fatalf("condición de dispositivo incorrecta: %s", got)
	}
	if got := stringAttribute(dispositivo.Item["PK"]); got != "USER#user-a" {
		t.Fatalf("PK incorrecto: %q", got)
	}
	if got := stringAttribute(dispositivo.Item["SK"]); got != "DEVICE#018f90ef-08e5-7a68-b09f-2b67a47c92f0" {
		t.Fatalf("SK incorrecto: %q", got)
	}
	if got := stringAttribute(dispositivo.Item["entityType"]); got != pushDeviceEntityType {
		t.Fatalf("entityType incorrecto: %q", got)
	}
	if got := stringAttribute(dispositivo.Item["registeredAt"]); got != e059AdaptadorAhora.Format(time.RFC3339Nano) {
		t.Fatalf("registeredAt debe ser RFC3339Nano: %q", got)
	}
	if !boolAttribute(dispositivo.Item["active"]) {
		t.Fatal("el dispositivo debe quedar activo")
	}
	if strings.Contains(stringAttribute(dispositivo.Item["tokenCipher"]), "sobre-aes-gcm-sintetico") {
		t.Fatal("el token cifrado no debe guardarse en claro")
	}
	if decode, err := base64.StdEncoding.DecodeString(stringAttribute(dispositivo.Item["tokenCipher"])); err != nil ||
		string(decode) != "sobre-aes-gcm-sintetico" {
		t.Fatalf("tokenCipher no es base64 del sobre: %v", err)
	}

	suma := sha256.Sum256([]byte("clave-idempotencia-0001"))
	esperadaSK := "IDEM#PUSH#" + hex.EncodeToString(suma[:])
	if got := stringAttribute(idem.Item["SK"]); got != esperadaSK {
		t.Fatalf("SK de idempotencia incorrecta: %q", got)
	}
	skIdem := stringAttribute(idem.Item["SK"])
	if strings.Contains(skIdem, "clave-idempotencia-0001") || skIdem == "" {
		t.Fatalf("la clave cruda no debe persistir: %q", skIdem)
	}
	if got := stringAttribute(idem.Item["payloadHash"]); got != strings.Repeat("a", 64) {
		t.Fatalf("payloadHash incorrecto: %q", got)
	}
	if got := stringAttribute(idem.Item["responseJSON"]); !strings.Contains(got, `"deviceLabel"`) || strings.Contains(got, "token") {
		t.Fatalf("responseJSON incorrecto: %q", got)
	}
	if expires, ok := attributeInt(idem.Item["expiresAt"]); !ok || expires != e059AdaptadorAhora.Add(24*time.Hour).Unix() {
		t.Fatalf("expiresAt incorrecto: %d %v", expires, ok)
	}
	if got := *idem.ConditionExpression; !strings.Contains(got, "attribute_not_exists(PK)") || !strings.Contains(got, "expiresAt") {
		t.Fatalf("condición de idempotencia incorrecta: %s", got)
	}
}

func TestE059AdaptadorRotacionExigeHuellaAnterior(t *testing.T) {
	cliente := &e059FakeClient{}
	anterior := strings.Repeat("c", 64)
	if err := NewPushDevicesStore(cliente, "nexum-table").Register(context.Background(), escrituraE059(anterior)); err != nil {
		t.Fatalf("rotación: %v", err)
	}
	put := cliente.ultimaTransac.TransactItems[0].Put
	condicion := *put.ConditionExpression
	if !strings.Contains(condicion, "attribute_exists(PK)") || !strings.Contains(condicion, "#tokenFingerprint") {
		t.Fatalf("la rotación no exige la huella anterior: %s", condicion)
	}
	if got := stringAttribute(put.ExpressionAttributeValues[":previous"]); got != anterior {
		t.Fatalf(":previous incorrecto: %q", got)
	}
}

func TestE059AdaptadorCancelacionConcurrente(t *testing.T) {
	razon := func() types.CancellationReason {
		return types.CancellationReason{Code: aws.String("ConditionalCheckFailed")}
	}
	casos := []struct {
		nombre  string
		razones []types.CancellationReason
		espera  error
	}{
		{"idempotencia", []types.CancellationReason{{}, razon()}, appPush.ErrIdempotencyCondition},
		{"dispositivo", []types.CancellationReason{razon(), {}}, appPush.ErrDeviceCondition},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			cliente := &e059FakeClient{transactErr: &types.TransactionCanceledException{CancellationReasons: caso.razones}}
			err := NewPushDevicesStore(cliente, "nexum-table").Register(context.Background(), escrituraE059(""))
			if !errors.Is(err, caso.espera) {
				t.Fatalf("esperado %v, obtenido %v", caso.espera, err)
			}
		})
	}
}

func TestE059AdaptadorQueryPorEtiquetaSinScan(t *testing.T) {
	cliente := &e059FakeClient{querySalida: &dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{
		itemDispositivoE059("id-otro", "fcm", "Otro", strings.Repeat("0", 64), true),
		itemDispositivoE059("id-ina", "fcm", "Android prueba", strings.Repeat("1", 64), false),
		itemDispositivoE059("id-proveedor", "apns", "Android prueba", strings.Repeat("2", 64), true),
		itemDispositivoE059("id-valido", "fcm", "Android prueba", strings.Repeat("3", 64), true),
		{"entityType": stringAttr("OTRA_ENTIDAD"), "SK": stringAttr("DEVICE#id-ajeno")},
	}}}
	encontrado, err := NewPushDevicesStore(cliente, "nexum-table").FindActiveByLabel(context.Background(), "user-a", "fcm", "Android prueba")
	if err != nil || encontrado == nil || encontrado.ID != "id-valido" {
		t.Fatalf("filtro en memoria incorrecto: %+v %v", encontrado, err)
	}
	q := cliente.ultimoQuery
	if q == nil || *q.KeyConditionExpression != "PK = :pk AND begins_with(SK, :prefix)" {
		t.Fatalf("la consulta no es Query por partición: %+v", q)
	}
	if got := stringAttribute(q.ExpressionAttributeValues[":pk"]); got != "USER#user-a" {
		t.Fatalf("PK consultado incorrecto: %q", got)
	}
	if got := stringAttribute(q.ExpressionAttributeValues[":prefix"]); got != pushDeviceSKPrefix {
		t.Fatalf("prefijo consultado incorrecto: %q", got)
	}
}

func TestE060AdaptadorGetDevice(t *testing.T) {
	cliente := &e059FakeClient{getSalida: &dynamodb.GetItemOutput{Item: itemDispositivoE059(
		"018f90ef-08e5-7a68-b09f-2b67a47c92f0", "fcm", "iPhone", strings.Repeat("4", 64), true)}}
	store := NewPushDevicesStore(cliente, "nexum-table")
	dispositivo, err := store.GetDevice(context.Background(), "user-a", "018f90ef-08e5-7a68-b09f-2b67a47c92f0")
	if err != nil || dispositivo == nil || dispositivo.ID != "018f90ef-08e5-7a68-b09f-2b67a47c92f0" ||
		dispositivo.DeviceLabel != "iPhone" || !dispositivo.Active ||
		dispositivo.TokenFingerprint != strings.Repeat("4", 64) {
		t.Fatalf("proyección incorrecta: %+v %v", dispositivo, err)
	}
	if got := stringAttribute(cliente.ultimoGet.Key["SK"]); got != "DEVICE#018f90ef-08e5-7a68-b09f-2b67a47c92f0" {
		t.Fatalf("SK leído incorrecto: %q", got)
	}

	cliente.getSalida = &dynamodb.GetItemOutput{Item: map[string]types.AttributeValue{"entityType": stringAttr("OTRA_ENTIDAD")}}
	dispositivo, err = store.GetDevice(context.Background(), "user-a", "018f90ef-08e5-7a68-b09f-2b67a47c92f0")
	if err != nil || dispositivo != nil {
		t.Fatalf("entidad ajena no debe interpretarse: %+v %v", dispositivo, err)
	}
}

func TestE060AdaptadorBajaUpdateItem(t *testing.T) {
	cliente := &e059FakeClient{}
	actualizado, err := NewPushDevicesStore(cliente, "nexum-table").Revoke(
		context.Background(), "user-a", "018f90ef-08e5-7a68-b09f-2b67a47c92f0", e059AdaptadorAhora)
	if err != nil || !actualizado {
		t.Fatalf("baja: %v %v", actualizado, err)
	}
	u := cliente.ultimoUpdate
	if u == nil || *u.ConditionExpression != "#active = :active" {
		t.Fatalf("la baja no es condicional por active: %+v", u)
	}
	exp := *u.UpdateExpression
	if !strings.Contains(exp, "SET ") || !strings.Contains(exp, "#revokedAt = :revokedAt") ||
		!strings.Contains(exp, "REMOVE ") || !strings.Contains(exp, "#tokenCipher") || !strings.Contains(exp, "#tokenFingerprint") {
		t.Fatalf("expresión de baja incorrecta: %s", exp)
	}
	if got := stringAttribute(u.ExpressionAttributeValues[":revokedAt"]); got != e059AdaptadorAhora.Format(time.RFC3339Nano) {
		t.Fatalf("revokedAt incorrecto: %q", got)
	}

	cliente.updateErr = &types.ConditionalCheckFailedException{}
	actualizado, err = NewPushDevicesStore(cliente, "nexum-table").Revoke(
		context.Background(), "user-a", "018f90ef-08e5-7a68-b09f-2b67a47c92f0", e059AdaptadorAhora)
	if err != nil || actualizado {
		t.Fatalf("condición fallida debe ser baja ya aplicada: %v %v", actualizado, err)
	}
	cliente.updateErr = errors.New("dynamo down")
	if _, err = NewPushDevicesStore(cliente, "nexum-table").Revoke(
		context.Background(), "user-a", "018f90ef-08e5-7a68-b09f-2b67a47c92f0", e059AdaptadorAhora); !errors.Is(err, appPush.ErrUnavailable) {
		t.Fatalf("fallo real de Dynamo: %v", err)
	}
}

func TestE059AdaptadorFallaCerrada(t *testing.T) {
	store := NewPushDevicesStore(nil, "nexum-table")
	ctx := context.Background()
	escritura := escrituraE059("")
	if _, err := store.LoadIdempotency(ctx, "user-a", "clave-idempotencia-0001", e059AdaptadorAhora); !errors.Is(err, appPush.ErrUnavailable) {
		t.Fatalf("sin cliente LoadIdempotency: %v", err)
	}
	if _, err := store.FindActiveByLabel(ctx, "user-a", "fcm", "iPhone"); !errors.Is(err, appPush.ErrUnavailable) {
		t.Fatalf("sin cliente Query: %v", err)
	}
	if err := store.Register(ctx, escritura); !errors.Is(err, appPush.ErrUnavailable) {
		t.Fatalf("sin cliente Register: %v", err)
	}
	if _, err := store.GetDevice(ctx, "user-a", "id"); !errors.Is(err, appPush.ErrUnavailable) {
		t.Fatalf("sin cliente GetDevice: %v", err)
	}
	if _, err := store.Revoke(ctx, "user-a", "id", e059AdaptadorAhora); !errors.Is(err, appPush.ErrUnavailable) {
		t.Fatalf("sin cliente Revoke: %v", err)
	}
	sinTabla := NewPushDevicesStore(&e059FakeClient{}, "")
	if err := sinTabla.Register(ctx, escritura); !errors.Is(err, appPush.ErrUnavailable) {
		t.Fatalf("sin tabla: %v", err)
	}
}
