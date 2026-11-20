package dynamo

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	appDashboard "github.com/nexumpr/Nexum-Backend/internal/application/dashboard"
	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
)

// DashboardAPI es el subconjunto del cliente DynamoDB para E051. No incluye Scan.
type DashboardAPI interface {
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
}

// DashboardStore implementa `dashboard.Store` sobre la tabla única.
type DashboardStore struct {
	client DashboardAPI
	table  string
}

// NewDashboardStore construye el adaptador.
func NewDashboardStore(client DashboardAPI, table string) *DashboardStore {
	return &DashboardStore{client: client, table: strings.TrimSpace(table)}
}

// ListActiveGoals consulta metas no borradas y activas, en orden de SK.
func (s *DashboardStore) ListActiveGoals(ctx context.Context, userID string, limit int) ([]appDashboard.GoalRow, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, appDashboard.ErrUnavailable
	}
	if limit < 1 {
		limit = appDashboard.MaxGoals
	}
	out, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(s.table),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :prefix)"),
		FilterExpression:       aws.String("#status = :active AND (attribute_not_exists(#deleted) OR #deleted = :false)"),
		ExpressionAttributeNames: map[string]string{
			"#status":  "status",
			"#deleted": "deleted",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":     stringAttr(domainDynamo.PrefixUser + strings.TrimSpace(userID)),
			":prefix": stringAttr(domainDynamo.PrefixGoal),
			":active": stringAttr("active"),
			":false":  &types.AttributeValueMemberBOOL{Value: false},
		},
		Limit: aws.Int32(int32(limit)),
	})
	if err != nil {
		return nil, appDashboard.ErrUnavailable
	}
	salida := make([]appDashboard.GoalRow, 0, len(out.Items))
	for _, item := range out.Items {
		goalID := stringAttribute(item["goalId"])
		target, okTarget := attributeInt(item["targetAmountCents"])
		saved, okSaved := attributeInt(item["savedAmountCents"])
		opening, okOpening := attributeInt(item["openingSavedCents"])
		version, _ := attributeInt(item["version"])
		if goalID == "" || !okTarget || !okSaved || !okOpening {
			return nil, appDashboard.ErrInvalid
		}
		salida = append(salida, appDashboard.GoalRow{
			ID:           goalID,
			Name:         stringAttribute(item["name"]),
			TargetCents:  target,
			SavedCents:   saved,
			OpeningCents: opening,
			Currency:     stringAttribute(item["currency"]),
			TargetDate:   stringAttribute(item["targetDate"]),
			Status:       stringAttribute(item["status"]),
			Version:      version,
		})
	}
	return salida, nil
}

// ListLatestMovements consulta los movimientos más recientes en orden descendente.
func (s *DashboardStore) ListLatestMovements(ctx context.Context, userID string, limit int) ([]appMovements.Movement, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, appDashboard.ErrUnavailable
	}
	if limit < 1 {
		limit = appDashboard.LatestMovements
	}
	out, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(s.table),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :prefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":     stringAttr(domainDynamo.PrefixUser + strings.TrimSpace(userID)),
			":prefix": stringAttr(domainDynamo.PrefixMovement),
		},
		ScanIndexForward: aws.Bool(false),
		Limit:            aws.Int32(int32(limit)),
	})
	if err != nil {
		return nil, appDashboard.ErrUnavailable
	}
	salida := make([]appMovements.Movement, 0, len(out.Items))
	for _, item := range out.Items {
		m, ok := movimientoDesdeItem(item, strings.TrimSpace(userID))
		if !ok {
			return nil, appDashboard.ErrInvalid
		}
		salida = append(salida, m)
	}
	return salida, nil
}
