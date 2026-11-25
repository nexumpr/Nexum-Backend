package notifications

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"time"
)

// Constantes del worker durable de E063.
const (
	// MaxPaginasPorCiclo acota las páginas que un ciclo dedica a un mismo
	// trabajo: un ciclo nunca vacía la bandeja completa en una sola pasada.
	MaxPaginasPorCiclo = 4

	// ReadAllLease es la duración del lease de un trabajo reclamado. Se renueva
	// en cada avance; si el proceso cae, otro worker lo recupera al vencer.
	ReadAllLease = 2 * time.Minute

	// ReadAllMaxIntentos acota los reintentos observados de un trabajo. Al
	// alcanzarlo el trabajo sigue en la cola (nunca se da por completado sin
	// trabajo real) y el backoff queda en su máximo.
	ReadAllMaxIntentos = 8

	// ReadAllLotePorCiclo es cuántos trabajos como máximo reclama un ciclo.
	ReadAllLotePorCiclo = 10

	readAllIntervaloPorDefecto = 5 * time.Second
	readAllReintentoBase       = 2 * time.Second
	readAllReintentoMax        = 30 * time.Second

	// readAllCierrePresencia acota el retiro ordenado del heartbeat cuando el
	// contexto de ejecución ya está cancelado. Si no hay Redis, el TTL de la
	// presencia (60 s) termina de limpiarla sin depender de este cierre.
	readAllCierrePresencia = 3 * time.Second
)

// Clases de log del heartbeat. Se registran sin identificadores de usuario ni
// detalles internos: sólo indican si la capacidad de proceso está anunciada.
const (
	claseHeartbeatRenovado    = "READALL_HEARTBEAT_RENOVADO"
	claseHeartbeatAusente     = "READALL_HEARTBEAT_AUSENTE"
	claseHeartbeatRetirado    = "READALL_HEARTBEAT_RETIRADO"
	claseHeartbeatRetiroFallo = "READALL_HEARTBEAT_RETIRO_FALLIDO"
)

// ReadAllJob es el ítem durable de la cola global de E063:
// `PK=QUEUE#READALL`, `SK=REQ#<createdAt>#<requestId>`.
type ReadAllJob struct {
	UserID     string
	RequestID  string
	CreatedAt  time.Time
	LeaseOwner string
	LeaseUntil time.Time
	Fence      int64
	Attempts   int
}

// WorkerStore es la capacidad de persistencia que necesita ReadAllWorker. La
// implementación productiva es *dynamo.NotificationsStore; los dobles de
// prueba reproducen las condiciones de DynamoDB sin relajarlas.
//
// Lease y fence viven en el ítem de la cola: una reclamación incrementa el
// fence y sólo el dueño vigente puede avanzar, completar o liberar el trabajo.
type WorkerStore interface {
	Store

	// DueReadAll devuelve trabajos sin lease o con lease vencido, acotados.
	DueReadAll(ctx context.Context, now time.Time, limit int) ([]ReadAllJob, error)
	// ClaimReadAll reclama un trabajo de forma atómica. `ok=false` significa
	// que otro worker ganó la carrera; nunca es un error de disponibilidad.
	ClaimReadAll(ctx context.Context, job ReadAllJob, owner string, now time.Time, lease time.Duration) (ReadAllJob, bool, error)
	// ReleaseReadAll libera el lease y fija el siguiente intento.
	ReleaseReadAll(ctx context.Context, job ReadAllJob, now time.Time, retryAt time.Time) error
	// FinishReadAll borra el ítem de cola y el puntero del titular.
	FinishReadAll(ctx context.Context, job ReadAllJob, now time.Time) error
}

// ReadAllWorker ejecuta el fan-out de E063 fuera de la petición HTTP.
//
// Coordinación: la autoridad es el ítem de la cola en DynamoDB. Reclamar es un
// UpdateItem condicional (lease libre o vencido) que incrementa el fence; sólo
// ese fence puede avanzar o cerrar el registro READALL. No hace falta un líder
// en Redis: dos nodos pueden reclamar a la vez sin duplicar trabajo, y la
// recuperación tras una caída es inmediata al vencer el lease.
type ReadAllWorker struct {
	store     WorkerStore
	logger    *slog.Logger
	owner     string
	now       func() time.Time
	presencia PresenciaReadAll
	latido    bool

	// Intervalo es la pausa entre ciclos dentro de Run.
	Intervalo time.Duration
	// Lote es el máximo de trabajos reclamados por ciclo.
	Lote int
	// Lease es la duración de la concesión de cada trabajo.
	Lease time.Duration
	// MaxPaginasPorCiclo acota las páginas procesadas por trabajo y ciclo.
	MaxPaginasPorCiclo int
	// MaxIntentos acota el backoff observado.
	MaxIntentos int
}

