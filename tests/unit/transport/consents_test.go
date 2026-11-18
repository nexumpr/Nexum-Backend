package transport_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	appPrivacy "github.com/nexumpr/Nexum-Backend/internal/application/privacy"
	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// Pruebas HTTP de E016 (`GET /api/v1/users/me/consents`): middleware de autenticación,
// contrato `PageOfConsent` con EXACTAMENTE cinco campos por consentimiento, paginación por
// cursor, rechazo de cursor inválido y fail-closed sin persistencia. Los dobles son
// exclusivamente de prueba; la composición productiva usa DynamoDB real.

var e016HTTPAhora = time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)

const e016ClaveCursores = "clave-de-prueba-para-cursores-http-de-consentimientos"

type e016HTTPPagina struct {
	registros []appPrivacy.ConsentRecord
	nextSK    string
}

type e016HTTPStore struct {
	mu       sync.Mutex
	paginas  map[string]e016HTTPPagina
	usuarios []string
	startSKs []string
	fallo    error
}

func newE016HTTPStore() *e016HTTPStore {
	return &e016HTTPStore{paginas: map[string]e016HTTPPagina{}}
}

func (s *e016HTTPStore) ListUserConsents(_ context.Context, userID, startSK string, _ int) ([]appPrivacy.ConsentRecord, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.usuarios = append(s.usuarios, userID)
	s.startSKs = append(s.startSKs, startSK)
	if s.fallo != nil {
		return nil, "", s.fallo
	}
	pagina, ok := s.paginas[userID+"\x00"+startSK]
	if !ok {
		return nil, "", nil
	}
	return pagina.registros, pagina.nextSK, nil
}

func e016HTTPRegistro(id, purpose string, granted bool) appPrivacy.ConsentRecord {
	grabado := e016HTTPAhora.Add(-2 * time.Hour)
	return appPrivacy.ConsentRecord{
		SortKey:       "CONSENT#" + grabado.Format("2006-01-02T15:04:05.000000000Z") + "#" + id,
		ID:            id,
		NoticeVersion: "2026-11",
		Purpose:       purpose,
		Granted:       granted,
		RecordedAt:    grabado,
	}
}

func e016HTTPHandler(store appPrivacy.ConsentsStore) *internalhttp.ConsentsHandler {
	var servicio *appPrivacy.ConsentsService
	if store != nil {
		servicio = appPrivacy.NewConsentsService(store, []byte(e016ClaveCursores)).
			WithClock(func() time.Time { return e016HTTPAhora })
	}
	return internalhttp.NewConsentsHandler(servicio, e004TokensValidos(), newE004Sesiones(), silencioso())
}

func e016HTTPPeticion(token, query string) *http.Request {
	ruta := "/api/v1/users/me/consents"
	if query != "" {
		ruta += "?" + query
	}
	return e004Peticion(http.MethodGet, ruta, token, "")
}

