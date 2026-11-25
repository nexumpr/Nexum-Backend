package reminders

// Scheduler durable de recordatorios (Oleada 5).
//
// Diseño aprobado (Gate de Oleada 5, `SCHEDULE#GLOBAL`):
//
//	PK = SCHEDULE#GLOBAL
//	SK = DUE#<RFC3339 UTC de ancho fijo>#<reminderID>
//	     atributos: userID, reminderID, title, dueAt, timezone, frequency
//	                (ausente = disparo único), owner, leaseUntil, fence,
//	                attempts, version, reminderVersion
//
//	PK = USER#<sub>
//	SK = SCHEDULEPTR#<reminderID>   -> skGlobal (SK exacta del ítem global)
//
// El ítem global vive en una partición del SERVICIO y se localiza con una Query
// acotada por SK; el puntero vive en la partición del titular y existe para que
// E014 pueda purgar el ítem global sin `Scan` (contrato de punteros). Ambos se
// escriben y se retiran en la MISMA transacción que el recordatorio (ver
// `reminders_store.go`), de modo que no existe estado parcial observable.
//
// Coordinación entre los dos nodos, sin tercer servidor:
//
//   - `Claim` es un `UpdateItem` condicional que sólo prospera si el lease
//     venció (`leaseUntil <= ahora` o no hay `owner`). Incrementa `owner`,
//     `fence`, `version` y `attempts`, y devuelve el `Lease` con los valores
//     exactos que exigirán las mutaciones posteriores.
//   - `Advance`/`Complete`/`Release` exigen `owner` + `fence` + `version`; si
//     otro nodo reclamó o el titular editó el recordatorio (E066) el lease deja
//     de valer y la mutación se rechaza con `ErrLeasePerdido`, sin reintento
//     inmediato. Nunca se escribe a medias: son transacciones.
//   - Un lease vencido es la recuperación tras un reinicio: cualquier nodo puede
//     reclamar el trabajo huérfano en el ciclo siguiente.
//
// Entrega: el scheduler NO envía notificaciones; encola un trabajo durable en el
// outbox push (Oleada 4), que es idempotente por `eventID`. El `eventID` es
// determinista (`reminder:<reminderID>:<dueAt RFC3339Nano>`) y `OcurridoEn` es la
// propia ocurrencia, de modo que un replay reutiliza la MISMA clave de outbox y no
// duplica el mensaje.
//
// El estado de la cuenta (`PROFILE ACTIVE`) lo verifica el outbox en la misma
// transacción que encola (ConditionCheck) y lo comunica con
// `pushoutbox.ErrCuentaNoActiva`; esa señal es el `EstadoUsuario` del scheduler y
// evita que la capa de aplicación importe el adaptador de DynamoDB.

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/nexumpr/Nexum-Backend/internal/application/pushoutbox"
)

const (
	// DefaultSchedulerInterval es el periodo entre ciclos.
	DefaultSchedulerInterval = 30 * time.Second
	// DefaultSchedulerLease es la vigencia de la reclamación de una ocurrencia.
	DefaultSchedulerLease = 30 * time.Second
	// DefaultSchedulerBatch es el número máximo de ocurrencias por ciclo.
	DefaultSchedulerBatch = 25
	// MaxSchedulerBatch acota el lote: DynamoDB limita la página y un lote mayor
	// no aporta y dificulta el drenado ordenado.
	MaxSchedulerBatch = 100
	// DefaultSchedulerMaxAttempts es el intento a partir del cual el backoff deja
	// de crecer (se reprograma al máximo). No descarta trabajo: un recordatorio no
	// entregado no se puede reconstruir.
	DefaultSchedulerMaxAttempts = 5

	// TipoNotificacionRecordatorio es el `type` del trabajo del outbox y de la
	// entrada E061. Es una constante, no contiene datos del titular.
	TipoNotificacionRecordatorio = "reminder"

	// marcaEventoRecordatorio precede al eventID determinista.
	marcaEventoRecordatorio = "reminder:"
)

