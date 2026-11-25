package main

// Arranque del worker durable de E063 (`POST /api/v1/notifications/read-all`).
//
// El endpoint responde 202 tras persistir el registro READALL, el puntero del
// titular y el ítem de cola `QUEUE#READALL/REQ#<createdAt>#<requestId>` en una
// sola transacción. Este worker procesa esa cola fuera de la petición, en lotes
// acotados por ciclo, con lease y fence en DynamoDB. Sin DynamoDB o sin tabla no
// se arranca: el endpoint seguiría aceptando trabajo que nadie ejecutaría, así
// que el montaje falla cerrado y se registra el motivo.
//
// No requiere Redis para procesar: la exclusión mutua y la recuperación tras
// caída viven en el ítem de cola (lease vencido ⇒ otro nodo lo reclama). Redis
// sí es necesario para anunciar la capacidad de proceso: sin heartbeat vigente
// E063 responde 503 y no acepta trabajo que nadie ejecutaría.

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appNotifications "github.com/nexumpr/Nexum-Backend/internal/application/notifications"
	cacheinfra "github.com/nexumpr/Nexum-Backend/internal/infrastructure/cache"
)

// iniciarReadAllWorkerOleada4 arranca el worker y devuelve su cierre ordenado.
func iniciarReadAllWorkerOleada4(ctx context.Context, d *dependenciasOleada4) (func(), error) {
	if ctx == nil {
		return nil, errors.New("E063: contexto nulo; worker no iniciado")
	}
	if d == nil || d.Dynamo == nil || d.Tabla == "" {
		return nil, errors.New("E063: DynamoDB o tabla no configurados; worker no iniciado")
	}
	cliente, ok := d.Dynamo.(dynamoAdapter.NotificationsAPI)
	if !ok || cliente == nil {
		return nil, errors.New("E063: el cliente DynamoDB no expone las operaciones de la cola READALL")
	}
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}

	worker := appNotifications.NewReadAllWorker(dynamoAdapter.NewNotificationsStore(cliente, d.Tabla), logger)
	if !worker.Disponible() {
		return nil, errors.New("E063: worker no disponible con la configuración actual")
	}
	// Presencia compartida: cada nodo publica su heartbeat; basta uno vigente
	// para que la admisión de E063 acepte trabajo. Sin Redis el worker sigue
	// procesando la cola, pero no anuncia capacidad y el endpoint responde 503.
	if d.Redis != nil {
		worker.ConPresencia(cacheinfra.NewReadAllPresence(d.Redis))
	} else {
		logger.WarnContext(ctx, "E063: Redis no configurado; el worker no publicara heartbeat",
			slog.String("clase", "READALL_HEARTBEAT_AUSENTE"),
			slog.String("motivo", "redis_no_configurado"))
	}

	hijo, cancelar := context.WithCancel(ctx)
	terminado := make(chan struct{})
	go func() {
		defer close(terminado)
		worker.Run(hijo)
	}()

	var unaVez sync.Once
	cerrar := func() {
		unaVez.Do(func() {
			cancelar()
			<-terminado
		})
	}
	logger.InfoContext(ctx, "E063: worker durable iniciado", slog.String("clase", "READALL_WORKER_STARTED"))
	return cerrar, nil
}
