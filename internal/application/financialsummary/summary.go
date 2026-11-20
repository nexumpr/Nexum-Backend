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
package financialsummary

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

const (
	// CurrencyMXN es la única divisa del contrato.
	CurrencyMXN = "MXN"

	// DefaultTimezone se aplica si el titular no tiene una zona IANA válida.
	DefaultTimezone = "America/Mexico_City"

	// MaxMovements y MaxContributions acotan el periodo para no construir totales parciales.
	// Al superarlos se falla cerrado (503); no se recorta la suma.
	MaxMovements     = 10000
	MaxContributions = 5000
)

var (
	ErrInvalidPeriod = errors.New("periodo invalido")
	ErrUnauthorized  = errors.New("identidad no autorizada")
	ErrUnavailable   = errors.New("resumen financiero no disponible")
	ErrInternal      = errors.New("fallo interno del resumen financiero")
	ErrTooManyItems  = errors.New("volumen del periodo excede el limite de calculo")
)

// MovementRow es la proyección mínima de un movimiento.
type MovementRow struct {
	Type           string
	AmountCents    int64
	ContributionID string
}

// ContributionRow es la proyección mínima de una aportación.
type ContributionRow struct {
	Status      string
	AmountCents int64
}

// Store es el puerto de lectura. El adaptador usa Query por rango de SK; nunca Scan.
type Store interface {
	LoadTimezone(ctx context.Context, userID string) (string, error)
	ListMovements(ctx context.Context, userID string, from, to time.Time) ([]MovementRow, error)
	ListContributions(ctx context.Context, userID string, from, to time.Time) ([]ContributionRow, error)
}

// Summary es la proyección exacta de `FinancialSummary`.
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
	zona := s.loadLocation(ctx, titular)
	desde, hasta, periodo, err := periodBounds(period, ahora, zona)
	if err != nil {
		return nil, err
	}
	movimientos, err := s.store.ListMovements(ctx, titular, desde, hasta)
	if err != nil {
		return nil, normalize(err)
	}
	if len(movimientos) > MaxMovements {
		return nil, ErrTooManyItems
	}
	aportaciones, err := s.store.ListContributions(ctx, titular, desde, hasta)
	if err != nil {
		return nil, normalize(err)
	}
	if len(aportaciones) > MaxContributions {
		return nil, ErrTooManyItems
	}

	var income, expenses, independentSavings, activeContributions int64
	for _, m := range movimientos {
		switch strings.TrimSpace(m.Type) {
		case "income":
			income += m.AmountCents
		case "expense":
			expenses += m.AmountCents
		case "saving":
			// Un movimiento vinculado a una aportación no se suma: la aportación activa es
			// la autoridad. Un movimiento independiente (E028 o E037) sí cuenta una vez.
			if strings.TrimSpace(m.ContributionID) == "" {
				independentSavings += m.AmountCents
			}
		}
	}
	for _, c := range aportaciones {
		if c.Status == "active" {
			activeContributions += c.AmountCents
		}
	}
	savings := independentSavings + activeContributions
	return &Summary{
		Period:   periodo,
		Currency: CurrencyMXN,
		Income:   income,
		Expenses: expenses,
		Savings:  savings,
		Net:      income - expenses - savings,
		AsOf:     ahora,
	}, nil
}

func (s *Service) loadLocation(ctx context.Context, userID string) *time.Location {
	nombre, err := s.store.LoadTimezone(ctx, userID)
	if err != nil || strings.TrimSpace(nombre) == "" {
		nombre = DefaultTimezone
	}
	loc, err := time.LoadLocation(strings.TrimSpace(nombre))
	if err != nil {
		loc, _ = time.LoadLocation(DefaultTimezone)
	}
	if loc == nil {
		loc = time.UTC
	}
	return loc
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

// periodBounds interpreta `YYYY-MM` y devuelve [desde, hasta) en UTC y el periodo canónico.
func normalize(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrInvalidPeriod), errors.Is(err, ErrUnauthorized),
		errors.Is(err, ErrUnavailable), errors.Is(err, ErrInternal), errors.Is(err, ErrTooManyItems):
		return err
	default:
		return ErrUnavailable
	}
}