// Errores del scheduler. Los dos primeros son señales de coordinación, no fallos
// de servicio: el ciclo los trata como «el trabajo ya no es nuestro».
var (
	// ErrLeasePerdido indica que otro nodo reclamó la ocurrencia o que el titular
	// modificó el recordatorio después de la reclamación.
	ErrLeasePerdido = errors.New("el lease de la ocurrencia no pertenece a este nodo")
	// ErrOcurrenciaAusente indica que la ocurrencia o su recordatorio ya no existen.
	ErrOcurrenciaAusente = errors.New("la ocurrencia programada no existe")
	// ErrOcurrenciaInvalida indica una ocurrencia sin claves o un lease no positivo.
	ErrOcurrenciaInvalida = errors.New("ocurrencia programada inválida")
	// ErrZonaInvalida indica un nombre IANA no cargable.
	ErrZonaInvalida = errors.New("zona IANA inválida para la recurrencia")
	// ErrFrecuenciaInvalida indica una frecuencia fuera del catálogo cerrado.
	ErrFrecuenciaInvalida = errors.New("frecuencia de recurrencia inválida")
)

// Ocurrencia es una fila materializada de `SCHEDULE#GLOBAL`.
type Ocurrencia struct {
	SK              string
	UserID          string
	ReminderID      string
	Titulo          string
	DueAt           time.Time
	Timezone        string
	Frequency       *Frequency
	ReminderVersion int64
	Version         int64
	Fence           int64
	Attempts        int
	Owner           string
	LeaseUntil      time.Time
}

// Disponible indica si la ocurrencia puede reclamarse en `ahora`: su
// `leaseUntil` venció (o está vacío). El `owner` no participa: tras un `Release`
// el trabajo sigue retenido hasta el reintento programado, aunque ya no tenga
// dueño. Es una comprobación de aplicación; la autoridad es la condición atómica
// de `Claim`.
func (o Ocurrencia) Disponible(ahora time.Time) bool {
	if o.LeaseUntil.IsZero() {
		return true
	}
	return !o.LeaseUntil.After(ahora.UTC())
}

// Lease es el token de propiedad devuelto por `Claim`.
type Lease struct {
	Ocurrencia Ocurrencia
	Owner      string
	Fence      int64
	Version    int64
	Attempts   int
	LeaseUntil time.Time
}

// ScheduleStore es el puerto de persistencia del scheduler. La implementación
// productiva es *dynamo.ScheduleStore; en pruebas se sustituye por un doble con
// estado que falla si se intenta paginar con `Scan` (la interfaz no lo expone).
type ScheduleStore interface {
	// Due devuelve hasta `limite` ocurrencias vencidas y no reclamadas.
	Due(ctx context.Context, ahora time.Time, limite int) ([]Ocurrencia, error)
	// Claim toma el lease de una ocurrencia. Devuelve ErrLeasePerdido si otro
	// nodo la reclamó o si la ocurrencia cambió desde la lectura.
	Claim(ctx context.Context, oc Ocurrencia, ahora time.Time, lease time.Duration) (*Lease, error)
	// Advance mueve la ocurrencia (y su recordatorio) a `siguiente`.
	Advance(ctx context.Context, lease Lease, siguiente time.Time, ahora time.Time) error
	// Complete retira la ocurrencia y desactiva el recordatorio (disparo único,
	// cuenta no activa o recurrencia inválida).
	Complete(ctx context.Context, lease Lease, ahora time.Time) error
	// Release libera el lease y deja `siguiente` como instante mínimo de reintento.
	Release(ctx context.Context, lease Lease, motivo string, siguiente time.Time, ahora time.Time) error
}

