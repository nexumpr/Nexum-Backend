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

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// Pruebas HTTP de E008 (`GET /api/v1/auth/sessions`): middleware de autenticación, contrato
// `PageOfSession` con EXACTAMENTE cinco campos por sesión, paginación por cursor, rechazo de
// cursor inválido y fail-closed sin persistencia. Los dobles son exclusivamente de prueba; la
// composición productiva usa DynamoDB real.

var e008HTTPAhora = time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)

const e008ClaveCursores = "clave-de-prueba-para-cursores-http"

type e008HTTPPagina struct {
	registros []appIdentity.SessionListRecord
	nextSK    string
}

type e008HTTPStore struct {
	mu       sync.Mutex
	paginas  map[string]e008HTTPPagina
	usuarios []string
	startSKs []string
	fallo    error
}

func newE008HTTPStore() *e008HTTPStore {
	return &e008HTTPStore{paginas: map[string]e008HTTPPagina{}}
}

func (s *e008HTTPStore) ListUserSessions(_ context.Context, userID, startSK string, _ int) ([]appIdentity.SessionListRecord, string, error) {
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

func e008HTTPRegistro(sessionID, status string) appIdentity.SessionListRecord {
	return appIdentity.SessionListRecord{
		SortKey:    "SESSION#" + sessionID,
		SessionID:  sessionID,
		CreatedAt:  e008HTTPAhora.Add(-2 * time.Hour),
		LastSeenAt: e008HTTPAhora.Add(-time.Hour),
		Status:     status,
	}
}

func e008HTTPHandler(store appIdentity.SessionsStore) *internalhttp.SessionsHandler {
	var servicio *appIdentity.SessionsService
	if store != nil {
		servicio = appIdentity.NewSessionsService(store, []byte(e008ClaveCursores)).
			WithClock(func() time.Time { return e008HTTPAhora })
	}
	return internalhttp.NewSessionsHandler(servicio, e004TokensValidos(), newE004Sesiones(), nil, silencioso())
}

// e008HTTPPeticion construye la petición GET con la sesión y el usuario del token indicado.
func e008HTTPPeticion(token, query string) *http.Request {
	ruta := "/api/v1/auth/sessions"
	if query != "" {
		ruta += "?" + query
	}
	return e004Peticion(http.MethodGet, ruta, token, "")
}

// TestE008HTTPEsquemaExacto cubre T01: 200 con `PageOfSession`, exactamente cinco campos por
// sesión y ningún atributo interno en el cuerpo.
func TestE008HTTPEsquemaExacto(t *testing.T) {
	store := newE008HTTPStore()
	store.paginas[e004HTTPUsuario+"\x00"] = e008HTTPPagina{registros: []appIdentity.SessionListRecord{
		e008HTTPRegistro(e004HTTPSesion, appIdentity.StatusActive),
		e008HTTPRegistro(e004HTTPSesionB, appIdentity.StatusRevoked),
	}}

	rec := httptest.NewRecorder()
	e008HTTPHandler(store).ServeHTTP(rec, e008HTTPPeticion("jwt-valido", ""))

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
			t.Fatalf("PageOfSession no admite la propiedad %q", clave)
		}
	}
	items, ok := cuerpo["items"].([]any)
	if !ok {
		t.Fatalf("items debe ser un array: %v", cuerpo["items"])
	}
	if len(items) != 2 {
		t.Fatalf("se esperaban 2 sesiones, hay %d", len(items))
	}

	esperados := map[string]struct{ actual, revocado bool }{
		e004HTTPSesion:  {actual: true, revocado: false},
		e004HTTPSesionB: {actual: false, revocado: true},
	}
	for _, entrada := range items {
		item, ok := entrada.(map[string]any)
		if !ok {
			t.Fatalf("cada sesion debe ser un objeto: %v", entrada)
		}
		claves := make(map[string]bool, len(item))
		for clave := range item {
			claves[clave] = true
		}
		for _, requerida := range []string{"id", "createdAt", "lastSeenAt", "current", "revoked"} {
			if !claves[requerida] {
				t.Fatalf("falta el campo requerido %q en %v", requerida, item)
			}
		}
		if len(claves) != 5 {
			t.Fatalf("Session es un objeto cerrado de 5 campos, se obtuvo %v", item)
		}
		id, _ := item["id"].(string)
		esperado, conocido := esperados[id]
		if !conocido {
			t.Fatalf("id inesperado en la respuesta: %q", id)
		}
		if actual, _ := item["current"].(bool); actual != esperado.actual {
			t.Fatalf("current de %q debe ser %v", id, esperado.actual)
		}
		if revocado, _ := item["revoked"].(bool); revocado != esperado.revocado {
			t.Fatalf("revoked de %q debe ser %v", id, esperado.revocado)
		}
		if _, err := time.Parse(time.RFC3339, item["createdAt"].(string)); err != nil {
			t.Fatalf("createdAt debe ser date-time RFC3339: %v", item["createdAt"])
		}
		if _, err := time.Parse(time.RFC3339, item["lastSeenAt"].(string)); err != nil {
			t.Fatalf("lastSeenAt debe ser date-time RFC3339: %v", item["lastSeenAt"])
		}
	}

	// Defensa contra fugas: ningún atributo interno puede aparecer serializado.
	for _, prohibido := range []string{"refreshHash", "accessJti", "familyId", "deviceId", "deviceName", "platform", "expiresAt", "sessionEpoch"} {
		if strings.Contains(rec.Body.String(), prohibido) {
			t.Fatalf("la respuesta no debe contener %q: %s", prohibido, rec.Body.String())
		}
	}
}

