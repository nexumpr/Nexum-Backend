package reminders

// Pruebas del scheduler durable con dobles estrictos y reloj inyectado.
//
// Los dobles NO son cajas negras: replican las condiciones del adaptador real
// (lease por owner/fence/version, versión del recordatorio, transición de SK al
// avanzar) y fallan de forma explícita cuando el scheduler intenta algo que el
// diseño prohíbe (por ejemplo, un `Scan`, que ni siquiera existe en el puerto).

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nexumpr/Nexum-Backend/internal/application/pushoutbox"
)

// ---------------------------------------------------------------------------
// Estado compartido por los nodos falsos
// ---------------------------------------------------------------------------

type estadoAgenda struct {
	mu            sync.Mutex
	items         map[string]*Ocurrencia
	recordatorio  map[string]bool // reminderID -> activo
	punteros      map[string]string
	reclamos      int
	completadas   []string
	avanzadas     []string
	liberadas     []string
	motivos       []string
	llamadasScan  int
	falloDue      error
	falloClaim    error
	falloAdvance  error
	falloComplete error
	falloRelease  error
}

func nuevoEstadoAgenda() *estadoAgenda {
	return &estadoAgenda{
		items:        map[string]*Ocurrencia{},
		recordatorio: map[string]bool{},
		punteros:     map[string]string{},
	}
}

func (e *estadoAgenda) agregar(oc Ocurrencia) {
	e.items[oc.SK] = &oc
	e.recordatorio[oc.ReminderID] = true
	e.punteros[oc.ReminderID] = oc.SK
}

func (e *estadoAgenda) activo(reminderID string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.recordatorio[reminderID]
}

func (e *estadoAgenda) existe(sk string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	_, ok := e.items[sk]
	return ok
}

func (e *estadoAgenda) puntero(reminderID string) (string, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	sk, ok := e.punteros[reminderID]
	return sk, ok
}

func (e *estadoAgenda) instantanea(sk string) (Ocurrencia, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	oc, ok := e.items[sk]
	if !ok {
		return Ocurrencia{}, false
	}
	return *oc, true
}

// agendaFalsa es un nodo: el estado es compartido y el `owner` es propio, de modo
// que dos instancias compiten de verdad por el mismo trabajo.
type agendaFalsa struct {
	estado *estadoAgenda
	owner  string
}

func (f *agendaFalsa) Due(_ context.Context, ahora time.Time, limite int) ([]Ocurrencia, error) {
	f.estado.mu.Lock()
	defer f.estado.mu.Unlock()
	if f.estado.falloDue != nil {
		return nil, f.estado.falloDue
	}
	salida := make([]Ocurrencia, 0, limite)
	for _, oc := range f.estado.items {
		if !oc.DueAt.After(ahora.UTC()) && oc.Disponible(ahora) {
			salida = append(salida, *oc)
			if len(salida) >= limite {
				break
			}
		}
	}
	return salida, nil
}

func (f *agendaFalsa) Claim(_ context.Context, oc Ocurrencia, ahora time.Time, lease time.Duration) (*Lease, error) {
	f.estado.mu.Lock()
	defer f.estado.mu.Unlock()
	if f.estado.falloClaim != nil {
		return nil, f.estado.falloClaim
	}
	actual, ok := f.estado.items[oc.SK]
	if !ok {
		return nil, ErrOcurrenciaAusente
	}
	if actual.Version != oc.Version {
		return nil, ErrLeasePerdido
	}
	if !actual.Disponible(ahora) {
		return nil, ErrLeasePerdido
	}
	f.estado.reclamos++
	reclamada := *actual
	reclamada.Owner = f.owner
	reclamada.LeaseUntil = ahora.UTC().Add(lease)
	reclamada.Version++
	reclamada.Fence++
	reclamada.Attempts++
	f.estado.items[oc.SK] = &reclamada
	return &Lease{
		Ocurrencia: reclamada,
		Owner:      f.owner,
		Fence:      reclamada.Fence,
		Version:    reclamada.Version,
		Attempts:   reclamada.Attempts,
		LeaseUntil: reclamada.LeaseUntil,
	}, nil
}

// comprobarLease replica la condición atómica del adaptador.
func (f *agendaFalsa) comprobarLease(lease Lease, ahora time.Time) (*Ocurrencia, error) {
	actual, ok := f.estado.items[lease.Ocurrencia.SK]
	if !ok {
		return nil, ErrOcurrenciaAusente
	}
	if actual.Owner != lease.Owner || actual.Fence != lease.Fence || actual.Version != lease.Version {
		return nil, ErrLeasePerdido
	}
	if !actual.LeaseUntil.After(ahora.UTC()) {
		return nil, ErrLeasePerdido
	}
	return actual, nil
}

