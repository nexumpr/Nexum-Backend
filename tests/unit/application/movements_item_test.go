package application_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
)

// Pruebas de E029/E030/E031 en la capa de aplicación.

const (
	itemMovID      = "018f90ef-08e5-7a68-b09f-0000000000aa"
	itemMovIDAjeno = "018f90ef-08e5-7a68-b09f-0000000000bb"
)

// Los dobles de E027/E028 (definidos en movements_test.go) deben seguir satisfaciendo la
// interfaz completa. Sus métodos de ítem fallan cerrados: esas pruebas no los ejercitan.
func (m *movStore) LoadMovement(context.Context, string, string) (*appMovements.Movement, error) {
	return nil, appMovements.ErrUnavailable
}

func (m *movStore) ReplaceMovement(context.Context, appMovements.ReplaceWrite) (*appMovements.Movement, error) {
	return nil, appMovements.ErrUnavailable
}

func (m *movStore) DeleteMovement(context.Context, appMovements.DeleteWrite) error {
	return appMovements.ErrUnavailable
}

// movItemStore implementa los cinco métodos con estado propio.
type movItemStore struct {
	*movStore
	mu           sync.Mutex
	movimientos  map[string]appMovements.Movement // clave: userID + "\x00" + movementID
	replaceFn    func(appMovements.ReplaceWrite) (*appMovements.Movement, error)
	deleteFn     func(appMovements.DeleteWrite) error
	reemplazos   []appMovements.ReplaceWrite
	borrados     []appMovements.DeleteWrite
	lecturas     int
	falloLectura error
}

func newMovItemStore() *movItemStore {
	return &movItemStore{movStore: &movStore{}, movimientos: map[string]appMovements.Movement{}}
}

func itemClave(userID, movementID string) string { return userID + "\x00" + movementID }

func (m *movItemStore) sembrar(userID string, movimiento appMovements.Movement) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.movimientos[itemClave(userID, movimiento.ID)] = movimiento
}

func (m *movItemStore) LoadMovement(_ context.Context, userID, movementID string) (*appMovements.Movement, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lecturas++
	if m.falloLectura != nil {
		return nil, m.falloLectura
	}
	movimiento, ok := m.movimientos[itemClave(userID, movementID)]
	if !ok {
		return nil, nil
	}
	copia := movimiento
	return &copia, nil
}

