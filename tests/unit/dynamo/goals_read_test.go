package dynamo_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appGoals "github.com/nexumpr/Nexum-Backend/internal/application/goals"
	domainGoal "github.com/nexumpr/Nexum-Backend/internal/domain/goal"
)

// Pruebas de E040 sobre el adaptador REAL `GoalsStore`: lectura consistente por clave exacta,
// (nil, nil) para recursos ausentes o ajenos y fallo cerrado con items incoherentes. Las pruebas
// de paginación y totales de E038 viven en goals_list_test.go.

const e038Tabla = "NEXUM_CORE_TEST"

var e038Sembrado = time.Date(2026, 11, 18, 16, 0, 0, 0, time.UTC)

// sembrarMeta escribe una meta con el propio adaptador (transacción real de E039), de modo que
// el item sembrado tiene exactamente la forma que produce el backend.
func sembrarMeta(t *testing.T, store *dynamoAdapter.GoalsStore, userID, goalID string, status domainGoal.Status, savedCents int64) {
	t.Helper()
	meta, err := domainGoal.New(
		userID, goalID, "Meta "+goalID,
		savedCents+10000, savedCents,
		time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
		e038Sembrado,
	)
	if err != nil {
		t.Fatalf("no se pudo construir %s: %v", goalID, err)
	}
	meta.Status = status
	_, err = store.CreateGoal(context.Background(), appGoals.CreateWrite{
		Goal:           &meta,
		Operation:      appGoals.OperationCreateGoal,
		IdempotencyKey: "018f90ef-08e5-7a68-b09f-2b67a47c" + goalIDSufijo(goalID),
		PayloadHash:    "hash-" + goalID,
		ResponseBody:   []byte(`{}`),
		Location:       "/api/v1/goals/" + goalID,
		TTL:            appGoals.IdempotencyTTL,
		Now:            e038Sembrado,
	})
	if err != nil {
		t.Fatalf("no se pudo sembrar %s: %v", goalID, err)
	}
}

// goalIDSufijo deriva una clave de idempotencia de forma estable a partir del identificador.
func goalIDSufijo(goalID string) string {
	if len(goalID) >= 6 {
		return goalID[len(goalID)-6:]
	}
	return goalID
}

// TestE040LoadGoalConsistenteY404 cubre la lectura por clave exacta y sus casos 404.
func TestE040LoadGoalConsistenteY404(t *testing.T) {
	memoria := newMemoriaMetas()
	store := dynamoAdapter.NewGoalsStore(memoria, e038Tabla)
	sembrarMeta(t, store, e039Usuario, "goal-propia", domainGoal.StatusActive, 12345)

	meta, err := store.LoadGoal(context.Background(), e039Usuario, "goal-propia")
	if err != nil || meta == nil {
		t.Fatalf("la meta propia debe leerse: %v", err)
	}
	if meta.OpeningSaved.Amount() != 12345 || meta.SavedAmount.Amount() != 12345 {
		t.Fatalf("importes inesperados: %+v", meta)
	}
	if !memoria.lecturaConsistente {
		t.Fatal("la lectura por clave debe ser consistente")
	}

	if meta, err := store.LoadGoal(context.Background(), e039Usuario, "goal-inexistente"); err != nil || meta != nil {
		t.Fatalf("una meta inexistente debe ser (nil, nil): %v %v", meta, err)
	}
	if meta, err := store.LoadGoal(context.Background(), "99999999-8888-4777-8666-555555555555", "goal-propia"); err != nil || meta != nil {
		t.Fatalf("una meta de otro titular debe ser (nil, nil): %v %v", meta, err)
	}

	// Otra entidad ocupando la misma clave no es una meta.
	memoria.items[claveDe("USER#"+e039Usuario, "GOAL#goal-ocupada")] = map[string]types.AttributeValue{
		"PK": strAttr("USER#" + e039Usuario), "SK": strAttr("GOAL#goal-ocupada"), "entityType": strAttr("OTRA_COSA"),
	}
	if meta, err := store.LoadGoal(context.Background(), e039Usuario, "goal-ocupada"); err != nil || meta != nil {
		t.Fatalf("una entidad ajena en la clave debe ser (nil, nil): %v %v", meta, err)
	}
}

// TestE040ItemIncoherente comprueba que un item con forma inválida se falla cerrado.
func TestE040ItemIncoherente(t *testing.T) {
	memoria := newMemoriaMetas()
	store := dynamoAdapter.NewGoalsStore(memoria, e038Tabla)
	memoria.items[claveDe("USER#"+e039Usuario, "GOAL#rota")] = map[string]types.AttributeValue{
		"PK":                strAttr("USER#" + e039Usuario),
		"SK":                strAttr("GOAL#rota"),
		"entityType":        strAttr("GOAL"),
		"userId":            strAttr(e039Usuario),
		"goalId":            strAttr("rota"),
		"name":              strAttr("Rota"),
		"targetAmountCents": numAttr(1000),
		"savedAmountCents":  numAttr(0),
		"openingSavedCents": numAttr(0),
		"currency":          strAttr("MXN"),
		"targetDate":        strAttr("2026-12-31"),
		"status":            strAttr("estado-inventado"),
		"version":           numAttr(1),
		"createdAt":         strAttr("2026-11-18T16:00:00Z"),
	}
	if _, err := store.LoadGoal(context.Background(), e039Usuario, "rota"); !errors.Is(err, appGoals.ErrInconsistent) {
		t.Fatalf("esperado ErrInconsistent, obtenido %v", err)
	}

	// El agregado sólo lee metas activas: para probar su camino incoherente hace falta una
	// activa con el importe ilegible.
	memoria.items[claveDe("USER#"+e039Usuario, "GOAL#activa-rota")] = map[string]types.AttributeValue{
		"PK":               strAttr("USER#" + e039Usuario),
		"SK":               strAttr("GOAL#activa-rota"),
		"entityType":       strAttr("GOAL"),
		"status":           strAttr("active"),
		"savedAmountCents": strAttr("no-es-un-numero"),
	}
	if _, err := store.AggregateUserGoals(context.Background(), e039Usuario); !errors.Is(err, appGoals.ErrInconsistent) {
		t.Fatalf("el agregado debe fallar cerrado con un importe ilegible: %v", err)
	}
}

// TestE040SinDynamo comprueba el 503 de la lectura.
func TestE040SinDynamo(t *testing.T) {
	store := dynamoAdapter.NewGoalsStore(nil, "")
	if _, err := store.LoadGoal(context.Background(), e039Usuario, "goal-01"); !errors.Is(err, appGoals.ErrUnavailable) {
		t.Fatalf("esperado ErrUnavailable, obtenido %v", err)
	}
}
