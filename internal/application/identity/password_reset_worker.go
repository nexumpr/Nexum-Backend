package identity

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"time"

	"github.com/nexumpr/Nexum-Backend/internal/ports"
)

// ResetWorker procesa la cola durable de recuperacion de contrasena.
//
// Coordinacion: solo el nodo con el lease en Redis trabaja, y cada trabajo se reclama
// de forma atomica en DynamoDB con un `owner` unico y un `fence` creciente. Un worker
// cuyo lease o fence quedo obsoleto NO puede cerrar ni reprogramar el trabajo.
// Entrega at-least-once: si SES acepta y el proceso cae, se reenvia el mismo enlace.
type ResetWorker struct {
	store       ports.ResetStore
	correo      ports.Correo
	sellador    ports.Sellador
	arrendador  ports.Arrendador
	logger      *slog.Logger
	owner       string
	Intervalo   time.Duration
	Lote        int
	Lease       time.Duration
	MaxIntentos int
}

// NewResetWorker construye el worker. Cualquier dependencia nil lo deja inerte.
func NewResetWorker(
	store ports.ResetStore,
	correo ports.Correo,
	sellador ports.Sellador,
	arrendador ports.Arrendador,
	logger *slog.Logger,
) *ResetWorker {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		b = []byte{0, 1, 2, 3, 4, 5, 6, 7}
	}
	return &ResetWorker{
		store:       store,
		correo:      correo,
		sellador:    sellador,
		arrendador:  arrendador,
		logger:      logger,
		owner:       "pwreset-" + hex.EncodeToString(b),
		Intervalo:   15 * time.Second,
		Lote:        25,
		Lease:       30 * time.Second,
		MaxIntentos: 6,
	}
}

// Disponible indica si el worker puede ejecutar.
func (w *ResetWorker) Disponible() bool {
	return w != nil && w.store != nil && w.correo != nil && w.sellador != nil && w.arrendador != nil
}

// Owner expone el identificador de instancia usado en la reclamacion.
func (w *ResetWorker) Owner() string { return w.owner }

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

// Ciclo hace una pasada: comprueba liderazgo y procesa un lote.
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
			_ = w.store.Reintentar(ctx, t, t.Intentos, ahora, ahora, true)
			continue
		}
		fence, ok, err := w.store.Reclamar(ctx, t.OutboxID, w.owner, ahora, w.Lease)
		if err != nil || !ok {
			continue // otro nodo lo tomo
		}
		// El fence de ESTA reclamacion viaja con el trabajo: sin el no se puede cerrar.
		t.Fence = fence
		t.Owner = w.owner

		token, err := w.sellador.Abrir(t.TokenSello, t.UserID+"|"+t.OutboxID)
		if err != nil {
			_ = w.store.Reintentar(ctx, t, t.Intentos, ahora, ahora, true)
			continue
		}
		if _, err := w.correo.EnviarRecuperacion(ctx, t.Destino, token, t.ExpiraEn); err != nil {
			intentos := t.Intentos + 1
			_ = w.store.Reintentar(ctx, t, intentos, ahora.Add(espera(intentos)), ahora, intentos >= w.MaxIntentos)
			continue
		}
		if err := w.store.MarcarEnviado(ctx, t, "", ahora); err == nil {
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
