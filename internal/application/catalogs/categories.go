// Package catalogs sirve los catálogos públicos versionados de la API: categorías
// financieras (E022) y frecuencias admitidas (E023).
//
// Los catálogos NO se leen de DynamoDB. H06 autoriza expresamente «recursos estáticos
// versionados» para catálogos (DYNAMODB_ACCESOS_H06 §4) y el Gate H08-E Oleada 1
// (D-E022 y D-E023) fija esa decisión: el contenido vive en código, se versiona con
// ContentVersion y se cachea en Redis bajo una clave pública. No se crea la
// partición `PUBLIC#`, no se hace `Scan` y no se amplía la política IAM.
package catalogs

import (
	"time"

	"github.com/nexumpr/Nexum-Backend/internal/domain/identity"
)

// ContentVersion identifica la versión publicada de los catálogos. Cambiar un
// identificador, un nombre, una etiqueta o el orden exige incrementarla: la clave de
// caché la incluye, de modo que una versión nueva nunca sirve contenido de la anterior.
const ContentVersion = "2026-11-v1"

// CacheTTL es la vigencia de la entrada pública en Redis. Coincide con el `max-age`
// de la cabecera `Cache-Control` que emite el transporte (`public, max-age=300`).
const CacheTTL = 300 * time.Second

// Tipos de categoría admitidos por el contrato (`Category.type`).
const (
	// TypeIncome agrupa las categorías de ingreso.
	TypeIncome = "income"
	// TypeExpense agrupa las categorías de gasto.
	TypeExpense = "expense"
	// TypeSaving agrupa las categorías de ahorro.
	TypeSaving = "saving"
)

// Category es un ítem del catálogo cerrado de categorías financieras.
//
// `ID` es el identificador estable que viaja en `Category.id`. El contrato impone el
// patrón `^[a-z][a-z0-9-]{0,39}$`, por lo que el identificador es la forma ASCII en
// minúsculas del valor correspondiente y `Name` conserva EXACTAMENTE el valor que
// E001 persiste en `recurringExpenses[].category`. Así el cliente puede enviar el
// `name` de vuelta al alta sin traducción y el identificador sigue siendo válido para
// el esquema publicado.
type Category struct {
	ID   string
	Name string
	Type string
}

// Nombres de las categorías que E001 acepta en el alta (enum cerrado de
// identity.AllowedCategories). El catálogo reutiliza esas constantes: no duplica la
// fuente de verdad.
const (
	// categoryIncomeName y categorySavingName completan el catálogo público. E001 no las
	// admite como compromiso fijo porque no son gastos, pero sí forman parte de las
	// categorías financieras de la aplicación.
	categoryIncomeName = "Ingreso"
	categorySavingName = "Ahorro"
)

// categorias es el catálogo publicado, en el orden del Gate D-E022. Es inmutable: los
// accesores devuelven copias para que ningún llamador pueda alterarlo.
var categorias = [...]Category{
	{ID: "vivienda", Name: identity.CategoryHousing, Type: TypeExpense},
	{ID: "servicios", Name: identity.CategoryServices, Type: TypeExpense},
	{ID: "transporte", Name: identity.CategoryTransport, Type: TypeExpense},
	{ID: "comida", Name: identity.CategoryFood, Type: TypeExpense},
	{ID: "otros", Name: identity.CategoryOther, Type: TypeExpense},
	{ID: "ingreso", Name: categoryIncomeName, Type: TypeIncome},
	{ID: "ahorro", Name: categorySavingName, Type: TypeSaving},
}

// Categories devuelve una copia del catálogo completo de categorías.
func Categories() []Category {
	salida := make([]Category, len(categorias))
	copy(salida, categorias[:])
	return salida
}

// CategoriesCacheKey es la clave pública de Redis del catálogo de categorías. Incluye
// la versión publicada; no contiene identidad de usuario (nunca se cachean datos
// personales bajo este espacio de claves).
const CategoriesCacheKey = "nexum:v1:catalog:categories:" + ContentVersion