// NewReadAllWorker construye el worker. Sin almacén queda inerte: el llamador
// debe tratarlo como fail-closed (no arranca y el trabajo no se ejecutaría).
func NewReadAllWorker(store WorkerStore, logger *slog.Logger) *ReadAllWorker {
	if logger == nil {
		logger = slog.Default()
	}
	return &ReadAllWorker{
		store:              store,
		logger:             logger,
		owner:              nuevoOwnerReadAll(),
		now:                func() time.Time { return time.Now().UTC() },
		Intervalo:          readAllIntervaloPorDefecto,
		Lote:               ReadAllLotePorCiclo,
		Lease:              ReadAllLease,
		MaxPaginasPorCiclo: MaxPaginasPorCiclo,
		MaxIntentos:        ReadAllMaxIntentos,
	}
}

// Disponible indica si el worker puede ejecutar trabajo real.
func (w *ReadAllWorker) Disponible() bool {
	return w != nil && w.store != nil && w.Lease > 0 && w.MaxPaginasPorCiclo > 0
}

// Owner expone el identificador de instancia usado en la reclamación.
func (w *ReadAllWorker) Owner() string {
	if w == nil {
		return ""
	}
	return w.owner
}

// WithClock inyecta un reloj determinista en pruebas.
func (w *ReadAllWorker) WithClock(now func() time.Time) *ReadAllWorker {
	if w != nil && now != nil {
		w.now = now
	}
	return w
}

// ConPresencia conecta el heartbeat compartido. Sin presencia el worker sigue
// procesando la cola, pero ningún nodo anuncia capacidad de proceso y E063
// rechaza trabajo nuevo con 503 (fail-closed).
func (w *ReadAllWorker) ConPresencia(presencia PresenciaReadAll) *ReadAllWorker {
	if w != nil {
		w.presencia = presencia
	}
	return w
}

// Run procesa la cola hasta que se cancela el contexto (apagado controlado).
// Publica el heartbeat al arrancar y en cada ciclo, y lo retira al cerrarse.
func (w *ReadAllWorker) Run(ctx context.Context) {
	if !w.Disponible() || ctx == nil {
		return
	}
	intervalo := w.Intervalo
	if intervalo <= 0 {
		intervalo = readAllIntervaloPorDefecto
	}
	ticker := time.NewTicker(intervalo)
	defer ticker.Stop()
	defer w.retirarPresencia()
	w.latirPresencia(ctx)
	w.Ciclo(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.latirPresencia(ctx)
			w.Ciclo(ctx)
		}
	}
}

// latirPresencia renueva la presencia compartida. Un fallo no detiene el
// procesamiento de trabajos ya encolados, pero el endpoint deja de aceptar
// trabajo nuevo mientras no haya un worker activo visible.
func (w *ReadAllWorker) latirPresencia(ctx context.Context) {
	if w == nil || w.presencia == nil {
		return
	}
	if err := w.presencia.Latir(ctx, w.owner); err != nil {
		w.latido = false
		w.logger.WarnContext(ctx, "E063: heartbeat del worker no renovado",
			slog.String("clase", claseHeartbeatAusente),
			slog.String("error_class", claseErrorReadAll(err)))
		return
	}
	if !w.latido {
		w.logger.InfoContext(ctx, "E063: heartbeat del worker renovado",
			slog.String("clase", claseHeartbeatRenovado))
		w.latido = true
	}
}

// retirarPresencia elimina el heartbeat propio al cerrar. Usa un contexto
// independiente del cancelado para no dejar presencia fantasma hasta el TTL; si
// falla, el TTL de 60 s la elimina igualmente.
func (w *ReadAllWorker) retirarPresencia() {
	if w == nil || w.presencia == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), readAllCierrePresencia)
	defer cancel()
	if err := w.presencia.Retirar(ctx, w.owner); err != nil {
		w.logger.WarnContext(ctx, "E063: heartbeat del worker no se pudo retirar",
			slog.String("clase", claseHeartbeatRetiroFallo),
			slog.String("error_class", claseErrorReadAll(err)))
		return
	}
	w.latido = false
	w.logger.InfoContext(ctx, "E063: heartbeat del worker retirado",
		slog.String("clase", claseHeartbeatRetirado))
}

