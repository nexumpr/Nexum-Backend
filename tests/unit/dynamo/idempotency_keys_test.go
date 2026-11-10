package dynamo_test

import (
	"strings"
	"testing"

	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
)

func TestIdempotencyKeysAreDeterministicAndDoNotLeakRawKey(t *testing.T) {
	pk1, sk1 := domainDynamo.IdempotencyKeys("E001_REGISTER", "0123456789abcdef")
	pk2, sk2 := domainDynamo.IdempotencyKeys("E001_REGISTER", "0123456789abcdef")

	if pk1 != pk2 || sk1 != sk2 {
		t.Fatal("las claves de idempotencia deben ser deterministas")
	}
	if !strings.HasPrefix(pk1, domainDynamo.PrefixIdempotency) {
		t.Fatalf("prefijo inesperado: %q", pk1)
	}
	if strings.Contains(pk1, "0123456789abcdef") {
		t.Fatal("la clave cruda del cliente no debe aparecer en la PK")
	}
	if sk1 != "RECORD" {
		t.Fatalf("sort key inesperada: %q", sk1)
	}
}

func TestIdempotencyKeysDifferByOperationAndKey(t *testing.T) {
	basePK, _ := domainDynamo.IdempotencyKeys("E001_REGISTER", "0123456789abcdef")
	otherOpPK, _ := domainDynamo.IdempotencyKeys("E002_LOGIN", "0123456789abcdef")
	otherKeyPK, _ := domainDynamo.IdempotencyKeys("E001_REGISTER", "fedcba9876543210")

	if basePK == otherOpPK {
		t.Fatal("la operacion debe formar parte de la clave")
	}
	if basePK == otherKeyPK {
		t.Fatal("la Idempotency-Key debe formar parte de la clave")
	}
}
