package telemetry_test

import (
	"encoding/json"
	"strings"
	"testing"

	infraTelemetry "github.com/nexumpr/Nexum-Backend/internal/infrastructure/telemetry"
)

func registroValido() infraTelemetry.RequestRecord {
	return infraTelemetry.RequestRecord{
		Timestamp:   "2026-11-08T12:00:00Z",
		Severity:    "INFO",
		Service:     "nexum-backend",
		Environment: "h08i",
		TraceID:     "4bf92f3577b34da6a3ce929d0e0e4736",
		RequestID:   "req-1",
		Route:       "/api/v1/health/live",
		Status:      200,
		LatencyMs:   12,
		InstanceID:  "i-0123456789abcdef0",
	}
}

func TestRequestRecordValido(t *testing.T) {
	if err := registroValido().Validate(); err != nil {
		t.Fatalf("registro válido rechazado: %v", err)
	}
}

func TestRequestRecordCamposObligatorios(t *testing.T) {
	casos := map[string]func(*infraTelemetry.RequestRecord){
		"traceId":     func(r *infraTelemetry.RequestRecord) { r.TraceID = " " },
		"requestId":   func(r *infraTelemetry.RequestRecord) { r.RequestID = "" },
		"route":       func(r *infraTelemetry.RequestRecord) { r.Route = "" },
		"instanceId":  func(r *infraTelemetry.RequestRecord) { r.InstanceID = "" },
		"environment": func(r *infraTelemetry.RequestRecord) { r.Environment = "" },
	}
	for nombre, mutar := range casos {
		r := registroValido()
		mutar(&r)
		if err := r.Validate(); err == nil || !strings.Contains(err.Error(), nombre) {
			t.Errorf("%s: se esperaba error mencionando el campo, obtenido %v", nombre, err)
		}
	}
}

func TestRequestRecordRechazaRangoYLatencia(t *testing.T) {
	r := registroValido()
	r.Status = 99
	if err := r.Validate(); err == nil {
		t.Errorf("status 99 debe rechazarse")
	}
	r = registroValido()
	r.Status = 600
	if err := r.Validate(); err == nil {
		t.Errorf("status 600 debe rechazarse")
	}
	r = registroValido()
	r.LatencyMs = -1
	if err := r.Validate(); err == nil {
		t.Errorf("latencia negativa debe rechazarse")
	}
}

func TestRequestRecordRechazaAtributosSensibles(t *testing.T) {
	for _, clave := range []string{"authorization", "email", "monto", "raw_voice", "token"} {
		r := registroValido()
		r.Attributes = map[string]string{clave: "x"}
		if err := r.Validate(); err == nil {
			t.Errorf("atributo sensible %q debe rechazarse", clave)
		}
	}
}

func TestRequestRecordSanitizaValores(t *testing.T) {
	r := registroValido()
	r.Error = "fallo con Bearer abc.def.ghi y usuario@nexum.com"
	r.Attributes = map[string]string{
		"detalle": "contacto usuario@nexum.com",
		"email":   "otro@nexum.com",
	}
	s := r.Sanitized()
	if strings.Contains(s.Attributes["detalle"], "usuario@nexum.com") {
		t.Errorf("el email debe quedar enmascarado: %q", s.Attributes["detalle"])
	}
	if s.Attributes["email"] != infraTelemetry.RedactedPlaceholder {
		t.Errorf("clave sensible debe enmascararse: %q", s.Attributes["email"])
	}
	if strings.Contains(s.Error, "abc.def.ghi") {
		t.Errorf("el token del error debe quedar enmascarado: %q", s.Error)
	}
}

func TestRequestRecordJSONContieneCampos(t *testing.T) {
	datos, err := registroValido().JSON()
	if err != nil {
		t.Fatalf("JSON: %v", err)
	}
	var mapa map[string]any
	if err := json.Unmarshal(datos, &mapa); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, campo := range []string{
		"timestamp", "severity", "service", "environment", "traceId", "requestId",
		"route", "status", "latencyMs", "instanceId",
	} {
		if _, existe := mapa[campo]; !existe {
			t.Errorf("falta el campo %q en el JSON", campo)
		}
	}
}