func (f *agendaFalsa) Advance(_ context.Context, lease Lease, siguiente time.Time, ahora time.Time) error {
	f.estado.mu.Lock()
	defer f.estado.mu.Unlock()
	if f.estado.falloAdvance != nil {
		return f.estado.falloAdvance
	}
	actual, err := f.comprobarLease(lease, ahora)
	if err != nil {
		return err
	}
	if !f.estado.recordatorio[actual.ReminderID] {
		return ErrLeasePerdido
	}
	delete(f.estado.items, actual.SK)
	nueva := *actual
	nueva.SK = fmt.Sprintf("DUE#%s#%s", siguiente.UTC().Format(time.RFC3339Nano), actual.ReminderID)
	nueva.DueAt = siguiente.UTC()
	nueva.Owner = ""
	nueva.LeaseUntil = time.Time{}
	nueva.Version = 1
	nueva.Fence = 0
	nueva.Attempts = 0
	nueva.ReminderVersion++
	f.estado.items[nueva.SK] = &nueva
	f.estado.punteros[actual.ReminderID] = nueva.SK
	f.estado.avanzadas = append(f.estado.avanzadas, actual.ReminderID)
	return nil
}

func (f *agendaFalsa) Complete(_ context.Context, lease Lease, ahora time.Time) error {
	f.estado.mu.Lock()
	defer f.estado.mu.Unlock()
	if f.estado.falloComplete != nil {
		return f.estado.falloComplete
	}
	actual, err := f.comprobarLease(lease, ahora)
	if err != nil {
		return err
	}
	delete(f.estado.items, actual.SK)
	delete(f.estado.punteros, actual.ReminderID)
	f.estado.recordatorio[actual.ReminderID] = false
	f.estado.completadas = append(f.estado.completadas, actual.ReminderID)
	return nil
}

func (f *agendaFalsa) Release(_ context.Context, lease Lease, motivo string, siguiente time.Time, ahora time.Time) error {
	f.estado.mu.Lock()
	defer f.estado.mu.Unlock()
	if f.estado.falloRelease != nil {
		return f.estado.falloRelease
	}
	actual, err := f.comprobarLease(lease, ahora)
	if err != nil {
		return err
	}
	actual.Owner = ""
	actual.LeaseUntil = siguiente.UTC()
	actual.Version++
	f.estado.items[actual.SK] = actual
	f.estado.liberadas = append(f.estado.liberadas, actual.ReminderID)
	f.estado.motivos = append(f.estado.motivos, motivo)
	return nil
}

// Scan es una trampa: no existe en reminders.ScheduleStore, así que el scheduler
// no puede invocarlo. Si alguien lo añadiera al puerto, esta prueba lo detecta.
func (f *agendaFalsa) Scan(context.Context) error {
	f.estado.mu.Lock()
	defer f.estado.mu.Unlock()
	f.estado.llamadasScan++
	return errors.New("Scan prohibido en el scheduler")
}

// ---------------------------------------------------------------------------
// Productor falso (outbox push)
// ---------------------------------------------------------------------------

type productorFalso struct {
	mu          sync.Mutex
	porEvento   map[string]string
	solicitudes []pushoutbox.Solicitud
	errores     map[string]error
	fallos      int
}

func nuevoProductorFalso() *productorFalso {
	return &productorFalso{porEvento: map[string]string{}, errores: map[string]error{}}
}

func (p *productorFalso) Encolar(_ context.Context, solicitud pushoutbox.Solicitud) (string, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.solicitudes = append(p.solicitudes, solicitud)
	if err, ok := p.errores[solicitud.EventID]; ok && err != nil {
		p.fallos++
		return "", false, err
	}
	if id, ok := p.porEvento[solicitud.EventID]; ok {
		return id, false, nil
	}
	id := fmt.Sprintf("notif-%d", len(p.porEvento)+1)
	p.porEvento[solicitud.EventID] = id
	return id, true, nil
}

func (p *productorFalso) fallar(eventID string, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.errores[eventID] = err
}

func (p *productorFalso) dejarDeFallar(eventID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.errores, eventID)
}

func (p *productorFalso) total() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.solicitudes)
}

func (p *productorFalso) entregas() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.porEvento)
}

func (p *productorFalso) ultima() pushoutbox.Solicitud {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.solicitudes) == 0 {
		return pushoutbox.Solicitud{}
	}
	return p.solicitudes[len(p.solicitudes)-1]
}

// ---------------------------------------------------------------------------
// Utilidades de prueba
// ---------------------------------------------------------------------------

const (
	schedUsuario  = "usuario-recordatorio"
	schedReminder = "11111111-2222-4333-8444-555555555555"
)

var schedAhora = time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)

func schedSK(dueAt time.Time, id string) string {
	return fmt.Sprintf("DUE#%s#%s", dueAt.UTC().Format("2006-01-02T15:04:05.000000000Z"), id)
}

func schedOcurrencia(dueAt time.Time, frecuencia *Frequency) Ocurrencia {
	return Ocurrencia{
		SK:              schedSK(dueAt, schedReminder),
		UserID:          schedUsuario,
		ReminderID:      schedReminder,
		Titulo:          "Pagar la luz",
		DueAt:           dueAt.UTC(),
		Timezone:        "America/Mexico_City",
		Frequency:       frecuencia,
		ReminderVersion: 1,
		Version:         1,
	}
}

