package dynamo

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	domainDeletion "github.com/nexumpr/Nexum-Backend/internal/domain/deletion"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
)

// EmailVerificationAPI es el subconjunto del cliente DynamoDB que usan E010/E011: lectura
// consistente del perfil y del token, escritura condicional del token y una transaccion
// atomica de dos acciones.
type EmailVerificationAPI interface {
	GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	PutItem(context.Context, *dynamodb.PutItemInput, ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	TransactWriteItems(context.Context, *dynamodb.TransactWriteItemsInput, ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error)
}

// EmailVerificationStore implementa appIdentity.EmailVerificationStore (E010) y
// appIdentity.EmailVerificationConfirmStore (E011) sobre la tabla unica existente.
//
// Usa el namespace IAM ya autorizado `ONE_USE#*`, separado de `RESET#` de E005/E006: el
// proposito es distinto y un token de recuperacion no puede verificar un correo.
type EmailVerificationStore struct {
	client EmailVerificationAPI
	table  string
}

// NewEmailVerificationStore construye el adaptador. Sin cliente o tabla, todas las
// operaciones fallan cerradas (nunca se sustituyen por memoria local).
func NewEmailVerificationStore(client EmailVerificationAPI, table string) *EmailVerificationStore {
	return &EmailVerificationStore{client: client, table: strings.TrimSpace(table)}
}

// claveOneUse devuelve la clave canonica del token de un solo uso.
func claveOneUse(tokenHash string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"PK": stringAttr("ONE_USE#" + tokenHash),
		"SK": stringAttr("VERIFY"),
	}
}

// LoadEmailVerificationProfile lee `USER#<id>/PROFILE` con consistencia fuerte. Devuelve
// (nil, nil) cuando el perfil no existe: una cuenta eliminada no puede verificar su correo.
func (s *EmailVerificationStore) LoadEmailVerificationProfile(ctx context.Context, userID string) (*appIdentity.EmailVerificationProfile, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, appIdentity.ErrEmailVerificationNoDisponible
	}
	if strings.TrimSpace(userID) == "" {
		return nil, nil
	}
	pk, sk := domainDynamo.UserKeys(userID)
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(pk),
			"SK": stringAttr(sk),
		},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return nil, appIdentity.ErrEmailVerificationNoDisponible
	}
	if out == nil || len(out.Item) == 0 {
		return nil, nil
	}
	if entity := stringAttribute(out.Item["entityType"]); entity != "" && entity != "USER_PROFILE" {
		return nil, nil
	}
	perfil := &appIdentity.EmailVerificationProfile{
		UserID: stringAttribute(out.Item["userId"]),
		Email:  stringAttribute(out.Item["email"]),
		Status: stringAttribute(out.Item["status"]),
	}
	if perfil.UserID == "" {
		perfil.UserID = userID
	}
	if value, ok := out.Item["emailVerified"].(*types.AttributeValueMemberBOOL); ok {
		perfil.EmailVerified = value.Value
	}
	if version, ok := attributeInt(out.Item["version"]); ok {
		perfil.Version = version
		perfil.HasVersion = true
	}
	return perfil, nil
}

