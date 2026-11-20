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
	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

type almacenPresupuestosHTTP struct{}

func (almacenPresupuestosHTTP) LoadSet(context.Context, string, string) (*appBudget.Set, error) {
	return &appBudget.Set{
		Period: "2026-11", Currency: "MXN", Version: 2,
		Items: []appBudget.Budget{{CategoryID: "comida", LimitAmountCents: 100000}},
	}, nil
}
func (almacenPresupuestosHTTP) ListAllSets(context.Context, string) ([]appBudget.Set, error) {
	return nil, nil
}

func TestE048HTTPListadoYETag(t *testing.T) {
	servicio := appBudget.NewService(almacenPresupuestosHTTP{}).
		WithCursorKey([]byte("0123456789abcdef0123456789abcdef")).
		WithClock(func() time.Time { return time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC) })
	h := internalhttp.NewBudgetsListHandler(servicio, e004TokensValidos(), newE004Sesiones(), nil, silencioso())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/budgets?period=2026-11", strings.NewReader(""))
	req.Header.Set("Authorization", "Bearer jwt-valido")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("ETag") != `"b2026-11-2"` {
		t.Fatalf("ETag inesperado: %q", rec.Header().Get("ETag"))
	}
	var body struct {
		Items []struct {
			CategoryID string `json:"categoryId"`
			Period     string `json:"period"`
			Limit      string `json:"limitAmount"`
			Version    int64  `json:"version"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(body.Items) != 1 || body.Items[0].CategoryID != "comida" || body.Items[0].Limit != "1000.00" || body.Items[0].Version != 2 {
		t.Fatalf("items inesperados: %+v", body.Items)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/budgets?period=2026-13", strings.NewReader(""))
	req2.Header.Set("Authorization", "Bearer jwt-valido")
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusBadRequest {
		t.Fatalf("periodo invalido: esperado 400, obtenido %d", rec2.Code)
	}

	req3 := httptest.NewRequest(http.MethodGet, "/api/v1/budgets", strings.NewReader(""))
	rec3 := httptest.NewRecorder()
	h.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusUnauthorized {
		t.Fatalf("sin Bearer: esperado 401, obtenido %d", rec3.Code)
	}
}
