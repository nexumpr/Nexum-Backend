package telemetry

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// RequestRecord es el registro canónico de una petición HTTP (I-110, INF-110-01).
// Los campos cubren trazabilidad, latencia y estado sin incluir datos sensibles.
type RequestRecord struct {
	Timestamp   string            `json:"timestamp"`
	Severity    string            `json:"severity"`
	Service     string            `json:"service"`
	Environment string            `json:"environment"`
	TraceID     string            `json:"traceId"`
	RequestID   string            `json:"requestId"`
	Route       string            `json:"route"`
	Status      int               `json:"status"`
	LatencyMs   int64             `json:"latencyMs"`
	InstanceID  string            `json:"instanceId"`
	Error       string            `json:"error,omitempty"`
	Attributes  map[string]string `json:"attributes,omitempty"`
}

var clavesSensibles = []string{
	"password", "passwd", "secret", "token", "authorization", "cookie", "jwt",
	"api_key", "apikey", "email", "amount", "monto", "saldo", "voice", "transcript",
	"card", "cvv", "iban",
}

func claveSensible(clave string) bool {
	minuscula := strings.ToLower(clave)
	for _, prohibida := range clavesSensibles {
		if strings.Contains(minuscula, prohibida) {
			return true
		}
	}
	return false
}

// Validate comprueba campos obligatorios y rechaza atributos sensibles.
func (r RequestRecord) Validate() error {
	var faltantes []string
	for nombre, valor := range map[string]string{
		"timestamp": r.Timestamp, "severity": r.Severity, "service": r.Service,
		"environment": r.Environment, "traceId": r.TraceID, "requestId": r.RequestID,
		"route": r.Route, "instanceId": r.InstanceID,
	} {
		if strings.TrimSpace(valor) == "" {
			faltantes = append(faltantes, nombre)
		}
	}
	if len(faltantes) > 0 {
		return fmt.Errorf("campos obligatorios vacios: %s", strings.Join(faltantes, ","))
	}
	if r.Status < 100 || r.Status > 599 {
		return errors.New("status fuera de rango HTTP")
	}
	if r.LatencyMs < 0 {
		return errors.New("latencyMs no puede ser negativo")
	}
	for clave := range r.Attributes {
		if claveSensible(clave) {
			return fmt.Errorf("atributo sensible prohibido: %s", clave)
		}
	}
	return nil
}

// Sanitized devuelve una copia con valores sensibles enmascarados. No sustituye
// a Validate: un registro con claves prohibidas debe rechazarse, no publicarse.
func (r RequestRecord) Sanitized() RequestRecord {
	salida := r
	salida.Route = RedactSensitiveString(r.Route)
	salida.Error = RedactSensitiveString(r.Error)
	if r.Attributes != nil {
		salida.Attributes = make(map[string]string, len(r.Attributes))
		for clave, valor := range r.Attributes {
			if claveSensible(clave) {
				salida.Attributes[clave] = RedactedPlaceholder
				continue
			}
			salida.Attributes[clave] = RedactSensitiveString(valor)
		}
	}
	return salida
}

// JSON serializa el registro para emitirlo con slog o stdout.
func (r RequestRecord) JSON() ([]byte, error) {
	return json.Marshal(r)
}