// Producer es el productor durable de notificaciones. Lo satisface
// *pushoutbox.Service. Debe ser idempotente por `Solicitud.EventID` y devolver
// `pushoutbox.ErrCuentaNoActiva` cuando el titular ya no está ACTIVE.
type Producer interface {
	Encolar(ctx context.Context, solicitud pushoutbox.Solicitud) (string, bool, error)
}

// Opciones ajusta el scheduler sin acoplarlo al runtime HTTP.
type Opciones struct {
	Intervalo   time.Duration
	Lease       time.Duration
	Lote        int
	MaxIntentos int
	Logger      *slog.Logger
	Ahora       func() time.Time
}

func (o Opciones) conDefaults() Opciones {
	if o.Intervalo <= 0 {
		o.Intervalo = DefaultSchedulerInterval
	}
	if o.Lease <= 0 {
		o.Lease = DefaultSchedulerLease
	}
	if o.Lote <= 0 {
		o.Lote = DefaultSchedulerBatch
	}
	if o.Lote > MaxSchedulerBatch {
		o.Lote = MaxSchedulerBatch
	}
	if o.MaxIntentos <= 0 {
		o.MaxIntentos = DefaultSchedulerMaxAttempts
	}
	if o.Ahora == nil {
		o.Ahora = func() time.Time { return time.Now().UTC() }
	}
	return o
}

// ResultadoCiclo resume lo ocurrido en una pasada. Se usa en pruebas y en el log.
type ResultadoCiclo struct {
	Reclamadas     int
	Encoladas      int
	Replays        int
	Completadas    int
	Avanzadas      int
	Liberadas      int
	CuentaInactiva int
	Abandonadas    int
}

// Scheduler es el worker durable. Se construye una sola vez por proceso.
//
// La identidad de nodo que viaja en `owner` pertenece al `ScheduleStore` (una por
// proceso, como en el outbox push): así el scheduler de aplicación no necesita
// conocerla ni puede falsificarla, y las pruebas la fijan con `WithOwner`.
type Scheduler struct {
	store     ScheduleStore
	productor Producer
	opciones  Opciones
}

// NewScheduler construye el scheduler. Sin almacén o sin productor queda inerte:
// `Disponible` es falso y `Ciclo` no escribe nada (fail-closed).
func NewScheduler(store ScheduleStore, productor Producer, opciones Opciones) *Scheduler {
	return &Scheduler{
		store:     store,
		productor: productor,
		opciones:  opciones.conDefaults(),
	}
}

// Disponible indica si el scheduler puede ejecutar.
func (s *Scheduler) Disponible() bool {
	return s != nil && s.store != nil && s.productor != nil
}

// Run ejecuta un ciclo inmediato y continúa con un ticker hasta cancelar el
// contexto. No se invoca desde `init()`: el arranque lo compone el binario.
func (s *Scheduler) Run(ctx context.Context) {
	if !s.Disponible() || ctx == nil {
		return
	}
	s.Ciclo(ctx)
	ticker := time.NewTicker(s.opciones.Intervalo)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.Ciclo(ctx)
		}
	}
}

