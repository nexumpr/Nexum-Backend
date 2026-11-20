package transport_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appSummary "github.com/nexumpr/Nexum-Backend/internal/application/financialsummary"
	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

type resumenStoreHTTP struct {
	err error
}

func (s resumenStoreHTTP) LoadTimezone(context.Context, string) (string, error) { return "UTC", nil }

func (s resumenStoreHTTP) ListMovements(context.Context, string, time.Time, time.Time) ([]appSummary.MovementRow, error) {
	if s.err != nil {
		return nil, s.err
	}
	return []appSummary.MovementRow{{Type: "income", AmountCents: 1000}}, nil
}

func (s resumenStoreHTTP) ListContributions(context.Context, string, time.Time, time.Time) ([]appSummary.ContributionRow, error) {
	return nil, nil
}

func TestE026HTTPResumen(t *testing.T) {
	servicio := appSummary.NewService(resumenStoreHTTP{}).
		WithClock(func() time.Time { return time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC) })
	h := internalhttp.NewFinancialSummaryHandler(servicio, e004TokensValidos(), newE004Sesiones(), nil, silencioso())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/financial-summary?period=2026-11", strings.NewReader(""))
	req.Header.Set("Authorization", "Bearer jwt-valido")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Period  string `json:"period"`
		Income  string `json:"income"`
		Net     string `json:"net"`
		Savings string `json:"savings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("json: %v", err)
	}
	if body.Period != "2026-11" || body.Income != "10.00" || body.Net != "10.00" || body.Savings != "0.00" {
		t.Fatalf("respuesta inesperada: %+v", body)
	}
}

func TestE026HTTPErrores(t *testing.T) {
	servicio := appSummary.NewService(resumenStoreHTTP{}).
		WithClock(func() time.Time { return time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC) })
	h := internalhttp.NewFinancialSummaryHandler(servicio, e004TokensValidos(), newE004Sesiones(), nil, silencioso())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/financial-summary?period=2026-13", strings.NewReader(""))
	req.Header.Set("Authorization", "Bearer jwt-valido")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("periodo invalido: esperado 400, obtenido %d", rec.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/financial-summary", strings.NewReader(""))
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusUnauthorized {
		t.Fatalf("sin Bearer: esperado 401, obtenido %d", rec2.Code)
	}
}
