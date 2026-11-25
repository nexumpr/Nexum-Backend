package main

// Arranque del scheduler durable de recordatorios (Oleada 5).
//
// Este archivo NO toca `main.go`: expone `iniciarSchedulerOleada5` para que el
// orquestador la conecte desde `run()`. No arranca nada en `init()` y no registra
// rutas: el scheduler no es un endpoint.
//
// Piezas (todas reales, sin mocks en producción):
//
//	*dynamodb.Client
//	   ├── dynamo.NewScheduleStore     -> reminders.ScheduleStore (SCHEDULE#GLOBAL)
//	   └── dynamo.NewPushOutboxStore   -> pushoutbox.Store      (OUTBOX#GLOBAL)
//	pushoutbox.NewService(store, nil, ...) -> reminders.Producer (solo Encolar;
//	    el envío FCM lo hace el worker de Oleada 4, no este scheduler)
//	reminders.NewScheduler(store, productor, opciones) -> Run(ctx)
//
// Sin DynamoDB o con un cliente que no exponga las operaciones necesarias, la
// función devuelve error y el scheduler NO se inicia: un scheduler sin
// persistencia no puede prometer recordatorios.

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	"github.com/nexumpr/Nexum-Backend/internal/application/pushoutbox"
	appReminders "github.com/nexumpr/Nexum-Backend/internal/application/reminders"
)

// iniciarSchedulerOleada5 construye el scheduler durable y lo arranca en una
// goroutine ligada al contexto. Devuelve una función de cierre idempotente.
//
// El productor solo encola: el `push.Emisor` es nil porque la entrega FCM es
// responsabilidad del worker de Oleada 4, ya arrancado por `main.go`. El outbox
// es el mismo (`OUTBOX#GLOBAL`), de modo que el scheduler y el worker comparten
// trabajo, idempotencia (`eventId`) y bandeja E061.
func iniciarSchedulerOleada5(ctx context.Context, d *dependenciasOleada5) (func(), error) {
	return iniciarSchedulerOleada5Con(ctx, d, appReminders.Opciones{})
}

// iniciarSchedulerOleada5Con permite ajustar las opciones (intervalo, lease,
// lote, reloj) sin duplicar el cableado. Las pruebas usan el reloj inyectado.
func iniciarSchedulerOleada5Con(ctx context.Context, d *dependenciasOleada5, opciones appReminders.Opciones) (func(), error) {
	if ctx == nil {
		return nil, errors.New("scheduler Oleada 5: contexto nulo")
	}
	if d == nil || d.Dynamo == nil || strings.TrimSpace(d.Tabla) == "" {
		return nil, errors.New("scheduler Oleada 5: DynamoDB o tabla no configurados; scheduler no iniciado")
	}
	cliente, ok := d.Dynamo.(dynamoAdapter.ScheduleAPI)
	if !ok || cliente == nil {
		return nil, errors.New("scheduler Oleada 5: el cliente DynamoDB no expone Query/GetItem/UpdateItem/TransactWriteItems")
	}
	outboxAPI, ok := d.Dynamo.(dynamoAdapter.PushOutboxAPI)
	if !ok || outboxAPI == nil {
		return nil, errors.New("scheduler Oleada 5: el cliente DynamoDB no expone las operaciones del outbox push")
	}

	logger := loggerOleada5(d)
	store := dynamoAdapter.NewScheduleStore(cliente, d.Tabla)
	outbox := dynamoAdapter.NewPushOutboxStore(outboxAPI, d.Tabla)
	productor := pushoutbox.NewService(outbox, nil, nil, nil, nil, pushoutbox.Opciones{Logger: logger})

	if opciones.Logger == nil {
		opciones.Logger = logger
	}
	scheduler := appReminders.NewScheduler(store, productor, opciones)

	ctxScheduler, cancelar := context.WithCancel(ctx)
	go scheduler.Run(ctxScheduler)

	logger.InfoContext(ctx, "scheduler Oleada 5: iniciado",
		slog.String("particion", "SCHEDULE#GLOBAL"),
		slog.String("outbox", "OUTBOX#GLOBAL/DUE#"),
		slog.String("owner", store.Owner()))

	var cerrar sync.Once
	return func() {
		cerrar.Do(func() {
			cancelar()
			logger.InfoContext(context.Background(), "scheduler Oleada 5: detenido")
		})
	}, nil
}
