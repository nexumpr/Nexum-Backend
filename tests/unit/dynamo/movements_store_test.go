package dynamo_test

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
)

// Pruebas del adaptador real de movimientos sobre un doble CON ESTADO que interpreta las mismas
// condiciones que emite el adaptador y reproduce la semántica de paginación de DynamoDB: el
// `Limit` cuenta ítems EVALUADOS y el `FilterExpression` se aplica después. Así se prueba el
// bucle de paginación real, no una versión simplificada.
//
// No se usa ningún mock del SDK: el doble implementa la interfaz `MovementsAPI`, que no incluye
// `Scan`, de modo que el compilador impide siquiera intentarlo.

const (
	movDynUsuario  = "11111111-2222-4333-8444-555555555555"
	movDynUsuarioB = "99999999-8888-4777-8666-555555555555"
	movDynTabla    = "NEXUM_CORE_TEST"
	movDynClave    = "clave-sintetica-de-prueba-0001"
)

var movDynAhora = time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)

// movDynamo es el doble con estado.
type movDynamo struct {
	mu        sync.Mutex
	items     map[string]map[string]types.AttributeValue
	consultas int
}

func newMovDynamo() *movDynamo {
	return &movDynamo{items: make(map[string]map[string]types.AttributeValue)}
}

func (m *movDynamo) sembrar(pk, sk string, attrs map[string]types.AttributeValue) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item := map[string]types.AttributeValue{"PK": strAttr(pk), "SK": strAttr(sk)}
	for k, v := range attrs {
		item[k] = v
	}
	m.items[claveDe(pk, sk)] = item
}

func (m *movDynamo) leer(pk, sk string) map[string]types.AttributeValue {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.items[claveDe(pk, sk)]
	if !ok {
		return nil
	}
	return copiarItem(item)
}

// contar devuelve cuántos ítems hay en una partición que cumplen el predicado de SK.
func (m *movDynamo) contar(pk, prefijo string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	total := 0
	for _, item := range m.items {
		if mapaString(item, "PK") != pk {
			continue
		}
		if strings.HasPrefix(mapaString(item, "SK"), prefijo) {
			total++
		}
	}
	return total
}

func (m *movDynamo) GetItem(_ context.Context, params *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.items[claveDe(mapaString(params.Key, "PK"), mapaString(params.Key, "SK"))]
	if !ok {
		return &dynamodb.GetItemOutput{}, nil
	}
	return &dynamodb.GetItemOutput{Item: copiarItem(item)}, nil
}

// Query reproduce la semántica real de DynamoDB para una única partición.
func (m *movDynamo) Query(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.consultas++

	expresion := aws.ToString(params.KeyConditionExpression)
	if !strings.Contains(expresion, "PK = :pk") || !strings.Contains(expresion, "begins_with(SK, :prefijo)") {
		return nil, errors.New("condicion de clave no soportada por el doble: " + expresion)
	}
	valores := params.ExpressionAttributeValues
	pk := movValorTexto(valores[":pk"])
	prefijo := movValorTexto(valores[":prefijo"])
	desde := movValorTexto(valores[":desde"])
	hasta := movValorTexto(valores[":hasta"])

	// 1. Claves que cumplen la CONDICIÓN DE CLAVE (lo que DynamoDB "lee").
	var claves []string
	for _, item := range m.items {
		if mapaString(item, "PK") != pk {
			continue
		}
		sk := mapaString(item, "SK")
		if !strings.HasPrefix(sk, prefijo) {
			continue
		}
		if desde != "" && sk < desde {
			continue
		}
		if hasta != "" && sk >= hasta {
			continue
		}
		claves = append(claves, sk)
	}
	sort.Strings(claves)

	// 2. ExclusiveStartKey: se reanuda estrictamente después de la clave indicada.
	inicio := ""
	if params.ExclusiveStartKey != nil {
		inicio = mapaString(params.ExclusiveStartKey, "SK")
	}
	posicion := 0
	if inicio != "" {
		posicion = sort.SearchStrings(claves, inicio)
		for posicion < len(claves) && claves[posicion] <= inicio {
			posicion++
		}
	}

	// 3. Limit sobre EVALUADOS.
	limite := int(aws.ToInt32(params.Limit))
	if limite <= 0 {
		limite = len(claves)
	}
	fin := posicion + limite
	if fin > len(claves) {
		fin = len(claves)
	}
	evaluados := claves[posicion:fin]

	// 4. FilterExpression DESPUÉS del límite.
	salida := make([]map[string]types.AttributeValue, 0, len(evaluados))
	for _, sk := range evaluados {
		item := m.items[claveDe(pk, sk)]
		if params.FilterExpression != nil {
			ok, err := evaluarCondicion(aws.ToString(params.FilterExpression), item, params.ExpressionAttributeNames, valores)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
		}
		salida = append(salida, copiarItem(item))
	}

	// 5. LastEvaluatedKey sólo si quedan claves por evaluar.
	var ultima map[string]types.AttributeValue
	if fin < len(claves) && len(evaluados) > 0 {
		ultima = map[string]types.AttributeValue{
			"PK": strAttr(pk),
			"SK": strAttr(evaluados[len(evaluados)-1]),
		}
	}
	return &dynamodb.QueryOutput{Items: salida, LastEvaluatedKey: ultima}, nil
}

