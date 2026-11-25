package transport_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appAnalytics "github.com/nexumpr/Nexum-Backend/internal/application/analytics"
	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

type e052HTTPSource struct {
	byUser map[string][]appMovements.Movement
}

func (f *e052HTTPSource) ListMovements(_ context.Context, q appMovements.ListQuery, startSK string, _ int) ([]appMovements.Movement, string, error) {
	if startSK != "" {
		return nil, "", nil
	}
	return append([]appMovements.Movement(nil), f.byUser[q.UserID]...), "", nil
}

func e052HTTPMovimientos(userID string, cantidad int) []appMovements.Movement {
	salida := make([]appMovements.Movement, 0, cantidad)
	base := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	for i := 0; i < cantidad; i++ {
		salida = append(salida, appMovements.Movement{
			ID:          "e052-" + userID + "-" + time.Duration(i).String(),
			Type:        appMovements.TypeExpense,
			AmountCents: 10000,
			CategoryID:  "comida",
			OccurredAt:  base.Add(time.Duration(i) * time.Hour),
		})
	}
	return salida
}

func e052HTTPHandler(fuente *e052HTTPSource) *internalhttp.AnalyticsInsightsHandler {
	servicio := appAnalytics.NewService(fuente).WithClock(func() time.Time {
		return time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	})
	return internalhttp.NewAnalyticsInsightsHandler(servicio, e004TokensValidos(), newE004Sesiones(), nil, silencioso())
}

