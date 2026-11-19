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
	appRecurring "github.com/nexumpr/Nexum-Backend/internal/application/recurring"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
	domainRecurring "github.com/nexumpr/Nexum-Backend/internal/domain/recurring"
)

// Pruebas del adaptador REAL `RecurringStore` montado sobre un doble con estado que interpreta
// las mismas expresiones que emite el adaptador y falla si aparece otra forma: no adivina.
//
// Cubren la transacción atómica de E033 (recurso + idempotencia), el replay/409, el
// aislamiento por usuario, la consulta por partición sin Scan, el CAS por versión de E035, la
// desactivación idempotente de E036 y la proyección determinista de los ítems heredados del
// alta (D-E032-01).

const (
	e033Tabla    = "NEXUM_CORE_TEST"
	e033Usuario  = "11111111-2222-4333-8444-555555555555"
	e033Otro     = "99999999-8888-4777-8666-555555555555"
	e033Clave    = "clave-idempotencia-e033-0001"
	e033ID       = "018f90ef-08e5-7a68-b09f-2b67a47c92f0"
	e033IDOtro   = "018f90ef-08e5-7a68-b09f-2b67a47c92f1"
	e033HoyTexto = "2026-11-18"
)

var e033Hoy = time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)

func attrS(valor string) types.AttributeValue { return &types.AttributeValueMemberS{Value: valor} }
func attrN(valor int64) types.AttributeValue {
	return &types.AttributeValueMemberN{Value: strconv.FormatInt(valor, 10)}
}
func attrB(valor bool) types.AttributeValue { return &types.AttributeValueMemberBOOL{Value: valor} }

// memoriaRecurrentes implementa `dynamoAdapter.RecurringAPI` con estado.
type memoriaRecurrentes struct {
	mu                 sync.Mutex
	items              map[string]map[string]types.AttributeValue
	getItems           int
	consultas          int
	escrituras         int
	lecturaConsistente bool
	ultimaTransaccion  *dynamodb.TransactWriteItemsInput
	ultimaQuery        *dynamodb.QueryInput
	ultimaUpdate       *dynamodb.UpdateItemInput
	fallo              error
}

func newMemoriaRecurrentes() *memoriaRecurrentes {
	return &memoriaRecurrentes{items: map[string]map[string]types.AttributeValue{}}
}

func claveItem(pk, sk string) string { return pk + "|" + sk }

func copiaItem(item map[string]types.AttributeValue) map[string]types.AttributeValue {
	salida := make(map[string]types.AttributeValue, len(item))
	for clave, valor := range item {
		salida[clave] = valor
	}
	return salida
}

func (m *memoriaRecurrentes) GetItem(_ context.Context, params *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getItems++
	m.lecturaConsistente = aws.ToBool(params.ConsistentRead)
	if m.fallo != nil {
		return nil, m.fallo
	}
	item, ok := m.items[claveItem(textoAttr(params.Key["PK"]), textoAttr(params.Key["SK"]))]
	if !ok {
		return &dynamodb.GetItemOutput{}, nil
	}
	return &dynamodb.GetItemOutput{Item: copiaItem(item)}, nil
}

func (m *memoriaRecurrentes) Query(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.consultas++
	m.ultimaQuery = params
	if m.fallo != nil {
		return nil, m.fallo
	}

	if condicion := aws.ToString(params.KeyConditionExpression); condicion != "PK = :pk AND begins_with(SK, :prefix)" {
		return nil, errors.New("condicion de clave no soportada por el doble: " + condicion)
	}
	pk := textoAttr(params.ExpressionAttributeValues[":pk"])
	prefijo := textoAttr(params.ExpressionAttributeValues[":prefix"])

	claves := make([]string, 0, len(m.items))
	for clave, item := range m.items {
		if textoAttr(item["PK"]) != pk || !strings.HasPrefix(textoAttr(item["SK"]), prefijo) {
			continue
		}
		claves = append(claves, clave)
	}
	sort.Strings(claves)

	desde := ""
	if len(params.ExclusiveStartKey) > 0 {
		desde = textoAttr(params.ExclusiveStartKey["SK"])
	}
	filtrados := make([]map[string]types.AttributeValue, 0, len(claves))
	for _, clave := range claves {
		item := m.items[clave]
		if desde != "" && textoAttr(item["SK"]) <= desde {
			continue
		}
		coincide, err := e033EvaluarFiltro(aws.ToString(params.FilterExpression), item, params.ExpressionAttributeValues)
		if err != nil {
			return nil, err
		}
		if !coincide {
			continue
		}
		filtrados = append(filtrados, copiaItem(item))
	}

	limite := int(aws.ToInt32(params.Limit))
	if limite <= 0 || limite > len(filtrados) {
		limite = len(filtrados)
	}
	pagina := filtrados[:limite]
	salida := &dynamodb.QueryOutput{Items: pagina}
	if len(filtrados) > limite {
		ultimo := pagina[len(pagina)-1]
		salida.LastEvaluatedKey = map[string]types.AttributeValue{
			"PK": ultimo["PK"],
			"SK": ultimo["SK"],
		}
	}
	return salida, nil
}

