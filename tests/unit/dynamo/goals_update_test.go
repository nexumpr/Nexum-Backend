package dynamo_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appGoals "github.com/nexumpr/Nexum-Backend/internal/application/goals"
	domainGoal "github.com/nexumpr/Nexum-Backend/internal/domain/goal"
)

// Pruebas de E041 sobre el adaptador REAL `GoalsStore` montado sobre el doble con estado:
// forma exacta del `UpdateItem`, CAS de versión, guarda atómica del objetivo frente al ahorro y
// desenlace de una segunda escritura con la misma versión.

var e041Escrito = time.Date(2026, 11, 18, 17, 0, 0, 0, time.UTC)

func e041Patch(nombre *string, objetivo *int64) domainGoal.Patch {
	return domainGoal.Patch{Name: nombre, TargetCents: objetivo}
}

func e041Texto(valor string) *string { return &valor }

func e041Cents(valor int64) *int64 { return &valor }

// TestE041UpdateItemFormaYVersion comprueba la expresión, la condición, ALL_NEW y el resultado
// persistido: versión +1, campos aplicados e importes históricos intactos.
func TestE041UpdateItemFormaYVersion(t *testing.T) {
	memoria := newMemoriaMetas()
	store := dynamoAdapter.NewGoalsStore(memoria, e038Tabla)
	sembrarMeta(t, store, e039Usuario, "goal-patch", domainGoal.StatusActive, 5000)

	actualizada, err := store.UpdateGoal(context.Background(), appGoals.UpdateWrite{
		UserID:          e039Usuario,
		GoalID:          "goal-patch",
		Patch:           e041Patch(e041Texto("Nuevo nombre"), e041Cents(40000)),
		ExpectedVersion: 1,
		Now:             e041Escrito,
	})
	if err != nil {
		t.Fatalf("la modificacion no debe fallar: %v", err)
	}
	if actualizada.Name != "Nuevo nombre" || actualizada.TargetAmount.Amount() != 40000 {
		t.Fatalf("campos no aplicados: %+v", actualizada)
	}
	if actualizada.SavedAmount.Amount() != 5000 || actualizada.OpeningSaved.Amount() != 5000 {
		t.Fatalf("los importes historicos no deben cambiar: %+v", actualizada)
	}
	if actualizada.Version != 2 || !actualizada.UpdatedAt.Equal(e041Escrito) {
		t.Fatalf("version o updatedAt inesperados: %d %v", actualizada.Version, actualizada.UpdatedAt)
	}

	entrada := memoria.ultimoUpdate
	if entrada == nil {
		t.Fatal("debe registrarse un UpdateItem")
	}
	expresion := aws.ToString(entrada.UpdateExpression)
	if !strings.Contains(expresion, "SET ") || !strings.Contains(expresion, "ADD #version :one") {
		t.Fatalf("UpdateExpression inesperada: %q", expresion)
	}
	if !strings.Contains(expresion, "#name = :name") || !strings.Contains(expresion, "#targetAmountCents = :target") {
		t.Fatalf("faltan asignaciones del patch: %q", expresion)
	}
	if strings.Contains(expresion, "savedAmountCents =") || strings.Contains(expresion, "openingSavedCents =") {
		t.Fatal("el patch no puede escribir importes historicos")
	}
	condicion := aws.ToString(entrada.ConditionExpression)
	for _, esperado := range []string{"attribute_exists(PK)", "#version = :expected", "savedAmountCents <= :newTarget"} {
		if !strings.Contains(condicion, esperado) {
			t.Fatalf("condicion incompleta (%q): %q", esperado, condicion)
		}
	}
	if entrada.ReturnValues != types.ReturnValueAllNew {
		t.Fatalf("ReturnValues inesperado: %q", entrada.ReturnValues)
	}

	persistido := memoria.items[claveDe("USER#"+e039Usuario, "GOAL#goal-patch")]
	if persistido == nil {
		t.Fatal("la meta debe seguir existiendo")
	}
	if version := numeroDe(persistido, "version"); version != 2 {
		t.Fatalf("version persistida inesperada: %d", version)
	}
	if nombre := mapaString(persistido, "name"); nombre != "Nuevo nombre" {
		t.Fatalf("nombre persistido inesperado: %q", nombre)
	}
	if cents := numeroDe(persistido, "targetAmountCents"); cents != 40000 {
		t.Fatalf("objetivo persistido inesperado: %d", cents)
	}
}

// TestE041VersionObsoleta comprueba el 412: la condición falla y la relectura detecta que la
// versión esperada no es la vigente.
func TestE041VersionObsoleta(t *testing.T) {
	memoria := newMemoriaMetas()
	store := dynamoAdapter.NewGoalsStore(memoria, e038Tabla)
	sembrarMeta(t, store, e039Usuario, "goal-patch", domainGoal.StatusActive, 5000)

	_, err := store.UpdateGoal(context.Background(), appGoals.UpdateWrite{
		UserID:          e039Usuario,
		GoalID:          "goal-patch",
		Patch:           e041Patch(e041Texto("otra"), nil),
		ExpectedVersion: 7,
		Now:             e041Escrito,
	})
	if !errors.Is(err, appGoals.ErrVersionConflict) {
		t.Fatalf("esperado ErrVersionConflict, obtenido %v", err)
	}
	if version := numeroDe(memoria.items[claveDe("USER#"+e039Usuario, "GOAL#goal-patch")], "version"); version != 1 {
		t.Fatalf("una version obsoleta no debe escribir: %d", version)
	}
}

