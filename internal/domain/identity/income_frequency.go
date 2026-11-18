package identity

import (
	"errors"
	"fmt"
	"strings"
)

// Reglas de frecuencia e importe del ingreso base (E024/E025), conforme a RFC-020 v1.1.0.
//
// El contrato publicado de `Income.frequency` e `IncomeUpdate.frequency` enumera NUEVE
// valores: los siete canónicos que ya acepta el registro inicial (E001) más los dos alias
// históricos `biweekly` y `yearly`. Los alias se conservan para no reducir un enum ya
// publicado, pero NUNCA se emiten ni se persisten: se normalizan a `fortnightly` y
// `annual` antes de escribir y antes de responder (RFC-020 §2).
//
// Este archivo NO modifica el enum de E001 (`ParseFrequency` sigue aceptando exactamente
// siete valores en el alta): el contrato de E024/E025 es más amplio porque debe poder
// representar cualquier frecuencia que un usuario ya tenga persistida.
var (
	// ErrInvalidIncomeFrequency indica una frecuencia fuera de los nueve valores publicados
	// por `Income.frequency` e `IncomeUpdate.frequency`.
	ErrInvalidIncomeFrequency = errors.New("frecuencia de ingreso no admitida")
)

// MaxIncomeAmountCents es el importe máximo admitido por el patrón publicado
// `^(0|[1-9][0-9]{0,11})\.[0-9]{2}$`: doce dígitos enteros y dos decimales.
const MaxIncomeAmountCents int64 = 99_999_999_999_999

// MinIncomeAmountCents exige un importe estrictamente positivo: el ingreso base no puede
// ser cero (E025 §2). Un céntimo es el mínimo representable.
const MinIncomeAmountCents int64 = 1

// incomeFrequencyAliases son los alias históricos publicados. Un alias es sólo una forma
// de ENTRADA: la respuesta y la persistencia usan siempre el valor canónico.
var incomeFrequencyAliases = map[Frequency]Frequency{
	"biweekly": FrequencyFortnightly,
	"yearly":   FrequencyAnnual,
}

// ParseIncomeFrequency normaliza y valida una frecuencia del ingreso base.
//
// Acepta los nueve valores publicados y devuelve SIEMPRE el valor canónico: `biweekly`
// produce `fortnightly` y `yearly` produce `annual`. No admite `daily` ni `once`: no son
// periodicidades de un ingreso base (E001 §2.3, RFC-020 §2).
func ParseIncomeFrequency(value string) (Frequency, error) {
	normalized := Frequency(strings.ToLower(strings.TrimSpace(value)))
	if canonical, ok := incomeFrequencyAliases[normalized]; ok {
		return canonical, nil
	}
	for _, candidate := range allowedFrequencies {
		if candidate == normalized {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%w: %q", ErrInvalidIncomeFrequency, value)
}

// ValidateIncomeAmount valida el importe del ingreso base ya expresado en céntimos.
//
// El transporte comprueba el formato decimal publicado; aquí se aplica la regla de
// negocio: estrictamente positivo y como máximo doce dígitos enteros. Los importes son
// enteros de céntimos en todo el camino: no hay coma flotante en ningún punto (R-030,
// GOV-08).
func ValidateIncomeAmount(amountCents int64) error {
	if amountCents < MinIncomeAmountCents {
		return fmt.Errorf("%w: el ingreso debe ser mayor que cero", ErrInvalidAmount)
	}
	if amountCents > MaxIncomeAmountCents {
		return fmt.Errorf("%w: el ingreso supera el maximo admitido", ErrInvalidAmount)
	}
	return nil
}
