package dynamo

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	appNotifications "github.com/nexumpr/Nexum-Backend/internal/application/notifications"
)

// fake cliente DynamoDB para E061–E063. Emula Query por PK/prefijo con
// ExclusiveStartKey, Select COUNT, UpdateItem/PutItem/DeleteItem condicionales
// y TransactWriteItems atómico. No hay Scan en la interfaz del adaptador.
type e061FakeClient struct {
	mu                sync.Mutex
	items             map[string]map[string]types.AttributeValue
	queries           []*dynamodb.QueryInput
	updates           []*dynamodb.UpdateItemInput
	puts              []*dynamodb.PutItemInput
	transacts         []*dynamodb.TransactWriteItemsInput
	queryErr          error
	getErr            error
	putErr            error
	updateErr         error
	transactErr       error
	fuerzaCondicional bool
	maxEvaluados      int
}

func nuevoE061Fake() *e061FakeClient {
	return &e061FakeClient{items: map[string]map[string]types.AttributeValue{}, maxEvaluados: 1000}
}

func claveItem(pk, sk string) string { return pk + "\x00" + sk }

func (f *e061FakeClient) sembrar(userID string, cantidad, noLeidas int, tipo appNotifications.Type) {
	base := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	pk := notificationUserPK(userID)
	for i := 0; i < cantidad; i++ {
		id := fmt.Sprintf("018f90ef-08e5-7a68-b09f-%012d", i+1)
		creado := base.Add(time.Duration(i) * time.Minute)
		sk := notificationSortKey(creado, id)
		f.items[claveItem(pk, sk)] = map[string]types.AttributeValue{
			"PK":             stringAttr(pk),
			"SK":             stringAttr(sk),
			"entityType":     stringAttr(NotificationEntityType),
			"id":             stringAttr(id),
			"title":          stringAttr("Aviso"),
			"message":        stringAttr("Mensaje"),
			"type":           stringAttr(string(tipo)),
			"isRead":         boolAttr(i >= noLeidas),
			"createdAt":      stringAttr(creado.Format(time.RFC3339Nano)),
			"deliveryStatus": stringAttr("DELIVERED"),
		}
	}
}

func (f *e061FakeClient) Query(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queries = append(f.queries, params)
	if f.queryErr != nil {
		return nil, f.queryErr
	}
	pk := stringAttribute(params.ExpressionAttributeValues[":pk"])
	prefijo := ""
	if valor, ok := params.ExpressionAttributeValues[":prefijo"]; ok {
		prefijo = stringAttribute(valor)
	}
	desde := ""
	if len(params.ExclusiveStartKey) > 0 {
		desde = stringAttribute(params.ExclusiveStartKey["SK"])
	}

	claves := make([]string, 0, len(f.items))
	for clave := range f.items {
		claves = append(claves, clave)
	}
	sort.Strings(claves)

	evaluados := 0
	filtrados := make([]map[string]types.AttributeValue, 0)
	var ultimaEvaluada map[string]types.AttributeValue
	for _, clave := range claves {
		item := f.items[clave]
		if stringAttribute(item["PK"]) != pk || !strings.HasPrefix(stringAttribute(item["SK"]), prefijo) {
			continue
		}
		if stringAttribute(item["SK"]) <= desde {
			continue
		}
		if params.Limit != nil && evaluados >= int(*params.Limit) {
			break
		}
		if f.maxEvaluados > 0 && evaluados >= f.maxEvaluados {
			break
		}
		evaluados++
		ultimaEvaluada = item
		if cumpleFiltroFalso(item, params) {
			filtrados = append(filtrados, item)
		}
	}

	quedan := false
	for _, clave := range claves {
		item := f.items[clave]
		if stringAttribute(item["PK"]) == pk && strings.HasPrefix(stringAttribute(item["SK"]), prefijo) &&
			stringAttribute(item["SK"]) > stringAttribute(ultimaEvaluada["SK"]) {
			quedan = true
			break
		}
	}
	if f.maxEvaluados > 0 && evaluados >= f.maxEvaluados {
		quedan = true
	}

	salida := &dynamodb.QueryOutput{Count: int32(len(filtrados)), ScannedCount: int32(evaluados)}
	if string(params.Select) == string(types.SelectCount) {
		salida.Items = nil
	} else {
		salida.Items = append(salida.Items, filtrados...)
	}
	if quedan && ultimaEvaluada != nil {
		salida.LastEvaluatedKey = map[string]types.AttributeValue{
			"PK": ultimaEvaluada["PK"], "SK": ultimaEvaluada["SK"],
		}
	}
	return salida, nil
}

