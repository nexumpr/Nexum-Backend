package dynamo_test

import (
	"context"
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
	appGoals "github.com/nexumpr/Nexum-Backend/internal/application/goals"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
	domainGoal "github.com/nexumpr/Nexum-Backend/internal/domain/goal"
)

// Pruebas de E039 sobre el adaptador REAL `GoalsStore` montado sobre un doble con estado que
// interpreta la misma transacción que emite. Demuestran que la meta y el registro de
// idempotencia se confirman juntos, que la segunda petición es un replay sin duplicar y que un
// payload distinto es un 409 sin escribir nada. El doble no es DynamoDB: sólo evalúa la
// gramática de condiciones que este adaptador emite y falla si aparece otra forma.

const (
	e039Tabla     = "NEXUM_CORE_TEST"
	e039Usuario   = "11111111-2222-4333-8444-555555555555"
	e039MetaID    = "018f90ef-08e5-7a68-b09f-0000000000a1"
	e039Clave     = "018f90ef-08e5-7a68-b09f-2b67a47c92f0"
	e039Cuerpo    = `{"id":"018f90ef-08e5-7a68-b09f-0000000000a1"}`
	e039Ubicacion = "/api/v1/goals/" + e039MetaID
)

var e039Creado = time.Date(2026, 11, 18, 15, 0, 0, 0, time.UTC)

type memoriaMetas struct {
	mu                    sync.Mutex
	items                 map[string]map[string]types.AttributeValue
	transacciones         int
	ultimos               []types.TransactWriteItem
	fallo                 error
	forzarCancelacionIdem bool
	consultas             int
	ultimaQuery           *dynamodb.QueryInput
	lecturaConsistente    bool
}

func newMemoriaMetas() *memoriaMetas {
	return &memoriaMetas{items: map[string]map[string]types.AttributeValue{}}
}

func (m *memoriaMetas) GetItem(_ context.Context, params *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lecturaConsistente = aws.ToBool(params.ConsistentRead)
	if m.fallo != nil {
		return nil, m.fallo
	}
	pk := mapaString(params.Key, "PK")
	sk := mapaString(params.Key, "SK")
	item, ok := m.items[claveDe(pk, sk)]
	if !ok {
		return &dynamodb.GetItemOutput{}, nil
	}
	return &dynamodb.GetItemOutput{Item: copiarItem(item)}, nil
}

// Query implementa la gramática que emite el adaptador: `PK = :pk AND begins_with(SK, :prefix)`,
// `Limit` sobre los ítems LEÍDOS (como DynamoDB), filtro opcional por igualdad y paginación por
// `ExclusiveStartKey`. Si una página queda vacía por el filtro pero quedan datos, devuelve
// `LastEvaluatedKey`, que es justo el caso que el adaptador debe saber recorrer.
func (m *memoriaMetas) Query(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.consultas++
	m.ultimaQuery = params
	m.lecturaConsistente = aws.ToBool(params.ConsistentRead)
	if m.fallo != nil {
		return nil, m.fallo
	}
	condicion := aws.ToString(params.KeyConditionExpression)
	if condicion != "PK = :pk AND begins_with(SK, :prefix)" {
		return nil, errors.New("condicion de clave no soportada por el doble: " + condicion)
	}
	pk := textoDe(params.ExpressionAttributeValues[":pk"])
	prefijo := textoDe(params.ExpressionAttributeValues[":prefix"])
	if pk == "" || prefijo == "" {
		return nil, errors.New("la consulta debe declarar :pk y :prefix")
	}

	type entrada struct {
		sk   string
		item map[string]types.AttributeValue
	}
	candidatos := make([]entrada, 0, len(m.items))
	for _, item := range m.items {
		if mapaString(item, "PK") != pk {
			continue
		}
		sk := mapaString(item, "SK")
		if !strings.HasPrefix(sk, prefijo) {
			continue
		}
		candidatos = append(candidatos, entrada{sk: sk, item: copiarItem(item)})
	}
	sort.Slice(candidatos, func(i, j int) bool { return candidatos[i].sk < candidatos[j].sk })

	inicio := 0
	if len(params.ExclusiveStartKey) > 0 {
		startSK := mapaString(params.ExclusiveStartKey, "SK")
		for inicio < len(candidatos) && candidatos[inicio].sk <= startSK {
			inicio++
		}
	}

	limite := len(candidatos) - inicio
	if params.Limit != nil {
		if pedido := int(aws.ToInt32(params.Limit)); pedido < limite {
			limite = pedido
		}
	}
	fin := inicio + limite

	var items []map[string]types.AttributeValue
	filtro := aws.ToString(params.FilterExpression)
	for _, candidato := range candidatos[inicio:fin] {
		if filtro != "" {
			cumple, err := evaluarCondicion(filtro, candidato.item, params.ExpressionAttributeNames, params.ExpressionAttributeValues)
			if err != nil {
				return nil, err
			}
			if !cumple {
				continue
			}
		}
		items = append(items, candidato.item)
	}

	salida := &dynamodb.QueryOutput{
		Items:        items,
		Count:        int32(len(items)),
		ScannedCount: int32(fin - inicio),
	}
	if fin > inicio && fin < len(candidatos) {
		salida.LastEvaluatedKey = map[string]types.AttributeValue{
			"PK": strAttr(pk),
			"SK": strAttr(candidatos[fin-1].sk),
		}
	}
	return salida, nil
}

