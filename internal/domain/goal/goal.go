// Package goal contiene el agregado de meta de ahorro: validaciones de nombre, importes
// exactos en céntimos, fecha objetivo, estados y transiciones admitidas.
//
// Invariantes (E039/E040/E041, A7 de RFC-016):
//   - El monto objetivo es ESTRICTAMENTE positivo y no supera el máximo representable por el
//     patrón publicado (`^(0|[1-9][0-9]{0,11})\.[0-9]{2}$`).
//   - El ahorro de apertura es opcional, nunca negativo y nunca superior al monto objetivo.
//   - `savedAmount` arranca igual al ahorro de apertura; `openingSaved` conserva ese valor
//     histórico y NO se modifica por E041. Así `contributedNet = savedAmount - openingSaved`
//     es calculable por el cliente sin inventar cifras (A7).
//   - Los importes se operan como `int64` de céntimos a través de `internal/domain/money`:
//     no existe ningún camino con punto flotante (R-030, GOV-08).
package goal

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nexumpr/Nexum-Backend/internal/domain/money"
)

const (
	// NameMaxRunes es `maxLength: 120` de `GoalCreate.name`/`Goal.name`, contado en
	// caracteres (runas), no en bytes.
	NameMaxRunes = 120

	// MaxAmountCents es el mayor importe que admite el patrón publicado: doce dígitos
	// enteros y dos decimales (999 999 999 999.99). Cabe holgadamente en int64.
	MaxAmountCents int64 = 99_999_999_999_999

	// CurrencyMXN es la única divisa del contrato de metas (`GoalCreate.currency`
	// publica el enum cerrado `[MXN]`).
	CurrencyMXN = "MXN"

	// DateLayout es `format: date` del contrato: fecha civil sin zona horaria.
	DateLayout = "2006-01-02"
)

// Errores de dominio. La capa de aplicación los traduce a códigos HTTP del contrato; el
// dominio no conoce HTTP ni AWS.
var (
	// ErrNameInvalid indica un nombre vacío, sólo espacios o de más de 120 caracteres.
	ErrNameInvalid = errors.New("nombre de meta invalido")

	// ErrCurrencyInvalid indica una divisa distinta de MXN.
	ErrCurrencyInvalid = errors.New("divisa de meta invalida")

	// ErrAmountOutOfRange indica un importe negativo, no entero en céntimos o por encima
	// del máximo publicado.
	ErrAmountOutOfRange = errors.New("importe de meta fuera de rango")

	// ErrTargetNotPositive indica un monto objetivo igual a cero: el contrato exige que sea
	// positivo (E039 §1).
	ErrTargetNotPositive = errors.New("el monto objetivo debe ser positivo")

	// ErrOpeningOutOfRange indica un ahorro inicial negativo o superior al monto objetivo.
	ErrOpeningOutOfRange = errors.New("el ahorro inicial debe estar entre 0 y el monto objetivo")

	// ErrTargetBelowSaved indica que se intenta reducir el objetivo por debajo del ahorro ya
	// acumulado (E041 §1, §4).
	ErrTargetBelowSaved = errors.New("el monto objetivo no puede ser menor que el ahorro acumulado")

	// ErrTargetDateInvalid indica una fecha objetivo vacía o no representable.
	ErrTargetDateInvalid = errors.New("fecha objetivo invalida")

	// ErrStatusInvalid indica un estado fuera del enum `active|achieved|archived`.
	ErrStatusInvalid = errors.New("estado de meta invalido")

	// ErrTransitionInvalid indica una transición no admitida entre estados.
	ErrTransitionInvalid = errors.New("transicion de estado no admitida")

	// ErrEmptyPatch indica una modificación sin ningún campo aplicable.
	ErrEmptyPatch = errors.New("la modificacion no contiene cambios")

	// ErrInconsistent indica una meta persistida cuya forma no corresponde al contrato. Se
	// falla cerrado en lugar de emitir un valor fuera del enum o un importe inventado.
	ErrInconsistent = errors.New("meta persistida incoherente")
)

// Status es el enum cerrado de `Goal.status`.
type Status string

const (
	// StatusActive es una meta en curso.
	StatusActive Status = "active"
	// StatusAchieved es una meta alcanzada.
	StatusAchieved Status = "achieved"
	// StatusArchived es una meta archivada por el titular.
	StatusArchived Status = "archived"
)

// ParseStatus normaliza y valida el enum publicado. No acepta mayúsculas ni alias: el
// contrato es cerrado y un valor desconocido debe rechazarse, no adivinarse.
func ParseStatus(raw string) (Status, error) {
	switch strings.TrimSpace(raw) {
	case string(StatusActive):
		return StatusActive, nil
	case string(StatusAchieved):
		return StatusAchieved, nil
	case string(StatusArchived):
		return StatusArchived, nil
	default:
		return "", ErrStatusInvalid
	}
}

