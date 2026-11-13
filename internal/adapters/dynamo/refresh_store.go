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

// RefreshStore implementa appIdentity.RefreshStore sobre la tabla única existente.
//
// Patrón físico (H06/A-060, idéntico al que ya escribe E002):
//   - sesión autoritativa   USER#<userId> / SESSION#<sessionId>
//   - localizador de refresh REFRESH#<sha256(refresh)> / SESSION
//
// La rotación se aplica en UNA sola TransactWriteItems condicional: el token presentado
// queda consumido, la sesión apunta al digest nuevo y el localizador nuevo nace en la
// misma transacción. No hay `Scan`, ni escritura parcial, ni memoria como autoridad.
type RefreshStore struct {
	client DynamoAPI
	table  string
}

// NewRefreshStore construye el almacén real. Sin cliente o tabla, todas las operaciones
// fallan cerradas: nunca se sustituyen por almacenamiento local.
func NewRefreshStore(client DynamoAPI, table string) *RefreshStore {
	return &RefreshStore{client: client, table: strings.TrimSpace(table)}
}

// LocateRefresh resuelve el digest del token opaco contra su localizador.
// Devuelve (nil, nil) cuando el localizador no existe o el TTL ya lo purgó.
func (s *RefreshStore) LocateRefresh(ctx context.Context, digest string) (*appIdentity.RefreshLocator, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, appIdentity.ErrRefreshUnavailable
	}
	pk, sk := domainDynamo.RefreshKeys(digest)
	item, err := s.getItem(ctx, pk, sk)
	if err != nil {
		return nil, appIdentity.ErrRefreshUnavailable
	}
	if item == nil {
		return nil, nil
	}

	locator := &appIdentity.RefreshLocator{
		UserID:    stringAttribute(item["userId"]),
		SessionID: stringAttribute(item["sessionId"]),
		FamilyID:  stringAttribute(item["familyId"]),
		Status:    stringAttribute(item["status"]),
	}
	if locator.UserID == "" || locator.SessionID == "" {
		// Localizador incoherente: se trata como inexistente, nunca como sesión válida.
		return nil, nil
	}
	if value, ok := item["used"].(*types.AttributeValueMemberBOOL); ok {
		locator.Used = value.Value
	}
	if expires, ok := attributeTime(item); ok {
		locator.ExpiresAt = expires
		locator.HasExpiry = true
	}
	return locator, nil
}

// LoadSession lee la sesión autoritativa creada por E002 con consistencia fuerte.
func (s *RefreshStore) LoadSession(ctx context.Context, userID, sessionID string) (*appIdentity.RefreshSession, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, appIdentity.ErrRefreshUnavailable
	}
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(sessionID) == "" {
		return nil, nil
	}
	pk, sk := domainDynamo.UserSessionKeys(userID, sessionID)
	item, err := s.getItem(ctx, pk, sk)
	if err != nil {
		return nil, appIdentity.ErrRefreshUnavailable
	}
	if item == nil {
		return nil, nil
	}

	entity := stringAttribute(item["entityType"])
	if entity != "" && entity != "USER_SESSION" {
		return nil, nil
	}

	session := &appIdentity.RefreshSession{
		UserID:      stringAttribute(item["userId"]),
		SessionID:   stringAttribute(item["sessionId"]),
		FamilyID:    stringAttribute(item["familyId"]),
		RefreshHash: stringAttribute(item["refreshHash"]),
		AccessJTI:   stringAttribute(item["accessJti"]),
		Status:      stringAttribute(item["status"]),
		DeviceID:    stringAttribute(item["deviceId"]),
		DeviceName:  stringAttribute(item["deviceName"]),
		Platform:    stringAttribute(item["platform"]),
	}
	if session.UserID == "" || session.SessionID == "" {
		return nil, nil
	}
	if version, ok := attributeInt(item["version"]); ok {
		session.Version = version
	}
	if expires, ok := attributeTime(item); ok {
		session.ExpiresAt = expires
		session.HasExpiry = true
	}
	if created, ok := attributeISOTime(item["createdAt"]); ok {
		session.CreatedAt = created
	}
	if used, ok := attributeISOTime(item["lastUsedAt"]); ok {
		session.LastUsedAt = used
	}
	return session, nil
}

