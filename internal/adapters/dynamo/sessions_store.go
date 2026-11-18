package dynamo

import (
	"context"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
)

// SessionsAPI es el subconjunto del cliente DynamoDB que necesita el almacén de sesiones de
// E008/E009. Se declara aquí (y no se reutiliza `DynamoAPI`) porque E008 necesita `Query` y
// E009 `GetItem`+`TransactWriteItems`: así el compilador garantiza que ninguna ruta usa `Scan`.
type SessionsAPI interface {
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	TransactWriteItems(ctx context.Context, params *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error)
}

// SessionsStore implementa `appIdentity.SessionsStore` (E008) y, en la extensión de E009,
// `appIdentity.SessionRevokeStore`, sobre la tabla única existente. No crea tablas ni índices y
// nunca usa `Scan`.
//
// Patrón físico, idéntico al que ya escriben E002/E003/E004:
//   - sesión autoritativa     USER#<userId> / SESSION#<sessionId>
//   - localizador de refresh  REFRESH#<sha256(refresh)> / SESSION
type SessionsStore struct {
	client SessionsAPI
	table  string
}

// NewSessionsStore construye el almacén real. Sin cliente o tabla todas las operaciones fallan
// cerradas: nunca se sustituyen por memoria local.
func NewSessionsStore(client SessionsAPI, table string) *SessionsStore {
	return &SessionsStore{client: client, table: strings.TrimSpace(table)}
}

// sessionsOverfetch pide una fila de más para saber si existe página siguiente sin emitir un
// cursor que devuelva una página vacía.
const sessionsOverfetch = 1

// ListUserSessions consulta la partición del titular con la condición de clave
// `PK = USER#<id> AND begins_with(SK, "SESSION#")`, orden ascendente por `SK` y paginación por
// `ExclusiveStartKey`. Devuelve las filas de la página y la `SK` del último ítem devuelto cuando
// la persistencia confirma que hay más.
func (s *SessionsStore) ListUserSessions(ctx context.Context, userID, startSK string, limit int) ([]appIdentity.SessionListRecord, string, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, "", appIdentity.ErrSessionsUnavailable
	}
	if strings.TrimSpace(userID) == "" {
		return nil, "", appIdentity.ErrSessionsUnavailable
	}
	if limit <= 0 || limit > appIdentity.SessionsMaxLimit {
		return nil, "", appIdentity.ErrSessionsUnavailable
	}

	pk := domainDynamo.PrefixUser + userID
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.table),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :prefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":     stringAttr(pk),
			":prefix": stringAttr(domainDynamo.PrefixSession),
		},
		// Lectura consistente: la lista debe reflejar una revocación ya confirmada por otra
		// instancia (E008 §5, T05).
		ConsistentRead:   aws.Bool(true),
		ScanIndexForward: aws.Bool(true),
		Limit:            aws.Int32(int32(limit + sessionsOverfetch)),
	}
	if startSK != "" {
		input.ExclusiveStartKey = map[string]types.AttributeValue{
			"PK": stringAttr(pk),
			"SK": stringAttr(startSK),
		}
	}

	out, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, "", appIdentity.ErrSessionsUnavailable
	}
	if out == nil {
		return nil, "", nil
	}

	raw := out.Items
	nextSK := ""
	if len(raw) > limit {
		// El cursor apunta a la SK del último ítem devuelto, de modo que la página siguiente
		// empieza después de él: sin duplicados y sin saltos.
		nextSK = stringAttribute(raw[limit-1]["SK"])
		raw = raw[:limit]
	}

	records := make([]appIdentity.SessionListRecord, 0, len(raw))
	for _, item := range raw {
		record, ok := sessionListRecord(item)
		if !ok {
			// Ítem con otra entidad u hoja sin datos mínimos: no se inventa una sesión. El
			// cursor ya se calculó sobre la posición física, así que no se saltan filas.
			continue
		}
		records = append(records, record)
	}
	return records, nextSK, nil
}

// sessionListRecord reduce un ítem de DynamoDB a la fila de aplicación. Sólo acepta ítems de
// tipo `USER_SESSION` y exige `createdAt` legible: sin él no puede cumplirse el contrato.
func sessionListRecord(item map[string]types.AttributeValue) (appIdentity.SessionListRecord, bool) {
	if entity := stringAttribute(item["entityType"]); entity != "USER_SESSION" {
		return appIdentity.SessionListRecord{}, false
	}
	sessionID := stringAttribute(item["sessionId"])
	sortKey := stringAttribute(item["SK"])
	if sessionID == "" || sortKey == "" {
		return appIdentity.SessionListRecord{}, false
	}
	createdAt, ok := parseSessionTime(stringAttribute(item["createdAt"]))
	if !ok {
		return appIdentity.SessionListRecord{}, false
	}
	// `lastSeenAt` del contrato se mapea desde `lastUsedAt`, que es el atributo que escribe el
	// backend. Si falta, se usa `createdAt` (nunca la hora actual).
	lastSeenAt, ok := parseSessionTime(stringAttribute(item["lastUsedAt"]))
	if !ok {
		lastSeenAt = createdAt
	}
	return appIdentity.SessionListRecord{
		SortKey:    sortKey,
		SessionID:  sessionID,
		CreatedAt:  createdAt,
		LastSeenAt: lastSeenAt,
		Status:     stringAttribute(item["status"]),
	}, true
}

// parseSessionTime interpreta las marcas de tiempo ISO-8601 que escriben E002/E003/E004.
func parseSessionTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed.UTC(), true
	}
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed.UTC(), true
	}
	return time.Time{}, false
}
