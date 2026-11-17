package dynamo

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
)

// LogoutStore implementa E004 sobre la tabla única existente. Cumple dos puertos:
//
//   - appIdentity.LogoutStore: lectura autoritativa y revocación de la sesión actual.
//   - ports.SessionAuthorizer: verificación de revocación para el middleware de autenticación.
//
// Patrón físico (idéntico al que ya escriben E002 y E003, sin tablas ni índices nuevos):
//   - sesión autoritativa    USER#<userId> / SESSION#<id>
//   - localizador de refresh REFRESH#<sha256(refresh)> / SESSION
//
// Nunca usa Scan, ni memoria local, ni caché como autoridad: la revocación debe ser observable
// de inmediato por cualquier instancia (GOV-02, A-080 §2.6).
type LogoutStore struct {
	client DynamoAPI
	table  string
}

// NewLogoutStore construye el almacén real. Sin cliente o tabla, todas las operaciones fallan
// cerradas: nunca se sustituyen por almacenamiento local.
func NewLogoutStore(client DynamoAPI, table string) *LogoutStore {
	return &LogoutStore{client: client, table: strings.TrimSpace(table)}
}

// LoadLogoutSession lee `USER#<id>/SESSION#<id>` con consistencia fuerte.
// Devuelve (nil, nil) cuando la sesión no existe o el TTL ya la purgó.
func (s *LogoutStore) LoadLogoutSession(ctx context.Context, userID, sessionID string) (*appIdentity.LogoutSession, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, appIdentity.ErrLogoutUnavailable
	}
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(sessionID) == "" {
		return nil, nil
	}

	pk, sk := domainDynamo.UserSessionKeys(userID, sessionID)
	item, err := s.getItem(ctx, pk, sk)
	if err != nil {
		return nil, appIdentity.ErrLogoutUnavailable
	}
	if item == nil {
		return nil, nil
	}
	if entity := stringAttribute(item["entityType"]); entity != "" && entity != "USER_SESSION" {
		// Un ítem con otra entidad en la misma clave no es una sesión: no se revoca nada.
		return nil, nil
	}

	session := &appIdentity.LogoutSession{
		UserID:      stringAttribute(item["userId"]),
		SessionID:   stringAttribute(item["sessionId"]),
		FamilyID:    stringAttribute(item["familyId"]),
		RefreshHash: stringAttribute(item["refreshHash"]),
		Status:      stringAttribute(item["status"]),
	}
	if session.UserID == "" || session.SessionID == "" {
		return nil, nil
	}
	if version, ok := attributeInt(item["version"]); ok {
		session.Version = version
		session.HasVersion = true
	}
	if expires, ok := attributeTime(item); ok {
		session.ExpiresAt = expires
		session.HasExpiry = true
	}
	return session, nil
}

