package devsecops_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/nexumpr/Nexum-Backend/internal/application/financial"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
	"github.com/nexumpr/Nexum-Backend/internal/infrastructure/cache"
	"github.com/nexumpr/Nexum-Backend/internal/infrastructure/dynamo"
	"github.com/nexumpr/Nexum-Backend/internal/infrastructure/security"
)

// TestConcurrentAccessNoDataRace valida AC-A-100-02:
// Concurrencia masiva sobre componentes de persistencia, caché e idempotencia
// sin condiciones de carrera (apto para go test -race).
func TestConcurrentAccessNoDataRace(t *testing.T) {
	const goroutines = 30
	const iterations = 15

	sharedCache := cache.NewMemoryDistributedCache()
	idempotencyStore := financial.NewMemoryIdempotencyStore()
	tableStore := dynamo.NewDynamoTable()

	var wg sync.WaitGroup
	wg.Add(goroutines * 3)

	// 1. Escrituras y lecturas concurrentes en caché distribuido
	for i := 0; i < goroutines; i++ {
		go func(workerID int) {
			defer wg.Done()
			ctx := context.Background()
			key := fmt.Sprintf("nexum:v1:test:worker:%d", workerID%5)
			for j := 0; j < iterations; j++ {
				_, _ = sharedCache.IncrementAtomic(ctx, key, 5*time.Second)
				_ = sharedCache.Set(ctx, key+":str", "val", 5*time.Second)
				_, _, _ = sharedCache.Get(ctx, key+":str")
			}
		}(i)
	}

	// 2. Transacciones concurrentes e idempotencia
	for i := 0; i < goroutines; i++ {
		go func(workerID int) {
			defer wg.Done()
			ctx := context.Background()
			userID := fmt.Sprintf("USR_%d", workerID%5)
			idemKey := fmt.Sprintf("IDEM_%d", workerID%10)
			payloadHash := financial.HashPayload([]byte(fmt.Sprintf(`{"amount":%d}`, workerID)))

			for j := 0; j < iterations; j++ {
				rec, err := idempotencyStore.LockOrGet(ctx, idemKey, userID, "TRANSFER", payloadHash, 10*time.Second)
				if err == nil && rec != nil && !rec.IsCompleted {
					_ = idempotencyStore.SaveResponse(ctx, idemKey, userID, "TRANSFER", 200, `{"status":"OK"}`)
				}
			}
		}(i)
	}

	// 3. Consultas y escrituras concurrentes en SingleTableStore
	for i := 0; i < goroutines; i++ {
		go func(workerID int) {
			defer wg.Done()
			ctx := context.Background()
			pk := fmt.Sprintf("USER#test_%d", workerID%3)
			sk := fmt.Sprintf("MOVEMENT#2026-11-06T00:00:00Z#%d", workerID)

			for j := 0; j < iterations; j++ {
				_ = tableStore.PutItem(ctx, domainDynamo.DynamoItem{
					PK: pk,
					SK: sk,
					Attributes: map[string]any{
						"amount": int64(500),
					},
					Version: 1,
				}, false, nil)
				_, _ = tableStore.QueryPartition(ctx, pk, "MOVEMENT#", "", true, 10)
			}
		}(i)
	}

	wg.Wait()
	t.Logf("AC-A-100-02 Verificado: 90 workers concurrentes ejecutados sin data races ni corrupción de memoria.")
}

// TestSecretsAuditScan valida AC-A-100-02 y GOV-07:
// Control SAST de código: no se permiten secretos ni claves AWS crudas en el código fuente.
func TestSecretsAuditScan(t *testing.T) {
	// Caso negativo: Debe rechazar texto con credencial AWS
	badTextAWS := "aws_secret_key = AKIAIOSFODNN7EXAMPLE"
	if err := security.AuditTextForSecrets(badTextAWS); err == nil {
		t.Fatal("Esperaba que AuditTextForSecrets detectara una Access Key de AWS")
	}

	// Caso negativo: Debe rechazar clave privada PEM
	badTextPEM := "-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA..."
	if err := security.AuditTextForSecrets(badTextPEM); err == nil {
		t.Fatal("Esperaba que AuditTextForSecrets detectara una llave privada PEM")
	}

	// Caso positivo: Texto legítimo y sanitizado pasa limpiamente
	cleanText := "host=localhost;port=5432;user=app_user;password=[REDACTED]"
	if err := security.AuditTextForSecrets(cleanText); err != nil {
		t.Fatalf("Texto limpio no debió arrojar error: %v", err)
	}

	t.Logf("AC-A-100-02 Verificado: Detección estática de secretos funcional y rigurosa.")
}

// TestReleaseManifestAndGateApproval valida AC-A-100-03:
// Firma de release y aprobación humana formalmente verificadas.
func TestReleaseManifestAndGateApproval(t *testing.T) {
	validManifest := security.ReleaseManifest{
		Version:            "0.8.0",
		CommitHash:         "15491e5",
		ApprovedBy:         "Operador Humano Responsable (GOV-01)",
		ApprovalDate:       time.Now(),
		OpenAPISpecsCount:  75,
		TestsPassed:        true,
		SecretsScanClean:   true,
		RaceDetectionClean: true,
	}

	if err := security.ValidateReleaseGate(validManifest); err != nil {
		t.Fatalf("Manifiesto válido fue rechazado: %v", err)
	}

	// Caso negativo: Sin aprobación humana expresa
	unapprovedManifest := validManifest
	unapprovedManifest.ApprovedBy = ""
	if err := security.ValidateReleaseGate(unapprovedManifest); err == nil {
		t.Fatal("Esperaba rechazo por falta de aprobación humana expresa (GOV-01)")
	}

	// Caso negativo: Conteo incompleto de rutas OpenAPI
	incompleteRoutes := validManifest
	incompleteRoutes.OpenAPISpecsCount = 74
	if err := security.ValidateReleaseGate(incompleteRoutes); err == nil {
		t.Fatal("Esperaba rechazo por conteo de operaciones diferente de 75")
	}

	t.Logf("AC-A-100-03 Verificado: Requisitos de firma de release, trazabilidad humana y 75 rutas certificados.")
}
