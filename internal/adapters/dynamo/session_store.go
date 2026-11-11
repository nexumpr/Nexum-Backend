package dynamo

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
)

// SessionStore implementa appIdentity.LoginStore sobre la tabla existente NEXUM_CORE_<ENV>.
// Escribe la sesión autoritativa (USER#id / SESSION#id) y el localizador de refresh
// (REFRESH#digest / SESSION) en una sola transacción condicional. No usa Scan ni memoria.
type SessionStore struct {
	client DynamoAPI
	table  string
}

// NewSessionStore construye el almacén real. Sin cliente o tabla falla cerrado.
func NewSessionStore(client DynamoAPI, table string) *SessionStore {
	return &SessionStore{client: client, table: strings.TrimSpace(table)}
}

// LookupIdentity resuelve el usuario real desde la reserva de email y carga su perfil.
// Devuelve appIdentity.ErrInvalidCredentials cuando la reserva o el perfil no existen o
// son incoherentes (mismo error para todos los casos: sin enumeración de cuentas).
func (s *SessionStore) LookupIdentity(ctx context.Context, normalizedEmail string) (*appIdentity.StoredIdentity, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, appIdentity.ErrLoginUnavailable
	}

	emailPK, emailSK := domainDynamo.EmailReservationKeys(normalizedEmail)
	reservation, err := s.getItem(ctx, emailPK, emailSK)
	if err != nil {
		return nil, err
	}
	if reservation == nil {
		return nil, appIdentity.ErrInvalidCredentials
	}
	userID := stringAttribute(reservation["userId"])
	if userID == "" {
		return nil, appIdentity.ErrInvalidCredentials
	}

	userPK, userSK := domainDynamo.UserKeys(userID)
	perfil, err := s.getItem(ctx, userPK, userSK)
	if err != nil {
		return nil, err
	}
	if perfil == nil {
		return nil, appIdentity.ErrInvalidCredentials
	}

	profileUserID := stringAttribute(perfil["userId"])
	if profileUserID != "" && profileUserID != userID {
		// Reserva y perfil apuntan a usuarios distintos: dato incoherente.
		return nil, appIdentity.ErrInvalidCredentials
	}

	email := stringAttribute(perfil["email"])
	if email == "" {
		return nil, appIdentity.ErrInvalidCredentials
	}
	if !strings.EqualFold(email, normalizedEmail) {
		return nil, appIdentity.ErrInvalidCredentials
	}

	emailVerified := false
	if value, ok := perfil["emailVerified"].(*types.AttributeValueMemberBOOL); ok {
		emailVerified = value.Value
	}

	return &appIdentity.StoredIdentity{
		UserID:        userID,
		Email:         email,
		DisplayName:   stringAttribute(perfil["displayName"]),
		PasswordHash:  stringAttribute(perfil["passwordHash"]),
		EmailVerified: emailVerified,
		Status:        stringAttribute(perfil["status"]),
	}, nil
}

// OnboardingConfigured devuelve el estado real del onboarding. Un usuario histórico sin
// ítem ONBOARDING devuelve false (política de compatibilidad explícita de RFC-013), nunca true.
func (s *SessionStore) OnboardingConfigured(ctx context.Context, userID string) (bool, error) {
	if s == nil || s.client == nil || s.table == "" {
		return false, appIdentity.ErrLoginUnavailable
	}
	pk, sk := domainDynamo.OnboardingKeys(userID)
	item, err := s.getItem(ctx, pk, sk)
	if err != nil {
		return false, err
	}
	if item == nil {
		return false, nil
	}
	if value, ok := item["configured"].(*types.AttributeValueMemberBOOL); ok {
		return value.Value, nil
	}
	return false, nil
}

// CreateSession persiste sesión y localizador de refresh en una única transacción.
// El refresh en claro nunca llega aquí: sólo su digest. Si algo falla, no se emite sesión.
func (s *SessionStore) CreateSession(ctx context.Context, record appIdentity.SessionRecord) error {
	if s == nil || s.client == nil || s.table == "" {
		return appIdentity.ErrLoginUnavailable
	}

	sessionPK, sessionSK := domainDynamo.UserSessionKeys(record.UserID, record.SessionID)
	refreshPK, refreshSK := domainDynamo.RefreshKeys(record.RefreshHash)

	createdAt := record.CreatedAt.UTC().Format(time.RFC3339)
	expiresAt := record.ExpiresAt.UTC().Format(time.RFC3339)

	sessionItem := map[string]types.AttributeValue{
		"PK":             stringAttr(sessionPK),
		"SK":             stringAttr(sessionSK),
		"entityType":     stringAttr("USER_SESSION"),
		"userId":         stringAttr(record.UserID),
		"sessionId":      stringAttr(record.SessionID),
		"familyId":       stringAttr(record.FamilyID),
		"refreshHash":    stringAttr(record.RefreshHash),
		"accessJti":      stringAttr(record.AccessJTI),
		"status":         stringAttr(record.Status),
		"createdAt":      stringAttr(createdAt),
		"lastUsedAt":     stringAttr(createdAt),
		"expiresAt":      stringAttr(expiresAt),
		"expiresAtEpoch": numberAttr(record.ExpiresAt.UTC().Unix()),
		"version":        numberAttr(1),
	}
	if record.DeviceID != "" {
		sessionItem["deviceId"] = stringAttr(record.DeviceID)
	}
	if record.DeviceName != "" {
		sessionItem["deviceName"] = stringAttr(record.DeviceName)
	}
	if record.Platform != "" {
		sessionItem["platform"] = stringAttr(record.Platform)
	}

	refreshItem := map[string]types.AttributeValue{
		"PK":             stringAttr(refreshPK),
		"SK":             stringAttr(refreshSK),
		"entityType":     stringAttr("REFRESH_LOOKUP"),
		"userId":         stringAttr(record.UserID),
		"sessionId":      stringAttr(record.SessionID),
		"familyId":       stringAttr(record.FamilyID),
		"status":         stringAttr(record.Status),
		"createdAt":      stringAttr(createdAt),
		"expiresAt":      stringAttr(expiresAt),
		"expiresAtEpoch": numberAttr(record.ExpiresAt.UTC().Unix()),
	}

	input := &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{Put: &types.Put{
				TableName:           aws.String(s.table),
				Item:                sessionItem,
				ConditionExpression: aws.String("attribute_not_exists(PK)"),
			}},
			{Put: &types.Put{
				TableName:           aws.String(s.table),
				Item:                refreshItem,
				ConditionExpression: aws.String("attribute_not_exists(PK)"),
			}},
		},
	}

	if _, err := s.client.TransactWriteItems(ctx, input); err != nil {
		var canceled *types.TransactionCanceledException
		if errors.As(err, &canceled) {
			return appIdentity.ErrLoginUnavailable
		}
		return appIdentity.ErrLoginUnavailable
	}
	return nil
}

func (s *SessionStore) getItem(ctx context.Context, pk, sk string) (map[string]types.AttributeValue, error) {
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(pk),
			"SK": stringAttr(sk),
		},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return nil, appIdentity.ErrLoginUnavailable
	}
	if out == nil || out.Item == nil {
		return nil, nil
	}
	return out.Item, nil
}

var _ = strconv.Itoa