// Ciclo hace una pasada acotada: reclama como máximo `Lote` trabajos y dedica
// a cada uno como máximo `MaxPaginasPorCiclo` páginas. Devuelve cuántos
// trabajos quedaron completados en esta pasada.
func (w *ReadAllWorker) Ciclo(ctx context.Context) int {
	if !w.Disponible() || ctx == nil {
		return 0
	}
	trabajos, err := w.Due(ctx)
	if err != nil {
		w.logger.WarnContext(ctx, "E063: no se pudo leer la cola durable",
			slog.String("error_class", claseErrorReadAll(err)))
		return 0
	}
	completados := 0
	for _, trabajo := range trabajos {
		if ctx.Err() != nil {
			return completados
		}
		reclamado, ok, err := w.Claim(ctx, trabajo)
		if err != nil {
			w.logger.WarnContext(ctx, "E063: fallo al reclamar el trabajo",
				slog.String("error_class", claseErrorReadAll(err)))
			continue
		}
		if !ok {
			continue // otro worker lo tomó: no es un error
		}
		hecho, err := w.procesar(ctx, reclamado)
		if err != nil {
			if releaseErr := w.Release(ctx, reclamado, err); releaseErr != nil {
				w.logger.WarnContext(ctx, "E063: no se pudo liberar el trabajo",
					slog.String("request_id", reclamado.RequestID),
					slog.String("error_class", claseErrorReadAll(releaseErr)))
			}
			continue
		}
		if hecho {
			completados++
		}
	}
	return completados
}

// Due devuelve los trabajos pendientes con lease libre o vencido.
func (w *ReadAllWorker) Due(ctx context.Context) ([]ReadAllJob, error) {
	if !w.Disponible() {
		return nil, ErrUnavailable
	}
	lote := w.Lote
	if lote < 1 {
		lote = 1
	}
	return w.store.DueReadAll(ctx, w.now().UTC(), lote)
}

// Claim reclama un trabajo con lease y fence nuevos.
func (w *ReadAllWorker) Claim(ctx context.Context, job ReadAllJob) (ReadAllJob, bool, error) {
	if !w.Disponible() {
		return ReadAllJob{}, false, ErrUnavailable
	}
	return w.store.ClaimReadAll(ctx, job, w.owner, w.now().UTC(), w.Lease)
}

// Advance persiste el cursor y renueva el lease del trabajo reclamado.
func (w *ReadAllWorker) Advance(ctx context.Context, job ReadAllJob, next ReadAllState) (ReadAllJob, error) {
	if !w.Disponible() {
		return job, ErrUnavailable
	}
	if err := w.store.AdvanceReadAll(ctx, job.UserID, job.LeaseOwner, next); err != nil {
		return job, err
	}
	job.LeaseUntil = next.LeaseUntil
	return job, nil
}

// Complete cierra el registro READALL y borra el ítem de cola y el puntero. Si
// otro worker ya lo cerró (o perdimos el fence), limpia lo que quede y no
// propaga un conflicto: el trabajo ya no es nuestro.
func (w *ReadAllWorker) Complete(ctx context.Context, job ReadAllJob, processed int64) error {
	if !w.Disponible() {
		return ErrUnavailable
	}
	err := w.store.CompleteReadAll(ctx, job.UserID, job.RequestID, job.LeaseOwner, job.Fence, processed, w.now().UTC())
	if err == nil {
		return nil
	}
	if !errors.Is(err, ErrConflict) {
		return err
	}
	if errFin := w.store.FinishReadAll(ctx, job, w.now().UTC()); errFin != nil && !errors.Is(errFin, ErrConflict) {
		return errFin
	}
	return nil
}

// Release devuelve el trabajo a la cola con el siguiente intento programado.
func (w *ReadAllWorker) Release(ctx context.Context, job ReadAllJob, cause error) error {
	if !w.Disponible() {
		return ErrUnavailable
	}
	ahora := w.now().UTC()
	retryAt := ahora.Add(reintentoReadAll(job.Attempts))
	err := w.store.ReleaseReadAll(ctx, job, ahora, retryAt)
	if err != nil && !errors.Is(err, ErrConflict) {
		return err
	}
	w.logger.WarnContext(ctx, "E063: trabajo liberado para reintento",
		slog.String("request_id", job.RequestID),
		slog.Int("attempts", job.Attempts),
		slog.String("error_class", claseErrorReadAll(cause)))
	return nil
}