// textoDe extrae el valor de un atributo de texto.
func textoDe(valor types.AttributeValue) string {
	if texto, ok := valor.(*types.AttributeValueMemberS); ok {
		return texto.Value
	}
	return ""
}

func (m *memoriaMetas) TransactWriteItems(_ context.Context, params *dynamodb.TransactWriteItemsInput, _ ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.transacciones++
	m.ultimos = params.TransactItems
	if m.fallo != nil {
		return nil, m.fallo
	}
	if m.forzarCancelacionIdem {
		return nil, m.cancelacionEn(len(params.TransactItems), 1)
	}
	// Fase 1: evaluar TODAS las condiciones antes de escribir nada (atomicidad real). DynamoDB
	// informa una razón por acción, de modo que un replay puede fallar en la meta y en el
	// registro a la vez: el doble replica esa forma.
	fallos := make([]int, 0, len(params.TransactItems))
	for i := range params.TransactItems {
		if err := m.comprobar(params.TransactItems[i]); err != nil {
			var cancelada *types.TransactionCanceledException
			if errors.As(err, &cancelada) {
				fallos = append(fallos, i)
				continue
			}
			return nil, err
		}
	}
	if len(fallos) > 0 {
		razones := make([]types.CancellationReason, len(params.TransactItems))
		for i := range razones {
			razones[i] = types.CancellationReason{Code: aws.String("None")}
		}
		for _, i := range fallos {
			razones[i] = types.CancellationReason{Code: aws.String("ConditionalCheckFailed")}
		}
		return nil, &types.TransactionCanceledException{
			Message:             aws.String("Transaction cancelled"),
			CancellationReasons: razones,
		}
	}
	// Fase 2: aplicar.
	for _, item := range params.TransactItems {
		if item.Put == nil {
			continue
		}
		pk := mapaString(item.Put.Item, "PK")
		sk := mapaString(item.Put.Item, "SK")
		m.items[claveDe(pk, sk)] = copiarItem(item.Put.Item)
	}
	return &dynamodb.TransactWriteItemsOutput{}, nil
}

func (m *memoriaMetas) cancelacionEn(total, indice int) error {
	razones := make([]types.CancellationReason, total)
	for i := range razones {
		razones[i] = types.CancellationReason{Code: aws.String("None")}
	}
	razones[indice] = types.CancellationReason{Code: aws.String("ConditionalCheckFailed")}
	return &types.TransactionCanceledException{
		Message:             aws.String("Transaction cancelled"),
		CancellationReasons: razones,
	}
}

func (m *memoriaMetas) comprobar(item types.TransactWriteItem) error {
	if item.Put == nil {
		return errors.New("accion no soportada por el doble de prueba")
	}
	pk := mapaString(item.Put.Item, "PK")
	sk := mapaString(item.Put.Item, "SK")
	existente := m.items[claveDe(pk, sk)]
	condicion := aws.ToString(item.Put.ConditionExpression)
	if condicion == "" {
		return nil
	}
	cumple, err := evaluarCondicion(condicion, existente, item.Put.ExpressionAttributeNames, item.Put.ExpressionAttributeValues)
	if err != nil {
		return err
	}
	if !cumple {
		return &types.TransactionCanceledException{Message: aws.String("condicion no cumplida")}
	}
	return nil
}

func e039MetaAgregada(t *testing.T) *domainGoal.Goal {
	t.Helper()
	meta, err := domainGoal.New(
		e039Usuario, e039MetaID, "Viaje a Oaxaca",
		100000, 25000,
		time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
		e039Creado,
	)
	if err != nil {
		t.Fatalf("no se pudo construir la meta de prueba: %v", err)
	}
	return &meta
}

func e039Escritura(t *testing.T) appGoals.CreateWrite {
	t.Helper()
	return appGoals.CreateWrite{
		Goal:           e039MetaAgregada(t),
		Operation:      appGoals.OperationCreateGoal,
		IdempotencyKey: e039Clave,
		PayloadHash:    "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90",
		ResponseBody:   []byte(e039Cuerpo),
		Location:       e039Ubicacion,
		TTL:            appGoals.IdempotencyTTL,
		Now:            e039Creado,
	}
}

