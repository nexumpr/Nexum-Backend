package dynamo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	appReminders "github.com/nexumpr/Nexum-Backend/internal/application/reminders"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// ---------------------------------------------------------------------------
// Doble estricto de DynamoDB para E064–E067
//
// Reglas deliberadas:
//   - GetItem sólo acepta la clave exacta (PK,SK); cualquier atributo extra es
//     un error de prueba, no un acceso "aproximado".
//   - Query sólo acepta la condición usada por el adaptador y pagina de verdad.
//   - TransactWriteItems valida condiciones, prohíbe dos acciones sobre el mismo
//     ítem y aplica todo o nada.
//   - No existe Scan en la interfaz ni en el doble.
// ---------------------------------------------------------------------------

type remFakeDynamo struct {
	mu                  sync.Mutex
	items               map[string]map[string]types.AttributeValue
	tamanoPagina        int
	gets                int
	queries             int
	transacciones       int
	transaccionesVistas []*dynamodb.TransactWriteItemsInput
	errGet              error
	errQuery            error
	errTransact         error
	clavesInvalidas     []string
}

func nuevoRemFakeDynamo() *remFakeDynamo {
	return &remFakeDynamo{items: map[string]map[string]types.AttributeValue{}}
}

func remClave(pk, sk string) string { return pk + "\x00" + sk }

func remCopiar(item map[string]types.AttributeValue) map[string]types.AttributeValue {
	if len(item) == 0 {
		return nil
	}
	copia := make(map[string]types.AttributeValue, len(item))
	for clave, valor := range item {
		copia[clave] = valor
	}
	return copia
}

func (f *remFakeDynamo) poner(item map[string]types.AttributeValue) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.items[remClave(stringAttribute(item["PK"]), stringAttribute(item["SK"]))] = remCopiar(item)
}

func (f *remFakeDynamo) item(pk, sk string) map[string]types.AttributeValue {
	f.mu.Lock()
	defer f.mu.Unlock()
	return remCopiar(f.items[remClave(pk, sk)])
}

func (f *remFakeDynamo) claveExacta(llave map[string]types.AttributeValue) (string, error) {
	if len(llave) != 2 {
		return "", fmt.Errorf("clave no exacta: %d atributos (%v)", len(llave), llave)
	}
	pk, okPK := llave["PK"].(*types.AttributeValueMemberS)
	sk, okSK := llave["SK"].(*types.AttributeValueMemberS)
	if !okPK || !okSK {
		return "", fmt.Errorf("clave no exacta: PK y SK deben ser strings")
	}
	return remClave(pk.Value, sk.Value), nil
}

func (f *remFakeDynamo) GetItem(_ context.Context, params *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gets++
	clave, err := f.claveExacta(params.Key)
	if err != nil {
		f.clavesInvalidas = append(f.clavesInvalidas, err.Error())
		return nil, err
	}
	if f.errGet != nil {
		return nil, f.errGet
	}
	return &dynamodb.GetItemOutput{Item: remCopiar(f.items[clave])}, nil
}

func (f *remFakeDynamo) PutItem(_ context.Context, params *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	clave := remClave(stringAttribute(params.Item["PK"]), stringAttribute(params.Item["SK"]))
	if _, existe := f.items[clave]; existe {
		return nil, &types.ConditionalCheckFailedException{Message: aws.String("existe")}
	}
	f.items[clave] = remCopiar(params.Item)
	return &dynamodb.PutItemOutput{}, nil
}

func (f *remFakeDynamo) UpdateItem(_ context.Context, params *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	return nil, errors.New("E064–E067 no deben usar UpdateItem suelto: toda mutación es transaccional")
}

func (f *remFakeDynamo) Query(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queries++
	if f.errQuery != nil {
		return nil, f.errQuery
	}
	if aws.ToString(params.KeyConditionExpression) != "#pk = :pk AND begins_with(#sk, :prefijo)" {
		return nil, fmt.Errorf("condición de Query no soportada por el doble: %q", aws.ToString(params.KeyConditionExpression))
	}
	pk := stringAttribute(params.ExpressionAttributeValues[":pk"])
	prefijo := stringAttribute(params.ExpressionAttributeValues[":prefijo"])
	desde := ""
	if len(params.ExclusiveStartKey) > 0 {
		clave, err := f.claveExacta(params.ExclusiveStartKey)
		if err != nil {
			return nil, err
		}
		desde = strings.TrimPrefix(clave, pk+"\x00")
	}

	claves := make([]string, 0, len(f.items))
	for clave := range f.items {
		claves = append(claves, clave)
	}
	sort.Strings(claves)

	evaluados := 0
	ultima := ""
	salida := &dynamodb.QueryOutput{}
	for _, clave := range claves {
		item := f.items[clave]
		if stringAttribute(item["PK"]) != pk || !strings.HasPrefix(stringAttribute(item["SK"]), prefijo) {
			continue
		}
		if stringAttribute(item["SK"]) <= desde {
			continue
		}
		if f.tamanoPagina > 0 && evaluados >= f.tamanoPagina {
			break
		}
		evaluados++
		ultima = stringAttribute(item["SK"])
		cumple, err := remCumpleFiltro(item, params)
		if err != nil {
			return nil, err
		}
		if cumple {
			salida.Items = append(salida.Items, remCopiar(item))
		}
	}
	for _, clave := range claves {
		item := f.items[clave]
		if stringAttribute(item["PK"]) == pk && strings.HasPrefix(stringAttribute(item["SK"]), prefijo) &&
			stringAttribute(item["SK"]) > ultima && ultima != "" {
			salida.LastEvaluatedKey = map[string]types.AttributeValue{
				"PK": stringAttr(pk), "SK": stringAttr(ultima),
			}
			break
		}
	}
	salida.Count = int32(len(salida.Items))
	salida.ScannedCount = int32(evaluados)
	return salida, nil
}

