package dynamo

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/nexumpr/Nexum-Backend/internal/application/pushoutbox"
)

type outboxFake struct {
	mu            sync.Mutex
	items         map[string]map[string]types.AttributeValue
	ultimaQuery   *dynamodb.QueryOutput
	ultimaEntrada *dynamodb.QueryInput
	updates       []*dynamodb.UpdateItemInput
	puts          []*dynamodb.PutItemInput
	fallarQuery   bool
}

func nuevoOutboxFake() *outboxFake {
	return &outboxFake{items: make(map[string]map[string]types.AttributeValue)}
}

func claveItemInput(params *dynamodb.PutItemInput) string {
	return stringAttribute(params.Item["PK"]) + "|" + stringAttribute(params.Item["SK"])
}

func claveUpdateInput(params *dynamodb.UpdateItemInput) string {
	return stringAttribute(params.Key["PK"]) + "|" + stringAttribute(params.Key["SK"])
}

func (f *outboxFake) Query(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ultimaEntrada = params
	if f.fallarQuery {
		return nil, errors.New("query no disponible")
	}
	if f.ultimaQuery != nil {
		return f.ultimaQuery, nil
	}
	return &dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{}}, nil
}

func (f *outboxFake) GetItem(_ context.Context, params *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	clave := stringAttribute(params.Key["PK"]) + "|" + stringAttribute(params.Key["SK"])
	return &dynamodb.GetItemOutput{Item: f.items[clave]}, nil
}

func (f *outboxFake) PutItem(_ context.Context, params *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.puts = append(f.puts, params)
	clave := claveItemInput(params)
	if params.ConditionExpression != nil && strings.Contains(*params.ConditionExpression, "attribute_not_exists(PK)") {
		if _, existe := f.items[clave]; existe {
			return nil, &types.ConditionalCheckFailedException{}
		}
	}
	f.items[clave] = params.Item
	return &dynamodb.PutItemOutput{}, nil
}

func (f *outboxFake) UpdateItem(_ context.Context, params *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updates = append(f.updates, params)
	clave := claveUpdateInput(params)
	item := f.items[clave]
	if item == nil {
		item = map[string]types.AttributeValue{}
	}
	expresion := aws.ToString(params.ConditionExpression)
	if strings.Contains(expresion, "#estado = :pendiente") {
		estado := stringAttribute(item["status"])
		if estado != "" && estado != string(pushoutbox.EstadoPendiente) && estado != string(pushoutbox.EstadoFallido) {
			return nil, &types.ConditionalCheckFailedException{}
		}
	}
	if strings.Contains(expresion, "#owner = :owner") {
		ownerPrevio := stringAttribute(item["owner"])
		ownerNuevo := stringAttribute(params.ExpressionAttributeValues[":owner"])
		if ownerPrevio != ownerNuevo {
			return nil, &types.ConditionalCheckFailedException{}
		}
	}
	actualizado := make(map[string]types.AttributeValue, len(item)+4)
	for k, v := range item {
		actualizado[k] = v
	}
	updateExpr := aws.ToString(params.UpdateExpression)
	switch {
	case strings.Contains(updateExpr, "#estado = :enviando"):
		actualizado["status"] = stringAttr(string(pushoutbox.EstadoEnviando))
		actualizado["owner"] = params.ExpressionAttributeValues[":owner"]
		actualizado["attempts"] = numberAttr(1)
	case strings.Contains(updateExpr, "#estado = :enviado"):
		actualizado["status"] = stringAttr(string(pushoutbox.EstadoEnviado))
	case strings.Contains(updateExpr, "#estado = :revocado"):
		actualizado["status"] = stringAttr(string(pushoutbox.EstadoRevocado))
	case strings.Contains(updateExpr, "#estado = :fallido"):
		actualizado["status"] = stringAttr(string(pushoutbox.EstadoFallido))
	}
	f.items[clave] = actualizado
	return &dynamodb.UpdateItemOutput{Attributes: actualizado}, nil
}

func TestOUTBOXCrearEscribeClaveDUEYCondicional(t *testing.T) {
	fake := nuevoOutboxFake()
	store := NewPushOutboxStore(fake, "NEXUM_CORE_TEST").WithOwner("nodo-a")
	ahora := time.Date(2026, 11, 22, 12, 0, 0, 123456789, time.UTC)
	store.WithClock(func() time.Time { return ahora })

	creado, err := store.Crear(context.Background(), pushoutbox.Registro{
		UserID: "u1", NotificationID: "n1", Title: "Aviso", Message: "Mensaje",
		Type: "REMINDER", EventID: "evt-1", NextAttemptAt: ahora,
	})
	if err != nil || !creado {
		t.Fatalf("crear: %v %v", creado, err)
	}
	if len(fake.puts) != 1 {
		t.Fatalf("puts=%d", len(fake.puts))
	}
	put := fake.puts[0]
	if got := stringAttribute(put.Item["PK"]); got != pushOutboxPartition {
		t.Fatalf("PK=%q", got)
	}
	esperado := "DUE#" + ahora.Format(time.RFC3339Nano) + "#n1"
	if got := stringAttribute(put.Item["SK"]); got != esperado {
		t.Fatalf("SK=%q esperado %q", got, esperado)
	}
	if aws.ToString(put.ConditionExpression) != "attribute_not_exists(PK)" {
		t.Fatalf("condición=%q", aws.ToString(put.ConditionExpression))
	}
	creadoOtraVez, err := store.Crear(context.Background(), pushoutbox.Registro{
		UserID: "u1", NotificationID: "n1", EventID: "evt-1", NextAttemptAt: ahora,
	})
	if err != nil || creadoOtraVez {
		t.Fatalf("replay creó otro ítem: %v %v", creadoOtraVez, err)
	}
}

