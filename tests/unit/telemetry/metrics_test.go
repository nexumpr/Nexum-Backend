package telemetry_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	infraTelemetry "github.com/nexumpr/Nexum-Backend/internal/infrastructure/telemetry"
)

func TestMetricsCuentaPorRutaYEstado(t *testing.T) {
	m := infraTelemetry.NewMetrics()
	m.Observe(http.MethodGet, "/api/v1/health/live", 200, 10*time.Millisecond)
	m.Observe(http.MethodGet, "/api/v1/health/live", 200, 30*time.Millisecond)
	m.Observe(http.MethodPost, "/api/v1/movimientos", 500, 5*time.Millisecond)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("loopback debe recibir 200, obtenido %d", rec.Code)
	}
	cuerpo := rec.Body.String()
	if !strings.Contains(cuerpo, `nexum_http_requests_total{method="GET",route="/api/v1/health/live",status="200"} 2`) {
		t.Errorf("contador GET inesperado:\n%s", cuerpo)
	}
	if !strings.Contains(cuerpo, `nexum_http_requests_total{method="POST",route="/api/v1/movimientos",status="500"} 1`) {
		t.Errorf("contador POST inesperado:\n%s", cuerpo)
	}
	if !strings.Contains(cuerpo, "nexum_http_request_duration_seconds_sum") {
		t.Errorf("falta suma de latencia")
	}
	if !strings.Contains(cuerpo, "nexum_go_goroutines") {
		t.Errorf("falta métrica de runtime")
	}
}

func TestMetricsSoloLoopback(t *testing.T) {
	m := infraTelemetry.NewMetrics()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.RemoteAddr = "203.0.113.9:12345"
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("origen no loopback debe recibir 403, obtenido %d", rec.Code)
	}
}

func TestMetricsCardinalidadAcotada(t *testing.T) {
	m := infraTelemetry.NewMetrics()
	for i := 0; i < 2000; i++ {
		m.Observe(http.MethodGet, "/ruta/unica/"+strings.Repeat("x", i%20)+string(rune('a'+i%26)), 200, time.Millisecond)
	}
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, req)
	cuerpo := rec.Body.String()
	if !strings.Contains(cuerpo, "nexum_metrics_series 500") {
		t.Errorf("se esperaban 500 series máximas:\n%s", cuerpo)
	}
	if !strings.Contains(cuerpo, "nexum_metrics_series_dropped_total") {
		t.Errorf("falta contador de series descartadas")
	}
	if strings.Count(cuerpo, "nexum_http_requests_total{") > 500 {
		t.Errorf("se expusieron más de 500 series")
	}
}
