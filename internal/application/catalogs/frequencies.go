package catalogs

import "github.com/nexumpr/Nexum-Backend/internal/domain/identity"

// Frecuencias públicas (E023).
//
// El contrato publicado de `Frequency.id` conserva los alias heredados `biweekly` y
// `yearly` por compatibilidad (RFC-020 §3: ningún valor publicado se elimina), pero el
// catálogo emite EXCLUSIVAMENTE los siete valores canónicos que E001 acepta y persiste.
// Los alias no son ítems del catálogo: sólo se aceptan como entrada en E025 y se
// normalizan antes de persistir. Publicarlos aquí invitaría al cliente a enviar valores
// que el registro y las operaciones recurrentes no reconocen.
//
// Los identificadores se toman del enum de dominio (`identity.Frequency*`): no se duplica
// la fuente de verdad. El orden y las etiquetas son los del Gate D-E023 / RFC-020 §2.

// Frequency es un ítem del catálogo cerrado de frecuencias admitidas.
//
// `ID` es el valor estable que viaja en `Frequency.id` y que E001 persiste en
// `recurringExpenses[].frequency`; `Label` es la etiqueta en español del contrato.
type Frequency struct {
	ID    string
	Label string
}

// Alias heredados del contrato publicado. NO se emiten como ítems del catálogo: se
// declaran para documentar la decisión y para que las pruebas comprueben su ausencia.
const (
	// AliasBiweekly es el alias heredado de `fortnightly`; sólo se acepta en la entrada
	// de E025.
	AliasBiweekly = "biweekly"
	// AliasYearly es el alias heredado de `annual`; sólo se acepta en la entrada de E025.
	AliasYearly = "yearly"
)

// frecuencias es el catálogo publicado, en el orden del Gate D-E023 / RFC-020 §2. Es
// inmutable: el accesor devuelve una copia para que ningún llamador pueda alterarlo.
var frecuencias = [...]Frequency{
	{ID: string(identity.FrequencyWeekly), Label: "Semanal"},
	{ID: string(identity.FrequencyFortnightly), Label: "Quincenal"},
	{ID: string(identity.FrequencyMonthly), Label: "Mensual"},
	{ID: string(identity.FrequencyBimonthly), Label: "Bimestral"},
	{ID: string(identity.FrequencyQuarterly), Label: "Trimestral"},
	{ID: string(identity.FrequencySemiannual), Label: "Semestral"},
	{ID: string(identity.FrequencyAnnual), Label: "Anual"},
}

// Frequencies devuelve una copia del catálogo completo de frecuencias canónicas.
func Frequencies() []Frequency {
	salida := make([]Frequency, len(frecuencias))
	copy(salida, frecuencias[:])
	return salida
}

// FrequenciesCacheKey es la clave pública de Redis del catálogo de frecuencias. Incluye
// la versión publicada; no contiene identidad de usuario.
const FrequenciesCacheKey = "nexum:v1:catalog:frequencies:" + ContentVersion
