package dynamo_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
)

// Pruebas del adaptador para E029/E030/E031 sobre el doble con estado definido en
// movements_store_test.go (mismo paquete de pruebas).

const itemDynID = "018f90ef-08e5-7a68-b09f-0000000000aa"

// eliminar borra una clave del doble sin pasar por una transacción (para simular estados
// parciales que no puede producir el adaptador).
func (m *movDynamo) eliminar(pk, sk string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.items, claveDe(pk, sk))
}

// itemDynSembrar crea un movimiento por la vía real (transacción de E028) y lo devuelve.
func itemDynSembrar(t *testing.T, doble *movDynamo, store *dynamoAdapter.MovementsStore, ocurrido time.Time) appMovements.Movement {
	t.Helper()
	movimiento := movDynMovimiento(itemDynID, ocurrido, "expense")
	escritura := movDynEscritura(movDynUsuario, movimiento, "hash-"+movimiento.ID, movDynClave, movDynAhora)
	if _, err := store.CreateMovement(context.Background(), escritura); err != nil {
		t.Fatalf("alta de preparacion inesperada: %v", err)
	}
	return movimiento
}

// TestE029StoreCargaPorPuntero comprueba la resolución O(1) y que un recurso ajeno no se localiza.
func TestE029StoreCargaPorPuntero(t *testing.T) {
	doble := newMovDynamo()
	store := dynamoAdapter.NewMovementsStore(doble, movDynTabla)
	creado := itemDynSembrar(t, doble, store, movDynAhora.Add(-time.Hour))

	movimiento, err := store.LoadMovement(context.Background(), movDynUsuario, creado.ID)
	if err != nil {
		t.Fatalf("lectura inesperada: %v", err)
	}
	if movimiento == nil {
		t.Fatal("el movimiento propio debe poder leerse")
	}
	if movimiento.ID != creado.ID || movimiento.AmountCents != creado.AmountCents ||
		movimiento.CategoryID != creado.CategoryID || movimiento.Version != 1 {
		t.Fatalf("movimiento leido inesperado: %+v", movimiento)
	}
	if movimiento.CreatedAt.IsZero() {
		t.Fatal("createdAt debe leerse: E030 lo conserva al mover el item")
	}
	// Dos GetItem exactos: ninguna consulta de partición (nada de Query/Scan).
	if doble.consultas != 0 {
		t.Fatalf("la lectura por id no debe consultar la particion: %d", doble.consultas)
	}

	ajeno, err := store.LoadMovement(context.Background(), movDynUsuarioB, creado.ID)
	if err != nil {
		t.Fatalf("lectura B inesperada: %v", err)
	}
	if ajeno != nil {
		t.Fatal("un movimiento de otra particion no puede localizarse")
	}
}

// TestE029StorePunteroRoto comprueba que un localizador sin destino utilizable no se interpreta
// como un movimiento existente.
func TestE029StorePunteroRoto(t *testing.T) {
	doble := newMovDynamo()
	store := dynamoAdapter.NewMovementsStore(doble, movDynTabla)

	ptrPK, ptrSK := dynamoAdapter.MovementPointerKeys(movDynUsuario, itemDynID)
	doble.sembrar(ptrPK, ptrSK, map[string]types.AttributeValue{})

	movimiento, err := store.LoadMovement(context.Background(), movDynUsuario, itemDynID)
	if err != nil {
		t.Fatalf("lectura inesperada: %v", err)
	}
	if movimiento != nil {
		t.Fatal("un puntero sin targetSK no debe resolver un movimiento")
	}
}

