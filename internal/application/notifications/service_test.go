package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// Los dobles de esta capa son exclusivamente de prueba. La composición
// productiva usa *dynamo.NotificationsStore y la clave de entorno estable.

type fakeItem struct {
	notification Notification
	key          NotificationKey
}

type fakeStore struct {
	mu         sync.Mutex
	porUsuario map[string][]fakeItem
	readall    map[string]*ReadAllState
	jobs       map[string]*ReadAllJob
	punteros   map[string]map[string]bool

	beginCalls int
	listCalls  int
	markCalls  []string

	// contadorSemilla produce IDs únicos entre usuarios (la bandeja real usa UUID).
	contadorSemilla int

	// fallos inyectables: se comportan como DynamoDB caído, nunca como éxito.
	listErr  error
	markErr  error
	beginErr error

	now func() time.Time
}

func nuevoFakeStore() *fakeStore {
	return &fakeStore{
		porUsuario: map[string][]fakeItem{},
		readall:    map[string]*ReadAllState{},
		jobs:       map[string]*ReadAllJob{},
		punteros:   map[string]map[string]bool{},
		now:        func() time.Time { return time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC) },
	}
}

func claveReadAll(userID, requestID string) string { return userID + "\x00" + requestID }

func (f *fakeStore) sembrar(userID string, cantidad int, noLeidas int, tipo Type) {
	base := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	for i := 0; i < cantidad; i++ {
		// ID único por tirada: la bandeja real usa UUID y dos usuarios nunca
		// comparten identificador. Un contador por usuario produciría colisiones
		// entre A y B y ocultaría un fallo de aislamiento.
		f.contadorSemilla++
		id := fmt.Sprintf("018f90ef-08e5-7a68-b09f-%012d", f.contadorSemilla)
		creado := base.Add(time.Duration(i) * time.Minute)
		leida := i >= noLeidas
		f.porUsuario[userID] = append(f.porUsuario[userID], fakeItem{
			notification: Notification{
				ID: id, Title: "Aviso", Message: "Mensaje de prueba", Type: tipo,
				IsRead: leida, CreatedAt: creado,
			},
			key: NotificationKey{PK: "USER#" + userID, SK: "NOTIFICATION#" + creado.Format(time.RFC3339Nano) + "#" + id},
		})
	}
	sort.Slice(f.porUsuario[userID], func(i, j int) bool {
		return f.porUsuario[userID][i].key.SK < f.porUsuario[userID][j].key.SK
	})
}

func (f *fakeStore) productos(userID string) []fakeItem {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]fakeItem, 0, len(f.porUsuario[userID]))
	out = append(out, f.porUsuario[userID]...)
	return out
}

func (f *fakeStore) List(_ context.Context, q ListQuery) (*ListResult, error) {
	f.mu.Lock()
	f.listCalls++
	err := f.listErr
	f.mu.Unlock()
	if err != nil {
		return nil, err
	}
	items := f.productos(q.UserID)
	filtrados := make([]fakeItem, 0, len(items))
	for _, item := range items {
		if q.IsRead != nil && item.notification.IsRead != *q.IsRead {
			continue
		}
		if q.Type != "" && item.notification.Type != q.Type {
			continue
		}
		filtrados = append(filtrados, item)
	}
	desde := 0
	if q.After != nil {
		for desde < len(filtrados) && filtrados[desde].key.SK <= q.After.SK {
			desde++
		}
	}
	fin := desde + q.Limit
	if fin > len(filtrados) {
		fin = len(filtrados)
	}
	resultado := &ListResult{Items: []Notification{}}
	for _, item := range filtrados[desde:fin] {
		resultado.Items = append(resultado.Items, item.notification)
	}
	if fin < len(filtrados) && fin > desde {
		clave := filtrados[fin-1].key
		resultado.NextKey = &clave
	}
	return resultado, nil
}

func (f *fakeStore) CountUnread(_ context.Context, userID string) (int64, error) {
	var total int64
	for _, item := range f.productos(userID) {
		if !item.notification.IsRead {
			total++
		}
	}
	return total, nil
}

