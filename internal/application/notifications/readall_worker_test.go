package notifications

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"reflect"
	"sync"
	"testing"
	"time"
)

// relojPrueba es un reloj determinista compartido por el doble y el worker.
type relojPrueba struct {
	mu    sync.Mutex
	ahora time.Time
}

func nuevoReloj() *relojPrueba {
	return &relojPrueba{ahora: time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)}
}

func (r *relojPrueba) Ahora() time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ahora
}

func (r *relojPrueba) Avanzar(d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ahora = r.ahora.Add(d)
}

func loggerSilencioso() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func workerDePrueba(store *fakeStore, reloj *relojPrueba) *ReadAllWorker {
	store.now = reloj.Ahora
	worker := NewReadAllWorker(store, loggerSilencioso())
	worker.WithClock(reloj.Ahora)
	return worker
}

func admitir(t *testing.T, store *fakeStore, userID, requestID string, ahora time.Time) {
	t.Helper()
	servicio := NewService(store, []byte("0123456789abcdef0123456789abcdef"))
	servicio.WithClock(func() time.Time { return ahora })
	servicio.WithIDGenerator(func() (string, error) { return requestID, nil })
	aceptado, err := servicio.StartReadAll(context.Background(), userID)
	if err != nil || aceptado == nil || aceptado.RequestID != requestID {
		t.Fatalf("admisión E063: %+v %v", aceptado, err)
	}
}

func TestE063WorkerMarcaPorLotesAcotadosYCompleta(t *testing.T) {
	store := nuevoFakeStore()
	reloj := nuevoReloj()
	store.sembrar("user-a", 137, 137, TypeSystem)
	store.sembrar("user-b", 5, 5, TypeSystem)
	const requestID = "018f90ef-08e5-7a68-b09f-000000009101"
	admitir(t, store, "user-a", requestID, reloj.Ahora())

	worker := workerDePrueba(store, reloj)
	if completados := worker.Ciclo(context.Background()); completados != 0 {
		t.Fatalf("un ciclo no debe completar 137 pendientes: %d", completados)
	}
	if marcadas := len(store.markCalls); marcadas != MaxPaginasPorCiclo*ReadAllBatchSize {
		t.Fatalf("primer ciclo acotado incorrecto: %d marcas", marcadas)
	}
	if restantes := store.noLeidas("user-a"); restantes != 37 {
		t.Fatalf("tras el primer ciclo quedaban %d no leídas", restantes)
	}
	parcial := store.estado("user-a", requestID)
	if parcial == nil || parcial.Status != StatusPending || parcial.Cursor == nil || parcial.Fence != 1 {
		t.Fatalf("avance persistido incorrecto: %+v", parcial)
	}
	if store.trabajo("user-a", requestID) == nil {
		t.Fatal("el trabajo debe seguir en la cola hasta completarse")
	}

	reloj.Avanzar(10 * time.Second)
	if completados := worker.Ciclo(context.Background()); completados != 1 {
		t.Fatalf("el segundo ciclo debía completar el trabajo: %d", completados)
	}
	final := store.estado("user-a", requestID)
	if final == nil || final.Status != StatusCompleted || final.CompletedAt == nil {
		t.Fatalf("READALL no completado: %+v", final)
	}
	if final.ProcessedCount != 137 {
		t.Fatalf("processedCount=%d", final.ProcessedCount)
	}
	if store.trabajoCount() != 0 {
		t.Fatalf("el ítem de cola no se borró: %d", store.trabajoCount())
	}
	if store.tienePuntero("user-a", requestID) {
		t.Fatal("el puntero READALLPTR no se borró")
	}
	if store.noLeidas("user-a") != 0 {
		t.Fatal("quedaron notificaciones sin marcar")
	}
	if store.noLeidas("user-b") != 5 {
		t.Fatal("el worker tocó al usuario B")
	}
}