func cumpleFiltroFalso(item map[string]types.AttributeValue, params *dynamodb.QueryInput) bool {
	// Si el adaptador emitió una FilterExpression real, se evalúa con el mismo
	// intérprete estricto que las condiciones (incluye el filtro de lease
	// `attribute_not_exists(leaseUntil) OR leaseUntil <= :now`).
	if expresion := strings.TrimSpace(aws.ToString(params.FilterExpression)); expresion != "" {
		cumple, err := evaluarCondicionE063(item, expresion, params.ExpressionAttributeNames, params.ExpressionAttributeValues)
		if err != nil {
			return false
		}
		return cumple
	}
	valores := params.ExpressionAttributeValues
	if valor, ok := valores[":notificacion"]; ok && stringAttribute(item["entityType"]) != stringAttribute(valor) {
		return false
	}
	if valor, ok := valores[":id"]; ok && stringAttribute(item["id"]) != stringAttribute(valor) {
		return false
	}
	if valor, ok := valores[":isRead"]; ok {
		esperado, ok := valor.(*types.AttributeValueMemberBOOL)
		if !ok {
			return false
		}
		actual, ok := item["isRead"].(*types.AttributeValueMemberBOOL)
		if !ok || actual.Value != esperado.Value {
			return false
		}
	}
	if valor, ok := valores[":type"]; ok && stringAttribute(item["type"]) != stringAttribute(valor) {
		return false
	}
	return true
}

func (f *e061FakeClient) GetItem(_ context.Context, params *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getErr != nil {
		return nil, f.getErr
	}
	clave := claveItem(stringAttribute(params.Key["PK"]), stringAttribute(params.Key["SK"]))
	return &dynamodb.GetItemOutput{Item: f.items[clave]}, nil
}

func (f *e061FakeClient) PutItem(_ context.Context, params *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.puts = append(f.puts, params)
	if f.putErr != nil {
		return nil, f.putErr
	}
	clave := claveItem(stringAttribute(params.Item["PK"]), stringAttribute(params.Item["SK"]))
	cumple, err := evaluarCondicionE063(f.items[clave], aws.ToString(params.ConditionExpression),
		params.ExpressionAttributeNames, params.ExpressionAttributeValues)
	if err != nil {
		return nil, err
	}
	if !cumple {
		return nil, &types.ConditionalCheckFailedException{}
	}
	f.items[clave] = params.Item
	return &dynamodb.PutItemOutput{}, nil
}

func (f *e061FakeClient) UpdateItem(_ context.Context, params *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updates = append(f.updates, params)
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	clave := claveItem(stringAttribute(params.Key["PK"]), stringAttribute(params.Key["SK"]))
	item := f.items[clave]
	if item == nil {
		return nil, &types.ConditionalCheckFailedException{}
	}
	if f.fuerzaCondicional {
		return nil, &types.ConditionalCheckFailedException{}
	}
	cumple, err := evaluarCondicionE063(item, aws.ToString(params.ConditionExpression),
		params.ExpressionAttributeNames, params.ExpressionAttributeValues)
	if err != nil {
		return nil, err // condición no soportada por el doble: fallo visible
	}
	if !cumple {
		return nil, &types.ConditionalCheckFailedException{}
	}
	actualizado, err := aplicarUpdateE063(item, aws.ToString(params.UpdateExpression),
		params.ExpressionAttributeNames, params.ExpressionAttributeValues)
	if err != nil {
		return nil, err
	}
	f.items[clave] = actualizado
	copia := copiarMapa(actualizado)
	return &dynamodb.UpdateItemOutput{Attributes: copia}, nil
}

// TransactWriteItems evalúa todas las condiciones antes de aplicar nada y
// cancela el conjunto si una falla, como el servicio real.
func (f *e061FakeClient) TransactWriteItems(_ context.Context, params *dynamodb.TransactWriteItemsInput, _ ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.transacts = append(f.transacts, params)
	if f.transactErr != nil {
		return nil, f.transactErr
	}
	razones := make([]types.CancellationReason, 0, len(params.TransactItems))
	fallo := false
	for _, accion := range params.TransactItems {
		cumple, err := f.condicionDeAccion(accion)
		if err != nil {
			return nil, err
		}
		if cumple {
			razones = append(razones, types.CancellationReason{Code: aws.String("None")})
			continue
		}
		razones = append(razones, types.CancellationReason{Code: aws.String("ConditionalCheckFailed")})
		fallo = true
	}
	if fallo {
		return nil, &types.TransactionCanceledException{
			Message:             aws.String("Transaction cancelled"),
			CancellationReasons: razones,
		}
	}
	for _, accion := range params.TransactItems {
		switch {
		case accion.Put != nil:
			item := copiarMapa(accion.Put.Item)
			f.items[claveItem(stringAttribute(item["PK"]), stringAttribute(item["SK"]))] = item
		case accion.Update != nil:
			clave := claveItem(stringAttribute(accion.Update.Key["PK"]), stringAttribute(accion.Update.Key["SK"]))
			actualizado, err := aplicarUpdateE063(f.items[clave], aws.ToString(accion.Update.UpdateExpression),
				accion.Update.ExpressionAttributeNames, accion.Update.ExpressionAttributeValues)
			if err != nil {
				return nil, err
			}
			f.items[clave] = actualizado
		case accion.Delete != nil:
			delete(f.items, claveItem(stringAttribute(accion.Delete.Key["PK"]), stringAttribute(accion.Delete.Key["SK"])))
		case accion.ConditionCheck != nil:
			// No escribe: sólo valida.
		default:
			return nil, fmt.Errorf("accion no soportada por el doble de E063")
		}
	}
	return &dynamodb.TransactWriteItemsOutput{}, nil
}

