package dynamo

import (
	"context"
	"errors"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	appBudget "github.com/nexumpr/Nexum-Backend/internal/application/budgetset"
)

type budgetSetPutAPI interface {
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
}

// ReplaceSet escribe el conjunto completo con CAS por versión. expectedVersion=0 exige que el
// ítem no exista; cualquier otra versión exige coincidencia exacta.
func (s *BudgetSetStore) ReplaceSet(ctx context.Context, userID string, set *appBudget.Set, expectedVersion int64) error {
	if s == nil || s.client == nil || s.table == "" {
		return appBudget.ErrUnavailable
	}
	api, ok := s.client.(budgetSetPutAPI)
	if !ok {
		return appBudget.ErrUnavailable
	}
	if set == nil {
		return appBudget.ErrUnavailable
	}
	item, ok := BudgetSetItem(userID, set.Period, set.Currency, set.Version, set.Items, set.UpdatedAt)
	if !ok {
		return appBudget.ErrInconsistent
	}
	entrada := &dynamodb.PutItemInput{
		TableName: aws.String(s.table),
		Item:      item,
	}
	if expectedVersion == 0 {
		entrada.ConditionExpression = aws.String("attribute_not_exists(PK)")
	} else {
		entrada.ConditionExpression = aws.String("#version = :expected")
		entrada.ExpressionAttributeNames = map[string]string{"#version": "version"}
		entrada.ExpressionAttributeValues = map[string]types.AttributeValue{
			":expected": numberAttr(expectedVersion),
		}
	}
	if _, err := api.PutItem(ctx, entrada); err != nil {
		var condicional *types.ConditionalCheckFailedException
		if errors.As(err, &condicional) {
			return appBudget.ErrVersionConflict
		}
		if strings.Contains(err.Error(), "ConditionalCheckFailed") {
			return appBudget.ErrVersionConflict
		}
		return appBudget.ErrUnavailable
	}
	return nil
}
