package dynamo_test

import (
	"context"
	"regexp"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appGoals "github.com/nexumpr/Nexum-Backend/internal/application/goals"
	domainGoal "github.com/nexumpr/Nexum-Backend/internal/domain/goal"
)

// TestE042UpdateItemDeclaraTodosLosValores captura el UpdateItem real de E042 y exige que
// cada marcador usado en las expresiones tenga valor declarado. La regresion original
// (`ADD #version :one` sin `:one`) producia ValidationException en DynamoDB y 503.
func TestE042UpdateItemDeclaraTodosLosValores(t *testing.T) {
	memoria := newMemoriaMetas()
	store := dynamoAdapter.NewGoalsStore(memoria, e038Tabla)
	goalID := "018f90ef-08e5-7a68-b09f-0000000000e2"
	sembrarMeta(t, store, e039Usuario, goalID, domainGoal.StatusActive, 0)

	if err := store.DeleteGoal(context.Background(), appGoals.DeleteWrite{
		UserID: e039Usuario,
		GoalID: goalID,
		Now:    e038Sembrado,
	}); err != nil {
		t.Fatalf("DeleteGoal: %v", err)
	}
	if memoria.ultimoUpdate == nil {
		t.Fatal("no se capturo UpdateItem")
	}
	entrada := memoria.ultimoUpdate
	expresion := aws.ToString(entrada.UpdateExpression) + " " + aws.ToString(entrada.ConditionExpression)
	patron := regexp.MustCompile(`:[A-Za-z0-9_]+`)
	for _, marcador := range patron.FindAllString(expresion, -1) {
		if _, existe := entrada.ExpressionAttributeValues[marcador]; !existe {
			t.Fatalf("el marcador %s no tiene valor en ExpressionAttributeValues (expresion %q)", marcador, expresion)
		}
	}
	// El ADD de version debe conservar su valor numerico.
	if _, ok := entrada.ExpressionAttributeValues[":one"].(*types.AttributeValueMemberN); !ok {
		t.Fatalf(":one debe ser Number: %#v", entrada.ExpressionAttributeValues[":one"])
	}
}