func schedReloj(instante time.Time) func() time.Time {
	return func() time.Time { return instante }
}

func schedOpciones(ahora time.Time) Opciones {
	return Opciones{Intervalo: time.Minute, Lease: 30 * time.Second, Lote: 10, Ahora: schedReloj(ahora)}
}

// ---------------------------------------------------------------------------
// 1. Vencido: reclamar, encolar una vez y cerrar
// ---------------------------------------------------------------------------

func TestSchedulerReclamaEncolaYCompletaUnico(t *testing.T) {
	estado := nuevoEstadoAgenda()
	estado.agregar(schedOcurrencia(schedAhora.Add(-5*time.Minute), nil))
	nodo := &agendaFalsa{estado: estado, owner: "nodo-a"}
	productor := nuevoProductorFalso()
	scheduler := NewScheduler(nodo, productor, schedOpciones(schedAhora))

	resultado := scheduler.Ciclo(context.Background())

	if resultado.Reclamadas != 1 || resultado.Encoladas != 1 || resultado.Completadas != 1 {
		t.Fatalf("resultado inesperado: %+v", resultado)
	}
	if resultado.Avanzadas != 0 || resultado.Liberadas != 0 {
		t.Fatalf("un disparo único no avanza ni se libera: %+v", resultado)
	}
	if total := productor.total(); total != 1 {
		t.Fatalf("se esperaba 1 encolado, hubo %d", total)
	}
	solicitud := productor.ultima()
	if solicitud.UserID != schedUsuario || solicitud.Type != TipoNotificacionRecordatorio {
		t.Fatalf("solicitud inesperada: %+v", solicitud)
	}
	if solicitud.Title != "Pagar la luz" || !strings.Contains(solicitud.Message, "Pagar la luz") {
		t.Fatalf("título/mensaje inesperados: %+v", solicitud)
	}
	if !solicitud.OcurridoEn.Equal(schedAhora.Add(-5 * time.Minute)) {
		t.Fatalf("OcurridoEn debe ser la ocurrencia, no el reloj: %v", solicitud.OcurridoEn)
	}
	if estado.activo(schedReminder) {
		t.Fatal("un disparo único completado debe quedar inactivo")
	}
	if estado.existe(schedOcurrencia(schedAhora.Add(-5*time.Minute), nil).SK) {
		t.Fatal("la ocurrencia completada no debe permanecer en la agenda")
	}
	if _, ok := estado.puntero(schedReminder); ok {
		t.Fatal("el puntero de agenda debe retirarse al completar")
	}
	if estado.llamadasScan != 0 {
		t.Fatalf("el scheduler no puede llamar a Scan (%d)", estado.llamadasScan)
	}
}

// ---------------------------------------------------------------------------
// 2. Dos workers: sólo uno reclama
// ---------------------------------------------------------------------------

func TestSchedulerDosNodosSoloUnoReclama(t *testing.T) {
	estado := nuevoEstadoAgenda()
	estado.agregar(schedOcurrencia(schedAhora.Add(-time.Minute), nil))
	nodoA := &agendaFalsa{estado: estado, owner: "nodo-a"}
	nodoB := &agendaFalsa{estado: estado, owner: "nodo-b"}
	productor := nuevoProductorFalso()

	// Ambos leen la misma ocurrencia ANTES de reclamar: es la carrera real.
	leidasA, errA := nodoA.Due(context.Background(), schedAhora, 10)
	leidasB, errB := nodoB.Due(context.Background(), schedAhora, 10)
	if errA != nil || errB != nil || len(leidasA) != 1 || len(leidasB) != 1 {
		t.Fatalf("lectura inicial: %v %v %d %d", errA, errB, len(leidasA), len(leidasB))
	}

	schedulerA := NewScheduler(nodoA, productor, schedOpciones(schedAhora))
	resultadoA := schedulerA.Ciclo(context.Background())
	if resultadoA.Reclamadas != 1 || resultadoA.Encoladas != 1 || resultadoA.Completadas != 1 {
		t.Fatalf("el nodo A debía cerrar el trabajo: %+v", resultadoA)
	}

	// El nodo B corre con la lectura obsoleta: no puede reclamar y no duplica.
	if _, err := nodoB.Claim(context.Background(), leidasB[0], schedAhora, 30*time.Second); !errors.Is(err, ErrLeasePerdido) && !errors.Is(err, ErrOcurrenciaAusente) {
		t.Fatalf("el segundo nodo no debe reclamar: %v", err)
	}
	resultadoB := schedulerB(nodoB, productor).Ciclo(context.Background())
	if resultadoB.Reclamadas != 0 || resultadoB.Encoladas != 0 {
		t.Fatalf("el nodo B no debe reclamar nada: %+v", resultadoB)
	}
	if entregas := productor.entregas(); entregas != 1 {
		t.Fatalf("el mensaje se duplicó: %d entregas", entregas)
	}
}