func (m *movItemStore) ReplaceMovement(_ context.Context, write appMovements.ReplaceWrite) (*appMovements.Movement, error) {
	m.mu.Lock()
	m.reemplazos = append(m.reemplazos, write)
	fn := m.replaceFn
	m.mu.Unlock()
	if fn != nil {
		return fn(write)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.movimientos[itemClave(write.UserID, write.MovementID)] = write.Next
	resultado := write.Next
	return &resultado, nil
}

func (m *movItemStore) DeleteMovement(_ context.Context, write appMovements.DeleteWrite) error {
	m.mu.Lock()
	m.borrados = append(m.borrados, write)
	fn := m.deleteFn
	m.mu.Unlock()
	if fn != nil {
		return fn(write)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	// Tombstone lógico de E031: el ítem NO se elimina; se marca `deleted` y sube la versión.
	clave := itemClave(write.UserID, write.MovementID)
	actual, ok := m.movimientos[clave]
	if !ok {
		return appMovements.ErrConflict
	}
	if actual.Version != write.ExpectedVersion {
		return appMovements.ErrConflict
	}
	actual.Deleted = true
	actual.Version++
	actual.Description = ""
	m.movimientos[clave] = actual
	return nil
}

func (m *movItemStore) ultimoReemplazo(t *testing.T) appMovements.ReplaceWrite {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.reemplazos) == 0 {
		t.Fatal("no se registro ningun reemplazo")
	}
	return m.reemplazos[len(m.reemplazos)-1]
}

func itemMovimiento(id string, ocurrido time.Time) appMovements.Movement {
	return appMovements.Movement{
		ID: id, Type: "expense", AmountCents: 100000, Currency: "MXN",
		CategoryID: "food", Description: "Ejemplo de concepto", OccurredAt: ocurrido,
		Version: 1, CreatedAt: movAhora.Add(-48 * time.Hour),
	}
}

// TestE029Get cubre la lectura por identificador: 200, 404 homogéneo y 400 de forma.
func TestE029Get(t *testing.T) {
	store := newMovItemStore()
	store.sembrar(movUsuario, itemMovimiento(itemMovID, movAhora.Add(-time.Hour)))
	servicio := movServicio(store)

	movimiento, err := servicio.Get(context.Background(), movUsuario, itemMovID)
	if err != nil {
		t.Fatalf("lectura inesperada: %v", err)
	}
	if movimiento.ID != itemMovID || movimiento.AmountCents != 100000 {
		t.Fatalf("movimiento inesperado: %+v", movimiento)
	}

	// Inexistente para este titular (aunque exista para otro) -> 404.
	store.sembrar(movUsuarioB, itemMovimiento(itemMovIDAjeno, movAhora))
	if _, err := servicio.Get(context.Background(), movUsuario, itemMovIDAjeno); !errors.Is(err, appMovements.ErrNotFound) {
		t.Fatalf("un movimiento ajeno debe ser 404, se obtuvo %v", err)
	}

	for _, id := range []string{"", "no-es-uuid", "018f90ef08e57a68b09f0000000000aa", "018f90ef-08e5-7a68-b09f-0000000000zz"} {
		if _, err := servicio.Get(context.Background(), movUsuario, id); !errors.Is(err, appMovements.ErrInvalidRequest) {
			t.Fatalf("id %q debe ser 400, se obtuvo %v", id, err)
		}
	}

	if _, err := servicio.Get(context.Background(), "", itemMovID); !errors.Is(err, appMovements.ErrUnauthorized) {
		t.Fatalf("sin titular se esperaba 401 y se obtuvo %v", err)
	}
	if _, err := appMovements.NewService(nil).Get(context.Background(), movUsuario, itemMovID); !errors.Is(err, appMovements.ErrUnavailable) {
		t.Fatalf("sin almacen se esperaba 503 y se obtuvo %v", err)
	}
}

// TestE030PatchCampos aplica los tres campos y comprueba que la versión sube exactamente uno.
func TestE030PatchCampos(t *testing.T) {
	store := newMovItemStore()
	original := itemMovimiento(itemMovID, movAhora.Add(-time.Hour))
	store.sembrar(movUsuario, original)
	servicio := movServicio(store)

	categoria := "transport"
	descripcion := "  Descripcion actualizada  "
	actualizado, err := servicio.Patch(context.Background(), movUsuario, itemMovID, 1, appMovements.Patch{
		CategoryID:  &categoria,
		Description: &descripcion,
	})
	if err != nil {
		t.Fatalf("modificacion inesperada: %v", err)
	}
	if actualizado.Version != 2 {
		t.Fatalf("version inesperada: %d", actualizado.Version)
	}
	if actualizado.CategoryID != "transport" || actualizado.Description != "Descripcion actualizada" {
		t.Fatalf("campos no aplicados: %+v", actualizado)
	}

	escritura := store.ultimoReemplazo(t)
	if escritura.ExpectedVersion != 1 || escritura.Previous.Version != 1 || escritura.Next.Version != 2 {
		t.Fatalf("versiones inesperadas: %+v", escritura)
	}
	if !escritura.Next.OccurredAt.Equal(original.OccurredAt) {
		t.Fatal("sin cambio de occurredAt la posicion no debe cambiar")
	}
}

// TestE030PatchMueveFecha comprueba que cambiar `occurredAt` cambia la SK del ítem: es el caso que
// exige mover el registro con una transacción.
func TestE030PatchMueveFecha(t *testing.T) {
	store := newMovItemStore()
	original := itemMovimiento(itemMovID, movAhora.Add(-time.Hour))
	store.sembrar(movUsuario, original)
	servicio := movServicio(store)

	nueva := appMovements.FormatOccurredAt(movAhora.Add(-90 * time.Minute))
	actualizado, err := servicio.Patch(context.Background(), movUsuario, itemMovID, 1, appMovements.Patch{OccurredAt: &nueva})
	if err != nil {
		t.Fatalf("modificacion inesperada: %v", err)
	}
	if !actualizado.OccurredAt.Equal(movAhora.Add(-90 * time.Minute)) {
		t.Fatalf("occurredAt inesperado: %v", actualizado.OccurredAt)
	}

	_, skVieja, _, _ := domainDynamo.MovementKeys(movUsuario, itemMovID, original.Type, original.OccurredAt)
	_, skNueva, _, _ := domainDynamo.MovementKeys(movUsuario, itemMovID, actualizado.Type, actualizado.OccurredAt)
	if skVieja == skNueva {
		t.Fatal("el cambio de occurredAt debe cambiar la SK y forzar el movimiento del item")
	}

	escritura := store.ultimoReemplazo(t)
	if !escritura.Next.OccurredAt.Equal(actualizado.OccurredAt) || escritura.Next.Version != 2 {
		t.Fatalf("escritura inesperada: %+v", escritura)
	}
}

// TestE030PatchNoOp comprueba que un parche sin campos no escribe ni sube la versión.
func TestE030PatchNoOp(t *testing.T) {
	store := newMovItemStore()
	store.sembrar(movUsuario, itemMovimiento(itemMovID, movAhora.Add(-time.Hour)))
	servicio := movServicio(store)

	actual, err := servicio.Patch(context.Background(), movUsuario, itemMovID, 1, appMovements.Patch{})
	if err != nil {
		t.Fatalf("no-op inesperado: %v", err)
	}
	if actual.Version != 1 {
		t.Fatalf("un no-op no debe subir la version: %d", actual.Version)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.reemplazos) != 0 {
		t.Fatal("un no-op no debe escribir")
	}
}

// TestE030PatchVersionObsoleta cubre T07: 412 sin escribir, 404 si desapareció y 409 si la
// carrera no cambió la versión.
func TestE030PatchVersionObsoleta(t *testing.T) {
	store := newMovItemStore()
	store.sembrar(movUsuario, itemMovimiento(itemMovID, movAhora.Add(-time.Hour)))
	servicio := movServicio(store)
	descripcion := "otra"

	if _, err := servicio.Patch(context.Background(), movUsuario, itemMovID, 5, appMovements.Patch{Description: &descripcion}); !errors.Is(err, appMovements.ErrVersionConflict) {
		t.Fatalf("version obsoleta debe ser 412, se obtuvo %v", err)
	}
	store.mu.Lock()
	if len(store.reemplazos) != 0 {
		store.mu.Unlock()
		t.Fatal("una version obsoleta no puede escribir")
	}
	store.mu.Unlock()

	// Conflicto del adaptador: si al releer la versión ya cambió, la respuesta es 412.
	enTres := itemMovimiento(itemMovID, movAhora.Add(-time.Hour))
	enTres.Version = 3
	store.sembrar(movUsuario, enTres)
	store.replaceFn = func(appMovements.ReplaceWrite) (*appMovements.Movement, error) {
		// Otro escritor ganó la carrera entre la lectura previa y la transacción.
		enCuatro := itemMovimiento(itemMovID, movAhora.Add(-time.Hour))
		enCuatro.Version = 4
		store.sembrar(movUsuario, enCuatro)
		return nil, appMovements.ErrConflict
	}
	if _, err := servicio.Patch(context.Background(), movUsuario, itemMovID, 3, appMovements.Patch{Description: &descripcion}); !errors.Is(err, appMovements.ErrVersionConflict) {
		t.Fatalf("tras releer una version distinta se esperaba 412, se obtuvo %v", err)
	}

	// Conflicto sin cambio de versión -> 409.
	store.replaceFn = func(appMovements.ReplaceWrite) (*appMovements.Movement, error) {
		return nil, appMovements.ErrConflict
	}
	store.sembrar(movUsuario, itemMovimiento(itemMovID, movAhora.Add(-time.Hour)))
	if _, err := servicio.Patch(context.Background(), movUsuario, itemMovID, 1, appMovements.Patch{Description: &descripcion}); !errors.Is(err, appMovements.ErrConflict) {
		t.Fatalf("sin cambio de version se esperaba 409, se obtuvo %v", err)
	}

	// El ítem desapareció -> 404.
	store.sembrar(movUsuarioB, itemMovimiento(itemMovID, movAhora.Add(-time.Hour)))
	store.mu.Lock()
	delete(store.movimientos, itemClave(movUsuario, itemMovID))
	store.mu.Unlock()
	if _, err := servicio.Patch(context.Background(), movUsuario, itemMovID, 1, appMovements.Patch{Description: &descripcion}); !errors.Is(err, appMovements.ErrNotFound) {
		t.Fatalf("si desaparecio se esperaba 404, se obtuvo %v", err)
	}
}

// TestE030PatchValidaciones cubre 422 de dominio y 400 de forma.
func TestE030PatchValidaciones(t *testing.T) {
	store := newMovItemStore()
	store.sembrar(movUsuario, itemMovimiento(itemMovID, movAhora.Add(-time.Hour)))
	servicio := movServicio(store)

	vacia := "   "
	larga := strings.Repeat("a", appMovements.MaxDescriptionLength+1)
	conEspacios := " food "
	fechaMala := "05/11/2026"

	casos := []struct {
		nombre    string
		patch     appMovements.Patch
		violacion bool
	}{
		{"descripcion vacia", appMovements.Patch{Description: &vacia}, true},
		{"descripcion larga", appMovements.Patch{Description: &larga}, true},
		{"categoria con espacios", appMovements.Patch{CategoryID: &conEspacios}, true},
		{"fecha invalida", appMovements.Patch{OccurredAt: &fechaMala}, false},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			_, err := servicio.Patch(context.Background(), movUsuario, itemMovID, 1, caso.patch)
			var violacion *appMovements.FieldViolation
			if caso.violacion {
				if !errors.As(err, &violacion) {
					t.Fatalf("se esperaba 422 y se obtuvo %v", err)
				}
				return
			}
			if !errors.Is(err, appMovements.ErrInvalidRequest) {
				t.Fatalf("se esperaba 400 y se obtuvo %v", err)
			}
		})
	}

	if _, err := servicio.Patch(context.Background(), movUsuario, itemMovID, 0, appMovements.Patch{Description: &vacia}); !errors.Is(err, appMovements.ErrInvalidRequest) {
		t.Fatalf("If-Match sin version debe ser 400, se obtuvo %v", err)
	}
	if _, err := servicio.Patch(context.Background(), movUsuario, "no-uuid", 1, appMovements.Patch{}); !errors.Is(err, appMovements.ErrInvalidRequest) {
		t.Fatalf("id mal formado debe ser 400, se obtuvo %v", err)
	}
}

