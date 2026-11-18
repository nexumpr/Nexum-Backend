package dynamo

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	appPrivacy "github.com/nexumpr/Nexum-Backend/internal/application/privacy"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
)

// ConsentsAPI es el subconjunto del cliente DynamoDB que necesita el almacén de
// consentimientos. Se declara con las únicas operaciones que usa: el compilador garantiza que
// ninguna ruta puede ejecutar `Scan`.
type ConsentsAPI interface {
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
}

// ConsentsStore implementa `appPrivacy.ConsentsStore` (E016) sobre la tabla única existente.
//
// Patrón físico, en la partición del propietario:
//   - consentimiento nuevo   USER#<id> / CONSENT#<RFC3339Nano fijo>#<uuid>
//   - consentimiento E001    USER#<id> / CONSENT#<version>   (sólo lectura)
//
// No crea tablas ni índices, no reescribe ítems existentes y nunca usa `Scan`.
type ConsentsStore struct {
	client ConsentsAPI
	table  string
}

// NewConsentsStore construye el almacén real. Sin cliente o tabla todas las operaciones fallan
// cerradas: nunca se sustituyen por memoria local.
func NewConsentsStore(client ConsentsAPI, table string) *ConsentsStore {
	return &ConsentsStore{client: client, table: strings.TrimSpace(table)}
}

// consentsOverfetch pide una fila de más para saber si existe página siguiente sin emitir un
// cursor que devuelva una página vacía.
const consentsOverfetch = 1

// ListUserConsents consulta la partición del titular con la condición de clave
// `PK = USER#<id> AND begins_with(SK, "CONSENT#")`, orden DESCENDENTE por `SK` y paginación por
// `ExclusiveStartKey`. Devuelve las filas de la página y la `SK` del último ítem devuelto
// cuando la persistencia confirma que hay más.
//
// Los consentimientos de E001 (`CONSENT#<version>`) se leen y se proyectan al contrato sin
// modificarse: propósito `privacyNotice`, `granted=true`, `recordedAt=acceptedAt` e `id`
// determinista. Un ítem sin los datos mínimos se omite en lugar de inventarse.
func (s *ConsentsStore) ListUserConsents(ctx context.Context, userID, startSK string, limit int) ([]appPrivacy.ConsentRecord, string, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, "", appPrivacy.ErrConsentsUnavailable
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, "", appPrivacy.ErrConsentsUnauthorized
	}
	if limit <= 0 || limit > appPrivacy.ConsentsMaxLimit {
		return nil, "", appPrivacy.ErrConsentsInvalidRequest
	}

	pk := domainDynamo.PrefixUser + userID
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.table),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :prefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":     stringAttr(pk),
			":prefix": stringAttr(domainDynamo.PrefixConsent),
		},
		// Lectura consistente: el historial debe reflejar un consentimiento ya confirmado por
		// otra instancia.
		ConsistentRead:   aws.Bool(true),
		ScanIndexForward: aws.Bool(false),
		Limit:            aws.Int32(int32(limit + consentsOverfetch)),
	}
	if startSK != "" {
		input.ExclusiveStartKey = map[string]types.AttributeValue{
			"PK": stringAttr(pk),
			"SK": stringAttr(startSK),
		}
	}

	out, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, "", appPrivacy.ErrConsentsUnavailable
	}
	if out == nil {
		return nil, "", nil
	}

	raw := out.Items
	nextSK := ""
	if len(raw) > limit {
		// El cursor apunta a la SK del último ítem devuelto, de modo que la página siguiente
		// empieza después de él en el orden descendente: sin duplicados y sin saltos.
		nextSK = stringAttribute(raw[limit-1]["SK"])
		raw = raw[:limit]
	}

	records := make([]appPrivacy.ConsentRecord, 0, len(raw))
	for _, item := range raw {
		record, ok := consentRecordFromItem(item, userID)
		if !ok {
			// Ítem sin datos mínimos: no se inventa un consentimiento. El cursor ya se calculó
			// sobre la posición física, así que no se saltan filas.
			continue
		}
		records = append(records, record)
	}
	return records, nextSK, nil
}

// consentRecordFromItem reduce un ítem de DynamoDB a la fila de aplicación, distinguiendo el
// formato nuevo del histórico de E001 por la forma de la `SK`.
func consentRecordFromItem(item map[string]types.AttributeValue, userID string) (appPrivacy.ConsentRecord, bool) {
	sk := stringAttribute(item["SK"])
	if !strings.HasPrefix(sk, domainDynamo.PrefixConsent) {
		return appPrivacy.ConsentRecord{}, false
	}
	resto := strings.TrimPrefix(sk, domainDynamo.PrefixConsent)
	if resto == "" {
		return appPrivacy.ConsentRecord{}, false
	}

	if strings.Contains(resto, "#") {
		return consentRecordFromNewItem(item, sk)
	}
	return consentRecordFromLegacyItem(item, userID, sk, resto)
}

