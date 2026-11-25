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
// ExclusiveStartKey, Select COUNT, UpdateItem con condiciones y PutItem
// condicional. No hay Scan en la interfaz del adaptador.
type e061FakeClient struct {
	mu                sync.Mutex
	items             map[string]map[string]types.AttributeValue
	queries           []*dynamodb.QueryInput
	updates           []*dynamodb.UpdateItemInput
	puts              []*dynamodb.PutItemInput
	queryErr          error
	getErr            error
	putErr            error
	updateErr         error
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
	if params.ConditionExpression != nil && strings.Contains(*params.ConditionExpression, "attribute_not_exists(PK)") {
		if _, existe := f.items[clave]; existe {
			return nil, &types.ConditionalCheckFailedException{}
		}
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
	expresion := aws.ToString(params.ConditionExpression)
	switch {
	case f.fuerzaCondicional:
		return nil, &types.ConditionalCheckFailedException{}
	case strings.Contains(expresion, "#isRead = :falso"):
		actual, ok := item["isRead"].(*types.AttributeValueMemberBOOL)
		if !ok || actual.Value {
			return nil, &types.ConditionalCheckFailedException{}
		}
	case strings.Contains(expresion, "#status = :pending"):
		if stringAttribute(item["status"]) != stringAttribute(params.ExpressionAttributeValues[":pending"]) {
			return nil, &types.ConditionalCheckFailedException{}
		}
		if !leaseValido(item, params) {
			return nil, &types.ConditionalCheckFailedException{}
		}
	case strings.Contains(expresion, "attribute_not_exists(#leaseOwner)"):
		if !leaseValido(item, params) {
			return nil, &types.ConditionalCheckFailedException{}
		}
	}

	actualizado := make(map[string]types.AttributeValue, len(item))
	for k, v := range item {
		actualizado[k] = v
	}
	exp := aws.ToString(params.UpdateExpression)
	parteSet, parteRemove := exp, ""
	if i := strings.Index(exp, " REMOVE "); i >= 0 {
		parteSet, parteRemove = exp[:i], exp[i+len(" REMOVE "):]
	}
	parteSet = strings.TrimPrefix(parteSet, "SET ")
	for _, clausula := range strings.Split(parteSet, ",") {
		pares := strings.Split(strings.TrimSpace(clausula), " = ")
		if len(pares) != 2 {
			continue
		}
		nombre := params.ExpressionAttributeNames[pares[0]]
		valor := params.ExpressionAttributeValues[pares[1]]
		if nombre != "" && valor != nil {
			actualizado[nombre] = valor
		}
	}
	for _, nombre := range strings.Split(parteRemove, ",") {
		nombre = params.ExpressionAttributeNames[strings.TrimSpace(nombre)]
		if nombre != "" {
			delete(actualizado, nombre)
		}
	}
	f.items[clave] = actualizado
	return &dynamodb.UpdateItemOutput{Attributes: actualizado}, nil
}

func leaseValido(item map[string]types.AttributeValue, params *dynamodb.UpdateItemInput) bool {
	dueñoActual := stringAttribute(item["leaseOwner"])
	if dueñoActual == "" {
		return true
	}
	return dueñoActual == stringAttribute(params.ExpressionAttributeValues[":expected"])
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

func TestE063AdaptadorRegistroReadAllPersistidoYCerrado(t *testing.T) {
	cliente := nuevoE061Fake()
	cliente.sembrar("user-a", 31, 12, appNotifications.TypeSystem)
	store := NewNotificationsStore(cliente, "nexum-table")

	estado, err := store.BeginReadAll(context.Background(), "user-a", "req-1", time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC))
	if err != nil || estado == nil || estado.Status != appNotifications.StatusPending || estado.InitialUnread != 12 {
		t.Fatalf("BeginReadAll: %+v %v", estado, err)
	}
	put := cliente.puts[len(cliente.puts)-1]
	if put.ConditionExpression == nil || !strings.Contains(*put.ConditionExpression, "attribute_not_exists(PK)") {
		t.Fatalf("el registro no se creó de forma condicional: %v", put.ConditionExpression)
	}
	if stringAttribute(put.Item["entityType"]) != ReadAllEntityType || stringAttribute(put.Item["SK"]) != "READALL#req-1" {
		t.Fatalf("clave/entidad READALL incorrecta: %v", put.Item)
	}
	if stringAttribute(put.Item["createdAt"]) == "" || attributeIntMust(put.Item["processedCount"]) != 0 {
		t.Fatal("createdAt/processedCount ausentes en el registro persistido")
	}

	avance := appNotifications.ReadAllState{
		RequestID: "req-1", Status: appNotifications.StatusPending,
		ProcessedCount: 25, Cursor: &appNotifications.NotificationKey{PK: notificationUserPK("user-a"), SK: "NOTIFICATION#cierre"},
		LeaseOwner: "worker-1", LeaseUntil: time.Now().UTC().Add(time.Minute),
	}
	if err := store.AdvanceReadAll(context.Background(), "user-a", "", avance); err != nil {
		t.Fatalf("AdvanceReadAll: %v", err)
	}
	avanceUpdate := cliente.updates[len(cliente.updates)-1]
	if avanceUpdate.ConditionExpression == nil || !strings.Contains(*avanceUpdate.ConditionExpression, "#leaseOwner") {
		t.Fatalf("avance sin lease: %v", avanceUpdate.ConditionExpression)
	}
	if stringAttribute(avanceUpdate.ExpressionAttributeValues[":cursor"]) != "NOTIFICATION#cierre" {
		t.Fatal("el cursor no se persistió en el registro READALL")
	}

	if err := store.CompleteReadAll(context.Background(), "user-a", "req-1", "worker-1", 12, time.Now().UTC()); err != nil {
		t.Fatalf("CompleteReadAll: %v", err)
	}
	cierre := cliente.updates[len(cliente.updates)-1]
	if cierre.ConditionExpression == nil || !strings.Contains(*cierre.ConditionExpression, "#status = :pending") {
		t.Fatalf("cierre no condicionado al estado pendiente: %v", cierre.ConditionExpression)
	}
	if !strings.Contains(aws.ToString(cierre.UpdateExpression), "REMOVE #cursor") {
		t.Fatalf("el cierre no libera el lease/cursor: %s", aws.ToString(cierre.UpdateExpression))
	}
	final, err := store.LoadReadAll(context.Background(), "user-a", "req-1")
	if err != nil || final.Status != appNotifications.StatusCompleted || final.CompletedAt == nil {
		t.Fatalf("estado final: %+v %v", final, err)
	}

	// Un dueño distinto no puede avanzar ni cerrar el mismo request.
	if err := store.AdvanceReadAll(context.Background(), "user-a", "otro-worker", avance); !errors.Is(err, appNotifications.ErrConflict) {
		t.Fatalf("carrera de leases: %v", err)
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
	if err := NewNotificationsStore(nil, "nexum-table").CompleteReadAll(context.Background(), "user-a", "req", "", 0, time.Now()); !errors.Is(err, appNotifications.ErrUnavailable) {
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
