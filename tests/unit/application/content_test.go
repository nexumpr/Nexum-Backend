package application_test

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	appContent "github.com/nexumpr/Nexum-Backend/internal/application/content"
)

// Pruebas del contenido editorial de E070. El contenido es estatico y versionado:
// estas pruebas fijan su contrato (diez entradas, identificadores estables, JSON con
// exactamente los campos del esquema) y evitan que se cuele texto de relleno, marcas
// o recomendaciones de inversion.

const faqEsperadas = 10

func TestContenidoFAQVersionYCache(t *testing.T) {
	if appContent.Version != "2026-11-v2" {
		t.Fatalf("version inesperada: %q", appContent.Version)
	}
	if appContent.CacheKeyFAQ != "nexum:v1:content:faq:2026-11-v2" {
		t.Fatalf("clave de cache inesperada: %q", appContent.CacheKeyFAQ)
	}
	if appContent.CacheTTL != 300*time.Second {
		t.Fatalf("TTL de cache inesperado: %s", appContent.CacheTTL)
	}
}

func TestContenidoFAQDiezEntradasReales(t *testing.T) {
	faqs := appContent.FAQs()
	if len(faqs) != faqEsperadas {
		t.Fatalf("se esperaban %d preguntas, hay %d", faqEsperadas, len(faqs))
	}

	vistos := make(map[string]bool, len(faqs))
	for i, faq := range faqs {
		esperado := fmt.Sprintf("faq-%02d", i+1)
		if faq.ID != esperado {
			t.Fatalf("identificador inestable en la posicion %d: %q (esperado %q)", i, faq.ID, esperado)
		}
		if vistos[faq.ID] {
			t.Fatalf("identificador duplicado: %s", faq.ID)
		}
		vistos[faq.ID] = true

		if !strings.HasSuffix(faq.Question, "?") || len([]rune(faq.Question)) < 20 {
			t.Fatalf("%s: la pregunta debe ser una oracion interrogativa completa: %q", faq.ID, faq.Question)
		}
		if len([]rune(faq.Answer)) < 120 {
			t.Fatalf("%s: la respuesta es demasiado corta para ser util (%d caracteres)", faq.ID, len([]rune(faq.Answer)))
		}
		revisarTextoPublico(t, faq.ID, faq.Question+" "+faq.Answer)
	}
}

// TestContenidoFAQCubreLosTemasDelCatalogo comprueba que el contenido no se queda
// en un subconjunto: cada tema exigido aparece al menos una vez.
func TestContenidoFAQCubreLosTemasDelCatalogo(t *testing.T) {
	texto := strings.ToLower(concatenarFAQ(appContent.FAQs()))
	temas := []struct {
		nombre    string
		variantes []string
	}{
		{"registro", []string{"registro", "creo mi cuenta"}},
		{"acceso", []string{"iniciar sesión", "mensaje de error de acceso"}},
		{"sesion", []string{"15 minutos", "sesión"}},
		{"renovacion y cierre", []string{"renovación", "cerrar sesión"}},
		{"recuperacion de contrasena", []string{"restablecimiento", "recupero"}},
		{"perfil", []string{"perfil"}},
		{"consentimientos", []string{"consentimiento"}},
		{"configuracion inicial", []string{"configuración inicial"}},
		{"ingreso", []string{"ingreso habitual"}},
		{"preferencias", []string{"preferencias"}},
	}
	for _, tema := range temas {
		cumple := false
		for _, variante := range tema.variantes {
			if strings.Contains(texto, variante) {
				cumple = true
				break
			}
		}
		if !cumple {
			t.Fatalf("el contenido no cubre el tema %q", tema.nombre)
		}
	}
}

func TestContenidoFAQPayloadExacto(t *testing.T) {
	datos, err := appContent.FAQPayload()
	if err != nil {
		t.Fatalf("error al serializar: %v", err)
	}
	if len(datos) == 0 || len(datos) > 16*1024 {
		t.Fatalf("tamano de payload inesperado: %d bytes", len(datos))
	}

	var sobre map[string]json.RawMessage
	if err := json.Unmarshal(datos, &sobre); err != nil {
		t.Fatalf("el payload no es un objeto JSON: %v", err)
	}
	if len(sobre) != 1 {
		t.Fatalf("el objeto debe tener solo 'items', tiene %d claves", len(sobre))
	}
	if _, existe := sobre["items"]; !existe {
		t.Fatal("falta la clave 'items'")
	}
	if _, existe := sobre["nextCursor"]; existe {
		t.Fatal("PageOfFaq no declara nextCursor: el contrato no tiene paginacion")
	}

	var items []map[string]json.RawMessage
	if err := json.Unmarshal(sobre["items"], &items); err != nil {
		t.Fatalf("'items' no es un arreglo: %v", err)
	}
	if len(items) != faqEsperadas {
		t.Fatalf("se esperaban %d elementos, hay %d", faqEsperadas, len(items))
	}
	for i, item := range items {
		claves := make(map[string]bool, len(item))
		for clave := range item {
			claves[clave] = true
		}
		for _, requerida := range []string{"id", "question", "answer"} {
			if !claves[requerida] {
				t.Fatalf("elemento %d sin el campo %q", i, requerida)
			}
		}
		if len(claves) != 3 {
			t.Fatalf("elemento %d con campos adicionales: %v", i, claves)
		}
	}

	segunda, err := appContent.FAQPayload()
	if err != nil {
		t.Fatalf("error en la segunda serializacion: %v", err)
	}
	if string(datos) != string(segunda) {
		t.Fatal("el payload debe ser determinista")
	}
}

