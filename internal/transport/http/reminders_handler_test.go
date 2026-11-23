package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appReminders "github.com/nexumpr/Nexum-Backend/internal/application/reminders"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
)

type o5rStore struct {
	items map[string]*appReminders.Reminder
	idem  map[string]appReminders.IdempotencyRecord
}

func newO5RStore() *o5rStore {
	return &o5rStore{items: map[string]*appReminders.Reminder{}, idem: map[string]appReminders.IdempotencyRecord{}}
}

func o5rKey(userID, id string) string { return userID + "|" + id }

func (s *o5rStore) List(_ context.Context, q appReminders.ListQuery) (*appReminders.ListResult, error) {
	out := &appReminders.ListResult{Items: []appReminders.Reminder{}}
	for key, r := range s.items {
		if !strings.HasPrefix(key, q.UserID+"|") {
			continue
		}
		if r.Deleted || (q.Active != nil && r.Active != *q.Active) {
			continue
		}
		if q.After != nil && r.ID <= q.After.SK {
			continue
		}
		out.Items = append(out.Items, *r)
	}
	if len(out.Items) > q.Limit {
		out.Items = out.Items[:q.Limit]
		out.NextKey = &appReminders.Key{PK: "USER#" + q.UserID, SK: out.Items[len(out.Items)-1].ID}
	}
	return out, nil
}

func (s *o5rStore) Get(_ context.Context, userID, id string) (*appReminders.Reminder, error) {
	r, ok := s.items[o5rKey(userID, id)]
	if !ok {
		return nil, nil
	}
	copia := *r
	return &copia, nil
}

func (s *o5rStore) FindIdempotency(_ context.Context, userID, keyHash string) (*appReminders.IdempotencyRecord, error) {
	r, ok := s.idem[userID+"|"+keyHash]
	if !ok {
		return nil, nil
	}
	return &r, nil
}

func (s *o5rStore) Create(_ context.Context, in appReminders.CreateInput) error {
	dueAt, err := time.Parse(time.RFC3339, in.DueAt)
	if err != nil {
		return appReminders.ErrInvalidRequest
	}
	s.items[o5rKey(in.UserID, in.ID)] = &appReminders.Reminder{
		ID: in.ID, Title: in.Title, DueAt: dueAt, Timezone: in.Timezone,
		Active: true, Version: 1, Frequency: in.Frequency,
	}
	s.idem[in.UserID+"|"+in.KeyHash] = appReminders.IdempotencyRecord{
		ReminderID: in.ID, PayloadHash: in.PayloadHash,
	}
	return nil
}

func (s *o5rStore) Update(_ context.Context, in appReminders.UpdateInput) (*appReminders.Reminder, error) {
	r := s.items[o5rKey(in.UserID, in.ID)]
	if r == nil || r.Deleted {
		return nil, appReminders.ErrNotFound
	}
	if r.Version != in.ExpectedVersion {
		return nil, appReminders.ErrConflict
	}
	if in.Title != nil {
		r.Title = *in.Title
	}
	if in.Active != nil {
		r.Active = *in.Active
	}
	if in.Frequency.Set {
		r.Frequency = in.Frequency.Value
	}
	r.Version++
	copia := *r
	return &copia, nil
}

func (s *o5rStore) Delete(_ context.Context, userID, id string, version int64) error {
	r := s.items[o5rKey(userID, id)]
	if r == nil || r.Deleted {
		return appReminders.ErrNotFound
	}
	if r.Version != version {
		return appReminders.ErrConflict
	}
	r.Deleted = true
	r.Active = false
	r.Version++
	return nil
}

type o5rTokens struct{}

func (o5rTokens) GenerateAccessToken(context.Context, ports.TokenClaims) (string, error) {
	return "", nil
}
func (o5rTokens) GenerateRefreshToken(context.Context) (string, error) { return "", nil }
func (o5rTokens) ValidateAccessToken(_ context.Context, token string) (*ports.TokenClaims, error) {
	switch token {
	case "o5r-token-a":
		return &ports.TokenClaims{Subject: "user-a", SessionID: "session-a", Epoch: 1}, nil
	case "o5r-token-b":
		return &ports.TokenClaims{Subject: "user-b", SessionID: "session-b", Epoch: 1}, nil
	default:
		return nil, errors.New("token de prueba desconocido")
	}
}

type o5rAuthorizer struct{}

func (o5rAuthorizer) AuthorizeSession(context.Context, string, string, int64) error { return nil }

func o5rServicio(t *testing.T) *appReminders.Service {
	t.Helper()
	svc := appReminders.NewService(newO5RStore(), []byte("clave-hmac-de-prueba-oleada5"))
	contador := 0
	svc.WithIDGenerator(func() (string, error) {
		contador++
		return "00000000-0000-4000-8000-" + strings.Repeat("0", 11) + string(rune('0'+contador)), nil
	})
	return svc
}