func (m *movDynamo) TransactWriteItems(_ context.Context, params *dynamodb.TransactWriteItemsInput, _ ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Fase 1: evaluar TODAS las condiciones antes de escribir nada (atomicidad real).
	for indice := range params.TransactItems {
		if err := m.comprobar(params.TransactItems[indice]); err != nil {
			razones := make([]types.CancellationReason, len(params.TransactItems))
			for i := range razones {
				razones[i] = types.CancellationReason{Code: aws.String("None")}
			}
			razones[indice] = types.CancellationReason{Code: aws.String("ConditionalCheckFailed")}
			return nil, &types.TransactionCanceledException{
				Message:             aws.String("Transaction cancelled"),
				CancellationReasons: razones,
			}
		}
	}
	// Fase 2: aplicar.
	for _, item := range params.TransactItems {
		switch {
		case item.Put != nil:
			pk := mapaString(item.Put.Item, "PK")
			sk := mapaString(item.Put.Item, "SK")
			m.items[claveDe(pk, sk)] = copiarItem(item.Put.Item)
		case item.Update != nil:
			m.aplicarUpdate(item.Update)
		case item.Delete != nil:
			delete(m.items, claveDe(mapaString(item.Delete.Key, "PK"), mapaString(item.Delete.Key, "SK")))
		case item.ConditionCheck != nil:
			// Sólo condición: ya evaluada.
		}
	}
	return &dynamodb.TransactWriteItemsOutput{}, nil
}

func (m *movDynamo) comprobar(item types.TransactWriteItem) error {
	var (
		clave   map[string]types.AttributeValue
		expr    *string
		nombres map[string]string
		valores map[string]types.AttributeValue
	)
	switch {
	case item.Put != nil:
		clave = item.Put.Item
		expr, nombres, valores = item.Put.ConditionExpression, item.Put.ExpressionAttributeNames, item.Put.ExpressionAttributeValues
	case item.Update != nil:
		clave = item.Update.Key
		expr, nombres, valores = item.Update.ConditionExpression, item.Update.ExpressionAttributeNames, item.Update.ExpressionAttributeValues
	case item.Delete != nil:
		clave = item.Delete.Key
		expr, nombres, valores = item.Delete.ConditionExpression, item.Delete.ExpressionAttributeNames, item.Delete.ExpressionAttributeValues
	case item.ConditionCheck != nil:
		clave = item.ConditionCheck.Key
		expr, nombres, valores = item.ConditionCheck.ConditionExpression, item.ConditionCheck.ExpressionAttributeNames, item.ConditionCheck.ExpressionAttributeValues
	}
	if expr == nil {
		return nil
	}
	existente := m.items[claveDe(mapaString(clave, "PK"), mapaString(clave, "SK"))]
	ok, err := evaluarCondicion(aws.ToString(expr), existente, nombres, valores)
	if err != nil {
		return err
	}
	if !ok {
		return &types.TransactionCanceledException{
			Message:             aws.String("Transaction cancelled"),
			CancellationReasons: []types.CancellationReason{{Code: aws.String("ConditionalCheckFailed")}},
		}
	}
	return nil
}