// String devuelve el valor canónico del enum.
func (s Status) String() string {
	return string(s)
}

// CanTransitionTo decide si un cambio de estado es coherente.
//
// Reglas (documentadas porque el contrato no las enumera explícitamente):
//   - active   -> active | achieved | archived
//   - achieved -> achieved | active | archived  (reabrir una meta alcanzada es legítimo)
//   - archived -> archived | active             (restaurar exige pasar por active; NO se
//     admite archived -> achieved de forma directa porque afirmaría una meta alcanzada
//     sobre un recurso que el titular había retirado, sin que exista un periodo activo)
//
// Repetir el mismo estado se acepta: es una escritura explícita con CAS, no una transición.
func (s Status) CanTransitionTo(next Status) bool {
	switch s {
	case StatusActive:
		return next == StatusActive || next == StatusAchieved || next == StatusArchived
	case StatusAchieved:
		return next == StatusAchieved || next == StatusActive || next == StatusArchived
	case StatusArchived:
		return next == StatusArchived || next == StatusActive
	default:
		return false
	}
}

// Goal es la meta de ahorro del titular.
//
// No contiene identificadores de persistencia (PK/SK): esos viven en el adaptador. `Version`
// es la versión optimista que se publica como `ETag` fuerte `"vN"` y que E041 exige en
// `If-Match`.
type Goal struct {
	ID           string
	UserID       string
	Name         string
	TargetAmount money.Money
	SavedAmount  money.Money
	OpeningSaved money.Money
	TargetDate   time.Time
	Status       Status
	Version      int64
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// NormalizeName recorta espacios y valida la longitud en runas. Devuelve el nombre que debe
// persistirse, de modo que no existan dos representaciones del mismo nombre.
func NormalizeName(raw string) (string, error) {
	clean := strings.TrimSpace(raw)
	if clean == "" {
		return "", ErrNameInvalid
	}
	if utf8.RuneCountInString(clean) > NameMaxRunes {
		return "", ErrNameInvalid
	}
	return clean, nil
}

// ValidateTargetAmountCents valida el monto objetivo como regla de negocio: positivo y
// dentro del máximo representable.
func ValidateTargetAmountCents(cents int64) error {
	if cents <= 0 {
		return ErrTargetNotPositive
	}
	if cents > MaxAmountCents {
		return ErrAmountOutOfRange
	}
	return nil
}

// ValidateOpeningCents valida el ahorro de apertura contra el monto objetivo.
func ValidateOpeningCents(openingCents, targetCents int64) error {
	if openingCents < 0 || openingCents > targetCents {
		return ErrOpeningOutOfRange
	}
	return nil
}

// ValidateTargetDate valida una fecha civil ya parseada.
func ValidateTargetDate(value time.Time) error {
	if value.IsZero() || value.Year() < 1 {
		return ErrTargetDateInvalid
	}
	return nil
}

// FormatTargetDate serializa la fecha con el formato `date` del contrato.
func (g Goal) FormatTargetDate() string {
	return g.TargetDate.UTC().Format(DateLayout)
}

// Validate comprueba los invariantes del agregado. Se aplica al construir y al leer de
// persistencia: una fila incoherente no se emite nunca al cliente.
func (g Goal) Validate() error {
	if strings.TrimSpace(g.ID) == "" || strings.TrimSpace(g.UserID) == "" {
		return ErrInconsistent
	}
	if _, err := NormalizeName(g.Name); err != nil {
		return ErrInconsistent
	}
	if err := validateStoredMoney(g.TargetAmount); err != nil {
		return ErrInconsistent
	}
	if err := validateStoredMoney(g.SavedAmount); err != nil {
		return ErrInconsistent
	}
	if err := validateStoredMoney(g.OpeningSaved); err != nil {
		return ErrInconsistent
	}
	if g.TargetAmount.Currency() != CurrencyMXN ||
		g.SavedAmount.Currency() != g.TargetAmount.Currency() ||
		g.OpeningSaved.Currency() != g.TargetAmount.Currency() {
		return ErrInconsistent
	}
	if g.TargetAmount.Amount() <= 0 || g.TargetAmount.Amount() > MaxAmountCents {
		return ErrInconsistent
	}
	if g.OpeningSaved.Amount() > g.SavedAmount.Amount() {
		return ErrInconsistent
	}
	if _, err := ParseStatus(g.Status.String()); err != nil {
		return ErrInconsistent
	}
	if err := ValidateTargetDate(g.TargetDate); err != nil {
		return ErrInconsistent
	}
	if g.Version < 1 {
		return ErrInconsistent
	}
	if g.CreatedAt.IsZero() {
		return ErrInconsistent
	}
	return nil
}

// validateStoredMoney exige un importe no negativo en la divisa del contrato.
func validateStoredMoney(value money.Money) error {
	if value.Amount() < 0 || value.Amount() > MaxAmountCents {
		return ErrAmountOutOfRange
	}
	if value.Currency() != CurrencyMXN {
		return ErrCurrencyInvalid
	}
	return nil
}

// New construye una meta nueva en estado `active`, versión 1 y con `savedAmount` igual al
// ahorro de apertura (E039). El asiento de apertura queda así separado de las aportaciones
// periódicas, que sólo podrán modificar `savedAmount` en endpoints posteriores.
func New(
	userID, goalID, name string,
	targetCents, openingCents int64,
	targetDate, now time.Time,
) (Goal, error) {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(goalID) == "" {
		return Goal{}, ErrInconsistent
	}
	cleanName, err := NormalizeName(name)
	if err != nil {
		return Goal{}, err
	}
	if err := ValidateTargetAmountCents(targetCents); err != nil {
		return Goal{}, err
	}
	if err := ValidateOpeningCents(openingCents, targetCents); err != nil {
		return Goal{}, err
	}
	if err := ValidateTargetDate(targetDate); err != nil {
		return Goal{}, err
	}

	target, err := money.New(targetCents, CurrencyMXN)
	if err != nil {
		return Goal{}, ErrCurrencyInvalid
	}
	opening, err := money.New(openingCents, CurrencyMXN)
	if err != nil {
		return Goal{}, ErrCurrencyInvalid
	}

	now = now.UTC()
	goal := Goal{
		ID:           strings.TrimSpace(goalID),
		UserID:       strings.TrimSpace(userID),
		Name:         cleanName,
		TargetAmount: target,
		SavedAmount:  opening,
		OpeningSaved: opening,
		TargetDate:   targetDate.UTC(),
		Status:       StatusActive,
		Version:      1,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := goal.Validate(); err != nil {
		return Goal{}, err
	}
	return goal, nil
}

// Patch es una modificación parcial de una meta (E041). Los punteros distinguen «campo ausente»
// de «campo presente con valor»: un PATCH no enviado no se toca, y no existe forma de vaciar un
// campo obligatorio.
//
// `openingSaved` y `savedAmount` NO forman parte del Patch: el primero es historia del alta (A7)
// y el segundo sólo cambia con aportaciones confirmadas, que son otro endpoint. Este contrato
// tampoco permite cambiar `currency`: el enum publicado es `[MXN]`.
type Patch struct {
	Name        *string
	TargetCents *int64
	TargetDate  *time.Time
	Status      *Status
}

// Empty indica que la modificación no contiene ningún campo aplicable.
func (p Patch) Empty() bool {
	return p.Name == nil && p.TargetCents == nil && p.TargetDate == nil && p.Status == nil
}

// Apply valida la modificación contra la meta vigente y devuelve el agregado resultante.
//
// Reglas (documentadas porque el contrato no las enumera campo a campo):
//   - `name` se normaliza (recorte de espacios, 1..120 caracteres).
//   - `targetAmount` debe ser positivo y NO puede quedar por debajo del ahorro ya acumulado: la
//     persistencia vuelve a imponerlo con una condición atómica para que una aportación
//     concurrente no pueda saltárselo.
//   - `status` sólo admite el enum y transiciones coherentes (`Status.CanTransitionTo`).
//   - `version` se incrementa siempre en 1 y `updatedAt` pasa a la hora del servidor.
func (p Patch) Apply(current Goal, now time.Time) (Goal, error) {
	if p.Empty() {
		return Goal{}, ErrEmptyPatch
	}
	if err := current.Validate(); err != nil {
		return Goal{}, ErrInconsistent
	}

	actualizado := current

	if p.Name != nil {
		clean, err := NormalizeName(*p.Name)
		if err != nil {
			return Goal{}, err
		}
		actualizado.Name = clean
	}

	if p.TargetCents != nil {
		if err := ValidateTargetAmountCents(*p.TargetCents); err != nil {
			return Goal{}, err
		}
		if *p.TargetCents < current.SavedAmount.Amount() {
			return Goal{}, ErrTargetBelowSaved
		}
		target, err := money.New(*p.TargetCents, current.TargetAmount.Currency())
		if err != nil {
			return Goal{}, ErrCurrencyInvalid
		}
		actualizado.TargetAmount = target
	}

	if p.TargetDate != nil {
		if err := ValidateTargetDate(*p.TargetDate); err != nil {
			return Goal{}, err
		}
		actualizado.TargetDate = p.TargetDate.UTC()
	}

	if p.Status != nil {
		next := *p.Status
		if _, err := ParseStatus(next.String()); err != nil {
			return Goal{}, ErrStatusInvalid
		}
		if !current.Status.CanTransitionTo(next) {
			return Goal{}, ErrTransitionInvalid
		}
		actualizado.Status = next
	}

	actualizado.Version = current.Version + 1
	actualizado.UpdatedAt = now.UTC()
	if err := actualizado.Validate(); err != nil {
		return Goal{}, ErrInconsistent
	}
	return actualizado, nil
}