func TestOUTBOXQueryAcotadaSinScan(t *testing.T) {
	fake := nuevoOutboxFake()
	ahora := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	fake.ultimaQuery = &dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{{
		"entityType":     stringAttr(pushOutboxEntityType),
		"SK":             stringAttr("DUE#2026-11-22T12:00:00Z#n1"),
		"userID":         stringAttr("u1"),
		"notificationID": stringAttr("n1"),
		"title":          stringAttr("Aviso"),
		"message":        stringAttr("Mensaje"),
		"type":           stringAttr("REMINDER"),
		"eventId":        stringAttr("evt-1"),
		"status":         stringAttr("pending"),
		"attempts":       numberAttr(2),
		"nextAttemptAt":  stringAttr("2026-11-22T12:00:00Z"),
		"createdAt":      stringAttr("2026-11-22T11:00:00Z"),
		"updatedAt":      stringAttr("2026-11-22T11:00:00Z"),
	}}}
	store := NewPushOutboxStore(fake, "NEXUM_CORE_TEST")
	registros, err := store.Pendientes(context.Background(), ahora, 25)
	if err != nil || len(registros) != 1 {
		t.Fatalf("pendientes=%+v err=%v", registros, err)
	}
	entrada := fake.ultimaEntrada
	if entrada == nil {
		t.Fatal("no se ejecutó Query")
	}
	if aws.ToString(entrada.KeyConditionExpression) != "#pk = :pk AND begins_with(#sk, :prefijo)" {
		t.Fatalf("KeyCondition=%q", aws.ToString(entrada.KeyConditionExpression))
	}
	if stringAttribute(entrada.ExpressionAttributeValues[":pk"]) != pushOutboxPartition ||
		stringAttribute(entrada.ExpressionAttributeValues[":prefijo"]) != pushOutboxDuePrefix {
		t.Fatalf("valores de Query incorrectos: %+v", entrada.ExpressionAttributeValues)
	}
	if aws.ToInt32(entrada.Limit) != 25 {
		t.Fatalf("limit=%d", aws.ToInt32(entrada.Limit))
	}
	if registros[0].Attempts != 2 || registros[0].Status != pushoutbox.EstadoPendiente ||
		registros[0].ID != "DUE#2026-11-22T12:00:00Z#n1" {
		t.Fatalf("proyección incorrecta: %+v", registros[0])
	}
}

func TestOUTBOXReclamoCondicionalDosNodos(t *testing.T) {
	fake := nuevoOutboxFake()
	ahora := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	clave := "OUTBOX#GLOBAL|DUE#2026-11-22T12:00:00Z#n1"
	fake.items[clave] = map[string]types.AttributeValue{
		"status": stringAttr("pending"), "attempts": numberAttr(0),
	}
	nodoA := NewPushOutboxStore(fake, "NEXUM_CORE_TEST").WithOwner("nodo-a")
	nodoB := NewPushOutboxStore(fake, "NEXUM_CORE_TEST").WithOwner("nodo-b")

	intentos, ok, err := nodoA.Reclamar(context.Background(), "DUE#2026-11-22T12:00:00Z#n1", ahora, 30*time.Second)
	if err != nil || !ok || intentos != 1 {
		t.Fatalf("reclamo A: intentos=%d ok=%v err=%v", intentos, ok, err)
	}
	_, okB, errB := nodoB.Reclamar(context.Background(), "DUE#2026-11-22T12:00:00Z#n1", ahora, 30*time.Second)
	if errB != nil || okB {
		t.Fatalf("el nodo B no debió reclamar: ok=%v err=%v", okB, errB)
	}
	if len(fake.updates) != 2 {
		t.Fatalf("updates=%d", len(fake.updates))
	}
	condicion := aws.ToString(fake.updates[0].ConditionExpression)
	if !strings.Contains(condicion, "#estado = :pendiente") || !strings.Contains(condicion, "#leaseUntil < :ahora") {
		t.Fatalf("condición de lease incompleta: %q", condicion)
	}
}