func (m *movDynamo) aplicarUpdate(update *types.Update) {
	pk := mapaString(update.Key, "PK")
	sk := mapaString(update.Key, "SK")
	item := m.items[claveDe(pk, sk)]
	if item == nil {
		item = map[string]types.AttributeValue{"PK": strAttr(pk), "SK": strAttr(sk)}
	}
	expresion := strings.TrimSpace(aws.ToString(update.UpdateExpression))
	set, add := expresion, ""
	if indice := strings.Index(expresion, " ADD "); indice >= 0 {
		set, add = expresion[:indice], strings.TrimSpace(expresion[indice+len(" ADD "):])
	}
	set = strings.TrimPrefix(strings.TrimSpace(set), "SET ")
	for _, asignacion := range strings.Split(set, ", ") {
		partes := strings.SplitN(asignacion, " = ", 2)
		if len(partes) != 2 {
			continue
		}
		item[resolverNombre(partes[0], update.ExpressionAttributeNames)] = update.ExpressionAttributeValues[partes[1]]
	}
	if add != "" {
		for _, incremento := range strings.Split(add, ", ") {
			partes := strings.SplitN(incremento, " ", 2)
			if len(partes) != 2 {
				continue
			}
			nombre := resolverNombre(partes[0], update.ExpressionAttributeNames)
			delta := update.ExpressionAttributeValues[strings.TrimSpace(partes[1])]
			actual, hayActual := item[nombre].(*types.AttributeValueMemberN)
			salto, haySalto := delta.(*types.AttributeValueMemberN)
			if !haySalto {
				continue
			}
			var base int64
			if hayActual {
				base, _ = strconv.ParseInt(actual.Value, 10, 64)
			}
			incrementoValor, _ := strconv.ParseInt(salto.Value, 10, 64)
			item[nombre] = numAttr(base + incrementoValor)
		}
	}
	m.items[claveDe(pk, sk)] = item
}

// movValorTexto devuelve el valor textual de un placeholder ("" si no existe).
func movValorTexto(valor types.AttributeValue) string {
	if valor == nil {
		return ""
	}
	return stringAttributeDePrueba(valor)
}

func stringAttributeDePrueba(valor types.AttributeValue) string {
	switch v := valor.(type) {
	case *types.AttributeValueMemberS:
		return v.Value
	case *types.AttributeValueMemberN:
		return v.Value
	default:
		return ""
	}
}

// ---------- Utilidades de fixture ----------

func movDynMovimiento(id string, ocurrido time.Time, tipo string) appMovements.Movement {
	return appMovements.Movement{
		ID:          id,
		Type:        tipo,
		AmountCents: 100000,
		Currency:    "MXN",
		CategoryID:  "food",
		Description: "Ejemplo de concepto",
		OccurredAt:  ocurrido,
		Version:     1,
	}
}

func movDynEscritura(usuario string, movimiento appMovements.Movement, hash string, clave string, ahora time.Time) appMovements.CreateWrite {
	cuerpo, _ := appMovements.MarshalMovement(movimiento)
	return appMovements.CreateWrite{
		UserID:         usuario,
		Movement:       movimiento,
		IdempotencyKey: clave,
		Operation:      appMovements.OperationCreateMovement,
		PayloadHash:    hash,
		ResponseBody:   cuerpo,
		Location:       "/api/v1/movements/" + movimiento.ID,
		Status:         appMovements.StatusCreated,
		Now:            ahora,
		ExpiresAt:      ahora.Add(appMovements.IdempotencyTTL),
	}
}