// TestE016HTTPEsquemaExacto cubre el contrato: 200 con `PageOfConsent`, exactamente cinco
// campos por consentimiento, `granted:false` presente y ningún atributo interno.
func TestE016HTTPEsquemaExacto(t *testing.T) {
	store := newE016HTTPStore()
	store.paginas[e004HTTPUsuario+"\x00"] = e016HTTPPagina{registros: []appPrivacy.ConsentRecord{
		e016HTTPRegistro(e004HTTPSesion, appPrivacy.ConsentPurposeNotifications, false),
		e016HTTPRegistro(e004HTTPSesionB, appPrivacy.ConsentPurposePrivacyNotice, true),
	}}

	rec := httptest.NewRecorder()
	e016HTTPHandler(store).ServeHTTP(rec, e016HTTPPeticion("jwt-valido", ""))

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if cache := rec.Header().Get("Cache-Control"); !strings.Contains(cache, "private") || !strings.Contains(cache, "no-store") {
		t.Fatalf("Cache-Control debe ser 'private, no-store', obtenido %q", cache)
	}

	var cuerpo map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("respuesta no es JSON: %v", err)
	}
	for clave := range cuerpo {
		if clave != "items" && clave != "nextCursor" {
			t.Fatalf("PageOfConsent no admite la propiedad %q", clave)
		}
	}
	items, ok := cuerpo["items"].([]any)
	if !ok {
		t.Fatalf("items debe ser un array: %v", cuerpo["items"])
	}
	if len(items) != 2 {
		t.Fatalf("se esperaban 2 consentimientos, hay %d", len(items))
	}
	esperados := map[string]bool{e004HTTPSesion: false, e004HTTPSesionB: true}
	for _, entrada := range items {
		item, ok := entrada.(map[string]any)
		if !ok {
			t.Fatalf("cada consentimiento debe ser un objeto: %v", entrada)
		}
		if len(item) != 5 {
			t.Fatalf("Consent es un objeto cerrado de 5 campos, se obtuvo %v", item)
		}
		for _, requerida := range []string{"id", "noticeVersion", "purpose", "granted", "recordedAt"} {
			if _, existe := item[requerida]; !existe {
				t.Fatalf("falta el campo requerido %q en %v", requerida, item)
			}
		}
		id, _ := item["id"].(string)
		granted, existe := item["granted"].(bool)
		if !existe {
			t.Fatalf("granted debe ser booleano: %v", item["granted"])
		}
		if esperado, conocido := esperados[id]; !conocido || granted != esperado {
			t.Fatalf("granted de %q debe ser %v, obtenido %v", id, esperado, granted)
		}
		if _, err := time.Parse(time.RFC3339, item["recordedAt"].(string)); err != nil {
			t.Fatalf("recordedAt debe ser date-time RFC3339: %v", item["recordedAt"])
		}
	}

	// Defensa contra fugas: ningún atributo interno puede aparecer serializado.
	for _, prohibido := range []string{"source", "createdAt", "acceptedAt", "PK", "SK", "sortKey"} {
		if strings.Contains(rec.Body.String(), "\""+prohibido+"\"") {
			t.Fatalf("la respuesta no debe contener %q: %s", prohibido, rec.Body.String())
		}
	}
}

// TestE016HTTPPaginacion: la primera página emite `nextCursor` y la segunda lo usa.
func TestE016HTTPPaginacion(t *testing.T) {
	store := newE016HTTPStore()
	registroA := e016HTTPRegistro(e004HTTPSesion, appPrivacy.ConsentPurposeNotifications, true)
	registroB := e016HTTPRegistro(e004HTTPSesionB, appPrivacy.ConsentPurposeAnalytics, false)
	store.paginas[e004HTTPUsuario+"\x00"] = e016HTTPPagina{
		registros: []appPrivacy.ConsentRecord{registroA},
		nextSK:    registroA.SortKey,
	}
	store.paginas[e004HTTPUsuario+"\x00"+registroA.SortKey] = e016HTTPPagina{
		registros: []appPrivacy.ConsentRecord{registroB},
	}
	handler := e016HTTPHandler(store)

	primera := httptest.NewRecorder()
	handler.ServeHTTP(primera, e016HTTPPeticion("jwt-valido", "limit=1"))
	if primera.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d (%s)", primera.Code, primera.Body.String())
	}
	var pagina1 map[string]any
	if err := json.Unmarshal(primera.Body.Bytes(), &pagina1); err != nil {
		t.Fatalf("respuesta no es JSON: %v", err)
	}
	cursor, _ := pagina1["nextCursor"].(string)
	if cursor == "" {
		t.Fatal("con mas filas debe emitirse nextCursor")
	}

	segunda := httptest.NewRecorder()
	handler.ServeHTTP(segunda, e016HTTPPeticion("jwt-valido", "limit=1&cursor="+cursor))
	if segunda.Code != http.StatusOK {
		t.Fatalf("esperado 200 en la segunda pagina, obtenido %d (%s)", segunda.Code, segunda.Body.String())
	}
	var pagina2 map[string]any
	if err := json.Unmarshal(segunda.Body.Bytes(), &pagina2); err != nil {
		t.Fatalf("respuesta no es JSON: %v", err)
	}
	if _, existe := pagina2["nextCursor"]; existe {
		t.Fatalf("la ultima pagina no debe emitir nextCursor: %v", pagina2)
	}
	items, _ := pagina2["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("la segunda pagina debe traer un consentimiento, trae %d", len(items))
	}
	if len(store.startSKs) != 2 || store.startSKs[1] != registroA.SortKey {
		t.Fatalf("el cursor debe traducirse en ExclusiveStartKey: %+v", store.startSKs)
	}
}