// TestE031Delete cubre 204, 409 por aportación y 404.
func TestE031Delete(t *testing.T) {
	store := newMovItemStore()
	movimiento := itemMovimiento(itemMovID, movAhora.Add(-time.Hour))
	store.sembrar(movUsuario, movimiento)
	servicio := movServicio(store)

	if err := servicio.Delete(context.Background(), movUsuario, itemMovID); err != nil {
		t.Fatalf("borrado inesperado: %v", err)
	}
	store.mu.Lock()
	if len(store.borrados) != 1 {
		store.mu.Unlock()
		t.Fatalf("se esperaba un borrado y hubo %d", len(store.borrados))
	}
	store.mu.Unlock()

	// Ligado a una aportación confirmada -> 409 sin borrar.
	ligado := itemMovimiento(itemMovID, movAhora.Add(-time.Hour))
	ligado.ContributionID = itemMovIDAjeno
	store.sembrar(movUsuario, ligado)
	if err := servicio.Delete(context.Background(), movUsuario, itemMovID); !errors.Is(err, appMovements.ErrContributionLinked) {
		t.Fatalf("una aportacion confirmada debe ser 409, se obtuvo %v", err)
	}
	store.mu.Lock()
	if len(store.borrados) != 1 {
		store.mu.Unlock()
		t.Fatal("un 409 no puede borrar nada")
	}
	store.mu.Unlock()

	// Inexistente -> 404.
	if err := servicio.Delete(context.Background(), movUsuario, itemMovIDAjeno); !errors.Is(err, appMovements.ErrNotFound) {
		t.Fatalf("inexistente debe ser 404, se obtuvo %v", err)
	}
	// Id mal formado -> 400.
	if err := servicio.Delete(context.Background(), movUsuario, "x"); !errors.Is(err, appMovements.ErrInvalidRequest) {
		t.Fatalf("id mal formado debe ser 400, se obtuvo %v", err)
	}
}

