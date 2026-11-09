package transport_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	infraTelemetry "github.com/nexumpr/Nexum-Backend/internal/infrastructure/telemetry"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
	pkgTelemetry "github.com/nexumpr/Nexum-Backend/pkg/telemetry"
)

func TestRequestLoggerEmiteRegistroCanonicoSinIDs(t *testing.T) {
	var buffer bytes.Buffer
	logger := pkgTelemetry.NewJSONLogger(&buffer, 0)
	handler := middleware.RequestLogger(middleware.RequestLogOptions{
		Logger:      logger,
		Service:     "nexum-backend",
		Environment: "h08i",
		InstanceID:  "i-test",
		Metrics:     infraTelemetry.NewMetrics(),
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/movimientos/550e8400-e29b-41d4-a716-446655440000", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var registro map[string]any
	if err := json.Unmarshal(buffer.Bytes(), &registro); err != nil {
		t.Fatalf("log no es JSON válido: %v\n%s", err, buffer.String())
	}
	esperados := []string{"timestamp", "severity", "service", "environment", "traceId", "requestId", "route", "status", "latencyMs", "instanceId"}
	for _, campo := range esperados {
		if _, existe := registro[campo]; !existe {
			t.Errorf("falta %q en el registro: %s", campo, buffer.String())
		}
	}
	if registro["route"] != "/api/v1/movimientos/{id}" {
		t.Errorf("la ruta debe sanearse a plantilla, obtenido %v", registro["route"])
	}
	if registro["status"] != float64(http.StatusCreated) || registro["instanceId"] != "i-test" {
		t.Errorf("status/instanceId inesperados: %v", registro)
	}
	if strings.Contains(buffer.String(), "550e8400-e29b-41d4-a716-446655440000") {
		t.Errorf("el identificador crudo no debe aparecer en el log")
	}
	for _, prohibido := range []string{"authorization", "password", "token", "email", "monto"} {
		if strings.Contains(strings.ToLower(buffer.String()), prohibido) {
			t.Errorf("campo sensible %q presente en el log", prohibido)
		}
	}
}

func TestSanitizeRoute(t *testing.T) {
	casos := map[string]string{
		"/api/v1/health/live":                                "/api/v1/health/live",
		"/api/v1/movimientos/12345":                          "/api/v1/movimientos/{id}",
		"/api/v1/metas/550e8400-e29b-41d4-a716-446655440000": "/api/v1/metas/{id}",
		"/api/v1/sesiones/abcdefabcdefabcdefabcdef":          "/api/v1/sesiones/{id}",
	}
	for entrada, esperado := range casos {
		if obtenido := middleware.SanitizeRoute(entrada); obtenido != esperado {
			t.Errorf("%q: esperado %q, obtenido %q", entrada, esperado, obtenido)
		}
	}
}
