package dynamo_test

import (
	"strings"
	"testing"
	"time"

	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
)

func TestClavesDeUsuarioYEntidades(t *testing.T) {
	if pk, sk := domainDynamo.UserKeys("u1"); pk != "USER#u1" || sk != "PROFILE" {
		t.Errorf("UserKeys inesperado: %q %q", pk, sk)
	}
	if pk, sk := domainDynamo.GoalKeys("u1", "g1"); pk != "USER#u1" || sk != "GOAL#g1" {
		t.Errorf("GoalKeys inesperado: %q %q", pk, sk)
	}
	if pk, sk := domainDynamo.RecurringKeys("u1", "r1"); pk != "USER#u1" || sk != "RECURRING#r1" {
		t.Errorf("RecurringKeys inesperado: %q %q", pk, sk)
	}
	if pk, sk := domainDynamo.BudgetKeys("u1", "2026-11", "food"); pk != "USER#u1" || sk != "BUDGET#2026-11#FOOD" {
		t.Errorf("BudgetKeys debe normalizar la categoría a mayúsculas: %q %q", pk, sk)
	}
	if pk, sk := domainDynamo.SessionKeys("abc123"); pk != "SESSION#abc123" || sk != "METADATA" {
		t.Errorf("SessionKeys inesperado: %q %q", pk, sk)
	}
}

func TestEmailReservationKeysNormalizaYHashea(t *testing.T) {
	pk, sk := domainDynamo.EmailReservationKeys("  Usuario@Nexum.COM ")
	if sk != "OWNER" {
		t.Errorf("SK esperada OWNER, obtenida %q", sk)
	}
	if !strings.HasPrefix(pk, "EMAIL#") {
		t.Errorf("PK debe empezar por EMAIL#: %q", pk)
	}
	hash := strings.TrimPrefix(pk, "EMAIL#")
	if len(hash) != 64 {
		t.Errorf("se espera SHA-256 hex de 64 caracteres, obtenido %d", len(hash))
	}
	normalizado, _ := domainDynamo.EmailReservationKeys("usuario@nexum.com")
	if pk != normalizado {
		t.Errorf("el email debe normalizarse (trim + minúsculas)")
	}
	if strings.Contains(pk, "@") || strings.Contains(strings.ToLower(pk), "nexum.com") {
		t.Errorf("la PK no debe contener PII: %q", pk)
	}
	otroPK, _ := domainDynamo.EmailReservationKeys("otro@nexum.com")
	if pk == otroPK {
		t.Errorf("emails distintos deben producir reservas distintas")
	}
}

func TestMovementKeysOrdenablesYGsi1(t *testing.T) {
	ocurrido := time.Date(2026, 11, 8, 6, 30, 0, 0, time.FixedZone("UTC-5", -5*3600))
	pk, sk, gsi1pk, gsi1sk := domainDynamo.MovementKeys("u1", "m1", "expense", ocurrido)
	if pk != "USER#u1" {
		t.Errorf("PK inesperada: %q", pk)
	}
	if sk != "MOVEMENT#2026-11-08T11:30:00.000Z#m1" {
		t.Errorf("SK debe usar UTC ordenable: %q", sk)
	}
	if gsi1pk != "USER#u1#TYPE#EXPENSE" {
		t.Errorf("GSI1PK debe normalizar el tipo: %q", gsi1pk)
	}
	if gsi1sk != "2026-11-08T11:30:00.000Z#m1" {
		t.Errorf("GSI1SK inesperada: %q", gsi1sk)
	}
}

func TestDynamoItemVersionInicialYSparse(t *testing.T) {
	item := domainDynamo.NewDynamoItem("USER#u1", "PROFILE", "user", nil)
	if item.Version != 1 {
		t.Errorf("version inicial esperada 1, obtenida %d", item.Version)
	}
	if item.Attributes == nil {
		t.Errorf("los atributos nil deben inicializarse a mapa vacío")
	}
	if item.GSI1PK != "" || item.GSI1SK != "" || item.TTL != 0 {
		t.Errorf("GSI1/TTL deben quedar vacíos por defecto (sparse)")
	}
	conGsi := item.WithGSI1("USER#u1#TYPE#EXPENSE", "2026-11-08#m1")
	if conGsi.GSI1PK == "" || conGsi.GSI1SK == "" {
		t.Errorf("WithGSI1 debe rellenar ambos atributos")
	}
	if item.GSI1PK != "" {
		t.Errorf("WithGSI1 no debe mutar el valor original")
	}
	conTTL := conGsi.WithTTL(1_800_000_000)
	if conTTL.TTL != 1_800_000_000 || conTTL.GSI1PK != conGsi.GSI1PK {
		t.Errorf("WithTTL debe conservar el resto de campos")
	}
	attrs := map[string]any{"saldo": int64(100)}
	conAttrs := domainDynamo.NewDynamoItem("PK", "SK", "movement", attrs)
	if conAttrs.Attributes["saldo"] != int64(100) {
		t.Errorf("los atributos aportados deben conservarse")
	}
}