func (f *e061FakeClient) condicionDeAccion(accion types.TransactWriteItem) (bool, error) {
	switch {
	case accion.Put != nil:
		clave := claveItem(stringAttribute(accion.Put.Item["PK"]), stringAttribute(accion.Put.Item["SK"]))
		return evaluarCondicionE063(f.items[clave], aws.ToString(accion.Put.ConditionExpression),
			accion.Put.ExpressionAttributeNames, accion.Put.ExpressionAttributeValues)
	case accion.Update != nil:
		clave := claveItem(stringAttribute(accion.Update.Key["PK"]), stringAttribute(accion.Update.Key["SK"]))
		return evaluarCondicionE063(f.items[clave], aws.ToString(accion.Update.ConditionExpression),
			accion.Update.ExpressionAttributeNames, accion.Update.ExpressionAttributeValues)
	case accion.Delete != nil:
		clave := claveItem(stringAttribute(accion.Delete.Key["PK"]), stringAttribute(accion.Delete.Key["SK"]))
		return evaluarCondicionE063(f.items[clave], aws.ToString(accion.Delete.ConditionExpression),
			accion.Delete.ExpressionAttributeNames, accion.Delete.ExpressionAttributeValues)
	case accion.ConditionCheck != nil:
		clave := claveItem(stringAttribute(accion.ConditionCheck.Key["PK"]), stringAttribute(accion.ConditionCheck.Key["SK"]))
		return evaluarCondicionE063(f.items[clave], aws.ToString(accion.ConditionCheck.ConditionExpression),
			accion.ConditionCheck.ExpressionAttributeNames, accion.ConditionCheck.ExpressionAttributeValues)
	default:
		return false, fmt.Errorf("accion no soportada por el doble de E063")
	}
}

// evaluarCondicionE063 interpreta las condiciones que emite el adaptador.
// Cualquier forma desconocida es un error explícito: el doble nunca aprueba
// una condición que no entiende.
func evaluarCondicionE063(item map[string]types.AttributeValue, expresion string, nombres map[string]string, valores map[string]types.AttributeValue) (bool, error) {
	expresion = strings.TrimSpace(expresion)
	if expresion == "" {
		return true, nil
	}
	for _, parte := range strings.Split(expresion, " AND ") {
		cumple, err := evaluarAtomoE063(item, strings.TrimSpace(parte), nombres, valores)
		if err != nil {
			return false, err
		}
		if !cumple {
			return false, nil
		}
	}
	return true, nil
}

func evaluarAtomoE063(item map[string]types.AttributeValue, atomo string, nombres map[string]string, valores map[string]types.AttributeValue) (bool, error) {
	atomo = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(atomo), "("), ")"))
	if partes := strings.Split(atomo, " OR "); len(partes) > 1 {
		for _, parte := range partes {
			cumple, err := evaluarAtomoE063(item, strings.TrimSpace(parte), nombres, valores)
			if err != nil {
				return false, err
			}
			if cumple {
				return true, nil
			}
		}
		return false, nil
	}
	if strings.HasPrefix(atomo, "attribute_not_exists(") || strings.HasPrefix(atomo, "attribute_exists(") {
		niega := strings.HasPrefix(atomo, "attribute_not_exists(")
		nombre := strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(atomo, "attribute_not_exists("), "attribute_exists("), ")")
		_, existe := item[resolverNombreE063(nombre, nombres)]
		if niega {
			return !existe, nil
		}
		return existe, nil
	}
	for _, operador := range []string{" <= ", " >= ", " = ", " > ", " < "} {
		partes := strings.Split(atomo, operador)
		if len(partes) != 2 {
			continue
		}
		nombre := resolverNombreE063(strings.TrimSpace(partes[0]), nombres)
		valorEsperado, ok := valores[strings.TrimSpace(partes[1])]
		if !ok {
			return false, fmt.Errorf("valor de condicion ausente: %q", partes[1])
		}
		actual, existe := item[nombre]
		if !existe {
			return false, nil
		}
		return compararAtributosE063(actual, valorEsperado, operador), nil
	}
	return false, fmt.Errorf("condicion no soportada por el doble de E063: %q", atomo)
}

func resolverNombreE063(nombre string, nombres map[string]string) string {
	if strings.HasPrefix(nombre, "#") {
		return nombres[nombre]
	}
	return nombre
}

func compararAtributosE063(actual, esperado types.AttributeValue, operador string) bool {
	numeroActual, okActual := attributeInt(actual)
	numeroEsperado, okEsperado := attributeInt(esperado)
	if okActual && okEsperado {
		switch operador {
		case " = ":
			return numeroActual == numeroEsperado
		case " <= ":
			return numeroActual <= numeroEsperado
		case " >= ":
			return numeroActual >= numeroEsperado
		case " > ":
			return numeroActual > numeroEsperado
		case " < ":
			return numeroActual < numeroEsperado
		}
	}
	if operador != " = " {
		return false
	}
	switch esperado.(type) {
	case *types.AttributeValueMemberBOOL:
		valorActual, ok := actual.(*types.AttributeValueMemberBOOL)
		valorEsperado, okEsperado := esperado.(*types.AttributeValueMemberBOOL)
		return ok && okEsperado && valorActual.Value == valorEsperado.Value
	default:
		return stringAttribute(actual) == stringAttribute(esperado)
	}
}

