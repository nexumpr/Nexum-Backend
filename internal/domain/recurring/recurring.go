// Package recurring contiene las reglas de dominio de los pagos recurrentes
// (E032–E036): frecuencia canónica, importes exactos en céntimos, fechas civiles y
// coherencia de la programación declarada.
//
// Decisiones que este paquete fija y que no pueden relajarse en capas superiores:
//
//   - La frecuencia se acepta en los NUEVE valores publicados por el contrato
//     (RFC-021) y se normaliza SIEMPRE a uno de los siete canónicos antes de
//     persistir y antes de responder: `biweekly` -> `fortnightly` y
//     `yearly` -> `annual`. La tabla de alias NO se duplica aquí: se delega en
//     `domain/identity`, que es la única fuente (RFC-020 §2, RFC-021 §5).
//   - Los importes son enteros de céntimos. No existe ningún camino con coma
//     flotante (R-030, GOV-08).
//   - `nextDueDate` debe caer sobre la cadencia anclada en `startDate`; no se
//     inventa ni se corrige una fecha declarada por el cliente.
//   - `dueDay` sólo tiene sentido en frecuencias de calendario mensual y debe
//     coincidir con el día de la próxima ocurrencia.
package recurring

import (
	"errors"
	"fmt"
	"strings"
	"time"

	domainIdentity "github.com/nexumpr/Nexum-Backend/internal/domain/identity"
)

const (
	// CurrencyMXN es la única divisa publicada por `RecurringCreate.currency` y
	// `RecurringPayment.currency` (enum de un solo valor).
	CurrencyMXN = "MXN"

	// TypeIncome clasifica la regla como ingreso recurrente.
	TypeIncome = "income"
	// TypeExpense clasifica la regla como gasto recurrente.
	TypeExpense = "expense"
	// TypeSaving clasifica la regla como aportación recurrente al ahorro.
	TypeSaving = "saving"

	// MinAmountCents exige un importe estrictamente positivo (E033 §1: «Monto
	// positivo»). Un céntimo es el mínimo representable.
	MinAmountCents int64 = 1

	// MaxAmountCents deriva del patrón publicado por `RecurringCreate.amount`:
	// `^(0|[1-9][0-9]{0,11})\.[0-9]{2}$`, es decir doce dígitos enteros.
	MaxAmountCents int64 = 99_999_999_999_999

	// MaxNameRunes replica `maxLength: 120` de `name`.
	MaxNameRunes = 120

	// MinDueDay y MaxDueDay replican `minimum: 1` y `maximum: 28` de `dueDay`.
	MinDueDay = 1
	MaxDueDay = 28
)

// Errores de dominio. El transporte los traduce a 422 salvo que el incumplimiento
// sea de esquema, que se rechaza antes (400) con el DTO estricto.
var (
	// ErrInvalidName indica un nombre vacío tras recortar espacios.
	ErrInvalidName = errors.New("nombre de pago recurrente invalido")
	// ErrInvalidAmount indica un importe no positivo o fuera del máximo publicado.
	ErrInvalidAmount = errors.New("importe de pago recurrente invalido")
	// ErrInvalidType indica un `type` fuera del enum income|expense|saving.
	ErrInvalidType = errors.New("tipo de pago recurrente invalido")
	// ErrInvalidFrequency indica una frecuencia fuera de los nueve valores publicados.
	ErrInvalidFrequency = errors.New("frecuencia de pago recurrente invalida")
	// ErrInvalidDate indica una fecha civil que no es un calendario YYYY-MM-DD válido.
	ErrInvalidDate = errors.New("fecha civil invalida")
	// ErrInvalidDueDay indica un `dueDay` fuera de 1..28 o inaplicable a la frecuencia.
	ErrInvalidDueDay = errors.New("dia de vencimiento invalido")
	// ErrIncoherentSchedule indica que nextDueDate no cae sobre la cadencia anclada en
	// startDate, o que dueDay no coincide con la próxima ocurrencia.
	ErrIncoherentSchedule = errors.New("fecha de vencimiento incoherente con la frecuencia")
	// ErrTypeImmutable indica un intento de cambiar `type` en E035. El contrato admite el
	// campo en `RecurringPatch` (por eso el DTO lo acepta) pero el tipo de una regla no es
	// mutable: cambiarlo alteraría el significado de los pagos ya ejecutados (E035 §1).
	ErrTypeImmutable = errors.New("el tipo de pago recurrente no es modificable")
)

