package system

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// DecisionStatus define el estado de una decisión de arquitectura.
type DecisionStatus string

const (
	DecisionApproved DecisionStatus = "APPROVED"
	DecisionRejected DecisionStatus = "REJECTED"
	DecisionDraft    DecisionStatus = "DRAFT"
)

// DecisionRecord registra una decisión arquitectónica vinculante (A-110 §2.1, AC-A-110-01).
type DecisionRecord struct {
	ID            string         `json:"id"`
	SpecID        string         `json:"spec_id"`
	Title         string         `json:"title"`
	DecisionMaker string         `json:"decision_maker"`
	Date          time.Time      `json:"date"`
	Status        DecisionStatus `json:"status"`
	Rationale     string         `json:"rationale"`
	Impact        string         `json:"impact"`
}

// DecisionLog gestiona el historial auditable de decisiones.
type DecisionLog struct {
	mu        sync.RWMutex
	decisions map[string]DecisionRecord
}

// NewDecisionLog inicializa el registro de decisiones.
func NewDecisionLog() *DecisionLog {
	return &DecisionLog{
		decisions: make(map[string]DecisionRecord),
	}
}

// RecordDecision agrega una nueva decisión al registro auditable.
func (dl *DecisionLog) RecordDecision(rec DecisionRecord) error {
	dl.mu.Lock()
	defer dl.mu.Unlock()

	if strings.TrimSpace(rec.ID) == "" {
		return errors.New("id de decision obligatorio")
	}
	if strings.TrimSpace(rec.SpecID) == "" {
		return errors.New("spec_id obligatorio")
	}
	if strings.TrimSpace(rec.DecisionMaker) == "" {
		return errors.New("responsable humano obligatorio (GOV-01)")
	}

	dl.decisions[rec.ID] = rec
	return nil
}

// GetDecisionsBySpec retorna todas las decisiones asociadas a una spec particular.
func (dl *DecisionLog) GetDecisionsBySpec(specID string) []DecisionRecord {
	dl.mu.RLock()
	defer dl.mu.RUnlock()

	var results []DecisionRecord
	for _, d := range dl.decisions {
		if d.SpecID == specID {
			results = append(results, d)
		}
	}
	return results
}

// PRValidationRequest encapsula las condiciones para validar la integración de un Pull Request.
type PRValidationRequest struct {
	TargetBranch      string
	SourceBranch      string
	CIGreen           bool
	HumanApprovals    int
	DirectPushAttempt bool
	HasUnsavedSecrets bool
}

// ValidatePRMerge certifica las políticas de gates para ramas protegidas (A-110 §2.2, AC-A-110-02).
func ValidatePRMerge(req PRValidationRequest) error {
	if req.DirectPushAttempt && (req.TargetBranch == "main" || req.TargetBranch == "master") {
		return errors.New("push directo a rama principal protegido: requiere Pull Request formal")
	}
	if !req.CIGreen {
		return errors.New("el pipeline de integracion continua (CI) debe estar en estado GREEN (success)")
	}
	if req.HumanApprovals < 1 {
		return errors.New("se requiere al menos una aprobacion humana formal para fusionar el PR (GOV-01)")
	}
	if req.HasUnsavedSecrets {
		return errors.New("el escaneo de seguridad detecto secretos o credenciales crudas en el PR")
	}
	return nil
}

// DeployAuthorization encapsula la autorización humana expresa desacoplada para despliegues.
type DeployAuthorization struct {
	Environment       string
	AuthorizedBy      string
	SeparateFromPR    bool
	AuthorizationDate time.Time
}

// ValidateDeployAuthorization certifica que el despliegue requiere una aprobación humana separada (AC-A-110-02).
func ValidateDeployAuthorization(auth DeployAuthorization) error {
	if strings.TrimSpace(auth.AuthorizedBy) == "" {
		return errors.New("autorizacion de despliegue requiere aprobador humano expreso (GOV-01)")
	}
	if !auth.SeparateFromPR {
		return errors.New("la autorizacion de despliegue debe estar desacoplada del merge del PR (A-110 §2.2)")
	}
	if auth.Environment == "production" && time.Since(auth.AuthorizationDate) > 24*time.Hour {
		return errors.New("autorizacion de despliegue expirada: vigencia maxima de 24 horas")
	}
	return nil
}

// RFCRecord representa una solicitud formal de cambio sobre arquitectura o specs aprobadas (A-110 §2.4, AC-A-110-03).
type RFCRecord struct {
	ID             string
	TargetSpecID   string
	Title          string
	Author         string
	Status         string // "SUBMITTED", "APPROVED", "REJECTED"
	ImpactAnalysis string
	ApprovedBy     string
}

// RFCManager gestiona el ciclo de vida de los cambios a especificaciones aprobadas.
type RFCManager struct {
	mu   sync.RWMutex
	rfcs map[string]RFCRecord
}

// NewRFCManager inicializa el gestor de RFCs.
func NewRFCManager() *RFCManager {
	return &RFCManager{
		rfcs: make(map[string]RFCRecord),
	}
}

// SubmitRFC registra una nueva solicitud de cambio.
func (rm *RFCManager) SubmitRFC(rfc RFCRecord) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if strings.TrimSpace(rfc.ID) == "" {
		return errors.New("id de RFC requerido")
	}
	if strings.TrimSpace(rfc.TargetSpecID) == "" {
		return errors.New("spec_id objetivo requerido")
	}
	if strings.TrimSpace(rfc.ImpactAnalysis) == "" {
		return errors.New("analisis de impacto obligatorio para abrir un RFC")
	}

	rfc.Status = "SUBMITTED"
	rm.rfcs[rfc.ID] = rfc
	return nil
}

// ApproveRFC aprueba un RFC con firma humana expresa.
func (rm *RFCManager) ApproveRFC(rfcID, approver string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rfc, exists := rm.rfcs[rfcID]
	if !exists {
		return errors.New("RFC no encontrado")
	}
	if strings.TrimSpace(approver) == "" {
		return errors.New("aprobador humano requerido para autorizar RFC")
	}

	rfc.Status = "APPROVED"
	rfc.ApprovedBy = approver
	rm.rfcs[rfcID] = rfc
	return nil
}

// ValidateSpecChange verifica que cualquier cambio sobre una spec aprobada cuente con RFC formal aprobado (AC-A-110-03).
func (rm *RFCManager) ValidateSpecChange(specID, rfcID string) error {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	if rfcID == "" {
		return fmt.Errorf("modificacion a spec %s rechazada: requiere RFC formal previo (AC-A-110-03)", specID)
	}

	rfc, exists := rm.rfcs[rfcID]
	if !exists || rfc.TargetSpecID != specID || rfc.Status != "APPROVED" {
		return fmt.Errorf("modificacion a spec %s no autorizada: RFC %s no esta en estado APPROVED", specID, rfcID)
	}

	return nil
}
