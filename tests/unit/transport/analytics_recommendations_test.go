package transport_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	appAnalytics "github.com/nexumpr/Nexum-Backend/internal/application/analytics"
	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
	"github.com/nexumpr/Nexum-Backend/ml"
)

type e055HTTPSource struct {
	byUser map[string][]appMovements.Movement
}

func (f *e055HTTPSource) ListMovements(_ context.Context, q appMovements.ListQuery, startSK string, _ int) ([]appMovements.Movement, string, error) {
	if startSK != "" {
		return nil, "", nil
	}
	return append([]appMovements.Movement(nil), f.byUser[q.UserID]...), "", nil
}

func e055HTTPHandler(fuente *e055HTTPSource, limitador internalhttp.LimitadorRuta) *internalhttp.AnalyticsRecommendationsHandler {
	servicio := appAnalytics.NewService(fuente).WithClock(func() time.Time {
		return time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	})
	return internalhttp.NewAnalyticsRecommendationsHandler(servicio, e004TokensValidos(), newE004Sesiones(), limitador, silencioso())
}

func e055HTTPMovimientos(userID string) []appMovements.Movement {
	salida := make([]appMovements.Movement, 0, 12)
	for i := 0; i < 12; i++ {
		categoria := "comida"
		cents := int64(18000)
		if i >= 6 {
			categoria = "vivienda"
			cents = 280000
		}
		salida = append(salida, appMovements.Movement{
			ID:                 fmt.Sprintf("e055-http-%s-%d", userID, i),
			Type:               appMovements.TypeExpense,
			AmountCents:        cents,
			Currency:           "MXN",
			CategoryID:         categoria,
			OccurredAt:         time.Date(2026, 11, 1+i%28, 9+i%8, 0, 0, 0, time.UTC),
			RecurringPaymentID: fmt.Sprintf("rec-http-%d", i%3),
		})
	}
	return salida
}

func TestE055HTTPContratoValido(t *testing.T) {
	fuente := &e055HTTPSource{byUser: map[string][]appMovements.Movement{
		e004HTTPUsuario: e055HTTPMovimientos(e004HTTPUsuario),
	}}
	rec := httptest.NewRecorder()
	e055HTTPHandler(fuente, e053Limitador{permitido: true}).ServeHTTP(
		rec, e004Peticion(http.MethodGet, "/api/v1/analytics/recommendations?period=2026-11", "jwt-valido", ""))

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("Cache-Control incorrecto: %q", got)
	}
	var cuerpo struct {
		Items []struct {
			ID           string `json:"id"`
			Title        string `json:"title"`
			Reason       string `json:"reason"`
			Disclaimer   string `json:"disclaimer"`
			ModelVersion string `json:"modelVersion"`
		} `json:"items"`
		NextCursor    *string `json:"nextCursor"`
		HasEnoughData bool    `json:"hasEnoughData"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("JSON invalido: %v (%s)", err, rec.Body.String())
	}
	if !cuerpo.HasEnoughData || len(cuerpo.Items) == 0 {
		t.Fatalf("esperadas recomendaciones con historial suficiente: %s", rec.Body.String())
	}
	uuid := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-5[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	for _, item := range cuerpo.Items {
		if !uuid.MatchString(item.ID) || item.Title == "" || item.Reason == "" ||
			item.Disclaimer != appAnalytics.RecommendationDisclaimer || item.ModelVersion != ml.ActiveModelVersion {
			t.Fatalf("item fuera de contrato: %+v", item)
		}
	}
	if cuerpo.NextCursor != nil {
		t.Fatalf("nextCursor debe omitirse, obtenido %q", *cuerpo.NextCursor)
	}
	if strings.Contains(rec.Body.String(), e004HTTPUsuario) {
		t.Fatal("la respuesta expone el identificador del titular")
	}
}

func TestE055HTTPNegativas(t *testing.T) {
	fuente := &e055HTTPSource{byUser: map[string][]appMovements.Movement{
		e004HTTPUsuario: e055HTTPMovimientos(e004HTTPUsuario),
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
		{"periodo_invalido", http.MethodGet, "/api/v1/analytics/recommendations?period=2026-13", "jwt-valido", e053Limitador{permitido: true}, http.StatusBadRequest, "INVALID_REQUEST"},
		{"sin_token", http.MethodGet, "/api/v1/analytics/recommendations?period=2026-11", "", e053Limitador{permitido: true}, http.StatusUnauthorized, "UNAUTHORIZED"},
		{"metodo", http.MethodPost, "/api/v1/analytics/recommendations", "jwt-valido", e053Limitador{permitido: true}, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED"},
		{"cuota", http.MethodGet, "/api/v1/analytics/recommendations?period=2026-11", "jwt-valido", e053Limitador{permitido: false}, http.StatusTooManyRequests, "RATE_LIMITED"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			e055HTTPHandler(fuente, caso.limitador).ServeHTTP(rec, e004Peticion(caso.metodo, caso.ruta, caso.token, ""))
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

func TestE055HTTPFailClosedYAislamiento(t *testing.T) {
	sinServicio := internalhttp.NewAnalyticsRecommendationsHandler(nil, e004TokensValidos(), newE004Sesiones(), nil, silencioso())
	rec := httptest.NewRecorder()
	sinServicio.ServeHTTP(rec, e004Peticion(http.MethodGet, "/api/v1/analytics/recommendations", "jwt-valido", ""))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("sin servicio: esperado 503, obtenido %d", rec.Code)
	}

	fuente := &e055HTTPSource{byUser: map[string][]appMovements.Movement{
		e004HTTPUsuario: e055HTTPMovimientos(e004HTTPUsuario),
	}}
	recB := httptest.NewRecorder()
	e055HTTPHandler(fuente, nil).ServeHTTP(recB, e004Peticion(http.MethodGet, "/api/v1/analytics/recommendations?period=2026-11", "jwt-sujeto-ajeno", ""))
	if recB.Code != http.StatusOK {
		t.Fatalf("usuario B: esperado 200, obtenido %d: %s", recB.Code, recB.Body.String())
	}
	var cuerpo struct {
		Items         []json.RawMessage `json:"items"`
		HasEnoughData bool              `json:"hasEnoughData"`
	}
	if err := json.Unmarshal(recB.Body.Bytes(), &cuerpo); err != nil {
		t.Fatal(err)
	}
	if cuerpo.HasEnoughData || len(cuerpo.Items) != 0 {
		t.Fatalf("usuario B recibio datos de A: %s", recB.Body.String())
	}
}
