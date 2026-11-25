package main

// Pruebas del arranque del scheduler durable de recordatorios.
//
// El doble está en el borde del SDK de DynamoDB: por encima corren los adaptadores
// REALES (ScheduleStore y PushOutboxStore), el Servicio REAL de outbox y el
// Scheduler real. Así se comprueba el cableado completo sin AWS: reclamar una
// ocurrencia, encolarla en OUTBOX#GLOBAL y cerrarla desactivando el recordatorio.
//
// No se registra ninguna ruta: el scheduler no es un endpoint y este archivo no
// toca `main.go`.

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appReminders "github.com/nexumpr/Nexum-Backend/internal/application/reminders"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

const schedulerTablaPrueba = "NEXUM_CORE_TEST"

func schedAttrS(valor string) types.AttributeValue { return &types.AttributeValueMemberS{Value: valor} }
func schedAttrN(valor int64) types.AttributeValue {
	return &types.AttributeValueMemberN{Value: strconv.FormatInt(valor, 10)}
}
func schedAttrB(valor bool) types.AttributeValue {
	return &types.AttributeValueMemberBOOL{Value: valor}
}

// schedFakeSDK implementa ScheduleAPI y PushOutboxAPI sobre un mapa.
type schedFakeSDK struct {
	mu            sync.Mutex
	items         map[string]map[string]types.AttributeValue
	queries       int
	updates       int
	transacciones []*dynamodb.TransactWriteItemsInput
}

func nuevoSchedFakeSDK() *schedFakeSDK {
	return &schedFakeSDK{items: map[string]map[string]types.AttributeValue{}}
}

func schedClaveSDK(pk, sk string) string { return pk + "\x00" + sk }

func schedCopiaSDK(item map[string]types.AttributeValue) map[string]types.AttributeValue {
	if item == nil {
		return nil
	}
	copia := make(map[string]types.AttributeValue, len(item))
	for clave, valor := range item {
		copia[clave] = valor
	}
	return copia
}

func (f *schedFakeSDK) poner(item map[string]types.AttributeValue) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.items[schedClaveSDK(textoScheduler(item["PK"]), textoScheduler(item["SK"]))] = schedCopiaSDK(item)
}

func (f *schedFakeSDK) Query(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queries++
	pk := textoScheduler(params.ExpressionAttributeValues[":pk"])
	salida := &dynamodb.QueryOutput{}
	for _, item := range f.items {
		if textoScheduler(item["PK"]) == pk {
			salida.Items = append(salida.Items, schedCopiaSDK(item))
		}
	}
	return salida, nil
}

func (f *schedFakeSDK) GetItem(_ context.Context, params *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	clave := schedClaveSDK(textoScheduler(params.Key["PK"]), textoScheduler(params.Key["SK"]))
	return &dynamodb.GetItemOutput{Item: schedCopiaSDK(f.items[clave])}, nil
}

func (f *schedFakeSDK) PutItem(_ context.Context, params *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.items[schedClaveSDK(textoScheduler(params.Item["PK"]), textoScheduler(params.Item["SK"]))] = schedCopiaSDK(params.Item)
	return &dynamodb.PutItemOutput{}, nil
}

func (f *schedFakeSDK) UpdateItem(_ context.Context, params *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updates++
	clave := schedClaveSDK(textoScheduler(params.Key["PK"]), textoScheduler(params.Key["SK"]))
	item := schedCopiaSDK(f.items[clave])
	if item == nil {
		item = map[string]types.AttributeValue{"PK": params.Key["PK"], "SK": params.Key["SK"]}
	}
	// Aplica las asignaciones SET simples (el adaptador escribe valores absolutos).
	expresion := ""
	if params.UpdateExpression != nil {
		expresion = *params.UpdateExpression
	}
	if strings.HasPrefix(expresion, "SET ") {
		for _, asignacion := range strings.Split(strings.TrimPrefix(expresion, "SET "), ", ") {
			partes := strings.Split(strings.TrimSpace(asignacion), " = ")
			if len(partes) != 2 {
				continue
			}
			nombre := partes[0]
			if alias, ok := params.ExpressionAttributeNames[nombre]; ok {
				nombre = alias
			}
			if valor, ok := params.ExpressionAttributeValues[partes[1]]; ok {
				item[nombre] = valor
			}
		}
	}
	f.items[clave] = item
	return &dynamodb.UpdateItemOutput{Attributes: schedCopiaSDK(item)}, nil
}

