package identity

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/nexumpr/Nexum-Backend/internal/domain/financial"
)

// Reglas del ingreso habitual y de los gastos fijos iniciales de E001 V6.0.2.
//
// El prototipo Android usa siete frecuencias exactas y excluye expresamente
// `daily` (365/12) y `once` en el alta: no son periodicidades de un compromiso
// habitual. El equivalente mensual se calcula con el motor exacto de A-070 y
// nunca con coma flotante (R-030, GOV-08).
var (
	// ErrInvalidAmount indica un importe monetario ausente, mal formado o no positivo.
	ErrInvalidAmount = errors.New("importe monetario invalido")

	// ErrInvalidFrequency indica una frecuencia fuera del enum cerrado de E001.
	ErrInvalidFrequency = errors.New("frecuencia no admitida en el registro inicial")

	// ErrInvalidCategory indica una categoria fuera del catalogo derivado del prototipo.
	ErrInvalidCategory = errors.New("categoria de gasto fijo invalida")

	// ErrInvalidExpenseType indica un tipo distinto de `fixed` en el alta inicial.
	ErrInvalidExpenseType = errors.New("tipo de gasto inicial invalido")

	// ErrInvalidExpenseConcept indica un concepto vacio, no UTF-8 o fuera de rango.
	ErrInvalidExpenseConcept = errors.New("concepto de gasto recurrente invalido")

	// ErrTooManyRecurringExpenses indica una lista que no cabe en una transaccion atomica.
	ErrTooManyRecurringExpenses = errors.New("demasiados gastos recurrentes en el registro inicial")
)

// Frequency es la periodicidad declarada por el cliente, en el valor exacto del
// contrato HTTP (minusculas). Se distingue de financial.Frequency, que es el enum
// interno en mayusculas y admite DAILY/ONCE para otros casos de uso.
type Frequency string

const (
	// FrequencyWeekly equivale a 52/12 mensual.
	FrequencyWeekly Frequency = "weekly"
	// FrequencyFortnightly equivale a 2 mensual.
	FrequencyFortnightly Frequency = "fortnightly"
	// FrequencyMonthly equivale a 1 mensual.
	FrequencyMonthly Frequency = "monthly"
	// FrequencyBimonthly equivale a 1/2 mensual.
	FrequencyBimonthly Frequency = "bimonthly"
	// FrequencyQuarterly equivale a 1/3 mensual.
	FrequencyQuarterly Frequency = "quarterly"
	// FrequencySemiannual equivale a 1/6 mensual.
	FrequencySemiannual Frequency = "semiannual"
	// FrequencyAnnual equivale a 1/12 mensual.
	FrequencyAnnual Frequency = "annual"
)

// allowedFrequencies es el enum cerrado del contrato. `daily` y `once` no figuran
// a proposito: R01/R02 del prototipo V6.0.2 no los ofrecen en el alta.
var allowedFrequencies = [...]Frequency{
	FrequencyWeekly,
	FrequencyFortnightly,
	FrequencyMonthly,
	FrequencyBimonthly,
	FrequencyQuarterly,
	FrequencySemiannual,
	FrequencyAnnual,
}

// AllowedFrequencies devuelve una copia del enum cerrado para validadores y pruebas.
func AllowedFrequencies() []Frequency {
	out := make([]Frequency, len(allowedFrequencies))
	copy(out, allowedFrequencies[:])
	return out
}