// RevokeLogoutSession aplica `ACTIVE -> REVOKED` en UNA transacción condicional:
//
//  1. la sesión pasa a REVOKED conservando su identidad, con CAS de estado y versión
//     (`#status = :active AND #version = :expected`); la versión se incrementa;
//  2. el localizador del refresh vigente (`refreshHash` leído de la sesión) pasa a REVOKED,
//     de modo que E003 deje de poder rotar y responda 401.
//
// Un fallo de condición de la sesión se traduce en ErrLogoutConflict (el llamador relee y
// decide con el estado vigente). Un fallo de servicio se traduce en ErrLogoutUnavailable:
// nunca se confunden, y ninguna de las dos rutas permite responder 204.
func (s *LogoutStore) RevokeLogoutSession(ctx context.Context, revocation appIdentity.LogoutRevocation) error {
	if s == nil || s.client == nil || s.table == "" {
		return appIdentity.ErrLogoutUnavailable
	}
	if strings.TrimSpace(revocation.UserID) == "" || strings.TrimSpace(revocation.SessionID) == "" {
		return appIdentity.ErrLogoutUnavailable
	}

	now := revocation.Now.UTC()
	nowISO := now.Format(time.RFC3339)
	reason := revocation.Reason
	if reason == "" {
		reason = appIdentity.RevokeReasonLogout
	}

	sessionPK, sessionSK := domainDynamo.UserSessionKeys(revocation.UserID, revocation.SessionID)

	// Transición condicional. Con `version` presente (E002/E003) se usa compare-and-swap de
	// estado y versión: una rotación concurrente invalida la escritura y obliga a releer. En un
	// ítem heredado sin `version` no hay rotador posible (E003 exige el atributo), así que basta
	// el CAS de estado y la versión se inicializa a 1.
	condicion := "attribute_exists(PK) AND #status = :active"
	actualizacion := "SET #status = :revoked, revokedAt = :now, revokedAtISO = :nowIso, revokeReason = :reason, #version = :one"
	valores := map[string]types.AttributeValue{
		":revoked": stringAttr(appIdentity.StatusRevoked),
		":now":     stringAttr(nowISO),
		":nowIso":  stringAttr(nowISO),
		":reason":  stringAttr(reason),
		":active":  stringAttr(appIdentity.StatusActive),
		":one":     numberAttr(1),
	}
	if revocation.HasVersion {
		condicion += " AND #version = :expected"
		actualizacion = "SET #status = :revoked, revokedAt = :now, revokedAtISO = :nowIso, revokeReason = :reason ADD #version :one"
		valores[":expected"] = numberAttr(revocation.ExpectedVersion)
	}

	sessionItem := types.TransactWriteItem{Update: &types.Update{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(sessionPK),
			"SK": stringAttr(sessionSK),
		},
		UpdateExpression:    aws.String(actualizacion),
		ConditionExpression: aws.String(condicion),
		ExpressionAttributeNames: map[string]string{
			"#status":  "status",
			"#version": "version",
		},
		ExpressionAttributeValues: valores,
	}}

	items := []types.TransactWriteItem{sessionItem}
	if digest := strings.TrimSpace(revocation.RefreshHash); digest != "" {
		pk, sk := domainDynamo.RefreshKeys(digest)
		items = append(items, types.TransactWriteItem{Update: &types.Update{
			TableName: aws.String(s.table),
			Key: map[string]types.AttributeValue{
				"PK": stringAttr(pk),
				"SK": stringAttr(sk),
			},
			UpdateExpression:    aws.String("SET #status = :revoked, revokedAt = :now, revokeReason = :reason"),
			ConditionExpression: aws.String("attribute_exists(PK)"),
			ExpressionAttributeNames: map[string]string{
				"#status": "status",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":revoked": stringAttr(appIdentity.StatusRevoked),
				":now":     stringAttr(nowISO),
				":reason":  stringAttr(reason),
			},
		}})
	}

	_, err := s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: items})
	if err == nil {
		return nil
	}
	if !isConditionalCancel(err) {
		return appIdentity.ErrLogoutUnavailable
	}
	if transactionItemFailed(err, 0) {
		// La sesión no estaba ACTIVE o cambió de versión: es un conflicto real, no un fallo de
		// servicio. El llamador releyendo decide (revocada ⇒ 401; activa ⇒ revoca de nuevo).
		return appIdentity.ErrLogoutConflict
	}
	// Sólo falló el localizador: pudo purgarlo el TTL entre la lectura y la transacción. La
	// sesión es la autoridad y ya está revocada por esta misma transacción, que se canceló
	// completa; se repite únicamente la sesión para dejar la revocación aplicada.
	if _, errRetry := s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{sessionItem},
	}); errRetry != nil {
		if isConditionalCancel(errRetry) {
			return appIdentity.ErrLogoutConflict
		}
		return appIdentity.ErrLogoutUnavailable
	}
	return nil
}