func (f *fakeStore) MarkRead(_ context.Context, userID, notificationID string, now time.Time) (*Notification, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.markErr != nil {
		return nil, f.markErr
	}
	for i := range f.porUsuario[userID] {
		if f.porUsuario[userID][i].notification.ID != notificationID {
			continue
		}
		if !f.porUsuario[userID][i].notification.IsRead {
			f.porUsuario[userID][i].notification.IsRead = true
			lectura := now.UTC()
			f.porUsuario[userID][i].notification.ReadAt = &lectura
		}
		f.markCalls = append(f.markCalls, notificationID)
		copia := f.porUsuario[userID][i].notification
		return &copia, nil
	}
	return nil, ErrNotFound
}

// BeginReadAll modela la transacción de admisión: registro + ítem de cola +
// puntero en una sola operación idempotente por requestId.
func (f *fakeStore) BeginReadAll(ctx context.Context, userID, requestID string, now time.Time) (*ReadAllState, error) {
	inicial, err := f.CountUnread(ctx, userID)
	if err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.beginErr != nil {
		return nil, f.beginErr
	}
	f.beginCalls++
	clave := claveReadAll(userID, requestID)
	if actual := f.readall[clave]; actual != nil {
		copia := *actual
		return &copia, nil
	}
	estado := &ReadAllState{
		RequestID: requestID, Status: StatusPending, CreatedAt: now.UTC(),
		InitialUnread: inicial, Fence: 0,
	}
	f.readall[clave] = estado
	f.jobs[clave] = &ReadAllJob{
		UserID: userID, RequestID: requestID, CreatedAt: now.UTC(), Fence: 0,
	}
	if f.punteros[userID] == nil {
		f.punteros[userID] = map[string]bool{}
	}
	f.punteros[userID][requestID] = true
	copia := *estado
	return &copia, nil
}

func (f *fakeStore) LoadReadAll(_ context.Context, userID, requestID string) (*ReadAllState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	actual := f.readall[claveReadAll(userID, requestID)]
	if actual == nil {
		return nil, ErrNotFound
	}
	copia := *actual
	return &copia, nil
}

func (f *fakeStore) AdvanceReadAll(_ context.Context, userID, expectedLeaseOwner string, next ReadAllState) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	clave := claveReadAll(userID, next.RequestID)
	actual := f.readall[clave]
	if actual == nil {
		return ErrNotFound
	}
	job := f.jobs[clave]
	if job == nil {
		return ErrConflict
	}
	if expectedLeaseOwner != "" && next.LeaseOwner != expectedLeaseOwner {
		return ErrInvalidRequest
	}
	if job.Fence != next.Fence || job.LeaseOwner != next.LeaseOwner {
		return ErrConflict
	}
	if job.LeaseUntil.IsZero() || !job.LeaseUntil.After(f.now().UTC()) {
		return ErrConflict
	}
	if next.Fence < actual.Fence {
		return ErrConflict
	}
	copia := next
	f.readall[clave] = &copia
	job.LeaseUntil = next.LeaseUntil
	return nil
}

func (f *fakeStore) CompleteReadAll(_ context.Context, userID, requestID, expectedLeaseOwner string, fence, processed int64, now time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	clave := claveReadAll(userID, requestID)
	actual := f.readall[clave]
	if actual == nil {
		return ErrNotFound
	}
	if actual.Status == StatusCompleted {
		return nil
	}
	job := f.jobs[clave]
	if job == nil || job.Fence != fence || job.LeaseOwner != expectedLeaseOwner {
		return ErrConflict
	}
	actual.Status = StatusCompleted
	actual.ProcessedCount = processed
	actual.Fence = fence
	final := now.UTC()
	actual.CompletedAt = &final
	delete(f.jobs, clave)
	if f.punteros[userID] != nil {
		delete(f.punteros[userID], requestID)
	}
	return nil
}

func (f *fakeStore) DueReadAll(_ context.Context, now time.Time, limit int) ([]ReadAllJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	claves := make([]string, 0, len(f.jobs))
	for clave := range f.jobs {
		claves = append(claves, clave)
	}
	sort.Strings(claves)
	salida := make([]ReadAllJob, 0)
	for _, clave := range claves {
		job := f.jobs[clave]
		// Due por lease: un backoff de Release deja leaseUntil futuro aunque el
		// dueño se haya limpiado; sólo vence al llegar la hora.
		if !job.LeaseUntil.IsZero() && job.LeaseUntil.After(now.UTC()) {
			continue
		}
		salida = append(salida, *job)
		if len(salida) == limit {
			break
		}
	}
	return salida, nil
}

