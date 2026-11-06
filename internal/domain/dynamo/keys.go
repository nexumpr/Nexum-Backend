package dynamo

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// Constantes de prefijos canónicos de Single-Table Design (A-060 §2.2, §2.3).
const (
	PrefixUser      = "USER#"
	PrefixEmail     = "EMAIL#"
	PrefixProfile   = "PROFILE"
	PrefixMovement  = "MOVEMENT#"
	PrefixGoal      = "GOAL#"
	PrefixRecurring = "RECURRING#"
	PrefixBudget    = "BUDGET#"
	PrefixSession   = "SESSION#"
	PrefixOwner     = "OWNER"
	TypePrefix      = "#TYPE#"
)

// UserKeys genera PK y SK para el perfil del usuario.
func UserKeys(userID string) (pk, sk string) {
	return PrefixUser + userID, PrefixProfile
}

// EmailReservationKeys genera PK y SK para garantizar la unicidad de email mediante item reservado (A-060 §2.2).
// El email se normaliza (minúsculas, trim) y se hashea con SHA-256 para evitar PII en claves primarias.
func EmailReservationKeys(email string) (pk, sk string) {
	normalized := strings.TrimSpace(strings.ToLower(email))
	hash := sha256.Sum256([]byte(normalized))
	hashHex := hex.EncodeToString(hash[:])
	return PrefixEmail + hashHex, PrefixOwner
}

// MovementKeys genera las claves compuestas para un movimiento financiero (A-060 §2.3, §2.4):
// PK: USER#<id>
// SK: MOVEMENT#<RFC3339Sortable>#<movementID>
// GSI1PK: USER#<id>#TYPE#<type>
// GSI1SK: <RFC3339Sortable>#<movementID>
func MovementKeys(userID, movementID, movementType string, occurredAt time.Time) (pk, sk, gsi1pk, gsi1sk string) {
	// RFC3339 con formato seguro ordenable alfanuméricamente
	sortableTime := occurredAt.UTC().Format("2006-01-02T15:04:05.000Z")
	pk = PrefixUser + userID
	sk = fmt.Sprintf("%s%s#%s", PrefixMovement, sortableTime, movementID)
	gsi1pk = fmt.Sprintf("%s%s%s%s", PrefixUser, userID, TypePrefix, strings.ToUpper(movementType))
	gsi1sk = fmt.Sprintf("%s#%s", sortableTime, movementID)
	return pk, sk, gsi1pk, gsi1sk
}

// GoalKeys genera PK y SK para una meta de ahorro.
func GoalKeys(userID, goalID string) (pk, sk string) {
	return PrefixUser + userID, PrefixGoal + goalID
}

// RecurringKeys genera PK y SK para un pago recurrente programado.
func RecurringKeys(userID, recurringID string) (pk, sk string) {
	return PrefixUser + userID, PrefixRecurring + recurringID
}

// BudgetKeys genera PK y SK para un presupuesto por periodo y categoría.
func BudgetKeys(userID, period, category string) (pk, sk string) {
	return PrefixUser + userID, fmt.Sprintf("%s%s#%s", PrefixBudget, period, strings.ToUpper(category))
}

// SessionKeys genera PK y SK para una sesión persistida por hash de token.
func SessionKeys(tokenHash string) (pk, sk string) {
	return PrefixSession + tokenHash, "METADATA"
}

