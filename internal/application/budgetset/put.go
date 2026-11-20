package budgetset

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

var (
	ErrPreconditionRequired = errors.New("If-Match requerido")
	ErrPreconditionInvalid  = errors.New("If-Match invalido")
	ErrVersionConflict      = errors.New("version del conjunto desactualizada")
)

// Violation es una regla de negocio del cuerpo (HTTP 422).
type Violation struct {
	Field  string
	Reason string
}

func (v *Violation) Error() string {
	if v == nil {
		return "presupuestos: regla de negocio"
	}
	return fmt.Sprintf("presupuestos: campo %q invalido (%s)", v.Field, v.Reason)
}

func violacion(campo, motivo string) error { return &Violation{Field: campo, Reason: motivo} }

// BudgetInput es una fila del cuerpo `BudgetReplace`.
type BudgetInput struct {
	CategoryID  string `json:"categoryId"`
	LimitAmount string `json:"limitAmount"`
}

// ReplaceCommand es la entrada de E049.
type ReplaceCommand struct {
	UserID   string
	IfMatch  string
	Period   string
	Currency string
	Budgets  []BudgetInput
}

// WriteStore es el puerto de escritura. Se separa de `Store` para no obligar a los dobles de
// lectura de E048 a implementar la escritura.
type WriteStore interface {
	ReplaceSet(ctx context.Context, userID string, set *Set, expectedVersion int64) error
}

// ParseBudgetETag valida `"b<YYYY-MM>-<version>"`.
func ParseBudgetETag(valor string) (string, int64, error) {
	limpio := strings.TrimSpace(valor)
	if limpio == "" {
		return "", 0, ErrPreconditionRequired
	}
	if strings.HasPrefix(limpio, "W/") {
		return "", 0, ErrPreconditionInvalid
	}
	if len(limpio) < 6 || limpio[0] != '"' || limpio[len(limpio)-1] != '"' {
		return "", 0, ErrPreconditionInvalid
	}
	contenido := limpio[1 : len(limpio)-1]
	if !strings.HasPrefix(contenido, "b") {
		return "", 0, ErrPreconditionInvalid
	}
	resto := contenido[1:]
	if len(resto) < 9 || resto[4] != '-' || resto[7] != '-' {
		return "", 0, ErrPreconditionInvalid
	}
	periodo := resto[:7]
	var version int64
	if _, err := fmt.Sscanf(resto[8:], "%d", &version); err != nil || version < 0 {
		return "", 0, ErrPreconditionInvalid
	}
	if _, err := time.Parse("2006-01", periodo); err != nil {
		return "", 0, ErrPreconditionInvalid
	}
	return periodo, version, nil
}

// Replace ejecuta E049.
func (s *Service) Replace(ctx context.Context, cmd ReplaceCommand) (*Page, error) {
	if s == nil || s.store == nil {
		return nil, ErrUnavailable
	}
	escritura, ok := s.store.(WriteStore)
	if !ok {
		return nil, ErrUnavailable
	}
	titular := strings.TrimSpace(cmd.UserID)
	if titular == "" {
		return nil, ErrUnauthorized
	}
	periodo := strings.TrimSpace(cmd.Period)
	if len(periodo) != 7 || periodo[4] != '-' {
		return nil, fmt.Errorf("%w: period", ErrInvalidRequest)
	}
	if _, err := time.Parse("2006-01", periodo); err != nil {
		return nil, fmt.Errorf("%w: period", ErrInvalidRequest)
	}
	divisa := strings.ToUpper(strings.TrimSpace(cmd.Currency))
	if divisa != CurrencyMXN {
		return nil, violacion("currency", "unsupported")
	}
	etagPeriod, etagVersion, err := ParseBudgetETag(cmd.IfMatch)
	if err != nil {
		return nil, err
	}
	if etagPeriod != periodo {
		return nil, ErrVersionConflict
	}
	items, err := normalizeBudgets(cmd.Budgets)
	if err != nil {
		return nil, err
	}
	actual, err := s.store.LoadSet(ctx, titular, periodo)
	if err != nil {
		return nil, err
	}
	versionActual := int64(0)
	if actual != nil {
		versionActual = actual.Version
	}
	if etagVersion != versionActual {
		return nil, ErrVersionConflict
	}
	nuevaVersion := versionActual + 1
	set := &Set{Period: periodo, Currency: divisa, Version: nuevaVersion, Items: items, UpdatedAt: s.now().UTC()}
	if err := escritura.ReplaceSet(ctx, titular, set, etagVersion); err != nil {
		return nil, err
	}
	page := &Page{ETag: FormatETag(periodo, nuevaVersion)}
	for _, b := range items {
		page.Items = append(page.Items, Item{Period: periodo, Currency: divisa, Version: nuevaVersion, CategoryID: b.CategoryID, LimitAmountCents: b.LimitAmountCents})
	}
	return page, nil
}

func normalizeBudgets(crudos []BudgetInput) ([]Budget, error) {
	if len(crudos) > MaxCategories {
		return nil, violacion("budgets", "too_many")
	}
	vistos := make(map[string]struct{}, len(crudos))
	salida := make([]Budget, 0, len(crudos))
	for _, c := range crudos {
		id := strings.TrimSpace(c.CategoryID)
		if id == "" {
			return nil, violacion("budgets.categoryId", "required")
		}
		if len(id) > 64 || strings.ContainsAny(id, "\x00\r\n\t") {
			return nil, violacion("budgets.categoryId", "invalid_format")
		}
		if _, existe := vistos[id]; existe {
			return nil, violacion("budgets.categoryId", "duplicate")
		}
		vistos[id] = struct{}{}
		centavos, err := parseBudgetAmount(c.LimitAmount)
		if err != nil {
			return nil, err
		}
		if centavos < 0 {
			return nil, violacion("budgets.limitAmount", "out_of_range")
		}
		salida = append(salida, Budget{CategoryID: id, LimitAmountCents: centavos})
	}
	sort.Slice(salida, func(i, j int) bool { return salida[i].CategoryID < salida[j].CategoryID })
	return salida, nil
}

func parseBudgetAmount(raw string) (int64, error) {
	limpio := strings.TrimSpace(raw)
	if limpio == "" {
		return 0, violacion("budgets.limitAmount", "required")
	}
	if strings.HasPrefix(limpio, "-") {
		return 0, violacion("budgets.limitAmount", "out_of_range")
	}
	if !patronImporte(limpio) {
		return 0, fmt.Errorf("%w: limitAmount", ErrInvalidRequest)
	}
	centavos, err := httputil.ParseDecimalToCents(limpio)
	if err != nil {
		return 0, fmt.Errorf("%w: limitAmount", ErrInvalidRequest)
	}
	return centavos, nil
}

func patronImporte(valor string) bool {
	punto := strings.IndexByte(valor, '.')
	if punto < 0 {
		return false
	}
	entero, decimales := valor[:punto], valor[punto+1:]
	if len(decimales) != 2 || len(entero) == 0 || len(entero) > 12 {
		return false
	}
	for i := 0; i < len(decimales); i++ {
		if decimales[i] < '0' || decimales[i] > '9' {
			return false
		}
	}
	if entero == "0" {
		return true
	}
	if entero[0] < '1' || entero[0] > '9' {
		return false
	}
	for i := 1; i < len(entero); i++ {
		if entero[i] < '0' || entero[i] > '9' {
			return false
		}
	}
	return true
}
