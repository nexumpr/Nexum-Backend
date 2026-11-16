package identity

import (
	"context"
	"log/slog"
	"time"

	"github.com/nexumpr/Nexum-Backend/internal/ports"
)

// ResetWorker procesa la cola durable de recuperacion de contrasena.
//
// Coordinacion: solo el nodo que obtiene el lease en Redis trabaja, y cada trabajo se
// reclama de forma atomica en DynamoDB. El lease NO sustituye la reclamacion.
// Semantica de entrega: at-least-once (SES puede aceptar y el proceso caer antes de
// marcar ENVIADO); el reenvio usa el mismo enlace de un solo uso.
type ResetWorker struct {
	store      ports.ResetStore
	correo     ports.Correo
	sellador   ports.Sellador
	arrendador ports.Arrendador
	logger     *slog.Logger
	Intervalo  time.Duration
	Lote       int
	Lease      time.Duration
	AhorroMax  int
}

// NewResetWorker construye el worker. Cualquier dependencia nil lo deja inerte.
func NewResetWorker(
	store ports.ResetStore,
	correo ports.Correo,
	sellador ports.Sellador,
	arrendador ports.Arrendador,
	logger *slog.Logger,
) *ResetWorker {
	return &ResetWorker{
		store:      store,
		correo:     correo,
		sellador:   sellador,
		arrendador: arrendador,
		logger:     logger,
		Intervalo:  15 * time.Second,
		Lote:       25,
		Lease:      30 * time.Second,
		AhorroMax:  6,
	}
}

// Disponible indica si el worker puede ejecutar.
func (w *ResetWorker) Disponible() bool {
	return w != nil && w.store != nil && w.correo != nil && w.sellador != nil && w.arrendador != nil
}

// Run se ejecuta hasta cancelar el contexto (apagado controlado del servidor).
func (w *ResetWorker) Run(ctx context.Context) {
	if !w.Disponible() {
		return
	}
	ticker := time.NewTicker(w.Intervalo)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.Ciclo(ctx)
		}
	}
}

// Ciclo ejecuta una pasada completa: comprueba liderazgo y procesa un lote.
// Devuelve cuantos envios se completaron.
func (w *ResetWorker) Ciclo(ctx context.Context) int {
	if !w.Disponible() {
		return 0
	}
	lider, err := w.arrendador.Adquirir(ctx, "nexum:v1:worker:pwreset:leader", w.Lease)
	if err != nil || !lider {
		return 0
	}
	ahora := time.Now()
	trabajos, err := w.store.Pendientes(ctx, ahora, w.Lote)
	if err != nil {
		if w.logger != nil {
			w.logger.WarnContext(ctx, "no se pudo leer la cola de recuperacion")
		}
		return 0
	}
	enviados := 0
	for _, t := range trabajos {
		if !t.ExpiraEn.After(ahora) {
			_ = w.store.Reintentar(ctx, t.OutboxID, t.Intentos, ahora, ahora, true)
			continue
		}
		ok, err := w.store.Reclamar(ctx, t.OutboxID, ahora, w.Lease)
		if err != nil || !ok {
			continue
		}
		token, err := w.sellador.Abrir(t.TokenSello, t.UserID+"|"+t.OutboxID)
		if err != nil {
			_ = w.store.Reintentar(ctx, t.OutboxID, t.Intentos, ahora, ahora, true)
			continue
		}
		if _, err := w.correo.EnviarRecuperacion(ctx, t.Destino, token, t.ExpiraEn); err != nil {
			intentos := t.Intentos + 1
			_ = w.store.Reintentar(ctx, t.OutboxID, intentos, ahora.Add(espera(intentos)), ahora, intentos >= w.AhorroMax)
			continue
		}
		if err := w.store.MarcarEnviado(ctx, t.OutboxID, "", ahora); err == nil {
			enviados++
		}
	}
	return enviados
}

// espera aplica el backoff aprobado: 30 s, 2 min, 10 min, 1 h, 6 h.
func espera(intentos int) time.Duration {
	pasos := []time.Duration{30 * time.Second, 2 * time.Minute, 10 * time.Minute, time.Hour, 6 * time.Hour}
	if intentos < 1 {
		return pasos[0]
	}
	if intentos > len(pasos) {
		return pasos[len(pasos)-1]
	}
	return pasos[intentos-1]
}