// RotateRefresh aplica la rotación atómica por compare-and-swap.
//
// Condiciones (todas obligatorias y en la misma transacción):
//  1. localizador presentado existe, ACTIVE y NO usado (acepta `used` ausente de E002 y
//     `used=false` de E003) -> consumo de un solo uso;
//  2. sesión existe, está ACTIVE, conserva la versión leída y su refreshHash es el
//     presentado -> nadie rotó antes que nosotros;
//  3. perfil `USER#<id>/PROFILE` existe y está ACTIVE -> la cuenta sigue habilitada;
//  4. localizador nuevo no existe -> el digest emitido es inédito.
//
// Un fallo de condición se traduce en ErrRefreshConflict (el llamador clasifica con una
// relectura). Un fallo de servicio se traduce en ErrRefreshUnavailable: nunca se confunden.
func (s *RefreshStore) RotateRefresh(ctx context.Context, rotation appIdentity.RefreshRotation) error {
	if s == nil || s.client == nil || s.table == "" {
		return appIdentity.ErrRefreshUnavailable
	}
	if rotation.OldDigest == "" || rotation.NewDigest == "" || rotation.NewDigest == rotation.OldDigest {
		return appIdentity.ErrRefreshUnavailable
	}

	now := rotation.Now.UTC()
	nowISO := now.Format(time.RFC3339)
	sessionPK, sessionSK := domainDynamo.UserSessionKeys(rotation.UserID, rotation.SessionID)
	userPK, userSK := domainDynamo.UserKeys(rotation.UserID)
	oldPK, oldSK := domainDynamo.RefreshKeys(rotation.OldDigest)
	newPK, newSK := domainDynamo.RefreshKeys(rotation.NewDigest)

	newLocator := map[string]types.AttributeValue{
		"PK":         stringAttr(newPK),
		"SK":         stringAttr(newSK),
		"entityType": stringAttr("REFRESH_LOOKUP"),
		"userId":     stringAttr(rotation.UserID),
		"sessionId":  stringAttr(rotation.SessionID),
		"familyId":   stringAttr(rotation.FamilyID),
		"status":     stringAttr(appIdentity.StatusActive),
		"createdAt":  stringAttr(nowISO),
		"used":       boolAttr(false),
	}
	// El localizador nuevo hereda la expiración absoluta de la sesión: rotar no extiende
	// la vida de la familia. Sin expiración conocida no se inventa una.
	if !rotation.ExpiresAt.IsZero() {
		expira := rotation.ExpiresAt.UTC()
		newLocator["expiresAt"] = numberAttr(expira.Unix())
		newLocator["expiresAtISO"] = stringAttr(expira.Format(time.RFC3339))
	}

	items := []types.TransactWriteItem{
		// 1. El token presentado se consume de forma durable: base de la detección de
		//    reutilización. El localizador NO se borra; el TTL de la tabla lo purga.
		//
		//    Compatibilidad E002/E003: los localizadores creados por E002 no tienen el
		//    atributo `used`, mientras que los creados por E003 nacen con `used=false`.
		//    La condición acepta ambos estados y sigue rechazando `used=true`.
		{Update: &types.Update{
			TableName: aws.String(s.table),
			Key: map[string]types.AttributeValue{
				"PK": stringAttr(oldPK),
				"SK": stringAttr(oldSK),
			},
			UpdateExpression:    aws.String("SET #used = :used, usedAt = :now, consumedBy = :newDigest, #status = :consumed"),
			ConditionExpression: aws.String("attribute_exists(PK) AND #status = :active AND (attribute_not_exists(#used) OR #used = :false)"),
			ExpressionAttributeNames: map[string]string{
				"#used":   "used",
				"#status": "status",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":used":      boolAttr(true),
				":false":     boolAttr(false),
				":now":       stringAttr(nowISO),
				":newDigest": stringAttr(rotation.NewDigest),
				":consumed":  stringAttr(appIdentity.StatusConsumed),
				":active":    stringAttr(appIdentity.StatusActive),
			},
		}},
		// 2. La sesión autoritativa pasa al digest nuevo conservando su identidad,
		//    familia, dispositivo y fecha de expiración absoluta (no se extiende).
		{Update: &types.Update{
			TableName: aws.String(s.table),
			Key: map[string]types.AttributeValue{
				"PK": stringAttr(sessionPK),
				"SK": stringAttr(sessionSK),
			},
			UpdateExpression:    aws.String("SET refreshHash = :newHash, accessJti = :jti, lastUsedAt = :now, #version = :newVersion"),
			ConditionExpression: aws.String("attribute_exists(PK) AND #status = :active AND #version = :expected AND refreshHash = :oldHash"),
			ExpressionAttributeNames: map[string]string{
				"#status":  "status",
				"#version": "version",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":newHash":    stringAttr(rotation.NewDigest),
				":oldHash":    stringAttr(rotation.OldDigest),
				":jti":        stringAttr(rotation.NewAccessJTI),
				":now":        stringAttr(nowISO),
				":newVersion": numberAttr(rotation.NewVersion),
				":expected":   numberAttr(rotation.ExpectedVersion),
				":active":     stringAttr(appIdentity.StatusActive),
			},
		}},
		// 3. La cuenta propietaria debe seguir ACTIVE en el momento atómico de la
		//    rotación: cierra la carrera entre comprobar el perfil y escribir la sesión.
		//    Deshabilitar o eliminar el usuario impide cualquier renovación posterior.
		{ConditionCheck: &types.ConditionCheck{
			TableName: aws.String(s.table),
			Key: map[string]types.AttributeValue{
				"PK": stringAttr(userPK),
				"SK": stringAttr(userSK),
			},
			ConditionExpression: aws.String("attribute_exists(PK) AND #status = :active"),
			ExpressionAttributeNames: map[string]string{
				"#status": "status",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":active": stringAttr(appIdentity.StatusActive),
			},
		}},
		// 4. El localizador del token nuevo nace en la misma transacción.
		{Put: &types.Put{
			TableName:           aws.String(s.table),
			Item:                newLocator,
			ConditionExpression: aws.String("attribute_not_exists(PK)"),
		}},
	}

	_, err := s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: items})
	if err != nil {
		if isConditionalCancel(err) {
			return appIdentity.ErrRefreshConflict
		}
		return appIdentity.ErrRefreshUnavailable
	}
	return nil
}