// TestE039TransaccionDeDosPutConCents comprueba la forma exacta de la transacción: meta y
// registro en la misma operación, claves canónicas e importes en céntimos enteros.
func TestE039TransaccionDeDosPutConCents(t *testing.T) {
	memoria := newMemoriaMetas()
	store := dynamoAdapter.NewGoalsStore(memoria, e039Tabla)

	respuesta, err := store.CreateGoal(context.Background(), e039Escritura(t))
	if err != nil {
		t.Fatalf("la creacion no debe fallar: %v", err)
	}
	if respuesta.Status != 201 || respuesta.Replayed {
		t.Fatalf("respuesta inesperada: %+v", respuesta)
	}
	if respuesta.ResourceID != e039MetaID || respuesta.Location != e039Ubicacion {
		t.Fatalf("identificador o Location incorrectos: %+v", respuesta)
	}
	if memoria.transacciones != 1 {
		t.Fatalf("se esperaba una transaccion, hubo %d", memoria.transacciones)
	}
	if len(memoria.ultimos) != 2 {
		t.Fatalf("la transaccion debe tener dos acciones, tiene %d", len(memoria.ultimos))
	}

	meta := memoria.ultimos[0].Put
	if meta == nil {
		t.Fatal("la primera accion debe ser un Put de la meta")
	}
	if got := mapaString(meta.Item, "PK"); got != domainDynamo.PrefixUser+e039Usuario {
		t.Fatalf("PK de la meta inesperada: %q", got)
	}
	if got := mapaString(meta.Item, "SK"); got != domainDynamo.PrefixGoal+e039MetaID {
		t.Fatalf("SK de la meta inesperada: %q", got)
	}
	if condicion := aws.ToString(meta.ConditionExpression); condicion != "attribute_not_exists(PK)" {
		t.Fatalf("condicion de la meta inesperada: %q", condicion)
	}
	if cents := numeroDe(meta.Item, "targetAmountCents"); cents != 100000 {
		t.Fatalf("targetAmountCents inesperado: %d", cents)
	}
	if cents := numeroDe(meta.Item, "savedAmountCents"); cents != 25000 {
		t.Fatalf("savedAmountCents inesperado: %d", cents)
	}
	if cents := numeroDe(meta.Item, "openingSavedCents"); cents != 25000 {
		t.Fatalf("openingSavedCents inesperado: %d", cents)
	}
	if _, existe := meta.Item["targetAmount"]; existe {
		t.Fatal("no debe persistirse un importe decimal: solo centimos enteros")
	}
	if version := numeroDe(meta.Item, "version"); version != 1 {
		t.Fatalf("version inesperada: %d", version)
	}

	idemPK, idemSK := domainDynamo.UserIdempotencyKeys(e039Usuario, appGoals.OperationCreateGoal, e039Clave)
	registro := memoria.ultimos[1].Put
	if registro == nil {
		t.Fatal("la segunda accion debe ser un Put del registro de idempotencia")
	}
	if got := mapaString(registro.Item, "PK"); got != idemPK {
		t.Fatalf("PK del registro inesperada: %q (esperada %q)", got, idemPK)
	}
	if got := mapaString(registro.Item, "SK"); got != idemSK {
		t.Fatalf("SK del registro inesperada: %q (esperada %q)", got, idemSK)
	}
	if condicion := aws.ToString(registro.ConditionExpression); condicion == "" {
		t.Fatal("el registro debe tener condicion propia")
	}
	if estado := mapaString(registro.Item, "status"); estado != "COMPLETED" {
		t.Fatalf("estado del registro inesperado: %q", estado)
	}
	if hash := mapaString(registro.Item, "payloadHash"); hash != e039Escritura(t).PayloadHash {
		t.Fatalf("payloadHash persistido inesperado: %q", hash)
	}
	if body := mapaString(registro.Item, "responseBody"); body != e039Cuerpo {
		t.Fatalf("responseBody persistido inesperado: %q", body)
	}
	if recurso := mapaString(registro.Item, "resourceId"); recurso != e039MetaID {
		t.Fatalf("resourceId persistido inesperado: %q", recurso)
	}
	expira := numeroDe(registro.Item, "expiresAt")
	if expira != e039Creado.Add(appGoals.IdempotencyTTL).Unix() {
		t.Fatalf("expiresAt inesperado: %d (esperado %d)", expira, e039Creado.Add(appGoals.IdempotencyTTL).Unix())
	}
	if len(memoria.items) != 2 {
		t.Fatalf("deben existir exactamente dos items, hay %d", len(memoria.items))
	}
}

