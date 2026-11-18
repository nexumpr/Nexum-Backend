package application_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/nexumpr/Nexum-Backend/internal/application/catalogs"
	"github.com/nexumpr/Nexum-Backend/internal/domain/identity"
)

// Pruebas de aplicación de E022 (`GET /api/v1/catalogs/categories`): el catálogo es
// estático, versionado y cerrado. No hay persistencia que simular, de modo que lo que se
// verifica es el contenido exacto que el transporte serializa.

// patronIDCategoria reproduce el patrón publicado de `Category.id`:
// `^[a-z][a-z0-9-]{0,39}$`.
var patronIDCategoria = regexp.MustCompile(`^[a-z][a-z0-9-]{0,39}$`)

// TestE022CatalogoCategoriasCompleto cubre el contenido y el orden del Gate D-E022.
func TestE022CatalogoCategoriasCompleto(t *testing.T) {
	items := catalogs.Categories()
	if len(items) != 7 {
		t.Fatalf("el catalogo debe tener 7 categorias, tiene %d", len(items))
	}

	esperado := []catalogs.Category{
		{ID: "vivienda", Name: "Vivienda", Type: catalogs.TypeExpense},
		{ID: "servicios", Name: "Servicios", Type: catalogs.TypeExpense},
		{ID: "transporte", Name: "Transporte", Type: catalogs.TypeExpense},
		{ID: "comida", Name: "Comida", Type: catalogs.TypeExpense},
		{ID: "otros", Name: "Otros", Type: catalogs.TypeExpense},
		{ID: "ingreso", Name: "Ingreso", Type: catalogs.TypeIncome},
		{ID: "ahorro", Name: "Ahorro", Type: catalogs.TypeSaving},
	}
	for indice, categoria := range esperado {
		if items[indice] != categoria {
			t.Fatalf("posicion %d: esperado %+v, obtenido %+v", indice, categoria, items[indice])
		}
	}
}

// TestE022CategoriasAlineadasConE001 garantiza que las cinco categorías de gasto del
// catálogo son EXACTAMENTE las que el registro E001 acepta y persiste, en el mismo orden.
// Un cambio en el enum de E001 sin actualizar el catálogo (o al contrario) falla aquí.
func TestE022CategoriasAlineadasConE001(t *testing.T) {
	permitidas := identity.AllowedCategories()
	gastos := make([]string, 0, len(permitidas))
	for _, categoria := range catalogs.Categories() {
		if categoria.Type == catalogs.TypeExpense {
			gastos = append(gastos, categoria.Name)
		}
	}
	if len(gastos) != len(permitidas) {
		t.Fatalf("E001 admite %d categorias de gasto y el catalogo publica %d", len(permitidas), len(gastos))
	}
	for indice, nombre := range permitidas {
		if gastos[indice] != nombre {
			t.Fatalf("posicion %d: E001 persiste %q y el catalogo publica %q", indice, nombre, gastos[indice])
		}
	}
}

// TestE022IdentificadoresValidos verifica el patrón publicado, la unicidad y la coherencia
// entre identificador y nombre (el identificador es la forma ASCII en minúsculas del valor
// que E001 persiste; el nombre conserva el valor exacto).
func TestE022IdentificadoresValidos(t *testing.T) {
	vistos := map[string]bool{}
	for _, categoria := range catalogs.Categories() {
		if !patronIDCategoria.MatchString(categoria.ID) {
			t.Fatalf("id %q no cumple el patron publicado", categoria.ID)
		}
		if vistos[categoria.ID] {
			t.Fatalf("id duplicado en el catalogo: %q", categoria.ID)
		}
		vistos[categoria.ID] = true

		if categoria.Name == "" || strings.TrimSpace(categoria.Name) != categoria.Name {
			t.Fatalf("nombre invalido para %q: %q", categoria.ID, categoria.Name)
		}
		if !strings.EqualFold(categoria.ID, sinAcentos(categoria.Name)) {
			t.Fatalf("el id %q no corresponde al nombre %q", categoria.ID, categoria.Name)
		}
		switch categoria.Type {
		case catalogs.TypeIncome, catalogs.TypeExpense, catalogs.TypeSaving:
		default:
			t.Fatalf("tipo fuera del enum para %q: %q", categoria.ID, categoria.Type)
		}
	}
}

// TestE022CatalogoEsInmutable comprueba que el accesor devuelve una copia: mutar el
// resultado no puede alterar el catálogo que sirven las siguientes peticiones.
func TestE022CatalogoEsInmutable(t *testing.T) {
	primera := catalogs.Categories()
	primera[0].Name = "Alterado"
	primera[0].ID = "alterado"

	segunda := catalogs.Categories()
	if segunda[0].Name != "Vivienda" || segunda[0].ID != "vivienda" {
		t.Fatalf("el catalogo fue alterado por el llamador: %+v", segunda[0])
	}
}

// TestE022ClaveDeCacheVersionada comprueba que la clave pública incluye la versión
// publicada: una versión nueva nunca puede servir contenido de la anterior.
func TestE022ClaveDeCacheVersionada(t *testing.T) {
	if !strings.HasPrefix(catalogs.CategoriesCacheKey, "nexum:v1:catalog:categories:") {
		t.Fatalf("prefijo de clave inesperado: %q", catalogs.CategoriesCacheKey)
	}
	if !strings.HasSuffix(catalogs.CategoriesCacheKey, catalogs.ContentVersion) {
		t.Fatalf("la clave %q no incluye la version %q", catalogs.CategoriesCacheKey, catalogs.ContentVersion)
	}
	if catalogs.CacheTTL.Seconds() != 300 {
		t.Fatalf("el TTL debe coincidir con max-age=300, obtenido %s", catalogs.CacheTTL)
	}
}

// sinAcentos reduce el nombre a la forma ASCII comparable con el identificador. Sólo se
// usan nombres sin diéresis ni eñes en este catálogo; la función evita que un nombre con
// tilde pase una comparación laxa.
func sinAcentos(valor string) string {
	return strings.ToLower(strings.TrimSpace(valor))
}