func schedulerB(store ScheduleStore, productor Producer) *Scheduler {
	return NewScheduler(store, productor, Opciones{Intervalo: time.Minute, Lease: 30 * time.Second, Lote: 10, Ahora: schedReloj(schedAhora)})
}

// ---------------------------------------------------------------------------
// 3. Recurrente: siguiente ocurrencia, DST y fin de mes
// ---------------------------------------------------------------------------

func TestSchedulerRecurrenteAvanzaALaSiguienteOcurrencia(t *testing.T) {
	estado := nuevoEstadoAgenda()
	frecuencia := FrequencyDaily
	dueAt := schedAhora.Add(-time.Minute)
	estado.agregar(schedOcurrencia(dueAt, &frecuencia))
	nodo := &agendaFalsa{estado: estado, owner: "nodo-a"}
	productor := nuevoProductorFalso()
	scheduler := NewScheduler(nodo, productor, schedOpciones(schedAhora))

	resultado := scheduler.Ciclo(context.Background())
	if resultado.Avanzadas != 1 || resultado.Completadas != 0 || resultado.Encoladas != 1 {
		t.Fatalf("resultado inesperado: %+v", resultado)
	}
	if !estado.activo(schedReminder) {
		t.Fatal("un recurrente no debe quedar inactivo")
	}
	esperada, err := NextOccurrence(dueAt, "America/Mexico_City", FrequencyDaily)
	if err != nil {
		t.Fatalf("NextOccurrence: %v", err)
	}
	skNueva, ok := estado.puntero(schedReminder)
	if !ok {
		t.Fatal("el puntero debe apuntar a la nueva ocurrencia")
	}
	oc, existe := estado.instantanea(skNueva)
	if !existe || !oc.DueAt.Equal(esperada) {
		t.Fatalf("ocurrencia avanzada inesperada: %+v (esperada %v)", oc, esperada)
	}
	if oc.Owner != "" || oc.Version != 1 || oc.Fence != 0 || oc.Attempts != 0 {
		t.Fatalf("la nueva ocurrencia debe nacer libre: %+v", oc)
	}
	if oc.ReminderVersion != 2 {
		t.Fatalf("la versión del recordatorio debe avanzar: %d", oc.ReminderVersion)
	}
	// La ocurrencia antigua desaparece: no hay dos disparos para el mismo recordatorio.
	if estado.existe(schedSK(dueAt, schedReminder)) {
		t.Fatal("la ocurrencia anterior debe retirarse")
	}
}

func TestNextOccurrenceDiariaConservaHoraLocalEnCambioDeHorario(t *testing.T) {
	zona, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("zona IANA no disponible: %v", err)
	}
	// 2027-03-14 es el segundo domingo de marzo: en EE. UU. se adelanta la hora.
	antes := time.Date(2027, 3, 13, 9, 0, 0, 0, zona)
	siguiente, err := NextOccurrence(antes, "America/New_York", FrequencyDaily)
	if err != nil {
		t.Fatalf("NextOccurrence: %v", err)
	}
	local := siguiente.In(zona)
	if local.Hour() != 9 || local.Minute() != 0 || local.Day() != 14 {
		t.Fatalf("la hora local debe conservarse: %v", local)
	}
	// El día del cambio dura 23 h: 09:00 EST (-5) -> 09:00 EDT (-4).
	if delta := siguiente.Sub(antes); delta != 23*time.Hour {
		t.Fatalf("el salto de primavera debe durar 23 h, duró %v", delta)
	}

	// Hora local inexistente (02:30 del día del cambio): se interpreta con el
	// desfase posterior y cae una hora antes en hora local. Es el comportamiento
	// real de time.Date/AddDate y queda documentado en NextOccurrence.
	inexistente := time.Date(2027, 3, 13, 2, 30, 0, 0, zona)
	trasElSalto, err := NextOccurrence(inexistente, "America/New_York", FrequencyDaily)
	if err != nil {
		t.Fatalf("NextOccurrence en el salto: %v", err)
	}
	localTrasElSalto := trasElSalto.In(zona)
	if localTrasElSalto.Hour() != 1 || localTrasElSalto.Minute() != 30 || localTrasElSalto.Day() != 14 {
		t.Fatalf("la hora inexistente debe resolverse una hora antes: %v", localTrasElSalto)
	}
	if delta := trasElSalto.Sub(inexistente); delta != 23*time.Hour {
		t.Fatalf("el intervalo del salto debe durar 23 h, duró %v", delta)
	}

	// Hora local duplicada (retroceso de otoño): usa la primera de las dos (EDT).
	previaAlRetroceso := time.Date(2027, 11, 6, 1, 30, 0, 0, zona)
	trasElRetroceso, err := NextOccurrence(previaAlRetroceso, "America/New_York", FrequencyDaily)
	if err != nil {
		t.Fatalf("NextOccurrence en el retroceso: %v", err)
	}
	if nombre, _ := trasElRetroceso.In(zona).Zone(); nombre != "EDT" {
		t.Fatalf("la hora duplicada debe usar la primera ocurrencia: %s (%v)", nombre, trasElRetroceso.In(zona))
	}
	if delta := trasElRetroceso.Sub(previaAlRetroceso); delta != 24*time.Hour {
		t.Fatalf("el intervalo del retroceso debe durar 24 h, duró %v", delta)
	}
}