func (f *schedFakeSDK) DeleteItem(_ context.Context, params *dynamodb.DeleteItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.items, schedClaveSDK(textoScheduler(params.Key["PK"]), textoScheduler(params.Key["SK"])))
	return &dynamodb.DeleteItemOutput{}, nil
}

func (f *schedFakeSDK) TransactWriteItems(_ context.Context, params *dynamodb.TransactWriteItemsInput, _ ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.transacciones = append(f.transacciones, params)
	for _, accion := range params.TransactItems {
		switch {
		case accion.Put != nil:
			f.items[schedClaveSDK(textoScheduler(accion.Put.Item["PK"]), textoScheduler(accion.Put.Item["SK"]))] = schedCopiaSDK(accion.Put.Item)
		case accion.Delete != nil:
			delete(f.items, schedClaveSDK(textoScheduler(accion.Delete.Key["PK"]), textoScheduler(accion.Delete.Key["SK"])))
		}
	}
	return &dynamodb.TransactWriteItemsOutput{}, nil
}

func (f *schedFakeSDK) stats() (queries, updates, transacciones int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.queries, f.updates, len(f.transacciones)
}

func (f *schedFakeSDK) hayPutEnParticion(particion string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, tx := range f.transacciones {
		for _, accion := range tx.TransactItems {
			if accion.Put != nil && textoScheduler(accion.Put.Item["PK"]) == particion {
				return true
			}
		}
	}
	return false
}

func textoScheduler(valor types.AttributeValue) string {
	if v, ok := valor.(*types.AttributeValueMemberS); ok {
		return v.Value
	}
	return ""
}

// schedDynamoIncompatible sólo implementa DynamoAPI (lo mínimo de otros flujos).
type schedDynamoIncompatible struct{}

func (schedDynamoIncompatible) GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	return &dynamodb.GetItemOutput{}, nil
}

func (schedDynamoIncompatible) TransactWriteItems(context.Context, *dynamodb.TransactWriteItemsInput, ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	return &dynamodb.TransactWriteItemsOutput{}, nil
}

func schedDependencias(dynamo dynamoAdapter.DynamoAPI, tabla string, logger *slog.Logger) *dependenciasOleada5 {
	return &dependenciasOleada5{Dynamo: dynamo, Tabla: tabla, Logger: logger}
}

func schedOpcionesPrueba(ahora time.Time) appReminders.Opciones {
	return appReminders.Opciones{Intervalo: time.Hour, Lease: 30 * time.Second, Lote: 10, Ahora: func() time.Time { return ahora }}
}

// ---------------------------------------------------------------------------
// Fail-closed
// ---------------------------------------------------------------------------

func TestSchedulerOleada5SinDynamoFallaCerrado(t *testing.T) {
	if _, err := iniciarSchedulerOleada5(context.Background(), nil); err == nil {
		t.Fatal("sin dependencias debe fallar, no arrancar un scheduler sin persistencia")
	}
	var ctxNil context.Context
	if _, err := iniciarSchedulerOleada5(ctxNil, &dependenciasOleada5{}); err == nil {
		t.Fatal("un contexto nulo debe rechazarse")
	}
	if _, err := iniciarSchedulerOleada5(context.Background(), &dependenciasOleada5{}); err == nil {
		t.Fatal("sin DynamoDB debe fallar")
	}
	if _, err := iniciarSchedulerOleada5(context.Background(), schedDependencias(nuevoSchedFakeSDK(), "", slog.Default())); err == nil {
		t.Fatal("sin tabla debe fallar")
	}
}