// TestE031DeleteIdempotenteYLectores cubre la autorización
// APRUEBO_E031_IDEMPOTENTE_204_CON_PUNTERO: el segundo DELETE devuelve 204 sin escribir, un
// tombstone no se lee (404) ni se edita (404) y su versión no retrocede.
func TestE031DeleteIdempotenteYLectores(t *testing.T) {
	store := newMovItemStore()
	store.sembrar(movUsuario, itemMovimiento(itemMovID, movAhora.Add(-time.Hour)))
	servicio := movServicio(store)

	if err := servicio.Delete(context.Background(), movUsuario, itemMovID); err != nil {
		t.Fatalf("primer borrado inesperado: %v", err)
	}
	// Segundo DELETE: mismo efecto, 204, sin una segunda escritura ni versión nueva.
	if err := servicio.Delete(context.Background(), movUsuario, itemMovID); err != nil {
		t.Fatalf("segundo borrado debe ser idempotente: %v", err)
	}
	store.mu.Lock()
	if len(store.borrados) != 1 {
		store.mu.Unlock()
		t.Fatalf("el segundo DELETE no puede escribir: %d", len(store.borrados))
	}
	tombstone := store.movimientos[itemClave(movUsuario, itemMovID)]
	store.mu.Unlock()
	if !tombstone.Deleted || tombstone.Version != 2 {
		t.Fatalf("el tombstone debe conservarse con version 2: %+v", tombstone)
	}

	// Un movimiento borrado no es legible ni editable: 404 homogéneo, sin revelar su existencia.
	if _, err := servicio.Get(context.Background(), movUsuario, itemMovID); !errors.Is(err, appMovements.ErrNotFound) {
		t.Fatalf("leer un tombstone debe ser 404, se obtuvo %v", err)
	}
	descripcion := "otra"
	if _, err := servicio.Patch(context.Background(), movUsuario, itemMovID, 2, appMovements.Patch{Description: &descripcion}); !errors.Is(err, appMovements.ErrNotFound) {
		t.Fatalf("editar un tombstone debe ser 404, se obtuvo %v", err)
	}
	store.mu.Lock()
	escrituras := len(store.reemplazos)
	store.mu.Unlock()
	if escrituras != 0 {
		t.Fatal("un PATCH rechazado no puede escribir")
	}

	// Aislamiento: el borrado de A no afecta a un movimiento B con el mismo identificador.
	store.sembrar(movUsuarioB, itemMovimiento(itemMovID, movAhora.Add(-time.Hour)))
	if _, err := servicio.Get(context.Background(), movUsuarioB, itemMovID); err != nil {
		t.Fatalf("el movimiento de B debe seguir visible: %v", err)
	}
}