func o5rHandler(metodo string, svc *appReminders.Service) http.Handler {
	switch metodo {
	case http.MethodGet:
		return NewRemindersListHandler(svc, o5rTokens{}, o5rAuthorizer{}, nil, nil)
	case http.MethodPost:
		return NewRemindersCreateHandler(svc, o5rTokens{}, o5rAuthorizer{}, nil, nil)
	case http.MethodPatch:
		return NewRemindersPatchHandler(svc, o5rTokens{}, o5rAuthorizer{}, nil, nil)
	case http.MethodDelete:
		return NewRemindersDeleteHandler(svc, o5rTokens{}, o5rAuthorizer{}, nil, nil)
	default:
		panic("metodo no soportado en la prueba")
	}
}

func o5rRequest(metodo, ruta, token, cuerpo string, headers map[string]string) *http.Request {
	var req *http.Request
	if cuerpo == "" {
		req = httptest.NewRequest(metodo, ruta, nil)
	} else {
		req = httptest.NewRequest(metodo, ruta, strings.NewReader(cuerpo))
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return req
}

func o5rEjecutar(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func o5rBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("cuerpo no JSON: %s (%v)", rec.Body.String(), err)
	}
	return body
}

func o5rCodigo(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	body := o5rBody(t, rec)
	envelope, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("sin envelope: %s", rec.Body.String())
	}
	codigo, _ := envelope["code"].(string)
	return codigo
}