func TestE063ReinicioConLeaseVencidoRecupera(t *testing.T) {
	store := nuevoFakeStore()
	reloj := nuevoReloj()
	store.sembrar("user-a", 250, 250, TypeSystem)
	const requestID = "018f90ef-08e5-7a68-b09f-000000009102"
	admitir(t, store, "user-a", requestID, reloj.Ahora())

	workerA := workerDePrueba(store, reloj)
	trabajos, err := workerA.Due(context.Background())
	if err != nil || len(trabajos) != 1 {
		t.Fatalf("Due: %+v %v", trabajos, err)
	}
	reclamadoA, ok, err := workerA.Claim(context.Background(), trabajos[0])
	if err != nil || !ok || reclamadoA.Fence != 1 {
		t.Fatalf("Claim A: %+v %v", reclamadoA, err)
	}
	// Simula caída tras procesar cuatro páginas: no completa ni libera.
	estadoA, err := workerA.store.LoadReadAll(context.Background(), "user-a", requestID)
	if err != nil {
		t.Fatal(err)
	}
	noLeidas := false
	for pagina := 0; pagina < MaxPaginasPorCiclo; pagina++ {
		resultado, err := workerA.store.List(context.Background(), ListQuery{
			UserID: "user-a", Limit: ReadAllBatchSize, IsRead: &noLeidas, After: estadoA.Cursor,
		})
		if err != nil || resultado == nil {
			t.Fatalf("List: %v", err)
		}
		for _, notificacion := range resultado.Items {
			if _, err := workerA.store.MarkRead(context.Background(), "user-a", notificacion.ID, reloj.Ahora()); err != nil {
				t.Fatal(err)
			}
		}
		siguiente := *estadoA
		siguiente.Cursor = resultado.NextKey
		siguiente.LeaseOwner = reclamadoA.LeaseOwner
		siguiente.LeaseUntil = reloj.Ahora().Add(workerA.Lease)
		siguiente.Fence = reclamadoA.Fence
		if _, err := workerA.Advance(context.Background(), reclamadoA, siguiente); err != nil {
			t.Fatal(err)
		}
		estadoA = &siguiente
		if resultado.NextKey == nil {
			break
		}
	}
	if store.noLeidas("user-a") != 150 {
		t.Fatalf("el progreso parcial no quedó persistido: %d", store.noLeidas("user-a"))
	}

	// El lease vence: otro worker puede reclamar con un fence mayor.
	reloj.Avanzar(ReadAllLease + time.Second)
	workerB := workerDePrueba(store, reloj)
	trabajos, err = workerB.Due(context.Background())
	if err != nil || len(trabajos) != 1 {
		t.Fatalf("Due tras vencimiento: %+v %v", trabajos, err)
	}
	reclamadoB, ok, err := workerB.Claim(context.Background(), trabajos[0])
	if err != nil || !ok || reclamadoB.Fence != 2 {
		t.Fatalf("Claim B: %+v %v", reclamadoB, err)
	}

	// El worker A ya no es dueño: no puede avanzar ni cerrar.
	obsoleto := *estadoA
	obsoleto.Fence = reclamadoA.Fence
	obsoleto.LeaseOwner = reclamadoA.LeaseOwner
	obsoleto.LeaseUntil = reloj.Ahora().Add(workerA.Lease)
	if err := store.AdvanceReadAll(context.Background(), "user-a", reclamadoA.LeaseOwner, obsoleto); !errors.Is(err, ErrConflict) {
		t.Fatalf("un worker con lease vencido avanzó: %v", err)
	}
	if err := store.CompleteReadAll(context.Background(), "user-a", requestID, reclamadoA.LeaseOwner, reclamadoA.Fence, 100, reloj.Ahora()); !errors.Is(err, ErrConflict) {
		t.Fatalf("un worker con lease vencido completó: %v", err)
	}

	// B devuelve el lease que tomó a mano para que su Ciclo pueda reclamarlo.
	if err := store.ReleaseReadAll(context.Background(), reclamadoB, reloj.Ahora(), reloj.Ahora()); err != nil {
		t.Fatalf("Release B: %v", err)
	}

	// B sigue desde el cursor persistido y completa sin saltos.
	for ciclo := 0; ciclo < 4 && store.noLeidas("user-a") > 0; ciclo++ {
		workerB.Ciclo(context.Background())
	}
	final := store.estado("user-a", requestID)
	if final == nil || final.Status != StatusCompleted || final.ProcessedCount != 250 {
		t.Fatalf("recuperación incompleta: %+v quedan=%d", final, store.noLeidas("user-a"))
	}
	if store.noLeidas("user-a") != 0 {
		t.Fatal("quedaron notificaciones sin marcar tras la recuperación")
	}
	vistos := map[string]bool{}
	for _, id := range store.markCalls {
		if vistos[id] {
			t.Fatalf("notificación marcada dos veces: %s", id)
		}
		vistos[id] = true
	}
	if len(vistos) != 250 {
		t.Fatalf("se marcaron %d notificaciones distintas, esperadas 250", len(vistos))
	}
}