// ParseFrequency normaliza y valida la frecuencia declarada por el cliente.
func ParseFrequency(value string) (Frequency, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	for _, candidate := range allowedFrequencies {
		if string(candidate) == normalized {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%w: %q", ErrInvalidFrequency, value)
}

// String devuelve el valor exacto de transporte (minusculas).
func (f Frequency) String() string { return string(f) }

// financialFrequency traduce al enum interno de A-070 sin permitir DAILY ni ONCE.
func (f Frequency) financialFrequency() financial.Frequency {
	switch f {
	case FrequencyWeekly:
		return financial.FreqWeekly
	case FrequencyFortnightly:
		return financial.FreqFortnightly
	case FrequencyMonthly:
		return financial.FreqMonthly
	case FrequencyBimonthly:
		return financial.FreqBimonthly
	case FrequencyQuarterly:
		return financial.FreqQuarterly
	case FrequencySemiannual:
		return financial.FreqSemiannual
	case FrequencyAnnual:
		return financial.FreqAnnual
	default:
		return ""
	}
}

// MonthlyEquivalent calcula el equivalente mensual exacto en centavos con redondeo
// HALF_UP de A-070 §2.2. Es un valor DERIVADO: no se acepta como entrada.
func (f Frequency) MonthlyEquivalent(amountCents int64) (int64, error) {
	internal := f.financialFrequency()
	if internal == "" {
		return 0, fmt.Errorf("%w: %q", ErrInvalidFrequency, f)
	}
	equivalent, err := financial.CalculateMonthlyEquivalent(amountCents, internal)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrInvalidAmount, err)
	}
	return equivalent, nil
}

// Categorias derivadas por el prototipo (guessFixedCategory). El servidor NO las
// calcula por palabras clave: valida que el valor pertenezca al catalogo cerrado y
// persiste exactamente ese valor. No existen identificadores numericos de categoria.
const (
	// CategoryHousing agrupa renta, hipoteca y mantenimiento.
	CategoryHousing = "Vivienda"
	// CategoryServices agrupa agua, luz, gas, internet y telefonia.
	CategoryServices = "Servicios"
	// CategoryTransport agrupa combustible, transporte publico y peajes.
	CategoryTransport = "Transporte"
	// CategoryFood agrupa despensa y alimentacion.
	CategoryFood = "Comida"
	// CategoryOther agrupa lo que no encaja en las anteriores.
	CategoryOther = "Otros"
)

var allowedCategories = [...]string{
	CategoryHousing,
	CategoryServices,
	CategoryTransport,
	CategoryFood,
	CategoryOther,
}

// AllowedCategories devuelve una copia del catalogo cerrado de categorias.
func AllowedCategories() []string {
	out := make([]string, len(allowedCategories))
	copy(out, allowedCategories[:])
	return out
}

// FixedExpenseType es el unico tipo admitido en el alta de compromisos.
const FixedExpenseType = "fixed"

// MaxRecurringExpenses es el limite seguro de filas de la pantalla de registro.
//
// DynamoDB admite como maximo 100 acciones por TransactWriteItems. El alta de E001
// consume seis acciones fijas (reserva de email, perfil, ingreso, consentimiento,
// onboarding e idempotencia): 100 - 6 = 94 filas. El adaptador vuelve a comprobar el
// limite antes de emitir la transaccion y NUNCA trunca la lista para forzar el exito.
const MaxRecurringExpenses = 94

const (
	minConceptRunes = 1
	maxConceptRunes = 100
)

// InitialIncome es el ingreso habitual declarado en el registro (R01 del prototipo).
type InitialIncome struct {
	amountCents            int64
	frequency              Frequency
	monthlyEquivalentCents int64
}

// NewInitialIncome valida el importe exacto en centavos y la frecuencia, y deriva el
// equivalente mensual. El importe debe ser estrictamente mayor que cero.
func NewInitialIncome(amountCents int64, frequency string) (InitialIncome, error) {
	if amountCents <= 0 {
		return InitialIncome{}, fmt.Errorf("%w: el ingreso debe ser mayor que cero", ErrInvalidAmount)
	}
	parsed, err := ParseFrequency(frequency)
	if err != nil {
		return InitialIncome{}, err
	}
	equivalent, err := parsed.MonthlyEquivalent(amountCents)
	if err != nil {
		return InitialIncome{}, err
	}
	return InitialIncome{
		amountCents:            amountCents,
		frequency:              parsed,
		monthlyEquivalentCents: equivalent,
	}, nil
}

// AmountCents retorna el importe declarado en centavos.
func (i InitialIncome) AmountCents() int64 { return i.amountCents }

// Frequency retorna la periodicidad declarada.
func (i InitialIncome) Frequency() Frequency { return i.frequency }

// MonthlyEquivalentCents retorna el equivalente mensual derivado en centavos.
func (i InitialIncome) MonthlyEquivalentCents() int64 { return i.monthlyEquivalentCents }

