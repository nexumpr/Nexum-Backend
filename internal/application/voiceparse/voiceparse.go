// Package voiceparse implementa E075: interpretación determinista de texto ya
// transcrito en español, sin audio, sin persistencia y sin invocar movimientos.
package voiceparse

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	// LocaleESMX es el único locale aceptado por el contrato.
	LocaleESMX = "es-MX"
	// MaxTextRunes es el máximo de caracteres Unicode del campo text.
	MaxTextRunes = 500

	// IntentCreateMovement identifica un texto con un importe positivo reconocible.
	IntentCreateMovement = "createMovement"
	// IntentUnknown identifica texto ambiguo o desconocido.
	IntentUnknown = "unknown"
	// CurrencyMXN es la única moneda del contrato.
	CurrencyMXN = "MXN"

	maxUnitsDigits       = 12
	maxCents       int64 = 99999999999999
)

// VoiceParseRequestWire es el cuerpo JSON estricto de E075.
type VoiceParseRequestWire struct {
	Text   string `json:"text"`
	Locale string `json:"locale"`
}

// VoiceParseWire es la respuesta JSON estricta de E075.
type VoiceParseWire struct {
	Intent               string `json:"intent"`
	RequiresConfirmation bool   `json:"requiresConfirmation"`
	ParsedAmount         string `json:"parsedAmount"`
	Currency             string `json:"currency"`
}

var (
	// ErrInvalidRequest agrupa las violaciones del cuerpo (texto o locale).
	ErrInvalidRequest = errors.New("solicitud invalida")
	// ErrNegativeAmount señala un importe negativo: regla de negocio, no error de sintaxis.
	ErrNegativeAmount = errors.New("monto negativo no permitido")
)

// RequestError describe la violación de un campo sin incluir nunca el texto del usuario.
type RequestError struct {
	Field  string
	Reason string
}

func (e *RequestError) Error() string {
	return fmt.Sprintf("campo %s: %s", e.Field, e.Reason)
}

// Is permite errors.Is(err, ErrInvalidRequest).
func (e *RequestError) Is(target error) bool {
	return target == ErrInvalidRequest
}

// Service es un servicio puro: no tiene almacenes, clientes HTTP ni reloj.
type Service struct{}

// NewService crea el servicio de parsing de texto.
func NewService() *Service { return &Service{} }

// Parse aplica la validación estricta y el parser determinista.
func (s *Service) Parse(req VoiceParseRequestWire) (VoiceParseWire, error) {
	return Parse(req)
}

// Parse es el punto de entrada puro, seguro para uso concurrente.
func Parse(req VoiceParseRequestWire) (VoiceParseWire, error) {
	if err := validateRequest(req); err != nil {
		return VoiceParseWire{}, err
	}
	if contieneMontoNegativo(req.Text) {
		return VoiceParseWire{}, ErrNegativeAmount
	}

	cents, ok := extraerCents(req.Text)
	if !ok || cents <= 0 {
		return unknownResult(), nil
	}
	return VoiceParseWire{
		Intent:               IntentCreateMovement,
		RequiresConfirmation: true,
		ParsedAmount:         formatearCents(cents),
		Currency:             CurrencyMXN,
	}, nil
}

func validateRequest(req VoiceParseRequestWire) error {
	if req.Locale != LocaleESMX {
		return &RequestError{Field: "locale", Reason: "invalid_enum"}
	}
	runas := utf8.RuneCountInString(req.Text)
	if runas < 1 || strings.TrimSpace(req.Text) == "" {
		return &RequestError{Field: "text", Reason: "required"}
	}
	if runas > MaxTextRunes {
		return &RequestError{Field: "text", Reason: "too_long"}
	}
	return nil
}

func unknownResult() VoiceParseWire {
	return VoiceParseWire{
		Intent:               IntentUnknown,
		RequiresConfirmation: true,
		ParsedAmount:         "0.00",
		Currency:             CurrencyMXN,
	}
}

func formatearCents(cents int64) string {
	return fmt.Sprintf("%d.%02d", cents/100, cents%100)
}