// TestE039SegundaLlamadaEsReplay comprueba que repetir la misma escritura devuelve el cuerpo
// almacenado y no duplica la meta.
func TestE039SegundaLlamadaEsReplay(t *testing.T) {
	memoria := newMemoriaMetas()
	store := dynamoAdapter.NewGoalsStore(memoria, e039Tabla)
	escritura := e039Escritura(t)

	if _, err := store.CreateGoal(context.Background(), escritura); err != nil {
		t.Fatalf("primera creacion fallo: %v", err)
	}
	segunda, err := store.CreateGoal(context.Background(), escritura)
	if err != nil {
		t.Fatalf("el replay no debe fallar: %v", err)
	}
	if !segunda.Replayed {
		t.Fatal("la segunda llamada debe marcarse como replay")
	}
	if string(segunda.Body) != e039Cuerpo || segunda.Status != 201 || segunda.ResourceID != e039MetaID {
		t.Fatalf("replay incorrecto: %+v", segunda)
	}
	if memoria.transacciones != 2 {
		t.Fatalf("se esperaban dos intentos de transaccion, hubo %d", memoria.transacciones)
	}
	if len(memoria.items) != 2 {
		t.Fatalf("no debe duplicarse la meta: %d items", len(memoria.items))
	}
}

// TestE039PayloadDistintoEsConflicto comprueba el 409 sin escritura adicional.
func TestE039PayloadDistintoEsConflicto(t *testing.T) {
	memoria := newMemoriaMetas()
	store := dynamoAdapter.NewGoalsStore(memoria, e039Tabla)
	escritura := e039Escritura(t)
	if _, err := store.CreateGoal(context.Background(), escritura); err != nil {
		t.Fatalf("primera creacion fallo: %v", err)
	}

	otra := escritura
	otra.PayloadHash = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	_, err := store.CreateGoal(context.Background(), otra)
	if !errors.Is(err, appGoals.ErrConflict) {
		t.Fatalf("esperado ErrConflict, obtenido %v", err)
	}
	if len(memoria.items) != 2 {
		t.Fatalf("el conflicto no debe escribir: %d items", len(memoria.items))
	}
}

// TestE039SinRegistroReintentaYFalla reproduce la carrera con el TTL: la transacción se
// cancela, el registro no aparece y sólo se permite un reintento antes de responder 409.
func TestE039SinRegistroReintentaYFalla(t *testing.T) {
	memoria := newMemoriaMetas()
	memoria.forzarCancelacionIdem = true
	store := dynamoAdapter.NewGoalsStore(memoria, e039Tabla)

	_, err := store.CreateGoal(context.Background(), e039Escritura(t))
	if !errors.Is(err, appGoals.ErrConflict) {
		t.Fatalf("esperado ErrConflict, obtenido %v", err)
	}
	if memoria.transacciones != 2 {
		t.Fatalf("se esperaba un unico reintento (2 transacciones), hubo %d", memoria.transacciones)
	}
	if len(memoria.items) != 0 {
		t.Fatalf("no debe quedar ningun item escrito: %d", len(memoria.items))
	}
}

// TestE039FallaCerradoSinDynamoYConErrorDeServicio cubre los caminos 503.
func TestE039FallaCerradoSinDynamoYConErrorDeServicio(t *testing.T) {
	t.Run("sin cliente ni tabla", func(t *testing.T) {
		store := dynamoAdapter.NewGoalsStore(nil, "")
		if _, err := store.CreateGoal(context.Background(), e039Escritura(t)); !errors.Is(err, appGoals.ErrUnavailable) {
			t.Fatalf("esperado ErrUnavailable, obtenido %v", err)
		}
		if _, err := store.LoadIdempotency(context.Background(), e039Usuario, appGoals.OperationCreateGoal, e039Clave); !errors.Is(err, appGoals.ErrUnavailable) {
			t.Fatalf("esperado ErrUnavailable, obtenido %v", err)
		}
	})
	t.Run("error del servicio", func(t *testing.T) {
		memoria := newMemoriaMetas()
		memoria.fallo = errors.New("throttling")
		store := dynamoAdapter.NewGoalsStore(memoria, e039Tabla)
		if _, err := store.CreateGoal(context.Background(), e039Escritura(t)); !errors.Is(err, appGoals.ErrUnavailable) {
			t.Fatalf("esperado ErrUnavailable, obtenido %v", err)
		}
	})
}

// numeroDe lee un atributo numérico del item como entero.
func numeroDe(item map[string]types.AttributeValue, nombre string) int64 {
	valor, ok := item[nombre].(*types.AttributeValueMemberN)
	if !ok {
		return -1
	}
	parsed, err := strconv.ParseInt(valor.Value, 10, 64)
	if err != nil {
		return -1
	}
	return parsed
}