func TestE063DosWorkersSoloUnoReclama(t *testing.T) {
	store := nuevoFakeStore()
	reloj := nuevoReloj()
	store.sembrar("user-a", 10, 10, TypeSystem)
	const requestID = "018f90ef-08e5-7a68-b09f-000000009103"
	admitir(t, store, "user-a", requestID, reloj.Ahora())

	workerA := workerDePrueba(store, reloj)
	workerB := workerDePrueba(store, reloj)
	trabajos, err := workerA.Due(context.Background())
	if err != nil || len(trabajos) != 1 {
		t.Fatalf("Due: %+v %v", trabajos, err)
	}
	primero, okA, errA := workerA.Claim(context.Background(), trabajos[0])
	segundo, okB, errB := workerB.Claim(context.Background(), trabajos[0])
	if errA != nil || errB != nil {
		t.Fatalf("errores de reclamación: %v %v", errA, errB)
	}
	if okA == okB {
		t.Fatalf("exactamente un worker debe reclamar: A=%v B=%v", okA, okB)
	}
	ganador, perdedor := primero, segundo
	if !okA {
		ganador, perdedor = segundo, primero
	}
	if ganador.Fence != 1 || ganador.Attempts != 1 || ganador.LeaseOwner == "" {
		t.Fatalf("concesión incorrecta: %+v", ganador)
	}
	if perdedor.Fence != 0 {
		t.Fatalf("el perdedor recibió concesión: %+v", perdedor)
	}
	if completados := workerB.Ciclo(context.Background()); completados != 0 {
		t.Fatal("el worker B no debe procesar un trabajo ajeno")
	}
}

func TestE063TrabajoGrandeSinSaltosAcotadoPorCiclo(t *testing.T) {
	store := nuevoFakeStore()
	reloj := nuevoReloj()
	store.sembrar("user-a", 250, 250, TypeSystem)
	const requestID = "018f90ef-08e5-7a68-b09f-000000009104"
	admitir(t, store, "user-a", requestID, reloj.Ahora())
	worker := workerDePrueba(store, reloj)

	ciclos := 0
	for store.noLeidas("user-a") > 0 {
		ciclos++
		if ciclos > 10 {
			t.Fatal("el trabajo no converge en ciclos acotados")
		}
		antes := len(store.markCalls)
		worker.Ciclo(context.Background())
		marcadas := len(store.markCalls) - antes
		if marcadas == 0 || marcadas > MaxPaginasPorCiclo*ReadAllBatchSize {
			t.Fatalf("ciclo %d marcó %d notificaciones", ciclos, marcadas)
		}
		reloj.Avanzar(5 * time.Second)
	}
	final := store.estado("user-a", requestID)
	if final == nil || final.Status != StatusCompleted || final.ProcessedCount != 250 {
		t.Fatalf("cierre incorrecto: %+v", final)
	}
	if ciclos < 3 {
		t.Fatalf("250 pendientes no pueden completarse en %d ciclos sin superar el tope", ciclos)
	}
	vistos := map[string]bool{}
	for _, id := range store.markCalls {
		if vistos[id] {
			t.Fatalf("duplicado: %s", id)
		}
		vistos[id] = true
	}
	if len(vistos) != 250 {
		t.Fatalf("se marcaron %d distintas de 250", len(vistos))
	}
}

func TestE063ErrorTransitorioLiberaYReintenta(t *testing.T) {
	store := nuevoFakeStore()
	reloj := nuevoReloj()
	store.sembrar("user-a", 30, 30, TypeSystem)
	const requestID = "018f90ef-08e5-7a68-b09f-000000009105"
	admitir(t, store, "user-a", requestID, reloj.Ahora())
	worker := workerDePrueba(store, reloj)

	store.mu.Lock()
	store.listErr = errors.New("dynamodb caido")
	store.mu.Unlock()

	if completados := worker.Ciclo(context.Background()); completados != 0 {
		t.Fatalf("un fallo transitorio no debe completar: %d", completados)
	}
	job := store.trabajo("user-a", requestID)
	if job == nil || job.LeaseOwner != "" || job.Attempts != 1 || job.Fence != 1 {
		t.Fatalf("el trabajo no quedó liberado para reintento: %+v", job)
	}
	if job.LeaseUntil.Before(reloj.Ahora().Add(2 * time.Second)) {
		t.Fatalf("el reintento no respetó el backoff: %+v", job.LeaseUntil)
	}
	if store.noLeidas("user-a") != 30 {
		t.Fatal("un fallo transitorio no debe marcar notificaciones")
	}
	if trabajos, _ := worker.Due(context.Background()); len(trabajos) != 0 {
		t.Fatalf("el trabajo no debía estar due antes del backoff: %+v", trabajos)
	}

	store.mu.Lock()
	store.listErr = nil
	store.mu.Unlock()
	reloj.Avanzar(3 * time.Second)
	if trabajos, _ := worker.Due(context.Background()); len(trabajos) != 1 {
		t.Fatalf("tras el backoff debía estar due: %+v", trabajos)
	}
	for ciclo := 0; ciclo < 3 && store.noLeidas("user-a") > 0; ciclo++ {
		worker.Ciclo(context.Background())
		reloj.Avanzar(time.Second)
	}
	final := store.estado("user-a", requestID)
	if final == nil || final.Status != StatusCompleted || final.ProcessedCount != 30 {
		t.Fatalf("el reintento no completó: %+v", final)
	}
}

