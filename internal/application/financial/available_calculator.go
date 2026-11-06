package financial

import (
	"errors"
)

// MonthlyFinancialSummary contiene los componentes del cálculo de disponible del mes (A-070 §2.3).
type MonthlyFinancialSummary struct {
	IncomeEquivalentCents      int64 // Total de ingresos mensuales equivalentes
	RecurringFixedEstimateCents int64 // Estimación mensual de gastos fijos recurrentes programados
	PaidRecurringFixedCents    int64 // Gastos fijos recurrentes que YA fueron pagados y registrados este mes
	OneTimeFixedCents          int64 // Gastos fijos no recurrentes (puntuales del mes)
	AntExpensesCents           int64 // Gastos variables u hormiga efectuados en el mes
	GoalReservationsCents      int64 // Total aportado o reservado a metas en el mes
}

// CalculateAvailableV6 calcula el disponible financiero mensual según la especificación V6 (A-070 §2.3, AC-A-070-02).
// Regla AC-A-070-02 (Anti-doble descuento):
// Los pagos de gastos fijos recurrentes ya abonados durante el mes registran un movimiento, pero NO se restan dos veces.
// La obligación fija restante es max(0, RecurringFixedEstimate - PaidRecurringFixed).
func CalculateAvailableV6(summary MonthlyFinancialSummary) (int64, error) {
	if summary.IncomeEquivalentCents < 0 || summary.RecurringFixedEstimateCents < 0 ||
		summary.PaidRecurringFixedCents < 0 || summary.OneTimeFixedCents < 0 ||
		summary.AntExpensesCents < 0 || summary.GoalReservationsCents < 0 {
		return 0, errors.New("los valores monetarios del sumario no pueden ser negativos")
	}

	// Obligación fija pendiente de pago en lo que resta del mes
	pendingFixedEstimate := summary.RecurringFixedEstimateCents - summary.PaidRecurringFixedCents
	if pendingFixedEstimate < 0 {
		pendingFixedEstimate = 0
	}

	// Total de deducciones fijas (pagadas + pendientes) = exactamente el total de fijos sin duplicar (AC-A-070-02)
	totalFixedDeduction := summary.PaidRecurringFixedCents + pendingFixedEstimate

	// Total de compromisos y gastos del mes
	totalOutflows := totalFixedDeduction + summary.OneTimeFixedCents + summary.AntExpensesCents + summary.GoalReservationsCents

	// Disponible neto
	available := summary.IncomeEquivalentCents - totalOutflows
	return available, nil
}