func (m *memoriaRecurrentes) UpdateItem(_ context.Context, params *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ultimaUpdate = params
	if m.fallo != nil {
		return nil, m.fallo
	}

	clave := claveItem(textoAttr(params.Key["PK"]), textoAttr(params.Key["SK"]))
	item, existe := m.items[clave]

	ok, err := e033EvaluarCondicion(aws.ToString(params.ConditionExpression), item, existe, params.ExpressionAttributeValues)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, &types.ConditionalCheckFailedException{Message: aws.String("condicion no cumplida")}
	}

	nuevo := map[string]types.AttributeValue{}
	if existe {
		nuevo = copiaItem(item)
	}
	expresion := aws.ToString(params.UpdateExpression)
	if strings.Contains(expresion, "REMOVE dueDay") {
		delete(nuevo, "dueDay")
	}
	for marcador, valor := range params.ExpressionAttributeValues {
		nombre := strings.TrimPrefix(marcador, ":")
		if nombre == "true" || nombre == "false" {
			// `SET #active = :true|:false` de E036.
			if strings.Contains(expresion, "#active = :"+nombre) {
				nuevo["active"] = valor
			}
			continue
		}
		if nombre == "expected" || nombre == "one" || nombre == "now" {
			continue
		}
		alias := "#" + nombre
		destino := nombre
		if real, ok := params.ExpressionAttributeNames[alias]; ok {
			destino = real
		}
		if strings.Contains(expresion, alias+" = ") || strings.Contains(expresion, " "+nombre+" = ") {
			nuevo[destino] = valor
		}
	}
	if strings.Contains(expresion, "ADD #version :one") {
		actual, _ := numeroAttr(nuevo["version"])
		nuevo["version"] = attrN(actual + 1)
	}
	if _, tienePK := nuevo["PK"]; !tienePK {
		nuevo["PK"] = params.Key["PK"]
		nuevo["SK"] = params.Key["SK"]
	}
	m.items[clave] = nuevo
	return &dynamodb.UpdateItemOutput{Attributes: copiaItem(nuevo)}, nil
}

func (m *memoriaRecurrentes) TransactWriteItems(_ context.Context, params *dynamodb.TransactWriteItemsInput, _ ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ultimaTransaccion = params
	if m.fallo != nil {
		return nil, m.fallo
	}

	razones := make([]types.CancellationReason, len(params.TransactItems))
	fallo := false
	for indice, accion := range params.TransactItems {
		if accion.Put == nil {
			return nil, errors.New("el doble solo admite Put en transacciones")
		}
		clave := claveItem(textoAttr(accion.Put.Item["PK"]), textoAttr(accion.Put.Item["SK"]))
		item, existe := m.items[clave]
		ok, err := e033EvaluarCondicion(aws.ToString(accion.Put.ConditionExpression), item, existe, accion.Put.ExpressionAttributeValues)
		if err != nil {
			return nil, err
		}
		if ok {
			razones[indice] = types.CancellationReason{Code: aws.String("None")}
			continue
		}
		razones[indice] = types.CancellationReason{Code: aws.String("ConditionalCheckFailed")}
		fallo = true
	}
	if fallo {
		return nil, &types.TransactionCanceledException{
			Message:             aws.String("transaccion cancelada"),
			CancellationReasons: razones,
		}
	}

	for _, accion := range params.TransactItems {
		clave := claveItem(textoAttr(accion.Put.Item["PK"]), textoAttr(accion.Put.Item["SK"]))
		m.items[clave] = copiaItem(accion.Put.Item)
	}
	m.escrituras++
	return &dynamodb.TransactWriteItemsOutput{}, nil
}

