package transport_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	appAnalytics "github.com/nexumpr/Nexum-Backend/internal/application/analytics"
	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

type e053HTTPSource struct {
	byUser map[string][]appMovements.Movement
}

func (f *e053HTTPSource) ListMovements(_ context.Context, q appMovements.ListQuery, startSK string, _ int) ([]appMovements.Movement, string, error) {
	if startSK != "" {
		return nil, "", nil
	}
	return append([]appMovements.Movement(nil), f.byUser[q.UserID]...), "", nil
}

type e053Limitador struct{ permitido bool }

func (l e053Limitador) Allow(context.Context, string, string) (bool, time.Duration, error) {
	return l.permitido, time.Minute, nil
}

func e053HTTPMovimientos(userID string) []appMovements.Movement {
	base := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	datos := []struct {
		categoria string
		cents     int64
	}{
		{"food", 5000}, {"food", 2000}, {"transport", 2000}, {"home", 1000},
	}
	salida := make([]appMovements.Movement, 0, len(datos)+2)
	for i, dato := range datos {
		salida = append(salida, appMovements.Movement{
			ID:          "e053-" + userID + "-" + time.Duration(i).String(),
			Type:        appMovements.TypeExpense,
			AmountCents: dato.cents,
			Currency:    "MXN",
			CategoryID:  dato.categoria,
			OccurredAt:  base.Add(time.Duration(i) * time.Hour),
		})
	}
	// Un ingreso no debe aparecer en la distribución de gasto.
	salida = append(salida, appMovements.Movement{
		ID: "e053-ingreso", Type: appMovements.TypeIncome, AmountCents: 900000,
		Currency: "MXN", CategoryID: "nomina", OccurredAt: base,
	})
	return salida
}

func e053HTTPHandler(fuente *e053HTTPSource, limitador internalhttp.LimitadorRuta) *internalhttp.AnalyticsCategoriesHandler {
	servicio := appAnalytics.NewService(fuente).WithClock(func() time.Time {
		return time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	})
	return internalhttp.NewAnalyticsCategoriesHandler(servicio, e004TokensValidos(), newE004Sesiones(), limitador, silencioso())
}

