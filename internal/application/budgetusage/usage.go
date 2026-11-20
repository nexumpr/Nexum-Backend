// Package budgetusage implementa E050 (`GET /api/v1/budgets/usage`). El uso se calcula desde
// movimientos `expense` reales del periodo; no cuenta reglas recurrentes futuras ni duplica
// pagos de E037.
package budgetusage

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	appBudget "github.com/nexumpr/Nexum-Backend/internal/application/budgetset"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

const DefaultTimezone = "America/Mexico_City"

var (
	ErrInvalidPeriod = errors.New("periodo invalido")
	ErrUnauthorized  = errors.New("uso presupuestal no autorizado")
	ErrUnavailable   = errors.New("uso presupuestal no disponible")
	ErrInconsistent  = errors.New("registro financiero inconsistente")
	ErrOverflow      = errors.New("suma financiera desborda int64")
)

// Store lee el conjunto del periodo, la zona y los egresos reales.
type Store interface {
	LoadTimezone(ctx context.Context, userID string) (string, error)
	LoadSet(ctx context.Context, userID, period string) (*appBudget.Set, error)
	AggregateExpenses(ctx context.Context, userID string, from, to time.Time) (map[string]int64, error)
}

// Usage es una fila de `BudgetUsage`.
type Usage struct {
	CategoryID string
	Period     string
	UsedCents  int64
	LimitCents int64
	Currency   string
}

// WireUsage es el DTO del contrato.
type WireUsage struct {
	CategoryID  string `json:"categoryId"`
	Period      string `json:"period"`
	UsedAmount  string `json:"usedAmount"`
	LimitAmount string `json:"limitAmount"`
	Currency    string `json:"currency"`
}

// Service calcula el uso.
type Service struct {
	store Store
	now   func() time.Time
}

// NewService construye el servicio.
func NewService(store Store) *Service {
	return &Service{store: store, now: func() time.Time { return time.Now().UTC() }}
}

// WithClock inyecta reloj determinista.
func (s *Service) WithClock(now func() time.Time) *Service {
	if now == nil {
		return s
	}
	copia := *s
	copia.now = now
	return &copia
}

// Usage calcula E050.
func (s *Service) Usage(ctx context.Context, userID, period string) ([]Usage, error) {
	if s == nil || s.store == nil {
		return nil, ErrUnavailable
	}
	titular := strings.TrimSpace(userID)
	if titular == "" {
		return nil, ErrUnauthorized
	}
	ahora := s.now().UTC()
	zona, err := s.loadLocation(ctx, titular)
	if err != nil {
		return nil, err
	}
	desde, hasta, periodo, err := periodBounds(period, ahora, zona)
	if err != nil {
		return nil, err
	}
	set, err := s.store.LoadSet(ctx, titular, periodo)
	if err != nil {
		return nil, err
	}
	if set == nil {
		return []Usage{}, nil
	}
	gastos, err := s.store.AggregateExpenses(ctx, titular, desde, hasta)
	if err != nil {
		return nil, err
	}
	items := append([]appBudget.Budget(nil), set.Items...)
	sort.Slice(items, func(i, j int) bool { return items[i].CategoryID < items[j].CategoryID })
	salida := make([]Usage, 0, len(items))
	for _, b := range items {
		salida = append(salida, Usage{
			CategoryID: b.CategoryID,
			Period:     periodo,
			UsedCents:  gastos[b.CategoryID],
			LimitCents: b.LimitAmountCents,
			Currency:   set.Currency,
		})
	}
	return salida, nil
}

func (s *Service) loadLocation(ctx context.Context, userID string) (*time.Location, error) {
	nombre, err := s.store.LoadTimezone(ctx, userID)
	if err != nil {
		return nil, err
	}
	nombre = strings.TrimSpace(nombre)
	if nombre == "" {
		return time.LoadLocation(DefaultTimezone)
	}
	loc, err := time.LoadLocation(nombre)
	if err != nil {
		return time.LoadLocation(DefaultTimezone)
	}
	return loc, nil
}

func periodBounds(period string, now time.Time, loc *time.Location) (time.Time, time.Time, string, error) {
	limpio := strings.TrimSpace(period)
	var anio int
	var mes time.Month
	if limpio == "" {
		local := now.In(loc)
		anio, mes = local.Year(), local.Month()
	} else {
		if len(limpio) != 7 || limpio[4] != '-' {
			return time.Time{}, time.Time{}, "", ErrInvalidPeriod
		}
		if _, err := fmt.Sscanf(limpio, "%4d-%2d", &anio, &mes); err != nil || mes < 1 || mes > 12 {
			return time.Time{}, time.Time{}, "", ErrInvalidPeriod
		}
	}
	desdeLocal := time.Date(anio, mes, 1, 0, 0, 0, 0, loc)
	hastaLocal := desdeLocal.AddDate(0, 1, 0)
	return desdeLocal.UTC(), hastaLocal.UTC(), fmt.Sprintf("%04d-%02d", anio, int(mes)), nil
}

// WireItems proyecta las filas al contrato.
func WireItems(items []Usage) []WireUsage {
	salida := make([]WireUsage, 0, len(items))
	for _, u := range items {
		salida = append(salida, WireUsage{
			CategoryID:  u.CategoryID,
			Period:      u.Period,
			UsedAmount:  httputil.FormatCentsToDecimal(u.UsedCents),
			LimitAmount: httputil.FormatCentsToDecimal(u.LimitCents),
			Currency:    u.Currency,
		})
	}
	return salida
}

// SumarCentavos comprueba overflow; se usa en el adaptador.
func SumarCentavos(a, b int64) (int64, bool) {
	if b > 0 && a > math.MaxInt64-b {
		return 0, false
	}
	if b < 0 && a < math.MinInt64-b {
		return 0, false
	}
	return a + b, true
}
