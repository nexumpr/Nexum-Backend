package dynamo

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	appPrefs "github.com/nexumpr/Nexum-Backend/internal/application/notificationprefs"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
)

const (
	// notificationPreferencesSortKey es la SK dedicada de E057/E058 dentro de
	// `USER#<id>`. Distinta de `PREFERENCES` (perfil) para no mezclar contratos.
	notificationPreferencesSortKey = "NOTIFICATION_PREFERENCES"

	notificationPreferencesEntityType = "USER_NOTIFICATION_PREFERENCES"
)

// NotificationPreferencesAPI es el subconjunto del cliente que necesita este
// almacén. GetItem/PutItem resuelven la lectura perezosa por clave exacta; no
// hay Scan en la interfaz, por lo que el compilador lo impide.
type NotificationPreferencesAPI interface {
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
}

// NotificationPreferencesStore implementa `notificationprefs.Store` sobre la
// tabla única existente, en la partición del titular.
type NotificationPreferencesStore struct {
	client NotificationPreferencesAPI
	table  string
}

// NewNotificationPreferencesStore construye el adaptador. Sin cliente o tabla,
// todas las operaciones fallan cerradas: nunca se sustituyen por memoria local.
func NewNotificationPreferencesStore(client NotificationPreferencesAPI, table string) *NotificationPreferencesStore {
	return &NotificationPreferencesStore{client: client, table: strings.TrimSpace(table)}
}

func notificationPreferencesKeys(userID string) (pk, sk string) {
	return domainDynamo.PrefixUser + strings.TrimSpace(userID), notificationPreferencesSortKey
}

// Load lee con consistencia fuerte. Devuelve (nil, nil) si el item no existe o
// si la clave la ocupa otra entidad; un fallo real se propaga como ErrUnavailable.
func (s *NotificationPreferencesStore) Load(ctx context.Context, userID string) (*appPrefs.Preferences, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, appPrefs.ErrUnavailable
	}
	if strings.TrimSpace(userID) == "" {
		return nil, nil
	}
	pk, sk := notificationPreferencesKeys(userID)
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      aws.String(s.table),
		Key:            map[string]types.AttributeValue{"PK": stringAttr(pk), "SK": stringAttr(sk)},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return nil, appPrefs.ErrUnavailable
	}
	if out == nil || len(out.Item) == 0 {
		return nil, nil
	}
	if entity := stringAttribute(out.Item["entityType"]); entity != "" && entity != notificationPreferencesEntityType {
		return nil, nil
	}
	return notificationPreferencesFromItem(out.Item), nil
}

// Create materializa los defaults con `attribute_not_exists(PK)`. Si otra
// petición ganó la carrera, se devuelve el item ya creado (creación idempotente).
func (s *NotificationPreferencesStore) Create(ctx context.Context, userID string, defaults appPrefs.Preferences, now time.Time) (*appPrefs.Preferences, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, appPrefs.ErrUnavailable
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, appPrefs.ErrUnavailable
	}
	if defaults.Version < 1 {
		defaults.Version = appPrefs.DefaultVersion
	}
	pk, sk := notificationPreferencesKeys(userID)
	item := notificationPreferencesItem(userID, pk, sk, defaults, now)

	_, err := s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(s.table),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	})
	if err == nil {
		return &defaults, nil
	}
	var condicional *types.ConditionalCheckFailedException
	if !errors.As(err, &condicional) {
		return nil, appPrefs.ErrUnavailable
	}
	existente, errLectura := s.Load(ctx, userID)
	if errLectura != nil || existente == nil {
		return nil, appPrefs.ErrConflict
	}
	return existente, nil
}

func notificationPreferencesItem(userID, pk, sk string, p appPrefs.Preferences, now time.Time) map[string]types.AttributeValue {
	item := map[string]types.AttributeValue{
		"PK":                 stringAttr(pk),
		"SK":                 stringAttr(sk),
		"entityType":         stringAttr(notificationPreferencesEntityType),
		"userId":             stringAttr(userID),
		"pushEnabled":        boolAttr(p.PushEnabled),
		"emailEnabled":       boolAttr(p.EmailEnabled),
		"reminderEnabled":    boolAttr(p.ReminderEnabled),
		"budgetAlerts":       boolAttr(p.BudgetAlerts),
		"goalUpdates":        boolAttr(p.GoalUpdates),
		"recurringReminders": boolAttr(p.RecurringReminders),
		"streakReminders":    boolAttr(p.StreakReminders),
		"version":            numberAttr(p.Version),
		"createdAt":          stringAttr(now.UTC().Format(time.RFC3339)),
		"updatedAt":          stringAttr(now.UTC().Format(time.RFC3339)),
	}
	if p.QuietHours != nil {
		item["quietHoursEnabled"] = boolAttr(p.QuietHours.Enabled)
		if p.QuietHours.Start != "" {
			item["quietHoursStart"] = stringAttr(p.QuietHours.Start)
		}
		if p.QuietHours.End != "" {
			item["quietHoursEnd"] = stringAttr(p.QuietHours.End)
		}
	}
	if strings.TrimSpace(p.TimeZone) != "" {
		item["timeZone"] = stringAttr(strings.TrimSpace(p.TimeZone))
	}
	return item
}

func notificationPreferencesFromItem(item map[string]types.AttributeValue) *appPrefs.Preferences {
	if len(item) == 0 {
		return nil
	}
	p := &appPrefs.Preferences{
		PushEnabled:        boolAttribute(item["pushEnabled"]),
		EmailEnabled:       boolAttribute(item["emailEnabled"]),
		ReminderEnabled:    boolAttribute(item["reminderEnabled"]),
		BudgetAlerts:       boolAttribute(item["budgetAlerts"]),
		GoalUpdates:        boolAttribute(item["goalUpdates"]),
		RecurringReminders: boolAttribute(item["recurringReminders"]),
		StreakReminders:    boolAttribute(item["streakReminders"]),
		TimeZone:           stringAttribute(item["timeZone"]),
	}
	if version, ok := attributeInt(item["version"]); ok && version >= 1 {
		p.Version = version
	}
	if enabled, ok := notificationBoolOptional(item["quietHoursEnabled"]); ok {
		p.QuietHours = &appPrefs.QuietHours{
			Enabled: enabled,
			Start:   stringAttribute(item["quietHoursStart"]),
			End:     stringAttribute(item["quietHoursEnd"]),
		}
	}
	return p
}

func notificationBoolOptional(value types.AttributeValue) (bool, bool) {
	flag, ok := value.(*types.AttributeValueMemberBOOL)
	if !ok {
		return false, false
	}
	return flag.Value, true
}
