package export

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// rutaOpenAPI es la autoridad de transporte del contrato (espejo JSON de H06).
const rutaOpenAPI = "../../../sdd/03-endpoints/openapi/nexum-api-v1.json"

// TestE020FrecuenciaSerializaNullSinOmitEmpty comprueba que `frequency` viaja SIEMPRE
// en la proyección del recordatorio: `null` cuando no hay recurrencia y cadena canónica
// cuando la hay. `omitempty` haría indistinguible «sin recurrencia» de «campo ausente»
// para el titular que audita su exportación.
func TestE020FrecuenciaSerializaNullSinOmitEmpty(t *testing.T) {
	sinRecurrencia := recordatorioContrato(nil)
	crudo, err := json.Marshal(sinRecurrencia)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var objeto map[string]any
	if err := json.Unmarshal(crudo, &objeto); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	valor, presente := objeto["frequency"]
	if !presente {
		t.Fatalf("frequency debe estar presente aunque sea null: %s", crudo)
	}
	if valor != nil {
		t.Fatalf("frequency sin recurrencia debe ser null, obtenido %#v", valor)
	}

	for _, frecuencia := range []string{"daily", "weekly", "monthly"} {
		frecuencia := frecuencia
		crudo, err := json.Marshal(recordatorioContrato(&frecuencia))
		if err != nil {
			t.Fatalf("marshal %s: %v", frecuencia, err)
		}
		objeto = map[string]any{}
		if err := json.Unmarshal(crudo, &objeto); err != nil {
			t.Fatalf("unmarshal %s: %v", frecuencia, err)
		}
		if objeto["frequency"] != frecuencia {
			t.Fatalf("frequency=%#v, esperado %q (%s)", objeto["frequency"], frecuencia, crudo)
		}
	}
}

// TestE020FrecuenciaValidaContraOpenAPI valida el JSON realmente serializado contra el
// schema `ExportReminder` publicado, con un validador mínimo (tipo, enum, required y
// additionalProperties) sin dependencias nuevas.
func TestE020FrecuenciaValidaContraOpenAPI(t *testing.T) {
	raiz, esquema := cargarEsquemaExportReminder(t)

	// Control del propio validador: si no detecta violaciones, el resto no prueba nada.
	if problemas := validarDocumento(raiz, esquema, map[string]any{"id": "x"}); len(problemas) == 0 {
		t.Fatal("el validador no detecta un documento con campos obligatorios ausentes")
	}
	extra := documentoRecordatorio(t, nil)
	extra["campoInventado"] = "x"
	if problemas := validarDocumento(raiz, esquema, extra); len(problemas) == 0 {
		t.Fatal("el validador no detecta una propiedad adicional con additionalProperties=false")
	}
	malTipo := documentoRecordatorio(t, nil)
	malTipo["title"] = 42
	if problemas := validarDocumento(raiz, esquema, malTipo); len(problemas) == 0 {
		t.Fatal("el validador no detecta un tipo incorrecto")
	}

	for _, caso := range []struct {
		nombre    string
		doc       map[string]any
		problemas bool
	}{
		{"null es válido", documentoRecordatorio(t, nil), false},
		{"daily es válido", documentoRecordatorio(t, cadena("daily")), false},
		{"weekly es válido", documentoRecordatorio(t, cadena("weekly")), false},
		{"monthly es válido", documentoRecordatorio(t, cadena("monthly")), false},
		{"hourly no está en el enum", conFrecuencia(documentoRecordatorio(t, nil), "hourly"), true},
		{"cadena vacía no está en el enum", conFrecuencia(documentoRecordatorio(t, nil), ""), true},
		{"número no es string ni null", conFrecuencia(documentoRecordatorio(t, nil), 100), true},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			problemas := validarDocumento(raiz, esquema, caso.doc)
			if caso.problemas && len(problemas) == 0 {
				t.Fatalf("el documento debía rechazarse: %#v", caso.doc["frequency"])
			}
			if !caso.problemas && len(problemas) != 0 {
				t.Fatalf("el documento debía aceptarse: %v", problemas)
			}
		})
	}
}

// TestE020FrecuenciaNoEsObligatoriaPeroSeEmiteSiempre verifica las dos mitades de la
// decisión: el schema NO exige `frequency` (un recordatorio sin el campo es válido para
// consumidores antiguos) y el backend la emite siempre, incluido `null`.
func TestE020FrecuenciaNoEsObligatoriaPeroSeEmiteSiempre(t *testing.T) {
	raiz, esquema := cargarEsquemaExportReminder(t)
	requeridos, ok := esquema["required"].([]any)
	if !ok || len(requeridos) == 0 {
		t.Fatalf("ExportReminder debe declarar required: %#v", esquema["required"])
	}
	for _, campo := range requeridos {
		if campo == "frequency" {
			t.Fatal("frequency no debe figurar en required de ExportReminder")
		}
	}

	sinCampo := documentoRecordatorio(t, nil)
	delete(sinCampo, "frequency")
	if problemas := validarDocumento(raiz, esquema, sinCampo); len(problemas) != 0 {
		t.Fatalf("un documento sin frequency debe ser válido: %v", problemas)
	}

	// Pero el backend nunca omite el campo: la proyección siempre lo incluye.
	crudo, err := json.Marshal(recordatorioContrato(nil))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var objeto map[string]any
	if err := json.Unmarshal(crudo, &objeto); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, presente := objeto["frequency"]; !presente {
		t.Fatalf("la exportación debe emitir frequency siempre: %s", crudo)
	}
}

