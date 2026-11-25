package dynamo

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	appPush "github.com/nexumpr/Nexum-Backend/internal/application/pushdevices"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
)

const (
	pushDeviceEntityType = "DEVICE"
	pushIdemEntityType   = "PUSH_IDEMPOTENCY"
	pushDeviceSKPrefix   = "DEVICE#"
	// pushIdemSKPrefix es la SK aprobada en MATRIZ_ACCESOS_I040: IDEM#PUSH#
	// seguida del SHA-256 de la clave. La clave cruda nunca se persiste.
	pushIdemSKPrefix  = "IDEM#PUSH#"
	pushDeviceVersion = int64(1)

	// pushLabelQueryPages acota el recorrido paginado del Query por etiqueta.
	pushLabelQueryPages = 20
)

// PushDevicesAPI es el subconjunto del cliente DynamoDB usado por E059/E060.
// TransactWriteItems, GetItem, Query y UpdateItem son las únicas operaciones
// expuestas: no hay Scan, ni tablas o índices nuevos.
type PushDevicesAPI interface {
	TransactWriteItems(ctx context.Context, params *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error)
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
}

// PushDevicesStore implementa `pushdevices.Store` sobre la tabla única:
//   - USER#<sub> / DEVICE#<uuid>       -> dispositivo y token cifrado
//   - USER#<sub> / IDEM#PUSH#<sha256>  -> idempotencia de E059
type PushDevicesStore struct {
	client PushDevicesAPI
	table  string
}

// NewPushDevicesStore construye el adaptador. Sin cliente o tabla, todas las
// operaciones fallan cerradas: nunca se sustituye DynamoDB por memoria local.
func NewPushDevicesStore(client PushDevicesAPI, table string) *PushDevicesStore {
	return &PushDevicesStore{client: client, table: strings.TrimSpace(table)}
}

func pushDeviceKeys(userID, deviceID string) (string, string) {
	return domainDynamo.PrefixUser + strings.TrimSpace(userID), pushDeviceSKPrefix + strings.TrimSpace(deviceID)
}

func pushIdempotencyKeys(userID, key string) (string, string) {
	suma := sha256.Sum256([]byte(strings.TrimSpace(key)))
	return domainDynamo.PrefixUser + strings.TrimSpace(userID), pushIdemSKPrefix + hex.EncodeToString(suma[:])
}

// LoadIdempotency lee con consistencia fuerte. Un registro expirado cuenta como
// ausente: la clave puede reutilizarse sin arrastrar una respuesta obsoleta.
func (s *PushDevicesStore) LoadIdempotency(ctx context.Context, userID, key string, now time.Time) (*appPush.IdempotencyRecord, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, appPush.ErrUnavailable
	}
	pk, sk := pushIdempotencyKeys(userID, key)
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      aws.String(s.table),
		Key:            map[string]types.AttributeValue{"PK": stringAttr(pk), "SK": stringAttr(sk)},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return nil, appPush.ErrUnavailable
	}
	if out == nil || stringAttribute(out.Item["entityType"]) != pushIdemEntityType {
		return nil, nil
	}
	expiresAt, ok := attributeInt(out.Item["expiresAt"])
	if !ok {
		return nil, appPush.ErrUnavailable
	}
	if now.Unix() >= expiresAt {
		return nil, nil
	}
	return &appPush.IdempotencyRecord{
		DeviceID:     stringAttribute(out.Item["deviceId"]),
		PayloadHash:  stringAttribute(out.Item["payloadHash"]),
		ResponseJSON: []byte(stringAttribute(out.Item["responseJSON"])),
		ExpiresAt:    expiresAt,
	}, nil
}

