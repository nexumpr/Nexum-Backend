package account

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"strings"
	"time"

	domainCache "github.com/nexumpr/Nexum-Backend/internal/domain/cache"
	domainDeletion "github.com/nexumpr/Nexum-Backend/internal/domain/deletion"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
)

// Estados del control de purga (diseño §4.1). `RETRY` y `SLOW_RETRY` NO son terminales: un
// trabajo nunca queda abandonado.
const (
	PurgeStatePending   = "PENDING"
	PurgeStateRunning   = "RUNNING"
	PurgeStateRetry     = "RETRY"
	PurgeStateSlowRetry = "SLOW_RETRY"
	PurgeStatePurged    = "PURGED"
)

// Fases de la purga (diseño §5), en orden estricto.
const (
	PurgePhaseCollect   = "COLLECT"
	PurgePhaseExternal  = "EXTERNAL"
	PurgePhasePartition = "PARTITION"
	PurgePhaseVerify    = "VERIFY"
	PurgePhaseTerminal  = "TERMINAL"
)

const (
	// entityUserSession es el `entityType` que escriben E002/E003 en cada sesión.
	entityUserSession = "USER_SESSION"

	// slowRetryDelay es la reprogramación de un trabajo con reintentos agotados.
	slowRetryDelay = 24 * time.Hour

	// leaderKey es la clave de liderazgo del worker en Redis. No es una autoridad de
	// negocio: sólo evita que dos nodos recorran la misma cola; la exclusión real por
	// trabajo la da el `fence` en DynamoDB.
	leaderKey = "nexum:v1:worker:e014:leader"
)

// errPurgeWithoutOwnership indica que el `fence` del trabajador quedó obsoleto: otro nodo
// reclamó el trabajo. No es un fallo transitorio y no se debe reprogramar: el dueño vigente
// es responsable del trabajo.
var errPurgeWithoutOwnership = errors.New("el trabajo fue reclamado por otro nodo")

// errPurgeGlobalOcupado indica que el ítem global referenciado por un puntero tiene un envío
// en curso (lease vigente). Es transitorio: se reintenta y, si el lease vence, se borra. No
// se borra nunca bajo un envío activo.
var errPurgeGlobalOcupado = errors.New("el trabajo global referenciado tiene un envio en curso")

// PurgeCandidate es una entrada de cola descubierta y elegible.
type PurgeCandidate struct {
	UserID    string
	RequestID string
	State     string
	DueAt     time.Time
}

// PurgeControl es el ítem de control `USER#<id>/DELETION#<requestId>`: la fuente de verdad
// del progreso (la cola es sólo un índice de vencimiento).
type PurgeControl struct {
	UserID         string
	RequestID      string
	State          string
	Phase          string
	Owner          string
	Fence          int64
	Attempts       int64
	DueAt          time.Time
	LeaseUntil     time.Time
	EmailHash      string
	CatalogVersion string
	DeletedCount   int64
	UnknownKeys    int64
	RefreshDigests []string
	LastSK         string
}

// ClaimOutcome describe el resultado de intentar reclamar un trabajo.
type ClaimOutcome int

const (
	// ClaimClaimed: este nodo posee el trabajo con un `fence` nuevo.
	ClaimClaimed ClaimOutcome = iota
	// ClaimNotEligible: otro nodo lo posee, su lease está vigente o no ha vencido.
	ClaimNotEligible
	// ClaimAlreadyPurged: el control ya es un tombstone; sólo hay que limpiar la cola.
	ClaimAlreadyPurged
)

// PartitionItem es la proyección mínima de un ítem de la partición del titular.
type PartitionItem struct {
	SK          string
	EntityType  string
	RefreshHash string
	// GlobalSK es el atributo `skGlobal` de un puntero (`OUTBOXPTR#`, `SCHEDULEPTR#`):
	// la SK del ítem que vive en una partición global del servicio. Vacío en el resto de
	// entidades y en un puntero incompleto.
	GlobalSK string
}

// GlobalDeleteResult describe el desenlace de borrar el ítem global al que apunta un
// puntero del titular.
type GlobalDeleteResult int