// Ciclo hace una pasada: lee vencidas, reclama una a una, encola y cierra.
func (s *Scheduler) Ciclo(ctx context.Context) ResultadoCiclo {
	var resultado ResultadoCiclo
	if !s.Disponible() {
		return resultado
	}
	ahora := s.opciones.Ahora().UTC()
	ocurrencias, err := s.store.Due(ctx, ahora, s.opciones.Lote)
	if err != nil {
		s.avisar(ctx, "no se pudieron leer las ocurrencias programadas",
			slog.String("clase", "SCHEDULE_LECTURA"))
		return resultado
	}
	for _, oc := range ocurrencias {
		if ctx.Err() != nil {
			break
		}
		lease, errReclamo := s.store.Claim(ctx, oc, ahora, s.opciones.Lease)
		if errReclamo != nil {
			if errors.Is(errReclamo, ErrLeasePerdido) || errors.Is(errReclamo, ErrOcurrenciaAusente) {
				// Otro nodo ganó la reclamación: no se duplica el mensaje.
				resultado.Abandonadas++
				continue
			}
			s.avisar(ctx, "no se pudo reclamar la ocurrencia",
				slog.String("clase", "SCHEDULE_RECLAMO"),
				slog.String("reminderID", oc.ReminderID))
			continue
		}
		if lease == nil {
			resultado.Abandonadas++
			continue
		}
		resultado.Reclamadas++
		s.procesar(ctx, *lease, ahora, &resultado)
	}
	if resultado.Reclamadas > 0 || resultado.Encoladas > 0 {
		if s.opciones.Logger != nil {
			s.opciones.Logger.DebugContext(ctx, "ciclo del scheduler de recordatorios",
				slog.Int("reclamadas", resultado.Reclamadas),
				slog.Int("encoladas", resultado.Encoladas),
				slog.Int("replays", resultado.Replays),
				slog.Int("completadas", resultado.Completadas),
				slog.Int("avanzadas", resultado.Avanzadas),
				slog.Int("liberadas", resultado.Liberadas),
				slog.Int("cuenta_inactiva", resultado.CuentaInactiva))
		}
	}
	return resultado
}

// procesar encola la notificación y cierra la ocurrencia según el desenlace.
func (s *Scheduler) procesar(ctx context.Context, lease Lease, ahora time.Time, resultado *ResultadoCiclo) {
	oc := lease.Ocurrencia
	_, creado, err := s.productor.Encolar(ctx, solicitudRecordatorio(oc))
	switch {
	case err == nil:
		if creado {
			resultado.Encoladas++
		} else {
			// Replay de la misma ocurrencia: el outbox ya tenía la clave exacta.
			resultado.Replays++
		}
		s.cerrarOcurrencia(ctx, lease, ahora, resultado)
	case errors.Is(err, pushoutbox.ErrCuentaNoActiva):
		// Cuenta eliminada o en purga: se retira sin enviar y sin reintentar.
		// No es un fallo transitorio: reintentar crearía datos de una cuenta dada
		// de baja y un bucle infinito.
		resultado.CuentaInactiva++
		if errCerrar := s.store.Complete(ctx, lease, ahora); errCerrar != nil {
			s.avisar(ctx, "no se pudo retirar la ocurrencia de una cuenta no activa",
				slog.String("clase", "SCHEDULE_COMPLETE"),
				slog.String("detalle", claseDeError(errCerrar)),
				slog.String("reminderID", oc.ReminderID))
			return
		}
		resultado.Completadas++
	default:
		motivo := claseDeError(err)
		siguiente := ahora.Add(s.backoff(lease.Attempts))
		if errLiberar := s.store.Release(ctx, lease, motivo, siguiente, ahora); errLiberar != nil {
			s.avisar(ctx, "no se pudo liberar la ocurrencia",
				slog.String("clase", "SCHEDULE_RELEASE"),
				slog.String("detalle", claseDeError(errLiberar)),
				slog.String("reminderID", oc.ReminderID))
			return
		}
		resultado.Liberadas++
		s.avisar(ctx, "encolado no disponible; ocurrencia reprogramada",
			slog.String("clase", motivo),
			slog.String("reminderID", oc.ReminderID),
			slog.Int("intentos", lease.Attempts))
	}
}