// aplicarUpdateE063 soporta SET (con incremento if_not_exists) y REMOVE.
func aplicarUpdateE063(item map[string]types.AttributeValue, expresion string, nombres map[string]string, valores map[string]types.AttributeValue) (map[string]types.AttributeValue, error) {
	actualizado := copiarMapa(item)
	if actualizado == nil {
		actualizado = map[string]types.AttributeValue{}
	}
	expresion = strings.TrimSpace(expresion)
	parteSet, parteRemove := expresion, ""
	if i := strings.Index(expresion, " REMOVE "); i >= 0 {
		parteSet, parteRemove = expresion[:i], expresion[i+len(" REMOVE "):]
	}
	parteSet = strings.TrimPrefix(parteSet, "SET ")
	// Las cláusulas se separan por comas de PRIMER nivel: `if_not_exists(#x, :cero)`
	// contiene una coma interna y dividir a ciegas la partiría.
	for _, clausula := range dividirClausulasE063(parteSet) {
		pares := strings.Split(strings.TrimSpace(clausula), " = ")
		if len(pares) != 2 {
			continue
		}
		nombre := resolverNombreE063(strings.TrimSpace(pares[0]), nombres)
		if nombre == "" {
			return nil, fmt.Errorf("SET sin nombre resoluble: %q", clausula)
		}
		derecha := strings.TrimSpace(pares[1])
		if indice := strings.Index(derecha, " + "); indice >= 0 {
			incremento, ok := valores[strings.TrimSpace(derecha[indice+len(" + "):])]
			if !ok {
				return nil, fmt.Errorf("incremento sin valor: %q", derecha)
			}
			base, okBase := attributeInt(actualizado[nombre])
			if !okBase {
				base = 0
			}
			suma, okSuma := attributeInt(incremento)
			if !okSuma {
				return nil, fmt.Errorf("incremento no numerico: %q", derecha)
			}
			actualizado[nombre] = numberAttr(base + suma)
			continue
		}
		valor, ok := valores[derecha]
		if !ok {
			// `if_not_exists(#x, :valor)` sin suma conserva el valor actual.
			if strings.HasPrefix(derecha, "if_not_exists(") {
				continue
			}
			return nil, fmt.Errorf("SET sin valor: %q", derecha)
		}
		actualizado[nombre] = valor
	}
	for _, nombre := range strings.Split(parteRemove, ",") {
		nombre = resolverNombreE063(strings.TrimSpace(nombre), nombres)
		if nombre != "" {
			delete(actualizado, nombre)
		}
	}
	return actualizado, nil
}

// dividirClausulasE063 separa por comas de primer nivel, respetando paréntesis.
func dividirClausulasE063(texto string) []string {
	var salida []string
	profundidad := 0
	inicio := 0
	for i, r := range texto {
		switch r {
		case '(':
			profundidad++
		case ')':
			if profundidad > 0 {
				profundidad--
			}
		case ',':
			if profundidad == 0 {
				salida = append(salida, texto[inicio:i])
				inicio = i + 1
			}
		}
	}
	salida = append(salida, texto[inicio:])
	return salida
}

func copiarMapa(item map[string]types.AttributeValue) map[string]types.AttributeValue {
	if len(item) == 0 {
		return nil
	}
	copia := make(map[string]types.AttributeValue, len(item))
	for k, v := range item {
		copia[k] = v
	}
	return copia
}

func TestE061AdaptadorQueryExclusiveStartKeyYFiltros(t *testing.T) {
	cliente := nuevoE061Fake()
	cliente.sembrar("user-a", 6, 4, appNotifications.TypeBudget)
	cliente.sembrar("user-b", 3, 3, appNotifications.TypeBudget)
	store := NewNotificationsStore(cliente, "nexum-table")

	noLeidas := false
	primera, err := store.List(context.Background(), appNotifications.ListQuery{
		UserID: "user-a", Limit: 2, IsRead: &noLeidas, Type: appNotifications.TypeBudget,
	})
	if err != nil || len(primera.Items) != 2 || primera.NextKey == nil {
		t.Fatalf("primera página: %+v %v", primera, err)
	}
	q := cliente.queries[0]
	if q.KeyConditionExpression == nil || !strings.Contains(*q.KeyConditionExpression, "begins_with(#sk, :prefijo)") {
		t.Fatalf("KeyConditionExpression incorrecto: %v", q.KeyConditionExpression)
	}
	if q.FilterExpression == nil || !strings.Contains(*q.FilterExpression, "#isRead") || !strings.Contains(*q.FilterExpression, "#type") {
		t.Fatalf("FilterExpression no aplica filtros: %v", q.FilterExpression)
	}
	if stringAttribute(q.ExpressionAttributeValues[":pk"]) != notificationUserPK("user-a") {
		t.Fatal("la Query no quedó acotada al usuario autenticado")
	}

	segunda, err := store.List(context.Background(), appNotifications.ListQuery{
		UserID: "user-a", Limit: 2, IsRead: &noLeidas, Type: appNotifications.TypeBudget,
		After: primera.NextKey,
	})
	if err != nil || len(segunda.Items) != 2 {
		t.Fatalf("segunda página: %+v %v", segunda, err)
	}
	ultima := cliente.queries[len(cliente.queries)-1]
	if len(ultima.ExclusiveStartKey) == 0 || stringAttribute(ultima.ExclusiveStartKey["SK"]) != primera.NextKey.SK {
		t.Fatalf("ExclusiveStartKey no propagado: %+v", ultima.ExclusiveStartKey)
	}
	ids := map[string]bool{}
	for _, n := range append(primera.Items, segunda.Items...) {
		if ids[n.ID] {
			t.Fatalf("duplicado entre páginas: %s", n.ID)
		}
		ids[n.ID] = true
		if n.Type != appNotifications.TypeBudget || n.IsRead {
			t.Fatalf("filtro real no aplicado: %+v", n)
		}
	}
}