const (
	// GlobalItemDeleted: el ítem global ya no existe o se borró. El puntero puede borrarse.
	GlobalItemDeleted GlobalDeleteResult = iota
	// GlobalItemBusy: el ítem existe con un envío en curso (lease vigente). No se borra:
	// el trabajo se reintenta cuando el lease vence.
	GlobalItemBusy
	// GlobalItemForeign: el ítem existe pero su `userID` no es el titular. No se toca (es
	// de otro usuario); el puntero se borra y se contabiliza como clave desconocida.
	GlobalItemForeign
)

// RawItem identifica un ítem por su clave completa.
type RawItem struct {
	PK string
	SK string
}

// PurgeStore es el puerto de persistencia del worker. Todas las escrituras de progreso,
// latido, reprogramación y cierre se condicionan a `owner` + `fence`; el adapter lo aplica
// con `ConditionExpression` verificada por DynamoDB, no por convención.
type PurgeStore interface {
	// Discover consulta la partición `QUEUE#DELETION` con consistencia fuerte y la
	// FilterExpression del diseño §3 (PENDING/RETRY/SLOW_RETRY vencidos y RUNNING con
	// `leaseUntil` vencido). Pagina con `LastEvaluatedKey` y devuelve la clave de
	// continuación aunque la página no aporte coincidencias.
	Discover(ctx context.Context, startAfter string, now time.Time, pageSize int) ([]PurgeCandidate, string, error)

	// Claim incrementa `fence` y toma el trabajo de forma atómica.
	Claim(ctx context.Context, candidate PurgeCandidate, owner string, now time.Time, lease time.Duration) (*PurgeControl, ClaimOutcome, error)

	// HoldsLease relee el control con consistencia fuerte y exige owner, fence y lease
	// vigentes antes de cada lote destructivo.
	HoldsLease(ctx context.Context, control *PurgeControl, owner string, fence int64, now time.Time) (bool, error)

	// Heartbeat renueva el lease en control y cola con condición owner+fence. Un dueño
	// anterior no puede latir: la escritura no se aplica y devuelve false.
	Heartbeat(ctx context.Context, control *PurgeControl, owner string, fence int64, now time.Time, lease time.Duration) (bool, error)

	// SaveProgress persiste fase, cursor, digests pendientes y contadores, y renueva el
	// lease. Devuelve false si el fence dejó de ser propio.
	SaveProgress(ctx context.Context, control *PurgeControl, owner string, fence int64, phase, lastSK string, refreshDigests []string, deletedCount, unknownKeys int64, now time.Time, lease time.Duration) (bool, error)

	// ScheduleRetry reprograma `RETRY` o `SLOW_RETRY` con `dueAt` en la misma entrada de
	// cola (SK=requestId) y cede la propiedad.
	ScheduleRetry(ctx context.Context, control *PurgeControl, owner string, fence int64, state string, dueAt time.Time, deletedCount int64, now time.Time) (bool, error)

	// Complete convierte el control en tombstone mínimo y borra la entrada de cola.
	Complete(ctx context.Context, control *PurgeControl, owner string, fence int64, deletedCount int64, now time.Time) (bool, error)

	// ClearQueueEntry elimina una entrada de cola huérfana (control ya PURGED).
	ClearQueueEntry(ctx context.Context, requestID string) error

	// ListPartition recorre la partición del titular con consistencia fuerte y cursor por SK.
	ListPartition(ctx context.Context, userID, startSK string, limit int) ([]PartitionItem, error)
	// DeletePartitionItem borra un ítem de la partición. Nunca se invoca con `DELETION#`.
	DeletePartitionItem(ctx context.Context, userID, sk string) error
	// DeleteGlobalItem borra el ítem global referenciado por un puntero del titular, con
	// condición de ausencia de lease vigente y comprobando que `userID` coincide. Nunca
	// recibe una partición fuera de las declaradas en `domain/deletion`.
	DeleteGlobalItem(ctx context.Context, partition, sk, userID string, now time.Time) (GlobalDeleteResult, error)
	// ListPasswordResetQueue consulta `QUEUE#PWRESET` filtrando por `userId` y pagina.
	ListPasswordResetQueue(ctx context.Context, userID, startSK string, limit int) ([]RawItem, string, error)
	// DeleteRawItem borra un ítem por su clave completa.
	DeleteRawItem(ctx context.Context, pk, sk string) error
	// ExistsItem comprueba con consistencia fuerte si una clave sigue existiendo.
	ExistsItem(ctx context.Context, pk, sk string) (bool, error)
}