// TestContenidoFAQAfirmacionesVerificables fija que el texto publicado solo anuncie funciones
// realmente desplegadas. La auditoria del 2026-11-24 detecto tres afirmaciones falsas: correo
// editable (E013 ProfilePatch solo admite displayName), cuatro frecuencias cuando E023 publica
// siete canonicas, y notificaciones/alertas push que no existen en esta version.
func TestContenidoFAQAfirmacionesVerificables(t *testing.T) {
	porID := make(map[string]appContent.Faq, len(appContent.FAQs()))
	for _, faq := range appContent.FAQs() {
		porID[faq.ID] = faq
	}

	perfil := strings.ToLower(porID["faq-06"].Answer)
	if strings.Contains(perfil, "mi correo") || strings.Contains(porID["faq-06"].Question, "correo") {
		t.Fatal("faq-06 no puede anunciar edicion de correo: el contrato de E013 no la admite")
	}
	if !strings.Contains(perfil, "no se edita") {
		t.Fatalf("faq-06 debe declarar que el correo no se edita desde la app: %q", porID["faq-06"].Answer)
	}
	if !strings.Contains(perfil, "nombre") {
		t.Fatalf("faq-06 debe limitar la edicion al nombre visible: %q", porID["faq-06"].Answer)
	}

	ingreso := strings.ToLower(porID["faq-09"].Answer)
	for _, frecuencia := range []string{"semanal", "quincenal", "mensual", "bimestral", "trimestral", "semestral", "anual"} {
		if !strings.Contains(ingreso, frecuencia) {
			t.Fatalf("faq-09 debe listar las siete frecuencias canonicas; falta %q", frecuencia)
		}
	}

	preferencias := strings.ToLower(porID["faq-10"].Answer)
	if !strings.Contains(preferencias, "idioma") || !strings.Contains(preferencias, "zona horaria") {
		t.Fatalf("faq-10 debe limitarse a idioma y zona horaria: %q", porID["faq-10"].Answer)
	}
	for _, prohibido := range []string{"horario silencioso", "alertas de presupuesto", "alertas push", "recordatorios programados"} {
		if strings.Contains(preferencias, prohibido) && !strings.Contains(preferencias, "no se env") {
			t.Fatalf("faq-10 no puede prometer %q sin declarar que no esta disponible", prohibido)
		}
	}
	if !strings.Contains(preferencias, "no se env") {
		t.Fatalf("faq-10 debe declarar que no se envian notificaciones ni alertas: %q", porID["faq-10"].Answer)
	}
}

func TestContenidoFAQCopiaAislada(t *testing.T) {
	primera := appContent.FAQs()
	primera[0].Question = "alterada"
	primera[0].Answer = "alterada"

	segunda := appContent.FAQs()
	if segunda[0].Question == "alterada" || segunda[0].Answer == "alterada" {
		t.Fatal("FAQs debe devolver una copia y no exponer el contenido compartido")
	}
}

func concatenarFAQ(faqs []appContent.Faq) string {
	var b strings.Builder
	for _, faq := range faqs {
		b.WriteString(faq.Question)
		b.WriteString(" ")
		b.WriteString(faq.Answer)
		b.WriteString(" ")
	}
	return b.String()
}

// revisarTextoPublico rechaza relleno, marcas comerciales y promesas de inversion.
func revisarTextoPublico(t *testing.T, id, texto string) {
	t.Helper()
	minusculas := strings.ToLower(texto)
	prohibidos := []string{
		"example.invalid", "example.com", "lorem", "ipsum", "placeholder", "tbd",
		"<", "{{", "}}", "xxx",
		"rendimiento garantizado", "te recomiendo invertir", "compra acciones", "invierte en",
	}
	for _, prohibido := range prohibidos {
		if strings.Contains(minusculas, prohibido) {
			t.Fatalf("%s: el contenido publico no admite %q", id, prohibido)
		}
	}
	// Marcas y productos: se buscan como palabra completa para no confundir
	// "Visa" con el verbo "revisa".
	marcas := []string{
		"visa", "mastercard", "amex", "paypal", "bitcoin", "ethereum", "criptomoneda",
		"amazon", "google", "apple", "facebook", "netflix", "spotify",
	}
	for _, marca := range marcas {
		if regexp.MustCompile(`\b` + regexp.QuoteMeta(marca) + `\b`).MatchString(minusculas) {
			t.Fatalf("%s: el contenido publico no admite la marca %q", id, marca)
		}
	}
}
