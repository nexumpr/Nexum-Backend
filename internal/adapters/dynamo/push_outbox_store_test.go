package dynamo

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/nexumpr/Nexum-Backend/internal/application/pushoutbox"
)

// outboxFake emula la tabla única con las garantías que el outbox necesita:
// orden lexicográfico por SK, KeyCondition por rango, Limit aplicado antes de
// cualquier filtro, ExclusiveStartKey y transacciones atómicas con condiciones.
type outboxFake struct {
	mu            sync.Mutex
	items         map[string]map[string]types.AttributeValue
	consultas     []*dynamodb.QueryInput
	updates       []*dynamodb.UpdateItemInput
	puts          []*dynamodb.PutItemInput
	transacciones []*dynamodb.TransactWriteItemsInput
	fallarQuery   bool
	// tamanoPagina fuerza páginas más cortas que el Limit pedido, como hace
	// DynamoDB al alcanzar 1 MB o al aplicar cuotas internas.
	tamanoPagina int32
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

func skDeClave(clave string) string {
	_, sk, _ := strings.Cut(clave, "|")
	return sk
}

func (f *outboxFake) Query(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.consultas = append(f.consultas, params)
	if f.fallarQuery {
		return nil, errors.New("query no disponible")
	}
	particion := stringAttribute(params.ExpressionAttributeValues[":pk"])
	condicion := aws.ToString(params.KeyConditionExpression)
	prefijo, desde, hasta := "", "", ""
	switch condicion {
	case "#pk = :pk AND #sk BETWEEN :desde AND :hasta":
		desde = stringAttribute(params.ExpressionAttributeValues[":desde"])
		hasta = stringAttribute(params.ExpressionAttributeValues[":hasta"])
	case "#pk = :pk AND begins_with(#sk, :prefijo)":
		prefijo = stringAttribute(params.ExpressionAttributeValues[":prefijo"])
	default:
		return nil, fmt.Errorf("el doble de DynamoDB no soporta la condición %q", condicion)
	}
	claves := make([]string, 0, len(f.items))
	for clave := range f.items {
		pk, sk, _ := strings.Cut(clave, "|")
		if pk != particion {
			continue
		}
		if prefijo != "" && !strings.HasPrefix(sk, prefijo) {
			continue
		}
		if desde != "" && sk < desde {
			continue
		}
		if hasta != "" && sk > hasta {
			continue
		}
		claves = append(claves, clave)
	}
	sort.Slice(claves, func(i, j int) bool { return skDeClave(claves[i]) < skDeClave(claves[j]) })
	inicio := 0
	if params.ExclusiveStartKey != nil {
		desdeSK := stringAttribute(params.ExclusiveStartKey["SK"])
		for i, clave := range claves {
			if skDeClave(clave) == desdeSK {
				inicio = i + 1
				break
			}
		}
	}
	tope := int(aws.ToInt32(params.Limit))
	if f.tamanoPagina > 0 && int(f.tamanoPagina) < tope {
		tope = int(f.tamanoPagina)
	}
	fin := len(claves)
	if tope > 0 && inicio+tope < fin {
		fin = inicio + tope
	}
	salida := &dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{}}
	for _, clave := range claves[inicio:fin] {
		salida.Items = append(salida.Items, f.items[clave])
	}
	if fin < len(claves) && fin > inicio {
		salida.LastEvaluatedKey = map[string]types.AttributeValue{
			"PK": stringAttr(particion),
			"SK": stringAttr(skDeClave(claves[fin-1])),
		}
	}
	return salida, nil
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

// cumpleCondicion evalúa las condiciones exactas que emite el adaptador.
func (f *outboxFake) cumpleCondicion(clave, condicion string, valores map[string]types.AttributeValue) (bool, error) {
	switch condicion {
	case "":
		return true, nil
	case "attribute_not_exists(PK)":
		_, existe := f.items[clave]
		return !existe, nil
	case "#owner = :owner":
		return f.propietario(clave, valores[":owner"]), nil
	case "#owner = :owner AND #intentos = :intentos":
		item := f.items[clave]
		if item == nil {
			return false, nil
		}
		return f.propietario(clave, valores[":owner"]) &&
			stringAttribute(item["attempts"]) == stringAttribute(valores[":intentos"]), nil
	default:
		return false, fmt.Errorf("el doble de DynamoDB no soporta la condición %q", condicion)
	}
}

func (f *outboxFake) propietario(clave string, esperado types.AttributeValue) bool {
	item := f.items[clave]
	if item == nil {
		return false
	}
	return stringAttribute(item["owner"]) == stringAttribute(esperado)
}

// TransactWriteItems valida todas las condiciones antes de aplicar: o se mueven
// las dos claves, o no se mueve ninguna.
func (f *outboxFake) TransactWriteItems(_ context.Context, params *dynamodb.TransactWriteItemsInput, _ ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.transacciones = append(f.transacciones, params)
	for _, accion := range params.TransactItems {
		if accion.Put != nil {
			clave := claveItemInput(&dynamodb.PutItemInput{Item: accion.Put.Item})
			ok, err := f.cumpleCondicion(clave, aws.ToString(accion.Put.ConditionExpression), accion.Put.ExpressionAttributeValues)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, &types.TransactionCanceledException{Message: aws.String("TransactionCanceledException")}
			}
			continue
		}
		if accion.Delete != nil {
			clave := claveUpdateInput(&dynamodb.UpdateItemInput{Key: accion.Delete.Key})
			ok, err := f.cumpleCondicion(clave, aws.ToString(accion.Delete.ConditionExpression), accion.Delete.ExpressionAttributeValues)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, &types.TransactionCanceledException{Message: aws.String("TransactionCanceledException")}
			}
			continue
		}
		return nil, errors.New("el doble de DynamoDB solo soporta Put y Delete en transacción")
	}
	for _, accion := range params.TransactItems {
		if accion.Put != nil {
			f.items[claveItemInput(&dynamodb.PutItemInput{Item: accion.Put.Item})] = accion.Put.Item
		}
		if accion.Delete != nil {
			delete(f.items, claveUpdateInput(&dynamodb.UpdateItemInput{Key: accion.Delete.Key}))
		}
	}
	return &dynamodb.TransactWriteItemsOutput{}, nil
}

