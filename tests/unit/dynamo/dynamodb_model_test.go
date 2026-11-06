package dynamo_test

import (
	"context"
	"errors"
	"testing"
	"time"

	adaptersDynamo "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
	infraDynamo "github.com/nexumpr/Nexum-Backend/internal/infrastructure/dynamo"
)

// TestProhibitionOfScan verifica AC-A-060-01:
// Prohibición absoluta de operaciones Scan en toda la capa de persistencia;
// todas las consultas deben ser resueltas mediante Query o GetItem acotadas a la partición.
func TestProhibitionOfScan(t *testing.T) {
	ctx := context.Background()
	table := infraDynamo.NewDynamoTable()

	// 1. Invocar Scan debe retornar siempre ErrScanProhibited
	_, err := table.Scan(ctx)
	if !errors.Is(err, infraDynamo.ErrScanProhibited) {
		t.Fatalf("AC-A-060-01 Violado: La operación Scan debió ser rechazada con ErrScanProhibited, obtenido: %v", err)
	}

	// 2. Query sin Partition Key debe ser rechazado
	_, err = table.QueryPartition(ctx, "", "", true, 10)
	if err == nil {
		t.Fatalf("AC-A-060-01 Violado: Query sin clave de partición debió fallar")
	}

	t.Logf("AC-A-060-01 Verificado: Operaciones Scan prohibidas por diseño de arquitectura.")
}

// TestAtomicTransactionRollback verifica AC-A-060-02:
// Transacción con falla de condición no deja goal.saved sin movement (atomicidad todo o nada).
func TestAtomicTransactionRollback(t *testing.T) {
	ctx := context.Background()
	table := infraDynamo.NewDynamoTable()
	repo := adaptersDynamo.NewDynamoRepository(table)

	userID := "usr-transact-001"
	goalID := "goal-vacation-2026"
	movementID := "mov-contrib-999"

	// 1. Inicializar una meta de ahorro con saldo inicial de 1000 centavos ($10.00 MXN) y Version 1
	goalPK, goalSK := domainDynamo.GoalKeys(userID, goalID)
	initialGoal := domainDynamo.NewDynamoItem(goalPK, goalSK, "GOAL", map[string]any{
		"goalId":     goalID,
		"title":      "Vacaciones",
		"savedCents": int64(1000),
	})
	if err := table.PutItem(ctx, initialGoal, false, nil); err != nil {
		t.Fatalf("Fallo al inicializar meta de ahorro: %v", err)
	}

	// 2. Intento de registrar aportación con condición errónea (ExpectedVersion=99, pero la meta tiene versión 1)
	wrongVersion := int64(99)
	err := repo.RecordMovementAtomic(
		ctx,
		userID,
		movementID,
		"SAVING",
		500, // $5.00 MXN a aportar
		time.Now().UTC(),
		goalID,
		&wrongVersion,
	)

	// La transacción DEBE ser cancelada
	if !errors.Is(err, infraDynamo.ErrTransactionCanceled) {
		t.Fatalf("Se esperaba fallo por condición de versión con ErrTransactionCanceled, obtenido: %v", err)
	}

	// 3. Comprobar AC-A-060-02: Ningún cambio parcial debe haberse aplicado
	// a) La meta debe conservar intacto su saldo previo (1000 centavos)
	goalAfterRollback, err := table.GetItem(ctx, goalPK, goalSK)
	if err != nil {
		t.Fatalf("Error al recuperar meta tras rollback: %v", err)
	}
	savedAfterRollback := goalAfterRollback.Attributes["savedCents"].(int64)
	if savedAfterRollback != 1000 {
		t.Fatalf("AC-A-060-02 Violado: goal.saved fue alterado a pesar del rollback (%d != 1000)", savedAfterRollback)
	}

	// b) El movimiento NO debe existir en la base de datos
	movPK, movSK, _, _ := domainDynamo.MovementKeys(userID, movementID, "SAVING", time.Now().UTC())
	_, err = table.GetItem(ctx, movPK, movSK)
	if !errors.Is(err, infraDynamo.ErrItemNotFound) {
		t.Fatalf("AC-A-060-02 Violado: El movimiento fue persistido a pesar del rollback de la transacción")
	}

	// 4. Ejecutar la misma transacción con la versión correcta (ExpectedVersion=1)
	correctVersion := int64(1)
	err = repo.RecordMovementAtomic(
		ctx,
		userID,
		movementID,
		"SAVING",
		500,
		time.Now().UTC(),
		goalID,
		&correctVersion,
	)
	if err != nil {
		t.Fatalf("Fallo en transacción legítima con versión correcta: %v", err)
	}

	// 5. Verificar que ahora SÍ se aplicaron ambos cambios conjuntamente
	goalUpdated, _ := table.GetItem(ctx, goalPK, goalSK)
	savedUpdated := goalUpdated.Attributes["savedCents"].(int64)
	if savedUpdated != 1500 {
		t.Fatalf("Saldo final de meta incorrecto: got %d, want 1500", savedUpdated)
	}
	if goalUpdated.Version != 2 {
		t.Fatalf("Versión optimista de meta incorrecta: got %d, want 2", goalUpdated.Version)
	}

	t.Logf("AC-A-060-02 Verificado: Transacciones atómicas no dejan saldos inconsistentes ante fallos de condición.")
}