// e033EvaluarCondicion interpreta EXACTAMENTE las condiciones que emite el adaptador.
func e033EvaluarCondicion(condicion string, item map[string]types.AttributeValue, existe bool, valores map[string]types.AttributeValue) (bool, error) {
	switch condicion {
	case "attribute_not_exists(PK)":
		return !existe, nil
	case "attribute_not_exists(PK) OR expiresAt < :now":
		if !existe {
			return true, nil
		}
		expira, ok := numeroAttr(item["expiresAt"])
		if !ok {
			return true, nil
		}
		ahora, _ := numeroAttr(valores[":now"])
		return expira < ahora, nil
	case "attribute_exists(PK) AND #version = :expected":
		if !existe {
			return false, nil
		}
		version, ok := numeroAttr(item["version"])
		if !ok {
			return false, nil
		}
		esperada, _ := numeroAttr(valores[":expected"])
		return version == esperada, nil
	case "attribute_exists(PK) AND (attribute_not_exists(#active) OR #active = :true)":
		if !existe {
			return false, nil
		}
		activo, tiene := booleanoAttr(item["active"])
		if !tiene {
			return true, nil
		}
		return activo, nil
	default:
		return false, errors.New("condicion no soportada por el doble: " + condicion)
	}
}

// e033EvaluarFiltro interpreta los filtros que emite `List`.
func e033EvaluarFiltro(filtro string, item map[string]types.AttributeValue, valores map[string]types.AttributeValue) (bool, error) {
	if filtro == "" {
		return true, nil
	}
	partes := strings.Split(filtro, " AND ")
	for _, parte := range partes {
		switch strings.TrimSpace(parte) {
		case "#active = :active":
			activo, ok := booleanoAttr(item["active"])
			if !ok {
				return false, nil
			}
			esperado, _ := booleanoAttr(valores[":active"])
			if activo != esperado {
				return false, nil
			}
		case "#type = :type":
			if textoAttr(item["type"]) != textoAttr(valores[":type"]) {
				return false, nil
			}
		default:
			return false, errors.New("filtro no soportado por el doble: " + parte)
		}
	}
	return true, nil
}

func textoAttr(valor types.AttributeValue) string {
	if cadena, ok := valor.(*types.AttributeValueMemberS); ok {
		return cadena.Value
	}
	return ""
}

func numeroAttr(valor types.AttributeValue) (int64, bool) {
	numero, ok := valor.(*types.AttributeValueMemberN)
	if !ok || numero.Value == "" {
		return 0, false
	}
	var resultado int64
	for _, digito := range numero.Value {
		if digito < '0' || digito > '9' {
			return 0, false
		}
		resultado = resultado*10 + int64(digito-'0')
	}
	return resultado, true
}

func booleanoAttr(valor types.AttributeValue) (bool, bool) {
	booleano, ok := valor.(*types.AttributeValueMemberBOOL)
	if !ok {
		return false, false
	}
	return booleano.Value, true
}

// e033Pago construye el recurso de referencia.
func e033Pago(id string) appRecurring.Payment {
	dia := 5
	return appRecurring.Payment{
		ID:          id,
		Name:        "Renta",
		AmountCents: 100000,
		Currency:    "MXN",
		Frequency:   "monthly",
		NextDueDate: time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC),
		Active:      true,
		Version:     1,
		Type:        "expense",
		StartDate:   time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC),
		DueDay:      &dia,
	}
}

// e033Write construye la escritura de E033 de referencia.
func e033Write(userID, clave, id string) appRecurring.CreateWrite {
	pago := e033Pago(id)
	cuerpo, _ := pago.Body()
	return appRecurring.CreateWrite{
		UserID:         userID,
		IdempotencyKey: clave,
		Operation:      appRecurring.OperationCreate,
		PayloadHash:    "hash-" + id,
		Body:           cuerpo,
		TTL:            appRecurring.IdempotencyTTL,
		Payment:        pago,
		Now:            e033Hoy,
	}
}