// FindActiveByLabel hace Query sobre la partición del usuario con
// `begins_with(SK, "DEVICE#")` y filtra en memoria por proveedor, etiqueta y
// active=true. Nunca consulta otra PK ni usa Scan. Si hay más de una
// instalación activa con la misma etiqueta gana la de registeredAt más reciente.
func (s *PushDevicesStore) FindActiveByLabel(ctx context.Context, userID, provider, label string) (*appPush.Device, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, appPush.ErrUnavailable
	}
	pk, _ := pushDeviceKeys(userID, "")
	var start map[string]types.AttributeValue
	var mejor *appPush.Device
	for pagina := 0; pagina < pushLabelQueryPages; pagina++ {
		out, err := s.client.Query(ctx, &dynamodb.QueryInput{
			TableName:              aws.String(s.table),
			KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :prefix)"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":pk":     stringAttr(pk),
				":prefix": stringAttr(pushDeviceSKPrefix),
			},
			ConsistentRead:    aws.Bool(true),
			ScanIndexForward:  aws.Bool(true),
			Limit:             aws.Int32(100),
			ExclusiveStartKey: start,
		})
		if err != nil {
			return nil, appPush.ErrUnavailable
		}
		if out == nil {
			break
		}
		for _, item := range out.Items {
			dispositivo := pushDeviceFromItem(item)
			if dispositivo == nil || !dispositivo.Active ||
				dispositivo.Provider != provider || dispositivo.DeviceLabel != label {
				continue
			}
			if mejor == nil || dispositivo.RegisteredAt.After(mejor.RegisteredAt) {
				mejor = dispositivo
			}
		}
		if len(out.LastEvaluatedKey) == 0 {
			return mejor, nil
		}
		start = out.LastEvaluatedKey
	}
	return nil, appPush.ErrUnavailable
}

// Register ejecuta la transacción atómica de E059: Put del dispositivo y Put
// del registro de idempotencia. El Put del dispositivo exige attribute_not_exists
// para un ID nuevo o la huella anterior para una rotación; el de idempotencia
// exige que la clave no exista (o que haya expirado).
func (s *PushDevicesStore) Register(ctx context.Context, write appPush.RegisterWrite) error {
	if s == nil || s.client == nil || s.table == "" {
		return appPush.ErrUnavailable
	}
	if write.Device.ID == "" || write.UserID == "" || write.IdempotencyKey == "" ||
		write.PayloadHash == "" || len(write.ResponseJSON) == 0 {
		return appPush.ErrUnavailable
	}
	now := write.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	ttl := write.TTL
	if ttl <= 0 {
		ttl = appPush.IdempotencyTTL
	}
	pk, sk := pushDeviceKeys(write.UserID, write.Device.ID)
	idemPK, idemSK := pushIdempotencyKeys(write.UserID, write.IdempotencyKey)

	deviceItem := map[string]types.AttributeValue{
		"PK":               stringAttr(pk),
		"SK":               stringAttr(sk),
		"entityType":       stringAttr(pushDeviceEntityType),
		"userId":           stringAttr(strings.TrimSpace(write.UserID)),
		"provider":         stringAttr(write.Device.Provider),
		"deviceLabel":      stringAttr(write.Device.DeviceLabel),
		"registeredAt":     stringAttr(now.Format(time.RFC3339Nano)),
		"tokenCipher":      stringAttr(base64.StdEncoding.EncodeToString(write.Device.TokenCipher)),
		"tokenFingerprint": stringAttr(write.Device.TokenFingerprint),
		"active":           boolAttr(true),
		"version":          numberAttr(pushDeviceVersion),
	}

	deviceCondition := aws.String("attribute_not_exists(PK)")
	deviceNames := map[string]string(nil)
	deviceValues := map[string]types.AttributeValue(nil)
	if huella := strings.TrimSpace(write.PreviousFingerprint); huella != "" {
		deviceCondition = aws.String("attribute_exists(PK) AND #tokenFingerprint = :previous")
		deviceNames = map[string]string{"#tokenFingerprint": "tokenFingerprint"}
		deviceValues = map[string]types.AttributeValue{":previous": stringAttr(huella)}
	}

	idemItem := map[string]types.AttributeValue{
		"PK":           stringAttr(idemPK),
		"SK":           stringAttr(idemSK),
		"entityType":   stringAttr(pushIdemEntityType),
		"deviceId":     stringAttr(write.Device.ID),
		"payloadHash":  stringAttr(write.PayloadHash),
		"responseJSON": stringAttr(string(write.ResponseJSON)),
		"expiresAt":    numberAttr(now.Add(ttl).Unix()),
		"version":      numberAttr(pushDeviceVersion),
	}

	_, err := s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{Put: &types.Put{
				TableName:                 aws.String(s.table),
				Item:                      deviceItem,
				ConditionExpression:       deviceCondition,
				ExpressionAttributeNames:  deviceNames,
				ExpressionAttributeValues: deviceValues,
			}},
			{Put: &types.Put{
				TableName:           aws.String(s.table),
				Item:                idemItem,
				ConditionExpression: aws.String("attribute_not_exists(PK) OR #expiresAt < :now"),
				ExpressionAttributeNames: map[string]string{
					"#expiresAt": "expiresAt",
				},
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":now": numberAttr(now.Unix()),
				},
			}},
		},
	})
	if err == nil {
		return nil
	}
	var cancelada *types.TransactionCanceledException
	if errors.As(err, &cancelada) {
		switch pushFailureCode(cancelada.CancellationReasons, 0) {
		case "ConditionalCheckFailed":
			return appPush.ErrDeviceCondition
		}
		switch pushFailureCode(cancelada.CancellationReasons, 1) {
		case "ConditionalCheckFailed":
			return appPush.ErrIdempotencyCondition
		}
	}
	return appPush.ErrUnavailable
}

