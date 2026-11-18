package dynamo

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	appProfile "github.com/nexumpr/Nexum-Backend/internal/application/profile"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
)

// ProfileUpdateAPI es el subconjunto del cliente DynamoDB que necesita E013. Se declara con
// la única operación de escritura: el compilador garantiza que esta ruta no puede leer,
// ejecutar `Scan` ni tocar otra clave que la que se le pasa.
type ProfileUpdateAPI interface {
	UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
}

// ProfileUpdateStore implementa appProfile.UpdateStore sobre la tabla única existente.
//
// Sólo escribe `USER#<id>/PROFILE`, la misma clave que ya escriben E001/E002/E006/E007 y que
// la política IAM vigente autoriza (`USER#*`). No crea tablas, índices, particiones ni
// claves nuevas y nunca usa `Scan`.
//
// La escritura es un CAS real: la condición exige que el item exista y que `version` sea la
// que el cliente recibió. El `UpdateExpression` sólo modifica `displayName`, `updatedAt` y
// `version`; el resto de atributos del perfil (email, passwordHash, emailVerified,
// sessionEpoch, status, privacy*) ni siquiera se mencionan, de modo que no pueden
// sobrescribirse por accidente.
type ProfileUpdateStore struct {
	client ProfileUpdateAPI
	table  string
}

// NewProfileUpdateStore construye el adaptador. Sin cliente o tabla, toda escritura falla
// cerrada (nunca se sustituye por memoria local).
func NewProfileUpdateStore(client ProfileUpdateAPI, table string) *ProfileUpdateStore {
	return &ProfileUpdateStore{client: client, table: strings.TrimSpace(table)}
}

// UpdateProfile aplica la modificación condicional de E013 y devuelve el item resultante.
//
// Devuelve ErrProfileCASFailed cuando la condición no se cumple (la versión cambió o el
// perfil no existe) y ErrProfileUnavailable ante cualquier otro fallo de DynamoDB. No hay
// escritura parcial: `UpdateItem` es atómico y `ReturnValues=ALL_NEW` entrega exactamente el
// estado posterior, que es el que se proyecta al contrato.
func (s *ProfileUpdateStore) UpdateProfile(ctx context.Context, input appProfile.UpdateInput) (*appProfile.Record, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, appProfile.ErrProfileUnavailable
	}
	userID := strings.TrimSpace(input.UserID)
	nombre := strings.TrimSpace(input.DisplayName)
	if userID == "" || nombre == "" || input.ExpectedVersion < 1 {
		return nil, appProfile.ErrProfileUnavailable
	}

	pk, sk := domainDynamo.UserKeys(userID)
	nowISO := input.Now.UTC().Format(time.RFC3339)

	condicion := "attribute_exists(PK) AND #version = :expected"
	actualizacion := "SET displayName = :name, updatedAt = :now ADD #version :one"
	valores := map[string]types.AttributeValue{
		":name":     stringAttr(nombre),
		":now":      stringAttr(nowISO),
		":one":      numberAttr(1),
		":expected": numberAttr(input.ExpectedVersion),
	}
	if !input.ProfileHasVersion {
		// Perfil heredado sin `version`: E012 lo expone como v1, así que el CAS exige su
		// ausencia y la escritura inicializa el contador. `:expected` no se declara porque
		// DynamoDB rechaza los valores de expresión no utilizados.
		condicion = "attribute_exists(PK) AND attribute_not_exists(#version)"
		actualizacion = "SET displayName = :name, updatedAt = :now, #version = :one"
		delete(valores, ":expected")
	}

	out, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(pk),
			"SK": stringAttr(sk),
		},
		UpdateExpression:          aws.String(actualizacion),
		ConditionExpression:       aws.String(condicion),
		ExpressionAttributeNames:  map[string]string{"#version": "version"},
		ExpressionAttributeValues: valores,
		ReturnValues:              types.ReturnValueAllNew,
	})
	if err != nil {
		if esFalloDeCondicion(err) {
			// La versión cambió o el perfil no existe: ninguna escritura se aplicó.
			return nil, appProfile.ErrProfileCASFailed
		}
		return nil, appProfile.ErrProfileUnavailable
	}
	if out == nil || len(out.Attributes) == 0 {
		// `ALL_NEW` siempre devuelve el item escrito; sin atributos no se puede cumplir el
		// contrato `UserProfile` y no se inventa una respuesta.
		return nil, appProfile.ErrProfileInternal
	}
	registro, err := profileRecordFromItem(out.Attributes, userID)
	if err != nil || registro == nil {
		return nil, appProfile.ErrProfileInternal
	}
	return registro, nil
}

// esFalloDeCondicion reconoce las dos formas con las que DynamoDB informa de una condición
// incumplida: `UpdateItem` devuelve ConditionalCheckFailedException y una transacción
// devuelve TransactionCanceledException con la razón por item.
func esFalloDeCondicion(err error) bool {
	var condicional *types.ConditionalCheckFailedException
	if errors.As(err, &condicional) {
		return true
	}
	return isConditionalCancel(err)
}