// TestE033StoreCreateTransaccionAtomica verifica la forma exacta de la transacción.
func TestE033StoreCreateTransaccionAtomica(t *testing.T) {
	memoria := newMemoriaRecurrentes()
	store := dynamoAdapter.NewRecurringStore(memoria, e033Tabla).WithClock(func() time.Time { return e033Hoy })

	escritura := e033Write(e033Usuario, e033Clave, e033ID)
	id, cuerpo, replayed, err := store.Create(context.Background(), escritura)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if id != e033ID || replayed {
		t.Fatalf("resultado inesperado: id=%q replayed=%v", id, replayed)
	}
	if string(cuerpo) != string(escritura.Body) {
		t.Fatal("la respuesta debe ser el cuerpo autoritativo")
	}

	transaccion := memoria.ultimaTransaccion
	if transaccion == nil || len(transaccion.TransactItems) != 2 {
		t.Fatalf("se esperaban 2 acciones, obtenido %+v", transaccion)
	}
	recurso := transaccion.TransactItems[0].Put
	if aws.ToString(recurso.ConditionExpression) != "attribute_not_exists(PK)" {
		t.Fatalf("condicion del recurso inesperada: %q", aws.ToString(recurso.ConditionExpression))
	}
	esperadoPK, esperadoSK := domainDynamo.RecurringKeys(e033Usuario, e033ID)
	if textoAttr(recurso.Item["PK"]) != esperadoPK || textoAttr(recurso.Item["SK"]) != esperadoSK {
		t.Fatalf("clave del recurso inesperada: %s/%s", textoAttr(recurso.Item["PK"]), textoAttr(recurso.Item["SK"]))
	}
	if textoAttr(recurso.Item["frequency"]) != "monthly" || textoAttr(recurso.Item["nextDueDate"]) != "2026-11-05" {
		t.Fatalf("item del recurso inesperado: %v", recurso.Item)
	}
	if textoAttr(recurso.Item["name"]) != "Renta" || textoAttr(recurso.Item["type"]) != "expense" {
		t.Fatalf("campos del contrato ausentes: %v", recurso.Item)
	}

	idempotencia := transaccion.TransactItems[1].Put
	condicion := aws.ToString(idempotencia.ConditionExpression)
	if condicion != "attribute_not_exists(PK) OR expiresAt < :now" {
		t.Fatalf("condicion de idempotencia inesperada: %q", condicion)
	}
	esperadoPKIdem, esperadoSKIdem := domainDynamo.UserIdempotencyKeys(e033Usuario, appRecurring.OperationCreate, e033Clave)
	if textoAttr(idempotencia.Item["PK"]) != esperadoPKIdem || textoAttr(idempotencia.Item["SK"]) != esperadoSKIdem {
		t.Fatalf("clave de idempotencia inesperada: %s/%s", textoAttr(idempotencia.Item["PK"]), textoAttr(idempotencia.Item["SK"]))
	}
	if textoAttr(idempotencia.Item["responseBody"]) == "" || textoAttr(idempotencia.Item["resourceId"]) != e033ID {
		t.Fatalf("el registro debe guardar cuerpo y recurso: %v", idempotencia.Item)
	}
	if _, ok := numeroAttr(idempotencia.Item["expiresAt"]); !ok {
		t.Fatal("el registro debe declarar expiresAt")
	}
	if !strings.Contains(esperadoSKIdem, "RECURRING") && !strings.Contains(esperadoSKIdem, appRecurring.OperationCreate) {
		t.Fatalf("la SK de idempotencia debe identificar la operacion: %q", esperadoSKIdem)
	}
}

// TestE033StoreReplayMismoPayload: la segunda petición no escribe y devuelve el cuerpo
// almacenado.
func TestE033StoreReplayMismoPayload(t *testing.T) {
	memoria := newMemoriaRecurrentes()
	store := dynamoAdapter.NewRecurringStore(memoria, e033Tabla).WithClock(func() time.Time { return e033Hoy })
	escritura := e033Write(e033Usuario, e033Clave, e033ID)

	if _, _, _, err := store.Create(context.Background(), escritura); err != nil {
		t.Fatalf("primera creacion: %v", err)
	}
	id, cuerpo, replayed, err := store.Create(context.Background(), escritura)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if !replayed || id != e033ID {
		t.Fatalf("se esperaba replay del mismo recurso: id=%q replayed=%v", id, replayed)
	}
	if string(cuerpo) != string(escritura.Body) {
		t.Fatal("el replay debe devolver exactamente el cuerpo almacenado")
	}
	if memoria.escrituras != 1 {
		t.Fatalf("el replay no puede volver a escribir, escrituras=%d", memoria.escrituras)
	}
	if !memoria.lecturaConsistente {
		t.Fatal("el replay debe leer con consistencia fuerte")
	}
}