// TestE008HTTPPaginacion cubre T06: la primera página emite `nextCursor` y la segunda lo usa.
func TestE008HTTPPaginacion(t *testing.T) {
	store := newE008HTTPStore()
	store.paginas[e004HTTPUsuario+"\x00"] = e008HTTPPagina{
		registros: []appIdentity.SessionListRecord{e008HTTPRegistro(e004HTTPSesion, appIdentity.StatusActive)},
		nextSK:    "SESSION#" + e004HTTPSesion,
	}
	store.paginas[e004HTTPUsuario+"\x00SESSION#"+e004HTTPSesion] = e008HTTPPagina{
		registros: []appIdentity.SessionListRecord{e008HTTPRegistro(e004HTTPSesionB, appIdentity.StatusActive)},
	}
	handler := e008HTTPHandler(store)

	primera := httptest.NewRecorder()
	handler.ServeHTTP(primera, e008HTTPPeticion("jwt-valido", "limit=1"))
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
	handler.ServeHTTP(segunda, e008HTTPPeticion("jwt-valido", "limit=1&cursor="+cursor))
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
		t.Fatalf("la segunda pagina debe traer una sesion, trae %d", len(items))
	}
	if len(store.startSKs) != 2 || store.startSKs[1] != "SESSION#"+e004HTTPSesion {
		t.Fatalf("el cursor debe traducirse en ExclusiveStartKey: %+v", store.startSKs)
	}
}

// TestE008HTTPCursorInvalido cubre T02/T06: longitud fuera de rango o cursor no emitido por el
// servidor -> 400 INVALID_REQUEST.
func TestE008HTTPCursorInvalido(t *testing.T) {
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
			e008HTTPHandler(newE008HTTPStore()).ServeHTTP(rec, e008HTTPPeticion("jwt-valido", caso.query))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("esperado 400, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "INVALID_REQUEST") {
				t.Fatalf("debe devolver INVALID_REQUEST: %s", rec.Body.String())
			}
		})
	}
}

// TestE008HTTPLimiteInvalido: `limit` fuera de 1..100 o no numérico -> 400.
func TestE008HTTPLimiteInvalido(t *testing.T) {
	for _, query := range []string{"limit=0", "limit=101", "limit=-3", "limit=abc", "limit=1.5"} {
		rec := httptest.NewRecorder()
		e008HTTPHandler(newE008HTTPStore()).ServeHTTP(rec, e008HTTPPeticion("jwt-valido", query))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: esperado 400, obtenido %d (%s)", query, rec.Code, rec.Body.String())
		}
	}
}

// TestE008HTTPAutenticacion cubre T02: sin Bearer, esquema inválido, token desconocido y
// expirado -> 401 sin listar nada.
func TestE008HTTPAutenticacion(t *testing.T) {
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
			store := newE008HTTPStore()
			req := e008HTTPPeticion("", "")
			if caso.cabecera != "" {
				req.Header.Set("Authorization", caso.cabecera)
			}
			rec := httptest.NewRecorder()
			e008HTTPHandler(store).ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("esperado 401, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			if len(store.usuarios) != 0 {
				t.Fatalf("una peticion no autenticada no debe consultar la persistencia: %+v", store.usuarios)
			}
		})
	}
}

// TestE008HTTP503SinPersistencia cubre T04: sin servicio no hay listado, nunca una lista vacía.
func TestE008HTTP503SinPersistencia(t *testing.T) {
	rec := httptest.NewRecorder()
	e008HTTPHandler(nil).ServeHTTP(rec, e008HTTPPeticion("jwt-valido", ""))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("esperado 503, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "DEPENDENCY_UNAVAILABLE") {
		t.Fatalf("debe devolver DEPENDENCY_UNAVAILABLE: %s", rec.Body.String())
	}
}

// TestE008HTTP503StoreCaido: un fallo de persistencia se traduce en 503.
func TestE008HTTP503StoreCaido(t *testing.T) {
	store := newE008HTTPStore()
	store.fallo = errors.New("dynamodb inaccesible")
	rec := httptest.NewRecorder()
	e008HTTPHandler(store).ServeHTTP(rec, e008HTTPPeticion("jwt-valido", ""))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("esperado 503, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
}

// TestE008HTTPAislamientoEntreUsuarios cubre T03: la identidad sale de `sub`, de modo que cada
// token consulta exclusivamente su propia partición.
func TestE008HTTPAislamientoEntreUsuarios(t *testing.T) {
	store := newE008HTTPStore()
	store.paginas[e004HTTPUsuario+"\x00"] = e008HTTPPagina{
		registros: []appIdentity.SessionListRecord{e008HTTPRegistro(e004HTTPSesion, appIdentity.StatusActive)},
	}
	store.paginas[e004HTTPOtroUser+"\x00"] = e008HTTPPagina{
		registros: []appIdentity.SessionListRecord{e008HTTPRegistro(e004HTTPSesionB, appIdentity.StatusActive)},
	}
	handler := e008HTTPHandler(store)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, e008HTTPPeticion("jwt-sujeto-ajeno", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if len(store.usuarios) != 1 || store.usuarios[0] != e004HTTPOtroUser {
		t.Fatalf("la persistencia debe recibir el sub del token: %+v", store.usuarios)
	}
	if !strings.Contains(rec.Body.String(), e004HTTPSesionB) {
		t.Fatalf("debe devolverse la sesion del titular: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), e004HTTPSesion) {
		t.Fatalf("no debe filtrarse una sesion de otro usuario: %s", rec.Body.String())
	}
}

// TestE008HTTPListaVacia: sin sesiones se devuelve `items: []`, nunca `null`.
func TestE008HTTPListaVacia(t *testing.T) {
	rec := httptest.NewRecorder()
	e008HTTPHandler(newE008HTTPStore()).ServeHTTP(rec, e008HTTPPeticion("jwt-valido", ""))
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
