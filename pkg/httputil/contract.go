package httputil

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ErrorDetail representa un detalle específico dentro del envelope de error (A-040 §2.4).
type ErrorDetail struct {
	Field string `json:"field,omitempty"`
	Issue string `json:"issue"`
}

// ErrorPayload representa la carga útil estructurada de error.
type ErrorPayload struct {
	Code    string        `json:"code"`
	Message string        `json:"message"`
	Details []ErrorDetail `json:"details"`
	TraceID string        `json:"traceId"`
}

// ErrorEnvelope encapsula la estructura estándar OpenAPI 3.1: {"error": {...}} (A-040 §2.4).
type ErrorEnvelope struct {
	Error ErrorPayload `json:"error"`
}

// APIResponse representa el envoltorio estándar de éxito para respuestas con datos (A-040 §2.1).
type APIResponse[T any] struct {
	Data T   `json:"data"`
	Meta any `json:"meta,omitempty"`
}

// WriteErrorEnvelope escribe un error con el envelope canónico de Nexum (A-040 §2.4).
func WriteErrorEnvelope(w http.ResponseWriter, status int, code, message, traceID string, details []ErrorDetail) error {
	if details == nil {
		details = make([]ErrorDetail, 0)
	}
	envelope := ErrorEnvelope{
		Error: ErrorPayload{
			Code:    code,
			Message: message,
			Details: details,
			TraceID: traceID,
		},
	}
	return WriteJSON(w, status, envelope)
}

// WriteAPIResponse escribe una respuesta exitosa con envoltorio estándar (A-040 §2.1).
func WriteAPIResponse[T any](w http.ResponseWriter, status int, data T, meta any) error {
	response := APIResponse[T]{
		Data: data,
		Meta: meta,
	}
	return WriteJSON(w, status, response)
}

// DecimalMoney representa un valor monetario exacto en centavos (int64) serializado estrictamente
// como cadena decimal con dos decimales en JSON (ej: "123.45") para evitar pérdida de precisión (GOV-06, A-040 §2.2).
// Prohíbe terminantemente números flotantes nativos en JSON.
type DecimalMoney struct {
	Cents int64
}

// NewDecimalMoney crea un nuevo DecimalMoney a partir de centavos enteros.
func NewDecimalMoney(cents int64) DecimalMoney {
	return DecimalMoney{Cents: cents}
}

// regexDecimalMoney valida formato estricto: opcional '-', dígitos, '.' y exactamente dos dígitos.
var regexDecimalMoney = regexp.MustCompile(`^-?\d+\.\d{2}$`)

// MarshalJSON serializa DecimalMoney como cadena JSON con exactamente dos posiciones decimales.
func (m DecimalMoney) MarshalJSON() ([]byte, error) {
	formatted := FormatCentsToDecimal(m.Cents)
	return json.Marshal(formatted)
}

// UnmarshalJSON deserializa DecimalMoney verificando que el valor provenga de un string y no un float (A-040 §2.2).
func (m *DecimalMoney) UnmarshalJSON(data []byte) error {
	// Si comienza con comillas, es una cadena
	if len(data) >= 2 && data[0] == '"' && data[len(data)-1] == '"' {
		var str string
		if err := json.Unmarshal(data, &str); err != nil {
			return err
		}
		cents, err := ParseDecimalToCents(str)
		if err != nil {
			return fmt.Errorf("formato monetario invalido '%s': %w", str, err)
		}
		m.Cents = cents
		return nil
	}

	// Si no está entrecomillado, se detecta un intento de enviar número flotante o entero crudo
	return errors.New("floats monetarios prohibidos: el monto debe enviarse como string decimal con exactamente dos decimales (ej: \"123.45\")")
}

// FormatCentsToDecimal formatea un int64 de centavos a una representación string "123.45".
func FormatCentsToDecimal(cents int64) string {
	negative := cents < 0
	absCents := cents
	if negative {
		absCents = -cents
	}
	units := absCents / 100
	remainder := absCents % 100

	sign := ""
	if negative {
		sign = "-"
	}
	return fmt.Sprintf("%s%d.%02d", sign, units, remainder)
}

// ParseDecimalToCents parsea una cadena "123.45" a centavos int64 verificando exactamente dos decimales.
func ParseDecimalToCents(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if !regexDecimalMoney.MatchString(s) {
		return 0, errors.New("formato invalido: debe contener digitos enteros, un punto y exactamente dos decimales")
	}

	parts := strings.Split(s, ".")
	if len(parts) != 2 || len(parts[1]) != 2 {
		return 0, errors.New("debe incluir exactamente dos decimales")
	}

	units, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parte entera invalida: %w", err)
	}

	decimals, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parte decimal invalida: %w", err)
	}

	if units < 0 {
		return units*100 - decimals, nil
	}
	return units*100 + decimals, nil
}

// DateOnly representa una fecha civil sin zona horaria en formato YYYY-MM-DD (A-040 §2.2).
type DateOnly string

// Validate valida que la fecha cumpla con el formato YYYY-MM-DD y sea un calendario válido.
func (d DateOnly) Validate() error {
	_, err := time.Parse("2006-01-02", string(d))
	if err != nil {
		return fmt.Errorf("fecha civil invalida '%s': debe ser YYYY-MM-DD", string(d))
	}
	return nil
}

// YearMonth representa un periodo mensual en formato YYYY-MM (A-040 §2.2).
type YearMonth string

// Validate valida que el periodo cumpla con el formato YYYY-MM.
func (ym YearMonth) Validate() error {
	_, err := time.Parse("2006-01", string(ym))
	if err != nil {
		return fmt.Errorf("periodo mensual invalido '%s': debe ser YYYY-MM", string(ym))
	}
	return nil
}