// TestE061AdaptadorEntityTypeCanonicoYLegacy fija la regresión que ocultaba
// todos los avisos escritos por el worker: el outbox usaba USER_NOTIFICATION y
// E061 filtraba solo por NOTIFICATION. Ambos valores deben listarse, contarse
// y poder marcarse como leídos.
func TestE061AdaptadorEntityTypeCanonicoYLegacy(t *testing.T) {
	cliente := nuevoE061Fake()
	cliente.sembrar("user-a", 1, 1, appNotifications.TypeSystem)
	base := time.Date(2026, 11, 26, 6, 0, 0, 0, time.UTC)
	idLegacy := "018f90ef-08e5-7a68-b09f-000000000099"
	pk := notificationUserPK("user-a")
	sk := notificationSortKey(base, idLegacy)
	cliente.items[claveItem(pk, sk)] = map[string]types.AttributeValue{
		"PK":         stringAttr(pk),
		"SK":         stringAttr(sk),
		"entityType": stringAttr(legacyNotificationEntityType),
		"id":         stringAttr(idLegacy),
		"title":      stringAttr("Aviso legacy"),
		"message":    stringAttr("Mensaje"),
		"type":       stringAttr(string(appNotifications.TypeSystem)),
		"isRead":     boolAttr(false),
		"createdAt":  stringAttr(base.Format(time.RFC3339Nano)),
	}
	store := NewNotificationsStore(cliente, "nexum-table")

	lista, err := store.List(context.Background(), appNotifications.ListQuery{UserID: "user-a", Limit: 10})
	if err != nil || len(lista.Items) != 2 {
		t.Fatalf("la bandeja debe incluir el entityType canónico y el legacy: %+v %v", lista, err)
	}
	total, err := store.CountUnread(context.Background(), "user-a")
	if err != nil || total != 2 {
		t.Fatalf("CountUnread debe contar ambos entityType: %d %v", total, err)
	}
	leida, err := store.MarkRead(context.Background(), "user-a", idLegacy, base.Add(time.Minute))
	if err != nil || leida == nil || !leida.IsRead {
		t.Fatalf("MarkRead sobre un aviso legacy: %+v %v", leida, err)
	}
}

func TestE061AdaptadorSelectCountPagaTodaLaBandeja(t *testing.T) {
	cliente := nuevoE061Fake()
	cliente.sembrar("user-a", 30, 17, appNotifications.TypeSystem)
	cliente.maxEvaluados = 9
	store := NewNotificationsStore(cliente, "nexum-table")

	total, err := store.CountUnread(context.Background(), "user-a")
	if err != nil || total != 17 {
		t.Fatalf("CountUnread: %d %v", total, err)
	}
	if len(cliente.queries) < 4 {
		t.Fatalf("el conteo no paginó el Query: %d llamadas", len(cliente.queries))
	}
	for i, q := range cliente.queries {
		if q.Select != types.SelectCount {
			t.Fatalf("consulta %d sin Select COUNT: %v", i, q.Select)
		}
		if q.FilterExpression == nil || !strings.Contains(*q.FilterExpression, "#isRead") {
			t.Fatalf("consulta %d sin filtro de no leídas", i)
		}
		if i > 0 && len(q.ExclusiveStartKey) == 0 {
			t.Fatalf("consulta %d no reanudó desde LastEvaluatedKey", i)
		}
	}
}

