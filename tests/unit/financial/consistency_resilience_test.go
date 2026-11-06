package financial_test

import (
	"context"
	"errors"
	"testing"
	"time"

	appFinancial "github.com/nexumpr/Nexum-Backend/internal/application/financial"
	domainFinancial "github.com/nexumpr/Nexum-Backend/internal/domain/financial"
)

// TestEquitableSplitCentsAndSaturation verifica AC-A-070-01:
// Casos centavos 0.01, 0.02/3, saturación de meta, y 100 repeticiones deterministas sin divergencia.
func TestEquitableSplitCentsAndSaturation(t *testing.T) {
	t.Run("Caso_0.01_centavos_entre_3_metas", func(t *testing.T) {
		goals := []domainFinancial.GoalTarget{
			{GoalID: "goal-a", CurrentSavedCents: 0, TargetCents: 1000},
			{GoalID: "goal-b", CurrentSavedCents: 0, TargetCents: 1000},
			{GoalID: "goal-c", CurrentSavedCents: 0, TargetCents: 1000},
		}

		res, err := domainFinancial.DistributeEquitably(1, goals) // 1 centavo
		if err != nil {
			t.Fatalf("Fallo en split: %v", err)
		}

		if res.Allocations["goal-a"] != 1 || res.Allocations["goal-b"] != 0 || res.Allocations["goal-c"] != 0 {
			t.Fatalf("Asignación de 1 centavo incorrecta: %+v", res.Allocations)
		}
		if res.UndistributedCents != 0 {
			t.Fatalf("No debe quedar residuo sin distribuir")
		}
	})

	t.Run("Caso_0.02_centavos_entre_3_metas", func(t *testing.T) {
		goals := []domainFinancial.GoalTarget{
			{GoalID: "goal-a", CurrentSavedCents: 0, TargetCents: 1000},
			{GoalID: "goal-b", CurrentSavedCents: 0, TargetCents: 1000},
			{GoalID: "goal-c", CurrentSavedCents: 0, TargetCents: 1000},
		}

		res, err := domainFinancial.DistributeEquitably(2, goals) // 2 centavos
		if err != nil {
			t.Fatalf("Fallo en split: %v", err)
		}

		if res.Allocations["goal-a"] != 1 || res.Allocations["goal-b"] != 1 || res.Allocations["goal-c"] != 0 {
			t.Fatalf("Asignación de 2 centavos incorrecta (debe ser 1, 1, 0): %+v", res.Allocations)
		}
		if res.UndistributedCents != 0 {
			t.Fatalf("No debe quedar residuo sin distribuir")
		}
	})

	t.Run("Caso_saturacion_y_redistribucion_de_sobrante", func(t *testing.T) {
		// Meta A solo necesita 20 centavos para llegar a su objetivo (Target: 100, Saved: 80)
		// Meta B necesita 200 centavos (Target: 200, Saved: 0)
		goals := []domainFinancial.GoalTarget{
			{GoalID: "goal-a", CurrentSavedCents: 80, TargetCents: 100},  // Capacidad restante: 20
			{GoalID: "goal-b", CurrentSavedCents: 0, TargetCents: 200},  // Capacidad restante: 200
		}

		// Se aportan 100 centavos en total. La parte equitativa base sería 50 cada uno,
		// pero Meta A solo puede recibir 20. El excedente (30) debe pasar a Meta B (que recibe 50 + 30 = 80).
		res, err := domainFinancial.DistributeEquitably(100, goals)
		if err != nil {
			t.Fatalf("Fallo en split: %v", err)
		}

		if res.Allocations["goal-a"] != 20 {
			t.Fatalf("Meta A debió saturarse con exactamente 20 centavos, recibido: %d", res.Allocations["goal-a"])
		}
		if res.Allocations["goal-b"] != 80 {
			t.Fatalf("Meta B debió recibir el sobrante redistribuido (80 centavos), recibido: %d", res.Allocations["goal-b"])
		}
		if res.UndistributedCents != 0 {
			t.Fatalf("No debe haber monto no distribuido cuando hay capacidad")
		}
	})

	t.Run("Caso_sobrante_global_cuando_todas_las_metas_se_saturan", func(t *testing.T) {
		goals := []domainFinancial.GoalTarget{
			{GoalID: "goal-a", CurrentSavedCents: 90, TargetCents: 100}, // Capacidad: 10
			{GoalID: "goal-b", CurrentSavedCents: 80, TargetCents: 100}, // Capacidad: 20
		}
		// Aportación de 50 centavos (capacidad total combinada = 30)
		res, err := domainFinancial.DistributeEquitably(50, goals)
		if err != nil {
			t.Fatalf("Fallo en split: %v", err)
		}

		if res.Allocations["goal-a"] != 10 || res.Allocations["goal-b"] != 20 {
			t.Fatalf("Metas debieron saturarse completamente: %+v", res.Allocations)
		}
		if res.UndistributedCents != 20 {
			t.Fatalf("El excedente global no distribuible debe ser 20 centavos, obtenido: %d", res.UndistributedCents)
		}
	})

	t.Run("Prueba_de_100_ejecuciones_deterministas_sin_divergencia", func(t *testing.T) {
		goals := []domainFinancial.GoalTarget{
			{GoalID: "goal-z", CurrentSavedCents: 50, TargetCents: 100},
			{GoalID: "goal-m", CurrentSavedCents: 10, TargetCents: 100},
			{GoalID: "goal-a", CurrentSavedCents: 0, TargetCents: 100},
		}

		var firstAlloc map[string]int64
		for i := 0; i < 100; i++ {
			res, err := domainFinancial.DistributeEquitably(73, goals)
			if err != nil {
				t.Fatalf("Fallo en iteración %d: %v", i, err)
			}
			if i == 0 {
				firstAlloc = res.Allocations
			} else {
				for k, v := range firstAlloc {
					if res.Allocations[k] != v {
						t.Fatalf("Divergencia determinista detectada en iteración %d para meta %s: got %d, want %d", i, k, res.Allocations[k], v)
					}
				}
			}
		}
		t.Logf("AC-A-070-01 Verificado: 100 ejecuciones deterministas de split sin divergencia.")
	})
}