func (f *fakeStore) ClaimReadAll(_ context.Context, job ReadAllJob, owner string, now time.Time, lease time.Duration) (ReadAllJob, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	actual := f.jobs[claveReadAll(job.UserID, job.RequestID)]
	if actual == nil {
		return ReadAllJob{}, false, nil
	}
	if actual.LeaseOwner != "" && actual.LeaseUntil.After(now.UTC()) {
		return ReadAllJob{}, false, nil
	}
	if !actual.LeaseUntil.IsZero() && actual.LeaseUntil.After(now.UTC()) {
		return ReadAllJob{}, false, nil
	}
	actual.Fence++
	actual.Attempts++
	actual.LeaseOwner = owner
	actual.LeaseUntil = now.UTC().Add(lease)
	copia := *actual
	return copia, true, nil
}

func (f *fakeStore) ReleaseReadAll(_ context.Context, job ReadAllJob, now, retryAt time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	actual := f.jobs[claveReadAll(job.UserID, job.RequestID)]
	if actual == nil || actual.Fence != job.Fence || actual.LeaseOwner != job.LeaseOwner {
		return ErrConflict
	}
	actual.LeaseOwner = ""
	actual.LeaseUntil = retryAt.UTC()
	_ = now
	return nil
}

func (f *fakeStore) FinishReadAll(_ context.Context, job ReadAllJob, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	clave := claveReadAll(job.UserID, job.RequestID)
	actual := f.jobs[clave]
	if actual == nil || actual.Fence != job.Fence || actual.LeaseOwner != job.LeaseOwner {
		return ErrConflict
	}
	delete(f.jobs, clave)
	if f.punteros[job.UserID] != nil {
		delete(f.punteros[job.UserID], job.RequestID)
	}
	return nil
}

func (f *fakeStore) estado(userID, requestID string) *ReadAllState {
	f.mu.Lock()
	defer f.mu.Unlock()
	actual := f.readall[claveReadAll(userID, requestID)]
	if actual == nil {
		return nil
	}
	copia := *actual
	return &copia
}

func (f *fakeStore) trabajo(userID, requestID string) *ReadAllJob {
	f.mu.Lock()
	defer f.mu.Unlock()
	actual := f.jobs[claveReadAll(userID, requestID)]
	if actual == nil {
		return nil
	}
	copia := *actual
	return &copia
}

func (f *fakeStore) trabajoCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.jobs)
}

func (f *fakeStore) tienePuntero(userID, requestID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.punteros[userID] != nil && f.punteros[userID][requestID]
}

func (f *fakeStore) noLeidas(userID string) int64 {
	total, _ := f.CountUnread(context.Background(), userID)
	return total
}

func servicioDePrueba(store Store) *Service {
	return NewService(store, []byte("0123456789abcdef0123456789abcdef"))
}

func esSubconjuntoDe(cuerpo map[string]json.RawMessage, permitidos ...string) error {
	tabla := map[string]bool{}
	for _, campo := range permitidos {
		tabla[campo] = true
	}
	for campo := range cuerpo {
		if !tabla[campo] {
			return fmt.Errorf("campo no contractual: %s", campo)
		}
	}
	return nil
}

func TestE061PaginaVaciaYTotales(t *testing.T) {
	servicio := servicioDePrueba(nuevoFakeStore())
	pagina, err := servicio.List(context.Background(), "user-a", ListParams{Limit: DefaultLimit})
	if err != nil {
		t.Fatal(err)
	}
	if len(pagina.Items) != 0 || pagina.UnreadCount != 0 || pagina.NextCursor != "" {
		t.Fatalf("página vacía incorrecta: %+v", pagina)
	}
	bytes, err := json.Marshal(pagina.Wire())
	if err != nil {
		t.Fatal(err)
	}
	var generic map[string]json.RawMessage
	if err := json.Unmarshal(bytes, &generic); err != nil {
		t.Fatal(err)
	}
	if err := esSubconjuntoDe(generic, "items", "unreadCount", "nextCursor"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(bytes), `"items":[]`) || !strings.Contains(string(bytes), `"unreadCount":0`) {
		t.Fatalf("forma JSON inesperada: %s", bytes)
	}
}