// ParseFrequency normaliza y valida la frecuencia declarada en E033.
//
// Acepta los nueve valores publicados y devuelve SIEMPRE el valor canónico. La tabla de
// alias y el enum de siete valores viven en `domain/identity` (RFC-020): este paquete no
// mantiene una segunda copia que pudiera derivar (RFC-021 §6).
func ParseFrequency(value string) (domainIdentity.Frequency, error) {
	canonica, err := domainIdentity.ParseIncomeFrequency(value)
	if err != nil {
		return "", fmt.Errorf("%w: %q", ErrInvalidFrequency, strings.TrimSpace(value))
	}
	return canonica, nil
}

// ParseType valida el enum `income|expense|saving` sin normalizar mayúsculas: el contrato
// publica los valores en minúsculas y un valor distinto no es una variante, es un error.
func ParseType(value string) (string, error) {
	switch strings.TrimSpace(value) {
	case TypeIncome:
		return TypeIncome, nil
	case TypeExpense:
		return TypeExpense, nil
	case TypeSaving:
		return TypeSaving, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidType, strings.TrimSpace(value))
	}
}

// ValidateName recorta el nombre y exige que no quede vacío.
func ValidateName(value string) (string, error) {
	limpio := strings.TrimSpace(value)
	if limpio == "" {
		return "", ErrInvalidName
	}
	if len([]rune(limpio)) > MaxNameRunes {
		return "", fmt.Errorf("%w: supera %d caracteres", ErrInvalidName, MaxNameRunes)
	}
	return limpio, nil
}

// ValidateAmount aplica la regla de negocio del importe ya expresado en céntimos.
func ValidateAmount(amountCents int64) error {
	if amountCents < MinAmountCents {
		return fmt.Errorf("%w: el importe debe ser mayor que cero", ErrInvalidAmount)
	}
	if amountCents > MaxAmountCents {
		return fmt.Errorf("%w: el importe supera el maximo admitido", ErrInvalidAmount)
	}
	return nil
}

// ParseDate interpreta una fecha civil `YYYY-MM-DD` y la devuelve en UTC a medianoche.
// No se aceptan formatos alternativos ni desplazamientos horarios: la fecha del contrato
// es civil, sin zona.
func ParseDate(value string) (time.Time, error) {
	limpio := strings.TrimSpace(value)
	if len(limpio) != 10 {
		return time.Time{}, fmt.Errorf("%w: %q no es YYYY-MM-DD", ErrInvalidDate, value)
	}
	parsed, err := time.Parse("2006-01-02", limpio)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %q", ErrInvalidDate, value)
	}
	return DateOnly(parsed), nil
}

// FormatDate serializa una fecha civil en `YYYY-MM-DD`.
func FormatDate(value time.Time) string {
	return DateOnly(value).Format("2006-01-02")
}