// TestE041ObjetivoBajoAhorro comprueba la guarda atómica: con la versión correcta, un objetivo
// por debajo del ahorro acumulado se rechaza con 422 y no se aplica nada.
func TestE041ObjetivoBajoAhorro(t *testing.T) {
	memoria := newMemoriaMetas()
	store := dynamoAdapter.NewGoalsStore(memoria, e038Tabla)
	sembrarMeta(t, store, e039Usuario, "goal-patch", domainGoal.StatusActive, 5000)

	_, err := store.UpdateGoal(context.Background(), appGoals.UpdateWrite{
		UserID:          e039Usuario,
		GoalID:          "goal-patch",
		Patch:           e041Patch(nil, e041Cents(4000)),
		ExpectedVersion: 1,
		Now:             e041Escrito,
	})
	if !errors.Is(err, appGoals.ErrTargetBelowSaved) {
		t.Fatalf("esperado ErrTargetBelowSaved, obtenido %v", err)
	}
	persistido := memoria.items[claveDe("USER#"+e039Usuario, "GOAL#goal-patch")]
	if numeroDe(persistido, "version") != 1 || numeroDe(persistido, "targetAmountCents") != 15000 {
		t.Fatalf("la guarda no debe escribir: %+v", persistido)
	}
}

// TestE041DosEscriturasUnaGana simula dos nodos con la misma versión: la segunda recibe 412.
func TestE041DosEscriturasUnaGana(t *testing.T) {
	memoria := newMemoriaMetas()
	store := dynamoAdapter.NewGoalsStore(memoria, e038Tabla)
	sembrarMeta(t, store, e039Usuario, "goal-patch", domainGoal.StatusActive, 5000)

	primera := appGoals.UpdateWrite{
		UserID: e039Usuario, GoalID: "goal-patch",
		Patch: e041Patch(e041Texto("primera"), nil), ExpectedVersion: 1, Now: e041Escrito,
	}
	if _, err := store.UpdateGoal(context.Background(), primera); err != nil {
		t.Fatalf("la primera escritura debe ganar: %v", err)
	}
	segunda := primera
	segunda.Patch = e041Patch(e041Texto("segunda"), nil)
	if _, err := store.UpdateGoal(context.Background(), segunda); !errors.Is(err, appGoals.ErrVersionConflict) {
		t.Fatalf("la segunda debe recibir 412, obtuvo %v", err)
	}
	persistido := memoria.items[claveDe("USER#"+e039Usuario, "GOAL#goal-patch")]
	if mapaString(persistido, "name") != "primera" || numeroDe(persistido, "version") != 2 {
		t.Fatalf("estado final inesperado: %+v", persistido)
	}
}

// TestE041ValidacionesDelAdaptador cubre patch vacío, versión inválida y falta de DynamoDB.
func TestE041ValidacionesDelAdaptador(t *testing.T) {
	memoria := newMemoriaMetas()
	store := dynamoAdapter.NewGoalsStore(memoria, e038Tabla)
	sembrarMeta(t, store, e039Usuario, "goal-patch", domainGoal.StatusActive, 5000)

	if _, err := store.UpdateGoal(context.Background(), appGoals.UpdateWrite{
		UserID: e039Usuario, GoalID: "goal-patch", Patch: domainGoal.Patch{}, ExpectedVersion: 1, Now: e041Escrito,
	}); !errors.Is(err, appGoals.ErrInvalidRequest) {
		t.Fatalf("patch vacio: esperado ErrInvalidRequest, obtenido %v", err)
	}
	if _, err := store.UpdateGoal(context.Background(), appGoals.UpdateWrite{
		UserID: e039Usuario, GoalID: "goal-patch", Patch: e041Patch(e041Texto("x"), nil), ExpectedVersion: 0, Now: e041Escrito,
	}); !errors.Is(err, appGoals.ErrInvalidRequest) {
		t.Fatalf("version cero: esperado ErrInvalidRequest, obtenido %v", err)
	}
	if _, err := store.UpdateGoal(context.Background(), appGoals.UpdateWrite{
		UserID: e039Usuario, GoalID: "meta-inexistente", Patch: e041Patch(e041Texto("x"), nil), ExpectedVersion: 1, Now: e041Escrito,
	}); !errors.Is(err, appGoals.ErrNotFound) {
		t.Fatalf("meta inexistente: esperado ErrNotFound, obtenido %v", err)
	}

	vacio := dynamoAdapter.NewGoalsStore(nil, "")
	if _, err := vacio.UpdateGoal(context.Background(), appGoals.UpdateWrite{
		UserID: e039Usuario, GoalID: "goal-patch", Patch: e041Patch(e041Texto("x"), nil), ExpectedVersion: 1, Now: e041Escrito,
	}); !errors.Is(err, appGoals.ErrUnavailable) {
		t.Fatalf("sin DynamoDB: esperado ErrUnavailable, obtenido %v", err)
	}
}