func remCumpleFiltro(item map[string]types.AttributeValue, params *dynamodb.QueryInput) (bool, error) {
	if aws.ToString(params.FilterExpression) == "" {
		return true, nil
	}
	for _, parte := range strings.Split(aws.ToString(params.FilterExpression), " AND ") {
		parte = strings.TrimSpace(parte)
		campos := strings.Split(parte, " = ")
		if len(campos) != 2 {
			return false, fmt.Errorf("filtro no soportado por el doble: %q", parte)
		}
		nombre := campos[0]
		if alias, ok := params.ExpressionAttributeNames[nombre]; ok {
			nombre = alias
		}
		valor, existe := params.ExpressionAttributeValues[campos[1]]
		if !existe {
			return false, fmt.Errorf("filtro sin valor para %q", campos[1])
		}
		actual, ok := item[nombre]
		if !ok {
			return false, nil
		}
		esperadoBOOL, okEsperado := valor.(*types.AttributeValueMemberBOOL)
		actualBOOL, okActual := actual.(*types.AttributeValueMemberBOOL)
		if !okEsperado || !okActual {
			return false, fmt.Errorf("filtro no booleano no soportado: %q", parte)
		}
		if actualBOOL.Value != esperadoBOOL.Value {
			return false, nil
		}
	}
	return true, nil
}

func (f *remFakeDynamo) TransactWriteItems(_ context.Context, params *dynamodb.TransactWriteItemsInput, _ ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.transacciones++
	f.transaccionesVistas = append(f.transaccionesVistas, params)
	if f.errTransact != nil {
		return nil, f.errTransact
	}
	if len(params.TransactItems) == 0 || len(params.TransactItems) > 100 {
		return nil, fmt.Errorf("número de acciones transaccionales inválido: %d", len(params.TransactItems))
	}

	// DynamoDB prohíbe dos acciones sobre el mismo ítem.
	vistas := map[string]bool{}
	razones := make([]types.CancellationReason, 0, len(params.TransactItems))
	fallo := false
	for _, accion := range params.TransactItems {
		clave, condicion, nombres, valores, err := remAccion(accion)
		if err != nil {
			return nil, err
		}
		if vistas[clave] {
			return nil, fmt.Errorf("dos acciones sobre el mismo ítem: %s", clave)
		}
		vistas[clave] = true
		cumple, err := remEvaluaCondicion(f.items[clave], condicion, nombres, valores)
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
		clave, _, _, _, _ := remAccion(accion)
		switch {
		case accion.Put != nil:
			f.items[clave] = remCopiar(accion.Put.Item)
		case accion.Delete != nil:
			delete(f.items, clave)
		case accion.Update != nil:
			item := f.items[clave]
			if item == nil {
				item = map[string]types.AttributeValue{"PK": accion.Update.Key["PK"], "SK": accion.Update.Key["SK"]}
			}
			if err := remAplicaUpdate(item, accion.Update); err != nil {
				return nil, err
			}
			f.items[clave] = item
		default:
			return nil, errors.New("acción transaccional no soportada")
		}
	}
	return &dynamodb.TransactWriteItemsOutput{}, nil
}

func remAccion(accion types.TransactWriteItem) (clave, condicion string, nombres map[string]string, valores map[string]types.AttributeValue, err error) {
	switch {
	case accion.Put != nil:
		clave = remClave(stringAttribute(accion.Put.Item["PK"]), stringAttribute(accion.Put.Item["SK"]))
		return clave, aws.ToString(accion.Put.ConditionExpression), accion.Put.ExpressionAttributeNames, accion.Put.ExpressionAttributeValues, nil
	case accion.Delete != nil:
		clave, err = remClaveExactaDe(accion.Delete.Key)
		return clave, aws.ToString(accion.Delete.ConditionExpression), accion.Delete.ExpressionAttributeNames, accion.Delete.ExpressionAttributeValues, err
	case accion.Update != nil:
		clave, err = remClaveExactaDe(accion.Update.Key)
		return clave, aws.ToString(accion.Update.ConditionExpression), accion.Update.ExpressionAttributeNames, accion.Update.ExpressionAttributeValues, err
	case accion.ConditionCheck != nil:
		clave, err = remClaveExactaDe(accion.ConditionCheck.Key)
		return clave, aws.ToString(accion.ConditionCheck.ConditionExpression), accion.ConditionCheck.ExpressionAttributeNames, accion.ConditionCheck.ExpressionAttributeValues, err
	default:
		return "", "", nil, nil, errors.New("acción transaccional vacía")
	}
}

func remClaveExactaDe(llave map[string]types.AttributeValue) (string, error) {
	if len(llave) != 2 {
		return "", fmt.Errorf("clave transaccional no exacta: %v", llave)
	}
	pk, okPK := llave["PK"].(*types.AttributeValueMemberS)
	sk, okSK := llave["SK"].(*types.AttributeValueMemberS)
	if !okPK || !okSK {
		return "", errors.New("clave transaccional no exacta: PK/SK deben ser strings")
	}
	return remClave(pk.Value, sk.Value), nil
}

func remEvaluaCondicion(item map[string]types.AttributeValue, expresion string, nombres map[string]string, valores map[string]types.AttributeValue) (bool, error) {
	switch expresion {
	case "attribute_not_exists(PK)":
		_, existe := item["PK"]
		return !existe, nil
	case "attribute_exists(PK) AND #version = :expected AND (attribute_not_exists(#deleted) OR #deleted = :falso)":
		if _, existe := item["PK"]; !existe {
			return false, nil
		}
		if !remVersionEsperada(item, nombres["#version"], valores[":expected"]) {
			return false, nil
		}
		if !remBooleanoEsperado(item, nombres["#deleted"], valores[":falso"], true) {
			return false, nil
		}
		return true, nil
	case "attribute_exists(PK) AND #version = :expected AND #reminderSK = :sk":
		if _, existe := item["PK"]; !existe {
			return false, nil
		}
		if !remVersionEsperada(item, nombres["#version"], valores[":expected"]) {
			return false, nil
		}
		actual, ok := item[nombres["#reminderSK"]].(*types.AttributeValueMemberS)
		esperado, okEsperado := valores[":sk"].(*types.AttributeValueMemberS)
		return ok && okEsperado && actual.Value == esperado.Value, nil
	default:
		return false, fmt.Errorf("condición no soportada por el doble de recordatorios: %q", expresion)
	}
}

func remVersionEsperada(item map[string]types.AttributeValue, nombre string, esperado types.AttributeValue) bool {
	actual, ok := item[nombre].(*types.AttributeValueMemberN)
	objetivo, okObjetivo := esperado.(*types.AttributeValueMemberN)
	if !ok || !okObjetivo {
		return false
	}
	return actual.Value == objetivo.Value
}

