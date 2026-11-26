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

	appNotifications "github.com/nexumpr/Nexum-Backend/internal/application/notifications"
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
	deletes       []*dynamodb.DeleteItemInput
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
	return claveDeItem(params.Item)
}

func claveDeItem(item map[string]types.AttributeValue) string {
	return stringAttribute(item["PK"]) + "|" + stringAttribute(item["SK"])
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
	case "#skGlobal = :vieja":
		item := f.items[clave]
		if item == nil {
			return false, nil
		}
		return stringAttribute(item["skGlobal"]) == stringAttribute(valores[":vieja"]), nil
	case "attribute_exists(PK)":
		_, existe := f.items[clave]
		return existe, nil
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
// TransactWriteItems valida todas las condiciones antes de aplicar: o se mueven
// todas las acciones, o no se mueve ninguna. Soporta Put, Delete, Update y
// ConditionCheck, con una razón de cancelación por índice.
func (f *outboxFake) TransactWriteItems(_ context.Context, in *dynamodb.TransactWriteItemsInput, _ ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.transacciones = append(f.transacciones, in)

	razones := make([]types.CancellationReason, len(in.TransactItems))
	for i := range razones {
		razones[i] = types.CancellationReason{Code: aws.String("None")}
	}
	fallo := false
	for i, accion := range in.TransactItems {
		ok, err := f.condicionDeAccion(accion)
		if err != nil {
			return nil, err
		}
		if !ok {
			razones[i] = types.CancellationReason{Code: aws.String("ConditionalCheckFailed")}
			fallo = true
		}
	}
	if fallo {
		return nil, &types.TransactionCanceledException{CancellationReasons: razones}
	}

	for _, accion := range in.TransactItems {
		switch {
		case accion.Put != nil:
			f.items[claveDeItem(accion.Put.Item)] = accion.Put.Item
		case accion.Delete != nil:
			delete(f.items, claveUpdateInput(&dynamodb.UpdateItemInput{Key: accion.Delete.Key}))
		case accion.Update != nil:
			clave := claveUpdateInput(&dynamodb.UpdateItemInput{Key: accion.Update.Key})
			actualizado := map[string]types.AttributeValue{}
			for k, v := range f.items[clave] {
				actualizado[k] = v
			}
			for nombre, valor := range aplicarSetSimple(in, accion.Update) {
				actualizado[nombre] = valor
			}
			f.items[clave] = actualizado
		}
	}
	return &dynamodb.TransactWriteItemsOutput{}, nil
}

// condicionDeAccion evalúa la condición de cualquier acción transaccional.
func (f *outboxFake) condicionDeAccion(accion types.TransactWriteItem) (bool, error) {
	switch {
	case accion.Put != nil:
		return f.cumpleCondicion(claveDeItem(accion.Put.Item), aws.ToString(accion.Put.ConditionExpression), accion.Put.ExpressionAttributeValues)
	case accion.Delete != nil:
		clave := claveUpdateInput(&dynamodb.UpdateItemInput{Key: accion.Delete.Key})
		return f.cumpleCondicion(clave, aws.ToString(accion.Delete.ConditionExpression), accion.Delete.ExpressionAttributeValues)
	case accion.Update != nil:
		clave := claveUpdateInput(&dynamodb.UpdateItemInput{Key: accion.Update.Key})
		return f.cumpleCondicion(clave, aws.ToString(accion.Update.ConditionExpression), accion.Update.ExpressionAttributeValues)
	case accion.ConditionCheck != nil:
		item := f.items[claveDeItem(accion.ConditionCheck.Key)]
		if item == nil {
			return false, nil
		}
		esperado := stringAttribute(accion.ConditionCheck.ExpressionAttributeValues[":activo"])
		return item["status"] != nil && stringAttribute(item["status"]) == esperado, nil
	}
	return false, fmt.Errorf("acción transaccional no soportada por el doble")
}

// aplicarSetSimple aplica `SET #x = :v, #y = :w` de una acción Update.
func aplicarSetSimple(_ *dynamodb.TransactWriteItemsInput, accion *types.Update) map[string]types.AttributeValue {
	salida := map[string]types.AttributeValue{}
	expresion := strings.TrimPrefix(aws.ToString(accion.UpdateExpression), "SET ")
	for _, clausula := range strings.Split(expresion, ",") {
		pares := strings.SplitN(clausula, " = ", 2)
		if len(pares) != 2 {
			continue
		}
		nombre := strings.TrimSpace(pares[0])
		if strings.HasPrefix(nombre, "#") {
			nombre = accion.ExpressionAttributeNames[nombre]
		}
		valor, ok := accion.ExpressionAttributeValues[strings.TrimSpace(pares[1])]
		if !ok || nombre == "" {
			continue
		}
		salida[nombre] = valor
	}
	return salida
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
	anteriores := map[string]types.AttributeValue{}
	for k, v := range item {
		anteriores[k] = v
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
	if params.ReturnValues == types.ReturnValueAllOld {
		return &dynamodb.UpdateItemOutput{Attributes: anteriores}, nil
	}
	return &dynamodb.UpdateItemOutput{Attributes: actualizado}, nil
}

// DeleteItem interpreta las dos formas que usa el outbox: borrado del puntero (sin condición)
// y borrado del ítem global con guarda de lease.
func (f *outboxFake) DeleteItem(_ context.Context, params *dynamodb.DeleteItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deletes = append(f.deletes, params)
	clave := claveDeItem(params.Key)
	item, existe := f.items[clave]
	if params.ConditionExpression != nil {
		expresion := aws.ToString(params.ConditionExpression)
		if strings.Contains(expresion, "attribute_exists(PK)") && !existe {
			return nil, &types.ConditionalCheckFailedException{}
		}
		if strings.Contains(expresion, "#owner") {
			owner := ""
			lease := ""
			if existe {
				owner = stringAttribute(item["owner"])
				lease = stringAttribute(item["leaseUntil"])
			}
			ahora := stringAttribute(params.ExpressionAttributeValues[":ahora"])
			libre := owner == ""
			if !libre && lease != "" && lease < ahora {
				libre = true
			}
			if !libre {
				return nil, &types.ConditionalCheckFailedException{}
			}
		}
	}
	delete(f.items, clave)
	return &dynamodb.DeleteItemOutput{}, nil
}

// TransactWriteItems interpreta las tres acciones de `Crear`: dos `Put` condicionales y un
func TestOUTBOXCrearEscribeClaveDUEYCondicional(t *testing.T) {
	fake := nuevoOutboxFake()
	sembrarPerfilActivo(fake, "u1")
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
	if len(fake.transacciones) != 1 {
		t.Fatalf("transacciones=%d", len(fake.transacciones))
	}
	acciones := fake.transacciones[0].TransactItems
	if len(acciones) != 3 {
		t.Fatalf("la transaccion debe tener 3 acciones (global, puntero, perfil): %d", len(acciones))
	}
	global := acciones[0].Put
	if global == nil {
		t.Fatal("la primera accion debe ser el Put del item global")
	}
	if got := stringAttribute(global.Item["PK"]); got != pushOutboxPartition {
		t.Fatalf("PK=%q", got)
	}
	esperado := "DUE#" + ahora.Format(time.RFC3339Nano) + "#n1"
	if got := stringAttribute(global.Item["SK"]); got != esperado {
		t.Fatalf("SK=%q esperado %q", got, esperado)
	}
	if aws.ToString(global.ConditionExpression) != "attribute_not_exists(PK)" {
		t.Fatalf("condicion=%q", aws.ToString(global.ConditionExpression))
	}
	puntero := acciones[1].Put
	if puntero == nil {
		t.Fatal("la segunda accion debe ser el Put del puntero de purga")
	}
	if got := stringAttribute(puntero.Item["PK"]); got != "USER#u1" {
		t.Fatalf("PK del puntero=%q", got)
	}
	if got := stringAttribute(puntero.Item["SK"]); got != "OUTBOXPTR#n1" {
		t.Fatalf("SK del puntero=%q", got)
	}
	if got := stringAttribute(puntero.Item["skGlobal"]); got != esperado {
		t.Fatalf("skGlobal=%q esperado %q", got, esperado)
	}
	if got := stringAttribute(puntero.Item["userID"]); got != "u1" {
		t.Fatalf("userID del puntero=%q", got)
	}
	if acciones[2].ConditionCheck == nil {
		t.Fatal("la tercera accion debe ser el ConditionCheck del perfil")
	}

	// El puntero y el item global existen tras la transaccion: E014 puede encontrarlos.
	if _, ok := fake.items["USER#u1|OUTBOXPTR#n1"]; !ok {
		t.Fatal("el puntero debe persistirse en la misma transaccion")
	}
	if _, ok := fake.items[pushOutboxPartition+"|"+esperado]; !ok {
		t.Fatal("el item global debe persistirse")
	}

	// Replay de la misma clave: no crea otro item ni otro puntero.
	creadoOtraVez, err := store.Crear(context.Background(), pushoutbox.Registro{
		UserID: "u1", NotificationID: "n1", EventID: "evt-1", NextAttemptAt: ahora,
	})
	if err != nil || creadoOtraVez {
		t.Fatalf("replay creó otro ítem: %v %v", creadoOtraVez, err)
	}
}

// TestOUTBOXCrearRechazaCuentaNoActiva comprueba la guarda de productor: sin perfil ACTIVE no
// se encola nada (ni item global ni puntero).
func TestOUTBOXCrearRechazaCuentaNoActiva(t *testing.T) {
	for nombre, estado := range map[string]string{
		"perfil ausente":   "",
		"perfil en purga":  "DELETION_PENDING",
		"perfil eliminado": "DELETED",
	} {
		fake := nuevoOutboxFake()
		if estado != "" {
			fake.items["USER#u1|PROFILE"] = map[string]types.AttributeValue{"status": stringAttr(estado)}
		}
		store := NewPushOutboxStore(fake, "NEXUM_CORE_TEST")
		creado, err := store.Crear(context.Background(), pushoutbox.Registro{
			UserID: "u1", NotificationID: "n1", EventID: "evt-1", Title: "A", Message: "M",
		})
		if !errors.Is(err, ErrCuentaNoActiva) || creado {
			t.Fatalf("%s: esperado ErrCuentaNoActiva, obtenido creado=%v err=%v", nombre, creado, err)
		}
		esperados := 1
		if estado == "" {
			esperados = 0
		}
		if len(fake.items) != esperados {
			t.Fatalf("%s: items=%d, esperados=%d (no debe quedar ningun item de outbox)", nombre, len(fake.items), esperados)
		}
		if _, ok := fake.items["USER#u1|OUTBOXPTR#n1"]; ok {
			t.Fatalf("%s: no debe crearse el puntero", nombre)
		}
	}
}

// sembrarPerfilActivo crea el perfil mínimo que exige la guarda de cuenta.
func sembrarPerfilActivo(fake *outboxFake, userID string) {
	fake.items["USER#"+userID+"|PROFILE"] = map[string]types.AttributeValue{
		"status": stringAttr("ACTIVE"),
	}
}

func TestOUTBOXQueryPorVencimientoSinFilterNiScan(t *testing.T) {
	fake := nuevoOutboxFake()
	sembrarPerfilActivo(fake, "u1")
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
	sembrarPerfilActivo(fake, "u1")
	ahora := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	id := pushOutboxSK(ahora, "n1")
	clave := "OUTBOX#GLOBAL|" + id
	fake.items[clave] = map[string]types.AttributeValue{
		"userID": stringAttr("u1"), "notificationID": stringAttr("n1"),
		"status": stringAttr("pending"), "attempts": numberAttr(0),
		"nextAttemptAt": stringAttr(pushOutboxInstante(ahora)),
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
		if update.ReturnValues != types.ReturnValueAllOld {
			t.Fatalf("update %d debe pedir los atributos anteriores para retirar el puntero", i)
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

// TestOUTBOXWorkerDescartaTrabajoDeCuentaNoActiva comprueba la guarda del worker: con el perfil
// en purga o ausente no se reclama ni se envía; el trabajo y el puntero se retiran.
func TestOUTBOXWorkerDescartaTrabajoDeCuentaNoActiva(t *testing.T) {
	for nombre, estado := range map[string]string{
		"perfil ausente":  "",
		"perfil en purga": "DELETION_PENDING",
	} {
		fake := nuevoOutboxFake()
		if estado != "" {
			fake.items["USER#u1|PROFILE"] = map[string]types.AttributeValue{"status": stringAttr(estado)}
		}
		id := "DUE#2026-11-22T12:00:00Z#n1"
		fake.items["OUTBOX#GLOBAL|"+id] = map[string]types.AttributeValue{
			"status": stringAttr("pending"), "attempts": numberAttr(0),
			"userID": stringAttr("u1"), "notificationID": stringAttr("n1"),
		}
		fake.items["USER#u1|OUTBOXPTR#n1"] = map[string]types.AttributeValue{
			"skGlobal": stringAttr(id), "userID": stringAttr("u1"),
		}
		store := NewPushOutboxStore(fake, "NEXUM_CORE_TEST").WithOwner("nodo-a")
		intentos, ok, err := store.Reclamar(context.Background(), id, time.Now().UTC(), 30*time.Second)
		if err != nil || ok || intentos != 0 {
			t.Fatalf("%s: no debe reclamarse el trabajo: intentos=%d ok=%v err=%v", nombre, intentos, ok, err)
		}
		if len(fake.updates) != 0 {
			t.Fatalf("%s: no debe emitirse un Update de envio", nombre)
		}
		if _, existe := fake.items["OUTBOX#GLOBAL|"+id]; existe {
			t.Fatalf("%s: el trabajo debe retirarse", nombre)
		}
		if _, existe := fake.items["USER#u1|OUTBOXPTR#n1"]; existe {
			t.Fatalf("%s: el puntero debe retirarse", nombre)
		}
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

	// El registro de dedupe contiene identificadores vinculables: debe llevar TTL
	// (90 días) para no quedar permanentemente en la tabla.
	idem := fake.items[pushOutboxPartition+"|"+pushOutboxIdemSK("u1", "evt-1")]
	if idem == nil {
		t.Fatal("no se encontró el registro de idempotencia")
	}
	expira, ok := attributeInt(idem["expiresAt"])
	if !ok {
		t.Fatal("el dedupe global no declara expiresAt")
	}
	esperado := ahora.Add(90 * 24 * time.Hour).Unix()
	if expira != esperado {
		t.Fatalf("expiresAt=%d, esperado %d", expira, esperado)
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
	// El worker y E061 deben usar el MISMO entityType. Si diverge, el aviso se
	// escribe pero la bandeja nunca lo lista (defecto real de v0.21.2).
	if got := stringAttribute(bandeja.Item["entityType"]); got != NotificationEntityType {
		t.Fatalf("entityType bandeja=%q, el lector de E061 espera %q", got, NotificationEntityType)
	}
	if boolAttribute(bandeja.Item["isRead"]) {
		t.Fatal("la notificación nueva debe estar no leída")
	}
	// El caller pasó "REMINDER" (el valor histórico del scheduler). La bandeja sólo
	// admite el enum contractual, así que se persiste normalizado a `system`.
	if got := stringAttribute(bandeja.Item["type"]); got != string(appNotifications.TypeSystem) {
		t.Fatalf("type bandeja=%q, esperado %q", got, appNotifications.TypeSystem)
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

// TestOUTBOXTerminalRetiraGlobalYPuntero cubre la retirada segura del trabajo y de
// su puntero de purga cuando el envío termina (E014 no debe encontrar punteros
// colgando de ítems ya retirados).
func TestOUTBOXTerminalRetiraGlobalYPuntero(t *testing.T) {
	fake := nuevoOutboxFake()
	store := NewPushOutboxStore(fake, "NEXUM_CORE_TEST").WithOwner("nodo-a")
	ahora := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	id := pushOutboxSK(ahora, "n1")
	fake.items["OUTBOX#GLOBAL|"+id] = map[string]types.AttributeValue{
		"PK": stringAttr(pushOutboxPartition), "SK": stringAttr(id),
		"entityType": stringAttr(pushOutboxEntityType), "notificationID": stringAttr("n1"),
		"userID": stringAttr("u1"), "status": stringAttr(string(pushoutbox.EstadoEnviando)),
		"owner": stringAttr("nodo-a"), "attempts": numberAttr(1),
	}
	fake.items["USER#u1|OUTBOXPTR#n1"] = map[string]types.AttributeValue{
		"PK": stringAttr("USER#u1"), "SK": stringAttr("OUTBOXPTR#n1"),
		"entityType": stringAttr(pushOutboxPointerEntity), "userID": stringAttr("u1"),
		"notificationID": stringAttr("n1"), "skGlobal": stringAttr(id),
	}
	if err := store.MarcarEnviado(context.Background(), id, ahora); err != nil {
		t.Fatal(err)
	}
	if _, existe := fake.items["OUTBOX#GLOBAL|"+id]; existe {
		t.Fatal("el trabajo enviado debe retirarse")
	}
	if _, existe := fake.items["USER#u1|OUTBOXPTR#n1"]; existe {
		t.Fatal("el puntero debe retirarse al cerrar el trabajo")
	}
}
