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

	appPreferences "github.com/nexumpr/Nexum-Backend/internal/application/preferences"
	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// Pruebas HTTP de E068 (`GET /api/v1/users/me/preferences`): middleware de autenticación,
// contrato `UserPreferences` con exactamente cuatro campos (incluido el alias A8 `timeZone`),
// materialización de defaults, aislamiento entre usuarios y fail-closed. Los dobles son
// exclusivamente de prueba; la composición productiva usa DynamoDB real.

var e068HTTPAhora = time.Date(2026, 11, 18, 11, 0, 0, 0, time.UTC)

// e068HTTPStore es el puerto de preferencias en memoria de prueba.
type e068HTTPStore struct {
	mu         sync.Mutex
	items      map[string]appPreferences.Preferences
	usuarios   []string
	creaciones int
	fallo      error
}

func newE068HTTPStore() *e068HTTPStore {
	return &e068HTTPStore{items: map[string]appPreferences.Preferences{}}
}

func (s *e068HTTPStore) LoadPreferences(_ context.Context, userID string) (*appPreferences.Preferences, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.usuarios = append(s.usuarios, userID)
	if s.fallo != nil {
		return nil, s.fallo
	}
	item, ok := s.items[userID]
	if !ok {
		return nil, nil
	}
	copia := item
	return &copia, nil
}

func (s *e068HTTPStore) CreatePreferences(_ context.Context, userID string, defaults appPreferences.Preferences, _ time.Time) (*appPreferences.Preferences, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.creaciones++
	if s.fallo != nil {
		return nil, s.fallo
	}
	if existente, ok := s.items[userID]; ok {
		copia := existente
		return &copia, nil
	}
	s.items[userID] = defaults
	copia := defaults
	return &copia, nil
}

// e068HTTPHandler compone el handler con el servicio real de aplicación y el middleware real.
func e068HTTPHandler(store appPreferences.Store) *internalhttp.PreferencesHandler {
	var servicio *appPreferences.Service
	if store != nil {
		servicio = appPreferences.NewService(store).WithClock(func() time.Time { return e068HTTPAhora })
	}
	return internalhttp.NewPreferencesHandler(servicio, e004TokensValidos(), newE004Sesiones(), silencioso())
}

// e068HTTPPeticion construye la petición GET autenticada con el token indicado.
func e068HTTPPeticion(token string) *http.Request {
	return e004Peticion(http.MethodGet, "/api/v1/users/me/preferences", token, "")
}

// TestE068HTTPEsquemaExacto cubre T01: 200 con `UserPreferences` y exactamente los cuatro
// campos del contrato, con `timeZone` como alias exacto de `timezone`.
func TestE068HTTPEsquemaExacto(t *testing.T) {
	store := newE068HTTPStore()
	store.items[e004HTTPUsuario] = appPreferences.Preferences{
		Locale:     "es-MX",
		Timezone:   "America/Mexico_City",
		Version:    3,
		HasVersion: true,
	}

	rec := httptest.NewRecorder()
	e068HTTPHandler(store).ServeHTTP(rec, e068HTTPPeticion("jwt-valido"))

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if cache := rec.Header().Get("Cache-Control"); cache != "private, no-store" {
		t.Fatalf("Cache-Control debe ser 'private, no-store', obtenido %q", cache)
	}

	var cuerpo map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("respuesta no es JSON: %v", err)
	}
	if len(cuerpo) != 4 {
		t.Fatalf("UserPreferences es un objeto cerrado de 4 campos, se obtuvo %v", cuerpo)
	}
	esperados := map[string]any{
		"locale":   "es-MX",
		"timezone": "America/Mexico_City",
		"timeZone": "America/Mexico_City",
		"version":  float64(3),
	}
	for campo, esperado := range esperados {
		if obtenido, ok := cuerpo[campo]; !ok || obtenido != esperado {
			t.Fatalf("%s esperado %v, obtenido %v (cuerpo %v)", campo, esperado, obtenido, cuerpo)
		}
	}
	for _, prohibido := range []string{"userId", "user_id", "email", "refreshHash", "PK", "SK"} {
		if _, ok := cuerpo[prohibido]; ok {
			t.Fatalf("la respuesta no puede exponer %q", prohibido)
		}
	}
}

