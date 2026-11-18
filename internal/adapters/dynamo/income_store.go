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
	domainIdentity "github.com/nexumpr/Nexum-Backend/internal/domain/identity"
)

// IncomeAPI es el subconjunto del cliente DynamoDB que usan E024 y E025.
//
// La superficie es deliberadamente mínima: una lectura consistente y una escritura
// condicional. No existe `Scan`, de modo que el adaptador no puede enumerar la tabla ni
// siquiera por error (A-060 §2.5, AC-A-060-01).
type IncomeAPI interface {
	GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	UpdateItem(context.Context, *dynamodb.UpdateItemInput, ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
}

// IncomeStore implementa appIdentity.IncomeStore sobre la tabla única existente.
//
// Sólo usa `USER#<id>/INCOME` y `USER#<id>/PROFILE`, las mismas claves que ya escriben
// E001/E002 y que la política IAM vigente autoriza (`LeadingKeys` `USER#*`). No crea
// tablas, índices, particiones ni claves nuevas y nunca usa `Scan`.
type IncomeStore struct {
	client IncomeAPI
	table  string
}

// NewIncomeStore construye el adaptador. Sin cliente o sin tabla, todas las operaciones
// fallan cerradas: nunca se sustituyen por memoria local.
func NewIncomeStore(client IncomeAPI, table string) *IncomeStore {
	return &IncomeStore{client: client, table: strings.TrimSpace(table)}
}

// claveIngreso devuelve la PK/SK del ingreso base del titular.
func claveIngreso(userID string) (string, string) {
	pk, sk := domainDynamo.IncomeKeys(userID)
	return pk, sk
}

// LoadIncome lee `USER#<id>/INCOME` con consistencia fuerte.
//
// Devuelve (nil, nil) cuando el titular no tiene ingreso configurado o el item de esa clave
// no es un ingreso (clave reutilizada por otra entidad): E024 responde 404. Un error real de
// DynamoDB se propaga para que el servicio lo traduzca a 503 y nunca a «no configurado».
func (s *IncomeStore) LoadIncome(ctx context.Context, userID string) (*appIdentity.IncomeSnapshot, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, appIdentity.ErrIncomeUnavailable
	}
	clean := strings.TrimSpace(userID)
	if clean == "" {
		return nil, nil
	}

	pk, sk := claveIngreso(clean)
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(pk),
			"SK": stringAttr(sk),
		},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return nil, appIdentity.ErrIncomeUnavailable
	}
	if out == nil || len(out.Item) == 0 {
		return nil, nil
	}
	if entity := stringAttribute(out.Item["entityType"]); entity != "" && entity != "USER_INCOME" {
		return nil, nil
	}

	amountCents, okImporte := attributeInt(out.Item["amountCents"])
	if !okImporte {
		// Un item con la clave del ingreso pero sin importe no es un ingreso legible.
		return nil, nil
	}
	frequency := strings.TrimSpace(stringAttribute(out.Item["frequency"]))
	if frequency == "" {
		return nil, appIdentity.ErrIncomeInconsistent
	}

	// E001 escribe `version = 1` en el alta. Un item sin versión sólo puede preceder a ese
	// contrato; se interpreta como la versión inicial para no bloquear su primera escritura.
	version, okVersion := attributeInt(out.Item["version"])
	if !okVersion {
		version = 1
	}

	return &appIdentity.IncomeSnapshot{
		AmountCents: amountCents,
		Frequency:   domainIdentity.Frequency(frequency),
		Version:     version,
	}, nil
}

// ProfileExists comprueba que el titular existe y está ACTIVE.
//
// E025 la usa antes de escribir para no crear un ingreso huérfano de una cuenta eliminada.
// La lectura es consistente y sólo de `PROFILE`: esta unidad no modifica el perfil ni la
// autenticación.
func (s *IncomeStore) ProfileExists(ctx context.Context, userID string) (bool, error) {
	if s == nil || s.client == nil || s.table == "" {
		return false, appIdentity.ErrIncomeUnavailable
	}
	clean := strings.TrimSpace(userID)
	if clean == "" {
		return false, nil
	}

	pk, sk := domainDynamo.UserKeys(clean)
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(pk),
			"SK": stringAttr(sk),
		},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return false, appIdentity.ErrIncomeUnavailable
	}
	if out == nil || len(out.Item) == 0 {
		return false, nil
	}
	if entity := stringAttribute(out.Item["entityType"]); entity != "" && entity != "USER_PROFILE" {
		return false, nil
	}
	if status := stringAttribute(out.Item["status"]); status != "" && status != appIdentity.StatusActive {
		return false, nil
	}
	return true, nil
}