func TestOUTBOXCIERRESYReprogramacionExigenOwner(t *testing.T) {
	fake := nuevoOutboxFake()
	store := NewPushOutboxStore(fake, "NEXUM_CORE_TEST").WithOwner("nodo-a")
	ahora := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	id := "DUE#2026-11-22T12:00:00Z#n1"
	clave := "OUTBOX#GLOBAL|" + id
	fake.items[clave] = map[string]types.AttributeValue{
		"status": stringAttr("sending"), "owner": stringAttr("nodo-a"), "attempts": numberAttr(1),
	}
	if err := store.MarcarEnviado(context.Background(), id, ahora); err != nil {
		t.Fatal(err)
	}
	if err := store.MarcarRevocado(context.Background(), id, "UNREGISTERED", ahora); err != nil {
		t.Fatal(err)
	}
	if err := store.Reprogramar(context.Background(), id, 2, "REINTENTABLE", ahora.Add(time.Minute), ahora); err != nil {
		t.Fatal(err)
	}
	if len(fake.updates) != 3 {
		t.Fatalf("updates=%d", len(fake.updates))
	}
	for i, update := range fake.updates {
		if aws.ToString(update.ConditionExpression) != "#owner = :owner" {
			t.Fatalf("update %d sin condición de owner: %q", i, aws.ToString(update.ConditionExpression))
		}
	}
	reprogramar := aws.ToString(fake.updates[2].UpdateExpression)
	if !strings.Contains(reprogramar, "#siguiente = :siguiente") || !strings.Contains(reprogramar, "#intentos = :intentos") {
		t.Fatalf("reprogramación incompleta: %q", reprogramar)
	}
	otroNodo := NewPushOutboxStore(fake, "NEXUM_CORE_TEST").WithOwner("nodo-b")
	if err := otroNodo.MarcarEnviado(context.Background(), id, ahora); !errors.Is(err, ErrConflictoLease) {
		t.Fatalf("otro nodo debió recibir conflicto de lease: %v", err)
	}
}

func TestOUTBOXIdempotenciaYBandejaE061(t *testing.T) {
	fake := nuevoOutboxFake()
	store := NewPushOutboxStore(fake, "NEXUM_CORE_TEST")
	ctx := context.Background()
	entregado, err := store.YaEntregado(ctx, "u1", "evt-1")
	if err != nil || entregado {
		t.Fatalf("YaEntregado=%v err=%v", entregado, err)
	}
	ahora := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	if err := store.MarcarEntregado(ctx, "u1", "evt-1", "n1", ahora); err != nil {
		t.Fatal(err)
	}
	entregado, err = store.YaEntregado(ctx, "u1", "evt-1")
	if err != nil || !entregado {
		t.Fatalf("idempotencia no persistida: %v %v", entregado, err)
	}
	if err := store.MarcarEntregado(ctx, "u1", "evt-1", "n1", ahora); err != nil {
		t.Fatalf("replay de idempotencia debió ser inocuo: %v", err)
	}
	otroEvento, errOtro := store.YaEntregado(ctx, "u1", "evt-2")
	if errOtro != nil || otroEvento {
		t.Fatal("eventos distintos no deben compartir idempotencia")
	}

	if err := store.GuardarBandeja(ctx, pushoutbox.RegistroBandeja{
		UserID: "u1", ID: "n1", Title: "Aviso", Message: "Mensaje", Type: "REMINDER", CreatedAt: ahora,
	}); err != nil {
		t.Fatal(err)
	}
	bandeja := fake.puts[len(fake.puts)-1]
	if got := stringAttribute(bandeja.Item["PK"]); got != "USER#u1" {
		t.Fatalf("PK bandeja=%q", got)
	}
	esperadoSK := "NOTIFICATION#" + ahora.Format(time.RFC3339Nano) + "#n1"
	if got := stringAttribute(bandeja.Item["SK"]); got != esperadoSK {
		t.Fatalf("SK bandeja=%q esperado %q", got, esperadoSK)
	}
	if boolAttribute(bandeja.Item["isRead"]) {
		t.Fatal("la notificación nueva debe estar no leída")
	}
}

func TestOUTBOXSinClienteFallaCerrado(t *testing.T) {
	store := NewPushOutboxStore(nil, "")
	if _, err := store.Crear(context.Background(), pushoutbox.Registro{}); !errors.Is(err, ErrOutboxNoDisponible) {
		t.Fatalf("Crear sin cliente: %v", err)
	}
	if _, _, err := store.Reclamar(context.Background(), "id", time.Now(), time.Second); !errors.Is(err, ErrOutboxNoDisponible) {
		t.Fatalf("Reclamar sin cliente: %v", err)
	}
	if err := store.GuardarBandeja(context.Background(), pushoutbox.RegistroBandeja{}); !errors.Is(err, ErrOutboxNoDisponible) {
		t.Fatalf("GuardarBandeja sin cliente: %v", err)
	}
}
