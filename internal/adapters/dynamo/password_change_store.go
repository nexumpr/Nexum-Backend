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

// PasswordChangeAPI es el subconjunto del cliente DynamoDB que usa E007: lectura consistente
// del perfil y una transacción atómica de una sola acción.
type PasswordChangeAPI interface {
	GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	TransactWriteItems(context.Context, *dynamodb.TransactWriteItemsInput, ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error)
}

// PasswordChangeStore implementa appIdentity.PasswordChangeStore sobre la tabla única
// existente. Sólo toca `USER#<id>/PROFILE`, la misma partición que ya escriben E001/E002 y
// E006, y que la política IAM vigente ya autoriza (`USER#*`). No crea tablas, índices,
// particiones ni claves nuevas y nunca usa Scan.
type PasswordChangeStore struct {
	client PasswordChangeAPI
	table  string
}

// NewPasswordChangeStore construye el adaptador. Sin cliente o tabla, todas las operaciones
// fallan cerradas (nunca se sustituyen por memoria local).
func NewPasswordChangeStore(client PasswordChangeAPI, table string) *PasswordChangeStore {
	return &PasswordChangeStore{client: client, table: strings.TrimSpace(table)}
}

// LoadPasswordChangeProfile lee `USER#<id>/PROFILE` con consistencia fuerte.
//
// Devuelve (nil, nil) cuando el perfil no existe o el ítem de esa clave no es un perfil de
// usuario: una cuenta eliminada o una clave reutilizada por otra entidad no puede cambiar su
// contraseña.
func (s *PasswordChangeStore) LoadPasswordChangeProfile(ctx context.Context, userID string) (*appIdentity.PasswordChangeProfile, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, appIdentity.ErrPasswordChangeUnavailable
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
		return nil, appIdentity.ErrPasswordChangeUnavailable
	}
	if out == nil || len(out.Item) == 0 {
		return nil, nil
	}
	if entity := stringAttribute(out.Item["entityType"]); entity != "" && entity != "USER_PROFILE" {
		return nil, nil
	}

	perfil := &appIdentity.PasswordChangeProfile{
		UserID:       stringAttribute(out.Item["userId"]),
		Status:       stringAttribute(out.Item["status"]),
		PasswordHash: stringAttribute(out.Item["passwordHash"]),
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

// ApplyPasswordChange ejecuta la transacción atómica de E007: UNA sola acción Update sobre
// `USER#<id>/PROFILE`.
//
//   - nuevo `passwordHash` Argon2id,
//   - `passwordChangedAt` en RFC3339,
//   - `sessionEpoch` = época leída + 1 (A6): invalida todos los access tokens y refresh
//     tokens emitidos antes del cambio, incluida la sesión que ejecutó la petición,
//   - `version` incrementada para el CAS de los rotadores existentes.
//
// La condición exige perfil existente, estado ACTIVE, la versión leída y la época leída (o
// su ausencia, para perfiles anteriores a A6). Cualquier desalineación cancela la escritura
// completa: la contraseña anterior sigue siendo la vigente.
func (s *PasswordChangeStore) ApplyPasswordChange(ctx context.Context, apply appIdentity.PasswordChangeApplication) error {
	if s == nil || s.client == nil || s.table == "" {
		return appIdentity.ErrPasswordChangeUnavailable
	}
	if strings.TrimSpace(apply.UserID) == "" || strings.TrimSpace(apply.PasswordHash) == "" {
		return appIdentity.ErrPasswordChangeUnavailable
	}

	nowISO := apply.Now.UTC().Format(time.RFC3339)
	userPK, userSK := domainDynamo.UserKeys(apply.UserID)

	condicion := "attribute_exists(PK) AND #status = :active AND #version = :expected AND (attribute_not_exists(#epoch) OR #epoch = :expectedEpoch)"
	actualizacion := "SET passwordHash = :hash, passwordChangedAt = :now, sessionEpoch = :nextEpoch ADD #version :one"
	valores := map[string]types.AttributeValue{
		":hash":          stringAttr(apply.PasswordHash),
		":now":           stringAttr(nowISO),
		":one":           numberAttr(1),
		":active":        stringAttr(appIdentity.StatusActive),
		":expected":      numberAttr(apply.ExpectedVersion),
		":expectedEpoch": numberAttr(apply.ExpectedSessionEpoch),
		":nextEpoch":     numberAttr(apply.NextSessionEpoch),
	}
	if !apply.HasVersion {
		// Perfil legado sin `version`: no existe rotador previo que pueda perder un CAS, así
		// que la condición exige su ausencia y la transacción la inicializa a 1.
		condicion = "attribute_exists(PK) AND #status = :active AND attribute_not_exists(#version) AND (attribute_not_exists(#epoch) OR #epoch = :expectedEpoch)"
		actualizacion = "SET passwordHash = :hash, passwordChangedAt = :now, sessionEpoch = :nextEpoch, #version = :one"
		// `:expected` no se declara: DynamoDB rechaza valores de expresión no utilizados.
		delete(valores, ":expected")
	}

	items := []types.TransactWriteItem{
		{Update: &types.Update{
			TableName: aws.String(s.table),
			Key: map[string]types.AttributeValue{
				"PK": stringAttr(userPK),
				"SK": stringAttr(userSK),
			},
			UpdateExpression:    aws.String(actualizacion),
			ConditionExpression: aws.String(condicion),
			ExpressionAttributeNames: map[string]string{
				"#status":  "status",
				"#version": "version",
				"#epoch":   "sessionEpoch",
			},
			ExpressionAttributeValues: valores,
		}},
	}

	_, err := s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: items})
	if err == nil {
		return nil
	}
	if isConditionalCancel(err) {
		// La versión o la época del perfil cambió entre la lectura y la escritura, o la
		// cuenta dejó de estar ACTIVE. Ninguna escritura se aplicó.
		return appIdentity.ErrPasswordChangeConflict
	}
	return appIdentity.ErrPasswordChangeUnavailable
}