func TestE052HTTPContratoValido(t *testing.T) {
	fuente := &e052HTTPSource{byUser: map[string][]appMovements.Movement{
		e004HTTPUsuario: e052HTTPMovimientos(e004HTTPUsuario, 12),
	}}
	h := e052HTTPHandler(fuente)

	req := e004Peticion(http.MethodGet, "/api/v1/analytics/insights?period=2026-11", "jwt-valido", "")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("Cache-Control incorrecto: %q", got)
	}

	var cuerpo struct {
		Items []struct {
			ID            string `json:"id"`
			Title         string `json:"title"`
			Explanation   string `json:"explanation"`
			Period        string `json:"period"`
			EvidenceCount int    `json:"evidenceCount"`
			ModelVersion  string `json:"modelVersion"`
		} `json:"items"`
		NextCursor     *string `json:"nextCursor"`
		EnoughData     bool    `json:"enoughData"`
		MinimumSamples int     `json:"minimumSamples"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("JSON invalido: %v (%s)", err, rec.Body.String())
	}
	if !cuerpo.EnoughData || cuerpo.MinimumSamples != 12 || len(cuerpo.Items) != 1 {
		t.Fatalf("respuesta incompleta: %+v", cuerpo)
	}
	item := cuerpo.Items[0]
	if item.ID == "" || item.Title == "" || item.Explanation == "" || item.Period != "2026-11" ||
		item.EvidenceCount != 12 || item.ModelVersion != "analytics-v1" {
		t.Fatalf("Insight no conforme: %+v", item)
	}
	if cuerpo.NextCursor != nil {
		t.Fatalf("nextCursor debe omitirse, obtenido %q", *cuerpo.NextCursor)
	}
	if strings.Contains(rec.Body.String(), e004HTTPUsuario) {
		t.Fatal("la respuesta expone el identificador del titular")
	}
}

func TestE052HTTPNegativas(t *testing.T) {
	fuente := &e052HTTPSource{byUser: map[string][]appMovements.Movement{
		e004HTTPUsuario: e052HTTPMovimientos(e004HTTPUsuario, 12),
	}}
	h := e052HTTPHandler(fuente)

	casos := []struct {
		nombre string
		metodo string
		ruta   string
		token  string
		espera int
	}{
		{"periodo_invalido", http.MethodGet, "/api/v1/analytics/insights?period=2026-13", "jwt-valido", http.StatusBadRequest},
		{"sin_token", http.MethodGet, "/api/v1/analytics/insights?period=2026-11", "", http.StatusUnauthorized},
		{"metodo", http.MethodPost, "/api/v1/analytics/insights", "jwt-valido", http.StatusMethodNotAllowed},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, e004Peticion(caso.metodo, caso.ruta, caso.token, ""))
			if rec.Code != caso.espera {
				t.Fatalf("esperado %d, obtenido %d: %s", caso.espera, rec.Code, rec.Body.String())
			}
			var envelope struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil || envelope.Error.Code == "" {
				t.Fatalf("ErrorEnvelope ausente: %s", rec.Body.String())
			}
			if caso.nombre == "metodo" && rec.Header().Get("Allow") != http.MethodGet {
				t.Fatalf("Allow incorrecto: %q", rec.Header().Get("Allow"))
			}
		})
	}
}

func TestE052HTTPFailClosedYAislamiento(t *testing.T) {
	soloServicio := internalhttp.NewAnalyticsInsightsHandler(nil, e004TokensValidos(), newE004Sesiones(), nil, silencioso())
	rec := httptest.NewRecorder()
	soloServicio.ServeHTTP(rec, e004Peticion(http.MethodGet, "/api/v1/analytics/insights", "jwt-valido", ""))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("sin servicio: esperado 503, obtenido %d", rec.Code)
	}

	fuente := &e052HTTPSource{byUser: map[string][]appMovements.Movement{
		e004HTTPUsuario: e052HTTPMovimientos(e004HTTPUsuario, 12),
	}}
	h := e052HTTPHandler(fuente)
	recB := httptest.NewRecorder()
	h.ServeHTTP(recB, e004Peticion(http.MethodGet, "/api/v1/analytics/insights?period=2026-11", "jwt-sujeto-ajeno", ""))
	if recB.Code != http.StatusOK {
		t.Fatalf("usuario B: esperado 200, obtenido %d: %s", recB.Code, recB.Body.String())
	}
	if !strings.Contains(recB.Body.String(), `"enoughData":false`) || strings.Contains(recB.Body.String(), "comida") {
		t.Fatalf("usuario B recibio datos de A: %s", recB.Body.String())
	}
}

// TestE052SchemaPeriodRequeridoYPatron contrasta el contrato aprobado con la
// respuesta real. `period` es requerido y con pattern; el handler recibe `period`
// omitido y aun asi debe publicarlo normalizado (nunca "").
func TestE052SchemaPeriodRequeridoYPatron(t *testing.T) {
	root := repoRoot(t)
	yamlData, err := os.ReadFile(filepath.Join(root, "sdd", "03-endpoints", "openapi", "nexum-api-v1.yaml"))
	if err != nil {
		t.Fatalf("no se pudo leer el OpenAPI: %v", err)
	}
	bloque := sliceBetween(string(yamlData), "    Insight:", "    PageOfInsight:")
	if bloque == "" {
		t.Fatal("no se encontro el esquema Insight en el OpenAPI")
	}
	if !strings.Contains(bloque, "pattern: ^\\d{4}-(0[1-9]|1[0-2])$") {
		t.Fatalf("Insight.period no declara el pattern aprobado:\n%s", bloque)
	}
	requeridos := sliceBetween(bloque, "      required:", "      description:")
	if !strings.Contains(requeridos, "- period") {
		t.Fatalf("Insight.period debe ser requerido:\n%s", requeridos)
	}

	fuente := &e052HTTPSource{byUser: map[string][]appMovements.Movement{
		e004HTTPUsuario: e052HTTPMovimientos(e004HTTPUsuario, 12),
	}}
	h := e052HTTPHandler(fuente)
	rec := httptest.NewRecorder()
	// Sin `period`: el servicio debe usar el mes UTC del reloj inyectado (2026-11).
	h.ServeHTTP(rec, e004Peticion(http.MethodGet, "/api/v1/analytics/insights", "jwt-valido", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d: %s", rec.Code, rec.Body.String())
	}
	var cuerpo struct {
		Items []struct {
			Period string `json:"period"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("JSON invalido: %v", err)
	}
	if len(cuerpo.Items) != 1 || cuerpo.Items[0].Period == "" {
		t.Fatalf("el JSON debe incluir period no vacio: %s", rec.Body.String())
	}
	if cuerpo.Items[0].Period != "2026-11" {
		t.Fatalf("period omitido debe normalizarse a 2026-11: %q", cuerpo.Items[0].Period)
	}
	if strings.Contains(rec.Body.String(), `"period":""`) {
		t.Fatalf("period no puede serializarse vacio: %s", rec.Body.String())
	}
}
