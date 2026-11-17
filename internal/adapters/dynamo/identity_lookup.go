package dynamo

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
)

// IdentityLookup resuelve una cuenta a partir del correo reutilizando la reserva
// EMAIL#<sha256> que escribe E001. No usa Scan ni datos en memoria.
type IdentityLookup struct {
	client ResetAPI
	tabla  string
}

// NewIdentityLookup construye el buscador.
func NewIdentityLookup(client ResetAPI, tabla string) *IdentityLookup {
	return &IdentityLookup{client: client, tabla: tabla}
}

// BuscarPorEmail devuelve (userID, encontrada, error).
//
// Un fallo de DynamoDB se devuelve como error: el llamante no debe convertirlo en
// "cuenta inexistente", porque eso admitiria trabajo que no puede entregarse.
func (l *IdentityLookup) BuscarPorEmail(ctx context.Context, email string) (string, bool, error) {
	pk, sk := domainDynamo.EmailReservationKeys(email)
	out, err := l.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(l.tabla),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: sk},
		},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return "", false, fmt.Errorf("reserva de correo: %w", err)
	}
	if len(out.Item) == 0 {
		return "", false, nil
	}
	usuario, _ := out.Item["userId"].(*types.AttributeValueMemberS)
	if usuario == nil || usuario.Value == "" {
		return "", false, nil
	}
	return usuario.Value, true, nil
}