// TestE031DeleteTombstonePresenteEnAlmacen comprueba que el borrado es lógico: el ítem sigue en
// el almacén y solo cambia de estado.
func TestE031DeleteTombstonePresenteEnAlmacen(t *testing.T) {
	store := newMovItemStore()
	original := itemMovimiento(itemMovID, movAhora.Add(-time.Hour))
	store.sembrar(movUsuario, original)

	if err := movServicio(store).Delete(context.Background(), movUsuario, itemMovID); err != nil {
		t.Fatalf("borrado inesperado: %v", err)
	}
	store.mu.Lock()
	guardado, existe := store.movimientos[itemClave(movUsuario, itemMovID)]
	escritura := appMovements.DeleteWrite{}
	if len(store.borrados) > 0 {
		escritura = store.borrados[0]
	}
	totalBorrados := len(store.borrados)
	store.mu.Unlock()
	if !existe {
		t.Fatal("el movimiento no puede desaparecer del almacen: es un tombstone")
	}
	if !guardado.Deleted || guardado.Version != original.Version+1 || guardado.AmountCents != original.AmountCents {
		t.Fatalf("tombstone inesperado: %+v", guardado)
	}
	if totalBorrados != 1 || escritura.ExpectedVersion != original.Version {
		t.Fatalf("la escritura debe declarar la version leida: %+v", escritura)
	}
}

