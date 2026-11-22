package transport_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	appNotifications "github.com/nexumpr/Nexum-Backend/internal/application/notifications"
	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

var e061HTTPClave = []byte("0123456789abcdef0123456789abcdef")

type e061HTTPItem struct {
	n  appNotifications.Notification
	sk string
}

type e061HTTPStore struct {
	mu      sync.Mutex
	porUser map[string][]e061HTTPItem
	readall map[string]*appNotifications.ReadAllState
	err     error
}

func e061NuevoStore() *e061HTTPStore {
	return &e061HTTPStore{porUser: map[string][]e061HTTPItem{}, readall: map[string]*appNotifications.ReadAllState{}}
}

func (f *e061HTTPStore) sembrar(userID string, cantidad, noLeidas int, tipo appNotifications.Type) {
	f.sembrarConOffset(userID, cantidad, noLeidas, tipo, 0)
}

func (f *e061HTTPStore) sembrarConOffset(userID string, cantidad, noLeidas int, tipo appNotifications.Type, offset int) {
	base := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	for i := 0; i < cantidad; i++ {
		id := fmt.Sprintf("018f90ef-08e5-7a68-b09f-%012d", offset+i+1)
		creado := base.Add(time.Duration(i) * time.Minute).UTC()
		f.porUser[userID] = append(f.porUser[userID], e061HTTPItem{
			n: appNotifications.Notification{
				ID: id, Title: "Aviso", Message: "Revisa tu presupuesto", Type: tipo,
				IsRead: i >= noLeidas, CreatedAt: creado,
			},
			sk: "NOTIFICATION#" + creado.Format(time.RFC3339Nano) + "#" + id,
		})
	}
	sort.Slice(f.porUser[userID], func(i, j int) bool { return f.porUser[userID][i].sk < f.porUser[userID][j].sk })
}

func (f *e061HTTPStore) copias(userID string) []e061HTTPItem {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]e061HTTPItem(nil), f.porUser[userID]...)
}

func (f *e061HTTPStore) List(_ context.Context, q appNotifications.ListQuery) (*appNotifications.ListResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	filtrados := make([]e061HTTPItem, 0)
	for _, item := range f.copias(q.UserID) {
		if q.IsRead != nil && item.n.IsRead != *q.IsRead {
			continue
		}
		if q.Type != "" && item.n.Type != q.Type {
			continue
		}
		filtrados = append(filtrados, item)
	}
	desde := 0
	if q.After != nil {
		for desde < len(filtrados) && filtrados[desde].sk <= q.After.SK {
			desde++
		}
	}
	fin := desde + q.Limit
	if fin > len(filtrados) {
		fin = len(filtrados)
	}
	resultado := &appNotifications.ListResult{Items: []appNotifications.Notification{}}
	for _, item := range filtrados[desde:fin] {
		resultado.Items = append(resultado.Items, item.n)
	}
	if fin < len(filtrados) && fin > desde {
		clave := appNotifications.NotificationKey{PK: "USER#" + q.UserID, SK: filtrados[fin-1].sk}
		resultado.NextKey = &clave
	}
	return resultado, nil
}

func (f *e061HTTPStore) CountUnread(_ context.Context, userID string) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	var total int64
	for _, item := range f.copias(userID) {
		if !item.n.IsRead {
			total++
		}
	}
	return total, nil
}

func (f *e061HTTPStore) MarkRead(_ context.Context, userID, notificationID string, now time.Time) (*appNotifications.Notification, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.porUser[userID] {
		if f.porUser[userID][i].n.ID != notificationID {
			continue
		}
		if !f.porUser[userID][i].n.IsRead {
			f.porUser[userID][i].n.IsRead = true
			lectura := now.UTC()
			f.porUser[userID][i].n.ReadAt = &lectura
		}
		copia := f.porUser[userID][i].n
		return &copia, nil
	}
	return nil, appNotifications.ErrNotFound
}

func (f *e061HTTPStore) BeginReadAll(ctx context.Context, userID, requestID string, now time.Time) (*appNotifications.ReadAllState, error) {
	inicial, err := f.CountUnread(ctx, userID)
	if err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	clave := userID + "\x00" + requestID
	if actual := f.readall[clave]; actual != nil {
		copia := *actual
		return &copia, nil
	}
	estado := &appNotifications.ReadAllState{RequestID: requestID, Status: appNotifications.StatusPending, CreatedAt: now.UTC(), InitialUnread: inicial}
	f.readall[clave] = estado
	copia := *estado
	return &copia, nil
}

