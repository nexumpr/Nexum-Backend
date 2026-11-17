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
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
)

// ResetConfirmAPI es el subconjunto del cliente DynamoDB que usa E006: lectura consistente
// del token y del perfil, y una transacción atómica de dos acciones.
type ResetConfirmAPI interface {
	GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	TransactWriteItems(context.Context, *dynamodb.TransactWriteItemsInput, ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error)
}

// PasswordResetConfirmStore implementa appIdentity.ResetConfirmStore sobre la tabla única
// existente. Consume el MISMO ítem que escribe E005 (`RESET#<sha256(token)>` / `RESET`) y
// actualiza el perfil real (`USER#<id>` / `PROFILE`) en una única transacción de dos
// acciones sobre dos ítems distintos: DynamoDB no admite dos acciones sobre el mismo ítem.
type PasswordResetConfirmStore struct {
	client ResetConfirmAPI
	table  string
}

// NewPasswordResetConfirmStore construye el adaptador. Sin cliente o tabla, todas las
// operaciones fallan cerradas (nunca se sustituyen por memoria local).
func NewPasswordResetConfirmStore(client ResetConfirmAPI, table string) *PasswordResetConfirmStore {
	return &PasswordResetConfirmStore{client: client, table: strings.TrimSpace(table)}
}

// LoadResetToken resuelve `RESET#<sha256(token)>` con consistencia fuerte. Devuelve
// (nil, nil) cuando el token no existe o el TTL ya lo purgó: el llamador responde 400.
func (s *PasswordResetConfirmStore) LoadResetToken(ctx context.Context, tokenHash string, _ time.Time) (*appIdentity.ResetTokenRecord, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, appIdentity.ErrResetConfirmUnavailable
	}
	clave, _ := claveToken(tokenHash)
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      aws.String(s.table),
		Key:            clave,
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return nil, appIdentity.ErrResetConfirmUnavailable
	}
	if out == nil || len(out.Item) == 0 {
		return nil, nil
	}
	registro := &appIdentity.ResetTokenRecord{
		UserID: stringAttribute(out.Item["userId"]),
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

// LoadResetProfile lee `USER#<id>/PROFILE` con consistencia fuerte. Devuelve (nil, nil)
// cuando el perfil no existe: una cuenta eliminada no puede cambiar su contraseña.
func (s *PasswordResetConfirmStore) LoadResetProfile(ctx context.Context, userID string) (*appIdentity.ResetProfile, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, appIdentity.ErrResetConfirmUnavailable
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
		return nil, appIdentity.ErrResetConfirmUnavailable
	}
	if out == nil || len(out.Item) == 0 {
		return nil, nil
	}
	if entity := stringAttribute(out.Item["entityType"]); entity != "" && entity != "USER_PROFILE" {
		return nil, nil
	}
	perfil := &appIdentity.ResetProfile{
		UserID: stringAttribute(out.Item["userId"]),
		Status: stringAttribute(out.Item["status"]),
	}
	if perfil.UserID == "" {
		perfil.UserID = userID
	}
	if version, ok := attributeInt(out.Item["version"]); ok {
		perfil.Version = version
		perfil.HasVersion = true
	}
	if epoch, ok := attributeInt(out.Item["sessionEpoch"]); ok {
		perfil.SessionEpoch = epoch
	}
	return perfil, nil
}

// ApplyReset ejecuta la transacción atómica de E006:
//
//  1. `USER#<id>/PROFILE`: nuevo hash Argon2id, `passwordChangedAt`, `sessionEpoch`
//     incrementada (A6) y `version` incrementada, con CAS de versión vigente (o
//     `attribute_not_exists(version)` para perfiles legados).
//  2. `RESET#<sha256(token)>/RESET`: `used=true` y `usedAt`, condicionado a que siga sin
//     utilizarse y no haya expirado.
//
// Si cualquiera de las dos condiciones falla, TransactWriteItems cancela las dos acciones:
// no se modifica la contraseña ni se consume el token de forma parcial.
func (s *PasswordResetConfirmStore) ApplyReset(ctx context.Context, apply appIdentity.ResetApplication) error {
	if s == nil || s.client == nil || s.table == "" {
		return appIdentity.ErrResetConfirmUnavailable
	}
	if strings.TrimSpace(apply.UserID) == "" || strings.TrimSpace(apply.TokenHash) == "" || apply.PasswordHash == "" {
		return appIdentity.ErrResetConfirmUnavailable
	}

	nowISO := apply.Now.UTC().Format(time.RFC3339)
	userPK, userSK := domainDynamo.UserKeys(apply.UserID)
	clave, _ := claveToken(apply.TokenHash)

	// La condición incluye la época leída: aunque `version` cambiara, el incremento de
	// época nunca se aplica sobre una lectura obsoleta. El resultado es idéntico al
	// `if_not_exists(sessionEpoch, 0) + 1` del RFC-016 §17.3 y admite perfiles sin época.
	condicionPerfil := "attribute_exists(PK) AND #status = :active AND #version = :expected AND (attribute_not_exists(#epoch) OR #epoch = :expectedEpoch)"
	actualizacionPerfil := "SET passwordHash = :hash, passwordChangedAt = :now, sessionEpoch = :nextEpoch ADD #version :one"
	valoresPerfil := map[string]types.AttributeValue{
		":hash":          stringAttr(apply.PasswordHash),
		":now":           stringAttr(nowISO),
		":one":           numberAttr(1),
		":active":        stringAttr(appIdentity.StatusActive),
		":expected":      numberAttr(apply.ExpectedVersion),
		":expectedEpoch": numberAttr(apply.ExpectedSessionEpoch),
		":nextEpoch":     numberAttr(apply.NextSessionEpoch),
	}
	if !apply.HasVersion {
		// Perfil legado sin version: no hay rotador previo que pueda perder un CAS, así
		// que la condición exige su ausencia y la transacción la inicializa a 1.
		condicionPerfil = "attribute_exists(PK) AND #status = :active AND attribute_not_exists(#version) AND (attribute_not_exists(#epoch) OR #epoch = :expectedEpoch)"
		actualizacionPerfil = "SET passwordHash = :hash, passwordChangedAt = :now, sessionEpoch = :nextEpoch, #version = :one"
		// `:expected` no se declara: DynamoDB rechaza valores de expresión no utilizados.
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
				"#status":  "status",
				"#version": "version",
				"#epoch":   "sessionEpoch",
			},
			ExpressionAttributeValues: valoresPerfil,
		}},
		{Update: &types.Update{
			TableName:           aws.String(s.table),
			Key:                 clave,
			UpdateExpression:    aws.String("SET #used = :true, usedAt = :now"),
			ConditionExpression: aws.String("attribute_exists(PK) AND (attribute_not_exists(#used) OR #used = :false) AND expiresAt > :nowEpoch"),
			ExpressionAttributeNames: map[string]string{
				"#used": "used",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":true":     boolAttr(true),
				":false":    boolAttr(false),
				":now":      stringAttr(nowISO),
				":nowEpoch": numberAttr(apply.Now.UTC().Unix()),
			},
		}},
	}

	_, err := s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: items})
	if err == nil {
		return nil
	}
	if !isConditionalCancel(err) {
		return appIdentity.ErrResetConfirmUnavailable
	}
	if transactionItemFailed(err, 1) {
		// El token dejó de ser consumible: otra solicitud lo usó o caducó entre la
		// lectura y la transacción. Ninguna acción se aplicó.
		return appIdentity.ErrResetTokenUsado
	}
	// La versión del perfil cambió (otro reset concurrente): conflicto genérico.
	return appIdentity.ErrResetEnProceso
}