// TestE016HTTPCursorInvalido: longitud fuera de rango, cursor no emitido por el servidor o
// manipulado -> 400 INVALID_REQUEST.
func TestE016HTTPCursorInvalido(t *testing.T) {
	casos := []struct {
		nombre string
		query  string
	}{
		{"demasiado corto", "cursor=abc"},
		{"demasiado largo", "cursor=" + strings.Repeat("A", 3000)},
		{"no emitido por el servidor", "cursor=abcdefghij"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			e016HTTPHandler(newE016HTTPStore()).ServeHTTP(rec, e016HTTPPeticion("jwt-valido", caso.query))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("esperado 400, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "INVALID_REQUEST") {
				t.Fatalf("debe devolver INVALID_REQUEST: %s", rec.Body.String())
			}
		})
	}
}

// TestE016HTTPCursorConOtroLimite: reutilizar un cursor emitido con otro `limit` es 400.
func TestE016HTTPCursorConOtroLimite(t *testing.T) {
	store := newE016HTTPStore()
	registro := e016HTTPRegistro(e004HTTPSesion, appPrivacy.ConsentPurposeNotifications, true)
	store.paginas[e004HTTPUsuario+"\x00"] = e016HTTPPagina{
		registros: []appPrivacy.ConsentRecord{registro},
		nextSK:    registro.SortKey,
	}
	handler := e016HTTPHandler(store)

	primera := httptest.NewRecorder()
	handler.ServeHTTP(primera, e016HTTPPeticion("jwt-valido", "limit=1"))
	if primera.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d (%s)", primera.Code, primera.Body.String())
	}
	var pagina map[string]any
	if err := json.Unmarshal(primera.Body.Bytes(), &pagina); err != nil {
		t.Fatalf("respuesta no es JSON: %v", err)
	}
	cursor, _ := pagina["nextCursor"].(string)
	if cursor == "" {
		t.Fatal("se esperaba cursor")
	}

	segunda := httptest.NewRecorder()
	handler.ServeHTTP(segunda, e016HTTPPeticion("jwt-valido", "limit=2&cursor="+cursor))
	if segunda.Code != http.StatusBadRequest {
		t.Fatalf("un cursor con otros filtros debe ser 400, obtenido %d (%s)", segunda.Code, segunda.Body.String())
	}
}

// TestE016HTTPLimiteInvalido: `limit` fuera de 1..100 o no numérico -> 400.
func TestE016HTTPLimiteInvalido(t *testing.T) {
	for _, query := range []string{"limit=0", "limit=101", "limit=-3", "limit=abc", "limit=1.5"} {
		rec := httptest.NewRecorder()
		e016HTTPHandler(newE016HTTPStore()).ServeHTTP(rec, e016HTTPPeticion("jwt-valido", query))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: esperado 400, obtenido %d (%s)", query, rec.Code, rec.Body.String())
		}
	}
}

// TestE016HTTPAutenticacion: sin Bearer, esquema inválido, token desconocido y expirado -> 401
// sin listar nada.
func TestE016HTTPAutenticacion(t *testing.T) {
	casos := []struct {
		nombre   string
		cabecera string
	}{
		{"sin cabecera", ""},
		{"esquema basico", "Basic abc"},
		{"token desconocido", "Bearer desconocido"},
		{"token expirado", "Bearer jwt-expirado"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := newE016HTTPStore()
			req := e016HTTPPeticion("", "")
			if caso.cabecera != "" {
				req.Header.Set("Authorization", caso.cabecera)
			}
			rec := httptest.NewRecorder()
			e016HTTPHandler(store).ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("esperado 401, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			if len(store.usuarios) != 0 {
				t.Fatalf("una peticion no autenticada no debe consultar la persistencia: %+v", store.usuarios)
			}
		})
	}
}

// TestE016HTTPSesionRevocada: una sesión revocada no llega al handler (401) ni consulta la
// persistencia.
func TestE016HTTPSesionRevocada(t *testing.T) {
	store := newE016HTTPStore()
	sesiones := newE004Sesiones()
	sesiones.revocar(e004HTTPSesion)
	servicio := appPrivacy.NewConsentsService(store, []byte(e016ClaveCursores))
	handler := internalhttp.NewConsentsHandler(servicio, e004TokensValidos(), sesiones, silencioso())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, e016HTTPPeticion("jwt-valido", ""))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("esperado 401, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if len(store.usuarios) != 0 {
		t.Fatalf("una sesion revocada no debe consultar la persistencia: %+v", store.usuarios)
	}
}

