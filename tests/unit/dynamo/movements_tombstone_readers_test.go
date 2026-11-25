package dynamo_test

// Pruebas de E031 en los lectores de agregados: un movimiento con `deleted=true` (tombstone) NO
// suma en E026 (resumen), E050 (uso de presupuesto) ni aparece en E051 (tablero). Los ítems sin
// el atributo `deleted` (todo el histórico anterior a E031) cuentan como no borrados.
//
// Se ejecutan contra los adaptadores reales y dobles del cliente DynamoDB; no hay red ni AWS.

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
)

// tItemMovimiento construye el ítem principal que escribe E028/E031. `deleted` nulo omite el
// atributo (histórico anterior a E031).
func tItemMovimiento(id string, ocurrido time.Time, tipo string, centavos int64, categoria string, deleted *bool) map[string]types.AttributeValue {
	item := map[string]types.AttributeValue{
		"PK":          strAttr("USER#" + fiUsuario),
		"SK":          strAttr("MOVEMENT#" + appMovements.SortableTime(ocurrido) + "#" + id),
		"entityType":  strAttr("USER_MOVEMENT"),
		"userId":      strAttr(fiUsuario),
		"movementId":  strAttr(id),
		"type":        strAttr(tipo),
		"amountCents": numAttr(centavos),
		"currency":    strAttr("MXN"),
		"categoryId":  strAttr(categoria),
		"occurredAt":  strAttr(appMovements.SortableTime(ocurrido)),
		"version":     numAttr(2),
		"createdAt":   strAttr(ocurrido.UTC().Format(time.RFC3339)),
	}
	if deleted != nil {
		item["deleted"] = &types.AttributeValueMemberBOOL{Value: *deleted}
	}
	return item
}

func tBool(valor bool) *bool { return &valor }

// TestE026ResumenExcluyeTombstone comprueba que un ingreso borrado no suma y que el histórico sin
// atributo `deleted` sigue contando.
func TestE026ResumenExcluyeTombstone(t *testing.T) {
	memoria := nuevaMemoriaFinanciera()
	memoria.insertar(tItemMovimiento("m-vivo-1", fiDia(2), "income", 10000, "salary", nil))
	memoria.insertar(tItemMovimiento("m-borrado", fiDia(3), "income", 50000, "salary", tBool(true)))
	memoria.insertar(tItemMovimiento("m-vivo-2", fiDia(4), "income", 20000, "salary", tBool(false)))
	store := dynamoAdapter.NewFinancialSummaryStore(memoria, fiTabla)

	totales, err := store.AggregateMovements(context.Background(), fiUsuario, fiDia(1), fiDia(5))
	if err != nil {
		t.Fatalf("resumen inesperado: %v", err)
	}
	if totales.IncomeCents != 30000 {
		t.Fatalf("el tombstone no puede sumar: se esperaban 30000 centavos y hay %d", totales.IncomeCents)
	}
}

// TestE050PresupuestoExcluyeTombstone comprueba que un gasto borrado no consume presupuesto.
func TestE050PresupuestoExcluyeTombstone(t *testing.T) {
	memoria := nuevaMemoriaFinanciera()
	memoria.insertar(tItemMovimiento("m-gasto-vivo", fiDia(2), "expense", 1000, "food", nil))
	memoria.insertar(tItemMovimiento("m-gasto-borrado", fiDia(2), "expense", 900, "food", tBool(true)))
	memoria.insertar(tItemMovimiento("m-transporte", fiDia(3), "expense", 500, "transport", tBool(false)))
	memoria.insertar(tItemMovimiento("m-ingreso", fiDia(3), "income", 7000, "salary", nil))
	store := dynamoAdapter.NewBudgetUsageStore(memoria, fiTabla)

	gastos, err := store.AggregateExpenses(context.Background(), fiUsuario, fiDia(1), fiDia(5))
	if err != nil {
		t.Fatalf("uso inesperado: %v", err)
	}
	if gastos["food"] != 1000 {
		t.Fatalf("food debe ser 1000 sin el tombstone y es %d", gastos["food"])
	}
	if gastos["transport"] != 500 {
		t.Fatalf("transport debe ser 500 y es %d", gastos["transport"])
	}
	if _, existe := gastos["salary"]; existe {
		t.Fatal("un ingreso no consume presupuesto")
	}
}

