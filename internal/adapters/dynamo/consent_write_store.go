package dynamo

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	appPrivacy "github.com/nexumpr/Nexum-Backend/internal/application/privacy"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
)

// ConsentWriteAPI es el subconjunto del cliente DynamoDB que necesita el registro de
// consentimientos (E017). Se declara con la única operación que usa: el compilador garantiza
// que esta ruta no puede ejecutar `Scan`, `DeleteItem` ni ninguna escritura de reemplazo.
type ConsentWriteAPI interface {
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
}

// ConsentWriteStore implementa `appPrivacy.ConsentWriter` (E017) sobre la tabla única.
//
// Patrón físico, en la partición del propietario:
//
//	USER#<id> / CONSENT#<RFC3339 con 9 dígitos de fracción>#<uuid>
//
// El ancho fijo del instante hace que el orden lexicográfico de DynamoDB coincida con el orden
// cronológico, de modo que E016 (que consulta en orden descendente) devuelve primero la
// decisión más reciente sin ordenar en memoria. La escritura es append-only: `PutItem` con
// `attribute_not_exists(PK)` no sobrescribe ningún consentimiento previo, incluidos los de
// E001 (`CONSENT#<version>`).
//
// No crea tablas ni índices, no reescribe ítems existentes y nunca usa `Scan`.
type ConsentWriteStore struct {
	client ConsentWriteAPI
	table  string
}

// NewConsentWriteStore construye el almacén real. Sin cliente o sin tabla todas las
// operaciones fallan cerradas: nunca se sustituyen por memoria local.
func NewConsentWriteStore(client ConsentWriteAPI, table string) *ConsentWriteStore {
	return &ConsentWriteStore{client: client, table: strings.TrimSpace(table)}
}

// consentSortKeyTimeLayout fija 9 dígitos de fracción y el sufijo `Z` UTC. Es el mismo formato
// que documenta el historial de E016 y el que hace ordenable la `SK` como texto.
const consentSortKeyTimeLayout = "2006-01-02T15:04:05.000000000Z"

// AppendConsent escribe la decisión una sola vez en la partición de su titular.
//
// La condición `attribute_not_exists(PK)` es el requisito contractual de append-only: si la
// clave ya existe, DynamoDB devuelve `ConditionalCheckFailedException` y se traduce en 409 sin
// modificar el ítem previo. Cualquier otro fallo se traduce en 503.
func (s *ConsentWriteStore) AppendConsent(ctx context.Context, decision appPrivacy.ConsentDecision) error {
	if s == nil || s.client == nil || s.table == "" {
		return appPrivacy.ErrConsentUnavailable
	}

	userID := strings.TrimSpace(decision.UserID)
	if userID == "" {
		return appPrivacy.ErrConsentUnauthorized
	}

	id := strings.TrimSpace(decision.ID)
	noticeVersion := strings.TrimSpace(decision.NoticeVersion)
	purpose := strings.TrimSpace(decision.Purpose)
	if id == "" || noticeVersion == "" || !appPrivacy.IsConsentPurpose(purpose) {
		return appPrivacy.ErrConsentDecisionInvalid
	}

	recordedAt := decision.RecordedAt.UTC()
	if recordedAt.IsZero() {
		return appPrivacy.ErrConsentDecisionInvalid
	}
	createdAt := decision.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = recordedAt
	}
	source := strings.TrimSpace(decision.Source)
	if source == "" {
		source = appPrivacy.ConsentSourceAPI
	}

	item := map[string]types.AttributeValue{
		"PK":            stringAttr(domainDynamo.PrefixUser + userID),
		"SK":            stringAttr(consentSortKey(recordedAt, id)),
		"id":            stringAttr(id),
		"noticeVersion": stringAttr(noticeVersion),
		"purpose":       stringAttr(purpose),
		"granted":       &types.AttributeValueMemberBOOL{Value: decision.Granted},
		"recordedAt":    stringAttr(recordedAt.Format(time.RFC3339Nano)),
		"source":        stringAttr(source),
		"createdAt":     stringAttr(createdAt.Format(time.RFC3339Nano)),
	}

	_, err := s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(s.table),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	})
	if err != nil {
		// La condición sólo puede fallar si la clave ya existía: un consentimiento previo no
		// se sobrescribe jamás y el cliente recibe 409.
		var condicion *types.ConditionalCheckFailedException
		if errors.As(err, &condicion) {
			return appPrivacy.ErrConsentConflict
		}
		return appPrivacy.ErrConsentUnavailable
	}
	return nil
}

// consentSortKey compone la `SK` canónica del historial de consentimientos.
func consentSortKey(recordedAt time.Time, id string) string {
	return domainDynamo.PrefixConsent + recordedAt.UTC().Format(consentSortKeyTimeLayout) + "#" + id
}