func o5rCrear(t *testing.T, svc *appReminders.Service, userID, id, titulo string) *appReminders.Reminder {
	t.Helper()
	svc.WithIDGenerator(func() (string, error) { return id, nil })
	reminder, err := svc.Create(context.Background(), appReminders.CreateCommand{
		UserID: userID, IdempotencyKey: "clave-idempotencia-de-prueba-" + id,
		Title: titulo, DueAt: "2026-11-24T10:00:00Z", Timezone: "America/Mexico_City",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return reminder
}

func TestE064ListaConCacheControlYAislamiento(t *testing.T) {
	svc := o5rServicio(t)
	o5rCrear(t, svc, "user-a", "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "Recordatorio A")
	h := o5rHandler(http.MethodGet, svc)

	rec := o5rEjecutar(h, o5rRequest(http.MethodGet, "/api/v1/reminders?limit=10", "o5r-token-a", "", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("Cache-Control: %q", got)
	}
	body := o5rBody(t, rec)
	if len(body["items"].([]any)) != 1 {
		t.Fatalf("esperado un recordatorio: %s", rec.Body.String())
	}

	rec = o5rEjecutar(h, o5rRequest(http.MethodGet, "/api/v1/reminders", "o5r-token-b", "", nil))
	if rec.Code != http.StatusOK || len(o5rBody(t, rec)["items"].([]any)) != 0 {
		t.Fatalf("otro usuario no debe ver recordatorios: %d %s", rec.Code, rec.Body.String())
	}

	rec = o5rEjecutar(h, o5rRequest(http.MethodGet, "/api/v1/reminders?limit=101", "o5r-token-a", "", nil))
	if rec.Code != http.StatusBadRequest || o5rCodigo(t, rec) != "INVALID_REQUEST" {
		t.Fatalf("limit fuera de rango: %d %s", rec.Code, rec.Body.String())
	}

	rec = o5rEjecutar(h, o5rRequest(http.MethodGet, "/api/v1/reminders", "", "", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("sin JWT esperado 401, obtenido %d", rec.Code)
	}
}

func TestE065CreaConIdempotenciaYValida(t *testing.T) {
	svc := o5rServicio(t)
	h := o5rHandler(http.MethodPost, svc)
	cuerpo := `{"title":"Pagar luz","dueAt":"2026-11-24T10:00:00Z","timezone":"America/Mexico_City","frequency":"weekly"}`
	headers := map[string]string{HeaderIdempotencyKey: "clave-idempotencia-de-prueba-01"}

	rec := o5rEjecutar(h, o5rRequest(http.MethodPost, "/api/v1/reminders", "o5r-token-a", cuerpo, headers))
	if rec.Code != http.StatusCreated {
		t.Fatalf("esperado 201, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	primero := o5rBody(t, rec)
	id, _ := primero["id"].(string)
	if !strings.HasPrefix(rec.Header().Get("Location"), "/api/v1/reminders/"+id) {
		t.Fatalf("Location: %q", rec.Header().Get("Location"))
	}
	if rec.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("falta Cache-Control privada")
	}

	rec = o5rEjecutar(h, o5rRequest(http.MethodPost, "/api/v1/reminders", "o5r-token-a", cuerpo, headers))
	if rec.Code != http.StatusCreated || o5rBody(t, rec)["id"] != id {
		t.Fatalf("replay debe devolver el mismo recurso: %d %s", rec.Code, rec.Body.String())
	}

	otro := `{"title":"Otro titulo","dueAt":"2026-11-24T10:00:00Z","timezone":"America/Mexico_City"}`
	rec = o5rEjecutar(h, o5rRequest(http.MethodPost, "/api/v1/reminders", "o5r-token-a", otro, headers))
	if rec.Code != http.StatusConflict || o5rCodigo(t, rec) != "CONFLICT" {
		t.Fatalf("misma clave con otra carga: %d %s", rec.Code, rec.Body.String())
	}

	invalido := `{"title":"Pagar luz","dueAt":"2026-11-24T10:00:00Z","timezone":"Zona/No_Existe"}`
	headers2 := map[string]string{HeaderIdempotencyKey: "clave-idempotencia-de-prueba-02"}
	rec = o5rEjecutar(h, o5rRequest(http.MethodPost, "/api/v1/reminders", "o5r-token-a", invalido, headers2))
	if rec.Code != http.StatusUnprocessableEntity || o5rCodigo(t, rec) != "BUSINESS_RULE_VIOLATION" {
		t.Fatalf("zona invalida: %d %s", rec.Code, rec.Body.String())
	}

	rec = o5rEjecutar(h, o5rRequest(http.MethodPost, "/api/v1/reminders", "o5r-token-a", `{"title":"x","campoNuevo":1}`, headers2))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("campo desconocido: %d %s", rec.Code, rec.Body.String())
	}
}

func TestE066PatchExigeIfMatchYCancelaFrecuencia(t *testing.T) {
	svc := o5rServicio(t)
	id := "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	creado := o5rCrear(t, svc, "user-a", id, "Original")
	h := o5rHandler(http.MethodPatch, svc)
	ruta := "/api/v1/reminders/" + id
	cuerpo := `{"frequency":null}`

	rec := o5rEjecutar(h, o5rRequest(http.MethodPatch, ruta, "o5r-token-a", cuerpo, nil))
	if rec.Code != http.StatusPreconditionRequired || o5rCodigo(t, rec) != "PRECONDITION_REQUIRED" {
		t.Fatalf("sin If-Match: %d %s", rec.Code, rec.Body.String())
	}

	rec = o5rEjecutar(h, o5rRequest(http.MethodPatch, ruta, "o5r-token-a", cuerpo, map[string]string{"If-Match": `"v99"`}))
	if rec.Code != http.StatusPreconditionFailed || o5rCodigo(t, rec) != "VERSION_CONFLICT" {
		t.Fatalf("ETag viejo: %d %s", rec.Code, rec.Body.String())
	}

	rec = o5rEjecutar(h, o5rRequest(http.MethodPatch, ruta, "o5r-token-a", cuerpo, map[string]string{"If-Match": `"v1"`}))
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH: %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("ETag") != `"v2"` {
		t.Fatalf("ETag resultante: %q", rec.Header().Get("ETag"))
	}
	if _, ok := o5rBody(t, rec)["frequency"]; ok {
		t.Fatalf("frequency:null debe eliminar la cadencia: %s", rec.Body.String())
	}

	otraRuta := "/api/v1/reminders/cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	rec = o5rEjecutar(h, o5rRequest(http.MethodPatch, otraRuta, "o5r-token-b", cuerpo, map[string]string{"If-Match": `"v1"`}))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("otro usuario no debe parchear: %d %s", rec.Code, rec.Body.String())
	}
	_ = creado
}

func TestE067BorraLogicoYEsIdempotente(t *testing.T) {
	svc := o5rServicio(t)
	id := "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	o5rCrear(t, svc, "user-a", id, "Original")
	h := o5rHandler(http.MethodDelete, svc)
	ruta := "/api/v1/reminders/" + id

	for i := 0; i < 2; i++ {
		rec := o5rEjecutar(h, o5rRequest(http.MethodDelete, ruta, "o5r-token-a", "", nil))
		if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
			t.Fatalf("DELETE %d: esperado 204 sin cuerpo, obtenido %d %q", i, rec.Code, rec.Body.String())
		}
	}

	rec := o5rEjecutar(h, o5rRequest(http.MethodDelete, ruta, "o5r-token-b", "", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("otro usuario no debe borrar: %d %s", rec.Code, rec.Body.String())
	}

	rec = o5rEjecutar(h, o5rRequest(http.MethodDelete, "/api/v1/reminders/no-uuid", "o5r-token-a", "", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("id invalido: %d %s", rec.Code, rec.Body.String())
	}
}
