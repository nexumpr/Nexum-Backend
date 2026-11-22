package dynamo

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	appPrefs "github.com/nexumpr/Nexum-Backend/internal/application/notificationprefs"
)

type e057FakeClient struct {
	item     map[string]types.AttributeValue
	getErr   error
	putErr   error
	lastPut  *dynamodb.PutItemInput
	lastGet  *dynamodb.GetItemInput
	getCalls int
}

func (f *e057FakeClient) GetItem(_ context.Context, params *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	f.lastGet = params
	f.getCalls++
	if f.getErr != nil {
		return nil, f.getErr
	}
	return &dynamodb.GetItemOutput{Item: f.item}, nil
}

func (f *e057FakeClient) PutItem(_ context.Context, params *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	f.lastPut = params
	if f.putErr != nil {
		return nil, f.putErr
	}
	return &dynamodb.PutItemOutput{}, nil
}

func e057Item() map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"entityType":         stringAttr(notificationPreferencesEntityType),
		"pushEnabled":        boolAttr(true),
		"emailEnabled":       boolAttr(false),
		"reminderEnabled":    boolAttr(false),
		"budgetAlerts":       boolAttr(false),
		"goalUpdates":        boolAttr(true),
		"recurringReminders": boolAttr(false),
		"streakReminders":    boolAttr(true),
		"quietHoursEnabled":  boolAttr(false),
		"timeZone":           stringAttr("America/New_York"),
		"version":            numberAttr(9),
	}
}

func TestE057AdaptadorLoadProyeccion(t *testing.T) {
	cliente := &e057FakeClient{item: e057Item()}
	store := NewNotificationPreferencesStore(cliente, "nexum-table")
	p, err := store.Load(context.Background(), "user-a")
	if err != nil {
		t.Fatal(err)
	}
	if !p.PushEnabled || p.ReminderEnabled || p.BudgetAlerts || p.RecurringReminders ||
		!p.StreakReminders || p.Version != 9 || p.TimeZone != "America/New_York" {
		t.Fatalf("proyección incorrecta: %+v", p)
	}
	if p.QuietHours == nil || p.QuietHours.Enabled {
		t.Fatalf("quietHours presente con enabled=false debió conservarse: %+v", p.QuietHours)
	}
	if got := stringAttribute(cliente.lastGet.Key["PK"]); got != "USER#user-a" {
		t.Fatalf("PK incorrecto: %q", got)
	}
	if got := stringAttribute(cliente.lastGet.Key["SK"]); got != notificationPreferencesSortKey {
		t.Fatalf("SK incorrecto: %q", got)
	}
	if cliente.lastGet.ConsistentRead == nil || !*cliente.lastGet.ConsistentRead {
		t.Fatal("la lectura no es consistente")
	}
}

func TestE057AdaptadorLoadAusenteYForaneo(t *testing.T) {
	sinItem := &e057FakeClient{}
	p, err := NewNotificationPreferencesStore(sinItem, "nexum-table").Load(context.Background(), "user-a")
	if err != nil || p != nil {
		t.Fatalf("item ausente: %+v %v", p, err)
	}
	foraneo := &e057FakeClient{item: map[string]types.AttributeValue{
		"entityType": stringAttr("OTRA_ENTIDAD"),
	}}
	p, err = NewNotificationPreferencesStore(foraneo, "nexum-table").Load(context.Background(), "user-a")
	if err != nil || p != nil {
		t.Fatalf("entidad ajena no debió interpretarse: %+v %v", p, err)
	}
}

func TestE057AdaptadorCreateCondicional(t *testing.T) {
	cliente := &e057FakeClient{}
	store := NewNotificationPreferencesStore(cliente, "nexum-table")
	creado, err := store.Create(context.Background(), "user-a", appPrefs.Defaults(), time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC))
	if err != nil || creado == nil {
		t.Fatalf("create: %+v %v", creado, err)
	}
	if cliente.lastPut == nil || cliente.lastPut.ConditionExpression == nil ||
		*cliente.lastPut.ConditionExpression != "attribute_not_exists(PK)" {
		t.Fatal("la creación no es condicional")
	}
	if stringAttribute(cliente.lastPut.Item["SK"]) != notificationPreferencesSortKey {
		t.Fatal("SK de creación incorrecta")
	}
}

func TestE057AdaptadorCreateGanadorConcurrente(t *testing.T) {
	existente := e057Item()
	conditional := &types.ConditionalCheckFailedException{}
	cliente := &e057FakeClient{item: existente, putErr: conditional}
	store := NewNotificationPreferencesStore(cliente, "nexum-table")
	ganador, err := store.Create(context.Background(), "user-a", appPrefs.Defaults(), time.Now())
	if err != nil {
		t.Fatalf("la carrera no debió fallar: %v", err)
	}
	if ganador == nil || ganador.Version != 9 || !ganador.PushEnabled {
		t.Fatalf("no se devolvió el item ganador: %+v", ganador)
	}
	if cliente.getCalls != 1 {
		t.Fatalf("se esperaba una relectura, hubo %d", cliente.getCalls)
	}
}

func TestE057AdaptadorFallaCerrada(t *testing.T) {
	sinCliente := NewNotificationPreferencesStore(nil, "nexum-table")
	if _, err := sinCliente.Load(context.Background(), "user-a"); !errors.Is(err, appPrefs.ErrUnavailable) {
		t.Fatalf("sin cliente: %v", err)
	}
	if _, err := sinCliente.Create(context.Background(), "user-a", appPrefs.Defaults(), time.Now()); !errors.Is(err, appPrefs.ErrUnavailable) {
		t.Fatalf("sin cliente create: %v", err)
	}
	caido := &e057FakeClient{getErr: errors.New("dynamo down")}
	if _, err := NewNotificationPreferencesStore(caido, "nexum-table").Load(context.Background(), "user-a"); !errors.Is(err, appPrefs.ErrUnavailable) {
		t.Fatalf("store caído: %v", err)
	}
}