// consentRecordFromNewItem proyecta un ítem escrito por E017. Exige los cinco campos del
// contrato: sin ellos no puede emitirse un `Consent` válido.
func consentRecordFromNewItem(item map[string]types.AttributeValue, sk string) (appPrivacy.ConsentRecord, bool) {
	id := strings.TrimSpace(stringAttribute(item["id"]))
	noticeVersion := strings.TrimSpace(stringAttribute(item["noticeVersion"]))
	purpose := strings.TrimSpace(stringAttribute(item["purpose"]))
	if id == "" || noticeVersion == "" || !appPrivacy.IsConsentPurpose(purpose) {
		return appPrivacy.ConsentRecord{}, false
	}
	granted, ok := consentBoolAttribute(item["granted"])
	if !ok {
		return appPrivacy.ConsentRecord{}, false
	}
	recordedAt, ok := parseConsentTime(item["recordedAt"])
	if !ok {
		return appPrivacy.ConsentRecord{}, false
	}
	return appPrivacy.ConsentRecord{
		SortKey:       sk,
		ID:            id,
		NoticeVersion: noticeVersion,
		Purpose:       purpose,
		Granted:       granted,
		RecordedAt:    recordedAt,
	}, true
}

// consentRecordFromLegacyItem proyecta un consentimiento de E001 (`CONSENT#<version>`), que
// sólo guarda la versión aceptada y el instante. No se reescribe: la proyección es de lectura.
func consentRecordFromLegacyItem(item map[string]types.AttributeValue, userID, sk, noticeVersion string) (appPrivacy.ConsentRecord, bool) {
	acceptedAt, ok := parseLegacyAcceptedAt(item["acceptedAt"])
	if !ok {
		return appPrivacy.ConsentRecord{}, false
	}
	return appPrivacy.ConsentRecord{
		SortKey:       sk,
		ID:            legacyConsentID(userID, sk),
		NoticeVersion: noticeVersion,
		Purpose:       appPrivacy.ConsentPurposePrivacyNotice,
		Granted:       true,
		RecordedAt:    acceptedAt,
	}, true
}

// legacyConsentID deriva un identificador estable del ítem histórico. Es determinista (el
// mismo ítem produce siempre el mismo `id`) y tiene forma de UUID v4 para satisfacer
// `format: uuid`; no es un identificador aleatorio y no se presenta como tal.
func legacyConsentID(userID, sk string) string {
	sum := sha256.Sum256([]byte("nexum:consent:legacy:v1\x00" + userID + "\x00" + sk))
	var b [16]byte
	copy(b[:], sum[:16])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// consentBoolAttribute interpreta un atributo booleano y distingue el valor ausente.
func consentBoolAttribute(value types.AttributeValue) (bool, bool) {
	flag, ok := value.(*types.AttributeValueMemberBOOL)
	if !ok {
		return false, false
	}
	return flag.Value, true
}

// parseConsentTime interpreta la marca `recordedAt` de un consentimiento nuevo.
func parseConsentTime(value types.AttributeValue) (time.Time, bool) {
	return parseConsentTimeText(stringAttribute(value))
}

// parseConsentTimeText acepta RFC3339 con o sin fracción de segundo.
func parseConsentTimeText(texto string) (time.Time, bool) {
	texto = strings.TrimSpace(texto)
	if texto == "" {
		return time.Time{}, false
	}
	if parsed, err := time.Parse(time.RFC3339Nano, texto); err == nil {
		return parsed.UTC(), true
	}
	if parsed, err := time.Parse(time.RFC3339, texto); err == nil {
		return parsed.UTC(), true
	}
	return time.Time{}, false
}

// parseLegacyAcceptedAt interpreta `acceptedAt` del ítem de E001. Se admiten las dos formas
// que puede haber escrito el backend: texto ISO-8601 o Number con epoch en segundos (misma
// convención que el TTL físico `expiresAt`). Cualquier otra forma se rechaza.
func parseLegacyAcceptedAt(value types.AttributeValue) (time.Time, bool) {
	if seconds, ok := attributeInt(value); ok {
		if seconds <= 0 {
			return time.Time{}, false
		}
		return time.Unix(seconds, 0).UTC(), true
	}
	return parseConsentTimeText(stringAttribute(value))
}
