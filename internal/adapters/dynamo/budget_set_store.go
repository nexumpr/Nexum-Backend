package dynamo

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	appBudget "github.com/nexumpr/Nexum-Backend/internal/application/budgetset"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
)

// BudgetSetAPI es el subconjunto del cliente DynamoDB para E048. No incluye Scan.
type BudgetSetAPI interface {
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
}

// BudgetSetStore implementa `budgetset.Store` sobre `BUDGETSET#<period>`.
type BudgetSetStore struct {
	client BudgetSetAPI
	table  string
}

// NewBudgetSetStore construye el adaptador.
func NewBudgetSetStore(client BudgetSetAPI, table string) *BudgetSetStore {
	return &BudgetSetStore{client: client, table: strings.TrimSpace(table)}
}

type budgetJSON struct {
	CategoryID       string `json:"categoryId"`
	LimitAmountCents int64  `json:"limitAmountCents"`
}

// LoadSet lee un periodo con consistencia fuerte. Devuelve (nil, nil) si no existe.
func (s *BudgetSetStore) LoadSet(ctx context.Context, userID, period string) (*appBudget.Set, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, appBudget.ErrUnavailable
	}
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(domainDynamo.PrefixUser + strings.TrimSpace(userID)),
			"SK": stringAttr("BUDGETSET#" + strings.TrimSpace(period)),
		},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return nil, appBudget.ErrUnavailable
	}
	if out == nil || len(out.Item) == 0 {
		return nil, nil
	}
	set, ok := budgetSetFromItem(out.Item, period)
	if !ok {
		return nil, appBudget.ErrInconsistent
	}
	return set, nil
}

// ListAllSets pagina por SK hasta agotar la partición. No aplica topes artificiales.
func (s *BudgetSetStore) ListAllSets(ctx context.Context, userID string) ([]appBudget.Set, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, appBudget.ErrUnavailable
	}
	pk := domainDynamo.PrefixUser + strings.TrimSpace(userID)
	var start map[string]types.AttributeValue
	salida := make([]appBudget.Set, 0)
	for {
		entrada := &dynamodb.QueryInput{
			TableName:              aws.String(s.table),
			KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :prefix)"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":pk":     stringAttr(pk),
				":prefix": stringAttr("BUDGETSET#"),
			},
			ConsistentRead: aws.Bool(true),
		}
		if len(start) > 0 {
			entrada.ExclusiveStartKey = start
		}
		out, err := s.client.Query(ctx, entrada)
		if err != nil {
			return nil, appBudget.ErrUnavailable
		}
		for _, item := range out.Items {
			periodo := strings.TrimPrefix(stringAttribute(item["SK"]), "BUDGETSET#")
			set, ok := budgetSetFromItem(item, periodo)
			if !ok {
				return nil, appBudget.ErrInconsistent
			}
			salida = append(salida, *set)
		}
		if len(out.LastEvaluatedKey) == 0 {
			return salida, nil
		}
		start = out.LastEvaluatedKey
	}
}

func budgetSetFromItem(item map[string]types.AttributeValue, period string) (*appBudget.Set, bool) {
	if entidad := stringAttribute(item["entityType"]); entidad != "" && entidad != "BUDGET_SET" {
		return nil, false
	}
	version, okVersion := attributeInt(item["version"])
	if !okVersion || version < 0 {
		return nil, false
	}
	var crudos []budgetJSON
	if err := json.Unmarshal([]byte(stringAttribute(item["itemsJSON"])), &crudos); err != nil {
		return nil, false
	}
	items := make([]appBudget.Budget, 0, len(crudos))
	for _, c := range crudos {
		if strings.TrimSpace(c.CategoryID) == "" || c.LimitAmountCents < 0 {
			return nil, false
		}
		items = append(items, appBudget.Budget{CategoryID: c.CategoryID, LimitAmountCents: c.LimitAmountCents})
	}
	currency := stringAttribute(item["currency"])
	if currency == "" {
		currency = "MXN"
	}
	updated, _ := attributeISOTime(item["updatedAt"])
	return &appBudget.Set{Period: period, Currency: currency, Version: version, Items: items, UpdatedAt: updated}, true
}

// BudgetSetItem construye el ítem del conjunto. Se exporta para E049 y las pruebas.
func BudgetSetItem(userID, period, currency string, version int64, items []appBudget.Budget, now time.Time) (map[string]types.AttributeValue, bool) {
	crudos := make([]budgetJSON, 0, len(items))
	for _, b := range items {
		if strings.TrimSpace(b.CategoryID) == "" || b.LimitAmountCents < 0 {
			return nil, false
		}
		crudos = append(crudos, budgetJSON{CategoryID: b.CategoryID, LimitAmountCents: b.LimitAmountCents})
	}
	raw, err := json.Marshal(crudos)
	if err != nil {
		return nil, false
	}
	return map[string]types.AttributeValue{
		"PK":         stringAttr(domainDynamo.PrefixUser + strings.TrimSpace(userID)),
		"SK":         stringAttr("BUDGETSET#" + strings.TrimSpace(period)),
		"entityType": stringAttr("BUDGET_SET"),
		"userId":     stringAttr(strings.TrimSpace(userID)),
		"period":     stringAttr(strings.TrimSpace(period)),
		"currency":   stringAttr(currency),
		"version":    numberAttr(version),
		"itemsJSON":  stringAttr(string(raw)),
		"updatedAt":  stringAttr(now.UTC().Format(time.RFC3339)),
	}, true
}
