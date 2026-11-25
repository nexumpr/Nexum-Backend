package voiceparse

import (
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// TestE075ImportesDeterministas cubre formatos numéricos y palabras del
// contrato: separador de miles, decimal con coma o punto, centavos y palabras.
func TestE075ImportesDeterministas(t *testing.T) {
	casos := []struct {
		texto   string
		importe string
	}{
		{"Registrar gasto de 1,234 pesos", "1234.00"},
		{"Pagué 1.234,50 pesos", "1234.50"},
		{"Gasto de 1.234 pesos", "1234.00"},
		{"Café 12,34 pesos", "12.34"},
		{"Ingreso de 100.50", "100.50"},
		{"Un gasto de cien pesos", "100.00"},
		{"Cien pesos con cincuenta centavos", "100.50"},
		{"Gasto de 100 pesos con 25 centavos", "100.25"},
	}
	for _, caso := range casos {
		t.Run(caso.texto, func(t *testing.T) {
			resultado, err := Parse(VoiceParseRequestWire{Text: caso.texto, Locale: LocaleESMX})
			if err != nil {
				t.Fatalf("Parse(%q): %v", caso.texto, err)
			}
			if resultado.Intent != IntentCreateMovement {
				t.Fatalf("intent=%q, esperado %q", resultado.Intent, IntentCreateMovement)
			}
			if resultado.ParsedAmount != caso.importe {
				t.Fatalf("importe=%q, esperado %q", resultado.ParsedAmount, caso.importe)
			}
			if resultado.Currency != CurrencyMXN || !resultado.RequiresConfirmation {
				t.Fatalf("respuesta fuera de contrato: %+v", resultado)
			}
		})
	}
}

// TestE075TextosAmbiguosNoInventanImporte valida que la ambigüedad se resuelve
// como «unknown» con 0.00, nunca como una cifra inventada.
func TestE075TextosAmbiguosNoInventanImporte(t *testing.T) {
	for _, texto := range []string{
		"Hola",
		"Gasto de cien",        // sin divisa: no se asume moneda
		"Gasto de 0 pesos",     // cero no es un movimiento
		"9999999999999 pesos",  // fuera del máximo exacto
		"El recibo llegó ayer", // sin importe
	} {
		t.Run(texto, func(t *testing.T) {
			resultado, err := Parse(VoiceParseRequestWire{Text: texto, Locale: LocaleESMX})
			if err != nil {
				t.Fatalf("Parse(%q): %v", texto, err)
			}
			if resultado.Intent != IntentUnknown {
				t.Fatalf("intent=%q, esperado %q", resultado.Intent, IntentUnknown)
			}
			if resultado.ParsedAmount != "0.00" || resultado.Currency != CurrencyMXN {
				t.Fatalf("respuesta ambigua fuera de contrato: %+v", resultado)
			}
			if !resultado.RequiresConfirmation {
				t.Fatal("toda interpretación exige confirmación del usuario")
			}
		})
	}
}

// TestE075NegativosYLocaleSonRechazosDistintos separa error de negocio (422) de
// error de solicitud (400) sin persistir ni ejecutar movimientos.
func TestE075NegativosYLocaleSonRechazosDistintos(t *testing.T) {
	for _, texto := range []string{"Gasto de -100 pesos", "menos cien pesos"} {
		if _, err := Parse(VoiceParseRequestWire{Text: texto, Locale: LocaleESMX}); !errors.Is(err, ErrNegativeAmount) {
			t.Fatalf("%q: esperado ErrNegativeAmount, obtenido %v", texto, err)
		}
	}

	casos := []struct {
		nombre string
		req    VoiceParseRequestWire
		campo  string
	}{
		{"locale distinto", VoiceParseRequestWire{Text: "Gasto de 100 pesos", Locale: "en-US"}, "locale"},
		{"locale vacío", VoiceParseRequestWire{Text: "Gasto de 100 pesos"}, "locale"},
		{"texto vacío", VoiceParseRequestWire{Text: "", Locale: LocaleESMX}, "text"},
		{"texto en blanco", VoiceParseRequestWire{Text: "   ", Locale: LocaleESMX}, "text"},
		{"texto demasiado largo", VoiceParseRequestWire{Text: strings.Repeat("a", MaxTextRunes+1), Locale: LocaleESMX}, "text"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			_, err := Parse(caso.req)
			if !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("esperado ErrInvalidRequest, obtenido %v", err)
			}
			var solicitud *RequestError
			if !errors.As(err, &solicitud) || solicitud.Field != caso.campo {
				t.Fatalf("RequestError inesperado: %#v", err)
			}
			if strings.Contains(err.Error(), caso.req.Text) && caso.req.Text != "" {
				t.Fatalf("el error no debe repetir el texto del usuario: %v", err)
			}
		})
	}
}

// TestE075SinPersistenciaNiEfectos comprueba que el servicio es puro: no tiene
// dependencias, no muta la entrada y es determinista bajo concurrencia.
func TestE075SinPersistenciaNiEfectos(t *testing.T) {
	if campos := reflect.TypeOf(Service{}).NumField(); campos != 0 {
		t.Fatalf("el servicio de voz no debe tener dependencias: %d campos", campos)
	}
	entrada := VoiceParseRequestWire{Text: "Registrar gasto de cien pesos", Locale: LocaleESMX}
	copia := entrada
	primero, err := NewService().Parse(entrada)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if entrada != copia {
		t.Fatal("Parse no debe mutar la solicitud")
	}
	segundo, err := Parse(entrada)
	if err != nil || primero != segundo {
		t.Fatalf("Parse debe ser determinista: %+v vs %+v err=%v", primero, segundo, err)
	}

	var espera sync.WaitGroup
	resultados := make(chan VoiceParseWire, 32)
	for i := 0; i < 32; i++ {
		espera.Add(1)
		go func() {
			defer espera.Done()
			resultado, err := Parse(entrada)
			if err != nil {
				t.Errorf("Parse concurrente: %v", err)
				return
			}
			resultados <- resultado
		}()
	}
	espera.Wait()
	close(resultados)
	for resultado := range resultados {
		if resultado != primero {
			t.Fatalf("resultado no determinista: %+v", resultado)
		}
	}
}