// SaveEmailVerificationToken escribe `ONE_USE#<sha256(token)>/VERIFY` con
// `attribute_not_exists(PK)` y, en la MISMA transacción, el puntero de purga
// `USER#<sub>/TOKENPTR#<sha256(token)>`: nunca sobrescribe un token existente, sólo persiste
// el hash y la purga puede borrar el token global sin `Scan` si el titular se da de baja.
func (s *EmailVerificationStore) SaveEmailVerificationToken(ctx context.Context, token appIdentity.EmailVerificationToken) error {
	if s == nil || s.client == nil || s.table == "" {
		return appIdentity.ErrEmailVerificationNoDisponible
	}
	if strings.TrimSpace(token.TokenHash) == "" || strings.TrimSpace(token.UserID) == "" {
		return appIdentity.ErrEmailVerificationNoDisponible
	}
	item := claveOneUse(token.TokenHash)
	item["entityType"] = stringAttr(appIdentity.EmailVerificationEntityType)
	item["purpose"] = stringAttr(appIdentity.EmailVerificationPurpose)
	item["userId"] = stringAttr(token.UserID)
	item["email"] = stringAttr(token.Email)
	item["createdAt"] = stringAttr(token.CreatedAt.UTC().Format(time.RFC3339))
	// TTL fisico de DynamoDB: expira el item sin intervencion de la aplicacion.
	item["expiresAt"] = numberAttr(token.ExpiresAt.UTC().Unix())
	// Copia legible para diagnostico; la autoridad de caducidad es expiresAt (Number).
	item["expiresAtISO"] = stringAttr(token.ExpiresAt.UTC().Format(time.RFC3339))
	item["used"] = boolAttr(false)

	acciones := []types.TransactWriteItem{
		{Put: &types.Put{
			TableName:           aws.String(s.table),
			Item:                item,
			ConditionExpression: aws.String("attribute_not_exists(PK)"),
		}},
	}
	puntero, ok := punteroDeToken(token.UserID, domainDeletion.TokenFamilyOneUse, token.TokenHash, token.ExpiresAt, token.CreatedAt)
	if !ok {
		// Sin una clave de token o de titular válida no se crea nada: un token sin puntero
		// sería invisible para la purga y sobreviviría por TTL sin poder borrarse.
		return appIdentity.ErrEmailVerificationNoDisponible
	}
	acciones = append(acciones, types.TransactWriteItem{Put: &types.Put{
		TableName: aws.String(s.table),
		Item:      puntero,
	}})

	_, err := s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: acciones})
	if err != nil {
		if transactionItemFailed(err, 0) {
			return appIdentity.ErrEmailVerificationTokenDuplicado
		}
		return appIdentity.ErrEmailVerificationNoDisponible
	}
	return nil
}

// LoadEmailVerificationToken resuelve `ONE_USE#<sha256(token)>/VERIFY` con consistencia
// fuerte. Devuelve (nil, nil) cuando no existe o el TTL ya lo purgo: el llamador responde 400.
func (s *EmailVerificationStore) LoadEmailVerificationToken(ctx context.Context, tokenHash string) (*appIdentity.EmailVerificationTokenRecord, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, appIdentity.ErrEmailVerificationConfirmNoDisponible
	}
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      aws.String(s.table),
		Key:            claveOneUse(tokenHash),
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return nil, appIdentity.ErrEmailVerificationConfirmNoDisponible
	}
	if out == nil || len(out.Item) == 0 {
		return nil, nil
	}
	registro := &appIdentity.EmailVerificationTokenRecord{
		UserID:     stringAttribute(out.Item["userId"]),
		Email:      stringAttribute(out.Item["email"]),
		Purpose:    stringAttribute(out.Item["purpose"]),
		EntityType: stringAttribute(out.Item["entityType"]),
	}
	if value, ok := out.Item["used"].(*types.AttributeValueMemberBOOL); ok {
		registro.Used = value.Value
	}
	if value, ok := out.Item["expiresAt"].(*types.AttributeValueMemberN); ok {
		if unix, errParse := strconv.ParseInt(value.Value, 10, 64); errParse == nil {
			registro.ExpiresAt = time.Unix(unix, 0).UTC()
			registro.HasExpiry = true
		}
	}
	return registro, nil
}