// TestE030StoreSinMover comprueba el CAS cuando `occurredAt` no cambia.
func TestE030StoreSinMover(t *testing.T) {
	doble := newMovDynamo()
	store := dynamoAdapter.NewMovementsStore(doble, movDynTabla)
	creado := itemDynSembrar(t, doble, store, movDynAhora.Add(-time.Hour))

	actual, err := store.LoadMovement(context.Background(), movDynUsuario, creado.ID)
	if err != nil || actual == nil {
		t.Fatalf("lectura de preparacion inesperada: %v", err)
	}
	siguiente := *actual
	siguiente.CategoryID = "transport"
	siguiente.Description = "Descripcion actualizada"
	siguiente.Version = 2

	resultado, err := store.ReplaceMovement(context.Background(), appMovements.ReplaceWrite{
		UserID: movDynUsuario, MovementID: creado.ID, ExpectedVersion: 1,
		Previous: *actual, Next: siguiente, Now: movDynAhora,
	})
	if err != nil {
		t.Fatalf("modificacion inesperada: %v", err)
	}
	if resultado.Version != 2 || resultado.CategoryID != "transport" {
		t.Fatalf("resultado inesperado: %+v", resultado)
	}

	releido, err := store.LoadMovement(context.Background(), movDynUsuario, creado.ID)
	if err != nil || releido == nil {
		t.Fatalf("relectura inesperada: %v", err)
	}
	if releido.Version != 2 || releido.CategoryID != "transport" {
		t.Fatalf("el item no se actualizo: %+v", releido)
	}
	if total := doble.contar(domainDynamo.PrefixUser+movDynUsuario, domainDynamo.PrefixMovement); total != 1 {
		t.Fatalf("sin cambio de clave debe seguir habiendo un unico item: %d", total)
	}

	// La versión esperada ya no es la vigente: la transacción se cancela sin escribir.
	siguiente2 := *releido
	siguiente2.Version = 3
	if _, err := store.ReplaceMovement(context.Background(), appMovements.ReplaceWrite{
		UserID: movDynUsuario, MovementID: creado.ID, ExpectedVersion: 1,
		Previous: *releido, Next: siguiente2, Now: movDynAhora,
	}); !errors.Is(err, appMovements.ErrConflict) {
		t.Fatalf("una version obsoleta debe devolver conflicto, se obtuvo %v", err)
	}
}

// TestE030StoreMueveFecha comprueba que cambiar `occurredAt` mueve el ítem y actualiza el
// localizador en la misma transacción.
func TestE030StoreMueveFecha(t *testing.T) {
	doble := newMovDynamo()
	store := dynamoAdapter.NewMovementsStore(doble, movDynTabla)
	creado := itemDynSembrar(t, doble, store, movDynAhora.Add(-2*time.Hour))

	actual, err := store.LoadMovement(context.Background(), movDynUsuario, creado.ID)
	if err != nil || actual == nil {
		t.Fatalf("lectura de preparacion inesperada: %v", err)
	}
	nueva := movDynAhora.Add(-90 * time.Minute)
	siguiente := *actual
	siguiente.OccurredAt = nueva
	siguiente.Version = 2

	if _, err := store.ReplaceMovement(context.Background(), appMovements.ReplaceWrite{
		UserID: movDynUsuario, MovementID: creado.ID, ExpectedVersion: 1,
		Previous: *actual, Next: siguiente, Now: movDynAhora,
	}); err != nil {
		t.Fatalf("movimiento inesperado: %v", err)
	}

	_, skVieja, _, _ := domainDynamo.MovementKeys(movDynUsuario, creado.ID, actual.Type, actual.OccurredAt)
	_, skNueva, _, _ := domainDynamo.MovementKeys(movDynUsuario, creado.ID, siguiente.Type, siguiente.OccurredAt)
	if skVieja == skNueva {
		t.Fatal("el cambio de occurredAt debe producir otra SK")
	}
	if doble.leer(domainDynamo.PrefixUser+movDynUsuario, skVieja) != nil {
		t.Fatal("la posicion antigua debe desaparecer")
	}
	if doble.leer(domainDynamo.PrefixUser+movDynUsuario, skNueva) == nil {
		t.Fatal("la posicion nueva debe existir")
	}

	ptrPK, ptrSK := dynamoAdapter.MovementPointerKeys(movDynUsuario, creado.ID)
	puntero := doble.leer(ptrPK, ptrSK)
	if puntero == nil || mapaString(puntero, "targetSK") != skNueva {
		t.Fatalf("el puntero debe apuntar a la posicion nueva: %+v", puntero)
	}
	if total := doble.contar(domainDynamo.PrefixUser+movDynUsuario, domainDynamo.PrefixMovement); total != 1 {
		t.Fatalf("tras mover debe seguir habiendo un unico item: %d", total)
	}

	releido, err := store.LoadMovement(context.Background(), movDynUsuario, creado.ID)
	if err != nil || releido == nil {
		t.Fatalf("relectura inesperada: %v", err)
	}
	if !releido.OccurredAt.Equal(nueva) || releido.Version != 2 {
		t.Fatalf("el movimiento tras el cambio de fecha es inesperado: %+v", releido)
	}
	if !releido.CreatedAt.Equal(actual.CreatedAt) {
		t.Fatal("createdAt debe conservarse al mover el item")
	}
}