func TestE062AdaptadorUpdateCondicionalEIdempotente(t *testing.T) {
	cliente := nuevoE061Fake()
	cliente.sembrar("user-a", 2, 2, appNotifications.TypeStreak)
	store := NewNotificationsStore(cliente, "nexum-table")
	id := "018f90ef-08e5-7a68-b09f-000000000001"

	leida, err := store.MarkRead(context.Background(), "user-a", id, time.Now().UTC())
	if err != nil || leida == nil || !leida.IsRead {
		t.Fatalf("marcar leída: %+v %v", leida, err)
	}
	update := cliente.updates[len(cliente.updates)-1]
	if update.ConditionExpression == nil || !strings.Contains(*update.ConditionExpression, "#isRead = :falso") {
		t.Fatalf("UpdateItem no es condicional: %v", update.ConditionExpression)
	}
	if update.ReturnValues != types.ReturnValueAllNew {
		t.Fatalf("ReturnValues debió ser ALL_NEW: %v", update.ReturnValues)
	}
	if stringAttribute(update.ExpressionAttributeValues[":falso"]) != "" {
		t.Fatal(":falso debe ser booleano, no cadena")
	}
	if boolValue(update.ExpressionAttributeValues[":falso"]) {
		t.Fatal("la condición debe exigir isRead = false")
	}

	// Segunda llamada: la condición falla y la relectura devuelve el item ya leído.
	cliente.fuerzaCondicional = true
	repetida, err := store.MarkRead(context.Background(), "user-a", id, time.Now().UTC())
	if err != nil || repetida == nil || !repetida.IsRead {
		t.Fatalf("repetición idempotente: %+v %v", repetida, err)
	}
	if _, err := store.MarkRead(context.Background(), "user-b", id, time.Now().UTC()); !errors.Is(err, appNotifications.ErrNotFound) {
		t.Fatalf("recurso ajeno: %v", err)
	}
	cliente.fuerzaCondicional = false
	cliente.updateErr = errors.New("dynamo caído")
	if _, err := store.MarkRead(context.Background(), "user-a", id, time.Now().UTC()); !errors.Is(err, appNotifications.ErrUnavailable) {
		t.Fatalf("fallo de UpdateItem: %v", err)
	}
}

func boolValue(v types.AttributeValue) bool {
	b, ok := v.(*types.AttributeValueMemberBOOL)
	return ok && b.Value
}

