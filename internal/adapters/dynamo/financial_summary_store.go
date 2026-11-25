package dynamo

import (
	"context"
	"math"
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

// FinancialSummaryStore agrega mientras pagina; no guarda el periodo completo en memoria ni
// aplica topes artificiales que conviertan una cuenta válida en 503.
type FinancialSummaryStore struct {
	client FinancialSummaryAPI
	table  string
}

// NewFinancialSummaryStore construye el adaptador. Sin cliente o tabla falla cerrado.
func NewFinancialSummaryStore(client FinancialSummaryAPI, table string) *FinancialSummaryStore {
	return &FinancialSummaryStore{client: client, table: strings.TrimSpace(table)}
}

// LoadTimezone lee la zona IANA de PREFERENCES. Ausente -> "", nil (fallback aprobado);
// error real de DynamoDB -> ErrUnavailable (el servicio responde 503).
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

// AggregateMovements suma income/expense/saving independiente en una sola pasada paginada.
func (s *FinancialSummaryStore) AggregateMovements(ctx context.Context, userID string, from, to time.Time) (appSummary.MovementTotals, error) {
	var total appSummary.MovementTotals
	err := s.queryRange(ctx, userID,
		"MOVEMENT#"+appMovements.SortableTime(from),
		"MOVEMENT#"+appMovements.SortableTime(to),
		func(item map[string]types.AttributeValue) error {
			tipo := strings.TrimSpace(stringAttribute(item["type"]))
			centavos, ok := attributeInt(item["amountCents"])
			if !ok || tipo == "" {
				return appSummary.ErrInconsistent
			}
			if centavos < 0 {
				return appSummary.ErrInconsistent
			}
			switch tipo {
			case "income":
				suma, err := sumar(total.IncomeCents, centavos)
				if err != nil {
					return err
				}
				total.IncomeCents = suma
			case "expense":
				suma, err := sumar(total.ExpensesCents, centavos)
				if err != nil {
					return err
				}
				total.ExpensesCents = suma
			case "saving":
				// El movimiento vinculado a una aportación no se suma por separado.
				if strings.TrimSpace(stringAttribute(item["contributionId"])) == "" {
					suma, err := sumar(total.IndependentSavingsCents, centavos)
					if err != nil {
						return err
					}
					total.IndependentSavingsCents = suma
				}
			default:
				return appSummary.ErrInconsistent
			}
			return nil
		})
	if err != nil {
		return appSummary.MovementTotals{}, err
	}
	return total, nil
}

// AggregateContributions suma las aportaciones activas del periodo.
func (s *FinancialSummaryStore) AggregateContributions(ctx context.Context, userID string, from, to time.Time) (appSummary.ContributionTotals, error) {
	var total appSummary.ContributionTotals
	err := s.queryRange(ctx, userID,
		"CONTRIBUTION#"+appMovements.SortableTime(from),
		"CONTRIBUTION#"+appMovements.SortableTime(to),
		func(item map[string]types.AttributeValue) error {
			estado := strings.TrimSpace(stringAttribute(item["status"]))
			centavos, ok := attributeInt(item["amountCents"])
			if !ok || estado == "" {
				return appSummary.ErrInconsistent
			}
			if centavos < 0 {
				return appSummary.ErrInconsistent
			}
			switch estado {
			case "active":
				suma, err := sumar(total.ActiveSavingsCents, centavos)
				if err != nil {
					return err
				}
				total.ActiveSavingsCents = suma
			case "reversed":
				// No cuenta como ahorro activo; el movimiento histórico se conserva.
			default:
				return appSummary.ErrInconsistent
			}
			return nil
		})
	if err != nil {
		return appSummary.ContributionTotals{}, err
	}
	return total, nil
}

// queryRange pagina con ExclusiveStartKey hasta agotar el rango. Sin Scan y sin tope de
// páginas: una cuenta válida no se trunca. Cada ítem se agrega y se descarta.
func (s *FinancialSummaryStore) queryRange(ctx context.Context, userID, desde, hasta string, fn func(map[string]types.AttributeValue) error) error {
	if s == nil || s.client == nil || s.table == "" {
		return appSummary.ErrUnavailable
	}
	pk := domainDynamo.PrefixUser + strings.TrimSpace(userID)
	var start map[string]types.AttributeValue
	for {
		entrada := &dynamodb.QueryInput{
			TableName:              aws.String(s.table),
			KeyConditionExpression: aws.String("PK = :pk AND #sk BETWEEN :from AND :to"),
			// E031: los tombstones (`deleted=true`) no suman en E026. Un ítem sin el atributo
			// (todo el histórico anterior a E031 y las aportaciones, que no lo tienen) cuenta.
			FilterExpression: aws.String("(attribute_not_exists(#deleted) OR #deleted = :false)"),
			ExpressionAttributeNames: map[string]string{
				"#sk":      "SK",
				"#deleted": "deleted",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":pk":    stringAttr(pk),
				":from":  stringAttr(desde),
				":to":    stringAttr(hasta),
				":false": &types.AttributeValueMemberBOOL{Value: false},
			},
			ConsistentRead: aws.Bool(true),
		}
		if len(start) > 0 {
			entrada.ExclusiveStartKey = start
		}
		out, err := s.client.Query(ctx, entrada)
		if err != nil {
			return appSummary.ErrUnavailable
		}
		for _, item := range out.Items {
			// BETWEEN incluye el extremo superior; el contrato es [desde, hasta).
			if stringAttribute(item["SK"]) >= hasta {
				continue
			}
			if err := fn(item); err != nil {
				return err
			}
		}
		if len(out.LastEvaluatedKey) == 0 {
			return nil
		}
		start = out.LastEvaluatedKey
	}
}

func sumar(a, b int64) (int64, error) {
	if b > 0 && a > math.MaxInt64-b {
		return 0, appSummary.ErrOverflow
	}
	return a + b, nil
}
