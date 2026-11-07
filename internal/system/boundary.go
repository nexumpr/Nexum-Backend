package system

import (
	"fmt"

	"github.com/nexumpr/Nexum-Backend/pkg/types"
)

// BoundaryValidator verifica que las operaciones y actores respeten los límites de A-000.
type BoundaryValidator struct{}

// NewBoundaryValidator crea una nueva instancia del validador de fronteras.
func NewBoundaryValidator() *BoundaryValidator {
	return &BoundaryValidator{}
}

// ValidateActor comprueba que el actor sea un actor válido según el mapa de contexto A-000 §3.
func (v *BoundaryValidator) ValidateActor(actor types.ActorType) error {
	if !actor.IsValid() {
		return fmt.Errorf("%w: '%s'", ErrInvalidActor, actor)
	}
	return nil
}

// ValidateFinancialAuthority asegura la regla R-031 y CTX-005: ningún cliente es autoridad
// de saldo, total, titularidad, rol, permiso o cuantía sin validación del servidor Backend.
func (v *BoundaryValidator) ValidateFinancialAuthority(actor types.ActorType, clientClaimsBalance bool) error {
	if err := v.ValidateActor(actor); err != nil {
		return err
	}

	if actor == types.ActorMobileClient && clientClaimsBalance {
		return ErrClientBalanceAuthority
	}

	return nil
}

// ValidateSystemTopology comprueba que la topología cumpla las obligaciones GOV-02..GOV-05.
func (v *BoundaryValidator) ValidateSystemTopology(meta types.SystemMetadata) error {
	if err := meta.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrBoundaryViolation, err)
	}

	if meta.Environment == types.EnvProduction && meta.MinNodeCount < 2 {
		return ErrInvalidNodeTopology
	}

	if meta.NoSQLProvider != "DynamoDB" {
		return fmt.Errorf("%w: proveedor actual '%s', se requiere 'DynamoDB'", ErrMissingPersistence, meta.NoSQLProvider)
	}

	if meta.CacheProvider != "Redis" {
		return fmt.Errorf("%w: proveedor actual '%s', se requiere 'Redis'", ErrMissingCache, meta.CacheProvider)
	}

	return nil
}
