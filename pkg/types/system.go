package types

import "fmt"

// ActorType identifica los actores que interactúan con el sistema según A-000 §3.
type ActorType string

const (
	// ActorUser representa a la persona usuaria final de Nexum.
	ActorUser ActorType = "USER"
	// ActorMobileClient representa al cliente móvil consumidor (Android/Kotlin u otro).
	ActorMobileClient ActorType = "MOBILE_CLIENT"
	// ActorSystemOperator representa al operador técnico del Backend con mínimo privilegio.
	ActorSystemOperator ActorType = "SYSTEM_OPERATOR"
	// ActorInfrastructure representa la infraestructura de cómputo y servicios externos.
	ActorInfrastructure ActorType = "INFRASTRUCTURE"
)

// IsValid comprueba si el tipo de actor es reconocido en la especificación A-000.
func (a ActorType) IsValid() bool {
	switch a {
	case ActorUser, ActorMobileClient, ActorSystemOperator, ActorInfrastructure:
		return true
	default:
		return false
	}
}

// Environment representa el entorno de ejecución del sistema.
type Environment string

const (
	EnvLocal       Environment = "LOCAL"
	EnvCI          Environment = "CI"
	EnvProduction  Environment = "PRODUCTION"
)

// IsValid verifica que el entorno de ejecución sea soportado.
func (e Environment) IsValid() bool {
	switch e {
	case EnvLocal, EnvCI, EnvProduction:
		return true
	default:
		return false
	}
}

// SystemMetadata describe los atributos arquitectónicos del contexto del sistema.
type SystemMetadata struct {
	AppName       string      `json:"app_name"`
	Version       string      `json:"version"`
	SpecVersion   string      `json:"spec_version"`
	Environment   Environment `json:"environment"`
	MinNodeCount  int         `json:"min_node_count"`
	TLSRequired   bool        `json:"tls_required"`
	NoSQLProvider string      `json:"nosql_provider"`
	CacheProvider string      `json:"cache_provider"`
}

// Validate comprueba las invariantes obligatorias de contexto de arquitectura de A-000.
func (m SystemMetadata) Validate() error {
	if m.AppName == "" {
		return fmt.Errorf("app_name es requerido")
	}
	if !m.Environment.IsValid() {
		return fmt.Errorf("entorno inválido: %s", m.Environment)
	}
	if m.MinNodeCount < 2 && m.Environment == EnvProduction {
		return fmt.Errorf("la especificación GOV-02/CTX-002 exige un mínimo de 2 nodos independientes en producción")
	}
	if !m.TLSRequired && m.Environment == EnvProduction {
		return fmt.Errorf("la especificación GOV-03/CTX-002 exige HTTPS/TLS obligatorio en producción")
	}
	if m.NoSQLProvider == "" {
		return fmt.Errorf("la especificación GOV-04/CTX-003 exige persistencia NoSQL DynamoDB")
	}
	if m.CacheProvider == "" {
		return fmt.Errorf("la especificación GOV-05/CTX-003 exige caché distribuida Redis")
	}
	return nil
}