var (
	// Se prueba primero el signo menos para no interpretar "-100" como importe positivo.
	// El símbolo de peso es opcional entre el signo y la cifra ("- $50").
	negativeNumericPattern = regexp.MustCompile(`(^|[^0-9A-Za-z])-[ \t]*\$?[ \t]*[0-9]`)
	negativeWordPattern    = regexp.MustCompile(`(?i)(?:^|[^a-záéíóúñ])menos[ \t]+(?:[0-9]|cien|ciento|doscient|trescient|cuatrocient|quinient|seiscient|setecient|ochocient|novecient|mil|un|una|dos|tres|cuatro|cinco|seis|siete|ocho|nueve|diez|veinte|treinta|cuarenta|cincuenta|sesenta|setenta|ochenta|noventa)`)

	// Alternativas ordenadas de mayor a menor especificidad: "1,234.50",
	// "1.234,50", "100.50", "100,50" y "100".
	amountPattern = regexp.MustCompile(`(^|[^0-9.,-])([0-9]{1,3}(?:,[0-9]{3})+(?:\.[0-9]{2})?|[0-9]{1,3}(?:\.[0-9]{3})+(?:,[0-9]{2})?|[0-9]+(?:[.,][0-9]{1,2})?)([^0-9.,]|$)`)
	centsPattern  = regexp.MustCompile(`(?i)\bcon\b[ \t]+([0-9]{1,2}|[a-záéíóúñ]+)[ \t]*(?:centavos?)?\b`)
	wordPattern   = regexp.MustCompile(`[a-záéíóúñ]+`)

	// Fechas habladas o escritas. No describen un monto, así que se retiran antes de
	// buscar cifras: "el 5 de noviembre" no es un gasto de 5.00 y "2026-11-05" no es
	// un gasto de 2026.00.
	datePattern        = regexp.MustCompile(`(?i)\b(?:el[ \t]+)?[0-9]{1,2}[ \t]+de[ \t]+(?:enero|febrero|marzo|abril|mayo|junio|julio|agosto|septiembre|setiembre|octubre|noviembre|diciembre)(?:[ \t]+(?:de|del)[ \t]+[0-9]{4}|[ \t]*,[ \t]*[0-9]{4})?`)
	numericDatePattern = regexp.MustCompile(`\b[0-9]{1,2}/[0-9]{1,2}(?:/[0-9]{2,4})?\b`)
	isoDatePattern     = regexp.MustCompile(`\b[0-9]{4}-[0-9]{2}-[0-9]{2}\b`)
)

func contieneMontoNegativo(texto string) bool {
	return negativeNumericPattern.MatchString(texto) || negativeWordPattern.MatchString(texto)
}

func extraerCents(texto string) (int64, bool) {
	texto = sinFechas(texto)
	if match := amountPattern.FindStringSubmatch(texto); len(match) == 4 {
		cents, ok := parseNumericAmount(match[2])
		if ok {
			if !numericHasDecimal(match[2]) {
				if extra, ok := centavosSuffix(texto); ok {
					cents += extra
				}
			}
			return cents, true
		}
	}
	return parseSpanishWords(texto)
}

// sinFechas retira las expresiones de fecha (habladas, d/m o ISO) para que ninguna
// cifra de calendario se interprete como importe.
func sinFechas(texto string) string {
	for _, patron := range []*regexp.Regexp{datePattern, isoDatePattern, numericDatePattern} {
		texto = patron.ReplaceAllString(texto, " ")
	}
	return texto
}

func parseNumericAmount(raw string) (int64, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, false
	}

	var unitsPart, fracPart string
	dot := strings.IndexByte(s, '.')
	comma := strings.IndexByte(s, ',')
	switch {
	case dot >= 0 && comma >= 0:
		if comma < dot { // 1,234.50
			unitsPart = strings.ReplaceAll(s[:dot], ",", "")
			fracPart = s[dot+1:]
		} else { // 1.234,50
			unitsPart = strings.ReplaceAll(s[:comma], ".", "")
			fracPart = s[comma+1:]
		}
	case comma >= 0:
		if esAgrupamientoMiles(s, ',') {
			unitsPart = strings.ReplaceAll(s, ",", "")
		} else {
			partes := strings.SplitN(s, ",", 2)
			unitsPart, fracPart = partes[0], partes[1]
		}
	case dot >= 0:
		if esAgrupamientoMiles(s, '.') {
			unitsPart = strings.ReplaceAll(s, ".", "")
		} else {
			partes := strings.SplitN(s, ".", 2)
			unitsPart, fracPart = partes[0], partes[1]
		}
	default:
		unitsPart = s
	}

	if !soloDigitos(unitsPart) || (fracPart != "" && !soloDigitos(fracPart)) {
		return 0, false
	}
	if len(fracPart) > 2 {
		return 0, false
	}
	for len(fracPart) < 2 {
		fracPart += "0"
	}

	unitsPart = strings.TrimLeft(unitsPart, "0")
	if unitsPart == "" {
		unitsPart = "0"
	}
	if len(unitsPart) > maxUnitsDigits {
		return 0, false
	}
	units, err := strconv.ParseInt(unitsPart, 10, 64)
	if err != nil {
		return 0, false
	}
	frac, err := strconv.ParseInt(fracPart, 10, 64)
	if err != nil {
		return 0, false
	}
	cents := units*100 + frac
	if cents > maxCents {
		return 0, false
	}
	return cents, true
}

