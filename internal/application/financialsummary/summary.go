// Package financialsummary implementa E026 (`GET /api/v1/financial-summary`) con la semántica
// aprobada en RFC-022 v1.2.1:
//
//	savings = movimientos type=saving INDEPENDIENTES (contributionId nulo) del periodo
//	        + aportaciones status=active con occurredAt en el periodo
//	net     = income - expenses - savings
//
// El movimiento vinculado a una aportación NO se suma por separado: la aportación es la
// autoridad contable. Una aportación revertida no cuenta como ahorro activo. No se publica
// `availableEstimated` (alternativa B de RFC-022).
//
// La persistencia agrega de forma incremental mientras pagina por rango de SK: no hay topes
// artificiales, no se truncа una cuenta válida y no se guarda el periodo completo en memoria.
package financialsummary

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

const (
	CurrencyMXN     = "MXN"
	DefaultTimezone = "America/Mexico_City"
)

var (
	ErrInvalidPeriod = errors.New("periodo invalido")
	ErrUnauthorized  = errors.New("identidad no autorizada")
	ErrUnavailable   = errors.New("resumen financiero no disponible")
	ErrInternal      = errors.New("fallo interno del resumen financiero")
	ErrInconsistent  = errors.New("registro financiero inconsistente")
	ErrOverflow      = errors.New("suma financiera desborda int64")
)

// MovementTotals es la agregación incremental de movimientos del periodo.
type MovementTotals struct {
	IncomeCents             int64
	ExpensesCents           int64
	IndependentSavingsCents int64
}

// ContributionTotals es la agregación incremental de aportaciones del periodo.
type ContributionTotals struct {
	ActiveSavingsCents int64
}

// Store es el puerto de lectura. El adaptador pagina por rango de SK y suma en el momento;
// nunca Scan y nunca devuelve el periodo completo en memoria.
type Store interface {
	LoadTimezone(ctx context.Context, userID string) (string, error)
	AggregateMovements(ctx context.Context, userID string, from, to time.Time) (MovementTotals, error)
	AggregateContributions(ctx context.Context, userID string, from, to time.Time) (ContributionTotals, error)
}

// Summary es la proyección de `FinancialSummary`.
type Summary struct {
	Period   string
	Currency string
	Income   int64
	Expenses int64
	Savings  int64
	Net      int64
	AsOf     time.Time
}

// WireSummary es el DTO del contrato.
type WireSummary struct {
	Period   string `json:"period"`
	Currency string `json:"currency"`
	Income   string `json:"income"`
	Expenses string `json:"expenses"`
	Savings  string `json:"savings"`
	Net      string `json:"net"`
	AsOf     string `json:"asOf"`
}

// Wire proyecta los centavos al contrato.
func (s Summary) Wire() WireSummary {
	return WireSummary{
		Period:   s.Period,
		Currency: s.Currency,
		Income:   httputil.FormatCentsToDecimal(s.Income),
		Expenses: httputil.FormatCentsToDecimal(s.Expenses),
		Savings:  httputil.FormatCentsToDecimal(s.Savings),
		Net:      httputil.FormatCentsToDecimal(s.Net),
		AsOf:     s.AsOf.UTC().Truncate(time.Second).Format(time.RFC3339),
	}
}

// Service calcula el resumen.
type Service struct {
	store Store
	now   func() time.Time
}

// NewService construye el servicio. Sin almacén falla cerrado.
func NewService(store Store) *Service {
	return &Service{store: store, now: func() time.Time { return time.Now().UTC() }}
}

// WithClock inyecta un reloj determinista.
func (s *Service) WithClock(now func() time.Time) *Service {
	if now == nil {
		return s
	}
	copia := *s
	copia.now = now
	return &copia
}

// Summary calcula E026 para `period` (`YYYY-MM`). Vacío = mes actual en la zona del titular.
func (s *Service) Summary(ctx context.Context, userID, period string) (*Summary, error) {
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
	movimientos, err := s.store.AggregateMovements(ctx, titular, desde, hasta)
	if err != nil {
		return nil, normalize(err)
	}
	aportaciones, err := s.store.AggregateContributions(ctx, titular, desde, hasta)
	if err != nil {
		return nil, normalize(err)
	}
	savings, err := safeAdd(movimientos.IndependentSavingsCents, aportaciones.ActiveSavingsCents)
	if err != nil {
		return nil, err
	}
	neto, err := safeSub(movimientos.IncomeCents, movimientos.ExpensesCents)
	if err != nil {
		return nil, err
	}
	neto, err = safeSub(neto, savings)
	if err != nil {
		return nil, err
	}
	return &Summary{
		Period:   periodo,
		Currency: CurrencyMXN,
		Income:   movimientos.IncomeCents,
		Expenses: movimientos.ExpensesCents,
		Savings:  savings,
		Net:      neto,
		AsOf:     ahora,
	}, nil
}

// loadLocation diferencia: preferencia inexistente o zona IANA inválida -> fallback aprobado;
// error real de DynamoDB -> 503. Nunca se devuelve 200 con un periodo posiblemente incorrecto
// después de un fallo de persistencia.
func (s *Service) loadLocation(ctx context.Context, userID string) (*time.Location, error) {
	nombre, err := s.store.LoadTimezone(ctx, userID)
	if err != nil {
		return nil, normalize(err)
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

// periodBounds interpreta `YYYY-MM` y devuelve [desde, hasta) en UTC y el periodo canónico.
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

func safeAdd(a, b int64) (int64, error) {
	if b > 0 && a > math.MaxInt64-b {
		return 0, ErrOverflow
	}
	if b < 0 && a < math.MinInt64-b {
		return 0, ErrOverflow
	}
	return a + b, nil
}

func safeSub(a, b int64) (int64, error) {
	if b < 0 && a > math.MaxInt64+b {
		return 0, ErrOverflow
	}
	if b > 0 && a < math.MinInt64+b {
		return 0, ErrOverflow
	}
	return a - b, nil
}

func normalize(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrInvalidPeriod), errors.Is(err, ErrUnauthorized),
		errors.Is(err, ErrUnavailable), errors.Is(err, ErrInternal),
		errors.Is(err, ErrInconsistent), errors.Is(err, ErrOverflow):
		return err
	default:
		return ErrUnavailable
	}
}
