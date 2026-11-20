package transport_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appDashboard "github.com/nexumpr/Nexum-Backend/internal/application/dashboard"
	appSummary "github.com/nexumpr/Nexum-Backend/internal/application/financialsummary"
	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

type resumenDashboardHTTP struct{}

func (resumenDashboardHTTP) LoadTimezone(context.Context, string) (string, error) { return "UTC", nil }
func (resumenDashboardHTTP) AggregateMovements(context.Context, string, time.Time, time.Time) (appSummary.MovementTotals, error) {
	return appSummary.MovementTotals{IncomeCents: 1000}, nil
}
func (resumenDashboardHTTP) AggregateContributions(context.Context, string, time.Time, time.Time) (appSummary.ContributionTotals, error) {
	return appSummary.ContributionTotals{}, nil
}

type dashboardStoreHTTP struct{}

func (dashboardStoreHTTP) ListActiveGoals(context.Context, string, int) ([]appDashboard.GoalRow, error) {
	return nil, nil
}
func (dashboardStoreHTTP) ListLatestMovements(context.Context, string, int) ([]appMovements.Movement, error) {
	return nil, nil
}

func TestE051HTTPDashboard(t *testing.T) {
	summary := appSummary.NewService(resumenDashboardHTTP{}).WithClock(func() time.Time {
		return time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	})
	servicio := appDashboard.NewService(summary, dashboardStoreHTTP{})
	h := internalhttp.NewDashboardHandler(servicio, e004TokensValidos(), newE004Sesiones(), nil, silencioso())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard?period=2026-11", strings.NewReader(""))
	req.Header.Set("Authorization", "Bearer jwt-valido")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Period  string `json:"period"`
		Summary struct {
			Net string `json:"net"`
		} `json:"summary"`
		Goals           []any `json:"goals"`
		LatestMovements []any `json:"latestMovements"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("json: %v", err)
	}
	if body.Period != "2026-11" || body.Summary.Net != "10.00" || body.Goals == nil || body.LatestMovements == nil {
		t.Fatalf("dashboard inesperado: %s", rec.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard?period=2026-13", strings.NewReader(""))
	req2.Header.Set("Authorization", "Bearer jwt-valido")
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusBadRequest {
		t.Fatalf("periodo invalido: esperado 400, obtenido %d", rec2.Code)
	}
}
