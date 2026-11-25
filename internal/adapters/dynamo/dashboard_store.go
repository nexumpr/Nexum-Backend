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

// maxDashboardPaginas acota el número de consultas encadenadas de E051. Si un cursor no
// progresa, el bucle termina con error en vez de quedar colgado o truncar en silencio.
const maxDashboardPaginas = 50

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
	salida := make([]appDashboard.GoalRow, 0, limit)
	var start map[string]types.AttributeValue
	for pagina := 0; ; pagina++ {
		if pagina >= maxDashboardPaginas {
			return nil, appDashboard.ErrUnavailable
		}
		entrada := &dynamodb.QueryInput{
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
			// DEF-1: mismo motivo que en ListLatestMovements; las metas borradas
			// tampoco deben reaparecer por consistencia eventual.
			ConsistentRead: aws.Bool(true),
		}
		if len(start) > 0 {
			entrada.ExclusiveStartKey = start
		}
		out, err := s.client.Query(ctx, entrada)
		if err != nil {
			return nil, appDashboard.ErrUnavailable
		}
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
			if len(salida) >= limit {
				return salida, nil
			}
		}
		if len(out.LastEvaluatedKey) == 0 {
			return salida, nil
		}
		start = out.LastEvaluatedKey
	}
}

// ListLatestMovements consulta los movimientos más recientes en orden descendente.
//
// E031: los tombstones (`deleted=true`) no se sirven en el tablero. Como el filtro descarta
// elementos DESPUÉS de que DynamoDB aplique `Limit`, se pagina hasta reunir `limit` movimientos
// vivos (o agotar la partición): nunca se devuelve una página corta por culpa de un borrado.
func (s *DashboardStore) ListLatestMovements(ctx context.Context, userID string, limit int) ([]appMovements.Movement, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, appDashboard.ErrUnavailable
	}
	if limit < 1 {
		limit = appDashboard.LatestMovements
	}
	titular := strings.TrimSpace(userID)
	salida := make([]appMovements.Movement, 0, limit)
	var start map[string]types.AttributeValue
	for pagina := 0; ; pagina++ {
		if pagina >= maxDashboardPaginas {
			return nil, appDashboard.ErrUnavailable
		}
		entrada := &dynamodb.QueryInput{
			TableName:              aws.String(s.table),
			KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :prefix)"),
			FilterExpression:       aws.String("(attribute_not_exists(#deleted) OR #deleted = :false)"),
			ExpressionAttributeNames: map[string]string{
				"#deleted": "deleted",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":pk":     stringAttr(domainDynamo.PrefixUser + titular),
				":prefix": stringAttr(domainDynamo.PrefixMovement),
				":false":  &types.AttributeValueMemberBOOL{Value: false},
			},
			ScanIndexForward: aws.Bool(false),
			Limit:            aws.Int32(int32(limit)),
			// DEF-1: E051 exige agregados consistentes. Sin lectura fuerte una réplica
			// puede devolver la versión previa al tombstone y servir un movimiento borrado.
			ConsistentRead: aws.Bool(true),
		}
		if len(start) > 0 {
			entrada.ExclusiveStartKey = start
		}
		out, err := s.client.Query(ctx, entrada)
		if err != nil {
			return nil, appDashboard.ErrUnavailable
		}
		for _, item := range out.Items {
			m, ok := movimientoDesdeItem(item, titular)
			if !ok {
				return nil, appDashboard.ErrInvalid
			}
			if m.Deleted {
				// Defensa en profundidad: el filtro ya lo excluye; si el ítem llegara como
				// tombstone no se sirve ni se cuenta para el límite.
				continue
			}
			salida = append(salida, m)
			if len(salida) >= limit {
				return salida, nil
			}
		}
		if len(out.LastEvaluatedKey) == 0 {
			return salida, nil
		}
		start = out.LastEvaluatedKey
	}
}