// AuthorizeSession implementa ports.SessionAuthorizer: comprueba que la sesión del token siga
// existiendo, pertenezca al sujeto, no esté revocada, no haya expirado y que la época de
// revocación del token (`epo`) coincida con la vigente del perfil (A6, RFC-016 §17.3).
//
// No compara el `jti` del token con `accessJti`: E003 rota el jti en cada refresh, de modo que
// compararlo invalidaría access tokens todavía válidos de una sesión activa (T19). La
// revocación se aplica al estado de la sesión y a la época del perfil, no a un token concreto.
func (s *LogoutStore) AuthorizeSession(ctx context.Context, userID, sessionID string, epoch int64) error {
	if s == nil || s.client == nil || s.table == "" {
		return ports.ErrSessionUnavailable
	}
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(sessionID) == "" {
		return ports.ErrSessionUnauthorized
	}

	pk, sk := domainDynamo.UserSessionKeys(userID, sessionID)
	item, err := s.getItem(ctx, pk, sk)
	if err != nil {
		return ports.ErrSessionUnavailable
	}
	if item == nil {
		return ports.ErrSessionUnauthorized
	}
	if entity := stringAttribute(item["entityType"]); entity != "" && entity != "USER_SESSION" {
		return ports.ErrSessionUnauthorized
	}
	if owner := stringAttribute(item["userId"]); owner != "" && owner != userID {
		// Sesión de otro sujeto: 401 sin revelar existencia ni titularidad.
		return ports.ErrSessionUnauthorized
	}
	if !statusMatchesActive(stringAttribute(item["status"])) {
		return ports.ErrSessionUnauthorized
	}
	if expires, ok := attributeTime(item); ok && !time.Now().UTC().Before(expires) {
		return ports.ErrSessionUnauthorized
	}

	// Época de revocación: se lee el perfil con consistencia fuerte. Un fallo de DynamoDB
	// debe responder 503, nunca autorizar por defecto; una época desalineada responde 401.
	perfil, errPerfil := s.profileForSession(ctx, userID)
	if errPerfil != nil {
		return ports.ErrSessionUnavailable
	}
	if perfil == nil {
		return ports.ErrSessionUnauthorized
	}
	if perfil.Status != "" && !strings.EqualFold(strings.TrimSpace(perfil.Status), appIdentity.StatusActive) {
		return ports.ErrSessionUnauthorized
	}
	if perfil.Epoch != epoch {
		return ports.ErrSessionUnauthorized
	}
	return nil
}

// sessionProfile es la lectura mínima del perfil necesaria para verificar la época.
type sessionProfile struct {
	Status string
	Epoch  int64
}

// profileForSession lee `USER#<id>/PROFILE` con consistencia fuerte. Devuelve (nil, nil)
// cuando el perfil no existe: una cuenta eliminada no autoriza ninguna sesión.
func (s *LogoutStore) profileForSession(ctx context.Context, userID string) (*sessionProfile, error) {
	pk, sk := domainDynamo.UserKeys(userID)
	item, err := s.getItem(ctx, pk, sk)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, nil
	}
	if entity := stringAttribute(item["entityType"]); entity != "" && entity != "USER_PROFILE" {
		return nil, nil
	}
	perfil := &sessionProfile{Status: stringAttribute(item["status"])}
	if epoch, ok := attributeInt(item["sessionEpoch"]); ok {
		perfil.Epoch = epoch
	}
	return perfil, nil
}

// statusMatchesActive acepta `ACTIVE` y el estado vacío heredado de ítems escritos antes de
// que existiera el atributo; cualquier otro valor (incluido REVOKED y CONSUMED) se rechaza.
func statusMatchesActive(status string) bool {
	return status == "" || strings.EqualFold(strings.TrimSpace(status), appIdentity.StatusActive)
}

func (s *LogoutStore) getItem(ctx context.Context, pk, sk string) (map[string]types.AttributeValue, error) {
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(pk),
			"SK": stringAttr(sk),
		},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return nil, err
	}
	if out == nil || out.Item == nil {
		return nil, nil
	}
	return out.Item, nil
}

// transactionItemFailed informa si el ítem indicado de una transacción cancelada falló por
// condición. `isConditionalCancel` sólo dice que alguna condición falló; aquí importa cuál.
func transactionItemFailed(err error, index int) bool {
	var canceled *types.TransactionCanceledException
	if !errors.As(err, &canceled) {
		return false
	}
	if index < 0 || index >= len(canceled.CancellationReasons) {
		return false
	}
	reason := canceled.CancellationReasons[index]
	if reason.Code == nil {
		return false
	}
	return *reason.Code == "ConditionalCheckFailed"
}
