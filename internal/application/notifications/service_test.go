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
	mu             sync.Mutex
	porUsuario     map[string][]fakeItem
	readall        map[string]*ReadAllState
	beginCalls     int
	advanceCalls   int
	failAdvanceAt  int
	readallFallido bool
}

func nuevoFakeStore() *fakeStore {
	return &fakeStore{porUsuario: map[string][]fakeItem{}, readall: map[string]*ReadAllState{}}
}

func (f *fakeStore) sembrar(userID string, cantidad int, noLeidas int, tipo Type) {
	base := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	for i := 0; i < cantidad; i++ {
		id := fmt.Sprintf("018f90ef-08e5-7a68-b09f-%012d", i+1)
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
	for _, item := range f.porUsuario[userID] {
		out = append(out, item)
	}
	return out
}

func (f *fakeStore) List(_ context.Context, q ListQuery) (*ListResult, error) {
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
	for i := range f.porUsuario[userID] {
		if f.porUsuario[userID][i].notification.ID != notificationID {
			continue
		}
		if !f.porUsuario[userID][i].notification.IsRead {
			f.porUsuario[userID][i].notification.IsRead = true
			lectura := now.UTC()
			f.porUsuario[userID][i].notification.ReadAt = &lectura
		}
		copia := f.porUsuario[userID][i].notification
		return &copia, nil
	}
	return nil, ErrNotFound
}

func (f *fakeStore) BeginReadAll(ctx context.Context, userID, requestID string, now time.Time) (*ReadAllState, error) {
	inicial, err := f.CountUnread(ctx, userID)
	if err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.beginCalls++
	if actual := f.readall[userID+"\x00"+requestID]; actual != nil {
		copia := *actual
		return &copia, nil
	}
	estado := &ReadAllState{RequestID: requestID, Status: StatusPending, CreatedAt: now.UTC(), InitialUnread: inicial}
	f.readall[userID+"\x00"+requestID] = estado
	copia := *estado
	return &copia, nil
}

func (f *fakeStore) LoadReadAll(_ context.Context, userID, requestID string) (*ReadAllState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	actual := f.readall[userID+"\x00"+requestID]
	if actual == nil {
		return nil, ErrNotFound
	}
	copia := *actual
	return &copia, nil
}

func (f *fakeStore) AdvanceReadAll(_ context.Context, userID, expectedLeaseOwner string, next ReadAllState) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	actual := f.readall[userID+"\x00"+next.RequestID]
	if actual == nil {
		return ErrNotFound
	}
	if expectedLeaseOwner != "" && actual.LeaseOwner != expectedLeaseOwner {
		return ErrConflict
	}
	f.advanceCalls++
	if f.failAdvanceAt > 0 && f.advanceCalls == f.failAdvanceAt {
		return errors.New("fallo transitorio antes de persistir el avance")
	}
	copia := next
	f.readall[userID+"\x00"+next.RequestID] = &copia
	return nil
}

func (f *fakeStore) CompleteReadAll(_ context.Context, userID, requestID, expectedLeaseOwner string, processed int64, now time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	actual := f.readall[userID+"\x00"+requestID]
	if actual == nil {
		return ErrNotFound
	}
	if expectedLeaseOwner != "" && actual.LeaseOwner != expectedLeaseOwner {
		return ErrConflict
	}
	actual.Status = StatusCompleted
	actual.ProcessedCount = processed
	final := now.UTC()
	actual.CompletedAt = &final
	return nil
}

func (f *fakeStore) estado(userID, requestID string) *ReadAllState {
	f.mu.Lock()
	defer f.mu.Unlock()
	actual := f.readall[userID+"\x00"+requestID]
	if actual == nil {
		return nil
	}
	copia := *actual
	return &copia
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

func TestE063CeroUnYCientosConReanudacion(t *testing.T) {
	for _, caso := range []struct {
		nombre   string
		pending  int
		fallaEn  int
		reintent bool
	}{
		{nombre: "cero_pendientes", pending: 0},
		{nombre: "una_pendiente", pending: 1},
		{nombre: "cientos_con_reanudacion", pending: 137, fallaEn: 2, reintent: true},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			store := nuevoFakeStore()
			store.sembrar("user-a", caso.pending, caso.pending, TypeSystem)
			store.sembrar("user-b", 5, 5, TypeSystem)
			store.failAdvanceAt = caso.fallaEn

			secuencia := 0
			generador := func() (string, error) {
				secuencia++
				return fmt.Sprintf("018f90ef-08e5-7a68-b09f-%012d", 9000+secuencia), nil
			}
			servicio := servicioDePrueba(store).WithIDGenerator(generador)
			requestID := "018f90ef-08e5-7a68-b09f-000000009001"

			aceptado, err := servicio.StartReadAll(context.Background(), "user-a")
			if caso.fallaEn > 0 {
				if err == nil {
					t.Fatal("el fallo a mitad no se propagó: no se debe responder 202 falso")
				}
				if store.beginCalls == 0 {
					t.Fatal("el registro READALL debe persistirse antes de trabajar")
				}
				parcial := store.estado("user-a", requestID)
				if parcial == nil || parcial.Status != StatusPending || parcial.Cursor == nil {
					t.Fatalf("el avance persistido no permite reanudar: %+v", parcial)
				}
				if !caso.reintent {
					t.Fatal("caso de reanudación mal configurado")
				}
				store.failAdvanceAt = 0
				aceptado, err = servicio.ResumeReadAll(context.Background(), "user-a", requestID)
			}
			if err != nil || aceptado == nil || aceptado.Status != "accepted" || aceptado.RequestID != requestID {
				t.Fatalf("E063: %+v %v", aceptado, err)
			}
			final := store.estado("user-a", requestID)
			if final == nil || final.Status != StatusCompleted || final.CompletedAt == nil {
				t.Fatalf("READALL no quedó completado: %+v", final)
			}
			if final.ProcessedCount != int64(caso.pending) {
				t.Fatalf("processedCount=%d, esperado %d", final.ProcessedCount, caso.pending)
			}
			if noLeidas, err := servicio.UnreadCount(context.Background(), "user-a"); err != nil || noLeidas != 0 {
				t.Fatalf("unreadCount de A: %d %v", noLeidas, err)
			}
			if noLeidas, err := servicio.UnreadCount(context.Background(), "user-b"); err != nil || noLeidas != 5 {
				t.Fatalf("usuario B fue modificado: %d %v", noLeidas, err)
			}
		})
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
