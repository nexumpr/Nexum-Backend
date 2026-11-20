package transport_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appBudget "github.com/nexumpr/Nexum-Backend/internal/application/budgetset"
	appUsage "github.com/nexumpr/Nexum-Backend/internal/application/budgetusage"
	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

type almacenUsoHTTP struct{}

func (almacenUsoHTTP) LoadTimezone(context.Context, string) (string, error) { return "UTC", nil }
func (almacenUsoHTTP) LoadSet(context.Context, string, string) (*appBudget.Set, error) {
	return &appBudget.Set{Period: "2026-11", Currency: "MXN", Version: 1, Items: []appBudget.Budget{{CategoryID: "comida", LimitAmountCents: 100000}}}, nil
}
func (almacenUsoHTTP) AggregateExpenses(context.Context, string, time.Time, time.Time) (map[string]int64, error) {
	return map[string]int64{"comida": 25000}, nil
}

func TestE050HTTPUso(t *testing.T) {
	servicio := appUsage.NewService(almacenUsoHTTP{}).WithClock(func() time.Time {
		return time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	})
	h := internalhttp.NewBudgetUsageHandler(servicio, e004TokensValidos(), newE004Sesiones(), nil, silencioso())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/budgets/usage?period=2026-11", strings.NewReader(""))
	req.Header.Set("Authorization", "Bearer jwt-valido")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Items []struct {
			CategoryID string `json:"categoryId"`
			Used       string `json:"usedAmount"`
			Limit      string `json:"limitAmount"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(body.Items) != 1 || body.Items[0].Used != "250.00" || body.Items[0].Limit != "1000.00" {
		t.Fatalf("items inesperados: %+v", body.Items)
	}
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/budgets/usage?period=2026-13", strings.NewReader(""))
	req2.Header.Set("Authorization", "Bearer jwt-valido")
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusBadRequest {
		t.Fatalf("periodo invalido: esperado 400, obtenido %d", rec2.Code)
	}
}
