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

type e054HTTPSource struct {
	byUser map[string][]appMovements.Movement
}

func (f *e054HTTPSource) ListMovements(_ context.Context, q appMovements.ListQuery, startSK string, _ int) ([]appMovements.Movement, string, error) {
	if startSK != "" {
		return nil, "", nil
	}
	return append([]appMovements.Movement(nil), f.byUser[q.UserID]...), "", nil
}

func e054HTTPHandler(fuente *e054HTTPSource, limitador internalhttp.LimitadorRuta) *internalhttp.AnalyticsPatternsHandler {
	servicio := appAnalytics.NewService(fuente).WithClock(func() time.Time {
		return time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	})
	return internalhttp.NewAnalyticsPatternsHandler(servicio, e004TokensValidos(), newE004Sesiones(), limitador, silencioso())
}

func e054HTTPMovimientos(userID string, cantidad int) []appMovements.Movement {
	salida := make([]appMovements.Movement, 0, cantidad)
	for i := 0; i < cantidad; i++ {
		dia := 2 + i*7
		salida = append(salida, appMovements.Movement{
			ID:          "e054-" + userID + "-" + time.Duration(i).String(),
			Type:        appMovements.TypeExpense,
			AmountCents: 1500,
			Currency:    "MXN",
			CategoryID:  "food",
			OccurredAt:  time.Date(2026, 11, dia, 12, 0, 0, 0, time.UTC),
		})
	}
	return salida
}

func TestE054HTTPContratoValido(t *testing.T) {
	fuente := &e054HTTPSource{byUser: map[string][]appMovements.Movement{
		e004HTTPUsuario: e054HTTPMovimientos(e004HTTPUsuario, 4),
	}}
	rec := httptest.NewRecorder()
	e054HTTPHandler(fuente, e053Limitador{permitido: true}).ServeHTTP(
		rec, e004Peticion(http.MethodGet, "/api/v1/analytics/spending-patterns?period=2026-11", "jwt-valido", ""))

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("Cache-Control incorrecto: %q", got)
	}
	var cuerpo struct {
		Items []struct {
			CategoryID  string `json:"categoryId"`
			Description string `json:"description"`
			SampleSize  int    `json:"sampleSize"`
			Period      string `json:"period"`
		} `json:"items"`
		NextCursor *string `json:"nextCursor"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("JSON invalido: %v (%s)", err, rec.Body.String())
	}
	if len(cuerpo.Items) != 1 {
		t.Fatalf("esperado 1 patron, obtenidos %d: %s", len(cuerpo.Items), rec.Body.String())
	}
	item := cuerpo.Items[0]
	if item.CategoryID != "food" || item.SampleSize != 4 || item.Period != "2026-11" || item.Description == "" {
		t.Fatalf("item fuera de contrato: %+v", item)
	}
	if !regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`).MatchString(item.Period) {
		t.Fatalf("period no cumple el pattern: %q", item.Period)
	}
	if cuerpo.NextCursor != nil {
		t.Fatalf("nextCursor debe omitirse, obtenido %q", *cuerpo.NextCursor)
	}
	if strings.Contains(rec.Body.String(), e004HTTPUsuario) {
		t.Fatal("la respuesta expone el identificador del titular")
	}
}

func TestE054HTTPNegativas(t *testing.T) {
	fuente := &e054HTTPSource{byUser: map[string][]appMovements.Movement{
		e004HTTPUsuario: e054HTTPMovimientos(e004HTTPUsuario, 4),
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
		{"periodo_invalido", http.MethodGet, "/api/v1/analytics/spending-patterns?period=2026-13", "jwt-valido", e053Limitador{permitido: true}, http.StatusBadRequest, "INVALID_REQUEST"},
		{"sin_token", http.MethodGet, "/api/v1/analytics/spending-patterns?period=2026-11", "", e053Limitador{permitido: true}, http.StatusUnauthorized, "UNAUTHORIZED"},
		{"metodo", http.MethodPost, "/api/v1/analytics/spending-patterns", "jwt-valido", e053Limitador{permitido: true}, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED"},
		{"cuota", http.MethodGet, "/api/v1/analytics/spending-patterns?period=2026-11", "jwt-valido", e053Limitador{permitido: false}, http.StatusTooManyRequests, "RATE_LIMITED"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			e054HTTPHandler(fuente, caso.limitador).ServeHTTP(rec, e004Peticion(caso.metodo, caso.ruta, caso.token, ""))
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
		})
	}
}

func TestE054HTTPFailClosedYAislamiento(t *testing.T) {
	sinServicio := internalhttp.NewAnalyticsPatternsHandler(nil, e004TokensValidos(), newE004Sesiones(), nil, silencioso())
	rec := httptest.NewRecorder()
	sinServicio.ServeHTTP(rec, e004Peticion(http.MethodGet, "/api/v1/analytics/spending-patterns", "jwt-valido", ""))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("sin servicio: esperado 503, obtenido %d", rec.Code)
	}

	fuente := &e054HTTPSource{byUser: map[string][]appMovements.Movement{
		e004HTTPUsuario: e054HTTPMovimientos(e004HTTPUsuario, 4),
	}}
	recB := httptest.NewRecorder()
	e054HTTPHandler(fuente, nil).ServeHTTP(recB, e004Peticion(http.MethodGet, "/api/v1/analytics/spending-patterns?period=2026-11", "jwt-sujeto-ajeno", ""))
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