// TestE033StoreConflictoPayloadDistinto: misma clave, otro contenido -> 409.
func TestE033StoreConflictoPayloadDistinto(t *testing.T) {
	memoria := newMemoriaRecurrentes()
	store := dynamoAdapter.NewRecurringStore(memoria, e033Tabla).WithClock(func() time.Time { return e033Hoy })
	if _, _, _, err := store.Create(context.Background(), e033Write(e033Usuario, e033Clave, e033ID)); err != nil {
		t.Fatalf("primera creacion: %v", err)
	}

	otra := e033Write(e033Usuario, e033Clave, e033IDOtro)
	otra.PayloadHash = "hash-distinto"
	if _, _, _, err := store.Create(context.Background(), otra); !errors.Is(err, appRecurring.ErrConflict) {
		t.Fatalf("se esperaba conflicto, obtenido %v", err)
	}
	if memoria.escrituras != 1 {
		t.Fatal("un conflicto no puede escribir")
	}
}

// TestE033StoreAislamientoEntreUsuarios: la misma clave en dos cuentas no comparte registro.
func TestE033StoreAislamientoEntreUsuarios(t *testing.T) {
	memoria := newMemoriaRecurrentes()
	store := dynamoAdapter.NewRecurringStore(memoria, e033Tabla).WithClock(func() time.Time { return e033Hoy })

	if _, _, _, err := store.Create(context.Background(), e033Write(e033Usuario, e033Clave, e033ID)); err != nil {
		t.Fatalf("alta de A: %v", err)
	}
	if _, _, replayed, err := store.Create(context.Background(), e033Write(e033Otro, e033Clave, e033IDOtro)); err != nil || replayed {
		t.Fatalf("B debe crear su propio recurso: replayed=%v err=%v", replayed, err)
	}
	if memoria.escrituras != 2 {
		t.Fatalf("cada cuenta debe tener su registro, escrituras=%d", memoria.escrituras)
	}

	// B no puede leer el recurso de A: la partición es la del titular.
	pago, err := store.Get(context.Background(), e033Otro, e033ID)
	if err != nil || pago != nil {
		t.Fatalf("B no debe ver el recurso de A: pago=%+v err=%v", pago, err)
	}
}

// TestE033StoreGetConsistente cubre la lectura fuerte de E034.
func TestE033StoreGetConsistente(t *testing.T) {
	memoria := newMemoriaRecurrentes()
	store := dynamoAdapter.NewRecurringStore(memoria, e033Tabla).WithClock(func() time.Time { return e033Hoy })
	if _, _, _, err := store.Create(context.Background(), e033Write(e033Usuario, e033Clave, e033ID)); err != nil {
		t.Fatalf("alta: %v", err)
	}

	pago, err := store.Get(context.Background(), e033Usuario, e033ID)
	if err != nil || pago == nil {
		t.Fatalf("get: pago=%+v err=%v", pago, err)
	}
	if !memoria.lecturaConsistente {
		t.Fatal("E034 debe leer con consistencia fuerte")
	}
	if pago.Name != "Renta" || pago.AmountCents != 100000 || pago.Frequency.String() != "monthly" {
		t.Fatalf("proyeccion inesperada: %+v", pago)
	}
	if pago.DueDay == nil || *pago.DueDay != 5 {
		t.Fatalf("dueDay esperado 5: %+v", pago.DueDay)
	}
}