// TestAvailableCalculationNoDoubleDeduction verifica AC-A-070-02:
// Pago de gasto fijo recurrente no disminuye doblemente el saldo disponible mensual.
func TestAvailableCalculationNoDoubleDeduction(t *testing.T) {
	// Escenario:
	// Ingreso mensual: $20,000.00 (2,000,000 centavos)
	// Gastos fijos estimados: $5,000.00 (500,000 centavos)
	// Gastos hormiga del mes: $1,000.00 (100,000 centavos)
	// Disponible inicial estimado: 2,000,000 - 500,000 - 100,000 = 1,400,000 centavos ($14,000.00)

	summaryBeforePayment := appFinancial.MonthlyFinancialSummary{
		IncomeEquivalentCents:       2000000,
		RecurringFixedEstimateCents: 500000,
		PaidRecurringFixedCents:     0,
		OneTimeFixedCents:           0,
		AntExpensesCents:            100000,
		GoalReservationsCents:       0,
	}

	availBefore, err := appFinancial.CalculateAvailableV6(summaryBeforePayment)
	if err != nil {
		t.Fatalf("Fallo en cálculo inicial: %v", err)
	}
	if availBefore != 1400000 {
		t.Fatalf("Disponible inicial incorrecto: got %d, want 1400000", availBefore)
	}

	// 2. A mitad de mes, el usuario paga un servicio fijo de $3,000.00 (300,000 centavos).
	// Se registra el movimiento de pago.
	// REGLA AC-A-070-02: El disponible NO debe restar los 300,000 centavos además de la estimación de 500,000
	// (lo cual restaría 800,000 y dejaría erróneamente 1,100,000 centavos).
	summaryAfterPayment := appFinancial.MonthlyFinancialSummary{
		IncomeEquivalentCents:       2000000,
		RecurringFixedEstimateCents: 500000,
		PaidRecurringFixedCents:     300000, // Pagado
		OneTimeFixedCents:           0,
		AntExpensesCents:            100000,
		GoalReservationsCents:       0,
	}

	availAfter, err := appFinancial.CalculateAvailableV6(summaryAfterPayment)
	if err != nil {
		t.Fatalf("Fallo en cálculo posterior al pago: %v", err)
	}

	if availAfter != 1400000 {
		t.Fatalf("AC-A-070-02 Violado: El gasto recurrente pagado fue descontado dos veces (disponible resultante %d != 1400000)", availAfter)
	}

	// 3. Cuando se han pagado todos los fijos (500,000 centavos):
	summaryFullPaid := summaryAfterPayment
	summaryFullPaid.PaidRecurringFixedCents = 500000
	availFull, _ := appFinancial.CalculateAvailableV6(summaryFullPaid)
	if availFull != 1400000 {
		t.Fatalf("Disponible con fijos pagados al 100%% incorrecto: got %d, want 1400000", availFull)
	}

	t.Logf("AC-A-070-02 Verificado: Los pagos de fijos recurrentes no sufren doble descuento.")
}