// TestE031DeleteReintentaTrasMovimiento comprueba que, si el ítem se movió entre la lectura y la
// transacción, se relee y se reintenta con la posición nueva.
func TestE031DeleteReintentaTrasMovimiento(t *testing.T) {
	store := newMovItemStore()
	store.sembrar(movUsuario, itemMovimiento(itemMovID, movAhora.Add(-time.Hour)))
	servicio := movServicio(store)

	var intentos int
	store.deleteFn = func(write appMovements.DeleteWrite) error {
		intentos++
		if intentos == 1 {
			// Simula un PATCH concurrente que movió el ítem: la primera posición ya no existe.
			movido := write.Previous
			movido.OccurredAt = movAhora.Add(-30 * time.Minute)
			store.sembrar(write.UserID, movido)
			return appMovements.ErrConflict
		}
		return nil
	}

	if err := servicio.Delete(context.Background(), movUsuario, itemMovID); err != nil {
		t.Fatalf("el reintento debia completar el borrado: %v", err)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if intentos != 2 || len(store.borrados) != 2 {
		t.Fatalf("se esperaban dos intentos: %d / %d", intentos, len(store.borrados))
	}
	if store.borrados[1].Previous.OccurredAt.Equal(store.borrados[0].Previous.OccurredAt) {
		t.Fatal("el reintento debe usar la posicion releida, no la original")
	}
}

// TestE031DeleteAgotaReintentos comprueba que dos conflictos seguidos no producen un bucle.
func TestE031DeleteAgotaReintentos(t *testing.T) {
	store := newMovItemStore()
	store.sembrar(movUsuario, itemMovimiento(itemMovID, movAhora.Add(-time.Hour)))
	store.deleteFn = func(appMovements.DeleteWrite) error { return appMovements.ErrConflict }

	if err := movServicio(store).Delete(context.Background(), movUsuario, itemMovID); !errors.Is(err, appMovements.ErrConflict) {
		t.Fatalf("se esperaba 409 tras agotar reintentos y se obtuvo %v", err)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.borrados) != 2 {
		t.Fatalf("se esperaban exactamente dos intentos y hubo %d", len(store.borrados))
	}
}

// TestE030PatchRechazaMovimientoVinculado cubre T-1: un movimiento con contributionId no puede
// cambiar categoryId, description ni occurredAt. Un movimiento independiente conserva E030.
func TestE030PatchRechazaMovimientoVinculado(t *testing.T) {
	store := newMovItemStore()
	ligado := itemMovimiento(itemMovID, movAhora.Add(-time.Hour))
	ligado.ContributionID = itemMovIDAjeno
	store.sembrar(movUsuario, ligado)
	servicio := movServicio(store)

	descripcion := "Otra descripcion"
	if _, err := servicio.Patch(context.Background(), movUsuario, itemMovID, 1, appMovements.Patch{Description: &descripcion}); !errors.Is(err, appMovements.ErrContributionLinked) {
		t.Fatalf("movimiento vinculado debe ser 409, se obtuvo %v", err)
	}
	store.mu.Lock()
	if len(store.reemplazos) != 0 {
		store.mu.Unlock()
		t.Fatal("T-1 no puede escribir nada")
	}
	store.mu.Unlock()

	// Movimiento independiente: el comportamiento anterior se conserva.
	independiente := newMovItemStore()
	independiente.sembrar(movUsuario, itemMovimiento(itemMovID, movAhora.Add(-time.Hour)))
	servicioIndependiente := movServicio(independiente)
	if _, err := servicioIndependiente.Patch(context.Background(), movUsuario, itemMovID, 1, appMovements.Patch{Description: &descripcion}); err != nil {
		t.Fatalf("movimiento independiente debe poder editarse: %v", err)
	}
}
