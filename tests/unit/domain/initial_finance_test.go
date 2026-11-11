package domain_test

import (
	"errors"
	"testing"

	"github.com/nexumpr/Nexum-Backend/internal/domain/identity"
)

// Los siete valores del contrato V6.0.2 y su equivalente mensual exacto en centavos.
// El prototipo declara los factores 52/12, 2, 1, 1/2, 1/3, 1/6 y 1/12.
func TestFrequenciesDelContrato(t *testing.T) {
	casos := []struct {
		frequency string
		amount    int64
		expected  int64
	}{
		{"weekly", 10000, 43333},      // 100.00 * 52/12 = 433.33
		{"fortnightly", 10000, 20000}, // 100.00 * 2
		{"monthly", 10000, 10000},     // 100.00 * 1
		{"bimonthly", 10000, 5000},    // 100.00 * 1/2
		{"quarterly", 10000, 3333},    // 100.00 * 1/3
		{"semiannual", 10000, 1667},   // 100.00 * 1/6
		{"annual", 10000, 833},        // 100.00 * 1/12
	}

	allowed := identity.AllowedFrequencies()
	if len(allowed) != len(casos) {
		t.Fatalf("el enum debe tener exactamente %d frecuencias, tiene %d", len(casos), len(allowed))
	}

	for _, caso := range casos {
		t.Run(caso.frequency, func(t *testing.T) {
			income, err := identity.NewInitialIncome(caso.amount, caso.frequency)
			if err != nil {
				t.Fatalf("frecuencia %q rechazada: %v", caso.frequency, err)
			}
			if income.MonthlyEquivalentCents() != caso.expected {
				t.Fatalf("equivalente mensual de %q: esperado %d, obtenido %d",
					caso.frequency, caso.expected, income.MonthlyEquivalentCents())
			}
		})
	}
}

func TestFrequenciesRechazanDailyYOnce(t *testing.T) {
	for _, value := range []string{"daily", "once", "DAILY", "ONCE", "", "semanal", "mensual"} {
		if _, err := identity.ParseFrequency(value); !errors.Is(err, identity.ErrInvalidFrequency) {
			t.Fatalf("frecuencia %q debía rechazarse, se obtuvo %v", value, err)
		}
	}
}

func TestFrequencyNormalizaMayusculasYEspacios(t *testing.T) {
	parsed, err := identity.ParseFrequency("  Monthly ")
	if err != nil {
		t.Fatalf("frecuencia valida rechazada: %v", err)
	}
	if parsed != identity.FrequencyMonthly || parsed.String() != "monthly" {
		t.Fatalf("normalizacion inesperada: %q", parsed.String())
	}
}

func TestRedondeoHalfUpMensual(t *testing.T) {
	// 0.06 MXN anuales = 0.005 mensuales -> HALF_UP a 0.01 (1 centavo).
	income, err := identity.NewInitialIncome(6, "annual")
	if err != nil {
		t.Fatalf("ingreso rechazado: %v", err)
	}
	if income.MonthlyEquivalentCents() != 1 {
		t.Fatalf("se esperaba redondeo HALF_UP a 1 centavo, obtenido %d", income.MonthlyEquivalentCents())
	}
}

func TestInitialIncomeRechazaImportesInvalidos(t *testing.T) {
	for _, amount := range []int64{0, -1, -100000} {
		if _, err := identity.NewInitialIncome(amount, "monthly"); !errors.Is(err, identity.ErrInvalidAmount) {
			t.Fatalf("importe %d debía rechazarse, se obtuvo %v", amount, err)
		}
	}
	if _, err := identity.NewInitialIncome(10000, "daily"); !errors.Is(err, identity.ErrInvalidFrequency) {
		t.Fatalf("se esperaba ErrInvalidFrequency, se obtuvo %v", err)
	}
}

func TestCategoriasDelPrototipo(t *testing.T) {
	expected := []string{"Vivienda", "Servicios", "Transporte", "Comida", "Otros"}
	allowed := identity.AllowedCategories()
	if len(allowed) != len(expected) {
		t.Fatalf("se esperaban %d categorias, hay %d", len(expected), len(allowed))
	}
	for i, categoria := range expected {
		if allowed[i] != categoria {
			t.Fatalf("categoria %d: esperada %q, obtenida %q", i, categoria, allowed[i])
		}
	}
}