// dashboardMovStub devuelve páginas ya preparadas aplicando el FilterExpression de tombstones,
// para verificar que el adaptador pagina hasta llenar el límite.
type dashboardMovStub struct {
	paginas  []*dynamodb.QueryOutput
	indice   int
	entradas []*dynamodb.QueryInput
}

func (s *dashboardMovStub) Query(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	s.entradas = append(s.entradas, params)
	if s.indice >= len(s.paginas) {
		return &dynamodb.QueryOutput{}, nil
	}
	pagina := s.paginas[s.indice]
	s.indice++
	filtrados := make([]map[string]types.AttributeValue, 0, len(pagina.Items))
	for _, item := range pagina.Items {
		if borrado, ok := item["deleted"].(*types.AttributeValueMemberBOOL); ok && borrado.Value {
			continue
		}
		filtrados = append(filtrados, item)
	}
	return &dynamodb.QueryOutput{Items: filtrados, LastEvaluatedKey: pagina.LastEvaluatedKey}, nil
}

// TestE051TableroExcluyeTombstoneYPagina comprueba que los movimientos borrados no se sirven y
// que una página llena de tombstones no acorta el resultado: se sigue con ExclusiveStartKey.
func TestE051TableroExcluyeTombstoneYPagina(t *testing.T) {
	pagina1 := []map[string]types.AttributeValue{
		tItemMovimiento("m-borrado-1", fiDia(3), "expense", 100, "food", tBool(true)),
		tItemMovimiento("m-borrado-2", fiDia(2), "expense", 200, "food", tBool(true)),
	}
	stub := &dashboardMovStub{paginas: []*dynamodb.QueryOutput{
		{
			Items:            pagina1,
			LastEvaluatedKey: map[string]types.AttributeValue{"PK": strAttr("USER#" + fiUsuario), "SK": strAttr("MOVEMENT#2026-11-02T10:00:00.000Z#m-borrado-2")},
		},
		{
			Items: []map[string]types.AttributeValue{
				tItemMovimiento("m-vivo-1", fiDia(1), "expense", 300, "food", nil),
			},
			LastEvaluatedKey: map[string]types.AttributeValue{"PK": strAttr("USER#" + fiUsuario), "SK": strAttr("MOVEMENT#2026-11-01T10:00:00.000Z#m-vivo-1")},
		},
		{
			Items: []map[string]types.AttributeValue{
				tItemMovimiento("m-vivo-2", fiDia(1), "income", 400, "salary", nil),
				tItemMovimiento("m-vivo-3", fiDia(1), "saving", 500, "ahorro", nil),
			},
		},
	}}
	store := dynamoAdapter.NewDashboardStore(stub, fiTabla)

	movimientos, err := store.ListLatestMovements(context.Background(), fiUsuario, 3)
	if err != nil {
		t.Fatalf("tablero inesperado: %v", err)
	}
	if len(movimientos) != 3 {
		t.Fatalf("se esperaban 3 movimientos vivos y hay %d", len(movimientos))
	}
	for _, mov := range movimientos {
		if mov.Deleted {
			t.Fatalf("un tombstone no puede servirse en el tablero: %+v", mov)
		}
	}
	if len(stub.entradas) != 3 {
		t.Fatalf("debe paginar hasta llenar el limite: %d consultas", len(stub.entradas))
	}
	if stub.entradas[1].ExclusiveStartKey == nil {
		t.Fatal("la segunda pagina debe usar ExclusiveStartKey")
	}
	for _, entrada := range stub.entradas {
		if aws.ToString(entrada.FilterExpression) != "(attribute_not_exists(#deleted) OR #deleted = :false)" {
			t.Fatalf("el tablero debe excluir tombstones en la consulta: %q", aws.ToString(entrada.FilterExpression))
		}
	}
}