// cerrarOcurrencia completa un disparo único o avanza la recurrencia.
func (s *Scheduler) cerrarOcurrencia(ctx context.Context, lease Lease, ahora time.Time, resultado *ResultadoCiclo) {
	oc := lease.Ocurrencia
	if oc.Frequency == nil {
		if err := s.store.Complete(ctx, lease, ahora); err != nil {
			s.rechazoDeCierre(ctx, "SCHEDULE_COMPLETE", oc, err)
			return
		}
		resultado.Completadas++
		return
	}
	siguiente, err := NextOccurrence(oc.DueAt, oc.Timezone, *oc.Frequency)
	if err != nil {
		// Una recurrencia irrecuperable no puede avanzar: reintentarla sería un
		// bucle infinito. Se retira la ocurrencia (el recordatorio sigue visible
		// como inactivo) y se registra la clase.
		s.avisar(ctx, "recurrencia inválida; se retira la ocurrencia",
			slog.String("clase", "RECURRENCIA_INVALIDA"),
			slog.String("reminderID", oc.ReminderID),
			slog.String("detalle", claseDeError(err)))
		if errCerrar := s.store.Complete(ctx, lease, ahora); errCerrar != nil {
			s.rechazoDeCierre(ctx, "SCHEDULE_COMPLETE", oc, errCerrar)
			return
		}
		resultado.Completadas++
		return
	}
	if errAvanzar := s.store.Advance(ctx, lease, siguiente, ahora); errAvanzar != nil {
		s.rechazoDeCierre(ctx, "SCHEDULE_ADVANCE", oc, errAvanzar)
		return
	}
	resultado.Avanzadas++
}

// rechazoDeCierre registra un cierre que no pudo aplicarse. Un lease perdido no
// es un error de servicio: otro nodo (o el titular con E066) ya lo resolvió.
func (s *Scheduler) rechazoDeCierre(ctx context.Context, clase string, oc Ocurrencia, err error) {
	if errors.Is(err, ErrLeasePerdido) || errors.Is(err, ErrOcurrenciaAusente) {
		s.avisar(ctx, "el lease se perdió antes de cerrar la ocurrencia",
			slog.String("clase", clase),
			slog.String("reminderID", oc.ReminderID))
		return
	}
	s.avisar(ctx, "no se pudo cerrar la ocurrencia",
		slog.String("clase", clase),
		slog.String("detalle", claseDeError(err)),
		slog.String("reminderID", oc.ReminderID))
}

// backoff devuelve el retraso antes de reintentar. Se acota con MaxIntentos para
// no crecer sin límite; el trabajo NO se descarta al alcanzarlo.
func (s *Scheduler) backoff(intentos int) time.Duration {
	if intentos < 1 {
		intentos = 1
	}
	if intentos > s.opciones.MaxIntentos {
		intentos = s.opciones.MaxIntentos
	}
	retraso := 30 * time.Second * time.Duration(1<<uint(intentos-1))
	if maximo := 6 * time.Hour; retraso > maximo {
		retraso = maximo
	}
	return retraso
}

func (s *Scheduler) avisar(ctx context.Context, mensaje string, pares ...slog.Attr) {
	if s == nil || s.opciones.Logger == nil {
		return
	}
	argumentos := make([]any, 0, len(pares))
	for _, par := range pares {
		argumentos = append(argumentos, par)
	}
	s.opciones.Logger.WarnContext(ctx, mensaje, argumentos...)
}

// solicitudRecordatorio construye el trabajo del outbox.
//
// `OcurridoEn` es la propia ocurrencia (no `ahora`): la clave del outbox
// (`DUE#<ocurridoEn>#<notificationID>`) y el `notificationID` derivado de
// (userID, eventID) quedan fijos, de modo que reencolar la misma ocurrencia
// reutiliza el mismo trabajo.
func solicitudRecordatorio(oc Ocurrencia) pushoutbox.Solicitud {
	titulo := strings.TrimSpace(oc.Titulo)
	return pushoutbox.Solicitud{
		UserID:     strings.TrimSpace(oc.UserID),
		Title:      titulo,
		Message:    "Recordatorio: " + titulo,
		Type:       TipoNotificacionRecordatorio,
		EventID:    EventIDRecordatorio(oc.ReminderID, oc.DueAt),
		OcurridoEn: oc.DueAt.UTC(),
	}
}

