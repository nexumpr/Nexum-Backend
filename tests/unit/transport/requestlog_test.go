package transport_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nexumpr/Nexum-Backend/internal/infrastructure/telemetry"
	transportHttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
	pkgTelemetry "github.com/nexumpr/Nexum-Backend/pkg/telemetry"
)

func loggerDePrueba(buffer *bytes.Buffer, router *transportHttp.Router) middleware.Middleware {
	return middleware.RequestLogger(middleware.RequestLogOptions{
		Logger:        pkgTelemetry.NewJSONLogger(buffer, 0),
		Service:       "nexum-backend",
		Environment:   "h08i",
		InstanceID:    "i-test",
		Metrics:       telemetry.NewMetrics(),
		RouteResolver: router.TemplateFor,
	})
}

func TestRequestLoggerEmiteRegistroCanonicoSinIDs(t *testing.T) {
	var buffer bytes.Buffer
	router := transportHttp.NewRouter()
	router.HandleFunc(http.MethodGet, "/health/live", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	handler := loggerDePrueba(&buffer, router)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health/live", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var registro map[string]any
	if err := json.Unmarshal(buffer.Bytes(), &registro); err != nil {
		t.Fatalf("log no es JSON válido: %v\n%s", err, buffer.String())
	}
	for _, campo := range []string{"timestamp", "severity", "service", "environment", "traceId", "requestId", "route", "status", "latencyMs", "instanceId"} {
		if _, existe := registro[campo]; !existe {
			t.Errorf("falta %q en el registro: %s", campo, buffer.String())
		}
	}
	if registro["route"] != "/api/v1/health/live" {
		t.Errorf("ruta conocida debe conservar plantilla, obtenido %v", registro["route"])
	}
	if registro["status"] != float64(http.StatusCreated) || registro["instanceId"] != "i-test" {
		t.Errorf("status/instanceId inesperados: %v", registro)
	}
}

func TestRequestLoggerNoFiltraRutasDesconocidasNiSensibles(t *testing.T) {
	router := transportHttp.NewRouter()
	router.HandleFunc(http.MethodGet, "/health/live", func(w http.ResponseWriter, r *http.Request) {})
	casos := []string{
		"/api/v1/movimientos/550e8400-e29b-41d4-a716-446655440000",
		"/api/v1/metas/12345",
		"/api/v1/usuarios/persona@nexum.com",
		"/api/v1/usuarios/juan.perez",
		"/api/v1/sesiones/eyJhbGciOiJSUzI1NiJ9.token",
		"/api/v1/%2e%2e/%2e%2e/etc/passwd",
		"/api/v1/" + strings.Repeat("a", 2048),
	}
	for _, ruta := range casos {
		var buffer bytes.Buffer
		handler := loggerDePrueba(&buffer, router)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		req := httptest.NewRequest(http.MethodGet, ruta, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		var registro map[string]any
		if err := json.Unmarshal(buffer.Bytes(), &registro); err != nil {
			t.Fatalf("log inválido para %q: %v", ruta, err)
		}
		if registro["route"] != "/unknown" {
			t.Errorf("ruta desconocida %q debe registrarse como /unknown, obtenido %v", ruta, registro["route"])
		}
		if strings.Contains(buffer.String(), "persona@nexum.com") || strings.Contains(buffer.String(), "juan.perez") || strings.Contains(buffer.String(), "eyJhbGci") {
			t.Errorf("dato sensible filtrado en el log para %q: %s", ruta, buffer.String())
		}
	}
}

func TestRequestLoggerRutasUnicasAcotadas(t *testing.T) {
	var buffer bytes.Buffer
	router := transportHttp.NewRouter()
	handler := loggerDePrueba(&buffer, router)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for i := 0; i < 2000; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/desconocida/"+strings.Repeat("x", i%50)+string(rune('a'+i%26)), nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
	if strings.Count(buffer.String(), `"route":"/unknown"`) != 2000 {
		t.Errorf("todas las rutas desconocidas deben usar /unknown")
	}
}