// TestE020ReminderContractRechazaFrecuenciaDesconocidaYNormalizaVacia cubre la
// proyección: un valor fuera del enum es un item incoherente (500, nunca un DTO
// inventado) y un valor vacío significa «sin recurrencia».
func TestE020ReminderContractRechazaFrecuenciaDesconocidaYNormalizaVacia(t *testing.T) {
	casos := []struct {
		nombre     string
		frecuencia *string
		esperado   *string
		err        bool
	}{
		{"sin frecuencia", nil, nil, false},
		{"cadena vacía", cadena(""), nil, false},
		{"sólo espacios", cadena("   "), nil, false},
		{"daily", cadena("daily"), cadena("daily"), false},
		{"weekly", cadena("weekly"), cadena("weekly"), false},
		{"monthly", cadena("monthly"), cadena("monthly"), false},
		{"con espacios alrededor", cadena(" weekly "), cadena("weekly"), false},
		{"hourly", cadena("hourly"), nil, true},
		{"mayúsculas fuera del enum", cadena("DAILY"), nil, true},
		{"valor arbitrario", cadena("cada-lunes"), nil, true},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			contrato, err := reminderContract(filaRecordatorio(caso.frecuencia))
			if caso.err {
				if !errors.Is(err, ErrInconsistent) {
					t.Fatalf("esperado ErrInconsistent, obtenido %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("reminderContract: %v", err)
			}
			if caso.esperado == nil {
				if contrato.Frequency != nil {
					t.Fatalf("frequency=%q, esperado nil", *contrato.Frequency)
				}
				return
			}
			if contrato.Frequency == nil || *contrato.Frequency != *caso.esperado {
				t.Fatalf("frequency=%v, esperado %q", contrato.Frequency, *caso.esperado)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Utilidades locales de prueba (sin dependencias nuevas)
// ---------------------------------------------------------------------------

func cadena(valor string) *string { return &valor }

func filaRecordatorio(frecuencia *string) ReminderRow {
	return ReminderRow{
		ID:        "8f14e45f-ceea-4a3f-9c1e-2b3f6a0d7c11",
		Title:     "Pagar luz",
		DueAt:     time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC),
		Timezone:  "America/Mexico_City",
		Frequency: frecuencia,
		Active:    true,
		CreatedAt: time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 11, 23, 9, 30, 0, 0, time.UTC),
	}
}

func recordatorioContrato(frecuencia *string) ReminderContract {
	contrato, err := reminderContract(filaRecordatorio(frecuencia))
	if err != nil {
		panic(fmt.Sprintf("recordatorio de prueba inválido: %v", err))
	}
	return contrato
}

// documentoRecordatorio serializa la proyección y la devuelve como objeto JSON genérico.
func documentoRecordatorio(t *testing.T, frecuencia *string) map[string]any {
	t.Helper()
	crudo, err := json.Marshal(recordatorioContrato(frecuencia))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var objeto map[string]any
	if err := json.Unmarshal(crudo, &objeto); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return objeto
}

func conFrecuencia(doc map[string]any, valor any) map[string]any {
	doc["frequency"] = valor
	return doc
}

// cargarEsquemaExportReminder lee el OpenAPI publicado y devuelve la raíz (para resolver
// `$ref` locales) y el schema `ExportReminder`. Si el documento no se puede leer, la
// prueba falla: no se asume un contrato en memoria.
func cargarEsquemaExportReminder(t *testing.T) (map[string]any, map[string]any) {
	t.Helper()
	ruta, err := filepath.Abs(rutaOpenAPI)
	if err != nil {
		t.Fatalf("ruta OpenAPI: %v", err)
	}
	contenido, err := os.ReadFile(ruta)
	if err != nil {
		t.Fatalf("leer OpenAPI %s: %v", ruta, err)
	}
	var raiz map[string]any
	if err := json.Unmarshal(contenido, &raiz); err != nil {
		t.Fatalf("OpenAPI no es JSON válido: %v", err)
	}
	componentes, ok := raiz["components"].(map[string]any)
	if !ok {
		t.Fatal("OpenAPI sin components")
	}
	esquemas, ok := componentes["schemas"].(map[string]any)
	if !ok {
		t.Fatal("OpenAPI sin components.schemas")
	}
	esquema, ok := esquemas["ExportReminder"].(map[string]any)
	if !ok {
		t.Fatal("OpenAPI sin schema ExportReminder")
	}
	return raiz, esquema
}

// validarDocumento aplica las palabras clave que usa `ExportReminder` (type, enum,
// required, additionalProperties, minLength y maxLength) y resuelve `$ref` locales.
// Devuelve la lista de problemas: vacía significa válido.
func validarDocumento(raiz, esquema map[string]any, valor any) []string {
	return validarNodo(raiz, esquema, valor, "exportReminder")
}

func validarNodo(raiz, esquema map[string]any, valor any, ruta string) []string {
	if referencia, ok := esquema["$ref"].(string); ok {
		resuelto, err := resolverRef(raiz, referencia)
		if err != nil {
			return []string{ruta + ": " + err.Error()}
		}
		return validarNodo(raiz, resuelto, valor, ruta)
	}

	var problemas []string
	if enum, ok := esquema["enum"].([]any); ok {
		coincide := false
		for _, permitido := range enum {
			if igualJSON(permitido, valor) {
				coincide = true
				break
			}
		}
		if !coincide {
			problemas = append(problemas, fmt.Sprintf("%s: %#v no está en el enum", ruta, valor))
		}
	}

	if bruto, ok := esquema["type"]; ok {
		tipos := tiposDe(bruto)
		if len(tipos) > 0 && !coincideTipo(tipos, valor) {
			return append(problemas, fmt.Sprintf("%s: tipo %T no admitido por %v", ruta, valor, tipos))
		}
	}

	if objeto, ok := valor.(map[string]any); ok {
		if propiedades, ok := esquema["properties"].(map[string]any); ok {
			if requeridos, ok := esquema["required"].([]any); ok {
				for _, requerido := range requeridos {
					nombre, _ := requerido.(string)
					if _, presente := objeto[nombre]; !presente {
						problemas = append(problemas, fmt.Sprintf("%s: falta el campo obligatorio %q", ruta, nombre))
					}
				}
			}
			for clave, hijo := range objeto {
				definicion, declarada := propiedades[clave].(map[string]any)
				if !declarada {
					if adicionales, existe := esquema["additionalProperties"]; existe && adicionales == false {
						problemas = append(problemas, fmt.Sprintf("%s: propiedad no declarada %q", ruta, clave))
					}
					continue
				}
				problemas = append(problemas, validarNodo(raiz, definicion, hijo, ruta+"."+clave)...)
			}
		}
	}

	if texto, ok := valor.(string); ok {
		if minimo, ok := numero(esquema["minLength"]); ok && float64(len([]rune(texto))) < minimo {
			problemas = append(problemas, fmt.Sprintf("%s: longitud menor que %v", ruta, minimo))
		}
		if maximo, ok := numero(esquema["maxLength"]); ok && float64(len([]rune(texto))) > maximo {
			problemas = append(problemas, fmt.Sprintf("%s: longitud mayor que %v", ruta, maximo))
		}
	}
	return problemas
}

func resolverRef(raiz map[string]any, referencia string) (map[string]any, error) {
	if len(referencia) < 2 || referencia[0] != '#' || referencia[1] != '/' {
		return nil, fmt.Errorf("$ref no local no soportado: %s", referencia)
	}
	nodo := any(raiz)
	for _, parte := range splitRef(referencia[2:]) {
		objeto, ok := nodo.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("$ref no resoluble: %s", referencia)
		}
		nodo, ok = objeto[parte]
		if !ok {
			return nil, fmt.Errorf("$ref no resoluble: %s", referencia)
		}
	}
	esquema, ok := nodo.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("$ref no apunta a un schema: %s", referencia)
	}
	return esquema, nil
}

func splitRef(ruta string) []string {
	partes := []string{}
	actual := ""
	for _, caracter := range ruta {
		switch caracter {
		case '/':
			partes = append(partes, actual)
			actual = ""
		default:
			actual += string(caracter)
		}
	}
	return append(partes, actual)
}

func tiposDe(bruto any) []string {
	switch valor := bruto.(type) {
	case string:
		return []string{valor}
	case []any:
		tipos := make([]string, 0, len(valor))
		for _, item := range valor {
			if texto, ok := item.(string); ok {
				tipos = append(tipos, texto)
			}
		}
		return tipos
	default:
		return nil
	}
}

func coincideTipo(tipos []string, valor any) bool {
	for _, tipo := range tipos {
		switch tipo {
		case "null":
			if valor == nil {
				return true
			}
		case "string":
			if _, ok := valor.(string); ok {
				return true
			}
		case "boolean":
			if _, ok := valor.(bool); ok {
				return true
			}
		case "number":
			if _, ok := valor.(float64); ok {
				return true
			}
		case "integer":
			if numero, ok := valor.(float64); ok && numero == float64(int64(numero)) {
				return true
			}
		case "object":
			if _, ok := valor.(map[string]any); ok {
				return true
			}
		case "array":
			if _, ok := valor.([]any); ok {
				return true
			}
		}
	}
	return false
}

func numero(valor any) (float64, bool) {
	numero, ok := valor.(float64)
	return numero, ok
}

func igualJSON(a, b any) bool {
	crudoA, errA := json.Marshal(a)
	crudoB, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		return false
	}
	return string(crudoA) == string(crudoB)
}
