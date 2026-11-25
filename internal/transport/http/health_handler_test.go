package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

func healthProbe(t *testing.T, handler http.Handler, ruta string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, ruta, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// TestE072LivenessEsquemaContractualSinDependencias valida que liveness devuelve
// exactamente el esquema `Health` (additionalProperties:false) y que no consulta
// ninguna dependencia.
func TestE072LivenessEsquemaContractualSinDependencias(t *testing.T) {
	registry := NewHealthRegistry(50 * time.Millisecond)
	consultada := false
	registry.RegisterDependency("dynamodb", func(context.Context) error {
		consultada = true
		return errors.New("no debe consultarse en liveness")
	})

	rec := healthProbe(t, registry.LiveHandler(), "/api/v1/health/live")
	if rec.Code != http.StatusOK {
		t.Fatalf("liveness=%d, esperaba 200: %s", rec.Code, rec.Body.String())
	}
	if consultada {
		t.Fatal("liveness no debe consultar dependencias")
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control=%q", got)
	}
	if !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("Content-Type=%q", rec.Header().Get("Content-Type"))
	}

	var cuerpo map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("cuerpo no JSON: %v", err)
	}
	if len(cuerpo) != 3 {
		t.Fatalf("el esquema Health no admite campos extra: %s", rec.Body.String())
	}
	if cuerpo["status"] != "ok" || cuerpo["service"] != HealthServiceName {
		t.Fatalf("cuerpo fuera de contrato: %s", rec.Body.String())
	}
	timestamp, ok := cuerpo["timestamp"].(string)
	if !ok {
		t.Fatalf("timestamp ausente: %s", rec.Body.String())
	}
	if _, err := time.Parse(time.RFC3339, timestamp); err != nil {
		t.Fatalf("timestamp no RFC3339: %q", timestamp)
	}
	if strings.Contains(rec.Body.String(), "dependencies") || strings.Contains(rec.Body.String(), "UP") {
		t.Fatalf("la respuesta no debe exponer el estado interno: %s", rec.Body.String())
	}
}

// TestE073ReadinessSanaEsquemaContractual valida 200 con el mismo esquema Health.
func TestE073ReadinessSanaEsquemaContractual(t *testing.T) {
	registry := NewHealthRegistry(time.Second)
	registry.RegisterDependency("dynamodb", func(context.Context) error { return nil })
	registry.RegisterDependency("redis", func(context.Context) error { return nil })

	rec := healthProbe(t, registry.ReadyHandler(), "/api/v1/health/ready")
	if rec.Code != http.StatusOK {
		t.Fatalf("readiness=%d, esperaba 200: %s", rec.Code, rec.Body.String())
	}
	var resp HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("cuerpo no JSON: %v", err)
	}
	if resp.Status != "ok" || resp.Service != HealthServiceName {
		t.Fatalf("cuerpo fuera de contrato: %+v", resp)
	}
	if _, err := time.Parse(time.RFC3339, resp.Timestamp); err != nil {
		t.Fatalf("timestamp no RFC3339: %q", resp.Timestamp)
	}
	var cuerpo map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &cuerpo)
	if len(cuerpo) != 3 {
		t.Fatalf("el esquema Health no admite campos extra: %s", rec.Body.String())
	}
}

// TestE073Readiness503EsErrorEnvelopeSinDetalles valida que el 503 usa el
// envelope canónico, conserva el traceId y no filtra el error del proveedor.
func TestE073Readiness503EsErrorEnvelopeSinDetalles(t *testing.T) {
	registry := NewHealthRegistry(100 * time.Millisecond)
	registry.RegisterDependency("dynamodb", func(context.Context) error {
		return errors.New("credenciales internas usuario=admin host=10.0.0.5")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health/ready", nil)
	req.Header.Set("X-Request-ID", "trace-readiness-503")
	rec := httptest.NewRecorder()
	registry.ReadyHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("readiness=%d, esperaba 503: %s", rec.Code, rec.Body.String())
	}
	var envelope httputil.ErrorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("el 503 no es ErrorEnvelope: %v (%s)", err, rec.Body.String())
	}
	if envelope.Error.Code != "DEPENDENCY_UNAVAILABLE" {
		t.Fatalf("code=%q", envelope.Error.Code)
	}
	if envelope.Error.TraceID != "trace-readiness-503" {
		t.Fatalf("traceId=%q", envelope.Error.TraceID)
	}
	if envelope.Error.Details == nil || len(envelope.Error.Details) != 0 {
		t.Fatalf("details debe ser una lista vacía: %+v", envelope.Error.Details)
	}
	for _, prohibido := range []string{"credenciales", "admin", "10.0.0.5", "dependencies", "dynamodb"} {
		if strings.Contains(rec.Body.String(), prohibido) {
			t.Fatalf("el 503 filtra %q: %s", prohibido, rec.Body.String())
		}
	}
}

// TestE073ReadinessSinDependenciasYEnApagado valida los 503 por diseño.
func TestE073ReadinessSinDependenciasYEnApagado(t *testing.T) {
	registry := NewHealthRegistry(50 * time.Millisecond)
	rec := healthProbe(t, registry.ReadyHandler(), "/api/v1/health/ready")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("sin dependencias=%d, esperaba 503", rec.Code)
	}
	var envelope httputil.ErrorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("sin dependencias: cuerpo no es ErrorEnvelope: %v", err)
	}
	if envelope.Error.Code != "DEPENDENCY_UNAVAILABLE" {
		t.Fatalf("sin dependencias: code=%q", envelope.Error.Code)
	}

	registry.RegisterDependency("dynamodb", func(context.Context) error { return nil })
	if rec := healthProbe(t, registry.ReadyHandler(), "/api/v1/health/ready"); rec.Code != http.StatusOK {
		t.Fatalf("con dependencia sana=%d, esperaba 200", rec.Code)
	}
	registry.BeginShutdown()
	rec = healthProbe(t, registry.ReadyHandler(), "/api/v1/health/ready")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("en apagado=%d, esperaba 503", rec.Code)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("en apagado: cuerpo no es ErrorEnvelope: %v", err)
	}
}

// TestE073ReadinessTimeoutNoBloquea valida el timeout acotado: una dependencia
// que ignora el contexto no puede colgar la sonda.
func TestE073ReadinessTimeoutNoBloquea(t *testing.T) {
	registry := NewHealthRegistry(80 * time.Millisecond)
	registry.RegisterDependency("redis", func(context.Context) error {
		time.Sleep(2 * time.Second)
		return nil
	})

	inicio := time.Now()
	rec := healthProbe(t, registry.ReadyHandler(), "/api/v1/health/ready")
	transcurrido := time.Since(inicio)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("timeout=%d, esperaba 503", rec.Code)
	}
	if transcurrido > time.Second {
		t.Fatalf("readiness tardó %v y debe respetar el timeout", transcurrido)
	}
}