func TestE063AdaptadorAdmisionColaClaimAvanceYCierre(t *testing.T) {
	cliente := nuevoE061Fake()
	cliente.sembrar("user-a", 31, 12, appNotifications.TypeSystem)
	store := NewNotificationsStore(cliente, "nexum-table")
	creado := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)

	estado, err := store.BeginReadAll(context.Background(), "user-a", "req-1", creado)
	if err != nil || estado == nil || estado.Status != appNotifications.StatusPending || estado.InitialUnread != 12 {
		t.Fatalf("BeginReadAll: %+v %v", estado, err)
	}
	if len(cliente.transacts) != 1 || len(cliente.transacts[0].TransactItems) != 3 {
		t.Fatalf("la admisión no fue una transacción de tres ítems: %+v", cliente.transacts)
	}
	acciones := cliente.transacts[0].TransactItems
	if acciones[0].Put == nil || stringAttribute(acciones[0].Put.Item["SK"]) != "READALL#req-1" {
		t.Fatalf("registro READALL ausente de la transacción: %+v", acciones[0])
	}
	if acciones[1].Put == nil || stringAttribute(acciones[1].Put.Item["PK"]) != ReadAllQueuePK ||
		!strings.HasPrefix(stringAttribute(acciones[1].Put.Item["SK"]), ReadAllQueueSKPrefix) {
		t.Fatalf("ítem de cola ausente de la transacción: %+v", acciones[1])
	}
	if acciones[2].Put == nil || stringAttribute(acciones[2].Put.Item["SK"]) != "READALLPTR#req-1" {
		t.Fatalf("puntero del titular ausente: %+v", acciones[2])
	}
	for _, accion := range acciones {
		if accion.Put.ConditionExpression == nil || !strings.Contains(*accion.Put.ConditionExpression, "attribute_not_exists(PK)") {
			t.Fatalf("la admisión no es idempotente: %v", accion.Put.ConditionExpression)
		}
	}

	// Repetir la admisión no duplica el trabajo y devuelve el registro vigente.
	antes := len(cliente.transacts)
	if repetido, err := store.BeginReadAll(context.Background(), "user-a", "req-1", creado.Add(time.Minute)); err != nil ||
		repetido == nil || repetido.CreatedAt != estado.CreatedAt {
		t.Fatalf("admisión repetida: %+v %v", repetido, err)
	}
	if len(cliente.transacts) == antes {
		t.Fatal("la admisión repetida debió intentar la transacción y cancelarse por condición")
	}
	reintento := cliente.transacts[antes]
	if reintento.TransactItems[0].Put == nil || stringAttribute(reintento.TransactItems[0].Put.Item["SK"]) != "READALL#req-1" {
		t.Fatalf("la admisión repetida no fue condicional sobre el registro: %+v", reintento.TransactItems)
	}
	cola, err := store.DueReadAll(context.Background(), creado.Add(time.Minute), 10)
	if err != nil || len(cola) != 1 || cola[0].Fence != 0 {
		t.Fatalf("la cola se duplicó o cambió: %+v %v", cola, err)
	}

	trabajos, err := store.DueReadAll(context.Background(), creado, 10)
	if err != nil || len(trabajos) != 1 {
		t.Fatalf("DueReadAll: %+v %v", trabajos, err)
	}
	if trabajos[0].RequestID != "req-1" || trabajos[0].Fence != 0 || trabajos[0].LeaseOwner != "" {
		t.Fatalf("trabajo due incorrecto: %+v", trabajos[0])
	}

	reclamado, ok, err := store.ClaimReadAll(context.Background(), trabajos[0], "worker-1", creado.Add(time.Second), time.Minute)
	if err != nil || !ok || reclamado.Fence != 1 || reclamado.Attempts != 1 || reclamado.LeaseOwner != "worker-1" {
		t.Fatalf("ClaimReadAll: %+v %v", reclamado, err)
	}
	if _, ok, _ := store.ClaimReadAll(context.Background(), trabajos[0], "worker-2", creado.Add(2*time.Second), time.Minute); ok {
		t.Fatal("dos workers reclamaron el mismo trabajo")
	}
	if trabajosDue, _ := store.DueReadAll(context.Background(), creado.Add(2*time.Second), 10); len(trabajosDue) != 0 {
		t.Fatalf("un trabajo con lease vigente aparece como due: %+v", trabajosDue)
	}

	avance := appNotifications.ReadAllState{
		RequestID: "req-1", Status: appNotifications.StatusPending, CreatedAt: estado.CreatedAt,
		ProcessedCount: 12,
		Cursor:         &appNotifications.NotificationKey{PK: notificationUserPK("user-a"), SK: "NOTIFICATION#cierre"},
		LeaseOwner:     "worker-1", LeaseUntil: creado.Add(3 * time.Minute), Fence: reclamado.Fence,
	}
	errAvance := store.AdvanceReadAll(context.Background(), "user-a", "worker-1", avance)
	if errAvance != nil {
		t.Fatalf("AdvanceReadAll: %v", errAvance)
	}
	ultima := cliente.transacts[len(cliente.transacts)-1]
	if len(ultima.TransactItems) != 2 || ultima.TransactItems[0].Update == nil || ultima.TransactItems[1].Update == nil {
		t.Fatalf("el avance no renovó cola y registro en una transacción: %+v", ultima.TransactItems)
	}
	final, err := store.LoadReadAll(context.Background(), "user-a", "req-1")
	if err != nil || final.Cursor == nil || final.Cursor.SK != "NOTIFICATION#cierre" || final.Fence != 1 {
		t.Fatalf("cursor/fence no persistidos: %+v %v", final, err)
	}

	if err := store.CompleteReadAll(context.Background(), "user-a", "req-1", "worker-1", reclamado.Fence, 12, creado.Add(4*time.Minute)); err != nil {
		t.Fatalf("CompleteReadAll: %v", err)
	}
	cierre := cliente.transacts[len(cliente.transacts)-1]
	if len(cierre.TransactItems) != 3 || cierre.TransactItems[1].Delete == nil || cierre.TransactItems[2].Delete == nil {
		t.Fatalf("el cierre no borra cola y puntero: %+v", cierre.TransactItems)
	}
	if stringAttribute(cierre.TransactItems[1].Delete.Key["PK"]) != ReadAllQueuePK {
		t.Fatal("el cierre no borra el ítem de cola correcto")
	}
	completado, err := store.LoadReadAll(context.Background(), "user-a", "req-1")
	if err != nil || completado.Status != appNotifications.StatusCompleted || completado.CompletedAt == nil {
		t.Fatalf("estado final: %+v %v", completado, err)
	}
	if _, existe := cliente.items[claveItem(ReadAllQueuePK, readAllQueueSortKey(estado.CreatedAt, "req-1"))]; existe {
		t.Fatal("el ítem de cola sobrevivió al cierre")
	}
	if _, existe := cliente.items[claveItem(notificationUserPK("user-a"), "READALLPTR#req-1")]; existe {
		t.Fatal("el puntero sobrevivió al cierre")
	}

	// Un dueño distinto no puede avanzar ni cerrar el mismo request.
	if err := store.AdvanceReadAll(context.Background(), "user-a", "otro-worker", avance); !errors.Is(err, appNotifications.ErrConflict) {
		t.Fatalf("carrera de leases: %v", err)
	}
	if err := store.CompleteReadAll(context.Background(), "user-a", "req-1", "otro-worker", 9, 12, time.Now().UTC()); err != nil {
		t.Fatalf("completar un registro ya cerrado debe ser idempotente: %v", err)
	}
}