func TestE061TreintaPaginadasSinDuplicados(t *testing.T) {
	store := nuevoFakeStore()
	store.sembrar("user-a", 30, 17, TypeBudget)
	servicio := servicioDePrueba(store)

	primera, err := servicio.List(context.Background(), "user-a", ListParams{Limit: 25})
	if err != nil {
		t.Fatal(err)
	}
	if len(primera.Items) != 25 || primera.NextCursor == "" || primera.UnreadCount != 17 {
		t.Fatalf("primera página: items=%d next=%q unread=%d", len(primera.Items), primera.NextCursor, primera.UnreadCount)
	}
	segunda, err := servicio.List(context.Background(), "user-a", ListParams{Limit: 25, Cursor: primera.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(segunda.Items) != 5 || segunda.NextCursor != "" || segunda.UnreadCount != 17 {
		t.Fatalf("segunda página: items=%d next=%q unread=%d", len(segunda.Items), segunda.NextCursor, segunda.UnreadCount)
	}
	vistos := map[string]bool{}
	for _, item := range append(primera.Items, segunda.Items...) {
		if vistos[item.ID] {
			t.Fatalf("notificación duplicada: %s", item.ID)
		}
		vistos[item.ID] = true
	}
	if len(vistos) != 30 {
		t.Fatalf("se esperaban 30 IDs distintos, hubo %d", len(vistos))
	}
}

func TestE061FiltrosRealesYCursorLigado(t *testing.T) {
	store := nuevoFakeStore()
	store.sembrar("user-a", 6, 6, TypeBudget)
	store.sembrar("user-a", 4, 2, TypeGoal)
	servicio := servicioDePrueba(store)

	noLeidas := false
	pagina, err := servicio.List(context.Background(), "user-a", ListParams{
		Limit: 3, IsRead: &noLeidas, Type: "budget",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pagina.Items) != 3 || pagina.NextCursor == "" {
		t.Fatalf("página filtrada: %+v", pagina)
	}
	for _, item := range pagina.Items {
		if item.IsRead || item.Type != TypeBudget {
			t.Fatalf("el filtro no se aplicó: %+v", item)
		}
	}
	// El total es de TODA la bandeja (3 budget + 2 goal no leídas), no de la página.
	if pagina.UnreadCount != 8 {
		t.Fatalf("unreadCount global incorrecto: %d", pagina.UnreadCount)
	}

	// El mismo cursor con otro filtro debe rechazarse: está firmado con isRead/type.
	otroTipo := "goal"
	if _, err := servicio.List(context.Background(), "user-a", ListParams{
		Limit: 3, Cursor: pagina.NextCursor, IsRead: &noLeidas, Type: otroTipo,
	}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("cursor reutilizado con otro filtro: %v", err)
	}
}

func TestE061CursorTamperYOtroUsuario(t *testing.T) {
	store := nuevoFakeStore()
	store.sembrar("user-a", 30, 30, TypeSystem)
	servicio := servicioDePrueba(store)
	pagina, err := servicio.List(context.Background(), "user-a", ListParams{Limit: 25})
	if err != nil {
		t.Fatal(err)
	}
	cursor := pagina.NextCursor
	if cursor == "" {
		t.Fatal("sin cursor para probar firma")
	}

	if _, err := servicio.List(context.Background(), "user-a", ListParams{Limit: 25, Cursor: "corto"}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("cursor corto: %v", err)
	}
	tamper := []byte(cursor)
	if tamper[len(tamper)-2] == 'A' {
		tamper[len(tamper)-2] = 'B'
	} else {
		tamper[len(tamper)-2] = 'A'
	}
	if _, err := servicio.List(context.Background(), "user-a", ListParams{Limit: 25, Cursor: string(tamper)}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("cursor alterado: %v", err)
	}
	if _, err := servicio.List(context.Background(), "user-b", ListParams{Limit: 25, Cursor: cursor}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("cursor de otro usuario: %v", err)
	}
}

func TestE062IdempotenteYAjeno(t *testing.T) {
	store := nuevoFakeStore()
	store.sembrar("user-a", 2, 2, TypeStreak)
	servicio := servicioDePrueba(store)
	id := "018f90ef-08e5-7a68-b09f-000000000001"

	primera, err := servicio.MarkRead(context.Background(), "user-a", id)
	if err != nil || !primera.IsRead || primera.UnreadCount == nil || *primera.UnreadCount != 1 {
		t.Fatalf("primera lectura: %+v %v", primera, err)
	}
	segunda, err := servicio.MarkRead(context.Background(), "user-a", id)
	if err != nil || !segunda.IsRead || segunda.UnreadCount == nil || *segunda.UnreadCount != 1 {
		t.Fatalf("repetición no idempotente: %+v %v", segunda, err)
	}
	if _, err := servicio.MarkRead(context.Background(), "user-b", id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("notificación ajena: %v", err)
	}
}

func TestE063Admision202SinProcesarNotificaciones(t *testing.T) {
	store := nuevoFakeStore()
	store.sembrar("user-a", 30, 17, TypeSystem)
	servicio := servicioDePrueba(store).WithIDGenerator(func() (string, error) {
		return "018f90ef-08e5-7a68-b09f-000000009001", nil
	})

	antes := store.noLeidas("user-a")
	aceptado, err := servicio.StartReadAll(context.Background(), "user-a")
	if err != nil || aceptado == nil || aceptado.Status != "accepted" ||
		aceptado.RequestID != "018f90ef-08e5-7a68-b09f-000000009001" {
		t.Fatalf("E063 admisión: %+v %v", aceptado, err)
	}
	// La petición NO procesa: ni marca, ni lista páginas de la bandeja.
	if store.listCalls != 0 || len(store.markCalls) != 0 {
		t.Fatalf("la petición procesó trabajo: list=%d mark=%d", store.listCalls, len(store.markCalls))
	}
	if despues := store.noLeidas("user-a"); despues != antes {
		t.Fatalf("la petición marcó notificaciones: %d -> %d", antes, despues)
	}

	estado := store.estado("user-a", aceptado.RequestID)
	if estado == nil || estado.Status != StatusPending || estado.Cursor != nil || estado.InitialUnread != 17 {
		t.Fatalf("registro READALL pendiente incorrecto: %+v", estado)
	}
	trabajo := store.trabajo("user-a", aceptado.RequestID)
	if trabajo == nil || trabajo.Fence != 0 || trabajo.LeaseOwner != "" {
		t.Fatalf("ítem de cola durable incorrecto: %+v", trabajo)
	}
	if !store.tienePuntero("user-a", aceptado.RequestID) {
		t.Fatal("falta el puntero READALLPTR del titular")
	}

	// Reenviar el mismo requestId no duplica el trabajo.
	if _, err := servicio.ResumeReadAll(context.Background(), "user-a", aceptado.RequestID); err != nil {
		t.Fatalf("reanudación idempotente: %v", err)
	}
	if store.trabajoCount() != 1 {
		t.Fatalf("la cola duplicó el trabajo: %d", store.trabajoCount())
	}
}

func TestE063AdmisionFallaCerradaSinRegistro(t *testing.T) {
	store := nuevoFakeStore()
	store.sembrar("user-a", 3, 3, TypeSystem)
	store.beginErr = errors.New("dynamodb caido")
	servicio := servicioDePrueba(store)
	if _, err := servicio.StartReadAll(context.Background(), "user-a"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("sin persistencia no se debe aceptar: %v", err)
	}
	if store.trabajoCount() != 0 {
		t.Fatal("no debe quedar trabajo durable sin admisión confirmada")
	}
}

func TestE063SinClaveFalloCerrado(t *testing.T) {
	store := nuevoFakeStore()
	servicio := NewService(store, nil)
	if servicio.Disponible() {
		t.Fatal("sin clave HMAC el servicio no debe estar disponible")
	}
	if _, err := servicio.List(context.Background(), "user-a", ListParams{}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("list sin clave: %v", err)
	}
	if _, err := servicio.MarkRead(context.Background(), "user-a", "id"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("mark sin clave: %v", err)
	}
	if _, err := servicio.StartReadAll(context.Background(), "user-a"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("read-all sin clave: %v", err)
	}
}
