package system_test

import (
	"testing"
	"time"

	"github.com/nexumpr/Nexum-Backend/internal/system"
	"github.com/nexumpr/Nexum-Backend/pkg/types"
)

// TestSemVerFormatAndDecisionLogAccessibility valida AC-A-110-01:
// Versionamiento semántico del backend, prefijo /api/v1 e historial accesible de decisiones.
func TestSemVerFormatAndDecisionLogAccessibility(t *testing.T) {
	// 1. Parsing y formateo SemVer
	validVersions := []string{"0.8.0", "1.0.0-alpha.1", "2.1.3+20261106"}
	for _, v := range validVersions {
		sem, err := types.ParseSemVer(v)
		if err != nil {
			t.Fatalf("Error al parsear SemVer válido '%s': %v", v, err)
		}
		if sem.String() != v {
			t.Errorf("Formateo SemVer discrepante: esperaba '%s', obtuvo '%s'", v, sem.String())
		}
	}

	invalidVersions := []string{"invalid", "v1", "1.0", "latest"}
	for _, inv := range invalidVersions {
		if _, err := types.ParseSemVer(inv); err == nil {
			t.Errorf("Esperaba error al parsear versión inválida '%s'", inv)
		}
	}

	// 2. Validación de prefijo /api/v1
	if err := types.ValidateAPIRoute("/api/v1/movements"); err != nil {
		t.Errorf("Ruta legítima /api/v1/movements arrojó error: %v", err)
	}
	if err := types.ValidateAPIRoute("/unversioned/movements"); err == nil {
		t.Error("Ruta sin versionar debió arrojar error")
	}

	// 3. Registro auditable de decisiones de arquitectura
	decLog := system.NewDecisionLog()
	rec := system.DecisionRecord{
		ID:            "DEC-A000-01",
		SpecID:        "A-000",
		Title:         "Adopción de AWS, DynamoDB y Redis",
		DecisionMaker: "Operador Humano Responsable (H03/H04)",
		Date:          time.Now(),
		Status:        system.DecisionApproved,
		Rationale:     "Reemplazo formal del prototipo docente conservando contratos",
		Impact:        "Afecta topología completa de persistencia y caché",
	}

	if err := decLog.RecordDecision(rec); err != nil {
		t.Fatalf("Error al registrar decisión: %v", err)
	}

	decisions := decLog.GetDecisionsBySpec("A-000")
	if len(decisions) != 1 || decisions[0].ID != "DEC-A000-01" {
		t.Errorf("Historial de decisiones no accesible para A-000: %+v", decisions)
	}

	t.Logf("AC-A-110-01 Verificado: SemVer, /api/v1 e historial auditable de decisiones accesibles.")
}

// TestPRGatePolicyAndDeployDecoupling valida AC-A-110-02:
// Cada PR con CI verde y aprobación humana; autorización de despliegue separada y desacoplada.
func TestPRGatePolicyAndDeployDecoupling(t *testing.T) {
	// Caso 1: Push directo a main denegado
	err := system.ValidatePRMerge(system.PRValidationRequest{
		TargetBranch:      "main",
		SourceBranch:      "Arquitectura_Lizeth",
		CIGreen:           true,
		HumanApprovals:    1,
		DirectPushAttempt: true,
	})
	if err == nil {
		t.Fatal("Esperaba rechazo por intento de push directo a rama protegida main")
	}

	// Caso 2: CI no verde denegado
	err = system.ValidatePRMerge(system.PRValidationRequest{
		TargetBranch:   "main",
		SourceBranch:   "Arquitectura_Lizeth",
		CIGreen:        false,
		HumanApprovals: 1,
	})
	if err == nil {
		t.Fatal("Esperaba rechazo por CI en estado failing")
	}

	// Caso 3: Sin aprobación humana denegado (GOV-01)
	err = system.ValidatePRMerge(system.PRValidationRequest{
		TargetBranch:   "main",
		SourceBranch:   "Arquitectura_Lizeth",
		CIGreen:        true,
		HumanApprovals: 0,
	})
	if err == nil {
		t.Fatal("Esperaba rechazo por ausencia de aprobación humana formal")
	}

	// Caso 4: PR conforme aceptado
	err = system.ValidatePRMerge(system.PRValidationRequest{
		TargetBranch:   "main",
		SourceBranch:   "Arquitectura_Lizeth",
		CIGreen:        true,
		HumanApprovals: 1,
	})
	if err != nil {
		t.Fatalf("PR legítimo debió ser aceptado: %v", err)
	}

	// 5. Desacoplamiento de autorización de despliegue
	deployAuth := system.DeployAuthorization{
		Environment:        "production",
		AuthorizedBy:       "Responsable Técnico Humano",
		SeparateFromPR:     true,
		AuthorizationDate:  time.Now(),
	}
	if err := system.ValidateDeployAuthorization(deployAuth); err != nil {
		t.Fatalf("Autorización de despliegue desacoplada debió ser aceptada: %v", err)
	}

	// Caso negativo: Despliegue dependiente automáticamente del merge sin confirmación separada
	deployAuthCoupled := deployAuth
	deployAuthCoupled.SeparateFromPR = false
	if err := system.ValidateDeployAuthorization(deployAuthCoupled); err == nil {
		t.Fatal("Esperaba rechazo por acoplamiento de despliegue al merge del PR")
	}

	t.Logf("AC-A-110-02 Verificado: Gates de PR protegidos y autorización de deploy desacoplada.")
}

// TestRFCChangeControlProcess valida AC-A-110-03:
// Modificaciones a especificaciones aprobadas exigen RFC y revisión formal humana.
func TestRFCChangeControlProcess(t *testing.T) {
	rfcMgr := system.NewRFCManager()

	// 1. Intento de modificar spec sin RFC previo debe ser bloqueado
	err := rfcMgr.ValidateSpecChange("A-000", "")
	if err == nil {
		t.Fatal("Esperaba rechazo de modificación sin RFC previo (AC-A-110-03)")
	}

	// 2. Registrar RFC formal con análisis de impacto
	rfc := system.RFCRecord{
		ID:             "RFC-001",
		TargetSpecID:   "A-000",
		Title:          "Apertura de permisos limitados Go en H08-A",
		Author:         "Nexum Architect",
		ImpactAnalysis: "Habilita compilación local de Go sin tocar AWS ni Docker",
	}

	if err := rfcMgr.SubmitRFC(rfc); err != nil {
		t.Fatalf("Error al someter RFC: %v", err)
	}

	// 3. RFC recién sometido no autoriza cambio antes de aprobación
	if err := rfcMgr.ValidateSpecChange("A-000", "RFC-001"); err == nil {
		t.Fatal("RFC en estado SUBMITTED no debe autorizar modificaciones a specs")
	}

	// 4. Aprobación formal humana
	if err := rfcMgr.ApproveRFC("RFC-001", "Operador Humano Responsable"); err != nil {
		t.Fatalf("Error al aprobar RFC: %v", err)
	}

	// 5. Spec autorizada tras aprobación del RFC
	if err := rfcMgr.ValidateSpecChange("A-000", "RFC-001"); err != nil {
		t.Fatalf("Modificación debió ser autorizada con RFC aprobado: %v", err)
	}

	t.Logf("AC-A-110-03 Verificado: Control formal de cambios mediante RFC con revisión humana.")
}