func TestE063AdaptadorReleaseYPunteroDeCuentaEliminada(t *testing.T) {
	cliente := nuevoE061Fake()
	cliente.sembrar("user-a", 3, 3, appNotifications.TypeSystem)
	store := NewNotificationsStore(cliente, "nexum-table")
	creado := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	if _, err := store.BeginReadAll(context.Background(), "user-a", "req-2", creado); err != nil {
		t.Fatal(err)
	}
	trabajos, _ := store.DueReadAll(context.Background(), creado, 10)
	if len(trabajos) != 1 {
		t.Fatal("sin trabajo para liberar")
	}
	reclamado, ok, err := store.ClaimReadAll(context.Background(), trabajos[0], "worker-1", creado.Add(time.Second), time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim: %v", err)
	}
	retryAt := creado.Add(30 * time.Second)
	if err := store.ReleaseReadAll(context.Background(), reclamado, creado.Add(2*time.Second), retryAt); err != nil {
		t.Fatalf("release: %v", err)
	}
	if due, _ := store.DueReadAll(context.Background(), creado.Add(3*time.Second), 10); len(due) != 0 {
		t.Fatal("el trabajo liberado no debe estar due antes del reintento")
	}
	due, err := store.DueReadAll(context.Background(), retryAt.Add(time.Second), 10)
	if err != nil || len(due) != 1 {
		t.Fatalf("el trabajo no volvió tras el backoff: %+v %v", due, err)
	}
	if err := store.ReleaseReadAll(context.Background(), reclamado, creado.Add(40*time.Second), retryAt.Add(time.Minute)); !errors.Is(err, appNotifications.ErrConflict) {
		t.Fatalf("un dueño obsoleto liberó el trabajo: %v", err)
	}

	// La partición del titular se borra (E014): el trabajo pendiente se
	// descarta sin recrear el registro ni resucitar notificaciones.
	delete(cliente.items, claveItem(notificationUserPK("user-a"), readAllSortKey("req-2")))
	if _, err := store.LoadReadAll(context.Background(), "user-a", "req-2"); !errors.Is(err, appNotifications.ErrNotFound) {
		t.Fatalf("registro ausente: %v", err)
	}
	if err := store.FinishReadAll(context.Background(), reclamado, creado.Add(time.Minute)); err != nil {
		t.Fatalf("FinishReadAll: %v", err)
	}
	if _, existe := cliente.items[claveItem(ReadAllQueuePK, readAllQueueSortKey(creado, "req-2"))]; existe {
		t.Fatal("el trabajo obsoleto quedó en la cola")
	}
}

func TestE061AdaptadorFallosCerrados(t *testing.T) {
	sinCliente := NewNotificationsStore(nil, "nexum-table")
	if _, err := sinCliente.List(context.Background(), appNotifications.ListQuery{UserID: "user-a", Limit: 25}); !errors.Is(err, appNotifications.ErrUnavailable) {
		t.Fatalf("List sin cliente: %v", err)
	}
	if _, err := sinCliente.CountUnread(context.Background(), "user-a"); !errors.Is(err, appNotifications.ErrUnavailable) {
		t.Fatalf("Count sin cliente: %v", err)
	}
	if _, err := sinCliente.MarkRead(context.Background(), "user-a", "id", time.Now()); !errors.Is(err, appNotifications.ErrUnavailable) {
		t.Fatalf("MarkRead sin cliente: %v", err)
	}
	if _, err := sinCliente.BeginReadAll(context.Background(), "user-a", "req", time.Now()); !errors.Is(err, appNotifications.ErrUnavailable) {
		t.Fatalf("Begin sin cliente: %v", err)
	}

	cliente := nuevoE061Fake()
	cliente.sembrar("user-a", 2, 2, appNotifications.TypeSystem)
	cliente.queryErr = errors.New("dynamo down")
	if _, err := NewNotificationsStore(cliente, "nexum-table").List(context.Background(), appNotifications.ListQuery{UserID: "user-a", Limit: 10}); !errors.Is(err, appNotifications.ErrUnavailable) {
		t.Fatalf("Query caído: %v", err)
	}
	if err := NewNotificationsStore(cliente, "nexum-table").AdvanceReadAll(context.Background(), "", "x", appNotifications.ReadAllState{RequestID: "req"}); !errors.Is(err, appNotifications.ErrInvalidRequest) {
		t.Fatalf("usuario vacío: %v", err)
	}
	if err := NewNotificationsStore(nil, "nexum-table").CompleteReadAll(context.Background(), "user-a", "req", "", 0, 0, time.Now()); !errors.Is(err, appNotifications.ErrUnavailable) {
		t.Fatalf("Complete sin cliente: %v", err)
	}
}

// TestE061LimiteConsultaAcotado fija el rango contractual 1..100 antes de int32.
func TestE061LimiteConsultaAcotado(t *testing.T) {
	casos := []struct {
		entrada  int
		esperado int32
	}{
		{-1000, 1},
		{0, 1},
		{1, 1},
		{100, 100},
		{101, int32(appNotifications.MaxLimit)},
		{1 << 30, int32(appNotifications.MaxLimit)},
	}
	for _, caso := range casos {
		if got := limiteConsultaNotificaciones(caso.entrada); got != caso.esperado {
			t.Fatalf("limiteConsultaNotificaciones(%d)=%d, esperado %d", caso.entrada, got, caso.esperado)
		}
	}

	fake := nuevoE061Fake()
	store := NewNotificationsStore(fake, "NEXUM_CORE_TEST")
	if _, err := store.queryNotifications(context.Background(), notificationUserPK("u1"), nil, "", nil, 1<<30, false); err != nil {
		t.Fatalf("queryNotifications: %v", err)
	}
	if len(fake.queries) != 1 {
		t.Fatalf("queries capturadas: %d", len(fake.queries))
	}
	if limite := aws.ToInt32(fake.queries[0].Limit); limite != int32(appNotifications.MaxLimit) {
		t.Fatalf("Limit enviado a DynamoDB fuera de rango: %d", limite)
	}
}
