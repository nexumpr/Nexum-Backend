package dynamo

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	appProfile "github.com/nexumpr/Nexum-Backend/internal/application/profile"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
)

// ProfileAPI es el subconjunto del cliente DynamoDB que necesita E012. Se declara con la
// única operación de lectura: el compilador garantiza que la ruta no puede escribir ni
// ejecutar `Scan`.
type ProfileAPI interface {
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
}

// ProfileStore implementa appProfile.Store sobre la tabla única existente.
//
// Sólo lee `USER#<id>/PROFILE`, la misma clave que ya escriben E001/E002 y que la política
// IAM vigente autoriza (`USER#*`). No crea tablas, índices ni particiones y no usa `Scan`.
type ProfileStore struct {
	client ProfileAPI
	table  string
}

// NewProfileStore construye el adaptador. Sin cliente o tabla, toda lectura falla cerrada
// (nunca se sustituye por memoria local).
func NewProfileStore(client ProfileAPI, table string) *ProfileStore {
	return &ProfileStore{client: client, table: strings.TrimSpace(table)}
}

// LoadProfile lee `USER#<id>/PROFILE` con consistencia fuerte.
//
// Devuelve (nil, nil) cuando la clave no existe o el item de esa clave no es un perfil de
// usuario: una cuenta eliminada o una clave reutilizada por otra entidad no se presenta
// como perfil. Los atributos internos (passwordHash, sessionEpoch, privacy*) no se exponen:
// sólo se copian los seis valores del contrato.
func (s *ProfileStore) LoadProfile(ctx context.Context, userID string) (*appProfile.Record, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, appProfile.ErrProfileUnavailable
	}
	if strings.TrimSpace(userID) == "" {
		return nil, appProfile.ErrProfileUnauthorized
	}

	pk, sk := domainDynamo.UserKeys(userID)
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(pk),
			"SK": stringAttr(sk),
		},
		// Consistencia fuerte: la versión que se devuelve es la que el cliente usará en
		// `If-Match`; una réplica retrasada produciría 412 espurios.
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return nil, appProfile.ErrProfileUnavailable
	}
	if out == nil || len(out.Item) == 0 {
		return nil, nil
	}
	return profileRecordFromItem(out.Item, userID)
}

// profileRecordFromItem reduce el item de DynamoDB a la fila de aplicación. Rechaza items
// de otra entidad en la misma clave.
func profileRecordFromItem(item map[string]types.AttributeValue, userID string) (*appProfile.Record, error) {
	if entity := stringAttribute(item["entityType"]); entity != "" && entity != "USER_PROFILE" {
		return nil, nil
	}

	record := &appProfile.Record{
		UserID:        stringAttribute(item["userId"]),
		Email:         stringAttribute(item["email"]),
		DisplayName:   stringAttribute(item["displayName"]),
		EmailVerified: boolAttribute(item["emailVerified"]),
	}
	if record.UserID == "" {
		record.UserID = userID
	}
	if created, ok := parseSessionTime(stringAttribute(item["createdAt"])); ok {
		record.CreatedAt = created
		record.HasCreatedAt = true
	}
	if version, ok := attributeInt(item["version"]); ok && version > 0 {
		record.Version = version
		record.HasVersion = true
	}
	return record, nil
}

// boolAttribute interpreta un atributo booleano; un valor ausente o de otro tipo se lee
// como false (los items escritos por E001 siempre lo incluyen).
func boolAttribute(value types.AttributeValue) bool {
	if flag, ok := value.(*types.AttributeValueMemberBOOL); ok {
		return flag.Value
	}
	return false
}
