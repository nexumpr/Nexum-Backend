// Package demobootstrap implementa E074: respuesta de arranque de demostración
// aislada. No inserta datos, no llama a DynamoDB y no distingue entre sujetos.
package demobootstrap

import (
	"fmt"
	"os"
	"strings"
)

const (
	// ScopeSandbox es el único scope del contrato.
	ScopeSandbox = "sandbox"
	// EnvEnabled es la variable de entorno que habilita el modo demostrativo.
	// Si falta o no se puede interpretar, el valor es false.
	EnvEnabled = "NEXUM_DEMO_ENABLED"
)

// Wire es la respuesta JSON estricta de E074.
type Wire struct {
	Enabled bool   `json:"enabled"`
	Scope   string `json:"scope"`
}

// Service mantiene únicamente el interruptor de configuración: no tiene
// repositorios, clientes ni otra dependencia que pueda escribir datos reales.
type Service struct {
	enabled bool
}

// NewService crea el servicio con el valor ya resuelto desde configuración.
func NewService(enabled bool) *Service {
	return &Service{enabled: enabled}
}

// NewServiceFromEnvironment resuelve el interruptor desde el entorno del
// proceso con fail-closed: ausencia o valor desconocido desactiva la demo.
func NewServiceFromEnvironment() *Service {
	return NewService(EnabledFromEnvironment(os.LookupEnv))
}

// Bootstrap devuelve una respuesta estable e independiente del sujeto.
// Un receptor nil se comporta como desactivado.
func (s *Service) Bootstrap() Wire {
	enabled := false
	if s != nil {
		enabled = s.enabled
	}
	return Wire{Enabled: enabled, Scope: ScopeSandbox}
}

// EnabledFromEnvironment interpreta EnvEnabled usando lookup. lookup nil o
// variable ausente equivalen a false; cualquier valor no reconocido también.
func EnabledFromEnvironment(lookup func(string) (string, bool)) bool {
	if lookup == nil {
		return false
	}
	raw, ok := lookup(EnvEnabled)
	if !ok {
		return false
	}
	enabled, err := ParseEnabled(raw)
	if err != nil {
		return false
	}
	return enabled
}

// ParseEnabled acepta representaciones booleanas usuales de configuración.
// El valor vacío es false; cualquier otro valor devuelve error.
func ParseEnabled(raw string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "0", "false", "no", "off":
		return false, nil
	case "1", "true", "yes", "on":
		return true, nil
	default:
		return false, fmt.Errorf("valor booleano no reconocido para %s", EnvEnabled)
	}
}
