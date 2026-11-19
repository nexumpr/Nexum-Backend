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

// TestUserIdempotencyKeysAislaPorUsuario cubre el hallazgo F-02 del Gate de Oleada 2:
// dos cuentas con la misma Idempotency-Key y la misma operación NO pueden compartir
// registro, y la clave cruda no se persiste.
func TestUserIdempotencyKeysAislaPorUsuario(t *testing.T) {
	pkA, skA := domainDynamo.UserIdempotencyKeys("user-a", "E028_CREATE_MOVEMENT", "0123456789abcdef")
	pkB, skB := domainDynamo.UserIdempotencyKeys("user-b", "E028_CREATE_MOVEMENT", "0123456789abcdef")

	if pkA == pkB {
		t.Fatal("dos usuarios con la misma clave no pueden compartir particion")
	}
	if pkA != "USER#user-a" || pkB != "USER#user-b" {
		t.Fatalf("particion por usuario inesperada: %q / %q", pkA, pkB)
	}
	if skA != skB {
		t.Fatal("el SK depende de operacion y clave, no del usuario (ya aislado por PK)")
	}
	if !strings.HasPrefix(skA, domainDynamo.PrefixIdempotency) {
		t.Fatalf("prefijo de idempotencia inesperado: %q", skA)
	}
	if strings.Contains(skA, "0123456789abcdef") {
		t.Fatal("la Idempotency-Key cruda no debe aparecer en el SK")
	}
}

func TestUserIdempotencyKeysEsDeterministaYDistingueOperacion(t *testing.T) {
	pk1, sk1 := domainDynamo.UserIdempotencyKeys("user-a", "E028_CREATE_MOVEMENT", "0123456789abcdef")
	pk2, sk2 := domainDynamo.UserIdempotencyKeys("user-a", "E028_CREATE_MOVEMENT", "0123456789abcdef")
	_, skOtraOp := domainDynamo.UserIdempotencyKeys("user-a", "E033_CREATE_RECURRING", "0123456789abcdef")
	_, skOtraClave := domainDynamo.UserIdempotencyKeys("user-a", "E028_CREATE_MOVEMENT", "fedcba9876543210")

	if pk1 != pk2 || sk1 != sk2 {
		t.Fatal("la clave debe ser determinista para el mismo usuario, operacion y clave")
	}
	if sk1 == skOtraOp {
		t.Fatal("la operacion debe formar parte del SK")
	}
	if sk1 == skOtraClave {
		t.Fatal("la Idempotency-Key debe formar parte del SK")
	}
}

// TestUserIdempotencyKeysNoComparteConLaHeredada verifica que la funcion nueva no
// reutiliza el espacio de nombres de E001: ambas pueden coexistir sin colisionar.
func TestUserIdempotencyKeysNoComparteConLaHeredada(t *testing.T) {
	legacyPK, _ := domainDynamo.IdempotencyKeys("E001_REGISTER", "0123456789abcdef")
	nuevaPK, _ := domainDynamo.UserIdempotencyKeys("user-a", "E001_REGISTER", "0123456789abcdef")

	if legacyPK == nuevaPK {
		t.Fatal("la clave heredada y la nueva no pueden colisionar")
	}
	if !strings.HasPrefix(legacyPK, domainDynamo.PrefixIdempotency) {
		t.Fatalf("se altero la clave heredada de E001: %q", legacyPK)
	}
}