// LoadUserProfile lee el perfil propietario (`USER#<id>/PROFILE`) con consistencia fuerte.
// Devuelve (nil, nil) cuando el perfil no existe: una cuenta eliminada no renueva tokens.
func (s *RefreshStore) LoadUserProfile(ctx context.Context, userID string) (*appIdentity.RefreshUser, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, appIdentity.ErrRefreshUnavailable
	}
	if strings.TrimSpace(userID) == "" {
		return nil, nil
	}
	pk, sk := domainDynamo.UserKeys(userID)
	item, err := s.getItem(ctx, pk, sk)
	if err != nil {
		return nil, appIdentity.ErrRefreshUnavailable
	}
	if item == nil {
		return nil, nil
	}
	if entity := stringAttribute(item["entityType"]); entity != "" && entity != "USER_PROFILE" {
		return nil, nil
	}
	perfil := &appIdentity.RefreshUser{
		UserID: stringAttribute(item["userId"]),
		Status: stringAttribute(item["status"]),
	}
	if perfil.UserID == "" {
		perfil.UserID = userID
	}
	return perfil, nil
}

// RevokeFamily revoca la sesión de la familia afectada y marca los localizadores
// conocidos. La sesión es la autoridad: cualquier rotación posterior exige `status=ACTIVE`.
//
// Modelo vigente (E002): cada login crea una familia con exactamente una sesión. Si una
// familia llegara a tener varias sesiones, la revocación completa exigiría un índice por
// `familyId`, que no existe; se registra como límite explícito en la trazabilidad de E003.
func (s *RefreshStore) RevokeFamily(ctx context.Context, revocation appIdentity.RefreshRevocation) error {
	if s == nil || s.client == nil || s.table == "" {
		return appIdentity.ErrRefreshUnavailable
	}
	if strings.TrimSpace(revocation.UserID) == "" || strings.TrimSpace(revocation.SessionID) == "" {
		return appIdentity.ErrRefreshUnavailable
	}

	nowISO := revocation.Now.UTC().Format(time.RFC3339)
	sessionPK, sessionSK := domainDynamo.UserSessionKeys(revocation.UserID, revocation.SessionID)
	motivo := appIdentity.RevokeReasonReplay

	sessionItem := types.TransactWriteItem{Update: &types.Update{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(sessionPK),
			"SK": stringAttr(sessionSK),
		},
		UpdateExpression:    aws.String("SET #status = :revoked, revokedAt = :now, revokeReason = :reason ADD #version :one"),
		ConditionExpression: aws.String("attribute_exists(PK)"),
		ExpressionAttributeNames: map[string]string{
			"#status":  "status",
			"#version": "version",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":revoked": stringAttr(appIdentity.StatusRevoked),
			":now":     stringAttr(nowISO),
			":reason":  stringAttr(motivo),
			":one":     numberAttr(1),
		},
	}}

	items := []types.TransactWriteItem{sessionItem}
	for _, digest := range uniqueDigests(revocation.PresentedDigest, revocation.CurrentDigest) {
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
				":reason":  stringAttr(motivo),
			},
		}})
	}

	_, err := s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: items})
	if err == nil {
		return nil
	}
	if !isConditionalCancel(err) {
		return appIdentity.ErrRefreshUnavailable
	}
	// Un localizador pudo ser purgado por el TTL entre la lectura y la transacción. La
	// sesión es la autoridad: se reintenta sólo con ella para no dejar viva la familia.
	if _, errRetry := s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{sessionItem},
	}); errRetry != nil {
		return appIdentity.ErrRefreshUnavailable
	}
	return nil
}

