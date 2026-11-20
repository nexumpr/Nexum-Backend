package dynamo

import (
	"context"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	appBudget "github.com/nexumpr/Nexum-Backend/internal/application/budgetset"
	appUsage "github.com/nexumpr/Nexum-Backend/internal/application/budgetusage"
	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
)

// BudgetUsageAPI es el subconjunto del cliente DynamoDB para E050. No incluye Scan.
type BudgetUsageAPI interface {
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
}

// BudgetUsageStore implementa `budgetusage.Store`.
type BudgetUsageStore struct {
	client BudgetUsageAPI
	table  string
	budget *BudgetSetStore
}

// NewBudgetUsageStore construye el adaptador.
func NewBudgetUsageStore(client BudgetUsageAPI, table string) *BudgetUsageStore {
	return &BudgetUsageStore{client: client, table: strings.TrimSpace(table), budget: NewBudgetSetStore(client, table)}
}

func (s *BudgetUsageStore) LoadTimezone(ctx context.Context, userID string) (string, error) {
	if s == nil || s.client == nil || s.table == "" {
		return "", appUsage.ErrUnavailable
	}
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(domainDynamo.PrefixUser + strings.TrimSpace(userID)),
			"SK": stringAttr("PREFERENCES"),
		},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return "", appUsage.ErrUnavailable
	}
	if out == nil || len(out.Item) == 0 {
		return "", nil
	}
	if tz := strings.TrimSpace(stringAttribute(out.Item["timezone"])); tz != "" {
		return tz, nil
	}
	return strings.TrimSpace(stringAttribute(out.Item["timeZone"])), nil
}

func (s *BudgetUsageStore) LoadSet(ctx context.Context, userID, period string) (*appBudget.Set, error) {
	if s == nil || s.budget == nil {
		return nil, appUsage.ErrUnavailable
	}
	set, err := s.budget.LoadSet(ctx, userID, period)
	if err != nil {
		if err == appBudget.ErrUnavailable {
			return nil, appUsage.ErrUnavailable
		}
		if err == appBudget.ErrInconsistent {
			return nil, appUsage.ErrInconsistent
		}
		return nil, err
	}
	return set, nil
}

// AggregateExpenses suma los movimientos `expense` por categoría en [from, to).
func (s *BudgetUsageStore) AggregateExpenses(ctx context.Context, userID string, from, to time.Time) (map[string]int64, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, appUsage.ErrUnavailable
	}
	pk := domainDynamo.PrefixUser + strings.TrimSpace(userID)
	gastos := map[string]int64{}
	var start map[string]types.AttributeValue
	for {
		entrada := &dynamodb.QueryInput{
			TableName:              aws.String(s.table),
			KeyConditionExpression: aws.String("PK = :pk AND #sk >= :from AND #sk < :to"),
			ExpressionAttributeNames: map[string]string{
				"#sk": "SK",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":pk":   stringAttr(pk),
				":from": stringAttr("MOVEMENT#" + appMovements.SortableTime(from)),
				":to":   stringAttr("MOVEMENT#" + appMovements.SortableTime(to)),
			},
			ConsistentRead: aws.Bool(true),
		}
		if len(start) > 0 {
			entrada.ExclusiveStartKey = start
		}
		out, err := s.client.Query(ctx, entrada)
		if err != nil {
			return nil, appUsage.ErrUnavailable
		}
		for _, item := range out.Items {
			tipo := strings.TrimSpace(stringAttribute(item["type"]))
			centavos, ok := attributeInt(item["amountCents"])
			if !ok || tipo == "" {
				return nil, appUsage.ErrInconsistent
			}
			if centavos < 0 {
				return nil, appUsage.ErrInconsistent
			}
			if tipo != "expense" {
				continue
			}
			categoria := strings.TrimSpace(stringAttribute(item["categoryId"]))
			if categoria == "" {
				return nil, appUsage.ErrInconsistent
			}
			suma, ok := appUsage.SumarCentavos(gastos[categoria], centavos)
			if !ok {
				return nil, appUsage.ErrOverflow
			}
			gastos[categoria] = suma
		}
		if len(out.LastEvaluatedKey) == 0 {
			return gastos, nil
		}
		start = out.LastEvaluatedKey
	}
}