func TestE063IdempotenciaPorRequestID(t *testing.T) {
	store := nuevoFakeStore()
	reloj := nuevoReloj()
	store.sembrar("user-a", 12, 12, TypeSystem)
	const requestID = "018f90ef-08e5-7a68-b09f-000000009106"
	admitir(t, store, "user-a", requestID, reloj.Ahora())
	admitir(t, store, "user-a", requestID, reloj.Ahora())
	if store.trabajoCount() != 1 {
		t.Fatalf("el mismo requestId duplicó la cola: %d", store.trabajoCount())
	}
	servicio := NewService(store, []byte("0123456789abcdef0123456789abcdef"))
	aceptado, err := servicio.ResumeReadAll(context.Background(), "user-a", requestID)
	if err != nil || aceptado == nil || aceptado.RequestID != requestID {
		t.Fatalf("resume: %+v %v", aceptado, err)
	}

	worker := workerDePrueba(store, reloj)
	for ciclo := 0; ciclo < 2 && store.noLeidas("user-a") > 0; ciclo++ {
		worker.Ciclo(context.Background())
	}
	if store.noLeidas("user-a") != 0 || store.trabajoCount() != 0 {
		t.Fatalf("el trabajo no se cerró una sola vez: unread=%d jobs=%d",
			store.noLeidas("user-a"), store.trabajoCount())
	}
	// Reanudar después de completado no reabre trabajo ni marca nada.
	marcas := len(store.markCalls)
	if _, err := servicio.ResumeReadAll(context.Background(), "user-a", requestID); err != nil {
		t.Fatalf("resume completado: %v", err)
	}
	if store.trabajoCount() != 0 || len(store.markCalls) != marcas {
		t.Fatal("reanudar un trabajo completado volvió a encolarlo o a marcar")
	}
}

func TestE063AislamientoEntreUsuarios(t *testing.T) {
	store := nuevoFakeStore()
	reloj := nuevoReloj()
	store.sembrar("user-a", 40, 40, TypeSystem)
	store.sembrar("user-b", 40, 40, TypeSystem)
	admitir(t, store, "user-a", "018f90ef-08e5-7a68-b09f-000000009107", reloj.Ahora())

	worker := workerDePrueba(store, reloj)
	for ciclo := 0; ciclo < 3 && store.noLeidas("user-a") > 0; ciclo++ {
		worker.Ciclo(context.Background())
		reloj.Avanzar(time.Second)
	}
	if store.noLeidas("user-a") != 0 {
		t.Fatal("A no quedó marcado")
	}
	if store.noLeidas("user-b") != 40 {
		t.Fatalf("el worker modificó a B: %d", store.noLeidas("user-b"))
	}
	if store.trabajo("user-b", "") != nil {
		t.Fatal("se creó cola para un usuario sin solicitud")
	}
	for _, id := range store.markCalls {
		for _, item := range store.productos("user-b") {
			if item.notification.ID == id {
				t.Fatalf("se marcó una notificación de B: %s", id)
			}
		}
	}
}

func TestE063PuertoSinScan(t *testing.T) {
	for _, puerto := range []reflect.Type{
		reflect.TypeOf((*Store)(nil)).Elem(),
		reflect.TypeOf((*WorkerStore)(nil)).Elem(),
	} {
		for i := 0; i < puerto.NumMethod(); i++ {
			if puerto.Method(i).Name == "Scan" {
				t.Fatalf("%s expone Scan", puerto.Name())
			}
		}
	}
}

func TestE063ReintentoAcotado(t *testing.T) {
	casos := []struct {
		intentos int
		esperado time.Duration
	}{
		{0, 2 * time.Second},
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
		{4, 16 * time.Second},
		{5, 30 * time.Second},
		{50, 30 * time.Second},
	}
	for _, caso := range casos {
		if got := reintentoReadAll(caso.intentos); got != caso.esperado {
			t.Fatalf("reintentoReadAll(%d)=%s, esperado %s", caso.intentos, got, caso.esperado)
		}
	}
}