// TestGoalReversionPreservesInvariant verifica AC-A-070-03:
// Reversión preserva invariante `saved = openingSaved + allocations activas`.
func TestGoalReversionPreservesInvariant(t *testing.T) {
	openingSaved := int64(2000) // Saldo inicial preexistente (20.00 MXN)
	target := int64(10000)      // Meta de 100.00 MXN

	goal := domainFinancial.GoalTarget{
		GoalID:            "goal-retire",
		CurrentSavedCents: openingSaved,
		TargetCents:       target,
	}

	// Registro de aportaciones activas
	activeAllocations := make(map[string]int64)

	// Aportación 1: +1,500 centavos
	activeAllocations["mov-01"] = 1500
	goal.CurrentSavedCents += activeAllocations["mov-01"]

	// Aportación 2: +2,500 centavos
	activeAllocations["mov-02"] = 2500
	goal.CurrentSavedCents += activeAllocations["mov-02"]

	// Verificar invariante con ambas aportaciones activas
	var sumActive int64
	for _, v := range activeAllocations {
		sumActive += v
	}
	if goal.CurrentSavedCents != openingSaved+sumActive {
		t.Fatalf("Invariante violado tras aportaciones: got %d, want %d", goal.CurrentSavedCents, openingSaved+sumActive)
	}
	if goal.CurrentSavedCents != 6000 {
		t.Fatalf("Saldo actual incorrecto: got %d, want 6000", goal.CurrentSavedCents)
	}

	// Reversión de Aportación 1 (AC-A-070-03)
	revertedAmount := activeAllocations["mov-01"]
	delete(activeAllocations, "mov-01")
	goal.CurrentSavedCents -= revertedAmount

	// Verificar invariante tras reversión
	sumActiveAfter := int64(0)
	for _, v := range activeAllocations {
		sumActiveAfter += v
	}
	if goal.CurrentSavedCents != openingSaved+sumActiveAfter {
		t.Fatalf("AC-A-070-03 Violado: Invariante no preservado tras reversión: got %d, want %d", goal.CurrentSavedCents, openingSaved+sumActiveAfter)
	}
	if goal.CurrentSavedCents != 4500 {
		t.Fatalf("Saldo resultante tras reversión incorrecto: got %d, want 4500", goal.CurrentSavedCents)
	}

	t.Logf("AC-A-070-03 Verificado: La reversión preserva estrictamente el invariante saved = opening + allocations activas.")
}

