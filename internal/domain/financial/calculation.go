package financial

import (
	"errors"
	"math/big"
	"strings"
)

// Frequency define la periodicidad de ingresos o gastos recurrentes (A-070 §2.2).
type Frequency string

const (
	FreqOnce        Frequency = "ONCE"        // factor 0
	FreqDaily       Frequency = "DAILY"       // 365/12
	FreqWeekly      Frequency = "WEEKLY"      // 52/12
	FreqFortnightly Frequency = "FORTNIGHTLY" // 2
	FreqMonthly     Frequency = "MONTHLY"     // 1
	FreqBimonthly   Frequency = "BIMONTHLY"   // 1/2
	FreqQuarterly   Frequency = "QUARTERLY"   // 1/3
	FreqSemiannual  Frequency = "SEMIANNUAL"  // 1/6
	FreqAnnual      Frequency = "ANNUAL"      // 1/12
)

// FrequencyFactor devuelve el numerador y denominador de la fracción racional exacta (A-070 §2.2).
// Evita el uso de coma flotante para prevenir discrepancias acumuladas (GOV-06, R-A-070-01).
func FrequencyFactor(freq Frequency) (num int64, den int64, err error) {
	switch Frequency(strings.ToUpper(string(freq))) {
	case FreqOnce:
		return 0, 1, nil
	case FreqDaily:
		return 365, 12, nil
	case FreqWeekly:
		return 52, 12, nil
	case FreqFortnightly:
		return 2, 1, nil
	case FreqMonthly:
		return 1, 1, nil
	case FreqBimonthly:
		return 1, 2, nil
	case FreqQuarterly:
		return 1, 3, nil
	case FreqSemiannual:
		return 1, 6, nil
	case FreqAnnual:
		return 1, 12, nil
	default:
		return 0, 0, errors.New("frecuencia no soportada")
	}
}

// CalculateMonthlyEquivalent calcula el equivalente mensual en centavos usando aritmética racional
// y redondeo bancario exacto HALF_UP en la frontera de centavos (A-070 §2.2).
func CalculateMonthlyEquivalent(amountCents int64, freq Frequency) (int64, error) {
	if amountCents < 0 {
		return 0, errors.New("el importe no puede ser negativo")
	}
	if amountCents == 0 {
		return 0, nil
	}

	num, den, err := FrequencyFactor(freq)
	if err != nil {
		return 0, err
	}
	if num == 0 {
		return 0, nil
	}

	// Cálculo exacto con enteros grandes: (amountCents * num + (den / 2)) / den (redondeo HALF_UP)
	bigAmount := big.NewInt(amountCents)
	bigNum := big.NewInt(num)
	bigDen := big.NewInt(den)

	// Producto: amount * num
	prod := new(big.Int).Mul(bigAmount, bigNum)

	// Sumar den / 2 para redondeo HALF_UP
	halfDen := new(big.Int).Div(bigDen, big.NewInt(2))
	sum := new(big.Int).Add(prod, halfDen)

	// División entera
	result := new(big.Int).Div(sum, bigDen)

	return result.Int64(), nil
}
