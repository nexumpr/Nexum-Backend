package dynamo_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appGoals "github.com/nexumpr/Nexum-Backend/internal/application/goals"
	domainGoal "github.com/nexumpr/Nexum-Backend/internal/domain/goal"
)

// Pruebas de E038 sobre el adaptador REAL `GoalsStore` montado sobre el doble con estado:
// paginación por `LastEvaluatedKey`, filtro de estado que atraviesa páginas vacías y totales
// sobre la colección completa con varias páginas internas.

// TestE038PaginacionConLastEvaluatedKey recorre la partición por cursor y comprueba que no hay
// duplicados ni saltos y que se pide una fila de más para saber si hay página siguiente.
func TestE038PaginacionConLastEvaluatedKey(t *testing.T) {
	memoria := newMemoriaMetas()
	store := dynamoAdapter.NewGoalsStore(memoria, e038Tabla)
	for i := 1; i <= 5; i++ {
		sembrarMeta(t, store, e039Usuario, fmt.Sprintf("goal-%02d", i), domainGoal.StatusActive, int64(i)*100)
	}

	vistos := map[string]bool{}
	cursor := ""
	paginas := 0
	for {
		consultasPrevias := memoria.consultas
		filas, nextSK, err := store.ListUserGoals(context.Background(), appGoals.ListStoreQuery{
			UserID: e039Usuario, Limit: 2, StartSK: cursor,
		})
		if err != nil {
			t.Fatalf("listado fallo: %v", err)
		}
		paginas++
		if paginas == 1 {
			// Sobrelectura: se pide limit+1 para saber si hay más sin emitir un cursor vacío.
			if memoria.ultimaQuery == nil || memoria.ultimaQuery.Limit == nil || *memoria.ultimaQuery.Limit != 3 {
				t.Fatalf("la primera consulta debe pedir 3 items (limit+1)")
			}
			if !memoria.lecturaConsistente {
				t.Fatal("la consulta debe ser consistente")
			}
		}
		if memoria.consultas <= consultasPrevias {
			t.Fatal("cada pagina debe consultar DynamoDB")
		}
		for _, fila := range filas {
			if fila.SortKey != "GOAL#"+fila.Goal.ID {
				t.Fatalf("SortKey inesperada: %q", fila.SortKey)
			}
			if vistos[fila.Goal.ID] {
				t.Fatalf("meta repetida entre paginas: %s", fila.Goal.ID)
			}
			vistos[fila.Goal.ID] = true
		}
		cursor = nextSK
		if cursor == "" {
			break
		}
		if paginas > 10 {
			t.Fatal("el cursor no termina")
		}
	}
	if len(vistos) != 5 {
		t.Fatalf("se esperaban 5 metas, se vieron %d: %v", len(vistos), vistos)
	}
	if paginas != 3 {
		t.Fatalf("cinco metas con pagina de dos deben ser tres paginas, fueron %d", paginas)
	}
}

// TestE038FiltroAtraviesaPaginasVacias comprueba que una página sin coincidencias no se
// presenta como fin de la colección: DynamoDB aplica Limit antes del filtro.
func TestE038FiltroAtraviesaPaginasVacias(t *testing.T) {
	memoria := newMemoriaMetas()
	store := dynamoAdapter.NewGoalsStore(memoria, e038Tabla)
	for i := 1; i <= 4; i++ {
		sembrarMeta(t, store, e039Usuario, fmt.Sprintf("archivada-%02d", i), domainGoal.StatusArchived, 1000)
	}
	for i := 1; i <= 2; i++ {
		sembrarMeta(t, store, e039Usuario, fmt.Sprintf("zzz-activa-%02d", i), domainGoal.StatusActive, int64(i)*500)
	}

	filas, nextSK, err := store.ListUserGoals(context.Background(), appGoals.ListStoreQuery{
		UserID: e039Usuario, Limit: 2, HasStatus: true, Status: domainGoal.StatusActive,
	})
	if err != nil {
		t.Fatalf("listado filtrado fallo: %v", err)
	}
	if len(filas) != 2 {
		t.Fatalf("se esperaban 2 activas, se obtuvieron %d", len(filas))
	}
	for _, fila := range filas {
		if fila.Goal.Status != domainGoal.StatusActive {
			t.Fatalf("fila no activa en un listado filtrado: %+v", fila.Goal)
		}
	}
	if nextSK != "" {
		t.Fatalf("no quedan activas: el cursor debe estar vacio, obtuvo %q", nextSK)
	}
	if memoria.consultas < 2 {
		t.Fatalf("el adaptador debe recorrer al menos dos paginas para atravesar la vacia: %d consultas", memoria.consultas)
	}
}

// TestE038TotalesSobreColeccionCompleta comprueba el recorrido completo del agregado: 150 metas
// (100 activas) exigen dos páginas internas y la suma es exacta.
func TestE038TotalesSobreColeccionCompleta(t *testing.T) {
	memoria := newMemoriaMetas()
	store := dynamoAdapter.NewGoalsStore(memoria, e038Tabla)
	for i := 0; i < 150; i++ {
		estado := domainGoal.StatusActive
		if i >= 100 {
			estado = domainGoal.StatusArchived
		}
		sembrarMeta(t, store, e039Usuario, fmt.Sprintf("meta-%03d", i), estado, 100)
	}
	consultasAntes := memoria.consultas

	totales, err := store.AggregateUserGoals(context.Background(), e039Usuario)
	if err != nil {
		t.Fatalf("el agregado fallo: %v", err)
	}
	if totales.ActiveCount != 100 {
		t.Fatalf("activeCount inesperado: %d", totales.ActiveCount)
	}
	if totales.Saved.Amount() != 10000 {
		t.Fatalf("saved inesperado: %d centimos", totales.Saved.Amount())
	}
	if paginas := memoria.consultas - consultasAntes; paginas != 2 {
		t.Fatalf("150 metas con pagina de 100 deben ser 2 consultas, fueron %d", paginas)
	}
	if !memoria.lecturaConsistente {
		t.Fatal("el agregado debe leer con consistencia fuerte")
	}
}

// TestE038SinDynamo comprueba el 503 de lista y agregado.
func TestE038SinDynamo(t *testing.T) {
	store := dynamoAdapter.NewGoalsStore(nil, "")
	if _, _, err := store.ListUserGoals(context.Background(), appGoals.ListStoreQuery{UserID: e039Usuario, Limit: 10}); !errors.Is(err, appGoals.ErrUnavailable) {
		t.Fatalf("listado: esperado ErrUnavailable, obtenido %v", err)
	}
	if _, err := store.AggregateUserGoals(context.Background(), e039Usuario); !errors.Is(err, appGoals.ErrUnavailable) {
		t.Fatalf("agregado: esperado ErrUnavailable, obtenido %v", err)
	}
}