// TestE031StoreBorraPrincipalYPuntero comprueba el 204 real: ambos ítems desaparecen.
func TestE031StoreBorraPrincipalYPuntero(t *testing.T) {
	doble := newMovDynamo()
	store := dynamoAdapter.NewMovementsStore(doble, movDynTabla)
	creado := itemDynSembrar(t, doble, store, movDynAhora.Add(-time.Hour))

	actual, err := store.LoadMovement(context.Background(), movDynUsuario, creado.ID)
	if err != nil || actual == nil {
		t.Fatalf("lectura de preparacion inesperada: %v", err)
	}
	if err := store.DeleteMovement(context.Background(), appMovements.DeleteWrite{
		UserID: movDynUsuario, MovementID: creado.ID, Previous: *actual, Now: movDynAhora,
	}); err != nil {
		t.Fatalf("borrado inesperado: %v", err)
	}

	if total := doble.contar(domainDynamo.PrefixUser+movDynUsuario, domainDynamo.PrefixMovement); total != 0 {
		t.Fatalf("el item principal debe desaparecer: %d", total)
	}
	ptrPK, ptrSK := dynamoAdapter.MovementPointerKeys(movDynUsuario, creado.ID)
	if doble.leer(ptrPK, ptrSK) != nil {
		t.Fatal("el localizador debe desaparecer")
	}
	if restante, err := store.LoadMovement(context.Background(), movDynUsuario, creado.ID); err != nil || restante != nil {
		t.Fatalf("tras borrar no debe resolverse: %v / %+v", err, restante)
	}

	// Repetir el borrado con la posicion antigua ya no puede confirmar nada.
	if err := store.DeleteMovement(context.Background(), appMovements.DeleteWrite{
		UserID: movDynUsuario, MovementID: creado.ID, Previous: *actual, Now: movDynAhora,
	}); !errors.Is(err, appMovements.ErrConflict) {
		t.Fatalf("un segundo borrado debe ser conflicto, se obtuvo %v", err)
	}
}

// TestE031StorePunteroAusente comprueba que un localizador ya desaparecido no impide borrar el
// ítem principal (autoridad).
func TestE031StorePunteroAusente(t *testing.T) {
	doble := newMovDynamo()
	store := dynamoAdapter.NewMovementsStore(doble, movDynTabla)
	creado := itemDynSembrar(t, doble, store, movDynAhora.Add(-time.Hour))

	actual, err := store.LoadMovement(context.Background(), movDynUsuario, creado.ID)
	if err != nil || actual == nil {
		t.Fatalf("lectura de preparacion inesperada: %v", err)
	}
	ptrPK, ptrSK := dynamoAdapter.MovementPointerKeys(movDynUsuario, creado.ID)
	doble.eliminar(ptrPK, ptrSK)

	if err := store.DeleteMovement(context.Background(), appMovements.DeleteWrite{
		UserID: movDynUsuario, MovementID: creado.ID, Previous: *actual, Now: movDynAhora,
	}); err != nil {
		t.Fatalf("sin localizador el borrado del principal debe completarse: %v", err)
	}
	if total := doble.contar(domainDynamo.PrefixUser+movDynUsuario, domainDynamo.PrefixMovement); total != 0 {
		t.Fatalf("el item principal debe desaparecer: %d", total)
	}
}

// TestE029E030E031StoreFailClosed comprueba que sin persistencia no se inventa nada.
func TestE029E030E031StoreFailClosed(t *testing.T) {
	store := dynamoAdapter.NewMovementsStore(nil, "")
	if _, err := store.LoadMovement(context.Background(), movDynUsuario, itemDynID); !errors.Is(err, appMovements.ErrUnavailable) {
		t.Fatalf("se esperaba 503 y se obtuvo %v", err)
	}
	if _, err := store.ReplaceMovement(context.Background(), appMovements.ReplaceWrite{}); !errors.Is(err, appMovements.ErrUnavailable) {
		t.Fatalf("se esperaba 503 y se obtuvo %v", err)
	}
	if err := store.DeleteMovement(context.Background(), appMovements.DeleteWrite{}); !errors.Is(err, appMovements.ErrUnavailable) {
		t.Fatalf("se esperaba 503 y se obtuvo %v", err)
	}
}
