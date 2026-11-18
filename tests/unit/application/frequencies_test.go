package application_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/nexumpr/Nexum-Backend/internal/application/catalogs"
	"github.com/nexumpr/Nexum-Backend/internal/domain/identity"
)

// Pruebas de aplicación de E023 (`GET /api/v1/catalogs/frequencies`): el catálogo es
// estático, versionado y cerrado. No hay persistencia que simular, de modo que lo que se
// verifica es el contenido exacto que el transporte serializa: siete valores canónicos,
// sin los alias heredados `biweekly` ni `yearly`.

// patronIDFrecuencia reproduce el patrón publicado de identificadores estables
// (`^[a-z][a-z0-9-]{0,39}$`), el mismo que aplica `Category.id`.
var patronIDFrecuencia = regexp.MustCompile(`^[a-z][a-z0-9-]{0,39}$`)

// TestE023CatalogoFrecuenciasCompleto cubre el contenido y el orden del Gate D-E023 y las
// etiquetas fijadas en RFC-020 §2.
func TestE023CatalogoFrecuenciasCompleto(t *testing.T) {
	items := catalogs.Frequencies()
	if len(items) != 7 {
		t.Fatalf("el catalogo debe tener 7 frecuencias, tiene %d", len(items))
	}

	esperado := []catalogs.Frequency{
		{ID: "weekly", Label: "Semanal"},
		{ID: "fortnightly", Label: "Quincenal"},
		{ID: "monthly", Label: "Mensual"},
		{ID: "bimonthly", Label: "Bimestral"},
		{ID: "quarterly", Label: "Trimestral"},
		{ID: "semiannual", Label: "Semestral"},
		{ID: "annual", Label: "Anual"},
	}
	for indice, frecuencia := range esperado {
		if items[indice] != frecuencia {
			t.Fatalf("posicion %d: esperado %+v, obtenido %+v", indice, frecuencia, items[indice])
		}
	}
}

// TestE023FrecuenciasAlineadasConE001 garantiza que el catálogo publica EXACTAMENTE el
// enum cerrado que el registro E001 acepta y persiste, en el mismo orden. Un cambio en el
// enum de E001 sin actualizar el catálogo (o al contrario) falla aquí.
func TestE023FrecuenciasAlineadasConE001(t *testing.T) {
	canonicas := identity.AllowedFrequencies()
	publicadas := catalogs.Frequencies()
	if len(publicadas) != len(canonicas) {
		t.Fatalf("E001 admite %d frecuencias y el catalogo publica %d", len(canonicas), len(publicadas))
	}
	for indice, frecuencia := range canonicas {
		if publicadas[indice].ID != string(frecuencia) {
			t.Fatalf("posicion %d: E001 persiste %q y el catalogo publica %q",
				indice, frecuencia, publicadas[indice].ID)
		}
		if _, err := identity.ParseFrequency(publicadas[indice].ID); err != nil {
			t.Fatalf("el catalogo publica %q, que E001 rechaza: %v", publicadas[indice].ID, err)
		}
	}
}

// TestE023SinAliasHeredados comprueba que `biweekly` y `yearly` no se listan. Permanecen
// en el enum del esquema publicado por compatibilidad (RFC-020 §3), pero no son ítems del
// catálogo porque E001 no los reconoce.
func TestE023SinAliasHeredados(t *testing.T) {
	if catalogs.AliasBiweekly != "biweekly" || catalogs.AliasYearly != "yearly" {
		t.Fatalf("constantes de alias alteradas: %q, %q", catalogs.AliasBiweekly, catalogs.AliasYearly)
	}

	alias := map[string]bool{catalogs.AliasBiweekly: true, catalogs.AliasYearly: true}
	for _, frecuencia := range catalogs.Frequencies() {
		if alias[frecuencia.ID] {
			t.Fatalf("el catalogo no debe emitir el alias heredado %q", frecuencia.ID)
		}
	}
	for _, frecuencia := range catalogs.Frequencies() {
		if _, err := identity.ParseFrequency(frecuencia.ID); err != nil {
			t.Fatalf("frecuencia no canonica en el catalogo: %q", frecuencia.ID)
		}
	}
}

// TestE023IdentificadoresYEtiquetasValidos verifica el patrón, la unicidad y la calidad
// de las etiquetas publicadas.
func TestE023IdentificadoresYEtiquetasValidos(t *testing.T) {
	vistos := map[string]bool{}
	for _, frecuencia := range catalogs.Frequencies() {
		if !patronIDFrecuencia.MatchString(frecuencia.ID) {
			t.Fatalf("id %q no cumple el patron publicado", frecuencia.ID)
		}
		if vistos[frecuencia.ID] {
			t.Fatalf("id duplicado en el catalogo: %q", frecuencia.ID)
		}
		vistos[frecuencia.ID] = true

		if frecuencia.Label == "" || strings.TrimSpace(frecuencia.Label) != frecuencia.Label {
			t.Fatalf("etiqueta invalida para %q: %q", frecuencia.ID, frecuencia.Label)
		}
	}
}

// TestE023CatalogoEsInmutable comprueba que el accesor devuelve una copia: mutar el
// resultado no puede alterar el catálogo que sirven las siguientes peticiones.
func TestE023CatalogoEsInmutable(t *testing.T) {
	primera := catalogs.Frequencies()
	primera[0].Label = "Alterado"
	primera[0].ID = "alterado"

	segunda := catalogs.Frequencies()
	if segunda[0].Label != "Semanal" || segunda[0].ID != "weekly" {
		t.Fatalf("el catalogo fue alterado por el llamador: %+v", segunda[0])
	}
}

// TestE023ClaveDeCacheVersionada comprueba que la clave pública incluye la versión
// publicada, es distinta de la de E022 y comparte el TTL de `max-age=300`.
func TestE023ClaveDeCacheVersionada(t *testing.T) {
	if !strings.HasPrefix(catalogs.FrequenciesCacheKey, "nexum:v1:catalog:frequencies:") {
		t.Fatalf("prefijo de clave inesperado: %q", catalogs.FrequenciesCacheKey)
	}
	if !strings.HasSuffix(catalogs.FrequenciesCacheKey, catalogs.ContentVersion) {
		t.Fatalf("la clave %q no incluye la version %q", catalogs.FrequenciesCacheKey, catalogs.ContentVersion)
	}
	if catalogs.FrequenciesCacheKey == catalogs.CategoriesCacheKey {
		t.Fatal("las claves de E022 y E023 no pueden coincidir")
	}
	if catalogs.CacheTTL.Seconds() != 300 {
		t.Fatalf("el TTL debe coincidir con max-age=300, obtenido %s", catalogs.CacheTTL)
	}
}
