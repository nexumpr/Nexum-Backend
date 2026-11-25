package main

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

// readAllDynamoFake implementa NotificationsAPI sin AWS. La cola está vacía, de
// modo que el worker no reclama trabajo; se cuenta la consulta para demostrar
// que el ciclo de arranque se ejecutó realmente.
type readAllDynamoFake struct {
	mu        sync.Mutex
	consultas int
}

func (f *readAllDynamoFake) Query(context.Context, *dynamodb.QueryInput, ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	f.mu.Lock()
	f.consultas++
	f.mu.Unlock()
	return &dynamodb.QueryOutput{}, nil
}

func (f *readAllDynamoFake) GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	return &dynamodb.GetItemOutput{}, nil
}

func (f *readAllDynamoFake) PutItem(context.Context, *dynamodb.PutItemInput, ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	return &dynamodb.PutItemOutput{}, nil
}

func (f *readAllDynamoFake) UpdateItem(context.Context, *dynamodb.UpdateItemInput, ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	return &dynamodb.UpdateItemOutput{}, nil
}

func (f *readAllDynamoFake) TransactWriteItems(context.Context, *dynamodb.TransactWriteItemsInput, ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	return &dynamodb.TransactWriteItemsOutput{}, nil
}

func (f *readAllDynamoFake) totalConsultas() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.consultas
}

// dynamoParcialFake implementa DynamoAPI pero no NotificationsAPI: sirve para
// comprobar que un cliente sin las operaciones de la cola no arranca el worker.
type dynamoParcialFake struct{}

func (dynamoParcialFake) TransactWriteItems(context.Context, *dynamodb.TransactWriteItemsInput, ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	return &dynamodb.TransactWriteItemsOutput{}, nil
}

func (dynamoParcialFake) GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	return &dynamodb.GetItemOutput{}, nil
}

func loggerReadAllPrueba() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestIniciarReadAllWorkerOleada4FallaCerrado(t *testing.T) {
	casos := []struct {
		nombre string
		ctx    context.Context
		deps   *dependenciasOleada4
	}{
		{nombre: "contexto_nulo", ctx: nil, deps: &dependenciasOleada4{}},
		{nombre: "dependencias_nulas", ctx: context.Background(), deps: nil},
		{nombre: "sin_dynamo", ctx: context.Background(), deps: &dependenciasOleada4{Tabla: "NEXUM_CORE_TEST", Logger: loggerReadAllPrueba()}},
		{nombre: "sin_tabla", ctx: context.Background(), deps: &dependenciasOleada4{Dynamo: &readAllDynamoFake{}, Logger: loggerReadAllPrueba()}},
		{nombre: "cliente_incompatible", ctx: context.Background(), deps: &dependenciasOleada4{
			Dynamo: dynamoParcialFake{}, Tabla: "NEXUM_CORE_TEST", Logger: loggerReadAllPrueba(),
		}},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			cerrar, err := iniciarReadAllWorkerOleada4(caso.ctx, caso.deps)
			if err == nil {
				if cerrar != nil {
					cerrar()
				}
				t.Fatal("configuración incompleta debía impedir el arranque del worker")
			}
		})
	}
}

func TestIniciarReadAllWorkerOleada4ArrancaYSeCierra(t *testing.T) {
	fake := &readAllDynamoFake{}
	deps := &dependenciasOleada4{
		Dynamo: fake,
		Tabla:  "NEXUM_CORE_TEST",
		Logger: loggerReadAllPrueba(),
	}
	ctx, cancelar := context.WithCancel(context.Background())
	defer cancelar()

	cerrar, err := iniciarReadAllWorkerOleada4(ctx, deps)
	if err != nil || cerrar == nil {
		t.Fatalf("el worker debía arrancar con Dynamo y tabla: %v", err)
	}
	cerrar()
	cerrar() // el cierre es idempotente

	fin := time.Now().Add(2 * time.Second)
	for fake.totalConsultas() < 1 && time.Now().Before(fin) {
		time.Sleep(2 * time.Millisecond)
	}
	if fake.totalConsultas() < 1 {
		t.Fatal("el worker no ejecutó su primer ciclo")
	}
}