func remBooleanoEsperado(item map[string]types.AttributeValue, nombre string, esperado types.AttributeValue, ausenteVale bool) bool {
	actual, existe := item[nombre].(*types.AttributeValueMemberBOOL)
	if !existe {
		return ausenteVale
	}
	objetivo, ok := esperado.(*types.AttributeValueMemberBOOL)
	return ok && actual.Value == objetivo.Value
}

func remAplicaUpdate(item map[string]types.AttributeValue, in *types.Update) error {
	expresion := aws.ToString(in.UpdateExpression)
	if !strings.HasPrefix(expresion, "SET ") {
		return fmt.Errorf("expresión de update no soportada por el doble: %q", expresion)
	}
	for _, asignacion := range strings.Split(strings.TrimPrefix(expresion, "SET "), ", ") {
		partes := strings.Split(strings.TrimSpace(asignacion), " = ")
		if len(partes) != 2 {
			return fmt.Errorf("asignación no soportada: %q", asignacion)
		}
		nombre := partes[0]
		if alias, ok := in.ExpressionAttributeNames[nombre]; ok {
			nombre = alias
		}
		valor, ok := in.ExpressionAttributeValues[partes[1]]
		if !ok {
			return fmt.Errorf("valor de expresión ausente: %q", partes[1])
		}
		item[nombre] = valor
	}
	return nil
}

// ---------------------------------------------------------------------------
// Fixtures y utilidades
// ---------------------------------------------------------------------------

const (
	remUsuarioA = "usuario-recordatorios-a"
	remUsuarioB = "usuario-recordatorios-b"
	remID       = "11111111-2222-4333-8444-555555555555"
	remTabla    = "NEXUM_CORE_TEST"
)

var remAhora = time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)

func remEntorno(t *testing.T) (*RemindersStore, *remFakeDynamo, *appReminders.Service) {
	t.Helper()
	doble := nuevoRemFakeDynamo()
	store := NewRemindersStore(doble, remTabla)
	servicio := appReminders.NewService(store, []byte("clave-hmac-recordatorios-de-prueba"))
	servicio.WithClock(func() time.Time { return remAhora })
	return store, doble, servicio
}

func remAlta(usuario, id, clave, carga, titulo, dueAt string) appReminders.CreateInput {
	return appReminders.CreateInput{
		UserID: usuario, ID: id, KeyHash: clave, PayloadHash: carga,
		Title: titulo, DueAt: dueAt, Timezone: "America/Mexico_City", Now: remAhora,
	}
}

func remSK(t *testing.T, dueAt, id string) string {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, dueAt)
	if err != nil {
		t.Fatalf("dueAt invalido: %v", err)
	}
	return reminderSortKey(parsed, id)
}

func remAtributoNumber(t *testing.T, item map[string]types.AttributeValue, nombre string) int64 {
	t.Helper()
	valor, ok := item[nombre].(*types.AttributeValueMemberN)
	if !ok {
		t.Fatalf("atributo %s no numérico: %v", nombre, item[nombre])
	}
	numero, err := strconv.ParseInt(valor.Value, 10, 64)
	if err != nil {
		t.Fatalf("atributo %s no parseable: %v", nombre, err)
	}
	return numero
}

// ---------------------------------------------------------------------------
// Pruebas del adaptador
// ---------------------------------------------------------------------------