func TestRecurringExpenseValidaCategoriaYTipo(t *testing.T) {
	casos := []struct {
		name     string
		category string
		kind     string
		want     error
	}{
		{"categoria desconocida", "Mascotas", identity.FixedExpenseType, identity.ErrInvalidCategory},
		{"categoria numerica", "3", identity.FixedExpenseType, identity.ErrInvalidCategory},
		{"categoria vacia", "", identity.FixedExpenseType, identity.ErrInvalidCategory},
		{"tipo variable", identity.CategoryFood, "variable", identity.ErrInvalidExpenseType},
		{"tipo vacio", identity.CategoryFood, "", identity.ErrInvalidExpenseType},
	}
	for _, caso := range casos {
		t.Run(caso.name, func(t *testing.T) {
			_, err := identity.NewRecurringExpense("Super", 50000, "monthly", caso.category, caso.kind)
			if !errors.Is(err, caso.want) {
				t.Fatalf("se esperaba %v, se obtuvo %v", caso.want, err)
			}
		})
	}
}

func TestRecurringExpenseValidaConceptoEImporte(t *testing.T) {
	if _, err := identity.NewRecurringExpense("   ", 50000, "monthly", identity.CategoryFood, identity.FixedExpenseType); !errors.Is(err, identity.ErrInvalidExpenseConcept) {
		t.Fatalf("concepto vacio debía rechazarse, se obtuvo %v", err)
	}
	if _, err := identity.NewRecurringExpense("Super", 0, "monthly", identity.CategoryFood, identity.FixedExpenseType); !errors.Is(err, identity.ErrInvalidAmount) {
		t.Fatalf("importe cero debía rechazarse, se obtuvo %v", err)
	}
	if _, err := identity.NewRecurringExpense("Super", 100, "once", identity.CategoryFood, identity.FixedExpenseType); !errors.Is(err, identity.ErrInvalidFrequency) {
		t.Fatalf("frecuencia once debía rechazarse, se obtuvo %v", err)
	}
}

func TestRecurringExpenseWithID(t *testing.T) {
	expense, err := identity.NewRecurringExpense("Internet", 69900, "monthly", identity.CategoryServices, identity.FixedExpenseType)
	if err != nil {
		t.Fatalf("gasto valido rechazado: %v", err)
	}
	if expense.ID() != "" {
		t.Fatal("el identificador solo lo asigna el servidor")
	}
	withID, err := expense.WithID("018f90ef-08e5-4a68-b09f-000000000001")
	if err != nil {
		t.Fatalf("identificador rechazado: %v", err)
	}
	if withID.ID() != "018f90ef-08e5-4a68-b09f-000000000001" {
		t.Fatalf("identificador inesperado: %q", withID.ID())
	}
	if withID.Type() != identity.FixedExpenseType {
		t.Fatalf("tipo inesperado: %q", withID.Type())
	}
	if _, err := expense.WithID("  "); err == nil {
		t.Fatal("un identificador vacio debe rechazarse")
	}
}

func TestLimiteDeCompromisos(t *testing.T) {
	if identity.MaxRecurringExpenses != 94 {
		t.Fatalf("el limite debe ser 94 (100 acciones - 6 fijas), es %d", identity.MaxRecurringExpenses)
	}
}

func TestClientContextValidaZonaIANA(t *testing.T) {
	ok, err := identity.NewClientContext("es-MX", "America/Mexico_City")
	if err != nil {
		t.Fatalf("contexto valido rechazado: %v", err)
	}
	if ok.Locale() != "es-MX" || ok.TimeZone() != "America/Mexico_City" {
		t.Fatalf("contexto inesperado: %+v", ok)
	}

	vacio, err := identity.NewClientContext("", "")
	if err != nil {
		t.Fatalf("contexto vacio debe ser valido: %v", err)
	}
	if vacio.Locale() != "" || vacio.TimeZone() != "" {
		t.Fatalf("contexto vacio inesperado: %+v", vacio)
	}

	if _, err := identity.NewClientContext("es-MX", "Marte/Olympus"); !errors.Is(err, identity.ErrInvalidClientContext) {
		t.Fatalf("zona IANA invalida debía rechazarse, se obtuvo %v", err)
	}
	if _, err := identity.NewClientContext("españa!", "America/Mexico_City"); !errors.Is(err, identity.ErrInvalidClientContext) {
		t.Fatalf("locale invalido debía rechazarse, se obtuvo %v", err)
	}
}