// GetDevice lee exclusivamente USER#<sub>/DEVICE#<id>. Un ID ajeno cae en otra
// clave primaria y se devuelve como ausente, sin revelar si existe en otra
// partición.
func (s *PushDevicesStore) GetDevice(ctx context.Context, userID, deviceID string) (*appPush.Device, error) {
	if s == nil || s.client == nil || s.table == "" || strings.TrimSpace(deviceID) == "" {
		return nil, appPush.ErrUnavailable
	}
	pk, sk := pushDeviceKeys(userID, deviceID)
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      aws.String(s.table),
		Key:            map[string]types.AttributeValue{"PK": stringAttr(pk), "SK": stringAttr(sk)},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return nil, appPush.ErrUnavailable
	}
	return pushDeviceFromItem(out.Item), nil
}

// Revoke desactiva el dispositivo bajo condición `active = true`, retira el
// token cifrado y su huella, y fija revokedAt. Devuelve false si otra instancia
// ya había completado la baja; en ese caso el servicio decide entre 204 y 404
// con una relectura.
func (s *PushDevicesStore) Revoke(ctx context.Context, userID, deviceID string, now time.Time) (bool, error) {
	if s == nil || s.client == nil || s.table == "" || strings.TrimSpace(deviceID) == "" {
		return false, appPush.ErrUnavailable
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	pk, sk := pushDeviceKeys(userID, deviceID)
	_, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:           aws.String(s.table),
		Key:                 map[string]types.AttributeValue{"PK": stringAttr(pk), "SK": stringAttr(sk)},
		UpdateExpression:    aws.String("SET #active = :inactive, #revokedAt = :revokedAt, #version = :version + :one REMOVE #tokenCipher, #tokenFingerprint"),
		ConditionExpression: aws.String("#active = :active"),
		ExpressionAttributeNames: map[string]string{
			"#active":           "active",
			"#revokedAt":        "revokedAt",
			"#version":          "version",
			"#tokenCipher":      "tokenCipher",
			"#tokenFingerprint": "tokenFingerprint",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":active":    boolAttr(true),
			":inactive":  boolAttr(false),
			":revokedAt": stringAttr(now.UTC().Format(time.RFC3339Nano)),
			":version":   numberAttr(pushDeviceVersion),
			":one":       numberAttr(1),
		},
	})
	if err == nil {
		return true, nil
	}
	var condicional *types.ConditionalCheckFailedException
	if errors.As(err, &condicional) {
		return false, nil
	}
	return false, appPush.ErrUnavailable
}

func pushDeviceFromItem(item map[string]types.AttributeValue) *appPush.Device {
	if len(item) == 0 || stringAttribute(item["entityType"]) != pushDeviceEntityType {
		return nil
	}
	sk := stringAttribute(item["SK"])
	if !strings.HasPrefix(sk, pushDeviceSKPrefix) {
		return nil
	}
	registrado, err := time.Parse(time.RFC3339Nano, stringAttribute(item["registeredAt"]))
	if err != nil {
		registrado, err = time.Parse(time.RFC3339, stringAttribute(item["registeredAt"]))
		if err != nil {
			return nil
		}
	}
	return &appPush.Device{
		ID:               strings.TrimPrefix(sk, pushDeviceSKPrefix),
		Provider:         stringAttribute(item["provider"]),
		DeviceLabel:      stringAttribute(item["deviceLabel"]),
		RegisteredAt:     registrado.UTC(),
		Active:           boolAttribute(item["active"]),
		TokenFingerprint: stringAttribute(item["tokenFingerprint"]),
	}
}

func pushFailureCode(reasons []types.CancellationReason, index int) string {
	if index < 0 || index >= len(reasons) || reasons[index].Code == nil {
		return ""
	}
	return *reasons[index].Code
}