// TestE068HTTPMaterializaDefaults: un titular sin item recibe los valores del contrato y la
// identidad usada es la del token verificado.
func TestE068HTTPMaterializaDefaults(t *testing.T) {
	store := newE068HTTPStore()

	rec := httptest.NewRecorder()
	e068HTTPHandler(store).ServeHTTP(rec, e068HTTPPeticion("jwt-valido"))

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	var cuerpo map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("respuesta no es JSON: %v", err)
	}
	if cuerpo["locale"] != "es-MX" || cuerpo["timezone"] != "America/Mexico_City" || cuerpo["timeZone"] != "America/Mexico_City" {
		t.Fatalf("defaults inesperados: %v", cuerpo)
	}
	if cuerpo["version"] != float64(1) {
		t.Fatalf("la version inicial debe ser 1: %v", cuerpo["version"])
	}
	if store.creaciones != 1 {
		t.Fatalf("debe materializarse una sola vez, creaciones=%d", store.creaciones)
	}
	if len(store.usuarios) == 0 || store.usuarios[0] != e004HTTPUsuario {
		t.Fatalf("la persistencia debe recibir el sub del token: %v", store.usuarios)
	}
}

// TestE068HTTPAutenticacion cubre T02/401: sin Bearer, esquema distinto, token desconocido o
// expirado no se sirve ninguna preferencia.
func TestE068HTTPAutenticacion(t *testing.T) {
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
			store := newE068HTTPStore()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me/preferences", strings.NewReader(""))
			if caso.cabecera != "" {
				req.Header.Set("Authorization", caso.cabecera)
			}
			rec := httptest.NewRecorder()
			e068HTTPHandler(store).ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("esperado 401, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			if store.creaciones != 0 || len(store.usuarios) != 0 {
				t.Fatal("una peticion no autenticada no puede tocar la persistencia")
			}
		})
	}
}

// TestE068HTTPFallaCerrado: sin servicio, sin tokens o sin autorizador de sesiones el endpoint
// responde 503 y no se sirven valores efímeros.
func TestE068HTTPFallaCerrado(t *testing.T) {
	store := newE068HTTPStore()

	casos := []struct {
		nombre  string
		handler *internalhttp.PreferencesHandler
	}{
		{"sin servicio", internalhttp.NewPreferencesHandler(nil, e004TokensValidos(), newE004Sesiones(), silencioso())},
		{"sin tokens", internalhttp.NewPreferencesHandler(appPreferences.NewService(store), nil, newE004Sesiones(), silencioso())},
		{"sin autorizador", internalhttp.NewPreferencesHandler(appPreferences.NewService(store), e004TokensValidos(), nil, silencioso())},
		{"sin almacen", internalhttp.NewPreferencesHandler(appPreferences.NewService(nil), e004TokensValidos(), newE004Sesiones(), silencioso())},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			caso.handler.ServeHTTP(rec, e068HTTPPeticion("jwt-valido"))
			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("esperado 503, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			if cache := rec.Header().Get("Cache-Control"); cache != "private, no-store" {
				t.Fatalf("tambien en 503 la respuesta es privada, obtenido %q", cache)
			}
		})
	}
}

// TestE068HTTPAislamiento cubre T03: el titular B no recibe las preferencias de A.
func TestE068HTTPAislamiento(t *testing.T) {
	store := newE068HTTPStore()
	store.items[e004HTTPUsuario] = appPreferences.Preferences{
		Locale:     "en-US",
		Timezone:   "America/New_York",
		Version:    9,
		HasVersion: true,
	}

	rec := httptest.NewRecorder()
	e068HTTPHandler(store).ServeHTTP(rec, e004Peticion(http.MethodGet, "/api/v1/users/me/preferences", "jwt-sujeto-ajeno", ""))

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	var cuerpo map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("respuesta no es JSON: %v", err)
	}
	if cuerpo["timezone"] == "America/New_York" || cuerpo["locale"] == "en-US" {
		t.Fatalf("el usuario B recibio preferencias de A: %v", cuerpo)
	}
	if store.items[e004HTTPUsuario].Locale != "en-US" {
		t.Fatal("la peticion de B no puede modificar el item de A")
	}
}

// TestE068HTTPFalloDeDependencia: una caída de DynamoDB produce el sobre canónico 503 sin
// filtrar detalles internos.
func TestE068HTTPFalloDeDependencia(t *testing.T) {
	store := newE068HTTPStore()
	store.fallo = errors.New("dynamo no disponible")

	rec := httptest.NewRecorder()
	e068HTTPHandler(store).ServeHTTP(rec, e068HTTPPeticion("jwt-valido"))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("esperado 503, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if strings.Contains(strings.ToLower(rec.Body.String()), "dynamo") {
		t.Fatalf("la respuesta no debe revelar infraestructura: %s", rec.Body.String())
	}
	var sobre struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			TraceID string `json:"traceId"`
			Details []any  `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &sobre); err != nil {
		t.Fatalf("el error debe ser un ErrorEnvelope: %v", err)
	}
	if sobre.Error.Code != "DEPENDENCY_UNAVAILABLE" {
		t.Fatalf("codigo esperado DEPENDENCY_UNAVAILABLE, obtenido %q", sobre.Error.Code)
	}
	if sobre.Error.Details == nil {
		t.Fatal("details debe ser un array no nulo")
	}
}