// TestE028StoreCreaPrincipalPunteroEIdempotencia comprueba las tres acciones de la transacción y
// que la clave principal es exactamente la que produce `domainDynamo.MovementKeys`.
func TestE028StoreCreaPrincipalPunteroEIdempotencia(t *testing.T) {
	doble := newMovDynamo()
	store := dynamoAdapter.NewMovementsStore(doble, movDynTabla)

	movimiento := movDynMovimiento("018f90ef-08e5-7a68-b09f-000000000001", movDynAhora.Add(-time.Hour), "expense")
	escritura := movDynEscritura(movDynUsuario, movimiento, "hash-sintetico-uno", movDynClave, movDynAhora)

	resultado, err := store.CreateMovement(context.Background(), escritura)
	if err != nil {
		t.Fatalf("alta inesperada: %v", err)
	}
	if resultado.Replayed {
		t.Fatal("el primer alta no es un replay")
	}
	if resultado.Status != appMovements.StatusCreated || len(resultado.Body) == 0 {
		t.Fatalf("resultado inesperado: %+v", resultado)
	}

	pk, sk, gsi1pk, gsi1sk := domainDynamo.MovementKeys(movDynUsuario, movimiento.ID, movimiento.Type, movimiento.OccurredAt)
	principal := doble.leer(pk, sk)
	if principal == nil {
		t.Fatalf("no se escribio el item principal en %s / %s", pk, sk)
	}
	if mapaString(principal, "entityType") != "USER_MOVEMENT" {
		t.Fatalf("entityType inesperado: %q", mapaString(principal, "entityType"))
	}
	if movValorTexto(principal["amountCents"]) != "100000" {
		t.Fatalf("importe persistido inesperado: %q", movValorTexto(principal["amountCents"]))
	}
	if movValorTexto(principal["version"]) != "1" {
		t.Fatalf("version persistida inesperada: %q", movValorTexto(principal["version"]))
	}
	if mapaString(principal, "GSI1PK") != gsi1pk || mapaString(principal, "GSI1SK") != gsi1sk {
		t.Fatalf("atributos GSI1 inesperados: %q / %q", mapaString(principal, "GSI1PK"), mapaString(principal, "GSI1SK"))
	}
	if mapaString(principal, "occurredAt") != appMovements.SortableTime(movimiento.OccurredAt) {
		t.Fatalf("occurredAt persistido inesperado: %q", mapaString(principal, "occurredAt"))
	}

	ptrPK, ptrSK := dynamoAdapter.MovementPointerKeys(movDynUsuario, movimiento.ID)
	puntero := doble.leer(ptrPK, ptrSK)
	if puntero == nil {
		t.Fatalf("no se escribio el puntero en %s / %s", ptrPK, ptrSK)
	}
	if mapaString(puntero, "targetSK") != sk {
		t.Fatalf("el puntero debe apuntar a la SK principal: %q != %q", mapaString(puntero, "targetSK"), sk)
	}
	if strings.HasPrefix(ptrSK, domainDynamo.PrefixMovement) {
		// El listado usa begins_with(SK, "MOVEMENT#") y no debe devolver punteros.
		t.Fatal("la SK del puntero no puede empezar por MOVEMENT#")
	}

	idemPK, idemSK := domainDynamo.UserIdempotencyKeys(movDynUsuario, appMovements.OperationCreateMovement, movDynClave)
	registro := doble.leer(idemPK, idemSK)
	if registro == nil {
		t.Fatalf("no se escribio el registro de idempotencia en %s / %s", idemPK, idemSK)
	}
	if mapaString(registro, "payloadHash") != "hash-sintetico-uno" {
		t.Fatalf("payloadHash inesperado: %q", mapaString(registro, "payloadHash"))
	}
	if mapaString(registro, "resourceId") != movimiento.ID {
		t.Fatalf("resourceId inesperado: %q", mapaString(registro, "resourceId"))
	}
	if mapaString(registro, "responseBody") != string(escritura.ResponseBody) {
		t.Fatal("el cuerpo persistido debe ser identico al original")
	}
	if movValorTexto(registro["httpStatus"]) != "201" {
		t.Fatalf("httpStatus inesperado: %q", movValorTexto(registro["httpStatus"]))
	}
	if movValorTexto(registro["expiresAt"]) == "" {
		t.Fatal("el registro debe declarar vencimiento logico")
	}
}