// TestE033StoreListSinScanFiltraYPagina cubre T07 de E032: prefijo, filtros y cursor.
func TestE033StoreListSinScanFiltraYPagina(t *testing.T) {
	memoria := newMemoriaRecurrentes()
	store := dynamoAdapter.NewRecurringStore(memoria, e033Tabla).WithClock(func() time.Time { return e033Hoy })

	ids := []string{
		"018f90ef-08e5-7a68-b09f-2b67a47c92f0",
		"018f90ef-08e5-7a68-b09f-2b67a47c92f1",
		"018f90ef-08e5-7a68-b09f-2b67a47c92f2",
	}
	for indice, id := range ids {
		escritura := e033Write(e033Usuario, "clave-recurrente-"+id[len(id)-2:], id)
		escritura.Payment.Type = "expense"
		if indice == 2 {
			escritura.Payment.Type = "income"
		}
		if _, _, _, err := store.Create(context.Background(), escritura); err != nil {
			t.Fatalf("alta %d: %v", indice, err)
		}
	}

	pagina, nextSK, err := store.List(context.Background(), e033Usuario, "", 2, appRecurring.Filter{})
	if err != nil {
		t.Fatalf("pagina 1: %v", err)
	}
	if len(pagina) != 2 || nextSK == "" {
		t.Fatalf("se esperaban 2 elementos y cursor: %d / %q", len(pagina), nextSK)
	}
	if !strings.HasPrefix(nextSK, domainDynamo.PrefixRecurring) {
		t.Fatalf("el cursor debe apuntar a una SK de recurrente: %q", nextSK)
	}
	if condicion := aws.ToString(memoria.ultimaQuery.KeyConditionExpression); !strings.Contains(condicion, "begins_with") {
		t.Fatalf("la consulta debe usar begins_with, obtenido %q", condicion)
	}

	pagina2, nextSK2, err := store.List(context.Background(), e033Usuario, nextSK, 2, appRecurring.Filter{})
	if err != nil {
		t.Fatalf("pagina 2: %v", err)
	}
	if len(pagina2) != 1 || nextSK2 != "" {
		t.Fatalf("la pagina 2 debe agotar la lista: %d / %q", len(pagina2), nextSK2)
	}
	if vistos := map[string]bool{}; true {
		for _, pago := range append(append([]appRecurring.Payment{}, pagina...), pagina2...) {
			if vistos[pago.ID] {
				t.Fatalf("elemento duplicado entre paginas: %s", pago.ID)
			}
			vistos[pago.ID] = true
		}
	}

	filtrados, _, err := store.List(context.Background(), e033Usuario, "", 10, appRecurring.Filter{Type: "income"})
	if err != nil {
		t.Fatalf("filtro por tipo: %v", err)
	}
	if len(filtrados) != 1 || filtrados[0].Type != "income" {
		t.Fatalf("el filtro por tipo debe aplicarse en servidor: %+v", filtrados)
	}

	activos := true
	soloActivos, _, err := store.List(context.Background(), e033Usuario, "", 10, appRecurring.Filter{Active: &activos})
	if err != nil {
		t.Fatalf("filtro por estado: %v", err)
	}
	if len(soloActivos) != 3 {
		t.Fatalf("las tres reglas estan activas, obtenido %d", len(soloActivos))
	}
	if !strings.Contains(aws.ToString(memoria.ultimaQuery.FilterExpression), "#active = :active") {
		t.Fatalf("el filtro de estado debe ir en la consulta: %q", aws.ToString(memoria.ultimaQuery.FilterExpression))
	}
}

// TestE033StoreUpdateCAS cubre el compare-and-swap de E035 y el borrado de dueDay.
func TestE033StoreUpdateCAS(t *testing.T) {
	memoria := newMemoriaRecurrentes()
	store := dynamoAdapter.NewRecurringStore(memoria, e033Tabla).WithClock(func() time.Time { return e033Hoy })
	if _, _, _, err := store.Create(context.Background(), e033Write(e033Usuario, e033Clave, e033ID)); err != nil {
		t.Fatalf("alta: %v", err)
	}

	propuesto := e033Pago(e033ID)
	propuesto.Name = "Renta ajustada"
	propuesto.DueDay = nil

	if _, err := store.Update(context.Background(), appRecurring.UpdateWrite{
		UserID:          e033Usuario,
		PaymentID:       e033ID,
		ExpectedVersion: 99,
		Payment:         propuesto,
		Now:             e033Hoy,
	}); !errors.Is(err, appRecurring.ErrVersionConflict) {
		t.Fatalf("se esperaba conflicto de version, obtenido %v", err)
	}

	actualizado, err := store.Update(context.Background(), appRecurring.UpdateWrite{
		UserID:          e033Usuario,
		PaymentID:       e033ID,
		ExpectedVersion: 1,
		Payment:         propuesto,
		Now:             e033Hoy,
	})
	if err != nil {
		t.Fatalf("CAS valido: %v", err)
	}
	if actualizado.Name != "Renta ajustada" || actualizado.Version != 2 {
		t.Fatalf("actualizacion inesperada: %+v", actualizado)
	}
	if actualizado.DueDay != nil {
		t.Fatalf("dueDay debe haberse eliminado: %+v", actualizado.DueDay)
	}
	if !strings.Contains(aws.ToString(memoria.ultimaUpdate.UpdateExpression), "REMOVE dueDay") {
		t.Fatalf("la expresion debe eliminar dueDay: %q", aws.ToString(memoria.ultimaUpdate.UpdateExpression))
	}
}