// IsZero indica que el ingreso no fue inicializado.
func (i InitialIncome) IsZero() bool { return i.amountCents == 0 && i.frequency == "" }

// RecurringExpense es un compromiso programado creado en el registro inicial (R02).
// No es un gasto ejecutado: no genera ningun movimiento financiero.
type RecurringExpense struct {
	id          string
	concept     string
	amountCents int64
	frequency   Frequency
	category    string
	expenseType string
}

// NewRecurringExpense valida una fila del alta. `kind` debe ser exactamente `fixed`.
// El identificador lo asigna el servidor después de validar la fila (ver WithID).
func NewRecurringExpense(concept string, amountCents int64, frequency, category, kind string) (RecurringExpense, error) {
	cleanConcept, err := validateConcept(concept)
	if err != nil {
		return RecurringExpense{}, err
	}
	if amountCents <= 0 {
		return RecurringExpense{}, fmt.Errorf("%w: el importe debe ser mayor que cero", ErrInvalidAmount)
	}
	parsed, err := ParseFrequency(frequency)
	if err != nil {
		return RecurringExpense{}, err
	}
	cleanCategory, err := validateCategory(category)
	if err != nil {
		return RecurringExpense{}, err
	}
	cleanType := strings.ToLower(strings.TrimSpace(kind))
	if cleanType != FixedExpenseType {
		return RecurringExpense{}, fmt.Errorf("%w: %q", ErrInvalidExpenseType, kind)
	}
	return RecurringExpense{
		concept:     cleanConcept,
		amountCents: amountCents,
		frequency:   parsed,
		category:    cleanCategory,
		expenseType: FixedExpenseType,
	}, nil
}

// WithID devuelve una copia con el identificador generado por el servidor. Es el
// único camino para fijar el ID: el cliente nunca lo aporta.
func (e RecurringExpense) WithID(id string) (RecurringExpense, error) {
	clean := strings.TrimSpace(id)
	if clean == "" {
		return RecurringExpense{}, fmt.Errorf("%w: identificador requerido", ErrInvalidExpenseConcept)
	}
	e.id = clean
	return e, nil
}

// ID retorna el identificador generado por el servidor.
func (e RecurringExpense) ID() string { return e.id }

// Concept retorna el concepto normalizado.
func (e RecurringExpense) Concept() string { return e.concept }

// AmountCents retorna el importe en centavos.
func (e RecurringExpense) AmountCents() int64 { return e.amountCents }

// Frequency retorna la periodicidad declarada.
func (e RecurringExpense) Frequency() Frequency { return e.frequency }

// Category retorna la categoria validada.
func (e RecurringExpense) Category() string { return e.category }

// Type retorna el tipo, siempre `fixed`.
func (e RecurringExpense) Type() string { return e.expenseType }

// MonthlyEquivalentCents retorna el equivalente mensual derivado en centavos.
func (e RecurringExpense) MonthlyEquivalentCents() int64 {
	equivalent, err := e.frequency.MonthlyEquivalent(e.amountCents)
	if err != nil {
		return 0
	}
	return equivalent
}

func validateConcept(value string) (string, error) {
	clean := strings.TrimSpace(value)
	if clean == "" {
		return "", fmt.Errorf("%w: no puede estar vacio", ErrInvalidExpenseConcept)
	}
	if !utf8.ValidString(clean) {
		return "", fmt.Errorf("%w: no es UTF-8 valido", ErrInvalidExpenseConcept)
	}
	if runes := utf8.RuneCountInString(clean); runes < minConceptRunes || runes > maxConceptRunes {
		return "", fmt.Errorf("%w: longitud fuera de rango", ErrInvalidExpenseConcept)
	}
	if hasControlChar(clean) {
		return "", fmt.Errorf("%w: contiene caracteres de control", ErrInvalidExpenseConcept)
	}
	return clean, nil
}

func validateCategory(value string) (string, error) {
	clean := strings.TrimSpace(value)
	for _, allowed := range allowedCategories {
		if clean == allowed {
			return clean, nil
		}
	}
	return "", fmt.Errorf("%w: %q", ErrInvalidCategory, value)
}

func hasControlChar(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}