// TestE028StoreReplayYConflicto comprueba que la misma clave con el mismo payload reproduce el
// 201 sin crear un segundo recurso, y que un payload distinto produce 409 sin escribir nada.
func TestE028StoreReplayYConflicto(t *testing.T) {
	doble := newMovDynamo()
	store := dynamoAdapter.NewMovementsStore(doble, movDynTabla)

	movimiento := movDynMovimiento("018f90ef-08e5-7a68-b09f-000000000002", movDynAhora, "expense")
	primera := movDynEscritura(movDynUsuario, movimiento, "hash-uno", movDynClave, movDynAhora)
	if _, err := store.CreateMovement(context.Background(), primera); err != nil {
		t.Fatalf("alta inesperada: %v", err)
	}

	segunda, err := store.CreateMovement(context.Background(), primera)
	if err != nil {
		t.Fatalf("replay inesperado: %v", err)
	}
	if !segunda.Replayed {
		t.Fatal("la repeticion identica debe marcarse como replay")
	}
	if string(segunda.Body) != string(primera.ResponseBody) {
		t.Fatal("el replay debe devolver exactamente el cuerpo original")
	}
	if total := doble.contar(domainDynamo.PrefixUser+movDynUsuario, domainDynamo.PrefixMovement); total != 1 {
		t.Fatalf("el replay no debe crear otro movimiento: %d", total)
	}

	distinta := primera
	distinta.PayloadHash = "hash-dos"
	if _, err := store.CreateMovement(context.Background(), distinta); !errors.Is(err, appMovements.ErrConflict) {
		t.Fatalf("se esperaba conflicto y se obtuvo %v", err)
	}
	if total := doble.contar(domainDynamo.PrefixUser+movDynUsuario, domainDynamo.PrefixMovement); total != 1 {
		t.Fatalf("un conflicto no puede escribir: %d", total)
	}
}

// TestE028StoreRegistroExpiradoPermiteNuevaOperacion comprueba que el vencimiento lógico libera
// la clave sin dejar residuo bloqueante.
func TestE028StoreRegistroExpiradoPermiteNuevaOperacion(t *testing.T) {
	doble := newMovDynamo()
	store := dynamoAdapter.NewMovementsStore(doble, movDynTabla)

	idemPK, idemSK := domainDynamo.UserIdempotencyKeys(movDynUsuario, appMovements.OperationCreateMovement, movDynClave)
	doble.sembrar(idemPK, idemSK, map[string]types.AttributeValue{
		"entityType":   strAttr("IDEMPOTENCY_RECORD"),
		"userId":       strAttr(movDynUsuario),
		"payloadHash":  strAttr("hash-viejo"),
		"httpStatus":   numAttr(201),
		"responseBody": strAttr(`{"id":"viejo"}`),
		"expiresAt":    numAttr(movDynAhora.Add(-time.Minute).Unix()),
	})

	movimiento := movDynMovimiento("018f90ef-08e5-7a68-b09f-000000000003", movDynAhora, "expense")
	escritura := movDynEscritura(movDynUsuario, movimiento, "hash-nuevo", movDynClave, movDynAhora)

	resultado, err := store.CreateMovement(context.Background(), escritura)
	if err != nil {
		t.Fatalf("un registro vencido no debe bloquear: %v", err)
	}
	if resultado.Replayed {
		t.Fatal("con un registro vencido y otro payload debe crearse un recurso nuevo")
	}
	if total := doble.contar(domainDynamo.PrefixUser+movDynUsuario, domainDynamo.PrefixMovement); total != 1 {
		t.Fatalf("se esperaba un movimiento y hay %d", total)
	}
}