// DateOnly trunca un instante a su fecha civil en UTC.
func DateOnly(value time.Time) time.Time {
	year, month, day := value.UTC().Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// PeriodsMonths devuelve cuántos meses avanza una ocurrencia para las frecuencias de
// calendario. Semanal y quincenal no son frecuencias de calendario mensual y devuelven
// error.
func PeriodsMonths(frequency domainIdentity.Frequency) (int, error) {
	switch frequency {
	case domainIdentity.FrequencyMonthly:
		return 1, nil
	case domainIdentity.FrequencyBimonthly:
		return 2, nil
	case domainIdentity.FrequencyQuarterly:
		return 3, nil
	case domainIdentity.FrequencySemiannual:
		return 6, nil
	case domainIdentity.FrequencyAnnual:
		return 12, nil
	default:
		return 0, fmt.Errorf("%w: %q no es una frecuencia de calendario mensual", ErrInvalidFrequency, frequency)
	}
}

// AddPeriods devuelve la fecha `periods` ocurrencias después de `start` según la
// frecuencia.
//
// Las frecuencias semanal y quincenal avanzan en días exactos. Las de calendario avanzan
// meses y RECORTAN al último día del mes destino: el 31 de enero mensual vence el 28 (o 29)
// de febrero, no el 3 de marzo, que es lo que haría una suma ingenua de meses. El recorte
// se calcula siempre desde `start`, no desde la ocurrencia anterior, para no acumular
// desplazamiento.
func AddPeriods(start time.Time, frequency domainIdentity.Frequency, periods int) (time.Time, error) {
	if periods < 0 {
		return time.Time{}, fmt.Errorf("%w: numero de periodos negativo", ErrInvalidDate)
	}
	base := DateOnly(start)
	switch frequency {
	case domainIdentity.FrequencyWeekly:
		return base.AddDate(0, 0, 7*periods), nil
	case domainIdentity.FrequencyFortnightly:
		return base.AddDate(0, 0, 14*periods), nil
	default:
		months, err := PeriodsMonths(frequency)
		if err != nil {
			return time.Time{}, err
		}
		return addMonthsClamped(base, months*periods), nil
	}
}

// addMonthsClamped suma meses recortando el día al último del mes destino.
func addMonthsClamped(base time.Time, months int) time.Time {
	year, month, day := base.Date()
	primero := time.Date(year, month+time.Month(months), 1, 0, 0, 0, 0, time.UTC)
	ultimo := time.Date(primero.Year(), primero.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
	if day > ultimo {
		day = ultimo
	}
	return time.Date(primero.Year(), primero.Month(), day, 0, 0, 0, 0, time.UTC)
}

// ValidateSchedule exige que `next` caiga sobre la cadencia anclada en `start`: un número
// entero no negativo de ocurrencias después (o en) la fecha de inicio.
//
// No se admite una fecha arbitraria: si el cliente quiere un primer vencimiento lejano,
// puede declarar un múltiplo exacto de la periodicidad (por ejemplo el tercer mes), pero el
// servidor no «corrige» ni redondea la fecha, y tampoco inventa una cuando falta.
func ValidateSchedule(start, next time.Time, frequency domainIdentity.Frequency) error {
	inicio := DateOnly(start)
	proximo := DateOnly(next)
	if proximo.Before(inicio) {
		return fmt.Errorf("%w: nextDueDate no puede ser anterior a startDate", ErrIncoherentSchedule)
	}

	switch frequency {
	case domainIdentity.FrequencyWeekly:
		return validateDayCadence(inicio, proximo, 7)
	case domainIdentity.FrequencyFortnightly:
		return validateDayCadence(inicio, proximo, 14)
	default:
		months, err := PeriodsMonths(frequency)
		if err != nil {
			return err
		}
		// Estimación del número de ocurrencias por diferencia de meses; se comprueban la
		// estimación y sus vecinas para absorber el recorte de fin de mes.
		estimado := (proximo.Year()-inicio.Year())*12 + int(proximo.Month()) - int(inicio.Month())
		candidato := estimado / months
		for _, periodo := range []int{candidato - 1, candidato, candidato + 1} {
			if periodo < 0 {
				continue
			}
			ocurrencia := addMonthsClamped(inicio, months*periodo)
			if ocurrencia.Equal(proximo) {
				return nil
			}
		}
		return fmt.Errorf("%w: nextDueDate no coincide con ninguna ocurrencia de %q", ErrIncoherentSchedule, frequency)
	}
}

// validateDayCadence comprueba que la diferencia sea un múltiplo exacto de `paso` días.
func validateDayCadence(start, next time.Time, paso int) error {
	dias := int(next.Sub(start) / (24 * time.Hour))
	if dias%paso != 0 {
		return fmt.Errorf("%w: nextDueDate no es multiplo de %d dias", ErrIncoherentSchedule, paso)
	}
	return nil
}

// ValidateDueDay valida el día de vencimiento declarado.
//
//   - Ausente (`nil`) es válido en cualquier frecuencia.
//   - Presente fuera de 1..28: ErrInvalidDueDay.
//   - Presente en semanal o quincenal: ErrInvalidDueDay (no aplica; se rechaza en lugar de
//     ignorarlo en silencio).
//   - Presente en frecuencia de calendario: debe coincidir con el día de `next`; de lo
//     contrario la regla se contradice a sí misma.
func ValidateDueDay(dueDay *int, frequency domainIdentity.Frequency, next time.Time) error {
	if dueDay == nil {
		return nil
	}
	valor := *dueDay
	if valor < MinDueDay || valor > MaxDueDay {
		return fmt.Errorf("%w: debe estar entre %d y %d", ErrInvalidDueDay, MinDueDay, MaxDueDay)
	}
	if _, err := PeriodsMonths(frequency); err != nil {
		return fmt.Errorf("%w: no aplica a la frecuencia %q", ErrInvalidDueDay, frequency)
	}
	if DateOnly(next).Day() != valor {
		return fmt.Errorf("%w: no coincide con el dia de nextDueDate", ErrIncoherentSchedule)
	}
	return nil
}

// ValidateTypeChange aplica la inmutabilidad de `type` en E035: sólo se admite repetir el
// valor vigente.
func ValidateTypeChange(current, requested string) error {
	if strings.TrimSpace(requested) == strings.TrimSpace(current) {
		return nil
	}
	return ErrTypeImmutable
}

// NextOccurrenceOnOrAfter devuelve la primera ocurrencia de la cadencia anclada en `start`
// que no sea anterior a `today`.
//
// Se usa para PROYECTAR la fecha de vencimiento de una regla heredada del alta (D-E032-01),
// que no guardó programación: la fecha se calcula con la misma aritmética exacta que valida
// E033, de modo que la proyección siempre cae sobre la cadencia real. No se escribe en el
// ítem: es una lectura determinista.
func NextOccurrenceOnOrAfter(start time.Time, frequency domainIdentity.Frequency, today time.Time) (time.Time, error) {
	inicio := DateOnly(start)
	objetivo := DateOnly(today)
	if !objetivo.After(inicio) {
		return inicio, nil
	}

	estimado := 0
	switch frequency {
	case domainIdentity.FrequencyWeekly:
		estimado = int(objetivo.Sub(inicio)/(24*time.Hour)) / 7
	case domainIdentity.FrequencyFortnightly:
		estimado = int(objetivo.Sub(inicio)/(24*time.Hour)) / 14
	default:
		months, err := PeriodsMonths(frequency)
		if err != nil {
			return time.Time{}, err
		}
		estimado = ((objetivo.Year()-inicio.Year())*12 + int(objetivo.Month()) - int(inicio.Month())) / months
	}
	if estimado < 0 {
		estimado = 0
	}

	// La estimación absorbe el recorte de fin de mes; el margen acota el bucle sin permitir
	// una búsqueda no acotada.
	const margenMaximo = 2400
	for periodo := estimado; periodo <= estimado+margenMaximo; periodo++ {
		ocurrencia, err := AddPeriods(inicio, frequency, periodo)
		if err != nil {
			return time.Time{}, err
		}
		if !ocurrencia.Before(objetivo) {
			return ocurrencia, nil
		}
	}
	return time.Time{}, fmt.Errorf("%w: no se encontro ocurrencia desde %s", ErrInvalidDate, FormatDate(inicio))
}