// TestE016HTTP503SinPersistencia: sin servicio no hay listado, nunca una lista vacía.
func TestE016HTTP503SinPersistencia(t *testing.T) {
	rec := httptest.NewRecorder()
	e016HTTPHandler(nil).ServeHTTP(rec, e016HTTPPeticion("jwt-valido", ""))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("esperado 503, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "DEPENDENCY_UNAVAILABLE") {
		t.Fatalf("debe devolver DEPENDENCY_UNAVAILABLE: %s", rec.Body.String())
	}
}

// TestE016HTTP503StoreCaido: un fallo de persistencia se traduce en 503.
func TestE016HTTP503StoreCaido(t *testing.T) {
	store := newE016HTTPStore()
	store.fallo = errors.New("dynamodb inaccesible")
	rec := httptest.NewRecorder()
	e016HTTPHandler(store).ServeHTTP(rec, e016HTTPPeticion("jwt-valido", ""))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("esperado 503, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
}

// TestE016HTTPAislamientoEntreUsuarios: la identidad sale de `sub`, de modo que cada token
// consulta exclusivamente su propia partición.
func TestE016HTTPAislamientoEntreUsuarios(t *testing.T) {
	store := newE016HTTPStore()
	store.paginas[e004HTTPUsuario+"\x00"] = e016HTTPPagina{
		registros: []appPrivacy.ConsentRecord{e016HTTPRegistro(e004HTTPSesion, appPrivacy.ConsentPurposeNotifications, true)},
	}
	store.paginas[e004HTTPOtroUser+"\x00"] = e016HTTPPagina{
		registros: []appPrivacy.ConsentRecord{e016HTTPRegistro(e004HTTPSesionB, appPrivacy.ConsentPurposeAnalytics, false)},
	}
	handler := e016HTTPHandler(store)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, e016HTTPPeticion("jwt-sujeto-ajeno", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if len(store.usuarios) != 1 || store.usuarios[0] != e004HTTPOtroUser {
		t.Fatalf("la persistencia debe recibir el sub del token: %+v", store.usuarios)
	}
	if !strings.Contains(rec.Body.String(), e004HTTPSesionB) {
		t.Fatalf("debe devolverse el consentimiento del titular: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), e004HTTPSesion) {
		t.Fatalf("no debe filtrarse un consentimiento de otro usuario: %s", rec.Body.String())
	}
}

// TestE016HTTPListaVacia: sin consentimientos se devuelve `items: []`, nunca `null`.
func TestE016HTTPListaVacia(t *testing.T) {
	rec := httptest.NewRecorder()
	e016HTTPHandler(newE016HTTPStore()).ServeHTTP(rec, e016HTTPPeticion("jwt-valido", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	var cuerpo map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("respuesta no es JSON: %v", err)
	}
	items, ok := cuerpo["items"].([]any)
	if !ok {
		t.Fatalf("items debe ser un array vacio, obtenido %v", cuerpo["items"])
	}
	if len(items) != 0 {
		t.Fatalf("items debe estar vacio, hay %d", len(items))
	}
}

// TestE016HTTPMetodoNoAdmitido: el handler de listado sólo sirve GET.
func TestE016HTTPMetodoNoAdmitido(t *testing.T) {
	rec := httptest.NewRecorder()
	req := e004Peticion(http.MethodPost, "/api/v1/users/me/consents", "jwt-valido", `{"noticeVersion":"2026-11"}`)
	e016HTTPHandler(newE016HTTPStore()).ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("esperado 405, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if allow := rec.Header().Get("Allow"); allow != http.MethodGet {
		t.Fatalf("Allow debe anunciar GET, obtenido %q", allow)
	}
}

// TestE016HTTP404Subruta: no existe subrecurso por consentimiento; una ruta no registrada
// responde 404 y no se sirve el historial.
func TestE016HTTP404Subruta(t *testing.T) {
	router := internalhttp.NewRouter()
	handler := e016HTTPHandler(newE016HTTPStore())
	router.HandleFunc(http.MethodGet, "/users/me/consents", handler.ServeHTTP)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, e004Peticion(http.MethodGet, "/api/v1/users/me/consents/"+e004HTTPSesion, "jwt-valido", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("esperado 404, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "items") {
		t.Fatalf("una ruta inexistente no debe devolver el historial: %s", rec.Body.String())
	}
}