func TestNextOccurrenceMensualFinDeMesYAnio(t *testing.T) {
	casos := []struct {
		nombre   string
		entrada  time.Time
		esperada time.Time
	}{
		{
			nombre:   "31 de enero -> 28 de febrero (2027 no bisiesto)",
			entrada:  time.Date(2027, 1, 31, 9, 0, 0, 0, time.UTC),
			esperada: time.Date(2027, 2, 28, 9, 0, 0, 0, time.UTC),
		},
		{
			nombre:   "31 de enero -> 29 de febrero (2028 bisiesto)",
			entrada:  time.Date(2028, 1, 31, 9, 0, 0, 0, time.UTC),
			esperada: time.Date(2028, 2, 29, 9, 0, 0, 0, time.UTC),
		},
		{
			nombre:   "28 de febrero no recupera el 31",
			entrada:  time.Date(2027, 2, 28, 9, 0, 0, 0, time.UTC),
			esperada: time.Date(2027, 3, 28, 9, 0, 0, 0, time.UTC),
		},
		{
			nombre:   "31 de diciembre -> 31 de enero",
			entrada:  time.Date(2027, 12, 31, 9, 0, 0, 0, time.UTC),
			esperada: time.Date(2028, 1, 31, 9, 0, 0, 0, time.UTC),
		},
		{
			nombre:   "31 de marzo -> 30 de abril",
			entrada:  time.Date(2027, 3, 31, 9, 0, 0, 0, time.UTC),
			esperada: time.Date(2027, 4, 30, 9, 0, 0, 0, time.UTC),
		},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			obtenida, err := NextOccurrence(caso.entrada, "UTC", FrequencyMonthly)
			if err != nil {
				t.Fatalf("NextOccurrence: %v", err)
			}
			if !obtenida.Equal(caso.esperada) {
				t.Fatalf("se esperaba %v, se obtuvo %v", caso.esperada, obtenida)
			}
		})
	}

	if _, err := NextOccurrence(schedAhora, "Marte/Olympus", FrequencyDaily); !errors.Is(err, ErrZonaInvalida) {
		t.Fatalf("una zona inválida debe fallar: %v", err)
	}
	if _, err := NextOccurrence(schedAhora, "UTC", Frequency("hourly")); !errors.Is(err, ErrFrecuenciaInvalida) {
		t.Fatalf("una frecuencia desconocida debe fallar: %v", err)
	}
}

func TestSchedulerRecurrenciaInvalidaNoBucleInfinito(t *testing.T) {
	estado := nuevoEstadoAgenda()
	frecuencia := FrequencyMonthly
	oc := schedOcurrencia(schedAhora.Add(-time.Minute), &frecuencia)
	oc.Timezone = "Zona/Inventada" // dato corrupto: no puede avanzar nunca
	estado.agregar(oc)
	nodo := &agendaFalsa{estado: estado, owner: "nodo-a"}
	productor := nuevoProductorFalso()
	scheduler := NewScheduler(nodo, productor, schedOpciones(schedAhora))

	resultado := scheduler.Ciclo(context.Background())
	if resultado.Completadas != 1 || resultado.Avanzadas != 0 {
		t.Fatalf("una recurrencia inválida se retira, no se reintenta: %+v", resultado)
	}
	if estado.existe(oc.SK) {
		t.Fatal("la ocurrencia inválida no debe permanecer")
	}
}

// ---------------------------------------------------------------------------
// 4. Único: inactivo y sin agenda ni puntero (cubierto arriba) + idempotencia
// ---------------------------------------------------------------------------

func TestSchedulerSegundoCicloSinTrabajo(t *testing.T) {
	estado := nuevoEstadoAgenda()
	estado.agregar(schedOcurrencia(schedAhora.Add(-time.Minute), nil))
	nodo := &agendaFalsa{estado: estado, owner: "nodo-a"}
	productor := nuevoProductorFalso()
	scheduler := NewScheduler(nodo, productor, schedOpciones(schedAhora))

	if r := scheduler.Ciclo(context.Background()); r.Completadas != 1 {
		t.Fatalf("primer ciclo: %+v", r)
	}
	if r := scheduler.Ciclo(context.Background()); r.Reclamadas != 0 || r.Encoladas != 0 {
		t.Fatalf("segundo ciclo no debe tener trabajo: %+v", r)
	}
	if productor.total() != 1 {
		t.Fatalf("no debe reencolarse: %d", productor.total())
	}
}

// ---------------------------------------------------------------------------
// 5. Cuenta no activa: no encola y completa sin reintento
// ---------------------------------------------------------------------------

