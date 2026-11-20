// Package dashboard implementa E051 (`GET /api/v1/dashboard`) con datos reales de DynamoDB.
// No usa caché Redis: el resumen se calcula con E026 y las metas/movimientos se leen por Query.
package dashboard

import (
	"context"
	"errors"
	"strings"
	"time"

	appSummary "github.com/nexumpr/Nexum-Backend/internal/application/financialsummary"
	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

const (
	// MaxGoals acota la lista del dashboard; el contrato no fija cardinalidad.
	MaxGoals = 100
	// LatestMovements es el número de movimientos recientes del contrato.
	LatestMovements = 10
)

var (
	ErrUnauthorized = errors.New("dashboard no autorizado")
	ErrUnavailable  = errors.New("dashboard no disponible")
	ErrInvalid      = errors.New("dashboard invalido")
)

// GoalRow es la proyección mínima de una meta.
type GoalRow struct {
	ID           string
	Name         string
	TargetCents  int64
	SavedCents   int64
	OpeningCents int64
	Currency     string
	TargetDate   string
	Status       string
	Version      int64
}

// GoalWire es el objeto `Goal` del contrato.
type GoalWire struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	TargetAmount string `json:"targetAmount"`
	SavedAmount  string `json:"savedAmount"`
	Currency     string `json:"currency"`
	TargetDate   string `json:"targetDate"`
	Status       string `json:"status"`
	Version      int64  `json:"version"`
	OpeningSaved string `json:"openingSaved"`
}

// Store lee metas activas y movimientos recientes. Nunca Scan.
type Store interface {
	ListActiveGoals(ctx context.Context, userID string, limit int) ([]GoalRow, error)
	ListLatestMovements(ctx context.Context, userID string, limit int) ([]appMovements.Movement, error)
}

// Service compone E051.
type Service struct {
	summary *appSummary.Service
	store   Store
}

// NewService construye el servicio.
func NewService(summary *appSummary.Service, store Store) *Service {
	return &Service{summary: summary, store: store}
}

// DashboardWire es el DTO `Dashboard`.
type DashboardWire struct {
	Period          string                          `json:"period"`
	Summary         appSummary.WireSummary          `json:"summary"`
	Goals           []GoalWire                      `json:"goals"`
	LatestMovements []appMovements.MovementResponse `json:"latestMovements"`
	AsOf            string                          `json:"asOf"`
}

// Get calcula el dashboard. Sin caché y sin datos inventados.
func (s *Service) Get(ctx context.Context, userID, period string) (*DashboardWire, error) {
	if s == nil || s.summary == nil || s.store == nil {
		return nil, ErrUnavailable
	}
	titular := strings.TrimSpace(userID)
	if titular == "" {
		return nil, ErrUnauthorized
	}
	resumen, err := s.summary.Summary(ctx, titular, period)
	if err != nil {
		return nil, err
	}
	goals, err := s.store.ListActiveGoals(ctx, titular, MaxGoals)
	if err != nil {
		return nil, err
	}
	movs, err := s.store.ListLatestMovements(ctx, titular, LatestMovements)
	if err != nil {
		return nil, err
	}
	salida := &DashboardWire{
		Period:  resumen.Period,
		Summary: resumen.Wire(),
		Goals:   make([]GoalWire, 0, len(goals)),
		AsOf:    resumen.AsOf.UTC().Truncate(time.Second).Format(time.RFC3339),
	}
	for _, g := range goals {
		salida.Goals = append(salida.Goals, GoalWire{
			ID:           g.ID,
			Name:         g.Name,
			TargetAmount: httputil.FormatCentsToDecimal(g.TargetCents),
			SavedAmount:  httputil.FormatCentsToDecimal(g.SavedCents),
			Currency:     g.Currency,
			TargetDate:   g.TargetDate,
			Status:       g.Status,
			Version:      g.Version,
			OpeningSaved: httputil.FormatCentsToDecimal(g.OpeningCents),
		})
	}
	salida.LatestMovements = make([]appMovements.MovementResponse, 0, len(movs))
	for _, m := range movs {
		salida.LatestMovements = append(salida.LatestMovements, appMovements.NewMovementResponse(m))
	}
	return salida, nil
}