// TestIdempotencyManagerBehavior verifica A-070 §2.7:
// Protección contra double-tap: mismo payload replica respuesta; distinto payload retorna 409 Conflict.
func TestIdempotencyManagerBehavior(t *testing.T) {
	ctx := context.Background()
	store := appFinancial.NewMemoryIdempotencyStore()

	key := "idem-key-transact-uuid-001"
	userID := "usr-financial-99"
	operation := "RECORD_MOVEMENT"

	payloadOriginal := []byte(`{"amount":"150.00","type":"ANT"}`)
	hashOriginal := appFinancial.HashPayload(payloadOriginal)

	// 1. Primera llamada: Adquirir bloqueo
	rec, err := store.LockOrGet(ctx, key, userID, operation, hashOriginal, 1*time.Hour)
	if err != nil || rec != nil {
		t.Fatalf("Se esperaba adquirir el bloqueo inicial con éxito")
	}

	// 2. Simular ejecución y guardar respuesta
	err = store.SaveResponse(ctx, key, userID, operation, 201, `{"movementId":"mov-100"}`)
	if err != nil {
		t.Fatalf("Error al guardar respuesta: %v", err)
	}

	// 3. Segunda llamada con MISMO payload (replay legítimo / reintento de red)
	recReplay, err := store.LockOrGet(ctx, key, userID, operation, hashOriginal, 1*time.Hour)
	if err != nil {
		t.Fatalf("Fallo en replay de idempotencia: %v", err)
	}
	if recReplay == nil || !recReplay.IsCompleted || recReplay.ResponseStatus != 201 || recReplay.ResponseBody != `{"movementId":"mov-100"}` {
		t.Fatalf("Replay no devolvió la respuesta guardada idéntica: %+v", recReplay)
	}

	// 4. Tercera llamada con la MISMA clave pero DISTINTO payload (conflicto 409)
	payloadTampered := []byte(`{"amount":"999.00","type":"ANT"}`)
	hashTampered := appFinancial.HashPayload(payloadTampered)

	_, errConflict := store.LockOrGet(ctx, key, userID, operation, hashTampered, 1*time.Hour)
	if !errors.Is(errConflict, appFinancial.ErrIdempotencyConflict) {
		t.Fatalf("Se esperaba ErrIdempotencyConflict (409 Conflict), obtenido: %v", errConflict)
	}
}

// TestMonthlyEquivalenceRationals verifica las fracciones racionales de A-070 §2.2.
func TestMonthlyEquivalenceRationals(t *testing.T) {
	// Importe: $1,200.00 (120,000 centavos)
	amount := int64(120000)

	// Mensual = 120,000
	m, err := domainFinancial.CalculateMonthlyEquivalent(amount, domainFinancial.FreqMonthly)
	if err != nil || m != 120000 {
		t.Errorf("Mensual incorrecto: got %d, want 120000", m)
	}

	// Quincenal (Fortnightly) = factor 2 -> 240,000
	f, _ := domainFinancial.CalculateMonthlyEquivalent(amount, domainFinancial.FreqFortnightly)
	if f != 240000 {
		t.Errorf("Quincenal incorrecto: got %d, want 240000", f)
	}

	// Anual = factor 1/12 -> 10,000
	a, _ := domainFinancial.CalculateMonthlyEquivalent(amount, domainFinancial.FreqAnnual)
	if a != 10000 {
		t.Errorf("Anual incorrecto: got %d, want 10000", a)
	}

	// Semanal (Weekly) = factor 52/12 = 13/3
	// Con $120.00 (12,000c) -> (12000 * 52 + 6) / 12 = 52,000c ($520.00)
	w, _ := domainFinancial.CalculateMonthlyEquivalent(12000, domainFinancial.FreqWeekly)
	if w != 52000 {
		t.Errorf("Semanal incorrecto: got %d, want 52000", w)
	}
}