func (f *outboxFake) UpdateItem(_ context.Context, params *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updates = append(f.updates, params)
	clave := claveUpdateInput(params)
	item := f.items[clave]
	if item == nil {
		return nil, &types.ConditionalCheckFailedException{}
	}
	expresion := aws.ToString(params.ConditionExpression)
	estado := stringAttribute(item["status"])
	if strings.Contains(expresion, "#estado = :pendiente") {
		if estado != string(pushoutbox.EstadoPendiente) && estado != string(pushoutbox.EstadoFallido) &&
			estado != string(pushoutbox.EstadoEnviando) {
			return nil, &types.ConditionalCheckFailedException{}
		}
	}
	if strings.Contains(expresion, "#leaseUntil < :ahora") {
		lease := stringAttribute(item["leaseUntil"])
		if lease != "" && lease >= stringAttribute(params.ExpressionAttributeValues[":ahora"]) {
			return nil, &types.ConditionalCheckFailedException{}
		}
	}
	if strings.Contains(expresion, "#siguiente <= :ahora") {
		siguiente := stringAttribute(item["nextAttemptAt"])
		if siguiente != "" && siguiente > stringAttribute(params.ExpressionAttributeValues[":ahora"]) {
			return nil, &types.ConditionalCheckFailedException{}
		}
	}
	if strings.Contains(expresion, "#owner = :owner") {
		if !f.propietario(clave, params.ExpressionAttributeValues[":owner"]) {
			return nil, &types.ConditionalCheckFailedException{}
		}
	}
	actualizado := make(map[string]types.AttributeValue, len(item)+4)
	for k, v := range item {
		actualizado[k] = v
	}
	updateExpr := aws.ToString(params.UpdateExpression)
	valores := params.ExpressionAttributeValues
	switch {
	case strings.Contains(updateExpr, "#estado = :enviando"):
		previos, _ := strconv.Atoi(stringAttribute(item["attempts"]))
		actualizado["status"] = stringAttr(string(pushoutbox.EstadoEnviando))
		actualizado["owner"] = valores[":owner"]
		actualizado["leaseUntil"] = valores[":lease"]
		actualizado["attempts"] = numberAttr(int64(previos + 1))
	case strings.Contains(updateExpr, "#estado = :enviado"):
		actualizado["status"] = stringAttr(string(pushoutbox.EstadoEnviado))
	case strings.Contains(updateExpr, "#estado = :revocado"):
		actualizado["status"] = stringAttr(string(pushoutbox.EstadoRevocado))
	case strings.Contains(updateExpr, "#estado = :fallido"):
		actualizado["status"] = stringAttr(string(pushoutbox.EstadoFallido))
	}
	if v, ok := valores[":intentos"]; ok {
		actualizado["attempts"] = v
	}
	if v, ok := valores[":siguiente"]; ok {
		actualizado["nextAttemptAt"] = v
	}
	if v, ok := valores[":clase"]; ok {
		actualizado["lastErrorClass"] = v
	}
	if v, ok := valores[":ahora"]; ok {
		actualizado["updatedAt"] = v
	}
	if strings.Contains(updateExpr, "REMOVE") {
		if strings.Contains(updateExpr, "#leaseUntil") {
			delete(actualizado, "leaseUntil")
		}
		if strings.Contains(updateExpr, "#owner") {
			delete(actualizado, "owner")
		}
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

func TestOUTBOXQueryPorVencimientoSinFilterNiScan(t *testing.T) {
	fake := nuevoOutboxFake()
	ahora := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	store := NewPushOutboxStore(fake, "NEXUM_CORE_TEST")
	creado, err := store.Crear(context.Background(), pushoutbox.Registro{
		UserID: "u1", NotificationID: "n1", Title: "Aviso", Message: "Mensaje",
		Type: "REMINDER", EventID: "evt-1", NextAttemptAt: ahora,
	})
	if err != nil || !creado {
		t.Fatalf("crear: %v %v", creado, err)
	}
	registros, err := store.Pendientes(context.Background(), ahora, 25)
	if err != nil || len(registros) != 1 {
		t.Fatalf("pendientes=%+v err=%v", registros, err)
	}
	if len(fake.consultas) != 1 {
		t.Fatalf("consultas=%d", len(fake.consultas))
	}
	entrada := fake.consultas[0]
	if entrada.FilterExpression != nil {
		t.Fatalf("Pendientes no debe filtrar en el servidor: %q", aws.ToString(entrada.FilterExpression))
	}
	if got := aws.ToString(entrada.KeyConditionExpression); got != "#pk = :pk AND #sk BETWEEN :desde AND :hasta" {
		t.Fatalf("KeyCondition=%q", got)
	}
	if got := stringAttribute(entrada.ExpressionAttributeValues[":desde"]); got != pushOutboxDuePrefix {
		t.Fatalf("cota inferior=%q", got)
	}
	if got, esperado := stringAttribute(entrada.ExpressionAttributeValues[":hasta"]), pushOutboxSKLimite(ahora); got != esperado {
		t.Fatalf("cota superior=%q esperado %q", got, esperado)
	}
	if aws.ToInt32(entrada.Limit) != 25 {
		t.Fatalf("limit=%d", aws.ToInt32(entrada.Limit))
	}
	if registros[0].Attempts != 0 || registros[0].Status != pushoutbox.EstadoPendiente ||
		registros[0].ID != pushOutboxSK(ahora, "n1") {
		t.Fatalf("proyección incorrecta: %+v", registros[0])
	}
	// El compilador impide un barrido global: PushOutboxAPI no declara Scan.
	tipo := reflect.TypeOf((*PushOutboxAPI)(nil)).Elem()
	if _, existe := tipo.MethodByName("Scan"); existe {
		t.Fatal("PushOutboxAPI no debe exponer Scan")
	}
}

func TestOUTBOXReclamoCondicionalDosNodos(t *testing.T) {
	fake := nuevoOutboxFake()
	ahora := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	id := pushOutboxSK(ahora, "n1")
	clave := "OUTBOX#GLOBAL|" + id
	fake.items[clave] = map[string]types.AttributeValue{
		"status": stringAttr("pending"), "attempts": numberAttr(0),
	}
	nodoA := NewPushOutboxStore(fake, "NEXUM_CORE_TEST").WithOwner("nodo-a")
	nodoB := NewPushOutboxStore(fake, "NEXUM_CORE_TEST").WithOwner("nodo-b")

	intentos, ok, err := nodoA.Reclamar(context.Background(), id, ahora, 30*time.Second)
	if err != nil || !ok || intentos != 1 {
		t.Fatalf("reclamo A: intentos=%d ok=%v err=%v", intentos, ok, err)
	}
	_, okB, errB := nodoB.Reclamar(context.Background(), id, ahora, 30*time.Second)
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

func TestOUTBOXCIERRESExigenOwnerYReprogramarMueveLaClave(t *testing.T) {
	fake := nuevoOutboxFake()
	store := NewPushOutboxStore(fake, "NEXUM_CORE_TEST").WithOwner("nodo-a")
	ahora := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	siguiente := ahora.Add(5 * time.Minute)
	ctx := context.Background()

	enviado := pushOutboxSK(ahora, "n1")
	revocado := pushOutboxSK(ahora, "n2")
	reprogramado := pushOutboxSK(ahora, "n3")
	ajeno := pushOutboxSK(ahora, "n4")
	for _, id := range []string{enviado, revocado, reprogramado, ajeno} {
		fake.items["OUTBOX#GLOBAL|"+id] = map[string]types.AttributeValue{
			"PK":             stringAttr(pushOutboxPartition),
			"SK":             stringAttr(id),
			"entityType":     stringAttr(pushOutboxEntityType),
			"notificationID": stringAttr(pushOutboxNotificationID(id)),
			"status":         stringAttr(string(pushoutbox.EstadoEnviando)),
			"owner":          stringAttr("nodo-a"),
			"attempts":       numberAttr(1),
		}
	}
	if err := store.MarcarEnviado(ctx, enviado, ahora); err != nil {
		t.Fatal(err)
	}
	if err := store.MarcarRevocado(ctx, revocado, "UNREGISTERED", ahora); err != nil {
		t.Fatal(err)
	}
	if err := store.Reprogramar(ctx, reprogramado, 2, "REINTENTABLE", siguiente, ahora); err != nil {
		t.Fatal(err)
	}
	for i, update := range fake.updates {
		if aws.ToString(update.ConditionExpression) != "#owner = :owner" {
			t.Fatalf("update %d sin condición de owner: %q", i, aws.ToString(update.ConditionExpression))
		}
	}
	if len(fake.updates) != 2 {
		t.Fatalf("updates=%d", len(fake.updates))
	}

	if len(fake.transacciones) != 1 {
		t.Fatalf("la reprogramación debe ser una transacción: %d", len(fake.transacciones))
	}
	transaccion := fake.transacciones[0].TransactItems
	if len(transaccion) != 2 || transaccion[0].Put == nil || transaccion[1].Delete == nil {
		t.Fatalf("la transacción debe mover la clave con Put + Delete: %+v", transaccion)
	}
	nuevo := transaccion[0].Put.Item
	if got, esperado := stringAttribute(nuevo["SK"]), pushOutboxSK(siguiente, "n3"); got != esperado {
		t.Fatalf("SK nueva=%q esperado %q", got, esperado)
	}
	if _, presente := nuevo["owner"]; presente {
		t.Fatal("el trabajo reprogramado no debe conservar el owner")
	}
	if _, presente := nuevo["leaseUntil"]; presente {
		t.Fatal("el trabajo reprogramado no debe conservar el lease")
	}
	if got := stringAttribute(nuevo["status"]); got != string(pushoutbox.EstadoFallido) {
		t.Fatalf("estado=%q", got)
	}
	if got := aws.ToString(transaccion[1].Delete.ConditionExpression); got != "#owner = :owner AND #intentos = :intentos" {
		t.Fatalf("condición de borrado=%q", got)
	}
	if _, existe := fake.items["OUTBOX#GLOBAL|"+reprogramado]; existe {
		t.Fatal("la clave anterior debió borrarse en la misma transacción")
	}
	if _, existe := fake.items["OUTBOX#GLOBAL|"+pushOutboxSK(siguiente, "n3")]; !existe {
		t.Fatal("la clave nueva debió quedar escrita")
	}

	otroNodo := NewPushOutboxStore(fake, "NEXUM_CORE_TEST").WithOwner("nodo-b")
	if err := otroNodo.Reprogramar(ctx, ajeno, 2, "REINTENTABLE", siguiente, ahora); !errors.Is(err, ErrConflictoLease) {
		t.Fatalf("otro nodo debió recibir conflicto de lease: %v", err)
	}
	if _, existe := fake.items["OUTBOX#GLOBAL|"+ajeno]; !existe {
		t.Fatal("un conflicto de lease no debe borrar el trabajo ajeno")
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
