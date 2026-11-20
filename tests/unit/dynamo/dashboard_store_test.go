package dynamo_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
)

type dashboardAPIStub struct {
	paginas  []*dynamodb.QueryOutput
	indice   int
	entradas []*dynamodb.QueryInput
}

func (s *dashboardAPIStub) Query(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	s.entradas = append(s.entradas, params)
	if s.indice >= len(s.paginas) {
		return &dynamodb.QueryOutput{}, nil
	}
	pagina := s.paginas[s.indice]
	s.indice++
	items := make([]map[string]types.AttributeValue, 0, len(pagina.Items))
	for _, item := range pagina.Items {
		if attr, ok := item["status"].(*types.AttributeValueMemberS); !ok || attr.Value != "active" {
			continue
		}
		if attr, ok := item["deleted"].(*types.AttributeValueMemberBOOL); ok && attr.Value {
			continue
		}
		items = append(items, item)
	}
	return &dynamodb.QueryOutput{Items: items, LastEvaluatedKey: pagina.LastEvaluatedKey}, nil
}

func metaDashboard(id, status string, deleted bool) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"PK": strAttr("USER#u"), "SK": strAttr("GOAL#" + id),
		"goalId": strAttr(id), "name": strAttr("Meta " + id),
		"targetAmountCents": numAttrDashboard(100000), "savedAmountCents": numAttrDashboard(25000),
		"openingSavedCents": numAttrDashboard(5000), "currency": strAttr("MXN"),
		"targetDate": strAttr("2026-12-31"), "status": strAttr(status),
		"version": numAttrDashboard(1), "deleted": &types.AttributeValueMemberBOOL{Value: deleted},
	}
}

// TestE051DashboardPaginaMetasActivas comprueba que una página con metas inactivas no agota la
// consulta: se sigue con LastEvaluatedKey hasta reunir las metas activas o agotar la partición.
func TestE051DashboardPaginaMetasActivas(t *testing.T) {
	stub := &dashboardAPIStub{paginas: []*dynamodb.QueryOutput{
		{
			Items: []map[string]types.AttributeValue{metaDashboard("g1", "archived", false)},
			LastEvaluatedKey: map[string]types.AttributeValue{
				"PK": strAttr("USER#u"), "SK": strAttr("GOAL#g1"),
			},
		},
		{
			Items: []map[string]types.AttributeValue{
				metaDashboard("g2", "active", false),
				metaDashboard("g3", "active", false),
			},
		},
	}}
	store := dynamoAdapter.NewDashboardStore(stub, "NEXUM_CORE_TEST")
	goals, err := store.ListActiveGoals(context.Background(), "u", 100)
	if err != nil {
		t.Fatalf("listado: %v", err)
	}
	if len(goals) != 2 || goals[0].ID != "g2" || goals[1].ID != "g3" {
		t.Fatalf("metas activas inesperadas: %+v", goals)
	}
	if len(stub.entradas) != 2 {
		t.Fatalf("se esperaban 2 paginas, hubo %d", len(stub.entradas))
	}
	if stub.entradas[1].ExclusiveStartKey == nil {
		t.Fatal("la segunda pagina debe usar ExclusiveStartKey")
	}
	if aws.ToString(stub.entradas[0].FilterExpression) == "" {
		t.Fatal("debe filtrar metas activas y no borradas")
	}
}

func numAttrDashboard(n int64) types.AttributeValue {
	return &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", n)}
}