func (f *e061HTTPStore) LoadReadAll(_ context.Context, userID, requestID string) (*appNotifications.ReadAllState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	actual := f.readall[userID+"\x00"+requestID]
	if actual == nil {
		return nil, appNotifications.ErrNotFound
	}
	copia := *actual
	return &copia, nil
}

func (f *e061HTTPStore) AdvanceReadAll(_ context.Context, userID, expectedOwner string, next appNotifications.ReadAllState) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	actual := f.readall[userID+"\x00"+next.RequestID]
	if actual == nil || (expectedOwner != "" && actual.LeaseOwner != expectedOwner) {
		return appNotifications.ErrConflict
	}
	copia := next
	f.readall[userID+"\x00"+next.RequestID] = &copia
	return nil
}

func (f *e061HTTPStore) CompleteReadAll(_ context.Context, userID, requestID, owner string, processed int64, now time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	actual := f.readall[userID+"\x00"+requestID]
	if actual == nil || (owner != "" && actual.LeaseOwner != owner) {
		return appNotifications.ErrConflict
	}
	actual.Status = appNotifications.StatusCompleted
	actual.ProcessedCount = processed
	final := now.UTC()
	actual.CompletedAt = &final
	return nil
}

func e061HTTPHandler(store appNotifications.Store, limitador internalhttp.LimitadorRuta, clave []byte) *internalhttp.NotificationsHandler {
	servicio := appNotifications.NewService(store, clave)
	return internalhttp.NewNotificationsHandler(servicio, e004TokensValidos(), newE004Sesiones(), limitador, clave, silencioso())
}

func e061HTTPStoreValido() *e061HTTPStore {
	store := e061NuevoStore()
	store.sembrar(e004HTTPUsuario, 30, 17, appNotifications.TypeBudget)
	store.sembrarConOffset(e004HTTPOtroUser, 4, 4, appNotifications.TypeGoal, 1000)
	return store
}

func e061HTTPPeticion(metodo, ruta, token string) *http.Request {
	return e004Peticion(metodo, ruta, token, "")
}

func e061HTTPEnvelope(t *testing.T, rec *httptest.ResponseRecorder, esperado string) {
	t.Helper()
	var envelope struct {
		Error struct {
			Code    string            `json:"code"`
			Message string            `json:"message"`
			TraceID string            `json:"traceId"`
			Details []json.RawMessage `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("ErrorEnvelope inválido: %v (%s)", err, rec.Body.String())
	}
	if envelope.Error.Code != esperado || envelope.Error.Message == "" || envelope.Error.Details == nil {
		t.Fatalf("ErrorEnvelope fuera de contrato: %s", rec.Body.String())
	}
}

func TestE061HTTPEsquemaYYYY(t *testing.T) {
	rec := httptest.NewRecorder()
	e061HTTPHandler(e061HTTPStoreValido(), e053Limitador{permitido: true}, e061HTTPClave).ServeHTTP(
		rec, e061HTTPPeticion(http.MethodGet, "/api/v1/notifications?limit=10&isRead=false", "jwt-valido"))

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("Cache-Control incorrecto: %q", got)
	}
	var cuerpo struct {
		Items       []map[string]json.RawMessage `json:"items"`
		NextCursor  string                       `json:"nextCursor"`
		UnreadCount int64                        `json:"unreadCount"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("JSON inválido: %v", err)
	}
	if len(cuerpo.Items) != 10 || cuerpo.NextCursor == "" || cuerpo.UnreadCount != 17 {
		t.Fatalf("cuerpo fuera de contrato: items=%d next=%q unread=%d", len(cuerpo.Items), cuerpo.NextCursor, cuerpo.UnreadCount)
	}
	var general map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &general); err != nil {
		t.Fatal(err)
	}
	for campo := range general {
		if campo != "items" && campo != "nextCursor" && campo != "unreadCount" {
			t.Fatalf("campo no contractual: %s", campo)
		}
	}
	permitidos := map[string]bool{"id": true, "title": true, "message": true, "isRead": true, "createdAt": true}
	for _, item := range cuerpo.Items {
		for campo := range item {
			if !permitidos[campo] {
				t.Fatalf("campo no contractual en Notification: %s", campo)
			}
		}
		if string(item["isRead"]) != "false" {
			t.Fatalf("isRead debió ser false: %s", item["isRead"])
		}
	}
}

