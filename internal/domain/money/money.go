package money

import (
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrCurrencyMismatch se produce al intentar operar importes de diferentes monedas.
	ErrCurrencyMismatch = errors.New("conflicto de divisas: no se pueden sumar o restar importes con diferente moneda")

	// ErrInvalidCurrency indica que el código de divisa ISO no es válido o está vacío.
	ErrInvalidCurrency = errors.New("codigo de divisa invalido: se requiere codigo ISO 4217 de 3 letras mayusculas")

	// ErrNegativeAmount se produce cuando se requiere una cantidad estrictamente positiva.
	ErrNegativeAmount = errors.New("el importe no puede ser negativo")
)

// Money es un Value Object inmutable que representa cantidades monetarias exactas (R-030, GOV-08, CTX-005).
// Almacena el valor en la unidad mínima indivisible de la divisa (centavos para MXN/USD) como entero int64,
// evitando de forma absoluta los errores de precisión de punto flotante binario (float32/float64).
type Money struct {
	amount   int64  // Importe en centavos
	currency string // Código ISO 4217 (ej. "MXN", "USD")
}

// New crea una nueva instancia inmutable de Money validando la divisa.
func New(amount int64, currency string) (Money, error) {
	curr := strings.TrimSpace(strings.ToUpper(currency))
	if len(curr) != 3 {
		return Money{}, ErrInvalidCurrency
	}
	for _, ch := range curr {
		if ch < 'A' || ch > 'Z' {
			return Money{}, ErrInvalidCurrency
		}
	}
	return Money{
		amount:   amount,
		currency: curr,
	}, nil
}

// MustNew crea una instancia de Money o entra en pánico si la divisa es inválida (útil para constantes de dominio).
func MustNew(amount int64, currency string) Money {
	m, err := New(amount, currency)
	if err != nil {
		panic(err)
	}
	return m
}

// Amount retorna el valor en centavos.
func (m Money) Amount() int64 {
	return m.amount
}

// Currency retorna el código ISO de la moneda.
func (m Money) Currency() string {
	return m.currency
}

// Add suma dos importes monetarios asegurando que compartan la misma divisa.
func (m Money) Add(other Money) (Money, error) {
	if m.currency != other.currency {
		return Money{}, fmt.Errorf("%w: '%s' vs '%s'", ErrCurrencyMismatch, m.currency, other.currency)
	}
	return Money{
		amount:   m.amount + other.amount,
		currency: m.currency,
	}, nil
}

// Subtract resta dos importes monetarios asegurando la misma divisa.
func (m Money) Subtract(other Money) (Money, error) {
	if m.currency != other.currency {
		return Money{}, fmt.Errorf("%w: '%s' vs '%s'", ErrCurrencyMismatch, m.currency, other.currency)
	}
	return Money{
		amount:   m.amount - other.amount,
		currency: m.currency,
	}, nil
}

// Multiply multiplica el importe por un factor entero escalar.
func (m Money) Multiply(factor int64) Money {
	return Money{
		amount:   m.amount * factor,
		currency: m.currency,
	}
}

// IsZero indica si el importe monetario es exactamente cero.
func (m Money) IsZero() bool {
	return m.amount == 0
}

// IsPositive indica si el importe es mayor a cero.
func (m Money) IsPositive() bool {
	return m.amount > 0
}

// IsNegative indica si el importe es menor a cero.
func (m Money) IsNegative() bool {
	return m.amount < 0
}

// Equals compara igualdad de valor y moneda entre dos objetos Money.
func (m Money) Equals(other Money) bool {
	return m.amount == other.amount && m.currency == other.currency
}

// Formatted devuelve la representación decimal humana estándar de forma determinista (ej. "100.50 MXN").
func (m Money) Formatted() string {
	sign := ""
	cents := m.amount
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	units := cents / 100
	remainder := cents % 100
	return fmt.Sprintf("%s%d.%02d %s", sign, units, remainder, m.currency)
}

