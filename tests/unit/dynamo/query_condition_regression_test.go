package dynamo_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
)

// capturaConsulta registra la QueryInput exacta y devuelve los items precargados.
// No interpreta la condicion: la prueba verifica que la expresion enviada sea valida
// para DynamoDB (una sola condicion de rango por clave) y que el filtro de borde se aplique.
type capturaConsulta struct {
	ultima *dynamodb.QueryInput
	items  []map[string]types.AttributeValue
}

func (c *capturaConsulta) GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	return &dynamodb.GetItemOutput{}, nil
}

func (c *capturaConsulta) Query(_ context.Context, in *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	c.ultima = in
	return &dynamodb.QueryOutput{Items: c.items}, nil
}

func itemMovimiento(sk, tipo string, centavos, categoria string) map[string]types.AttributeValue {
	item := map[string]types.AttributeValue{
		"SK":          &types.AttributeValueMemberS{Value: sk},
		"type":        &types.AttributeValueMemberS{Value: tipo},
		"amountCents": &types.AttributeValueMemberN{Value: centavos},
	}
	if categoria != "" {
		item["categoryId"] = &types.AttributeValueMemberS{Value: categoria}
	}
	return item
}

func TestE026E050KeyConditionValidaYUnoSoloRango(t *testing.T) {
	desde := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	hasta := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	bordeMovimiento := "MOVEMENT#" + appMovements.SortableTime(hasta)
	fuera := itemMovimiento(bordeMovimiento, "expense", "999700", "comida")
	dentro := itemMovimiento("MOVEMENT#"+appMovements.SortableTime(desde.Add(10*24*time.Hour)), "expense", "10000", "comida")

	fake := &capturaConsulta{items: []map[string]types.AttributeValue{dentro, fuera}}
	resumen := dynamoAdapter.NewFinancialSummaryStore(fake, "NEXUM_CORE_TEST")
	totales, err := resumen.AggregateMovements(context.Background(), e020Titular, desde, hasta)
	if err != nil {
		t.Fatalf("AggregateMovements: %v", err)
	}
	if totales.ExpensesCents != 10000 {
		t.Fatalf("el borde superior no debe contarse: expenses=%d", totales.ExpensesCents)
	}
	expr := aws.ToString(fake.ultima.KeyConditionExpression)
	if !strings.Contains(expr, "BETWEEN") {
		t.Fatalf("se esperaba BETWEEN en la KeyConditionExpression: %q", expr)
	}
	if strings.Count(expr, ">=") > 0 || strings.Count(expr, "< ") > 0 {
		t.Fatalf("la clave no puede tener dos condiciones de rango: %q", expr)
	}
	if fake.ultima.ExpressionAttributeValues[":from"] == nil || fake.ultima.ExpressionAttributeValues[":to"] == nil {
		t.Fatalf("faltan :from/:to en la consulta: %#v", fake.ultima.ExpressionAttributeValues)
	}

	// La misma comprobacion para el agregado de aportaciones.
	aportacion := map[string]types.AttributeValue{
		"SK":          &types.AttributeValueMemberS{Value: "CONTRIBUTION#" + appMovements.SortableTime(desde.Add(2*24*time.Hour))},
		"status":      &types.AttributeValueMemberS{Value: "active"},
		"amountCents": &types.AttributeValueMemberN{Value: "30000"},
	}
	fake.items = []map[string]types.AttributeValue{aportacion}
	if _, err := resumen.AggregateContributions(context.Background(), e020Titular, desde, hasta); err != nil {
		t.Fatalf("AggregateContributions: %v", err)
	}
	expr = aws.ToString(fake.ultima.KeyConditionExpression)
	if !strings.Contains(expr, "BETWEEN") || strings.Count(expr, ">=") > 0 {
		t.Fatalf("aportaciones: KeyConditionExpression invalida: %q", expr)
	}

	// E050: mismo patron de consulta y mismo borde exclusivo.
	uso := dynamoAdapter.NewBudgetUsageStore(fake, "NEXUM_CORE_TEST")
	fake.items = []map[string]types.AttributeValue{dentro, fuera}
	gastos, err := uso.AggregateExpenses(context.Background(), e020Titular, desde, hasta)
	if err != nil {
		t.Fatalf("AggregateExpenses: %v", err)
	}
	if gastos["comida"] != 10000 {
		t.Fatalf("E050 debe contar solo el egreso dentro del periodo: %#v", gastos)
	}
	expr = aws.ToString(fake.ultima.KeyConditionExpression)
	if !strings.Contains(expr, "BETWEEN") || strings.Count(expr, ">=") > 0 {
		t.Fatalf("E050: KeyConditionExpression invalida: %q", expr)
	}
}