func TestSchedulerCuentaNoActivaCompletaSinEnviar(t *testing.T) {
	estado := nuevoEstadoAgenda()
	frecuencia := FrequencyWeekly
	dueAt := schedAhora.Add(-time.Minute)
	estado.agregar(schedOcurrencia(dueAt, &frecuencia))
	nodo := &agendaFalsa{estado: estado, owner: "nodo-a"}
	productor := nuevoProductorFalso()
	evento := EventIDRecordatorio(schedReminder, dueAt)
	productor.fallar(evento, pushoutbox.ErrCuentaNoActiva)
	scheduler := NewScheduler(nodo, productor, schedOpciones(schedAhora))

	resultado := scheduler.Ciclo(context.Background())
	if resultado.CuentaInactiva != 1 || resultado.Completadas != 1 {
		t.Fatalf("una cuenta no activa se completa sin enviar: %+v", resultado)
	}
	if resultado.Liberadas != 0 || resultado.Avanzadas != 0 {
		t.Fatalf("no debe liberarse ni avanzar: %+v", resultado)
	}
	if productor.entregas() != 0 {
		t.Fatal("no debe existir entrega para una cuenta no activa")
	}
	if estado.activo(schedReminder) {
		t.Fatal("el recordatorio debe quedar inactivo")
	}
	if estado.existe(schedSK(dueAt, schedReminder)) {
		t.Fatal("la ocurrencia debe retirarse")
	}
	// Un segundo ciclo no reintenta: no hay bucle infinito.
	if r := scheduler.Ciclo(context.Background()); r.Reclamadas != 0 {
		t.Fatalf("no debe reintentarse: %+v", r)
	}
	if productor.total() != 1 {
		t.Fatalf("sólo se intentó una vez: %d", productor.total())
	}
}

// ---------------------------------------------------------------------------
// 6. Error transitorio: Release, expira el lease y otro ciclo reintenta
// ---------------------------------------------------------------------------

func TestSchedulerErrorTransitorioLiberaYReintentaTrasElLease(t *testing.T) {
	estado := nuevoEstadoAgenda()
	dueAt := schedAhora.Add(-time.Minute)
	estado.agregar(schedOcurrencia(dueAt, nil))
	nodo := &agendaFalsa{estado: estado, owner: "nodo-a"}
	productor := nuevoProductorFalso()
	evento := EventIDRecordatorio(schedReminder, dueAt)
	productor.fallar(evento, errors.New("outbox no disponible"))

	instante := schedAhora
	scheduler := NewScheduler(nodo, productor, Opciones{
		Intervalo: time.Minute, Lease: 30 * time.Second, Lote: 10,
		Ahora: func() time.Time { return instante },
	})

	resultado := scheduler.Ciclo(context.Background())
	if resultado.Liberadas != 1 || resultado.Encoladas != 0 {
		t.Fatalf("un fallo transitorio libera: %+v", resultado)
	}
	if len(estado.liberadas) != 1 || estado.motivos[0] != "PRODUCTOR_ERROR" {
		t.Fatalf("clase de error inesperada: %v %v", estado.liberadas, estado.motivos)
	}
	oc, ok := estado.instantanea(schedSK(dueAt, schedReminder))
	if !ok || oc.Owner != "" {
		t.Fatalf("el lease debe liberarse: %+v", oc)
	}
	if !oc.LeaseUntil.After(instante) {
		t.Fatalf("el reintento debe tener backoff: %v", oc.LeaseUntil)
	}

	// Mismo instante: el backoff impide reclamarla.
	if r := scheduler.Ciclo(context.Background()); r.Reclamadas != 0 {
		t.Fatalf("el backoff debe evitar el reintento inmediato: %+v", r)
	}

	// El producto se recupera y el reloj avanza más allá del backoff.
	productor.dejarDeFallar(evento)
	instante = instante.Add(31 * time.Second)
	resultado = scheduler.Ciclo(context.Background())
	if resultado.Reclamadas != 1 || resultado.Encoladas != 1 || resultado.Completadas != 1 {
		t.Fatalf("tras el backoff debe completarse: %+v", resultado)
	}
}

// ---------------------------------------------------------------------------
// 7. Reinicio: lease vencido de un nodo que ya no está
// ---------------------------------------------------------------------------

func TestSchedulerRecuperaLeaseVencidoTrasReinicio(t *testing.T) {
	estado := nuevoEstadoAgenda()
	dueAt := schedAhora.Add(-10 * time.Minute)
	oc := schedOcurrencia(dueAt, nil)
	oc.Owner = "nodo-que-murio"
	oc.Fence = 3
	oc.Version = 4
	oc.Attempts = 2
	oc.LeaseUntil = schedAhora.Add(-2 * time.Minute)
	estado.agregar(oc)

	nodo := &agendaFalsa{estado: estado, owner: "nodo-nuevo"}
	productor := nuevoProductorFalso()
	scheduler := NewScheduler(nodo, productor, schedOpciones(schedAhora))

	resultado := scheduler.Ciclo(context.Background())
	if resultado.Reclamadas != 1 || resultado.Completadas != 1 {
		t.Fatalf("el trabajo huérfano debe recuperarse: %+v", resultado)
	}
	if productor.total() != 1 {
		t.Fatalf("debe encolarse una vez: %d", productor.total())
	}
}