// CacheInvalidator invalida la caché de un usuario por prefijo (A-080 §2.4). Se declara
// como interfaz para que el worker no dependa de Redis.
type CacheInvalidator interface {
	DeletePrefix(ctx context.Context, prefix string) error
}

// PurgeWorker ejecuta la purga durable dentro del binario Go existente.
//
// Coordinación: sólo el nodo con el lease en Redis recorre la cola; cada trabajo se reclama
// con un `fence` creciente en DynamoDB y toda escritura posterior exige ese `fence`. Un
// worker cuyo lease o fence quedó obsoleto no puede latir, avanzar ni cerrar el trabajo.
type PurgeWorker struct {
	store  PurgeStore
	cache  CacheInvalidator
	leader ports.Arrendador
	logger *slog.Logger
	owner  string
	env    string
	now    func() time.Time

	Interval    time.Duration
	Lease       time.Duration
	Batch       int
	PageSize    int
	MaxPages    int
	MaxBatches  int
	MaxJobs     int
	MaxAttempts int64

	// resumeKey evita la inanición cuando se alcanza el tope de páginas por ciclo: el
	// ciclo siguiente continúa desde aquí y sólo vuelve al inicio al agotar la partición.
	resumeKey string
}

// NewPurgeWorker construye el worker. Cualquier dependencia nil lo deja inerte.
func NewPurgeWorker(store PurgeStore, cache CacheInvalidator, leader ports.Arrendador, logger *slog.Logger) *PurgeWorker {
	owner := nuevoOwner()
	if logger == nil {
		logger = slog.Default()
	}
	return &PurgeWorker{
		store:       store,
		cache:       cache,
		leader:      leader,
		logger:      logger,
		owner:       owner,
		env:         "",
		now:         func() time.Time { return time.Now().UTC() },
		Interval:    30 * time.Second,
		Lease:       60 * time.Second,
		Batch:       25,
		PageSize:    50,
		MaxPages:    10,
		MaxBatches:  6,
		MaxJobs:     10,
		MaxAttempts: 8,
	}
}

// WithClock permite inyectar un reloj determinista en pruebas.
func (w *PurgeWorker) WithClock(now func() time.Time) *PurgeWorker {
	if now != nil {
		w.now = now
	}
	return w
}

// WithEnvironment fija el entorno usado en el prefijo de caché (`UserCachePrefix`).
func (w *PurgeWorker) WithEnvironment(env string) *PurgeWorker {
	w.env = strings.TrimSpace(env)
	return w
}

// Owner expone el identificador de instancia usado en la reclamación.
func (w *PurgeWorker) Owner() string { return w.owner }

// Disponible indica si el worker puede ejecutar.
func (w *PurgeWorker) Disponible() bool {
	return w != nil && w.store != nil && w.leader != nil
}