// procesar ejecuta como máximo `MaxPaginasPorCiclo` páginas del trabajo
// reclamado. Devuelve true sólo cuando el registro quedó completado.
func (w *ReadAllWorker) procesar(ctx context.Context, job ReadAllJob) (bool, error) {
	estado, err := w.store.LoadReadAll(ctx, job.UserID, job.RequestID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			// El registro desapareció (cuenta eliminada): el trabajo es obsoleto.
			if errFin := w.store.FinishReadAll(ctx, job, w.now().UTC()); errFin != nil && !errors.Is(errFin, ErrConflict) {
				return false, errFin
			}
			return true, nil
		}
		return false, err
	}
	if estado == nil {
		return false, ErrUnavailable
	}
	if estado.Status == StatusCompleted {
		if errFin := w.store.FinishReadAll(ctx, job, w.now().UTC()); errFin != nil && !errors.Is(errFin, ErrConflict) {
			return false, errFin
		}
		return true, nil
	}

	noLeidas := false
	for pagina := 0; pagina < w.MaxPaginasPorCiclo; pagina++ {
		resultado, err := w.store.List(ctx, ListQuery{
			UserID: job.UserID,
			Limit:  ReadAllBatchSize,
			IsRead: &noLeidas,
			After:  estado.Cursor,
		})
		if err != nil {
			return false, err
		}
		if resultado == nil {
			return false, ErrUnavailable
		}
		ahora := w.now().UTC()
		for _, notificacion := range resultado.Items {
			if _, err := w.store.MarkRead(ctx, job.UserID, notificacion.ID, ahora); err != nil {
				if errors.Is(err, ErrNotFound) {
					continue // la notificación desapareció entre la lectura y la marca
				}
				return false, err
			}
		}

		// El total procesado se deriva del diferencial de no leídas: si un lote
		// anterior falló tras marcar algunos items, ésos ya no aparecen y no se
		// cuentan dos veces ni se pierden del informe del registro READALL.
		restantes, err := w.store.CountUnread(ctx, job.UserID)
		if err != nil {
			return false, err
		}
		procesados := estado.InitialUnread - restantes
		if procesados < 0 {
			procesados = 0
		}

		siguiente := *estado
		siguiente.ProcessedCount = procesados
		siguiente.Cursor = resultado.NextKey
		siguiente.LeaseOwner = job.LeaseOwner
		siguiente.LeaseUntil = ahora.Add(w.Lease)
		siguiente.Fence = job.Fence

		renovado, err := w.Advance(ctx, job, siguiente)
		if err != nil {
			if errors.Is(err, ErrConflict) {
				// Perdimos el lease o el registro ya se cerró: no es un fallo
				// transitorio y no debe reintentarse desde este worker.
				return false, nil
			}
			return false, err
		}
		job = renovado
		estado = &siguiente

		if resultado.NextKey == nil {
			if err := w.Complete(ctx, job, procesados); err != nil {
				return false, err
			}
			return true, nil
		}
	}
	// Trabajo grande: se cede el lease para que el siguiente ciclo (o el otro
	// nodo) continúe de inmediato desde el cursor persistido, sin esperar 2 min.
	cesion := w.now().UTC()
	if err := w.store.ReleaseReadAll(ctx, job, cesion, cesion); err != nil && !errors.Is(err, ErrConflict) {
		return false, err
	}
	return false, nil
}

// reintentoReadAll aplica un backoff acotado: 2 s, 4 s, 8 s... hasta 30 s.
func reintentoReadAll(attempts int) time.Duration {
	if attempts < 1 {
		return readAllReintentoBase
	}
	espera := readAllReintentoBase
	for i := 1; i < attempts && espera < readAllReintentoMax; i++ {
		espera *= 2
	}
	if espera > readAllReintentoMax {
		return readAllReintentoMax
	}
	return espera
}

// claseErrorReadAll clasifica el error para el log sin volcar mensajes crudos.
func claseErrorReadAll(err error) string {
	switch {
	case err == nil:
		return "NONE"
	case errors.Is(err, ErrConflict):
		return "LEASE_CONFLICT"
	case errors.Is(err, ErrUnavailable):
		return "DEPENDENCY_UNAVAILABLE"
	case errors.Is(err, ErrNotFound):
		return "NOT_FOUND"
	case errors.Is(err, ErrInvalidRequest):
		return "INVALID_REQUEST"
	default:
		return "INTERNAL"
	}
}

func nuevoOwnerReadAll() string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return "readall-sin-entropia"
	}
	return "readall-" + hex.EncodeToString(bytes)
}