func TestE053HTTPContratoValido(t *testing.T) {
	fuente := &e053HTTPSource{byUser: map[string][]appMovements.Movement{
		e004HTTPUsuario: e053HTTPMovimientos(e004HTTPUsuario),
	}}
	h := e053HTTPHandler(fuente, e053Limitador{permitido: true})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, e004Peticion(http.MethodGet, "/api/v1/analytics/categories?period=2026-11", "jwt-valido", ""))

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("Cache-Control incorrecto: %q", got)
	}

	var cuerpo struct {
		Items []struct {
			CategoryID string `json:"categoryId"`
			Period     string `json:"period"`
			Amount     string `json:"amount"`
			Currency   string `json:"currency"`
			Percentage int    `json:"percentage"`
		} `json:"items"`
		NextCursor *string `json:"nextCursor"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("JSON invalido: %v (%s)", err, rec.Body.String())
	}
	if len(cuerpo.Items) != 3 {
		t.Fatalf("esperadas 3 categorias (sin ingresos), obtenidas %d: %s", len(cuerpo.Items), rec.Body.String())
	}
	patron := regexp.MustCompile(`^(0|[1-9][0-9]{0,11})\.[0-9]{2}$`)
	suma := 0
	for _, item := range cuerpo.Items {
		if item.CategoryID == "" || item.Period != "2026-11" || item.Currency != "MXN" {
			t.Fatalf("item fuera de contrato: %+v", item)
		}
		if !patron.MatchString(item.Amount) {
			t.Fatalf("amount no cumple el pattern OpenAPI: %q", item.Amount)
		}
		if item.Percentage < 0 || item.Percentage > 100 {
			t.Fatalf("percentage fuera de 0..100: %d", item.Percentage)
		}
		suma += item.Percentage
	}
	if suma != 100 {
		t.Fatalf("los porcentajes deben sumar 100, suman %d", suma)
	}
	if cuerpo.Items[0].CategoryID != "food" || cuerpo.Items[0].Amount != "70.00" || cuerpo.Items[0].Percentage != 70 {
		t.Fatalf("agregacion de food incorrecta: %+v", cuerpo.Items[0])
	}
	if cuerpo.NextCursor != nil {
		t.Fatalf("nextCursor debe omitirse, obtenido %q", *cuerpo.NextCursor)
	}
	if strings.Contains(rec.Body.String(), e004HTTPUsuario) {
		t.Fatal("la respuesta expone el identificador del titular")
	}
	if strings.Contains(rec.Body.String(), "nomina") {
		t.Fatal("la respuesta incluye ingresos ajenos al gasto")
	}
}

func TestE053HTTPNegativas(t *testing.T) {
	fuente := &e053HTTPSource{byUser: map[string][]appMovements.Movement{
		e004HTTPUsuario: e053HTTPMovimientos(e004HTTPUsuario),
	}}
	casos := []struct {
		nombre    string
		metodo    string
		ruta      string
		token     string
		limitador internalhttp.LimitadorRuta
		espera    int
		codigo    string
	}{
		{"periodo_invalido", http.MethodGet, "/api/v1/analytics/categories?period=2026-00", "jwt-valido", e053Limitador{permitido: true}, http.StatusBadRequest, "INVALID_REQUEST"},
		{"sin_token", http.MethodGet, "/api/v1/analytics/categories?period=2026-11", "", e053Limitador{permitido: true}, http.StatusUnauthorized, "UNAUTHORIZED"},
		{"metodo", http.MethodPost, "/api/v1/analytics/categories", "jwt-valido", e053Limitador{permitido: true}, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED"},
		{"cuota", http.MethodGet, "/api/v1/analytics/categories?period=2026-11", "jwt-valido", e053Limitador{permitido: false}, http.StatusTooManyRequests, "RATE_LIMITED"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			e053HTTPHandler(fuente, caso.limitador).ServeHTTP(rec, e004Peticion(caso.metodo, caso.ruta, caso.token, ""))
			if rec.Code != caso.espera {
				t.Fatalf("esperado %d, obtenido %d: %s", caso.espera, rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("Cache-Control"); got != "private, no-store" {
				t.Fatalf("Cache-Control ausente en error: %q", got)
			}
			var envelope struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil || envelope.Error.Code != caso.codigo {
				t.Fatalf("ErrorEnvelope incorrecto: %s", rec.Body.String())
			}
			if caso.nombre == "metodo" && rec.Header().Get("Allow") != http.MethodGet {
				t.Fatalf("Allow incorrecto: %q", rec.Header().Get("Allow"))
			}
		})
	}
}

func TestE053HTTPFailClosedYAislamiento(t *testing.T) {
	sinServicio := internalhttp.NewAnalyticsCategoriesHandler(nil, e004TokensValidos(), newE004Sesiones(), nil, silencioso())
	rec := httptest.NewRecorder()
	sinServicio.ServeHTTP(rec, e004Peticion(http.MethodGet, "/api/v1/analytics/categories", "jwt-valido", ""))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("sin servicio: esperado 503, obtenido %d", rec.Code)
	}

	fuente := &e053HTTPSource{byUser: map[string][]appMovements.Movement{
		e004HTTPUsuario: e053HTTPMovimientos(e004HTTPUsuario),
	}}
	h := e053HTTPHandler(fuente, nil)
	recB := httptest.NewRecorder()
	h.ServeHTTP(recB, e004Peticion(http.MethodGet, "/api/v1/analytics/categories?period=2026-11", "jwt-sujeto-ajeno", ""))
	if recB.Code != http.StatusOK {
		t.Fatalf("usuario B: esperado 200, obtenido %d: %s", recB.Code, recB.Body.String())
	}
	var cuerpo struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(recB.Body.Bytes(), &cuerpo); err != nil {
		t.Fatal(err)
	}
	if len(cuerpo.Items) != 0 {
		t.Fatalf("usuario B recibio datos de A: %s", recB.Body.String())
	}
}