// Run se ejecuta hasta cancelar el contexto (apagado controlado del servidor).
func (w *PurgeWorker) Run(ctx context.Context) {
	if !w.Disponible() {
		return
	}
	ticker := time.NewTicker(w.Interval)
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

// Ciclo hace una pasada acotada: comprueba liderazgo, recorre como máximo MaxPages páginas
// y procesa los trabajos descubiertos.
func (w *PurgeWorker) Ciclo(ctx context.Context) int {
	if !w.Disponible() {
		return 0
	}
	lider, err := w.leader.Adquirir(ctx, leaderKey, w.Lease)
	if err != nil || !lider {
		return 0
	}

	procesados := 0
	startAfter := w.resumeKey
	for pagina := 0; pagina < w.MaxPages; pagina++ {
		ahora := w.now().UTC()
		candidatos, siguiente, err := w.store.Discover(ctx, startAfter, ahora, w.PageSize)
		if err != nil {
			w.logger.WarnContext(ctx, "purga E014: no se pudo leer la cola",
				slog.String("error_class", "discover_failed"))
			break
		}
		for _, candidato := range candidatos {
			if w.procesar(ctx, candidato) {
				procesados++
			}
			if w.MaxJobs > 0 && procesados >= w.MaxJobs {
				// Tope por ciclo: la cola no puede monopolizar la goroutine. Se retoma desde
				// la página actual, no desde el principio, para no reescanear lo ya visto.
				w.resumeKey = startAfter
				return procesados
			}
		}
		if siguiente == "" {
			startAfter = ""
			break
		}
		startAfter = siguiente
	}
	// Si se alcanzó el tope con continuación, el ciclo siguiente retoma desde ahí.
	w.resumeKey = startAfter
	return procesados
}

// procesar reclama y ejecuta un trabajo. Devuelve true si este nodo lo trabajó.
func (w *PurgeWorker) procesar(ctx context.Context, candidato PurgeCandidate) bool {
	control, resultado, err := w.store.Claim(ctx, candidato, w.owner, w.now().UTC(), w.Lease)
	if err != nil {
		w.logger.WarnContext(ctx, "purga E014: no se pudo reclamar el trabajo",
			slog.String("error_class", "claim_failed"))
		return false
	}
	switch resultado {
	case ClaimAlreadyPurged:
		// Tombstone ya escrito: la entrada de cola es un residuo sin efecto. Se limpia de
		// forma idempotente; si falla, se reintenta en un ciclo posterior.
		_ = w.store.ClearQueueEntry(ctx, candidato.RequestID)
		return false
	case ClaimNotEligible:
		return false
	}

	for paso := 0; paso < w.MaxBatches; paso++ {
		ahora := w.now().UTC()
		// Comprobación previa a cada lote destructivo: si otro nodo reclamó el trabajo, se
		// aborta SIN borrar nada y se cede.
		propio, err := w.store.HoldsLease(ctx, control, w.owner, control.Fence, ahora)
		if err != nil || !propio {
			return false
		}

		terminado, err := w.paso(ctx, control)
		if err != nil {
			if errors.Is(err, errPurgeWithoutOwnership) {
				// El fence dejó de ser propio: el dueño vigente continúa. No se reprograma.
				return false
			}
			w.reprogramar(ctx, control, err)
			return false
		}
		if terminado {
			return true
		}
	}

	// Presupuesto de trabajo agotado por ciclo: el trabajo queda RUNNING con lease vigente
	// y será reclamado cuando venza. No se pierde el avance: cada paso ya lo persistió.
	// Se late una vez más para no perder la propiedad antes de terminar el ciclo; si otro
	// nodo reclamó el trabajo, el latido no se aplica y no se escribe nada.
	_, _ = w.store.Heartbeat(ctx, control, w.owner, control.Fence, w.now().UTC(), w.Lease)
	return true
}

// paso ejecuta la fase vigente una vez. Devuelve true cuando el trabajo quedó cerrado.
func (w *PurgeWorker) paso(ctx context.Context, control *PurgeControl) (bool, error) {
	switch control.Phase {
	case "", PurgePhaseCollect:
		return false, w.pasoCollect(ctx, control)
	case PurgePhaseExternal:
		return false, w.pasoExternal(ctx, control)
	case PurgePhasePartition:
		return false, w.pasoPartition(ctx, control)
	case PurgePhaseVerify:
		return false, w.pasoVerify(ctx, control)
	case PurgePhaseTerminal:
		return w.pasoTerminal(ctx, control)
	default:
		// Fase desconocida: no se inventa una transición destructiva. Se reprograma para
		// revisión y el trabajo nunca se abandona.
		return false, errors.New("fase de purga desconocida")
	}
}

func (w *PurgeWorker) pasoCollect(ctx context.Context, control *PurgeControl) error {
	// Sólo se recopila el `refreshHash` vigente de cada sesión. Los localizadores
	// históricos que E003 dejó marcados como CONSUMED (`consumedBy`) NO son enumerables por
	// usuario sin un índice nuevo: el servidor ya no conoce sus digests. Quedan con
	// vencimiento lógico por TTL, inutilizables (E003 exige perfil ACTIVE) y su eliminación
	// física es asíncrona, sin plazo prometido. Se trata como el resto de residuos no
	// localizables del diseño §7; no se declaran borrados.
	items, err := w.store.ListPartition(ctx, control.UserID, control.LastSK, w.Batch)
	if err != nil {
		return err
	}
	digests := append([]string(nil), control.RefreshDigests...)
	for _, item := range items {
		if item.EntityType == entityUserSession && strings.TrimSpace(item.RefreshHash) != "" {
			digests = appendUnique(digests, item.RefreshHash)
		}
	}
	fase := PurgePhaseCollect
	lastSK := ""
	if len(items) > 0 {
		lastSK = items[len(items)-1].SK
	}
	if len(items) < w.Batch {
		fase = PurgePhaseExternal
		lastSK = ""
	}
	return w.persistir(ctx, control, fase, lastSK, digests, control.DeletedCount, control.UnknownKeys)
}

func (w *PurgeWorker) pasoExternal(ctx context.Context, control *PurgeControl) error {
	digests := append([]string(nil), control.RefreshDigests...)
	eliminados := 0
	var errBorrado error
	for len(digests) > 0 && eliminados < w.Batch {
		pk, sk := domainDeletion.RefreshLocatorKeys(digests[0])
		if err := w.store.DeleteRawItem(ctx, pk, sk); err != nil {
			errBorrado = err
			break
		}
		digests = digests[1:]
		eliminados++
	}
	if errBorrado != nil {
		return errBorrado
	}

	if strings.TrimSpace(control.EmailHash) != "" {
		pk, sk := domainDeletion.EmailReservationKeys(control.EmailHash)
		if err := w.store.DeleteRawItem(ctx, pk, sk); err != nil {
			return err
		}
	}

	// Cola de recuperación de contraseña: sólo las entradas del titular. Se recorren hasta
	// tres páginas por paso (el límite se aplica antes del filtro).
	cursor := control.LastSK
	for pagina := 0; pagina < 3; pagina++ {
		items, siguiente, err := w.store.ListPasswordResetQueue(ctx, control.UserID, cursor, w.Batch)
		if err != nil {
			return err
		}
		for _, item := range items {
			if err := w.store.DeleteRawItem(ctx, item.PK, item.SK); err != nil {
				return err
			}
		}
		cursor = siguiente
		if siguiente == "" {
			break
		}
	}

	fase := PurgePhaseExternal
	if len(digests) == 0 && cursor == "" {
		fase = PurgePhasePartition
	}
	return w.persistir(ctx, control, fase, cursor, digests, control.DeletedCount, control.UnknownKeys)
}

func (w *PurgeWorker) pasoPartition(ctx context.Context, control *PurgeControl) error {
	items, err := w.store.ListPartition(ctx, control.UserID, control.LastSK, w.Batch)
	if err != nil {
		return err
	}
	borrados := control.DeletedCount
	desconocidos := control.UnknownKeys
	lastSK := ""
	for _, item := range items {
		lastSK = item.SK
		if domainDeletion.IsControlSK(item.SK) {
			// El control de progreso nunca se borra: es la única clave protegida.
			continue
		}
		if domainDeletion.IsGlobalPointerSK(item.SK) {
			// Orden obligatorio: primero el ítem global referenciado (para que ningún
			// worker pueda volver a servirse de él) y sólo después el puntero. Si el
			// ítem global tiene un envío en curso se aborta la página sin borrar nada y
			// el trabajo se reintenta: nunca se deja un puntero colgando que apunte a un
			// ítem todavía en uso, ni se borra un ítem bajo un envío activo.
			ajeno, errPuntero := w.borrarPunteroGlobal(ctx, control.UserID, item)
			if errPuntero != nil {
				return errPuntero
			}
			if ajeno {
				// Puntero incompleto o que apunta a otro titular: no se toca el ítem
				// ajeno, pero el puntero del titular debe desaparecer.
				desconocidos++
			}
			if err := w.store.DeletePartitionItem(ctx, control.UserID, item.SK); err != nil {
				return err
			}
			borrados++
			continue
		}
		if err := w.store.DeletePartitionItem(ctx, control.UserID, item.SK); err != nil {
			return err
		}
		borrados++
		if strings.TrimSpace(item.EntityType) == "" {
			desconocidos++
		}
	}
	fase := PurgePhasePartition
	if len(items) < w.Batch {
		fase = PurgePhaseVerify
		lastSK = ""
	}
	return w.persistir(ctx, control, fase, lastSK, control.RefreshDigests, borrados, desconocidos)
}

// borrarPunteroGlobal resuelve y borra el ítem global al que apunta un puntero del titular.
// Devuelve `ajeno=true` cuando el ítem no se puede borrar porque pertenece a otro titular o
// porque el puntero no declara una referencia utilizable: en ese caso el puntero se elimina
// igualmente y se contabiliza como clave desconocida (nunca se borra datos de terceros).
//
// Sin `Scan`: la única lectura es el puntero (ya obtenido por ListPartition) y la clave
// global exacta que éste declara.
func (w *PurgeWorker) borrarPunteroGlobal(ctx context.Context, userID string, item PartitionItem) (bool, error) {
	partition, ok := domainDeletion.GlobalPartitionForPointer(item.SK)
	if !ok {
		return true, nil
	}
	sk := strings.TrimSpace(item.GlobalSK)
	if sk == "" {
		return true, nil
	}
	resultado, err := w.store.DeleteGlobalItem(ctx, partition, sk, userID, w.now().UTC())
	if err != nil {
		return false, err
	}
	switch resultado {
	case GlobalItemForeign:
		return true, nil
	case GlobalItemDeleted:
		return false, nil
	default:
		// Busy: se reintenta la página completa sin borrar el puntero.
		return false, errPurgeGlobalOcupado
	}
}

func (w *PurgeWorker) pasoVerify(ctx context.Context, control *PurgeControl) error {
	items, err := w.store.ListPartition(ctx, control.UserID, control.LastSK, w.Batch)
	if err != nil {
		return err
	}
	for _, item := range items {
		if !domainDeletion.IsControlSK(item.SK) {
			// Queda un ítem distinto del control: se vuelve a barrer. Nunca se declara
			// PURGED con datos localizables pendientes.
			return w.persistir(ctx, control, PurgePhasePartition, "", control.RefreshDigests, control.DeletedCount, control.UnknownKeys)
		}
	}
	if len(items) >= w.Batch {
		// Puede quedar más partición por revisar (p. ej. sólo el control y una página llena
		// por otros ítems): se continúa desde la última SK.
		return w.persistir(ctx, control, PurgePhaseVerify, items[len(items)-1].SK, control.RefreshDigests, control.DeletedCount, control.UnknownKeys)
	}

	// Partición limpia: comprobar referencias externas.
	for _, digest := range control.RefreshDigests {
		pk, sk := domainDeletion.RefreshLocatorKeys(digest)
		existe, err := w.store.ExistsItem(ctx, pk, sk)
		if err != nil {
			return err
		}
		if existe {
			return w.persistir(ctx, control, PurgePhaseExternal, "", control.RefreshDigests, control.DeletedCount, control.UnknownKeys)
		}
	}
	if strings.TrimSpace(control.EmailHash) != "" {
		pk, sk := domainDeletion.EmailReservationKeys(control.EmailHash)
		existe, err := w.store.ExistsItem(ctx, pk, sk)
		if err != nil {
			return err
		}
		if existe {
			return w.persistir(ctx, control, PurgePhaseExternal, "", control.RefreshDigests, control.DeletedCount, control.UnknownKeys)
		}
	}
	restantes, _, err := w.store.ListPasswordResetQueue(ctx, control.UserID, "", w.Batch)
	if err != nil {
		return err
	}
	if len(restantes) > 0 {
		return w.persistir(ctx, control, PurgePhaseExternal, "", control.RefreshDigests, control.DeletedCount, control.UnknownKeys)
	}

	// Todo verificado: se entra en TERMINAL. Los digests se vacían aquí porque ya se
	// comprobó que no existen; `Complete` vuelve a eliminarlos del control como parte del
	// tombstone mínimo. Si TERMINAL falla, la reanudación entra directamente en TERMINAL y
	// no repite comprobaciones ya superadas.
	return w.persistir(ctx, control, PurgePhaseTerminal, "", nil, control.DeletedCount, control.UnknownKeys)
}

func (w *PurgeWorker) pasoTerminal(ctx context.Context, control *PurgeControl) (bool, error) {
	// La caché se invalida ANTES de declarar PURGED: si Redis no responde, el trabajo queda
	// en RETRY y no se anuncia una eliminación con datos aún servibles desde caché.
	prefijo := domainCache.UserCachePrefix(w.env, control.UserID)
	if w.cache == nil {
		return false, errors.New("invalidador de cache no configurado")
	}
	if err := w.cache.DeletePrefix(ctx, prefijo); err != nil {
		// El ACL de Redis de A-080 deniega SCAN y KEYS de forma deliberada, de modo que
		// DeletePrefix no puede enumerar claves. Eso NO es un fallo transitorio: las
		// entradas del usuario son inalcanzables tras el tombstone (toda ruta autenticada
		// exige PROFILE ACTIVE) y su TTL esta acotado (30 s en agregados). Se registra y se
		// continua con la verificacion durable; cualquier otro error si bloquea el cierre.
		if errors.Is(err, ports.ErrCacheScanDenegado) {
			w.logger.WarnContext(ctx, "E014: invalidacion de cache denegada por ACL; residuo acotado por TTL",
				slog.String("error_class", "cache_invalidation_denied"),
				slog.String("request_id", control.RequestID))
		} else {
			return false, err
		}
	}

	aplicado, err := w.store.Complete(ctx, control, w.owner, control.Fence, control.DeletedCount, w.now().UTC())
	if err != nil {
		return false, err
	}
	if !aplicado {
		return false, errPurgeWithoutOwnership
	}
	// La entrada de cola ya se borró en la misma operación; se reintenta de forma idempotente
	// por si el borrado condicional no se aplicó.
	_ = w.store.ClearQueueEntry(ctx, control.RequestID)
	control.State = PurgeStatePurged
	control.Phase = PurgePhaseTerminal
	return true, nil
}

// persistir guarda el avance con condición de propiedad. Actualiza el control local sólo si
// DynamoDB confirmó la escritura.
func (w *PurgeWorker) persistir(ctx context.Context, control *PurgeControl, fase, lastSK string, digests []string, borrados, desconocidos int64) error {
	lease := w.Lease
	aplicado, err := w.store.SaveProgress(ctx, control, w.owner, control.Fence, fase, lastSK, digests, borrados, desconocidos, w.now().UTC(), lease)
	if err != nil {
		return err
	}
	if !aplicado {
		return errPurgeWithoutOwnership
	}
	control.Phase = fase
	control.LastSK = lastSK
	control.RefreshDigests = append([]string(nil), digests...)
	control.DeletedCount = borrados
	control.UnknownKeys = desconocidos
	return nil
}

// reprogramar aplica RETRY (backoff) o SLOW_RETRY (24 h) tras 8 intentos. Nunca abandona.
func (w *PurgeWorker) reprogramar(ctx context.Context, control *PurgeControl, causa error) {
	ahora := w.now().UTC()
	estado := PurgeStateRetry
	espera := backoffPurga(control.Attempts)
	if control.Attempts >= w.MaxAttempts {
		estado = PurgeStateSlowRetry
		espera = slowRetryDelay
	}
	aplicado, err := w.store.ScheduleRetry(ctx, control, w.owner, control.Fence, estado, ahora.Add(espera), control.DeletedCount, ahora)
	if err != nil || !aplicado {
		w.logger.WarnContext(ctx, "purga E014: no se pudo reprogramar el trabajo",
			slog.String("error_class", "retry_failed"))
		return
	}
	w.logger.InfoContext(ctx, "purga E014: trabajo reprogramado",
		slog.String("estado", estado),
		slog.Int64("intentos", control.Attempts))
	_ = causa
}

// backoffPurga aplica el mismo escalón que el resto de workers del proyecto.
func backoffPurga(intentos int64) time.Duration {
	pasos := []time.Duration{30 * time.Second, 2 * time.Minute, 10 * time.Minute, time.Hour, 6 * time.Hour}
	if intentos <= 0 {
		return pasos[0]
	}
	if intentos >= int64(len(pasos)) {
		return pasos[len(pasos)-1]
	}
	indice := int(intentos - 1)
	// Guarda explícita: el índice debe caer dentro del slice (gosec G602).
	if indice < 0 || indice >= len(pasos) {
		return pasos[len(pasos)-1]
	}
	return pasos[indice]
}

func appendUnique(valores []string, valor string) []string {
	limpio := strings.TrimSpace(valor)
	if limpio == "" {
		return valores
	}
	for _, existente := range valores {
		if existente == limpio {
			return valores
		}
	}
	return append(valores, limpio)
}

func nuevoOwner() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "purga-e014"
	}
	return "purga-e014-" + hex.EncodeToString(b)
}
