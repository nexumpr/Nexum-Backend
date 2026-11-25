package transport_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appAnalytics "github.com/nexumpr/Nexum-Backend/internal/application/analytics"
	appContrib "github.com/nexumpr/Nexum-Backend/internal/application/contributions"
	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

type e056HTTPStore struct {
	byUser map[string][]appContrib.Contribution
	zonas  map[string]string
	err    error
}

func (f *e056HTTPStore) ListContributions(_ context.Context, q appContrib.ListQuery, startSK string, _ int) ([]appContrib.Contribution, string, error) {
	if startSK != "" || f.err != nil {
		return nil, "", f.err
	}
	return append([]appContrib.Contribution(nil), f.byUser[q.UserID]...), "", nil
}

func (f *e056HTTPStore) LoadTimezone(_ context.Context, userID string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.zonas[userID], nil
}

func e056HTTPHandler(store *e056HTTPStore, limitador internalhttp.LimitadorRuta) *internalhttp.AnalyticsSavingsHandler {
	servicio := appAnalytics.NewService(nil).WithSavings(store, store).WithClock(func() time.Time {
		return time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	})
	return internalhttp.NewAnalyticsSavingsHandler(servicio, e004TokensValidos(), newE004Sesiones(), limitador, silencioso())
}

func e056HTTPStoreValido() *e056HTTPStore {
	return &e056HTTPStore{
		byUser: map[string][]appContrib.Contribution{
			e004HTTPUsuario: {
				{
					ID:          "ap-19",
					UserID:      e004HTTPUsuario,
					AmountCents: 10000,
					Currency:    "MXN",
					OccurredAt:  time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC),
					Status:      appContrib.StatusActive,
				},
				{
					ID:          "ap-20",
					UserID:      e004HTTPUsuario,
					AmountCents: 10000,
					Currency:    "MXN",
					OccurredAt:  time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC),
					Status:      appContrib.StatusActive,
				},
			},
		},
		zonas: map[string]string{e004HTTPUsuario: "America/Mexico_City", "jwt-sujeto-ajeno": "America/Mexico_City"},
	}
}

func TestE056HTTPContratoValido(t *testing.T) {
	rec := httptest.NewRecorder()
	e056HTTPHandler(e056HTTPStoreValido(), e053Limitador{permitido: true}).ServeHTTP(
		rec, e004Peticion(http.MethodGet, "/api/v1/analytics/savings-streak", "jwt-valido", ""))

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("Cache-Control incorrecto: %q", got)
	}
	var cuerpo struct {
		Days     int    `json:"days"`
		Timezone string `json:"timezone"`
		AsOfDate string `json:"asOfDate"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("JSON invalido: %v (%s)", err, rec.Body.String())
	}
	if cuerpo.Days != 2 || cuerpo.Timezone != "America/Mexico_City" || cuerpo.AsOfDate != "2026-11-20" {
		t.Fatalf("cuerpo fuera de contrato: %+v", cuerpo)
	}
	if strings.Contains(rec.Body.String(), e004HTTPUsuario) {
		t.Fatal("la respuesta expone el identificador del titular")
	}
}

func TestE056HTTPNegativas(t *testing.T) {
	casos := []struct {
		nombre    string
		metodo    string
		token     string
		limitador internalhttp.LimitadorRuta
		espera    int
		codigo    string
	}{
		{"sin_token", http.MethodGet, "", e053Limitador{permitido: true}, http.StatusUnauthorized, "UNAUTHORIZED"},
		{"metodo", http.MethodPost, "jwt-valido", e053Limitador{permitido: true}, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED"},
		{"cuota", http.MethodGet, "jwt-valido", e053Limitador{permitido: false}, http.StatusTooManyRequests, "RATE_LIMITED"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			e056HTTPHandler(e056HTTPStoreValido(), caso.limitador).ServeHTTP(
				rec, e004Peticion(caso.metodo, "/api/v1/analytics/savings-streak", caso.token, ""))
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

func TestE056HTTPFailClosedYAislamiento(t *testing.T) {
	sinServicio := internalhttp.NewAnalyticsSavingsHandler(nil, e004TokensValidos(), newE004Sesiones(), nil, silencioso())
	rec := httptest.NewRecorder()
	sinServicio.ServeHTTP(rec, e004Peticion(http.MethodGet, "/api/v1/analytics/savings-streak", "jwt-valido", ""))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("sin servicio: esperado 503, obtenido %d", rec.Code)
	}

	recB := httptest.NewRecorder()
	e056HTTPHandler(e056HTTPStoreValido(), nil).ServeHTTP(recB, e004Peticion(http.MethodGet, "/api/v1/analytics/savings-streak", "jwt-sujeto-ajeno", ""))
	if recB.Code != http.StatusOK {
		t.Fatalf("usuario B: esperado 200, obtenido %d: %s", recB.Code, recB.Body.String())
	}
	var cuerpo struct {
		Days int `json:"days"`
	}
	if err := json.Unmarshal(recB.Body.Bytes(), &cuerpo); err != nil {
		t.Fatal(err)
	}
	if cuerpo.Days != 0 {
		t.Fatalf("usuario B recibio racha de A: %s", recB.Body.String())
	}
}
