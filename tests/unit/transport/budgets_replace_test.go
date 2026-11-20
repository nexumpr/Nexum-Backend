package transport_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	appBudget "github.com/nexumpr/Nexum-Backend/internal/application/budgetset"
	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

type almacenPresupuestosPutHTTP struct {
	mu   sync.Mutex
	sets map[string]*appBudget.Set
}

func (a *almacenPresupuestosPutHTTP) LoadSet(_ context.Context, userID, period string) (*appBudget.Set, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if set, ok := a.sets[userID+"|"+period]; ok {
		copia := *set
		return &copia, nil
	}
	return nil, nil
}
func (a *almacenPresupuestosPutHTTP) ListAllSets(context.Context, string) ([]appBudget.Set, error) {
	return nil, nil
}
func (a *almacenPresupuestosPutHTTP) ReplaceSet(_ context.Context, userID string, set *appBudget.Set, expected int64) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	clave := userID + "|" + set.Period
	if actual, ok := a.sets[clave]; ok {
		if actual.Version != expected {
			return appBudget.ErrVersionConflict
		}
	} else if expected != 0 {
		return appBudget.ErrVersionConflict
	}
	copia := *set
	a.sets[clave] = &copia
	return nil
}

func TestE049HTTPPut(t *testing.T) {
	almacen := &almacenPresupuestosPutHTTP{sets: map[string]*appBudget.Set{}}
	servicio := appBudget.NewService(almacen).WithClock(func() time.Time { return time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC) })
	h := internalhttp.NewBudgetsReplaceHandler(servicio, e004TokensValidos(), newE004Sesiones(), nil, silencioso())

	cuerpo := `{"period":"2026-11","currency":"MXN","budgets":[{"categoryId":"comida","limitAmount":"1000.00"}]}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/budgets", strings.NewReader(cuerpo))
	req.Header.Set("Authorization", "Bearer jwt-valido")
	req.Header.Set("If-Match", `"b2026-11-0"`)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Header().Get("ETag") != `"b2026-11-1"` {
		t.Fatalf("esperado 200/ETag, obtenido %d/%q: %s", rec.Code, rec.Header().Get("ETag"), rec.Body.String())
	}
	req2 := httptest.NewRequest(http.MethodPut, "/api/v1/budgets", strings.NewReader(cuerpo))
	req2.Header.Set("Authorization", "Bearer jwt-valido")
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusPreconditionRequired {
		t.Fatalf("sin If-Match: esperado 428, obtenido %d", rec2.Code)
	}
	req3 := httptest.NewRequest(http.MethodPut, "/api/v1/budgets", strings.NewReader(cuerpo))
	req3.Header.Set("Authorization", "Bearer jwt-valido")
	req3.Header.Set("If-Match", `"b2026-11-0"`)
	rec3 := httptest.NewRecorder()
	h.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusPreconditionFailed {
		t.Fatalf("etag obsoleto: esperado 412, obtenido %d", rec3.Code)
	}
	req4 := httptest.NewRequest(http.MethodPut, "/api/v1/budgets", strings.NewReader(`{"period":"2026-11","currency":"MXN","budgets":[{"categoryId":"comida","limitAmount":"-1.00"}]}`))
	req4.Header.Set("Authorization", "Bearer jwt-valido")
	req4.Header.Set("If-Match", `"b2026-11-1"`)
	rec4 := httptest.NewRecorder()
	h.ServeHTTP(rec4, req4)
	if rec4.Code != http.StatusUnprocessableEntity {
		t.Fatalf("negativo: esperado 422, obtenido %d", rec4.Code)
	}
}
