package system

import (
	"fmt"

	"github.com/nexumpr/Nexum-Backend/pkg/types"
)

// SystemContext encapsula el contexto y los límites de arquitectura del sistema Nexum-Backend.
type SystemContext struct {
	Metadata  types.SystemMetadata
	Validator *BoundaryValidator
}

// NewSystemContext inicializa un nuevo contexto de sistema validando sus invariantes arquitectónicas.
func NewSystemContext(meta types.SystemMetadata) (*SystemContext, error) {
	validator := NewBoundaryValidator()
	if err := validator.ValidateSystemTopology(meta); err != nil {
		return nil, fmt.Errorf("fallo al inicializar SystemContext: %w", err)
	}

	return &SystemContext{
		Metadata:  meta,
		Validator: validator,
	}, nil
}

// DefaultLocalMetadata genera la metadata por defecto para ejecución y pruebas en entorno local.
func DefaultLocalMetadata() types.SystemMetadata {
	return types.SystemMetadata{
		AppName:       "Nexum-Backend",
		Version:       "0.1.0-alpha",
		SpecVersion:   "A-000-v0.1.1",
		Environment:   types.EnvLocal,
		MinNodeCount:  1,
		TLSRequired:   false,
		NoSQLProvider: "DynamoDB",
		CacheProvider: "Redis",
	}
}

// DefaultProductionMetadata genera la configuración para entorno de producción con cumplimiento estricto.
func DefaultProductionMetadata() types.SystemMetadata {
	return types.SystemMetadata{
		AppName:       "Nexum-Backend",
		Version:       "1.0.0",
		SpecVersion:   "A-000-v0.1.1",
		Environment:   types.EnvProduction,
		MinNodeCount:  2,
		TLSRequired:   true,
		NoSQLProvider: "DynamoDB",
		CacheProvider: "Redis",
	}
}

