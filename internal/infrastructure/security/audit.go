package security

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	awsKeyRegex    = regexp.MustCompile(`(?i)(AKIA[0-9A-Z]{16})`)
	privateKeyRegex = regexp.MustCompile(`-----BEGIN (RSA |EC |DSA |OPENSSH )?PRIVATE KEY-----`)
	jwtTokenRegex   = regexp.MustCompile(`eyJ[A-Za-z0-9-_]+\.eyJ[A-Za-z0-9-_]+\.[A-Za-z0-9-_]+`)
)

// ErrHardcodedSecretDetected indica que se detectó un secreto o credencial cruda.
var ErrHardcodedSecretDetected = errors.New("hardcoded secret or sensitive credential detected")

// AuditTextForSecrets analiza un texto o archivo de configuración para verificar que no contenga
// secretos crudos (claves AWS, llaves privadas PEM o JWTs estáticos) (AC-A-100-02, GOV-07).
func AuditTextForSecrets(content string) error {
	if awsKeyRegex.MatchString(content) {
		return fmt.Errorf("%w: AWS Access Key detectada", ErrHardcodedSecretDetected)
	}
	if privateKeyRegex.MatchString(content) {
		return fmt.Errorf("%w: Llave privada PEM en texto claro detectada", ErrHardcodedSecretDetected)
	}
	// Si contiene JWT en código estático que no sea un marcador o redacted
	if jwtTokenRegex.MatchString(content) && !strings.Contains(content, "[REDACTED") && !strings.Contains(content, "test") {
		return fmt.Errorf("%w: Token JWT estático detectado", ErrHardcodedSecretDetected)
	}
	return nil
}

// ReleaseManifest certifica la integridad de un release y su aprobación humana (AC-A-100-03).
type ReleaseManifest struct {
	Version            string    `json:"version"`
	CommitHash         string    `json:"commit_hash"`
	ApprovedBy         string    `json:"approved_by"`
	ApprovalDate       time.Time `json:"approval_date"`
	OpenAPISpecsCount  int       `json:"openapi_specs_count"`
	TestsPassed        bool      `json:"tests_passed"`
	SecretsScanClean   bool      `json:"secrets_scan_clean"`
	RaceDetectionClean bool      `json:"race_detection_clean"`
}

// ValidateReleaseGate valida que el artefacto de release cuente con todas las condiciones
// de gobernanza, pruebas limpias y aprobación humana expresa (A-100 §2.8).
func ValidateReleaseGate(manifest ReleaseManifest) error {
	if strings.TrimSpace(manifest.Version) == "" {
		return errors.New("version de release requerida")
	}
	if strings.TrimSpace(manifest.CommitHash) == "" {
		return errors.New("commit hash de release requerido")
	}
	if strings.TrimSpace(manifest.ApprovedBy) == "" {
		return errors.New("aprobacion humana expresa obligatoria (GOV-01)")
	}
	if manifest.OpenAPISpecsCount != 75 {
		return fmt.Errorf("se requieren exactamente 75 operaciones OpenAPI 3.1, registradas %d", manifest.OpenAPISpecsCount)
	}
	if !manifest.TestsPassed {
		return errors.New("todas las pruebas unitarias y de arquitectura deben ser PASS")
	}
	if !manifest.SecretsScanClean {
		return errors.New("analisis estático de secretos no superado")
	}
	if !manifest.RaceDetectionClean {
		return errors.New("deteccion de carreras (race detector) no superada")
	}
	return nil
}

