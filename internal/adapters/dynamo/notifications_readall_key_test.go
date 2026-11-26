package dynamo

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	appNotifications "github.com/nexumpr/Nexum-Backend/internal/application/notifications"
)

// TestE063DueYClaimConservanLaClaveDeLaCola reproduce el defecto real de producción:
// la SK de la cola lleva nanosegundos, pero `createdAt` se persiste como epoch en
// segundos. Si el trabajo se reconstruye desde `createdAt`, `ClaimReadAll` busca una
// SK distinta, la condición `attribute_exists(PK)` falla y el trabajo queda pendiente
// para siempre con attempts=0 aunque la API responda 202.
func TestE063DueYClaimConservanLaClaveDeLaCola(t *testing.T) {
	cliente := nuevoE061Fake()
	creado := time.Date(2026, 11, 26, 17, 43, 23, 746644546, time.UTC)
	requestID := "a45e32ac-f1c6-441d-9656-f31604fc812c"
	sk := readAllQueueSortKey(creado, requestID)
	if sk != "REQ#2026-11-26T17:43:23.746644546Z#"+requestID {
		t.Fatalf("SK inesperada: %q", sk)
	}
	pk := ReadAllQueuePK
	cliente.items[claveItem(pk, sk)] = map[string]types.AttributeValue{
		"PK":         stringAttr(pk),
		"SK":         stringAttr(sk),
		"entityType": stringAttr(ReadAllQueueEntityType),
		"userId":     stringAttr("11111111-2222-4333-8444-555555555555"),
		"requestId":  stringAttr(requestID),
		// Producción guarda epoch en segundos: los nanosegundos no están aquí.
		"createdAt": numberAttr(creado.Unix()),
		"fence":     numberAttr(0),
		"attempts":  numberAttr(0),
	}

	store := NewNotificationsStore(cliente, "nexum-table")
	ctx := context.Background()

	jobs, err := store.DueReadAll(ctx, creado.Add(time.Minute), 25)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("DueReadAll: %d %v", len(jobs), err)
	}
	if !jobs[0].CreatedAt.Equal(creado) {
		t.Fatalf("CreatedAt=%s, esperado %s (la SK conserva los nanosegundos)",
			jobs[0].CreatedAt.Format(time.RFC3339Nano), creado.Format(time.RFC3339Nano))
	}

	reclamado, ok, err := store.ClaimReadAll(ctx, jobs[0], "nodo-a", creado.Add(time.Minute), 30*time.Second)
	if err != nil || !ok {
		t.Fatalf("ClaimReadAll debe reclamar el trabajo real: ok=%v err=%v", ok, err)
	}
	if reclamado.Attempts != 1 || reclamado.LeaseOwner != "nodo-a" {
		t.Fatalf("trabajo reclamado inesperado: attempts=%d owner=%q", reclamado.Attempts, reclamado.LeaseOwner)
	}
	item := cliente.items[claveItem(pk, sk)]
	if intentos, _ := attributeInt(item["attempts"]); intentos != 1 {
		t.Fatalf("attempts persistido=%d, esperado 1", intentos)
	}
}

// TestE063ReclamoDeItemSinSKParseableFallaCerrado fija que un item con SK corrupta no
// se reclama silenciosamente contra otra clave.
func TestE063ReclamoDeItemSinSKParseableFallaCerrado(t *testing.T) {
	item := map[string]types.AttributeValue{
		"PK":        stringAttr(ReadAllQueuePK),
		"SK":        stringAttr(ReadAllQueueSKPrefix + "sin-fecha#" + "req-1"),
		"userId":    stringAttr("u-1"),
		"requestId": stringAttr("req-1"),
		"createdAt": numberAttr(1795700000),
	}
	job, ok := readAllJobFromItem(item)
	if !ok || job.CreatedAt.IsZero() {
		t.Fatalf("el respaldo por createdAt debe permitir diagnosticar: %+v %v", job, ok)
	}
	// La clave reconstruida no coincide con la SK real: el claim sobre esa SK falla
	// cerrado y no toca otro item.
	if readAllQueueSortKey(job.CreatedAt, job.RequestID) == stringAttribute(item["SK"]) {
		t.Fatal("un SK no parseable no debe producir la misma clave")
	}
}

var _ = appNotifications.ReadAllJob{}