// TestE033StoreDeactivateIdempotente cubre E036: desactivar sin borrar y sin repetir versión.
func TestE033StoreDeactivateIdempotente(t *testing.T) {
	memoria := newMemoriaRecurrentes()
	store := dynamoAdapter.NewRecurringStore(memoria, e033Tabla).WithClock(func() time.Time { return e033Hoy })
	if _, _, _, err := store.Create(context.Background(), e033Write(e033Usuario, e033Clave, e033ID)); err != nil {
		t.Fatalf("alta: %v", err)
	}

	existia, err := store.Deactivate(context.Background(), e033Usuario, e033ID, e033Hoy)
	if err != nil || !existia {
		t.Fatalf("desactivacion: existia=%v err=%v", existia, err)
	}
	pago, err := store.Get(context.Background(), e033Usuario, e033ID)
	if err != nil || pago == nil {
		t.Fatalf("lectura posterior: %+v %v", pago, err)
	}
	if pago.Active || pago.Version != 2 {
		t.Fatalf("la regla debe quedar inactiva en version 2: %+v", pago)
	}

	existia, err = store.Deactivate(context.Background(), e033Usuario, e033ID, e033Hoy)
	if err != nil || !existia {
		t.Fatalf("segunda desactivacion debe ser idempotente: existia=%v err=%v", existia, err)
	}
	pago, _ = store.Get(context.Background(), e033Usuario, e033ID)
	if pago.Version != 2 {
		t.Fatalf("una regla ya inactiva no incrementa version: %d", pago.Version)
	}

	existia, err = store.Deactivate(context.Background(), e033Usuario, e033IDOtro, e033Hoy)
	if err != nil || existia {
		t.Fatalf("un recurso inexistente no existe para desactivar: existia=%v err=%v", existia, err)
	}
}

// TestE033StoreItemHeredado cubre D-E032-01: proyección determinista del ítem del alta.
func TestE033StoreItemHeredado(t *testing.T) {
	memoria := newMemoriaRecurrentes()
	pk, sk := domainDynamo.RecurringKeys(e033Usuario, e033ID)
	memoria.items[claveItem(pk, sk)] = map[string]types.AttributeValue{
		"PK":          attrS(pk),
		"SK":          attrS(sk),
		"entityType":  attrS("RECURRING_PAYMENT"),
		"userId":      attrS(e033Usuario),
		"recurringId": attrS(e033ID),
		"concept":     attrS("Internet"),
		"currency":    attrS("MXN"),
		"amountCents": attrN(89900),
		"frequency":   attrS("biweekly"),
		"category":    attrS("Servicios"),
		"expenseType": attrS("fixed"),
		"createdAt":   attrS("2026-01-15T08:00:00Z"),
		"updatedAt":   attrS("2026-01-15T08:00:00Z"),
		"version":     attrN(1),
	}

	store := dynamoAdapter.NewRecurringStore(memoria, e033Tabla).WithClock(func() time.Time { return e033Hoy })
	pago, err := store.Get(context.Background(), e033Usuario, e033ID)
	if err != nil || pago == nil {
		t.Fatalf("el item heredado debe ser legible: %+v %v", pago, err)
	}
	if pago.Name != "Internet" {
		t.Fatalf("name debe venir de concept: %q", pago.Name)
	}
	if pago.Type != "expense" {
		t.Fatalf("expenseType=fixed debe proyectarse como gasto: %q", pago.Type)
	}
	if pago.Currency != "MXN" || pago.AmountCents != 89900 || pago.Version != 1 {
		t.Fatalf("campos base inesperados: %+v", pago)
	}
	if !pago.Active {
		t.Fatal("una regla del alta no esta cancelada: active debe ser true")
	}
	if pago.Frequency.String() != "fortnightly" {
		t.Fatalf("biweekly debe normalizarse a fortnightly: %q", pago.Frequency)
	}
	if inicio := domainRecurring.FormatDate(pago.StartDate); inicio != "2026-01-15" {
		t.Fatalf("startDate debe ser la fecha de createdAt: %q", inicio)
	}
	if pago.NextDueDate.Before(domainRecurring.DateOnly(e033Hoy)) {
		t.Fatalf("nextDueDate proyectada no puede ser pasada: %s", domainRecurring.FormatDate(pago.NextDueDate))
	}
	if pago.DueDay != nil {
		t.Fatalf("una frecuencia quincenal no admite dueDay: %+v", pago.DueDay)
	}

	// El listado devuelve el mismo ítem proyectado.
	lista, _, err := store.List(context.Background(), e033Usuario, "", 10, appRecurring.Filter{})
	if err != nil || len(lista) != 1 || lista[0].ID != e033ID {
		t.Fatalf("el listado debe incluir el item heredado: %+v %v", lista, err)
	}

	// E036 lo desactiva aunque no tenga `active`.
	existia, err := store.Deactivate(context.Background(), e033Usuario, e033ID, e033Hoy)
	if err != nil || !existia {
		t.Fatalf("desactivar un item heredado: existia=%v err=%v", existia, err)
	}
	pago, _ = store.Get(context.Background(), e033Usuario, e033ID)
	if pago == nil || pago.Active {
		t.Fatalf("el item heredado debe quedar inactivo: %+v", pago)
	}
}

