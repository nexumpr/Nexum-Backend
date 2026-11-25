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

// movBool lee un atributo BOOL del doble (false si no existe o es de otro tipo).
func movBool(item map[string]types.AttributeValue, nombre string) bool {
	valor, ok := item[nombre].(*types.AttributeValueMemberBOOL)
	return ok && valor.Value
}

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

// TestE031StoreTombstonePrincipalYPuntero comprueba el 204 real: el ítem principal NO
// desaparece, se convierte en tombstone y el localizador se CONSERVA marcado.
func TestE031StoreTombstonePrincipalYPuntero(t *testing.T) {
	doble := newMovDynamo()
	store := dynamoAdapter.NewMovementsStore(doble, movDynTabla)
	creado := itemDynSembrar(t, doble, store, movDynAhora.Add(-time.Hour))

	actual, err := store.LoadMovement(context.Background(), movDynUsuario, creado.ID)
	if err != nil || actual == nil {
		t.Fatalf("lectura de preparacion inesperada: %v", err)
	}
	pk, sk, _, _ := domainDynamo.MovementKeys(movDynUsuario, creado.ID, actual.Type, actual.OccurredAt)
	if err := store.DeleteMovement(context.Background(), appMovements.DeleteWrite{
		UserID: movDynUsuario, MovementID: creado.ID, ExpectedVersion: actual.Version,
		Previous: *actual, Now: movDynAhora,
	}); err != nil {
		t.Fatalf("tombstone inesperado: %v", err)
	}

	// El movimiento NO desaparece de DynamoDB: sigue exactamente en la misma clave.
	if total := doble.contar(domainDynamo.PrefixUser+movDynUsuario, domainDynamo.PrefixMovement); total != 1 {
		t.Fatalf("el item principal debe conservarse como tombstone: %d", total)
	}
	principal := doble.leer(pk, sk)
	if principal == nil {
		t.Fatal("el item principal debe existir")
	}
	if !movBool(principal, "deleted") {
		t.Fatalf("deleted debe ser true: %+v", principal)
	}
	if mapaString(principal, "deletedBy") != "USER" || mapaString(principal, "deletedAt") == "" {
		t.Fatalf("metadatos de borrado inesperados: %+v", principal)
	}
	if movValorTexto(principal["version"]) != "2" {
		t.Fatalf("la version debe subir a 2: %q", movValorTexto(principal["version"]))
	}
	if _, existe := principal["description"]; existe {
		t.Fatal("description debe retirarse del tombstone")
	}
	if _, existe := principal["GSI1PK"]; existe {
		t.Fatal("GSI1PK debe retirarse para que el borrado salga del indice")
	}
	if _, existe := principal["GSI1SK"]; existe {
		t.Fatal("GSI1SK debe retirarse para que el borrado salga del indice")
	}
	// El importe se conserva: el tombstone mantiene los hechos financieros minimos.
	if movValorTexto(principal["amountCents"]) != "100000" {
		t.Fatalf("el importe debe conservarse: %q", movValorTexto(principal["amountCents"]))
	}

	// El puntero se CONSERVA (autorizacion APRUEBO_E031_IDEMPOTENTE_204_CON_PUNTERO), marcado.
	ptrPK, ptrSK := dynamoAdapter.MovementPointerKeys(movDynUsuario, creado.ID)
	puntero := doble.leer(ptrPK, ptrSK)
	if puntero == nil {
		t.Fatal("el localizador NO debe eliminarse")
	}
	if !movBool(puntero, "deleted") {
		t.Fatal("el localizador debe quedar marcado como borrado")
	}
	if mapaString(puntero, "targetSK") != sk {
		t.Fatalf("el localizador debe conservar targetSK: %q", mapaString(puntero, "targetSK"))
	}
	// Sigue resolviendose por id, pero como tombstone (el servicio lo traduce a 404/204).
	restante, err := store.LoadMovement(context.Background(), movDynUsuario, creado.ID)
	if err != nil || restante == nil || !restante.Deleted {
		t.Fatalf("tras borrar debe resolverse como tombstone: %v / %+v", err, restante)
	}

	// Repetir el borrado con la version ya consumida no vuelve a escribir: es un conflicto que
	// el servicio convierte en 204 idempotente sin subir la version.
	if err := store.DeleteMovement(context.Background(), appMovements.DeleteWrite{
		UserID: movDynUsuario, MovementID: creado.ID, ExpectedVersion: actual.Version,
		Previous: *actual, Now: movDynAhora,
	}); !errors.Is(err, appMovements.ErrConflict) {
		t.Fatalf("un segundo tombstone debe ser conflicto, se obtuvo %v", err)
	}
	if tras := doble.leer(pk, sk); movValorTexto(tras["version"]) != "2" {
		t.Fatalf("el segundo borrado no puede subir la version: %q", movValorTexto(tras["version"]))
	}

	// Un movimiento borrado no puede reemplazarse (E030): la condicion incluye `deleted=false`.
	siguiente := *restante
	siguiente.Version = restante.Version + 1
	if _, err := store.ReplaceMovement(context.Background(), appMovements.ReplaceWrite{
		UserID: movDynUsuario, MovementID: creado.ID, ExpectedVersion: restante.Version,
		Previous: *restante, Next: siguiente, Now: movDynAhora,
	}); !errors.Is(err, appMovements.ErrConflict) {
		t.Fatalf("editar un tombstone debe ser conflicto, se obtuvo %v", err)
	}
	if tras := doble.leer(pk, sk); movValorTexto(tras["version"]) != "2" {
		t.Fatalf("la edicion rechazada no puede subir la version: %q", movValorTexto(tras["version"]))
	}
}

// TestE031StorePunteroAusente comprueba que un localizador ya desaparecido no impide escribir el
// tombstone del ítem principal (autoridad).
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
		UserID: movDynUsuario, MovementID: creado.ID, ExpectedVersion: actual.Version,
		Previous: *actual, Now: movDynAhora,
	}); err != nil {
		t.Fatalf("sin localizador el tombstone del principal debe completarse: %v", err)
	}
	pk, sk, _, _ := domainDynamo.MovementKeys(movDynUsuario, creado.ID, actual.Type, actual.OccurredAt)
	principal := doble.leer(pk, sk)
	if principal == nil || !movBool(principal, "deleted") {
		t.Fatalf("el item principal debe quedar como tombstone: %+v", principal)
	}
	if doble.leer(ptrPK, ptrSK) != nil {
		t.Fatal("el puntero ausente no debe reaparecer")
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