func (s *RefreshStore) getItem(ctx context.Context, pk, sk string) (map[string]types.AttributeValue, error) {
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

// isConditionalCancel distingue un fallo de condición (compare-and-swap perdido) de un
// fallo de servicio. Sólo el primero es un conflicto de concurrencia.
func isConditionalCancel(err error) bool {
	var canceled *types.TransactionCanceledException
	if !errors.As(err, &canceled) {
		return false
	}
	for _, reason := range canceled.CancellationReasons {
		if reason.Code == nil {
			continue
		}
		if *reason.Code == "ConditionalCheckFailed" {
			return true
		}
	}
	return false
}

func uniqueDigests(values ...string) []string {
	vistos := make(map[string]bool, len(values))
	salida := make([]string, 0, len(values))
	for _, value := range values {
		limpio := strings.TrimSpace(value)
		if limpio == "" || vistos[limpio] {
			continue
		}
		vistos[limpio] = true
		salida = append(salida, limpio)
	}
	return salida
}

// attributeInt lee un número entero (atributo Number).
func attributeInt(value types.AttributeValue) (int64, bool) {
	number, ok := value.(*types.AttributeValueMemberN)
	if !ok || number.Value == "" {
		return 0, false
	}
	parsed, err := strconv.ParseInt(number.Value, 10, 64)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

// attributeISOTime lee una marca RFC3339 almacenada como texto de legibilidad.
func attributeISOTime(value types.AttributeValue) (time.Time, bool) {
	texto := stringAttribute(value)
	if texto == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339, texto)
	if err != nil {
		return time.Time{}, false
	}
	return parsed.UTC(), true
}

// attributeTime lee la expiración lógica. Prioriza `expiresAt` Number (epoch seconds, que
// es también el TTL físico de la tabla) y recurre a `expiresAtISO` como respaldo.
func attributeTime(item map[string]types.AttributeValue) (time.Time, bool) {
	if seconds, ok := attributeInt(item["expiresAt"]); ok && seconds > 0 {
		return time.Unix(seconds, 0).UTC(), true
	}
	return attributeISOTime(item["expiresAtISO"])
}
