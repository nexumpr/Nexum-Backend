package system_test

import (
	"errors"
	"testing"

	"github.com/nexumpr/Nexum-Backend/internal/system"
	"github.com/nexumpr/Nexum-Backend/pkg/types"
)

func TestActorValidation(t *testing.T) {
	v := system.NewBoundaryValidator()

	validActors := []types.ActorType{
		types.ActorUser,
		types.ActorMobileClient,
		types.ActorSystemOperator,
		types.ActorInfrastructure,
	}

	for _, actor := range validActors {
		if err := v.ValidateActor(actor); err != nil {
			t.Errorf("actor esperado como válido '%s' devolvió error: %v", actor, err)
		}
	}

	invalidActors := []types.ActorType{
		"ADMIN_INVENTADO",
		"SUPERUSER",
		"",
		"GUEST",
	}

	for _, actor := range invalidActors {
		err := v.ValidateActor(actor)
		if err == nil {
			t.Errorf("actor inválido '%s' fue erróneamente aceptado", actor)
		}
		if !errors.Is(err, system.ErrInvalidActor) {
			t.Errorf("se esperaba ErrInvalidActor para '%s', se obtuvo: %v", actor, err)
		}
	}
}

func TestFinancialAuthorityBoundary(t *testing.T) {
	v := system.NewBoundaryValidator()

	// Caso 1: Cliente móvil reporta intención de fijar saldo unilateralmente -> DEBE FALLAR (R-031, CTX-005)
	err := v.ValidateFinancialAuthority(types.ActorMobileClient, true)
	if err == nil {
		t.Fatal("se esperaba rechazo al intentar que el cliente móvil declare autoridad de saldo")
	}
	if !errors.Is(err, system.ErrClientBalanceAuthority) {
		t.Fatalf("se esperaba ErrClientBalanceAuthority, se obtuvo: %v", err)
	}

	// Caso 2: Cliente móvil envía petición normal sin atribuirse cálculo de balance -> VÁLIDO
	if err := v.ValidateFinancialAuthority(types.ActorMobileClient, false); err != nil {
		t.Fatalf("petición normal de cliente móvil no debió fallar: %v", err)
	}

	// Caso 3: Usuario interactuando normalmente -> VÁLIDO
	if err := v.ValidateFinancialAuthority(types.ActorUser, false); err != nil {
		t.Fatalf("interacción de usuario no debió fallar: %v", err)
	}
}

func TestSystemTopologyValidation(t *testing.T) {
	v := system.NewBoundaryValidator()

	t.Run("Entorno local válido", func(t *testing.T) {
		meta := system.DefaultLocalMetadata()
		if err := v.ValidateSystemTopology(meta); err != nil {
			t.Fatalf("configuración local válida falló: %v", err)
		}
	})

	t.Run("Entorno producción válido (GOV-02..05)", func(t *testing.T) {
		meta := system.DefaultProductionMetadata()
		if err := v.ValidateSystemTopology(meta); err != nil {
			t.Fatalf("configuración de producción válida falló: %v", err)
		}
	})

	t.Run("Producción con menos de 2 nodos viola GOV-02 y CTX-002", func(t *testing.T) {
		meta := system.DefaultProductionMetadata()
		meta.MinNodeCount = 1
		err := v.ValidateSystemTopology(meta)
		if err == nil {
			t.Fatal("se esperaba error de topología al configurar 1 solo nodo en producción")
		}
		if !errors.Is(err, system.ErrBoundaryViolation) && !errors.Is(err, system.ErrInvalidNodeTopology) {
			t.Fatalf("error inesperado: %v", err)
		}
	})

	t.Run("Producción sin TLS obligatorio viola GOV-03 y CTX-002", func(t *testing.T) {
		meta := system.DefaultProductionMetadata()
		meta.TLSRequired = false
		err := v.ValidateSystemTopology(meta)
		if err == nil {
			t.Fatal("se esperaba error al desactivar TLS en producción")
		}
		if !errors.Is(err, system.ErrBoundaryViolation) {
			t.Fatalf("se esperaba ErrBoundaryViolation, se obtuvo: %v", err)
		}
	})

	t.Run("Proveedor de persistencia distinto a DynamoDB viola GOV-04 y CTX-003", func(t *testing.T) {
		meta := system.DefaultLocalMetadata()
		meta.NoSQLProvider = "MongoDB" // proveedor histórico rechazado en favor de DynamoDB
		err := v.ValidateSystemTopology(meta)
		if err == nil {
			t.Fatal("se esperaba error al usar proveedor no autorizado de persistencia")
		}
		if !errors.Is(err, system.ErrMissingPersistence) {
			t.Fatalf("se esperaba ErrMissingPersistence, se obtuvo: %v", err)
		}
	})

	t.Run("Proveedor de caché distinto a Redis viola GOV-05 y CTX-003", func(t *testing.T) {
		meta := system.DefaultLocalMetadata()
		meta.CacheProvider = "Memcached"
		err := v.ValidateSystemTopology(meta)
		if err == nil {
			t.Fatal("se esperaba error al usar proveedor no autorizado de caché")
		}
		if !errors.Is(err, system.ErrMissingCache) {
			t.Fatalf("se esperaba ErrMissingCache, se obtuvo: %v", err)
		}
	})
}

func TestNewSystemContext(t *testing.T) {
	t.Run("Inicialización exitosa con metadata válida", func(t *testing.T) {
		meta := system.DefaultLocalMetadata()
		ctx, err := system.NewSystemContext(meta)
		if err != nil {
			t.Fatalf("error inesperado al inicializar contexto: %v", err)
		}
		if ctx == nil {
			t.Fatal("contexto inicializado no debe ser nil")
		}
		if ctx.Metadata.AppName != "Nexum-Backend" {
			t.Errorf("nombre de app esperado 'Nexum-Backend', se obtuvo '%s'", ctx.Metadata.AppName)
		}
	})

	t.Run("Fallo de inicialización con metadata inválida", func(t *testing.T) {
		meta := system.DefaultLocalMetadata()
		meta.AppName = "" // inválido
		ctx, err := system.NewSystemContext(meta)
		if err == nil {
			t.Fatal("se esperaba error al inicializar con AppName vacío")
		}
		if ctx != nil {
			t.Fatal("contexto debe ser nil si falla la validación")
		}
	})
}