// TestE028StoreAislamientoEntreUsuarios comprueba que la misma clave y el mismo payload en dos
// cuentas producen dos recursos independientes y ninguno ve el cuerpo del otro.
func TestE028StoreAislamientoEntreUsuarios(t *testing.T) {
	doble := newMovDynamo()
	store := dynamoAdapter.NewMovementsStore(doble, movDynTabla)

	movimientoA := movDynMovimiento("018f90ef-08e5-7a68-b09f-00000000000a", movDynAhora, "expense")
	movimientoB := movDynMovimiento("018f90ef-08e5-7a68-b09f-00000000000b", movDynAhora, "expense")

	if _, err := store.CreateMovement(context.Background(), movDynEscritura(movDynUsuario, movimientoA, "hash-compartido", movDynClave, movDynAhora)); err != nil {
		t.Fatalf("alta A inesperada: %v", err)
	}
	replayB, err := store.CreateMovement(context.Background(), movDynEscritura(movDynUsuarioB, movimientoB, "hash-compartido", movDynClave, movDynAhora))
	if err != nil {
		t.Fatalf("alta B inesperada: %v", err)
	}
	if replayB.Replayed {
		t.Fatal("la cuenta B no puede recibir el replay de la cuenta A")
	}
	if total := doble.contar(domainDynamo.PrefixUser+movDynUsuarioB, domainDynamo.PrefixMovement); total != 1 {
		t.Fatalf("la cuenta B debe tener su propio movimiento: %d", total)
	}
	if total := doble.contar(domainDynamo.PrefixUser+movDynUsuario, domainDynamo.PrefixMovement); total != 1 {
		t.Fatalf("la cuenta A conserva su movimiento: %d", total)
	}

	var cuerpo map[string]any
	if err := json.Unmarshal(replayB.Body, &cuerpo); err != nil {
		t.Fatalf("cuerpo de B no deserializable: %v", err)
	}
	if cuerpo["id"] != movimientoB.ID {
		t.Fatalf("el cuerpo de B debe describir su propio recurso: %v", cuerpo["id"])
	}
}

// TestE028StoreConcurrencia comprueba que la condición del registro de idempotencia impide
// duplicados cuando varias peticiones idénticas entran a la vez.
func TestE028StoreConcurrencia(t *testing.T) {
	doble := newMovDynamo()
	store := dynamoAdapter.NewMovementsStore(doble, movDynTabla)

	movimiento := movDynMovimiento("018f90ef-08e5-7a68-b09f-00000000000c", movDynAhora, "expense")
	escritura := movDynEscritura(movDynUsuario, movimiento, "hash-concurrente", movDynClave, movDynAhora)

	const peticiones = 16
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		creados int
		replays int
		fallos  []error
	)
	for i := 0; i < peticiones; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resultado, err := store.CreateMovement(context.Background(), escritura)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				fallos = append(fallos, err)
				return
			}
			if resultado.Replayed {
				replays++
			} else {
				creados++
			}
		}()
	}
	wg.Wait()

	if len(fallos) != 0 {
		t.Fatalf("fallos inesperados: %v", fallos)
	}
	if creados != 1 || replays != peticiones-1 {
		t.Fatalf("se esperaba 1 alta y %d replays; hubo %d y %d", peticiones-1, creados, replays)
	}
	if total := doble.contar(domainDynamo.PrefixUser+movDynUsuario, domainDynamo.PrefixMovement); total != 1 {
		t.Fatalf("solo puede existir un movimiento: %d", total)
	}
}