func esAgrupamientoMiles(s string, sep byte) bool {
	partes := strings.Split(s, string(sep))
	if len(partes) < 2 || len(partes[0]) == 0 || len(partes[0]) > 3 || !soloDigitos(partes[0]) {
		return false
	}
	for _, parte := range partes[1:] {
		if len(parte) != 3 || !soloDigitos(parte) {
			return false
		}
	}
	return true
}

func numericHasDecimal(raw string) bool {
	dot, comma := strings.IndexByte(raw, '.'), strings.IndexByte(raw, ',')
	switch {
	case dot >= 0 && comma >= 0:
		return true
	case comma >= 0:
		return !esAgrupamientoMiles(raw, ',')
	case dot >= 0:
		return !esAgrupamientoMiles(raw, '.')
	default:
		return false
	}
}

func soloDigitos(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

var spanishWordValues = map[string]int64{
	"un": 1, "uno": 1, "una": 1,
	"dos": 2, "tres": 3, "cuatro": 4, "cinco": 5,
	"seis": 6, "siete": 7, "ocho": 8, "nueve": 9,
	"diez": 10, "once": 11, "doce": 12, "trece": 13, "catorce": 14, "quince": 15,
	"dieciseis": 16, "diecisiete": 17, "dieciocho": 18, "diecinueve": 19,
	"veinte": 20, "veintiun": 21, "veintiuno": 21, "veintidos": 22, "veintitres": 23,
	"veinticuatro": 24, "veinticinco": 25, "veintiseis": 26, "veintisiete": 27,
	"veintiocho": 28, "veintinueve": 29,
	"treinta": 30, "cuarenta": 40, "cincuenta": 50, "sesenta": 60,
	"setenta": 70, "ochenta": 80, "noventa": 90,
	"cien": 100, "ciento": 100,
	"doscientos": 200, "doscientas": 200,
	"trescientos": 300, "trescientas": 300,
	"cuatrocientos": 400, "cuatrocientas": 400,
	"quinientos": 500, "quinientas": 500,
	"seiscientos": 600, "seiscientas": 600,
	"setecientos": 700, "setecientas": 700,
	"ochocientos": 800, "ochocientas": 800,
	"novecientos": 900, "novecientas": 900,
	"mil": 1000,
}

func parseSpanishWords(texto string) (int64, bool) {
	tokens := wordPattern.FindAllString(strings.ToLower(texto), -1)
	currencyIndex := -1
	for i, token := range tokens {
		if token == "peso" || token == "pesos" || token == "mxn" {
			currencyIndex = i
		}
	}
	if currencyIndex < 0 {
		return 0, false
	}

	// Sólo se considera la corrida contigua de palabras-numero inmediatamente
	// anterior a la divisa; así "un gasto de cien pesos" vale 100 y no 101.
	// «y» es un conector numérico ("treinta y cuatro"): se omite únicamente cuando
	// está entre dos palabras-número, nunca al principio ni tras una palabra ajena.
	var words []string
	for i := currencyIndex - 1; i >= 0; i-- {
		token := tokens[i]
		if token == "y" {
			if i == 0 || len(words) == 0 {
				break
			}
			if _, esNumero := spanishWordValues[tokens[i-1]]; !esNumero {
				break
			}
			continue
		}
		if _, ok := spanishWordValues[token]; !ok {
			break
		}
		words = append([]string{token}, words...)
	}
	if len(words) == 0 {
		return 0, false
	}

	units := int64(0)
	for _, word := range words {
		if word == "mil" {
			if units == 0 {
				units = 1000
			} else {
				units *= 1000
			}
			continue
		}
		value, ok := spanishWordValues[word]
		if !ok {
			return 0, false
		}
		units += value
	}
	if units > maxCents/100 {
		return 0, false
	}

	cents := units * 100
	if extra, ok := centavosSuffix(texto); ok {
		cents += extra
	}
	return cents, true
}

func centavosSuffix(texto string) (int64, bool) {
	match := centsPattern.FindStringSubmatch(strings.ToLower(texto))
	if len(match) < 2 {
		return 0, false
	}
	if value, err := strconv.ParseInt(match[1], 10, 64); err == nil {
		return value, value <= 99
	}
	if value, ok := spanishWordValues[match[1]]; ok {
		return value, value <= 99
	}
	return 0, false
}