// ConfirmEmailVerification ejecuta la transaccion atomica de E011:
//
//  1. `USER#<id>/PROFILE`: `emailVerified=true`, `emailVerifiedAt` y `version` incrementada,
//     condicionado a estado ACTIVE, version vigente y correo aun no verificado.
//  2. `ONE_USE#<sha256(token)>/VERIFY`: `used=true` y `usedAt`, condicionado a que siga sin
//     utilizarse y no haya expirado.
//
// Si cualquiera de las dos condiciones falla, TransactWriteItems cancela las dos acciones: el
// correo no queda verificado sin consumir el token ni viceversa.
func (s *EmailVerificationStore) ConfirmEmailVerification(ctx context.Context, confirmation appIdentity.EmailVerificationConfirmation) error {
	if s == nil || s.client == nil || s.table == "" {
		return appIdentity.ErrEmailVerificationConfirmNoDisponible
	}
	if strings.TrimSpace(confirmation.UserID) == "" || strings.TrimSpace(confirmation.TokenHash) == "" {
		return appIdentity.ErrEmailVerificationConfirmNoDisponible
	}

	nowISO := confirmation.Now.UTC().Format(time.RFC3339)
	userPK, userSK := domainDynamo.UserKeys(confirmation.UserID)

	condicionPerfil := "attribute_exists(PK) AND #status = :active AND #version = :expected AND (attribute_not_exists(#verified) OR #verified = :false)"
	actualizacionPerfil := "SET emailVerified = :true, emailVerifiedAt = :now ADD #version :one"
	valoresPerfil := map[string]types.AttributeValue{
		":true":     boolAttr(true),
		":false":    boolAttr(false),
		":now":      stringAttr(nowISO),
		":one":      numberAttr(1),
		":active":   stringAttr(appIdentity.StatusActive),
		":expected": numberAttr(confirmation.ExpectedVersion),
	}
	if !confirmation.HasVersion {
		// Perfil legado sin version: no hay rotador previo que pueda perder un CAS, asi que
		// la condicion exige su ausencia y la transaccion la inicializa a 1.
		condicionPerfil = "attribute_exists(PK) AND #status = :active AND attribute_not_exists(#version) AND (attribute_not_exists(#verified) OR #verified = :false)"
		actualizacionPerfil = "SET emailVerified = :true, emailVerifiedAt = :now, #version = :one"
		// `:expected` no se declara: DynamoDB rechaza valores de expresion no utilizados.
		delete(valoresPerfil, ":expected")
	}

	items := []types.TransactWriteItem{
		{Update: &types.Update{
			TableName: aws.String(s.table),
			Key: map[string]types.AttributeValue{
				"PK": stringAttr(userPK),
				"SK": stringAttr(userSK),
			},
			UpdateExpression:    aws.String(actualizacionPerfil),
			ConditionExpression: aws.String(condicionPerfil),
			ExpressionAttributeNames: map[string]string{
				"#status":   "status",
				"#version":  "version",
				"#verified": "emailVerified",
			},
			ExpressionAttributeValues: valoresPerfil,
		}},
		{Update: &types.Update{
			TableName:           aws.String(s.table),
			Key:                 claveOneUse(confirmation.TokenHash),
			UpdateExpression:    aws.String("SET #used = :true, usedAt = :now"),
			ConditionExpression: aws.String("attribute_exists(PK) AND (attribute_not_exists(#used) OR #used = :false) AND expiresAt > :nowEpoch"),
			ExpressionAttributeNames: map[string]string{
				"#used": "used",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":true":     boolAttr(true),
				":false":    boolAttr(false),
				":now":      stringAttr(nowISO),
				":nowEpoch": numberAttr(confirmation.Now.UTC().Unix()),
			},
		}},
	}

	_, err := s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: items})
	if err == nil {
		return nil
	}
	if !isConditionalCancel(err) {
		return appIdentity.ErrEmailVerificationConfirmNoDisponible
	}
	if transactionItemFailed(err, 1) {
		// El token dejo de ser consumible: otra solicitud lo uso o caduco entre la lectura y
		// la transaccion. Ninguna accion se aplico.
		return appIdentity.ErrEmailVerificationTokenUsado
	}
	// El perfil ya estaba verificado, cambio de version o dejo de estar ACTIVE.
	return appIdentity.ErrEmailVerificationConflicto
}
