package dynamo

import (
	"context"
	"errors"
	"fmt"
	"time"

	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
	infraDynamo "github.com/nexumpr/Nexum-Backend/internal/infrastructure/dynamo"
)

var (
	// ErrEmailAlreadyExists se retorna si un correo ya se encuentra registrado.
	ErrEmailAlreadyExists = errors.New("el correo electronico ya esta registrado")
)

// DynamoRepository proporciona operaciones de alto nivel sobre la tabla NEXUM_CORE.
type DynamoRepository struct {
	table *infraDynamo.DynamoTable
}

// NewDynamoRepository crea un nuevo repositorio sobre la tabla provista.
func NewDynamoRepository(table *infraDynamo.DynamoTable) *DynamoRepository {
	return &DynamoRepository{table: table}
}

// RegisterUserAtomic registra el usuario y reserva la unicidad del email en una sola transacción atómica (A-060 §2.2).
func (r *DynamoRepository) RegisterUserAtomic(ctx context.Context, userID, email, name, passwordHash string) error {
	userPK, userSK := domainDynamo.UserKeys(userID)
	emailPK, emailSK := domainDynamo.EmailReservationKeys(email)

	userItem := domainDynamo.NewDynamoItem(userPK, userSK, "USER_PROFILE", map[string]any{
		"userId":       userID,
		"email":        email,
		"name":         name,
		"passwordHash": passwordHash,
		"status":       "ACTIVE",
	})

	emailItem := domainDynamo.NewDynamoItem(emailPK, emailSK, "EMAIL_RESERVATION", map[string]any{
		"userId": userID,
		"email":  email,
	})

	actions := []infraDynamo.TransactAction{
		// 1. Reservar email: debe fallar si ya existe
		{
			Type:         infraDynamo.ActionPut,
			PK:           emailPK,
			SK:           emailSK,
			Item:         &emailItem,
			MustNotExist: true,
		},
		// 2. Crear perfil del usuario
		{
			Type:         infraDynamo.ActionPut,
			PK:           userPK,
			SK:           userSK,
			Item:         &userItem,
			MustNotExist: true,
		},
	}

	err := r.table.TransactWriteItems(ctx, actions)
	if errors.Is(err, infraDynamo.ErrTransactionCanceled) {
		return ErrEmailAlreadyExists
	}
	return err
}

// RecordMovementAtomic registra un movimiento financiero y opcionalmente actualiza una meta de forma atómica (A-060 §2.5, §2.6).
// Si la meta tiene un fallo de condición (ej: versión desactualizada o meta no encontrada),
// la transacción falla por completo y el movimiento no es registrado (AC-A-060-02).
func (r *DynamoRepository) RecordMovementAtomic(
	ctx context.Context,
	userID, movementID, movementType string,
	amountCents int64,
	occurredAt time.Time,
	goalID string,
	expectedGoalVersion *int64,
) error {
	pk, sk, gsi1pk, gsi1sk := domainDynamo.MovementKeys(userID, movementID, movementType, occurredAt)

	movItem := domainDynamo.NewDynamoItem(pk, sk, "MOVEMENT", map[string]any{
		"movementId":   movementID,
		"userId":       userID,
		"movementType": movementType,
		"amountCents":  amountCents,
		"occurredAt":   occurredAt.UTC().Format(time.RFC3339),
		"goalId":       goalID,
	}).WithGSI1(gsi1pk, gsi1sk)

	actions := []infraDynamo.TransactAction{
		{
			Type: infraDynamo.ActionPut,
			PK:   pk,
			SK:   sk,
			Item: &movItem,
		},
	}

	// Si está vinculado a una meta, incluir la actualización de la meta en la misma transacción (AC-A-060-02)
	if goalID != "" {
		goalPK, goalSK := domainDynamo.GoalKeys(userID, goalID)
		actions = append(actions, infraDynamo.TransactAction{
			Type:            infraDynamo.ActionUpdate,
			PK:              goalPK,
			SK:              goalSK,
			ExpectedVersion: expectedGoalVersion,
			UpdateFn: func(existing *domainDynamo.DynamoItem) error {
				currentSaved, _ := existing.Attributes["savedCents"].(int64)
				existing.Attributes["savedCents"] = currentSaved + amountCents
				return nil
			},
		})
	}

	return r.table.TransactWriteItems(ctx, actions)
}

// QueryMovementsByType consulta movimientos acotados por usuario y tipo utilizando GSI1 (sin Scan, AC-A-060-01).
func (r *DynamoRepository) QueryMovementsByType(ctx context.Context, userID, movementType string, limit int) ([]domainDynamo.DynamoItem, error) {
	gsi1pk := fmt.Sprintf("%s%s#TYPE#%s", domainDynamo.PrefixUser, userID, movementType)
	return r.table.QueryGSI1(ctx, gsi1pk, "", false, limit) // Más recientes primero
}

// QueryAllUserItems recupera todos los datos del usuario en su partición (E014 borrado de cuenta o exportación sin Scan).
func (r *DynamoRepository) QueryAllUserItems(ctx context.Context, userID string) ([]domainDynamo.DynamoItem, error) {
	pk := domainDynamo.PrefixUser + userID
	return r.table.QueryPartition(ctx, pk, "", "", true, 0)
}