func TestE061HTTPNegativas(t *testing.T) {
	store := e061HTTPStoreValido()
	var cursor string
	recursor := httptest.NewRecorder()
	e061HTTPHandler(store, e053Limitador{permitido: true}, e061HTTPClave).ServeHTTP(
		recursor, e061HTTPPeticion(http.MethodGet, "/api/v1/notifications?limit=25", "jwt-valido"))
	var pagina struct {
		NextCursor string `json:"nextCursor"`
	}
	if err := json.Unmarshal(recursor.Body.Bytes(), &pagina); err != nil || pagina.NextCursor == "" {
		t.Fatalf("no se pudo obtener un cursor válido: %v %s", err, recursor.Body.String())
	}
	cursor = pagina.NextCursor
	tamper := []byte(cursor)
	if tamper[10] == 'A' {
		tamper[10] = 'B'
	} else {
		tamper[10] = 'A'
	}

	casos := []struct {
		nombre    string
		ruta      string
		token     string
		limitador internalhttp.LimitadorRuta
		store     appNotifications.Store
		esperado  int
		codigo    string
	}{
		{"limite_cero", "/api/v1/notifications?limit=0", "jwt-valido", e053Limitador{permitido: true}, store, 400, "INVALID_REQUEST"},
		{"limite_101", "/api/v1/notifications?limit=101", "jwt-valido", e053Limitador{permitido: true}, store, 400, "INVALID_REQUEST"},
		{"cursor_corto", "/api/v1/notifications?cursor=abc", "jwt-valido", e053Limitador{permitido: true}, store, 400, "INVALID_REQUEST"},
		{"cursor_tamper", "/api/v1/notifications?cursor=" + string(tamper), "jwt-valido", e053Limitador{permitido: true}, store, 400, "INVALID_REQUEST"},
		{"isread_invalido", "/api/v1/notifications?isRead=quizas", "jwt-valido", e053Limitador{permitido: true}, store, 400, "INVALID_REQUEST"},
		{"type_invalido", "/api/v1/notifications?type=desconocido", "jwt-valido", e053Limitador{permitido: true}, store, 400, "INVALID_REQUEST"},
		{"sin_token", "/api/v1/notifications", "", e053Limitador{permitido: true}, store, 401, "UNAUTHORIZED"},
		{"cuota", "/api/v1/notifications", "jwt-valido", e053Limitador{permitido: false}, store, 429, "RATE_LIMITED"},
		{"store_caido", "/api/v1/notifications", "jwt-valido", e053Limitador{permitido: true}, &e061HTTPStore{err: context.DeadlineExceeded}, 503, "DEPENDENCY_UNAVAILABLE"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			e061HTTPHandler(caso.store, caso.limitador, e061HTTPClave).ServeHTTP(
				rec, e061HTTPPeticion(http.MethodGet, caso.ruta, caso.token))
			if rec.Code != caso.esperado {
				t.Fatalf("esperado %d, obtenido %d: %s", caso.esperado, rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("Cache-Control"); got != "private, no-store" {
				t.Fatalf("Cache-Control ausente en error: %q", got)
			}
			e061HTTPEnvelope(t, rec, caso.codigo)
		})
	}
}

func TestE061HTTPFailClosedSinClave(t *testing.T) {
	rec := httptest.NewRecorder()
	e061HTTPHandler(e061HTTPStoreValido(), e053Limitador{permitido: true}, nil).ServeHTTP(
		rec, e061HTTPPeticion(http.MethodGet, "/api/v1/notifications", "jwt-valido"))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("sin clave HMAC: esperado 503, obtenido %d: %s", rec.Code, rec.Body.String())
	}
	e061HTTPEnvelope(t, rec, "DEPENDENCY_UNAVAILABLE")

	sinServicio := internalhttp.NewNotificationsHandler(nil, e004TokensValidos(), newE004Sesiones(), nil, e061HTTPClave, silencioso())
	rec2 := httptest.NewRecorder()
	sinServicio.ServeHTTP(rec2, e061HTTPPeticion(http.MethodGet, "/api/v1/notifications", "jwt-valido"))
	if rec2.Code != http.StatusServiceUnavailable {
		t.Fatalf("sin servicio: esperado 503, obtenido %d", rec2.Code)
	}
}

