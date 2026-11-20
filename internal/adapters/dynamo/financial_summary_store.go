package dynamo

import (
	"context"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	appSummary "github.com/nexumpr/Nexum-Backend/internal/application/financialsummary"
	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
)

// FinancialSummaryAPI es el subconjunto del cliente DynamoDB para E026. No incluye Scan.
type FinancialSummaryAPI interface {
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
}

const (
	financialSummaryPageSize = 100
	financialSummaryMaxPages = 100
)

// FinancialSummaryStore implementa `financialsummary.Store` sobre la tabla única.
type FinancialSummaryStore struct {
	client FinancialSummaryAPI
	table  string
}

// NewFinancialSummaryStore construye el adaptador. Sin cliente o tabla falla cerrado.
func NewFinancialSummaryStore(client FinancialSummaryAPI, table string) *FinancialSummaryStore {
	return &FinancialSummaryStore{client: client, table: strings.TrimSpace(table)}
}

// LoadTimezone lee la zona IANA de PREFERENCES. Ausente o inválida no es error: el servicio
// aplica America/Mexico_City.
func (s *FinancialSummaryStore) LoadTimezone(ctx context.Context, userID string) (string, error) {
	if s == nil || s.client == nil || s.table == "" {
		return "", appSummary.ErrUnavailable
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
		return "", appSummary.ErrUnavailable
	}
	if out == nil || len(out.Item) == 0 {
		return "", nil
	}
	if entidad := stringAttribute(out.Item["entityType"]); entidad != "" && entidad != "USER_PREFERENCES" {
		return "", nil
	}
	if tz := strings.TrimSpace(stringAttribute(out.Item["timezone"])); tz != "" {
		return tz, nil
	}
	return strings.TrimSpace(stringAttribute(out.Item["timeZone"])), nil
}

// ListMovements consulta `[from, to)` por rango de SK. No usa Scan.
func (s *FinancialSummaryStore) ListMovements(ctx context.Context, userID string, from, to time.Time) ([]appSummary.MovementRow, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, appSummary.ErrUnavailable
	}
	rows, err := s.queryRange(ctx, userID,
		"MOVEMENT#"+appMovements.SortableTime(from),
		"MOVEMENT#"+appMovements.SortableTime(to),
		financialSummaryMaxPages,
	)
	if err != nil {
		return nil, err
	}
	salida := make([]appSummary.MovementRow, 0, len(rows))
	for _, item := range rows {
		tipo := strings.TrimSpace(stringAttribute(item["type"]))
		centavos, ok := attributeInt(item["amountCents"])
		if !ok || tipo == "" {
			continue
		}
		salida = append(salida, appSummary.MovementRow{
			Type:           tipo,
			AmountCents:    centavos,
			ContributionID: stringAttribute(item["contributionId"]),
		})
	}
	return salida, nil
}

// ListContributions consulta `[from, to)` por rango de SK. No usa Scan.
func (s *FinancialSummaryStore) ListContributions(ctx context.Context, userID string, from, to time.Time) ([]appSummary.ContributionRow, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, appSummary.ErrUnavailable
	}
	rows, err := s.queryRange(ctx, userID,
		"CONTRIBUTION#"+appMovements.SortableTime(from),
		"CONTRIBUTION#"+appMovements.SortableTime(to),
		financialSummaryMaxPages,
	)
	if err != nil {
		return nil, err
	}
	salida := make([]appSummary.ContributionRow, 0, len(rows))
	for _, item := range rows {
		estado := strings.TrimSpace(stringAttribute(item["status"]))
		centavos, ok := attributeInt(item["amountCents"])
		if !ok || estado == "" {
			continue
		}
		salida = append(salida, appSummary.ContributionRow{Status: estado, AmountCents: centavos})
	}
	return salida, nil
}

// queryRange pagina por SK entre `desde` (inclusive) y `hasta` (exclusive).
func (s *FinancialSummaryStore) queryRange(ctx context.Context, userID, desde, hasta string, maxPages int) ([]map[string]types.AttributeValue, error) {
	pk := domainDynamo.PrefixUser + strings.TrimSpace(userID)
	items := make([]map[string]types.AttributeValue, 0, financialSummaryPageSize)
	var start map[string]types.AttributeValue
	for pagina := 0; pagina < maxPages; pagina++ {
		entrada := &dynamodb.QueryInput{
			TableName:              aws.String(s.table),
			KeyConditionExpression: aws.String("PK = :pk AND #sk >= :from AND #sk < :to"),
			ExpressionAttributeNames: map[string]string{
				"#sk": "SK",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":pk":   stringAttr(pk),
				":from": stringAttr(desde),
				":to":   stringAttr(hasta),
			},
			ConsistentRead: aws.Bool(true),
			Limit:          aws.Int32(financialSummaryPageSize),
		}
		if len(start) > 0 {
			entrada.ExclusiveStartKey = start
		}
		out, err := s.client.Query(ctx, entrada)
		if err != nil {
			return nil, appSummary.ErrUnavailable
		}
		items = append(items, out.Items...)
		if len(out.LastEvaluatedKey) == 0 {
			return items, nil
		}
		start = out.LastEvaluatedKey
	}
	return nil, appSummary.ErrTooManyItems
}
