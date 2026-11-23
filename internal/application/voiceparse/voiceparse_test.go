package voiceparse

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"
)

var regexAmount = regexp.MustCompile(`^(0|[1-9][0-9]{0,11})\.[0-9]{2}$`)

func TestParseTextoValidoDeterminista(t *testing.T) {
	casos := []struct {
		nombre       string
		texto        string
		intent       string
		parsedAmount string
	}{
		{"ejemplo openapi", "Registrar gasto de cien pesos", IntentCreateMovement, "100.00"},
		{"entero", "Gasto de 100", IntentCreateMovement, "100.00"},
		{"decimal punto", "Pago de 100.50", IntentCreateMovement, "100.50"},
		{"miles y decimal", "Compra de 1,234.50", IntentCreateMovement, "1234.50"},
		{"formato latinoamericano", "Compra de 1.234,50", IntentCreateMovement, "1234.50"},
		{"palabras con mil", "Registrar dos mil pesos", IntentCreateMovement, "2000.00"},
		{"centavos en palabras", "Gasto de cien pesos con cincuenta", IntentCreateMovement, "100.50"},
		{"decima simple", "Gasto de 100,50 pesos", IntentCreateMovement, "100.50"},
		{"ambiguo sin monto", "Hola, ¿cómo estás?", IntentUnknown, "0.00"},
		{"cero", "Gasto de 0 pesos", IntentUnknown, "0.00"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			req := VoiceParseRequestWire{Text: caso.texto, Locale: LocaleESMX}
			primero, err := Parse(req)
			if err != nil {
				t.Fatalf("Parse devolvió error: %v", err)
			}
			if primero.Intent != caso.intent {
				t.Fatalf("intent=%q, esperado %q", primero.Intent, caso.intent)
			}
			if primero.ParsedAmount != caso.parsedAmount {
				t.Fatalf("parsedAmount=%q, esperado %q", primero.ParsedAmount, caso.parsedAmount)
			}
			if !primero.RequiresConfirmation {
				t.Fatal("requiresConfirmation debe ser siempre true")
			}
			if primero.Currency != CurrencyMXN {
				t.Fatalf("currency=%q, esperado MXN", primero.Currency)
			}
			segundo, err := Parse(req)
			if err != nil || segundo != primero {
				t.Fatalf("parser no determinista: %+v / %+v / %v", primero, segundo, err)
			}
			if !regexAmount.MatchString(primero.ParsedAmount) {
				t.Fatalf("parsedAmount no cumple el regex del contrato: %q", primero.ParsedAmount)
			}
		})
	}
}

func TestParseRespuestaSoloCamposDelContrato(t *testing.T) {
	resultado, err := NewService().Parse(VoiceParseRequestWire{Text: "Gasto de 100 pesos", Locale: LocaleESMX})
	if err != nil {
		t.Fatalf("Parse devolvió error: %v", err)
	}
	bytes, err := json.Marshal(resultado)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	campos := map[string]any{}
	if err := json.Unmarshal(bytes, &campos); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if len(campos) != 4 {
		t.Fatalf("la respuesta tiene campos extra: %s", bytes)
	}
}

func TestParseValidacionEstricta(t *testing.T) {
	casos := []struct {
		nombre string
		req    VoiceParseRequestWire
		field  string
		reason string
	}{
		{"locale distinto", VoiceParseRequestWire{Text: "Gasto de 100", Locale: "en-US"}, "locale", "invalid_enum"},
		{"locale con espacio", VoiceParseRequestWire{Text: "Gasto de 100", Locale: " es-MX"}, "locale", "invalid_enum"},
		{"texto vacio", VoiceParseRequestWire{Text: "", Locale: LocaleESMX}, "text", "required"},
		{"texto solo espacios", VoiceParseRequestWire{Text: "   ", Locale: LocaleESMX}, "text", "required"},
		{"texto demasiado largo", VoiceParseRequestWire{Text: strings.Repeat("á", MaxTextRunes+1), Locale: LocaleESMX}, "text", "too_long"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			_, err := Parse(caso.req)
			if !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("se esperaba ErrInvalidRequest, obtenido %v", err)
			}
			var reqErr *RequestError
			if !errors.As(err, &reqErr) || reqErr.Field != caso.field || reqErr.Reason != caso.reason {
				t.Fatalf("detalle inesperado: %#v", err)
			}
		})
	}
}

func TestParseRechazaMontosNegativos(t *testing.T) {
	for _, texto := range []string{"Gasto de -100 pesos", "Gasto de -100.50", "Gasto de menos cien pesos"} {
		t.Run(texto, func(t *testing.T) {
			_, err := Parse(VoiceParseRequestWire{Text: texto, Locale: LocaleESMX})
			if !errors.Is(err, ErrNegativeAmount) {
				t.Fatalf("se esperaba ErrNegativeAmount, obtenido %v", err)
			}
		})
	}
}

func TestParseRechazaMontosFueraDelRango(t *testing.T) {
	resultado, err := Parse(VoiceParseRequestWire{Text: "Gasto de 1000000000000 pesos", Locale: LocaleESMX})
	if err != nil {
		t.Fatalf("Parse devolvió error: %v", err)
	}
	if resultado.Intent != IntentUnknown || resultado.ParsedAmount != "0.00" {
		t.Fatalf("un monto fuera del contrato debe ser unknown: %+v", resultado)
	}
}