func TestE062HTTPContratoYAjeno(t *testing.T) {
	store := e061HTTPStoreValido()
	id := "018f90ef-08e5-7a68-b09f-000000000001"
	req := e061HTTPPeticion(http.MethodPatch, "/api/v1/notifications/"+id+"/read", "jwt-valido")
	req.SetPathValue("notificationId", id)
	rec := httptest.NewRecorder()
	e061HTTPHandler(store, e053Limitador{permitido: true}, e061HTTPClave).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("Cache-Control incorrecto: %q", got)
	}
	var notificacion struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Message     string `json:"message"`
		IsRead      bool   `json:"isRead"`
		CreatedAt   string `json:"createdAt"`
		UnreadCount int64  `json:"unreadCount"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &notificacion); err != nil {
		t.Fatal(err)
	}
	if notificacion.ID != id || !notificacion.IsRead || notificacion.Title == "" || notificacion.Message == "" ||
		notificacion.CreatedAt == "" || notificacion.UnreadCount != 16 {
		t.Fatalf("Notification fuera de contrato: %+v", notificacion)
	}
	var general map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &general); err != nil {
		t.Fatal(err)
	}
	for campo := range general {
		switch campo {
		case "id", "title", "message", "isRead", "createdAt", "unreadCount":
		default:
			t.Fatalf("campo no contractual en E062: %s", campo)
		}
	}

	// repetition must be idempotent: el total no se decrementa dos veces.
	repetida := httptest.NewRecorder()
	req2 := e061HTTPPeticion(http.MethodPatch, "/api/v1/notifications/"+id+"/read", "jwt-valido")
	req2.SetPathValue("notificationId", id)
	e061HTTPHandler(store, e053Limitador{permitido: true}, e061HTTPClave).ServeHTTP(repetida, req2)
	if repetida.Code != http.StatusOK || !strings.Contains(repetida.Body.String(), `"unreadCount":16`) {
		t.Fatalf("repetición no idempotente: %d %s", repetida.Code, repetida.Body.String())
	}

	ajena := httptest.NewRecorder()
	req3 := e061HTTPPeticion(http.MethodPatch, "/api/v1/notifications/"+id+"/read", "jwt-sujeto-ajeno")
	req3.SetPathValue("notificationId", id)
	e061HTTPHandler(store, e053Limitador{permitido: true}, e061HTTPClave).ServeHTTP(ajena, req3)
	if ajena.Code != http.StatusNotFound {
		t.Fatalf("recurso ajeno: esperado 404, obtenido %d: %s", ajena.Code, ajena.Body.String())
	}
	e061HTTPEnvelope(t, ajena, "NOT_FOUND")

	invalida := httptest.NewRecorder()
	req4 := e061HTTPPeticion(http.MethodPatch, "/api/v1/notifications/no-es-uuid/read", "jwt-valido")
	req4.SetPathValue("notificationId", "no-es-uuid")
	e061HTTPHandler(store, e053Limitador{permitido: true}, e061HTTPClave).ServeHTTP(invalida, req4)
	if invalida.Code != http.StatusBadRequest {
		t.Fatalf("UUID inválido: esperado 400, obtenido %d", invalida.Code)
	}
}

func TestE063HTTPAceptadoConTrabajoReal(t *testing.T) {
	store := e061HTTPStoreValido()
	rec := httptest.NewRecorder()
	e061HTTPHandler(store, e053Limitador{permitido: true}, e061HTTPClave).ServeHTTP(
		rec, e061HTTPPeticion(http.MethodPost, "/api/v1/notifications/read-all", "jwt-valido"))

	if rec.Code != http.StatusAccepted {
		t.Fatalf("esperado 202, obtenido %d: %s", rec.Code, rec.Body.String())
	}
	var accepted struct {
		Status    string `json:"status"`
		RequestID string `json:"requestId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &accepted); err != nil {
		t.Fatal(err)
	}
	if accepted.Status != "accepted" || !notificationIDPatternHTTP(accepted.RequestID) {
		t.Fatalf("Accepted fuera de contrato: %+v", accepted)
	}
	var general map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &general); err != nil {
		t.Fatal(err)
	}
	if len(general) != 2 || general["status"] == nil || general["requestId"] == nil {
		t.Fatalf("campos no contractuales en 202: %v", general)
	}
	if total, err := store.CountUnread(context.Background(), e004HTTPUsuario); err != nil || total != 0 {
		t.Fatalf("el trabajo real no se ejecutó: unreadCount=%d %v", total, err)
	}
	if total, err := store.CountUnread(context.Background(), e004HTTPOtroUser); err != nil || total != 4 {
		t.Fatalf("usuario B intacto: unreadCount=%d %v", total, err)
	}

	// El 202 posterior con el mismo store es seguro: no resurrect notificaciones.
	rec2 := httptest.NewRecorder()
	e061HTTPHandler(store, e053Limitador{permitido: true}, e061HTTPClave).ServeHTTP(
		rec2, e061HTTPPeticion(http.MethodPost, "/api/v1/notifications/read-all", "jwt-valido"))
	if rec2.Code != http.StatusAccepted {
		t.Fatalf("segundo read-all: %d %s", rec2.Code, rec2.Body.String())
	}
}

func notificationIDPatternHTTP(id string) bool {
	return len(id) == 36 && strings.Count(id, "-") == 4
}