// StoreIncome ejecuta la escritura condicional de E025 en UNA sola petición.
//
// La condición implementa el CAS por versión del contrato:
//
//   - versión esperada > 1: `#version = :expected`. El item debe existir con exactamente esa
//     versión; un item ausente también produce 412 (no se crea un ingreso pidiendo una
//     versión que nunca se leyó).
//   - versión esperada == 1: `attribute_not_exists(PK) OR attribute_not_exists(#version) OR
//     #version = :expected`. Con ETag `"v1"` se CREA el item cuando no existe (E025 §3) y se
//     actualiza cuando ya está en la versión 1. La rama `attribute_not_exists(#version)`
//     cubre un item heredado sin versión, que E001 no produce, para que no quede bloqueado
//     en 412 de forma permanente.
//
// La expresión usa `ADD #version :one`, de modo que el alta crea `version = 1` y una
// actualización incrementa la versión existente. Todos los valores de expresión declarados
// se usan en la condición o en la actualización: DynamoDB rechaza valores no utilizados.
func (s *IncomeStore) StoreIncome(ctx context.Context, write appIdentity.IncomeStoreWrite) (*appIdentity.IncomeSnapshot, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, appIdentity.ErrIncomeUnavailable
	}
	userID := strings.TrimSpace(write.UserID)
	if userID == "" || write.AmountCents <= 0 || write.Frequency == "" || write.ExpectedVersion < 1 {
		return nil, appIdentity.ErrIncomeInvalidRequest
	}

	pk, sk := claveIngreso(userID)
	condicion := "#version = :expected"
	if write.ExpectedVersion == 1 {
		condicion = "attribute_not_exists(PK) OR attribute_not_exists(#version) OR #version = :expected"
	}

	const actualizacion = "SET amountCents = :amount, currency = :currency, frequency = :frequency, " +
		"monthlyEquivalentCents = :monthly, updatedAt = :updated ADD #version :one"

	out, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(pk),
			"SK": stringAttr(sk),
		},
		UpdateExpression:    aws.String(actualizacion),
		ConditionExpression: aws.String(condicion),
		ExpressionAttributeNames: map[string]string{
			"#version": "version",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":amount":    numberAttr(write.AmountCents),
			":currency":  stringAttr(appIdentity.RegisterCurrency),
			":frequency": stringAttr(write.Frequency.String()),
			":monthly":   numberAttr(write.MonthlyEquivalentCents),
			":updated":   stringAttr(write.Now.UTC().Format(time.RFC3339)),
			":one":       numberAttr(1),
			":expected":  numberAttr(write.ExpectedVersion),
		},
		ReturnValues: types.ReturnValueAllNew,
	})
	if err != nil {
		var condicional *types.ConditionalCheckFailedException
		if errors.As(err, &condicional) {
			// La versión vigente no es la esperada: NADA se escribió.
			return nil, appIdentity.ErrIncomeVersionConflict
		}
		var conflicto *types.TransactionConflictException
		if errors.As(err, &conflicto) {
			// Escritura concurrente sobre el mismo item: reintentar es seguro.
			return nil, appIdentity.ErrIncomeConflict
		}
		return nil, appIdentity.ErrIncomeUnavailable
	}
	if out == nil || len(out.Attributes) == 0 {
		return nil, appIdentity.ErrIncomeUnavailable
	}

	version, okVersion := attributeInt(out.Attributes["version"])
	if !okVersion || version < 1 {
		return nil, appIdentity.ErrIncomeUnavailable
	}
	// Comprobación de integridad de la escritura: `ReturnValues = ALL_NEW` debe devolver
	// exactamente los valores recién escritos.
	if amountCents, ok := attributeInt(out.Attributes["amountCents"]); !ok || amountCents != write.AmountCents {
		return nil, appIdentity.ErrIncomeInconsistent
	}
	if stored := strings.TrimSpace(stringAttribute(out.Attributes["frequency"])); stored != write.Frequency.String() {
		return nil, appIdentity.ErrIncomeInconsistent
	}

	return &appIdentity.IncomeSnapshot{
		AmountCents: write.AmountCents,
		Frequency:   write.Frequency,
		Version:     version,
	}, nil
}