// TestE027StoreListadoPaginado comprueba orden ascendente, paginación sin duplicados y ausencia
// de punteros en el listado.
func TestE027StoreListadoPaginado(t *testing.T) {
	doble := newMovDynamo()
	store := dynamoAdapter.NewMovementsStore(doble, movDynTabla)

	for i := 0; i < 5; i++ {
		id := "018f90ef-08e5-7a68-b09f-00000000010" + string(rune('0'+i))
		ocurrido := movDynAhora.Add(time.Duration(i) * time.Hour)
		movimiento := movDynMovimiento(id, ocurrido, "expense")
		clave := "clave-sintetica-de-prueba-10" + string(rune('0'+i))
		if _, err := store.CreateMovement(context.Background(), movDynEscritura(movDynUsuario, movimiento, "hash-"+id, clave, movDynAhora)); err != nil {
			t.Fatalf("alta %d inesperada: %v", i, err)
		}
	}

	var (
		vistos  []string
		inicio  string
		paginas int
	)
	for {
		items, siguiente, err := store.ListMovements(context.Background(), appMovements.ListQuery{UserID: movDynUsuario}, inicio, 2)
		if err != nil {
			t.Fatalf("listado inesperado: %v", err)
		}
		paginas++
		if paginas > 10 {
			t.Fatal("la paginacion no termina")
		}
		for _, item := range items {
			vistos = append(vistos, item.ID)
		}
		if siguiente == "" {
			break
		}
		inicio = siguiente
	}

	if len(vistos) != 5 {
		t.Fatalf("se esperaban 5 movimientos y se obtuvieron %d (%v)", len(vistos), vistos)
	}
	for i := 1; i < len(vistos); i++ {
		if vistos[i-1] >= vistos[i] {
			t.Fatalf("el orden debe ser ascendente y sin duplicados: %v", vistos)
		}
	}

	// El listado de otra cuenta no ve nada y no consulta la partición ajena.
	ajenos, _, err := store.ListMovements(context.Background(), appMovements.ListQuery{UserID: movDynUsuarioB}, "", 10)
	if err != nil {
		t.Fatalf("listado B inesperado: %v", err)
	}
	if len(ajenos) != 0 {
		t.Fatalf("la cuenta B no debe ver movimientos ajenos: %d", len(ajenos))
	}
}

// TestE027StoreFiltrosYLimites comprueba `from`/`to` sobre la SK y el filtro por tipo.
func TestE027StoreFiltrosYLimites(t *testing.T) {
	doble := newMovDynamo()
	store := dynamoAdapter.NewMovementsStore(doble, movDynTabla)

	base := movDynAhora.Add(-10 * time.Hour)
	for i := 0; i < 6; i++ {
		tipo := "expense"
		if i%2 == 0 {
			tipo = "income"
		}
		id := "018f90ef-08e5-7a68-b09f-00000000020" + string(rune('0'+i))
		movimiento := movDynMovimiento(id, base.Add(time.Duration(i)*time.Hour), tipo)
		clave := "clave-sintetica-de-prueba-20" + string(rune('0'+i))
		if _, err := store.CreateMovement(context.Background(), movDynEscritura(movDynUsuario, movimiento, "hash-"+id, clave, movDynAhora)); err != nil {
			t.Fatalf("alta %d inesperada: %v", i, err)
		}
	}

	desde := base.Add(1 * time.Hour)
	hasta := base.Add(4 * time.Hour)
	items, _, err := store.ListMovements(context.Background(), appMovements.ListQuery{
		UserID: movDynUsuario, HasFrom: true, From: desde, HasTo: true, To: hasta,
	}, "", 10)
	if err != nil {
		t.Fatalf("listado con rango inesperado: %v", err)
	}
	if len(items) != 4 {
		t.Fatalf("se esperaban 4 movimientos en el rango y hay %d", len(items))
	}
	for _, item := range items {
		if item.OccurredAt.Before(desde) || item.OccurredAt.After(hasta) {
			t.Fatalf("movimiento fuera del rango: %v", item.OccurredAt)
		}
	}

	ingresos, _, err := store.ListMovements(context.Background(), appMovements.ListQuery{UserID: movDynUsuario, Type: "income"}, "", 10)
	if err != nil {
		t.Fatalf("listado filtrado inesperado: %v", err)
	}
	if len(ingresos) != 3 {
		t.Fatalf("se esperaban 3 ingresos y hay %d", len(ingresos))
	}
	for _, item := range ingresos {
		if item.Type != "income" {
			t.Fatalf("el filtro devolvio un tipo distinto: %q", item.Type)
		}
	}

	// Con filtro, el `Limit` cuenta EVALUADOS: el adaptador debe iterar hasta llenar la página.
	primeraPagina, siguiente, err := store.ListMovements(context.Background(), appMovements.ListQuery{UserID: movDynUsuario, Type: "income"}, "", 2)
	if err != nil {
		t.Fatalf("listado filtrado paginado inesperado: %v", err)
	}
	if len(primeraPagina) != 2 {
		t.Fatalf("se esperaban 2 ingresos en la primera pagina y hay %d", len(primeraPagina))
	}
	if siguiente == "" {
		t.Fatal("debe existir cursor para el ingreso restante")
	}
	resto, siguiente2, err := store.ListMovements(context.Background(), appMovements.ListQuery{UserID: movDynUsuario, Type: "income"}, siguiente, 2)
	if err != nil {
		t.Fatalf("segunda pagina inesperada: %v", err)
	}
	if len(resto) != 1 || siguiente2 != "" {
		t.Fatalf("se esperaba 1 ingreso y fin de paginacion: %d / %q", len(resto), siguiente2)
	}
}

