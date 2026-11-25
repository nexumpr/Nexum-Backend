package dynamo_test

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appDashboard "github.com/nexumpr/Nexum-Backend/internal/application/dashboard"
)

// consistenciaStub devuelve siempre la misma página vacía con LastEvaluatedKey para poder
// observar las consultas emitidas y detectar un cursor que nunca progresa.
type consistenciaStub struct {
	entradas []*dynamodb.QueryInput
}

func (s *consistenciaStub) Query(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	s.entradas = append(s.entradas, params)
	return &dynamodb.QueryOutput{
		Items: nil,
		LastEvaluatedKey: map[string]types.AttributeValue{
			"PK": strAttr("USER#u"),
			"SK": strAttr("MOVEMENT#2026-11-25T00:00:00Z#00000000-0000-0000-0000-000000000000"),
		},
	}, nil
}

// TestE051ConsultasConLecturaConsistente cubre DEF-1: tras E031, una réplica con retraso no
// puede devolver un movimiento o una meta ya borrados; ambas consultas exigen lectura fuerte.
func TestE051ConsultasConLecturaConsistente(t *testing.T) {
	stub := &consistenciaStub{}
	store := dynamoAdapter.NewDashboardStore(stub, "NEXUM_CORE_TEST")

	if _, err := store.ListActiveGoals(context.Background(), "u", 3); !errors.Is(err, appDashboard.ErrUnavailable) {
		t.Fatalf("metas: se esperaba ErrUnavailable al agotar el tope de páginas, got %v", err)
	}
	if _, err := store.ListLatestMovements(context.Background(), "u", 3); !errors.Is(err, appDashboard.ErrUnavailable) {
		t.Fatalf("movimientos: se esperaba ErrUnavailable al agotar el tope de páginas, got %v", err)
	}
	if len(stub.entradas) == 0 {
		t.Fatal("no se registró ninguna consulta")
	}
	for i, entrada := range stub.entradas {
		if entrada.ConsistentRead == nil || !aws.ToBool(entrada.ConsistentRead) {
			t.Fatalf("consulta %d sin ConsistentRead=true", i)
		}
	}
}

// TestE051FiltroTombstonesEnConsulta comprueba que el filtro de E031 viaja en la consulta,
// no solo en el descarte del cliente.
func TestE051FiltroTombstonesEnConsulta(t *testing.T) {
	stub := &consistenciaStub{}
	store := dynamoAdapter.NewDashboardStore(stub, "NEXUM_CORE_TEST")

	_, _ = store.ListLatestMovements(context.Background(), "u", 1)
	_, _ = store.ListActiveGoals(context.Background(), "u", 1)

	for i, entrada := range stub.entradas {
		if entrada.FilterExpression == nil || *entrada.FilterExpression == "" {
			t.Fatalf("consulta %d sin FilterExpression de tombstones", i)
		}
	}
	ultima := stub.entradas[len(stub.entradas)-1]
	if _, ok := ultima.ExpressionAttributeValues[":false"].(*types.AttributeValueMemberBOOL); !ok {
		t.Fatal("el filtro no declara el valor booleano :false")
	}
}

// TestE051CursorSinProgresoNoCuelga fija el tope de páginas: un LastEvaluatedKey que no
// avanza termina con error controlado en vez de truncar en silencio o quedar colgado.
func TestE051CursorSinProgresoNoCuelga(t *testing.T) {
	stub := &consistenciaStub{}
	store := dynamoAdapter.NewDashboardStore(stub, "NEXUM_CORE_TEST")

	_, err := store.ListLatestMovements(context.Background(), "u", 10)
	if !errors.Is(err, appDashboard.ErrUnavailable) {
		t.Fatalf("se esperaba ErrUnavailable, got %v", err)
	}
	if len(stub.entradas) > 60 {
		t.Fatalf("demasiadas consultas: %d", len(stub.entradas))
	}
}