func TestE065CreateEscribeRecordatorioPunteroEIdempotencia(t *testing.T) {
	store, doble, _ := remEntorno(t)
	dueAt := "2026-11-24T10:00:00Z"
	if err := store.Create(context.Background(), remAlta(remUsuarioA, remID, "clave-idem-1", "carga-1", "Pagar luz", dueAt)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if doble.transacciones != 1 {
		t.Fatalf("Create debe ser una única transacción, hubo %d", doble.transacciones)
	}
	if len(doble.transaccionesVistas[0].TransactItems) != 3 {
		t.Fatalf("Create debe escribir recordatorio, puntero e idempotencia: %d acciones", len(doble.transaccionesVistas[0].TransactItems))
	}

	pk := reminderPartition(remUsuarioA)
	sk := remSK(t, dueAt, remID)
	recordatorio := doble.item(pk, sk)
	if recordatorio == nil || stringAttribute(recordatorio["entityType"]) != reminderEntity {
		t.Fatalf("falta el recordatorio en %s: %v", sk, recordatorio)
	}
	if remAtributoNumber(t, recordatorio, "version") != 1 {
		t.Fatalf("version inicial: %v", recordatorio["version"])
	}

	puntero := doble.item(pk, reminderPointerSortKey(remID))
	if puntero == nil || stringAttribute(puntero["entityType"]) != reminderPtrEntity {
		t.Fatalf("falta el puntero REMINDERPTR: %v", puntero)
	}
	if stringAttribute(puntero["reminderSK"]) != sk {
		t.Fatalf("el puntero debe guardar la SK completa: %q != %q", stringAttribute(puntero["reminderSK"]), sk)
	}
	if remAtributoNumber(t, puntero, "version") != 1 {
		t.Fatalf("version del puntero: %v", puntero["version"])
	}

	idem := doble.item(pk, reminderIdempotencySortKey("clave-idem-1"))
	if idem == nil || stringAttribute(idem["reminderID"]) != remID || stringAttribute(idem["payloadHash"]) != "carga-1" {
		t.Fatalf("registro de idempotencia inesperado: %v", idem)
	}

	// Lectura tras escritura: el puntero resuelve la SK completa.
	leido, err := store.Get(context.Background(), remUsuarioA, remID)
	if err != nil {
		t.Fatalf("Get tras Create: %v", err)
	}
	if leido == nil || leido.ID != remID || leido.Title != "Pagar luz" || leido.Deleted {
		t.Fatalf("Get devolvió %+v", leido)
	}
	if len(doble.clavesInvalidas) != 0 {
		t.Fatalf("accesos con clave no exacta: %v", doble.clavesInvalidas)
	}
}

func TestE065CreateEsAtomicoYNoDuplica(t *testing.T) {
	store, doble, _ := remEntorno(t)
	dueAt := "2026-11-24T10:00:00Z"
	alta := remAlta(remUsuarioA, remID, "clave-idem-2", "carga-2", "Renta", dueAt)
	if err := store.Create(context.Background(), alta); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.Create(context.Background(), alta); !errors.Is(err, appReminders.ErrConflict) {
		t.Fatalf("reintento con la misma clave e id debe ser conflicto: %v", err)
	}
	// Misma clave, otra carga y otro id: la idempotencia impide crear un segundo recurso.
	otra := remAlta(remUsuarioA, "22222222-3333-4444-8555-666666666666", "clave-idem-2", "carga-distinta", "Renta", dueAt)
	if err := store.Create(context.Background(), otra); !errors.Is(err, appReminders.ErrConflict) {
		t.Fatalf("misma clave con otra carga debe ser conflicto: %v", err)
	}

	sk := remSK(t, dueAt, remID)
	if item := doble.item(reminderPartition(remUsuarioA), sk); item == nil {
		t.Fatal("el recordatorio original no debe desaparecer")
	}
	segundaSK := remSK(t, dueAt, "22222222-3333-4444-8555-666666666666")
	if item := doble.item(reminderPartition(remUsuarioA), segundaSK); item != nil {
		t.Fatal("no debe existir un segundo recordatorio")
	}
	if puntero := doble.item(reminderPartition(remUsuarioA), reminderPointerSortKey(remID)); remAtributoNumber(t, puntero, "version") != 1 {
		t.Fatal("el conflicto no debe modificar el puntero")
	}
}

func TestE066UpdateSinCambioDeFechaActualizaPuntero(t *testing.T) {
	store, doble, _ := remEntorno(t)
	dueAt := "2026-11-24T10:00:00Z"
	if err := store.Create(context.Background(), remAlta(remUsuarioA, remID, "clave-idem-3", "carga-3", "Original", dueAt)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	titulo := "Actualizado"
	actualizado, err := store.Update(context.Background(), appReminders.UpdateInput{
		UserID: remUsuarioA, ID: remID, ExpectedVersion: 1, Title: &titulo, Now: remAhora,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if actualizado == nil || actualizado.Title != titulo || actualizado.Version != 2 {
		t.Fatalf("resultado inesperado: %+v", actualizado)
	}
	puntero := doble.item(reminderPartition(remUsuarioA), reminderPointerSortKey(remID))
	if remAtributoNumber(t, puntero, "version") != 2 {
		t.Fatalf("el puntero debe seguir la versión: %v", puntero["version"])
	}
	if stringAttribute(puntero["reminderSK"]) != remSK(t, dueAt, remID) {
		t.Fatal("la SK no debe cambiar sin cambiar dueAt")
	}

	// CAS: una versión obsoleta no modifica nada.
	viejo := "Viejo"
	if _, err := store.Update(context.Background(), appReminders.UpdateInput{
		UserID: remUsuarioA, ID: remID, ExpectedVersion: 1, Title: &viejo, Now: remAhora,
	}); !errors.Is(err, appReminders.ErrConflict) {
		t.Fatalf("versión obsoleta debe ser conflicto: %v", err)
	}
	if leido, _ := store.Get(context.Background(), remUsuarioA, remID); leido.Title != titulo || leido.Version != 2 {
		t.Fatalf("el conflicto no debe mutar el recurso: %+v", leido)
	}
}

func TestE066UpdateConCambioDeFechaMueveSKYSinPunteroObsoleto(t *testing.T) {
	store, doble, _ := remEntorno(t)
	dueAtViejo := "2026-11-24T10:00:00Z"
	dueAtNuevo := "2026-11-25T09:30:00Z"
	if err := store.Create(context.Background(), remAlta(remUsuarioA, remID, "clave-idem-4", "carga-4", "Mudanza", dueAtViejo)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	nuevo := dueAtNuevo
	actualizado, err := store.Update(context.Background(), appReminders.UpdateInput{
		UserID: remUsuarioA, ID: remID, ExpectedVersion: 1, DueAt: &nuevo, Now: remAhora,
	})
	if err != nil {
		t.Fatalf("Update con cambio de fecha: %v", err)
	}
	if actualizado == nil || actualizado.Version != 2 || !actualizado.DueAt.Equal(remAhoraDe(t, dueAtNuevo)) {
		t.Fatalf("resultado inesperado: %+v", actualizado)
	}

	pk := reminderPartition(remUsuarioA)
	if item := doble.item(pk, remSK(t, dueAtViejo, remID)); item != nil {
		t.Fatal("la SK antigua debe eliminarse en la misma transacción")
	}
	if item := doble.item(pk, remSK(t, dueAtNuevo, remID)); item == nil {
		t.Fatal("la SK nueva debe existir")
	}
	puntero := doble.item(pk, reminderPointerSortKey(remID))
	if stringAttribute(puntero["reminderSK"]) != remSK(t, dueAtNuevo, remID) {
		t.Fatalf("puntero obsoleto: %q", stringAttribute(puntero["reminderSK"]))
	}
	if remAtributoNumber(t, puntero, "version") != 2 {
		t.Fatalf("versión del puntero: %v", puntero["version"])
	}
	if leido, err := store.Get(context.Background(), remUsuarioA, remID); err != nil || leido == nil || leido.ID != remID {
		t.Fatalf("Get tras mover SK: %+v err=%v", leido, err)
	}
	pagina, err := store.List(context.Background(), appReminders.ListQuery{UserID: remUsuarioA, Limit: 10})
	if err != nil || len(pagina.Items) != 1 || !pagina.Items[0].DueAt.Equal(remAhoraDe(t, dueAtNuevo)) {
		t.Fatalf("List tras mover SK: %+v err=%v", pagina, err)
	}

	// El movimiento es transaccional: la acción de borrado y el puntero van juntos.
	ultima := doble.transaccionesVistas[len(doble.transaccionesVistas)-1]
	if len(ultima.TransactItems) != 3 || ultima.TransactItems[1].Delete == nil {
		t.Fatalf("se esperaban Put+Delete+Update; acciones=%d", len(ultima.TransactItems))
	}
}

func remAhoraDe(t *testing.T, valor string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, valor)
	if err != nil {
		t.Fatalf("fecha inválida %q: %v", valor, err)
	}
	return parsed
}

func TestE066FrequencyNilEliminaAtributoYPreservaSinCambio(t *testing.T) {
	store, _, servicio := remEntorno(t)
	servicio.WithIDGenerator(func() (string, error) { return remID, nil })
	frecuencia := appReminders.FrequencyWeekly
	creado, err := servicio.Create(context.Background(), appReminders.CreateCommand{
		UserID: remUsuarioA, IdempotencyKey: "clave-idem-frecuencia", Title: "Gimnasio",
		DueAt: "2026-11-24T10:00:00Z", Timezone: "America/Mexico_City", Frequency: &frecuencia,
	})
	if err != nil || creado == nil || creado.Frequency == nil {
		t.Fatalf("Create con frecuencia: %+v err=%v", creado, err)
	}
	sinFrecuencia, err := store.Update(context.Background(), appReminders.UpdateInput{
		UserID: remUsuarioA, ID: remID, ExpectedVersion: 1,
		Frequency: appReminders.FrequencyPatch{Set: true, Value: nil}, Now: remAhora,
	})
	if err != nil || sinFrecuencia == nil || sinFrecuencia.Frequency != nil {
		t.Fatalf("frequency:null debe cancelar: %+v err=%v", sinFrecuencia, err)
	}
	item := servicioItem(t, store, remUsuarioA, remID)
	if _, existe := item["frequency"]; existe {
		t.Fatal("frequency debe eliminarse del ítem")
	}
}

func servicioItem(t *testing.T, store *RemindersStore, usuario, id string) map[string]types.AttributeValue {
	t.Helper()
	puntero, err := store.pointerFor(context.Background(), usuario, id)
	if err != nil || puntero == nil {
		t.Fatalf("puntero no disponible: %+v err=%v", puntero, err)
	}
	raw, _, err := store.reminderItemAt(context.Background(), usuario, puntero.SortKey)
	if err != nil || raw == nil {
		t.Fatalf("ítem no disponible: err=%v", err)
	}
	return raw
}

func TestE067DeleteMarcaRecordatorioYPuntero(t *testing.T) {
	store, doble, _ := remEntorno(t)
	dueAt := "2026-11-24T10:00:00Z"
	if err := store.Create(context.Background(), remAlta(remUsuarioA, remID, "clave-idem-5", "carga-5", "Borrar", dueAt)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.Delete(context.Background(), remUsuarioA, remID, 1); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	pk := reminderPartition(remUsuarioA)
	recordatorio := doble.item(pk, remSK(t, dueAt, remID))
	if recordatorio == nil || !boolAttribute(recordatorio["deleted"]) || boolAttribute(recordatorio["active"]) {
		t.Fatalf("estado del recordatorio tras Delete: %v", recordatorio)
	}
	if remAtributoNumber(t, recordatorio, "version") != 2 {
		t.Fatalf("versión tras Delete: %v", recordatorio["version"])
	}
	puntero := doble.item(pk, reminderPointerSortKey(remID))
	if puntero == nil || !boolAttribute(puntero["deleted"]) || remAtributoNumber(t, puntero, "version") != 2 {
		t.Fatalf("estado del puntero tras Delete: %v", puntero)
	}

	// El store sigue resolviendo el recurso borrado: el servicio distingue 204 de 404.
	leido, err := store.Get(context.Background(), remUsuarioA, remID)
	if err != nil || leido == nil || !leido.Deleted {
		t.Fatalf("Get de un borrado debe devolver el registro con Deleted=true: %+v err=%v", leido, err)
	}
	if err := store.Delete(context.Background(), remUsuarioA, remID, 2); !errors.Is(err, appReminders.ErrNotFound) {
		t.Fatalf("segundo Delete debe ser ErrNotFound: %v", err)
	}

	// Aislamiento: el mismo identificador en otro usuario es un recurso distinto.
	if err := store.Create(context.Background(), remAlta(remUsuarioB, remID, "clave-idem-6", "carga-6", "Ajeno", dueAt)); err != nil {
		t.Fatalf("Create usuario B: %v", err)
	}
	ajeno, err := store.Get(context.Background(), remUsuarioB, remID)
	if err != nil || ajeno == nil || ajeno.Deleted || ajeno.Title != "Ajeno" {
		t.Fatalf("el borrado no debe afectar al otro usuario: %+v err=%v", ajeno, err)
	}
}

// ---------------------------------------------------------------------------
// Paginación y cursor
// ---------------------------------------------------------------------------

func TestE064NextKeyEsElUltimoDevueltoYNoSaltaElementos(t *testing.T) {
	store, doble, servicio := remEntorno(t)
	doble.tamanoPagina = 2
	base := time.Date(2026, 11, 24, 8, 0, 0, 0, time.UTC)
	ids := []string{
		"11111111-0000-4000-8000-000000000001",
		"11111111-0000-4000-8000-000000000002",
		"11111111-0000-4000-8000-000000000003",
		"11111111-0000-4000-8000-000000000004",
		"11111111-0000-4000-8000-000000000005",
	}
	for i, id := range ids {
		due := base.Add(time.Duration(i) * time.Hour).Format(time.RFC3339)
		if err := store.Create(context.Background(), remAlta(remUsuarioA, id, fmt.Sprintf("clave-pagina-%d", i), fmt.Sprintf("carga-%d", i), fmt.Sprintf("Recordatorio %d", i), due)); err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}

	// El último ítem crudo de la página 2 es un recordatorio distinto: si el
	// cursor se emitiera desde él, se saltaría el elemento intermedio.
	vistos := map[string]bool{}
	cursor := ""
	paginas := 0
	for {
		pagina, err := servicio.List(context.Background(), remUsuarioA, 1, cursor, nil)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		paginas++
		for _, item := range pagina.Items {
			if vistos[item.ID] {
				t.Fatalf("elemento repetido en la paginación: %s", item.ID)
			}
			vistos[item.ID] = true
		}
		if pagina.NextCursor == "" {
			break
		}
		if len(pagina.Items) == 0 {
			t.Fatalf("página intermedia vacía con cursor: %+v", pagina)
		}
		cursor = pagina.NextCursor
		if paginas > 10 {
			t.Fatal("la paginación no termina")
		}
	}
	if len(vistos) != len(ids) {
		t.Fatalf("se perdieron elementos: %d de %d", len(vistos), len(ids))
	}
	for _, id := range ids {
		if !vistos[id] {
			t.Fatalf("cursor saltó el elemento %s", id)
		}
	}

	// NextKey directo: con limit=1 y página cruda de 2, el cursor debe ser la SK
	// del PRIMER recordatorio (el devuelto), no la del segundo.
	primera, err := store.List(context.Background(), appReminders.ListQuery{UserID: remUsuarioA, Limit: 1})
	if err != nil || primera.NextKey == nil {
		t.Fatalf("primera página: %+v err=%v", primera, err)
	}
	esperada := reminderSortKey(remAhoraDe(t, base.Format(time.RFC3339)), ids[0])
	if primera.NextKey.SK != esperada {
		t.Fatalf("NextKey=%q, esperado %q", primera.NextKey.SK, esperada)
	}
	segunda, err := store.List(context.Background(), appReminders.ListQuery{UserID: remUsuarioA, Limit: 1, After: primera.NextKey})
	if err != nil {
		t.Fatalf("segunda página: %v", err)
	}
	if len(segunda.Items) != 1 || segunda.Items[0].ID != ids[1] {
		t.Fatalf("segunda página inesperada: %+v", segunda.Items)
	}
}

func TestE064FiltrosEliminadosEmpatesYLimites(t *testing.T) {
	store, doble, _ := remEntorno(t)
	doble.tamanoPagina = 1
	ids := []string{
		"22222222-0000-4000-8000-000000000001",
		"22222222-0000-4000-8000-000000000002",
		"22222222-0000-4000-8000-000000000003",
	}
	for i, id := range ids {
		due := fmt.Sprintf("2026-11-24T1%d:00:00Z", i)
		if err := store.Create(context.Background(), remAlta(remUsuarioA, id, fmt.Sprintf("clave-filtro-%d", i), "carga", "Filtro", due)); err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}
	inactivo := false
	if _, err := store.Update(context.Background(), appReminders.UpdateInput{
		UserID: remUsuarioA, ID: ids[1], ExpectedVersion: 1, Active: &inactivo, Now: remAhora,
	}); err != nil {
		t.Fatalf("desactivar: %v", err)
	}
	if err := store.Delete(context.Background(), remUsuarioA, ids[2], 1); err != nil {
		t.Fatalf("borrar: %v", err)
	}

	activos := true
	pagina, err := store.List(context.Background(), appReminders.ListQuery{UserID: remUsuarioA, Limit: 10, Active: &activos})
	if err != nil || len(pagina.Items) != 1 || pagina.Items[0].ID != ids[0] || pagina.NextKey != nil {
		t.Fatalf("filtro active=true: %+v err=%v", pagina, err)
	}
	inactivos, err := store.List(context.Background(), appReminders.ListQuery{UserID: remUsuarioA, Limit: 10, Active: &inactivo})
	if err != nil || len(inactivos.Items) != 1 || inactivos.Items[0].ID != ids[1] {
		t.Fatalf("filtro active=false: %+v err=%v", inactivos, err)
	}
	sinFiltro, err := store.List(context.Background(), appReminders.ListQuery{UserID: remUsuarioA, Limit: 10})
	if err != nil || len(sinFiltro.Items) != 2 {
		t.Fatalf("el borrado no debe listarse: %+v err=%v", sinFiltro, err)
	}

	// Límite por defecto y límite explícito.
	porDefecto, err := store.List(context.Background(), appReminders.ListQuery{UserID: remUsuarioA, Limit: 0})
	if err != nil || len(porDefecto.Items) != 2 {
		t.Fatalf("limit=0 debe usar el valor por defecto: %+v err=%v", porDefecto, err)
	}
	if _, err := store.List(context.Background(), appReminders.ListQuery{UserID: remUsuarioA, After: &appReminders.Key{PK: reminderPartition(remUsuarioB), SK: "REMINDER#x"}}); !errors.Is(err, appReminders.ErrInvalidRequest) {
		t.Fatalf("cursor de otro usuario debe rechazarse: %v", err)
	}
}

func TestE064EmpateDeFechaYPaginaVaciaConLastEvaluatedKey(t *testing.T) {
	store, doble, _ := remEntorno(t)
	doble.tamanoPagina = 1
	due := "2026-11-24T09:00:00Z"
	idA := "33333333-0000-4000-8000-00000000000a"
	idB := "33333333-0000-4000-8000-00000000000b"
	if err := store.Create(context.Background(), remAlta(remUsuarioA, idA, "clave-empate-a", "carga", "A", due)); err != nil {
		t.Fatalf("Create A: %v", err)
	}
	// Ítem con el mismo prefijo pero entidad desconocida: la página puede salir
	// vacía y aun así traer LastEvaluatedKey. El bucle debe seguir, no rendirse.
	doble.poner(map[string]types.AttributeValue{
		"PK":         stringAttr(reminderPartition(remUsuarioA)),
		"SK":         stringAttr("REMINDER#2026-11-24T09:00:00.000Z#" + idA + "5"),
		"entityType": stringAttr("RUIDO"),
	})
	if err := store.Create(context.Background(), remAlta(remUsuarioA, idB, "clave-empate-b", "carga", "B", due)); err != nil {
		t.Fatalf("Create B: %v", err)
	}

	primera, err := store.List(context.Background(), appReminders.ListQuery{UserID: remUsuarioA, Limit: 1})
	if err != nil || len(primera.Items) != 1 || primera.NextKey == nil {
		t.Fatalf("primera página: %+v err=%v", primera, err)
	}
	segunda, err := store.List(context.Background(), appReminders.ListQuery{UserID: remUsuarioA, Limit: 1, After: primera.NextKey})
	if err != nil || len(segunda.Items) != 1 || segunda.Items[0].ID != idB || segunda.NextKey != nil {
		t.Fatalf("segunda página tras una página vacía: %+v err=%v", segunda, err)
	}
	if primera.Items[0].ID == segunda.Items[0].ID {
		t.Fatal("el empate de fecha no debe repetir el mismo registro")
	}
}

func TestE064CursorAlteradoYDeOtroUsuario(t *testing.T) {
	store, _, servicio := remEntorno(t)
	if err := store.Create(context.Background(), remAlta(remUsuarioA, remID, "clave-cursor-1", "carga", "Cursor", "2026-11-24T10:00:00Z")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.Create(context.Background(), remAlta(remUsuarioA, "44444444-0000-4000-8000-000000000001", "clave-cursor-2", "carga", "Cursor 2", "2026-11-24T11:00:00Z")); err != nil {
		t.Fatalf("Create 2: %v", err)
	}
	pagina, err := servicio.List(context.Background(), remUsuarioA, 1, "", nil)
	if err != nil || pagina.NextCursor == "" {
		t.Fatalf("List: %+v err=%v", pagina, err)
	}
	if _, err := servicio.List(context.Background(), remUsuarioB, 1, pagina.NextCursor, nil); !errors.Is(err, appReminders.ErrInvalidRequest) {
		t.Fatalf("cursor de otro usuario: %v", err)
	}
	if _, err := servicio.List(context.Background(), remUsuarioA, 1, pagina.NextCursor+"x", nil); !errors.Is(err, appReminders.ErrInvalidRequest) {
		t.Fatalf("cursor alterado: %v", err)
	}
	if _, err := servicio.List(context.Background(), remUsuarioA, 101, "", nil); !errors.Is(err, appReminders.ErrInvalidRequest) {
		t.Fatalf("límite fuera de rango: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Contrato HTTP sobre el adaptador real
// ---------------------------------------------------------------------------

const (
	remTokenA = "token-recordatorios-a"
	remTokenB = "token-recordatorios-b"
)

type remTokens struct{}

func (remTokens) GenerateAccessToken(context.Context, ports.TokenClaims) (string, error) {
	return "", errors.New("no usado")
}
func (remTokens) GenerateRefreshToken(context.Context) (string, error) {
	return "", errors.New("no usado")
}
func (remTokens) ValidateAccessToken(_ context.Context, token string) (*ports.TokenClaims, error) {
	switch token {
	case remTokenA:
		return &ports.TokenClaims{Subject: remUsuarioA, SessionID: "sesion-a", Epoch: 1}, nil
	case remTokenB:
		return &ports.TokenClaims{Subject: remUsuarioB, SessionID: "sesion-b", Epoch: 1}, nil
	default:
		return nil, ports.ErrInvalidToken
	}
}

type remSesiones struct{}

func (remSesiones) AuthorizeSession(context.Context, string, string, int64) error { return nil }

func remServicioHTTP(t *testing.T) (*appReminders.Service, *remFakeDynamo) {
	t.Helper()
	_, doble, servicio := remEntorno(t)
	contador := 0
	servicio.WithIDGenerator(func() (string, error) {
		contador++
		return fmt.Sprintf("55555555-0000-4000-8000-%012d", contador), nil
	})
	return servicio, doble
}

func remPeticion(metodo, ruta, token, cuerpo string, headers map[string]string) *http.Request {
	var req *http.Request
	if cuerpo == "" {
		req = httptest.NewRequest(metodo, ruta, nil)
	} else {
		req = httptest.NewRequest(metodo, ruta, strings.NewReader(cuerpo))
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for nombre, valor := range headers {
		req.Header.Set(nombre, valor)
	}
	return req
}

func remEjecutar(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func remJSON(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var cuerpo map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("cuerpo no JSON: %s (%v)", rec.Body.String(), err)
	}
	return cuerpo
}

func remCodigoError(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	envelope, ok := remJSON(t, rec)["error"].(map[string]any)
	if !ok {
		t.Fatalf("sin ErrorEnvelope: %s", rec.Body.String())
	}
	codigo, _ := envelope["code"].(string)
	return codigo
}

// TestE065HTTP201LocationYReplaySinPanic reproduce el defecto real: el handler
// desreferenciaba un recordatorio nil porque Get no resolvía la SK.
func TestE065HTTP201LocationYReplaySinPanic(t *testing.T) {
	servicio, doble := remServicioHTTP(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := nexumhttp.NewRemindersCreateHandler(servicio, remTokens{}, remSesiones{}, nil, logger)
	cuerpo := `{"title":"Pagar luz","dueAt":"2026-11-24T10:00:00Z","timezone":"America/Mexico_City","frequency":"weekly"}`
	headers := map[string]string{nexumhttp.HeaderIdempotencyKey: "clave-http-e065-0001"}

	rec := remEjecutar(handler, remPeticion(http.MethodPost, "/api/v1/reminders", remTokenA, cuerpo, headers))
	if rec.Code != http.StatusCreated {
		t.Fatalf("201 esperado, obtenido %d: %s", rec.Code, rec.Body.String())
	}
	body := remJSON(t, rec)
	id, _ := body["id"].(string)
	if id == "" {
		t.Fatalf("sin id en la respuesta: %s", rec.Body.String())
	}
	if rec.Header().Get("Location") != "/api/v1/reminders/"+id {
		t.Fatalf("Location=%q", rec.Header().Get("Location"))
	}
	if remAtributoNumber(t, doble.item(reminderPartition(remUsuarioA), reminderPointerSortKey(id)), "version") != 1 {
		t.Fatal("el alta HTTP debe dejar puntero con versión 1")
	}

	// Replay idempotente: mismo recurso, sin panic ni 500.
	rec = remEjecutar(handler, remPeticion(http.MethodPost, "/api/v1/reminders", remTokenA, cuerpo, headers))
	if rec.Code != http.StatusCreated || remJSON(t, rec)["id"] != id {
		t.Fatalf("replay: %d %s", rec.Code, rec.Body.String())
	}
	if remJSON(t, rec)["version"].(float64) != 1 {
		t.Fatalf("el replay no debe incrementar la versión: %s", rec.Body.String())
	}

	// Misma clave con otra carga: 409.
	otro := `{"title":"Otro","dueAt":"2026-11-24T10:00:00Z","timezone":"America/Mexico_City"}`
	rec = remEjecutar(handler, remPeticion(http.MethodPost, "/api/v1/reminders", remTokenA, otro, headers))
	if rec.Code != http.StatusConflict || remCodigoError(t, rec) != "CONFLICT" {
		t.Fatalf("payload distinto: %d %s", rec.Code, rec.Body.String())
	}

	// Lectura tras escritura: sin panic y con el recurso real.
	leido, err := servicio.Get(context.Background(), remUsuarioA, id)
	if err != nil || leido == nil || leido.ID != id {
		t.Fatalf("Get tras alta HTTP: %+v err=%v", leido, err)
	}
}

func TestE066HTTP200428412YMovimientoDeFecha(t *testing.T) {
	servicio, doble := remServicioHTTP(t)
	crear := nexumhttp.NewRemindersCreateHandler(servicio, remTokens{}, remSesiones{}, nil, nil)
	patch := nexumhttp.NewRemindersPatchHandler(servicio, remTokens{}, remSesiones{}, nil, nil)

	rec := remEjecutar(crear, remPeticion(http.MethodPost, "/api/v1/reminders", remTokenA,
		`{"title":"Original","dueAt":"2026-11-24T10:00:00Z","timezone":"America/Mexico_City"}`,
		map[string]string{nexumhttp.HeaderIdempotencyKey: "clave-http-e066-0001"}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("alta: %d %s", rec.Code, rec.Body.String())
	}
	id := remJSON(t, rec)["id"].(string)
	ruta := "/api/v1/reminders/" + id

	rec = remEjecutar(patch, remPeticion(http.MethodPatch, ruta, remTokenA, `{"title":"Sin If-Match"}`, nil))
	if rec.Code != http.StatusPreconditionRequired || remCodigoError(t, rec) != "PRECONDITION_REQUIRED" {
		t.Fatalf("sin If-Match: %d %s", rec.Code, rec.Body.String())
	}
	rec = remEjecutar(patch, remPeticion(http.MethodPatch, ruta, remTokenA, `{"title":"Viejo"}`, map[string]string{"If-Match": `"v99"`}))
	if rec.Code != http.StatusPreconditionFailed || remCodigoError(t, rec) != "VERSION_CONFLICT" {
		t.Fatalf("If-Match viejo: %d %s", rec.Code, rec.Body.String())
	}
	rec = remEjecutar(patch, remPeticion(http.MethodPatch, ruta, remTokenA,
		`{"title":"Nuevo","dueAt":"2026-11-25T09:30:00Z"}`, map[string]string{"If-Match": `"v1"`}))
	if rec.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("ETag") != `"v2"` {
		t.Fatalf("ETag=%q", rec.Header().Get("ETag"))
	}
	if body := remJSON(t, rec); body["title"] != "Nuevo" || body["dueAt"] != "2026-11-25T09:30:00Z" {
		t.Fatalf("cuerpo del patch: %s", rec.Body.String())
	}
	if item := doble.item(reminderPartition(remUsuarioA), remSK(t, "2026-11-24T10:00:00Z", id)); item != nil {
		t.Fatal("la SK antigua debe haberse eliminado")
	}

	// Aislamiento: otro usuario recibe 404 homogéneo.
	rec = remEjecutar(patch, remPeticion(http.MethodPatch, ruta, remTokenB, `{"title":"Ajeno"}`, map[string]string{"If-Match": `"v2"`}))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("otro usuario: %d %s", rec.Code, rec.Body.String())
	}
}

func TestE067HTTP204ReplayAislamientoYListado(t *testing.T) {
	servicio, _ := remServicioHTTP(t)
	crear := nexumhttp.NewRemindersCreateHandler(servicio, remTokens{}, remSesiones{}, nil, nil)
	borrar := nexumhttp.NewRemindersDeleteHandler(servicio, remTokens{}, remSesiones{}, nil, nil)
	listar := nexumhttp.NewRemindersListHandler(servicio, remTokens{}, remSesiones{}, nil, nil)

	rec := remEjecutar(crear, remPeticion(http.MethodPost, "/api/v1/reminders", remTokenA,
		`{"title":"Borrar","dueAt":"2026-11-24T10:00:00Z","timezone":"America/Mexico_City"}`,
		map[string]string{nexumhttp.HeaderIdempotencyKey: "clave-http-e067-0001"}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("alta: %d %s", rec.Code, rec.Body.String())
	}
	id := remJSON(t, rec)["id"].(string)
	ruta := "/api/v1/reminders/" + id

	for intento := 0; intento < 2; intento++ {
		rec = remEjecutar(borrar, remPeticion(http.MethodDelete, ruta, remTokenA, "", nil))
		if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
			t.Fatalf("DELETE %d: %d %q", intento, rec.Code, rec.Body.String())
		}
	}
	rec = remEjecutar(borrar, remPeticion(http.MethodDelete, ruta, remTokenB, "", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("otro usuario: %d %s", rec.Code, rec.Body.String())
	}
	rec = remEjecutar(listar, remPeticion(http.MethodGet, "/api/v1/reminders", remTokenA, "", nil))
	if rec.Code != http.StatusOK || len(remJSON(t, rec)["items"].([]any)) != 0 {
		t.Fatalf("el borrado no debe listarse: %d %s", rec.Code, rec.Body.String())
	}

	// Dos usuarios: cada uno ve solo lo suyo y no puede tocar lo ajeno.
	rec = remEjecutar(crear, remPeticion(http.MethodPost, "/api/v1/reminders", remTokenB,
		`{"title":"De B","dueAt":"2026-11-24T10:00:00Z","timezone":"America/Mexico_City"}`,
		map[string]string{nexumhttp.HeaderIdempotencyKey: "clave-http-e067-0002"}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("alta B: %d %s", rec.Code, rec.Body.String())
	}
	idB := remJSON(t, rec)["id"].(string)
	rec = remEjecutar(listar, remPeticion(http.MethodGet, "/api/v1/reminders", remTokenA, "", nil))
	if rec.Code != http.StatusOK || len(remJSON(t, rec)["items"].([]any)) != 0 {
		t.Fatalf("A no debe ver a B: %d %s", rec.Code, rec.Body.String())
	}
	rec = remEjecutar(borrar, remPeticion(http.MethodDelete, "/api/v1/reminders/"+idB, remTokenA, "", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("A no debe borrar a B: %d %s", rec.Code, rec.Body.String())
	}
}

func TestE064HTTPListaYLimite(t *testing.T) {
	servicio, _ := remServicioHTTP(t)
	crear := nexumhttp.NewRemindersCreateHandler(servicio, remTokens{}, remSesiones{}, nil, nil)
	listar := nexumhttp.NewRemindersListHandler(servicio, remTokens{}, remSesiones{}, nil, nil)
	for i := 0; i < 3; i++ {
		rec := remEjecutar(crear, remPeticion(http.MethodPost, "/api/v1/reminders", remTokenA,
			fmt.Sprintf(`{"title":"R%d","dueAt":"2026-11-24T1%d:00:00Z","timezone":"America/Mexico_City"}`, i, i),
			map[string]string{nexumhttp.HeaderIdempotencyKey: fmt.Sprintf("clave-http-e064-%04d", i)}))
		if rec.Code != http.StatusCreated {
			t.Fatalf("alta %d: %d %s", i, rec.Code, rec.Body.String())
		}
	}
	rec := remEjecutar(listar, remPeticion(http.MethodGet, "/api/v1/reminders?limit=2", remTokenA, "", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("listado: %d %s", rec.Code, rec.Body.String())
	}
	body := remJSON(t, rec)
	if len(body["items"].([]any)) != 2 || body["nextCursor"] == nil {
		t.Fatalf("página inesperada: %s", rec.Body.String())
	}
	rec = remEjecutar(listar, remPeticion(http.MethodGet, "/api/v1/reminders?limit=101", remTokenA, "", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("limit 101: %d %s", rec.Code, rec.Body.String())
	}
}