func TestSchedulerIgnoraLeaseVigenteDeOtroNodo(t *testing.T) {
	estado := nuevoEstadoAgenda()
	oc := schedOcurrencia(schedAhora.Add(-time.Minute), nil)
	oc.Owner = "nodo-vivo"
	oc.Fence = 1
	oc.Version = 2
	oc.LeaseUntil = schedAhora.Add(20 * time.Second)
	estado.agregar(oc)

	nodo := &agendaFalsa{estado: estado, owner: "nodo-b"}
	productor := nuevoProductorFalso()
	scheduler := NewScheduler(nodo, productor, schedOpciones(schedAhora))

	if r := scheduler.Ciclo(context.Background()); r.Reclamadas != 0 || r.Encoladas != 0 {
		t.Fatalf("no debe reclamar un lease vigente ajeno: %+v", r)
	}
	if productor.total() != 0 {
		t.Fatal("no debe encolarse nada")
	}
}

// ---------------------------------------------------------------------------
// 8. Sin Scan
// ---------------------------------------------------------------------------

func TestSchedulerNoExponeScan(t *testing.T) {
	tipo := reflect.TypeOf((*ScheduleStore)(nil)).Elem()
	for i := 0; i < tipo.NumMethod(); i++ {
		if strings.EqualFold(tipo.Method(i).Name, "Scan") {
			t.Fatalf("ScheduleStore no puede exponer Scan: %s", tipo.Method(i).Name)
		}
	}
	esperados := map[string]bool{"Due": true, "Claim": true, "Advance": true, "Complete": true, "Release": true}
	if tipo.NumMethod() != len(esperados) {
		t.Fatalf("métodos inesperados en ScheduleStore: %d", tipo.NumMethod())
	}
	for i := 0; i < tipo.NumMethod(); i++ {
		if !esperados[tipo.Method(i).Name] {
			t.Fatalf("método no autorizado en ScheduleStore: %s", tipo.Method(i).Name)
		}
	}

	// El doble registra cualquier intento y debe quedar en cero.
	estado := nuevoEstadoAgenda()
	estado.agregar(schedOcurrencia(schedAhora.Add(-time.Minute), nil))
	nodo := &agendaFalsa{estado: estado, owner: "nodo-a"}
	scheduler := NewScheduler(nodo, nuevoProductorFalso(), schedOpciones(schedAhora))
	scheduler.Ciclo(context.Background())
	if estado.llamadasScan != 0 {
		t.Fatalf("Scan se invocó %d veces", estado.llamadasScan)
	}
}

// ---------------------------------------------------------------------------
// 9. EventID determinista y deduplicación
// ---------------------------------------------------------------------------

func TestEventIDRecordatorioEsDeterminista(t *testing.T) {
	dueAt := time.Date(2026, 11, 24, 9, 30, 15, 250000000, time.UTC)
	esperado := "reminder:" + schedReminder + ":2026-11-24T09:30:15.25Z"
	if obtenido := EventIDRecordatorio(schedReminder, dueAt); obtenido != esperado {
		t.Fatalf("eventID=%q, esperado %q", obtenido, esperado)
	}
	// Mismo instante en otra zona: mismo identificador.
	zona, err := time.LoadLocation("America/Mexico_City")
	if err != nil {
		t.Fatalf("zona: %v", err)
	}
	if obtenido := EventIDRecordatorio(schedReminder, dueAt.In(zona)); obtenido != esperado {
		t.Fatalf("el eventID no debe depender de la zona: %q", obtenido)
	}
	if EventIDRecordatorio("otro", dueAt) == esperado {
		t.Fatal("recordatorios distintos no pueden compartir eventID")
	}
	if EventIDRecordatorio(schedReminder, dueAt.Add(time.Nanosecond)) == esperado {
		t.Fatal("ocurrencias distintas no pueden compartir eventID")
	}
}