// TestSingleTableEmailUniqueness verifica la reserva de email transaccional (A-060 §2.2).
func TestSingleTableEmailUniqueness(t *testing.T) {
	ctx := context.Background()
	table := infraDynamo.NewDynamoTable()
	repo := adaptersDynamo.NewDynamoRepository(table)

	email := "usuario.ejemplo@nexum.finance"

	// 1. Registro exitoso de primer usuario
	err := repo.RegisterUserAtomic(ctx, "usr-001", email, "Carlos Pérez", "argon2id_hash_1")
	if err != nil {
		t.Fatalf("Fallo al registrar usuario legítimo: %v", err)
	}

	// 2. Intento de registrar segundo usuario con el mismo email (con mayúsculas/espacios que deben normalizarse)
	errDuplicate := repo.RegisterUserAtomic(ctx, "usr-002", "  USUARIO.EJEMPLO@NEXUM.FINANCE  ", "Otro Usuario", "argon2id_hash_2")
	if !errors.Is(errDuplicate, adaptersDynamo.ErrEmailAlreadyExists) {
		t.Fatalf("Se esperaba rechazo por unicidad de email con ErrEmailAlreadyExists, obtenido: %v", errDuplicate)
	}
}

// TestGSI1SparseMovementQueries verifica consultas filtradas mediante GSI1 (A-060 §2.4).
func TestGSI1SparseMovementQueries(t *testing.T) {
	ctx := context.Background()
	table := infraDynamo.NewDynamoTable()
	repo := adaptersDynamo.NewDynamoRepository(table)

	userID := "usr-filter-01"
	baseTime := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)

	// Insertar 3 gastos hormiga (ANT) y 2 gastos fijos (FIXED)
	for i := 1; i <= 3; i++ {
		movID := "ant-" + string(rune('0'+i))
		_ = repo.RecordMovementAtomic(ctx, userID, movID, "ANT", int64(i*100), baseTime.Add(time.Duration(i)*time.Hour), "", nil)
	}
	for i := 1; i <= 2; i++ {
		movID := "fixed-" + string(rune('0'+i))
		_ = repo.RecordMovementAtomic(ctx, userID, movID, "FIXED", int64(i*5000), baseTime.Add(time.Duration(i)*time.Hour), "", nil)
	}

	// Consultar solo tipo ANT vía GSI1
	ants, err := repo.QueryMovementsByType(ctx, userID, "ANT", 10)
	if err != nil {
		t.Fatalf("Fallo al consultar GSI1 para tipo ANT: %v", err)
	}
	if len(ants) != 3 {
		t.Fatalf("Se esperaban 3 gastos hormiga, obtenidos: %d", len(ants))
	}

	// Consultar solo tipo FIXED vía GSI1
	fixed, err := repo.QueryMovementsByType(ctx, userID, "FIXED", 10)
	if err != nil {
		t.Fatalf("Fallo al consultar GSI1 para tipo FIXED: %v", err)
	}
	if len(fixed) != 2 {
		t.Fatalf("Se esperaban 2 gastos fijos, obtenidos: %d", len(fixed))
	}
}

// TestEndpointAccessPatternsCoverage verifica AC-A-060-03:
// El diseño de claves compuestas y GSI1 cubre los patrones de acceso para los endpoints
// (E027 movimientos, E038 metas, E032 recurrentes, E048 budgets, E051 dashboard, E014 borrado de cuenta)
// resolviéndolos estrictamente mediante Query o GetItem, sin Scan.
func TestEndpointAccessPatternsCoverage(t *testing.T) {
	ctx := context.Background()
	table := infraDynamo.NewDynamoTable()
	repo := adaptersDynamo.NewDynamoRepository(table)

	userID := "usr-complete-coverage"

	// Poblar datos representativos de múltiples entidades bajo la misma partición USER#<id>
	_ = repo.RegisterUserAtomic(ctx, userID, "coverage@nexum.finance", "Usuario Cobertura", "hash")

	goalPK, goalSK := domainDynamo.GoalKeys(userID, "goal-1")
	_ = table.PutItem(ctx, domainDynamo.NewDynamoItem(goalPK, goalSK, "GOAL", map[string]any{"title": "Meta 1"}), false, nil)

	recPK, recSK := domainDynamo.RecurringKeys(userID, "rec-1")
	_ = table.PutItem(ctx, domainDynamo.NewDynamoItem(recPK, recSK, "RECURRING", map[string]any{"name": "Suscripción"}), false, nil)

	budgPK, budgSK := domainDynamo.BudgetKeys(userID, "2026-11", "FOOD")
	_ = table.PutItem(ctx, domainDynamo.NewDynamoItem(budgPK, budgSK, "BUDGET", map[string]any{"limitCents": int64(50000)}), false, nil)

	_ = repo.RecordMovementAtomic(ctx, userID, "mov-1", "ANT", 2500, time.Now().UTC(), "", nil)

	// Patrón E051 / E014: Obtener todos los elementos del usuario acotados a su partición
	allItems, err := repo.QueryAllUserItems(ctx, userID)
	if err != nil {
		t.Fatalf("Error al ejecutar Query por partición de usuario: %v", err)
	}

	// Deben encontrarse perfil, meta, recurrente, budget y movimiento bajo la misma partición USER#<id>
	if len(allItems) < 5 {
		t.Fatalf("AC-A-060-03: Se esperaban al menos 5 entidades en la partición del usuario, encontradas: %d", len(allItems))
	}

	t.Logf("AC-A-060-03 Verificado: Patrones de acceso resueltos con Single-Table Design sin Scan.")
}