// TestE027StoreSinPerdidaConFiltroSelectivo comprueba que, cuando el filtro apenas coincide y la
// página se agota en consultas evaluadas, el cursor devuelto permite seguir sin perder filas.
func TestE027StoreSinPerdidaConFiltroSelectivo(t *testing.T) {
	doble := newMovDynamo()
	store := dynamoAdapter.NewMovementsStore(doble, movDynTabla)

	total := 60
	for i := 0; i < total; i++ {
		tipo := "expense"
		if i >= total-3 {
			tipo = "saving"
		}
		id := "018f90ef-08e5-7a68-b09f-00000000" + movEntero(i)
		movimiento := movDynMovimiento(id, movDynAhora.Add(time.Duration(i)*time.Minute), tipo)
		clave := "clave-sintetica-de-prueba-3" + movEntero(i)
		if _, err := store.CreateMovement(context.Background(), movDynEscritura(movDynUsuario, movimiento, "hash-"+id, clave, movDynAhora)); err != nil {
			t.Fatalf("alta %d inesperada: %v", i, err)
		}
	}

	var (
		vistos  []string
		inicio  string
		paginas int
	)
	for {
		items, siguiente, err := store.ListMovements(context.Background(), appMovements.ListQuery{UserID: movDynUsuario, Type: "saving"}, inicio, 5)
		if err != nil {
			t.Fatalf("listado inesperado: %v", err)
		}
		paginas++
		if paginas > 40 {
			t.Fatal("la paginacion no converge: posible perdida de filas")
		}
		for _, item := range items {
			vistos = append(vistos, item.ID)
		}
		if siguiente == "" {
			break
		}
		inicio = siguiente
	}

	if len(vistos) != 3 {
		t.Fatalf("se esperaban los 3 ahorros y se obtuvieron %d", len(vistos))
	}
	unicos := map[string]struct{}{}
	for _, id := range vistos {
		if _, repetido := unicos[id]; repetido {
			t.Fatalf("movimiento duplicado entre paginas: %s", id)
		}
		unicos[id] = struct{}{}
	}
}

// movEntero formatea un entero con tres dígitos para construir identificadores predecibles.
func movEntero(valor int) string {
	cientos := string(rune('0' + valor/100))
	decenas := string(rune('0' + (valor/10)%10))
	unidades := string(rune('0' + valor%10))
	return cientos + decenas + unidades
}

// TestE028StoreFailClosed comprueba que sin cliente o sin tabla no se inventa una respuesta.
func TestE028StoreFailClosed(t *testing.T) {
	store := dynamoAdapter.NewMovementsStore(nil, "")
	movimiento := movDynMovimiento("018f90ef-08e5-7a68-b09f-000000000030", movDynAhora, "expense")
	if _, err := store.CreateMovement(context.Background(), movDynEscritura(movDynUsuario, movimiento, "hash", movDynClave, movDynAhora)); !errors.Is(err, appMovements.ErrUnavailable) {
		t.Fatalf("se esperaba 503 sin persistencia y se obtuvo %v", err)
	}
	if _, _, err := store.ListMovements(context.Background(), appMovements.ListQuery{UserID: movDynUsuario}, "", 10); !errors.Is(err, appMovements.ErrUnavailable) {
		t.Fatalf("se esperaba 503 sin persistencia y se obtuvo %v", err)
	}
}