func TestSchedulerOleada5ClienteIncompatibleFallaCerrado(t *testing.T) {
	_, err := iniciarSchedulerOleada5(context.Background(), schedDependencias(schedDynamoIncompatible{}, schedulerTablaPrueba, slog.Default()))
	if err == nil || !strings.Contains(err.Error(), "Query") {
		t.Fatalf("un cliente sin Query/UpdateItem debe rechazarse: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Arranque real con adaptadores reales
// ---------------------------------------------------------------------------

func TestSchedulerOleada5ReclamaEncolaYCompletaSinAWS(t *testing.T) {
	ahora := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	dynamo := nuevoSchedFakeSDK()
	dynamo.poner(map[string]types.AttributeValue{
		"PK":              schedAttrS("SCHEDULE#GLOBAL"),
		"SK":              schedAttrS("DUE#2026-11-24T11:00:00.000000000Z#r-arranque"),
		"entityType":      schedAttrS("REMINDER_SCHEDULE"),
		"userID":          schedAttrS("usuario-arranque"),
		"reminderID":      schedAttrS("r-arranque"),
		"title":           schedAttrS("Pagar la luz"),
		"dueAt":           schedAttrS("2026-11-24T11:00:00Z"),
		"timezone":        schedAttrS("America/Mexico_City"),
		"owner":           schedAttrS(""),
		"leaseUntil":      schedAttrS(""),
		"version":         schedAttrN(1),
		"fence":           schedAttrN(0),
		"attempts":        schedAttrN(0),
		"reminderVersion": schedAttrN(1),
	})
	// Recordatorio y punteros reales, en la forma que escriben E065/E067.
	dynamo.poner(map[string]types.AttributeValue{
		"PK":         schedAttrS("USER#usuario-arranque"),
		"SK":         schedAttrS("REMINDER#2026-11-24T11:00:00.000Z#r-arranque"),
		"entityType": schedAttrS("USER_REMINDER"),
		"reminderID": schedAttrS("r-arranque"),
		"title":      schedAttrS("Pagar la luz"),
		"dueAt":      schedAttrS("2026-11-24T11:00:00Z"),
		"timezone":   schedAttrS("America/Mexico_City"),
		"active":     schedAttrB(true),
		"deleted":    schedAttrB(false),
		"version":    schedAttrN(1),
	})
	dynamo.poner(map[string]types.AttributeValue{
		"PK":         schedAttrS("USER#usuario-arranque"),
		"SK":         schedAttrS("REMINDERPTR#r-arranque"),
		"entityType": schedAttrS("USER_REMINDER_POINTER"),
		"reminderID": schedAttrS("r-arranque"),
		"reminderSK": schedAttrS("REMINDER#2026-11-24T11:00:00.000Z#r-arranque"),
		"deleted":    schedAttrB(false),
		"version":    schedAttrN(1),
	})

	var bitacora bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&bitacora, &slog.HandlerOptions{Level: slog.LevelDebug}))
	router := nexumhttp.NewRouter()

	deps := schedDependencias(dynamo, schedulerTablaPrueba, logger)
	deps.Router = router

	ctx, cancelar := context.WithCancel(context.Background())
	defer cancelar()

	cerrar, err := iniciarSchedulerOleada5Con(ctx, deps, schedOpcionesPrueba(ahora))
	if err != nil {
		t.Fatalf("iniciar scheduler: %v", err)
	}
	if cerrar == nil {
		t.Fatal("debe devolver una función de cierre")
	}

	// Un ciclo real: Query -> Claim -> Encolar (outbox) -> Complete.
	esperarHasta(t, 3*time.Second, func() bool {
		queries, _, _ := dynamo.stats()
		return queries > 0 && dynamo.hayPutEnParticion("OUTBOX#GLOBAL")
	})
	_, updates, transacciones := dynamo.stats()
	if updates < 1 {
		t.Fatal("el Claim debía usar UpdateItem condicional")
	}
	if transacciones < 1 {
		t.Fatal("el cierre debía usar una transacción")
	}

	// El scheduler no registra rutas: cualquier ruta nueva respondería 200/405,
	// no 404, y el router no cambia por arrancarlo.
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/scheduler", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("el scheduler no debe registrar rutas: %d %s", rec.Code, rec.Body.String())
	}

	cerrar()
	cerrar() // idempotente, sin panic

	queriesTrasCerrar, _, _ := dynamo.stats()
	time.Sleep(120 * time.Millisecond)
	queriesFinales, _, _ := dynamo.stats()
	if queriesFinales != queriesTrasCerrar {
		t.Fatalf("el scheduler siguió consultando tras cerrar: %d -> %d", queriesTrasCerrar, queriesFinales)
	}
	if !strings.Contains(bitacora.String(), "scheduler Oleada 5") {
		t.Fatalf("falta el log de arranque/cierre: %s", bitacora.String())
	}
	if strings.Contains(bitacora.String(), "password") || strings.Contains(bitacora.String(), "Pagar la luz") {
		t.Fatalf("el log del scheduler no debe contener datos sensibles ni el título: %s", bitacora.String())
	}
}

func esperarHasta(t *testing.T, limite time.Duration, condicion func() bool) {
	t.Helper()
	fin := time.Now().Add(limite)
	for time.Now().Before(fin) {
		if condicion() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("la condición no se cumplió dentro del plazo")
}