func TestSchedulerReplayNoDuplicaElMensaje(t *testing.T) {
	estado := nuevoEstadoAgenda()
	dueAt := schedAhora.Add(-time.Minute)
	estado.agregar(schedOcurrencia(dueAt, nil))
	nodo := &agendaFalsa{estado: estado, owner: "nodo-a"}
	productor := nuevoProductorFalso()
	estado.falloComplete = ErrLeasePerdido // el cierre falla: el trabajo queda vivo

	instante := schedAhora
	scheduler := NewScheduler(nodo, productor, Opciones{
		Intervalo: time.Minute, Lease: 20 * time.Second, Lote: 10,
		Ahora: func() time.Time { return instante },
	})

	if r := scheduler.Ciclo(context.Background()); r.Encoladas != 1 {
		t.Fatalf("primer ciclo: %+v", r)
	}
	// El cierre falló: se libera el lease para que otro ciclo lo reintente.
	estado.mu.Lock()
	estado.falloComplete = nil
	oc := estado.items[schedSK(dueAt, schedReminder)]
	oc.Owner = ""
	oc.LeaseUntil = time.Time{}
	estado.mu.Unlock()

	instante = instante.Add(time.Minute)
	resultado := scheduler.Ciclo(context.Background())
	if resultado.Replays != 1 || resultado.Encoladas != 0 {
		t.Fatalf("el replay no debe contar como encolado nuevo: %+v", resultado)
	}
	if productor.entregas() != 1 || productor.total() != 2 {
		t.Fatalf("debe haber 1 entrega y 2 intentos: %d %d", productor.entregas(), productor.total())
	}
	primera := productor.solicitudes[0]
	segunda := productor.solicitudes[1]
	if primera.EventID != segunda.EventID || primera.Title != segunda.Title {
		t.Fatalf("el replay debe reutilizar la clave exacta: %+v vs %+v", primera, segunda)
	}
	if !primera.OcurridoEn.Equal(segunda.OcurridoEn) {
		t.Fatal("OcurridoEn debe ser determinista")
	}
}

// ---------------------------------------------------------------------------
// Controles adicionales del ciclo
// ---------------------------------------------------------------------------

func TestSchedulerSinDependenciasQuedaInerte(t *testing.T) {
	var sinStore *Scheduler
	if sinStore.Disponible() {
		t.Fatal("un scheduler nulo no está disponible")
	}
	if r := sinStore.Ciclo(context.Background()); r != (ResultadoCiclo{}) {
		t.Fatalf("un scheduler nulo no debe actuar: %+v", r)
	}
	sinProductor := NewScheduler(&agendaFalsa{estado: nuevoEstadoAgenda()}, nil, Opciones{})
	if sinProductor.Disponible() {
		t.Fatal("sin productor no está disponible")
	}
	if r := sinProductor.Ciclo(context.Background()); r != (ResultadoCiclo{}) {
		t.Fatalf("sin productor no debe actuar: %+v", r)
	}
	// Run con contexto ya cancelado termina de inmediato en lugar de bloquear.
	ctx, cancelar := context.WithCancel(context.Background())
	cancelar()
	NewScheduler(&agendaFalsa{estado: nuevoEstadoAgenda()}, nuevoProductorFalso(), Opciones{Intervalo: time.Millisecond}).Run(ctx)
}

func TestSchedulerFalloDeLecturaNoRompeElCiclo(t *testing.T) {
	estado := nuevoEstadoAgenda()
	estado.falloDue = errors.New("dynamodb no disponible")
	scheduler := NewScheduler(&agendaFalsa{estado: estado, owner: "nodo-a"}, nuevoProductorFalso(), schedOpciones(schedAhora))
	resultado := scheduler.Ciclo(context.Background())
	if resultado != (ResultadoCiclo{}) {
		t.Fatalf("un fallo de lectura no produce acciones: %+v", resultado)
	}
}

func TestSchedulerBackoffAcotadoPorMaxIntentos(t *testing.T) {
	scheduler := NewScheduler(&agendaFalsa{estado: nuevoEstadoAgenda()}, nuevoProductorFalso(), Opciones{MaxIntentos: 3})
	if obtenido := scheduler.backoff(1); obtenido != 30*time.Second {
		t.Fatalf("primer backoff: %v", obtenido)
	}
	if obtenido := scheduler.backoff(3); obtenido != 2*time.Minute {
		t.Fatalf("backoff al máximo: %v", obtenido)
	}
	if obtenido := scheduler.backoff(9); obtenido != 2*time.Minute {
		t.Fatalf("no debe crecer tras MaxIntentos: %v", obtenido)
	}
	if obtenido := scheduler.backoff(0); obtenido != 30*time.Second {
		t.Fatalf("intentos inválidos: %v", obtenido)
	}
}

func TestSchedulerClasesDeErrorSinDatosPersonales(t *testing.T) {
	casos := map[error]string{
		ErrLeasePerdido:              "LEASE_PERDIDO",
		ErrOcurrenciaAusente:         "OCURRENCIA_AUSENTE",
		ErrOcurrenciaInvalida:        "OCURRENCIA_INVALIDA",
		ErrZonaInvalida:              "ZONA_INVALIDA",
		ErrFrecuenciaInvalida:        "FRECUENCIA_INVALIDA",
		pushoutbox.ErrCuentaNoActiva: "CUENTA_NO_ACTIVA",
	}
	for err, esperada := range casos {
		if obtenida := claseDeError(err); obtenida != esperada {
			t.Fatalf("clase de %v: %q != %q", err, obtenida, esperada)
		}
	}
	// Un error arbitrario no se persiste tal cual: podría contener datos.
	if obtenida := claseDeError(errors.New("error con correo usuario@example.com")); obtenida != "PRODUCTOR_ERROR" {
		t.Fatalf("un error desconocido no debe filtrarse: %q", obtenida)
	}
}