// EventIDRecordatorio devuelve el identificador determinista del evento de una
// ocurrencia: `reminder:<reminderID>:<dueAt RFC3339Nano>`. No contiene datos
// personales más allá del identificador opaco del recordatorio.
func EventIDRecordatorio(reminderID string, dueAt time.Time) string {
	return marcaEventoRecordatorio + strings.TrimSpace(reminderID) + ":" + dueAt.UTC().Format(time.RFC3339Nano)
}

// ErrorDeProductor describe las clases de error que se persisten como
// `lastErrorClass`. Nunca se guarda el mensaje del error: podría contener datos.
const (
	claseProductorGenerica = "PRODUCTOR_ERROR"
	claseProductorLease    = "LEASE_PERDIDO"
)

func claseDeError(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrLeasePerdido):
		return claseProductorLease
	case errors.Is(err, ErrOcurrenciaAusente):
		return "OCURRENCIA_AUSENTE"
	case errors.Is(err, ErrOcurrenciaInvalida):
		return "OCURRENCIA_INVALIDA"
	case errors.Is(err, ErrZonaInvalida):
		return "ZONA_INVALIDA"
	case errors.Is(err, ErrFrecuenciaInvalida):
		return "FRECUENCIA_INVALIDA"
	case errors.Is(err, pushoutbox.ErrCuentaNoActiva):
		return "CUENTA_NO_ACTIVA"
	default:
		return claseProductorGenerica
	}
}

// NextOccurrence calcula la siguiente ocurrencia de un recordatorio recurrente.
//
// Reglas, explícitas porque el calendario no es uniforme:
//
//   - La aritmética se hace en la hora local de `timezone` (IANA) y el resultado
//     se devuelve en UTC. `daily` y `weekly` conservan la hora de pared: un
//     recordatorio de las 09:00 locales sigue a las 09:00 locales tras un cambio
//     de horario de verano (el instante UTC varía una hora).
//   - `monthly` avanza al mismo día del mes siguiente. Si el día no existe, se
//     ancla al ÚLTIMO día de ese mes y NO se recupera el día original después:
//     31 ene → 28 feb (2027) / 29 feb (2028) → 28 mar → 28 abr.
//   - Una hora local inexistente (salto de primavera) se interpreta con el
//     desfase posterior al cambio, de modo que el instante resultante cae una
//     hora ANTES en hora local (02:30 → 01:30 del mismo día) y el intervalo dura
//     23 h. Es el comportamiento de `time.Date`/`AddDate` y está cubierto por
//     prueba; no se mueve ni se pierde la ocurrencia. Una hora local duplicada
//     (retroceso de otoño) usa la primera de las dos.
//   - Una zona IANA no cargable o una frecuencia desconocida devuelven error; el
//     scheduler retira la ocurrencia en lugar de iterar para siempre.
func NextOccurrence(dueAt time.Time, timezone string, frequency Frequency) (time.Time, error) {
	zona, err := time.LoadLocation(strings.TrimSpace(timezone))
	if err != nil || zona == nil {
		return time.Time{}, ErrZonaInvalida
	}
	local := dueAt.In(zona)
	switch frequency {
	case FrequencyDaily:
		return local.AddDate(0, 0, 1).UTC(), nil
	case FrequencyWeekly:
		return local.AddDate(0, 0, 7).UTC(), nil
	case FrequencyMonthly:
		primero := time.Date(local.Year(), local.Month()+1, 1, local.Hour(), local.Minute(), local.Second(), 0, zona)
		ultimoDia := primero.AddDate(0, 1, -1).Day()
		dia := local.Day()
		if dia > ultimoDia {
			dia = ultimoDia
		}
		return time.Date(primero.Year(), primero.Month(), dia, local.Hour(), local.Minute(), local.Second(), 0, zona).UTC(), nil
	default:
		return time.Time{}, ErrFrecuenciaInvalida
	}
}