// TestE033StoreFailClosed: un fallo de DynamoDB no se convierte en éxito ni en dato inventado.
func TestE033StoreFailClosed(t *testing.T) {
	memoria := newMemoriaRecurrentes()
	memoria.fallo = errors.New("dynamodb no disponible")
	store := dynamoAdapter.NewRecurringStore(memoria, e033Tabla).WithClock(func() time.Time { return e033Hoy })

	if _, _, _, err := store.Create(context.Background(), e033Write(e033Usuario, e033Clave, e033ID)); !errors.Is(err, appRecurring.ErrUnavailable) {
		t.Fatalf("create debe fallar cerrado, obtenido %v", err)
	}
	if _, err := store.Get(context.Background(), e033Usuario, e033ID); !errors.Is(err, appRecurring.ErrUnavailable) {
		t.Fatalf("get debe fallar cerrado, obtenido %v", err)
	}
	if _, _, err := store.List(context.Background(), e033Usuario, "", 10, appRecurring.Filter{}); !errors.Is(err, appRecurring.ErrUnavailable) {
		t.Fatalf("list debe fallar cerrado, obtenido %v", err)
	}
	if _, err := store.Deactivate(context.Background(), e033Usuario, e033ID, e033Hoy); !errors.Is(err, appRecurring.ErrUnavailable) {
		t.Fatalf("deactivate debe fallar cerrado, obtenido %v", err)
	}
	if _, err := dynamoAdapter.NewRecurringStore(nil, "").Get(context.Background(), e033Usuario, e033ID); !errors.Is(err, appRecurring.ErrUnavailable) {
		t.Fatalf("sin cliente debe fallar cerrado, obtenido %v", err)
	}
}

// TestE033StoreCuerpoSerializado cubre que el cuerpo almacenado sea el contrato exacto.
func TestE033StoreCuerpoSerializado(t *testing.T) {
	pago := e033Pago(e033ID)
	cuerpo, err := pago.Body()
	if err != nil {
		t.Fatalf("body: %v", err)
	}
	var campos map[string]any
	if err := json.Unmarshal(cuerpo, &campos); err != nil {
		t.Fatalf("body no es JSON: %v", err)
	}
	if len(campos) != 11 {
		t.Fatalf("el contrato tiene once campos, obtenidos %d: %v", len(campos), campos)
	}
	if campos["dueDay"] != float64(5) {
		t.Fatalf("dueDay debe serializarse como numero: %v", campos["dueDay"])
	}
	if _, ok := campos["concept"]; ok {
		t.Fatal("el cuerpo no puede exponer atributos del alta")
	}
}

// TestE032HoyTextoAncla garantiza que la constante de fecha usada en las pruebas es la que se
// inyecta en el reloj del adaptador.
func TestE032HoyTextoAncla(t *testing.T) {
	if domainRecurring.FormatDate(e033Hoy) != e033HoyTexto {
		t.Fatalf("la fecha de prueba debe ser %s, obtenido %s", e033HoyTexto, domainRecurring.FormatDate(e033Hoy))
	}
}
