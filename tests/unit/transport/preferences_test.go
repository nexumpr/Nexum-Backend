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
	mu              sync.Mutex
	items           map[string]appPreferences.Preferences
	usuarios        []string
	creaciones      int
	actualizaciones int
	ultimaUpdate    appPreferences.Update
	fallo           error
	falloUpdate     error
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

// UpdatePreferences replica el compare-and-swap del adaptador real sobre el mapa compartido.
func (s *e068HTTPStore) UpdatePreferences(_ context.Context, userID string, update appPreferences.Update) (*appPreferences.Preferences, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.actualizaciones++
	s.ultimaUpdate = update
	if s.falloUpdate != nil {
		return nil, s.falloUpdate
	}
	item, ok := s.items[userID]
	if !ok {
		return nil, appPreferences.ErrPreferencesConflict
	}
	observada := item.Version
	if !item.HasVersion {
		observada = appPreferences.DefaultVersion
	}
	if observada != update.ExpectedVersion {
		return nil, appPreferences.ErrPreferencesVersionConflict
	}
	item.Locale = update.Locale
	item.Timezone = update.Timezone
	item.Version = observada + 1
	item.HasVersion = true
	s.items[userID] = item
	copia := item
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

// ---------- E069: modificacion de preferencias ----------

// e069HTTPHandler compone el handler de E069 con el servicio real de aplicacion y el middleware
// real de autenticacion y sesion.
func e069HTTPHandler(store appPreferences.UpdateStore) *internalhttp.PreferencesPatchHandler {
	var servicio *appPreferences.Service
	if store != nil {
		servicio = appPreferences.NewService(store).WithClock(func() time.Time { return e068HTTPAhora })
	}
	return internalhttp.NewPreferencesPatchHandler(servicio, e004TokensValidos(), newE004Sesiones(), silencioso())
}

// e069Peticion construye el PATCH autenticado con cuerpo JSON y If-Match opcional.
func e069Peticion(cuerpo, ifMatch string) *http.Request {
	req := e004Peticion(http.MethodPatch, "/api/v1/users/me/preferences", "jwt-valido", cuerpo)
	if cuerpo == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if ifMatch != "" {
		req.Header.Set("If-Match", ifMatch)
	}
	return req
}

// e069SobreError decodifica el ErrorEnvelope canonico de una respuesta de error.
func e069SobreError(t *testing.T, rec *httptest.ResponseRecorder) (codigo string, detalles []struct {
	Field  string `json:"field"`
	Reason string `json:"reason"`
}) {
	t.Helper()
	var sobre struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			TraceID string `json:"traceId"`
			Details []struct {
				Field  string `json:"field"`
				Reason string `json:"reason"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &sobre); err != nil {
		t.Fatalf("la respuesta de error debe ser JSON: %v (%s)", err, rec.Body.String())
	}
	if sobre.Error.Details == nil {
		t.Fatalf("details debe ser un array no nulo: %s", rec.Body.String())
	}
	return sobre.Error.Code, sobre.Error.Details
}

func sembrarPreferenciasHTTP(store *e068HTTPStore, version int64, locale, timezone string) {
	store.items[e004HTTPUsuario] = appPreferences.Preferences{
		Locale:     locale,
		Timezone:   timezone,
		Version:    version,
		HasVersion: true,
	}
}

// TestE069HTTP200ConETagYAliasExacto: PATCH valido devuelve 200, los cuatro campos del contrato
// con `timeZone` como alias exacto y el ETag de la version resultante.
func TestE069HTTP200ConETagYAliasExacto(t *testing.T) {
	store := newE068HTTPStore()
	sembrarPreferenciasHTTP(store, 3, "es-MX", "America/Mexico_City")

	rec := httptest.NewRecorder()
	e069HTTPHandler(store).ServeHTTP(rec, e069Peticion(`{"locale":"en-US"}`, `"v3"`))

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if etag := rec.Header().Get("ETag"); etag != `"v4"` {
		t.Fatalf("ETag esperado \"v4\", obtenido %q", etag)
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
		"locale":   "en-US",
		"timezone": "America/Mexico_City",
		"timeZone": "America/Mexico_City",
		"version":  float64(4),
	}
	for campo, esperado := range esperados {
		if obtenido, ok := cuerpo[campo]; !ok || obtenido != esperado {
			t.Fatalf("%s esperado %v, obtenido %v (cuerpo %v)", campo, esperado, obtenido, cuerpo)
		}
	}
	if store.ultimaUpdate.ExpectedVersion != 3 {
		t.Fatalf("el CAS debe recibir la version de If-Match: %+v", store.ultimaUpdate)
	}
	if store.actualizaciones != 1 || store.creaciones != 0 {
		t.Fatalf("debe hacerse una sola escritura y ninguna creacion: %d/%d", store.actualizaciones, store.creaciones)
	}
}

// TestE069HTTPIfMatchRequerido: sin If-Match es 428 PRECONDITION_REQUIRED y no se toca nada.
func TestE069HTTPIfMatchRequerido(t *testing.T) {
	store := newE068HTTPStore()
	sembrarPreferenciasHTTP(store, 1, "es-MX", "America/Mexico_City")

	rec := httptest.NewRecorder()
	e069HTTPHandler(store).ServeHTTP(rec, e069Peticion(`{"locale":"en-US"}`, ""))

	if rec.Code != http.StatusPreconditionRequired {
		t.Fatalf("esperado 428, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	codigo, _ := e069SobreError(t, rec)
	if codigo != "PRECONDITION_REQUIRED" {
		t.Fatalf("codigo esperado PRECONDITION_REQUIRED, obtenido %q", codigo)
	}
	if store.actualizaciones != 0 || len(store.usuarios) != 0 {
		t.Fatal("sin If-Match no puede leerse ni escribirse la persistencia")
	}
}

// TestE069HTTPIfMatchInvalido: un ETag mal formado es 400 INVALID_REQUEST.
func TestE069HTTPIfMatchInvalido(t *testing.T) {
	for _, valor := range []string{"v3", "abc", `"v0"`, `"vx"`, `W/"nope"`} {
		t.Run(valor, func(t *testing.T) {
			store := newE068HTTPStore()
			sembrarPreferenciasHTTP(store, 1, "es-MX", "America/Mexico_City")

			rec := httptest.NewRecorder()
			e069HTTPHandler(store).ServeHTTP(rec, e069Peticion(`{"locale":"en-US"}`, valor))

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("esperado 400, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			codigo, detalles := e069SobreError(t, rec)
			if codigo != "INVALID_REQUEST" || len(detalles) != 1 || detalles[0].Field != "If-Match" {
				t.Fatalf("sobre inesperado: %q %v", codigo, detalles)
			}
			if store.actualizaciones != 0 {
				t.Fatal("un If-Match invalido no puede escribir")
			}
		})
	}
}

// TestE069HTTPVersionDistinta: If-Match desactualizado es 412 VERSION_CONFLICT y no escribe.
func TestE069HTTPVersionDistinta(t *testing.T) {
	store := newE068HTTPStore()
	sembrarPreferenciasHTTP(store, 5, "es-MX", "America/Mexico_City")

	rec := httptest.NewRecorder()
	e069HTTPHandler(store).ServeHTTP(rec, e069Peticion(`{"locale":"en-US"}`, `"v4"`))

	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("esperado 412, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	codigo, _ := e069SobreError(t, rec)
	if codigo != "VERSION_CONFLICT" {
		t.Fatalf("codigo esperado VERSION_CONFLICT, obtenido %q", codigo)
	}
	if store.actualizaciones != 0 {
		t.Fatal("un 412 no puede escribir")
	}
	if store.items[e004HTTPUsuario].Locale != "es-MX" {
		t.Fatal("un 412 no puede modificar el item")
	}
}

// TestE069HTTPConflictoDeCAS: si el CAS agota reintentos, el cliente recibe 409 CONFLICT.
func TestE069HTTPConflictoDeCAS(t *testing.T) {
	store := newE068HTTPStore()
	sembrarPreferenciasHTTP(store, 1, "es-MX", "America/Mexico_City")
	store.falloUpdate = appPreferences.ErrPreferencesConflict

	rec := httptest.NewRecorder()
	e069HTTPHandler(store).ServeHTTP(rec, e069Peticion(`{"locale":"en-US"}`, `"v1"`))

	if rec.Code != http.StatusConflict {
		t.Fatalf("esperado 409, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	codigo, _ := e069SobreError(t, rec)
	if codigo != "CONFLICT" {
		t.Fatalf("codigo esperado CONFLICT, obtenido %q", codigo)
	}
	if store.items[e004HTTPUsuario].Locale != "es-MX" {
		t.Fatal("un 409 no puede dejar una escritura parcial")
	}
}

// TestE069HTTPAliasEnConflicto: timezone y timeZone con valores distintos son 422; nunca se
// elige uno en silencio ni se consulta la persistencia.
func TestE069HTTPAliasEnConflicto(t *testing.T) {
	store := newE068HTTPStore()
	sembrarPreferenciasHTTP(store, 1, "es-MX", "America/Mexico_City")

	rec := httptest.NewRecorder()
	e069HTTPHandler(store).ServeHTTP(rec, e069Peticion(`{"timezone":"America/Mexico_City","timeZone":"Europe/Madrid"}`, `"v1"`))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("esperado 422, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	codigo, detalles := e069SobreError(t, rec)
	if codigo != "BUSINESS_RULE_VIOLATION" {
		t.Fatalf("codigo esperado BUSINESS_RULE_VIOLATION, obtenido %q", codigo)
	}
	if len(detalles) != 1 || detalles[0].Field != "timeZone" || detalles[0].Reason != "timezone_alias_mismatch" {
		t.Fatalf("detalle inesperado: %v", detalles)
	}
	if store.actualizaciones != 0 || len(store.usuarios) != 0 {
		t.Fatal("un alias en conflicto no puede tocar la persistencia")
	}
}

// TestE069HTTPAliasAceptado: el alias `timeZone` es equivalente a `timezone`, tanto solo como
// repetido con el mismo valor.
func TestE069HTTPAliasAceptado(t *testing.T) {
	casos := []struct {
		nombre string
		cuerpo string
	}{
		{"solo alias", `{"timeZone":"Europe/Madrid"}`},
		{"ambos iguales", `{"timezone":"Europe/Madrid","timeZone":"Europe/Madrid"}`},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := newE068HTTPStore()
			sembrarPreferenciasHTTP(store, 1, "es-MX", "America/Mexico_City")

			rec := httptest.NewRecorder()
			e069HTTPHandler(store).ServeHTTP(rec, e069Peticion(caso.cuerpo, `"v1"`))

			if rec.Code != http.StatusOK {
				t.Fatalf("esperado 200, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			var cuerpo map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
				t.Fatalf("respuesta no es JSON: %v", err)
			}
			if cuerpo["timezone"] != "Europe/Madrid" || cuerpo["timeZone"] != "Europe/Madrid" {
				t.Fatalf("el alias debe aplicarse a los dos campos: %v", cuerpo)
			}
			if cuerpo["locale"] != "es-MX" {
				t.Fatalf("el idioma debe conservarse: %v", cuerpo)
			}
			if cuerpo["version"] != float64(2) {
				t.Fatalf("la version debe incrementarse: %v", cuerpo)
			}
		})
	}
}

// TestE069HTTPIANAInvalida: una zona que no existe en la base IANA es 422.
func TestE069HTTPIANAInvalida(t *testing.T) {
	for _, cuerpo := range []string{
		`{"timezone":"Marte/Olympus"}`,
		`{"timeZone":"Marte/Olympus"}`,
		`{"timezone":"Local"}`,
		`{"timezone":""}`,
	} {
		t.Run(cuerpo, func(t *testing.T) {
			store := newE068HTTPStore()
			sembrarPreferenciasHTTP(store, 1, "es-MX", "America/Mexico_City")

			rec := httptest.NewRecorder()
			e069HTTPHandler(store).ServeHTTP(rec, e069Peticion(cuerpo, `"v1"`))

			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("esperado 422, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			codigo, detalles := e069SobreError(t, rec)
			if codigo != "BUSINESS_RULE_VIOLATION" || len(detalles) != 1 || detalles[0].Field != "timezone" || detalles[0].Reason != "invalid_timezone" {
				t.Fatalf("sobre inesperado: %q %v", codigo, detalles)
			}
			if store.actualizaciones != 0 {
				t.Fatal("una zona invalida no puede escribir")
			}
		})
	}
}

// TestE069HTTPLocaleInvalido: el idioma esta limitado al enum del contrato (422).
func TestE069HTTPLocaleInvalido(t *testing.T) {
	store := newE068HTTPStore()
	sembrarPreferenciasHTTP(store, 1, "es-MX", "America/Mexico_City")

	rec := httptest.NewRecorder()
	e069HTTPHandler(store).ServeHTTP(rec, e069Peticion(`{"locale":"fr-FR"}`, `"v1"`))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("esperado 422, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	codigo, detalles := e069SobreError(t, rec)
	if codigo != "BUSINESS_RULE_VIOLATION" || len(detalles) != 1 || detalles[0].Field != "locale" || detalles[0].Reason != "unsupported_locale" {
		t.Fatalf("sobre inesperado: %q %v", codigo, detalles)
	}
	if store.actualizaciones != 0 {
		t.Fatal("un idioma invalido no puede escribir")
	}
}

// TestE069HTTPPatchVacio: `{}` con la version vigente es 200 sin cambio ni escritura.
func TestE069HTTPPatchVacio(t *testing.T) {
	store := newE068HTTPStore()
	sembrarPreferenciasHTTP(store, 2, "es-MX", "America/Mexico_City")

	rec := httptest.NewRecorder()
	e069HTTPHandler(store).ServeHTTP(rec, e069Peticion(`{}`, `"v2"`))

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	var cuerpo map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("respuesta no es JSON: %v", err)
	}
	if cuerpo["version"] != float64(2) || cuerpo["locale"] != "es-MX" || cuerpo["timezone"] != "America/Mexico_City" {
		t.Fatalf("un PATCH vacio debe devolver el estado vigente: %v", cuerpo)
	}
	if etag := rec.Header().Get("ETag"); etag != `"v2"` {
		t.Fatalf("ETag esperado \"v2\", obtenido %q", etag)
	}
	if store.actualizaciones != 0 {
		t.Fatalf("un PATCH vacio no debe escribir, escrituras=%d", store.actualizaciones)
	}
}

// TestE069HTTPMaterializaDefaults: una cuenta sin item admite If-Match "v1" y el cambio se
// aplica sobre los defaults persistidos.
func TestE069HTTPMaterializaDefaults(t *testing.T) {
	store := newE068HTTPStore()

	rec := httptest.NewRecorder()
	e069HTTPHandler(store).ServeHTTP(rec, e069Peticion(`{"locale":"en-US"}`, `"v1"`))

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	var cuerpo map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("respuesta no es JSON: %v", err)
	}
	if cuerpo["locale"] != "en-US" || cuerpo["timezone"] != "America/Mexico_City" || cuerpo["version"] != float64(2) {
		t.Fatalf("resultado inesperado: %v", cuerpo)
	}
	if store.creaciones != 1 || store.actualizaciones != 1 {
		t.Fatalf("debe materializarse y actualizarse una vez: %d/%d", store.creaciones, store.actualizaciones)
	}
	if store.ultimaUpdate.ExpectedVersion != 1 {
		t.Fatalf("el CAS debe comparar contra la version materializada: %+v", store.ultimaUpdate)
	}
}

// TestE069HTTPCuerpoInvalido: JSON roto, campo desconocido, cuerpo vacio o Content-Type no JSON
// producen 400 sin llegar al caso de uso.
func TestE069HTTPCuerpoInvalido(t *testing.T) {
	casos := []struct {
		nombre      string
		cuerpo      string
		contentType string
	}{
		{"json roto", `{"locale":`, "application/json"},
		{"campo desconocido", `{"pais":"MX"}`, "application/json"},
		{"json concatenado", `{} {}`, "application/json"},
		{"cuerpo vacio", ``, "application/json"},
		{"content type no json", `{"locale":"en-US"}`, "text/plain"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := newE068HTTPStore()
			sembrarPreferenciasHTTP(store, 1, "es-MX", "America/Mexico_City")

			req := e069Peticion(caso.cuerpo, `"v1"`)
			req.Header.Set("Content-Type", caso.contentType)
			rec := httptest.NewRecorder()
			e069HTTPHandler(store).ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("esperado 400, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			codigo, _ := e069SobreError(t, rec)
			if codigo != "INVALID_REQUEST" {
				t.Fatalf("codigo esperado INVALID_REQUEST, obtenido %q", codigo)
			}
			if store.actualizaciones != 0 || len(store.usuarios) != 0 {
				t.Fatal("un cuerpo invalido no puede tocar la persistencia")
			}
		})
	}
}

// TestE069HTTPAutenticacion: sin Bearer, con otro esquema, token desconocido o expirado no se
// sirve ni se escribe ninguna preferencia.
func TestE069HTTPAutenticacion(t *testing.T) {
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
			req := e069Peticion(`{"locale":"en-US"}`, `"v1"`)
			req.Header.Del("Authorization")
			if caso.cabecera != "" {
				req.Header.Set("Authorization", caso.cabecera)
			}
			rec := httptest.NewRecorder()
			e069HTTPHandler(store).ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("esperado 401, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			if store.actualizaciones != 0 || len(store.usuarios) != 0 {
				t.Fatal("una peticion no autenticada no puede tocar la persistencia")
			}
		})
	}
}

// TestE069HTTPFallaCerrado: sin servicio, sin tokens, sin autorizador o sin almacen el endpoint
// responde 503 y no escribe.
func TestE069HTTPFallaCerrado(t *testing.T) {
	store := newE068HTTPStore()
	casos := []struct {
		nombre  string
		handler *internalhttp.PreferencesPatchHandler
	}{
		{"sin servicio", internalhttp.NewPreferencesPatchHandler(nil, e004TokensValidos(), newE004Sesiones(), silencioso())},
		{"sin tokens", internalhttp.NewPreferencesPatchHandler(appPreferences.NewService(store), nil, newE004Sesiones(), silencioso())},
		{"sin autorizador", internalhttp.NewPreferencesPatchHandler(appPreferences.NewService(store), e004TokensValidos(), nil, silencioso())},
		{"sin almacen", internalhttp.NewPreferencesPatchHandler(appPreferences.NewService(nil), e004TokensValidos(), newE004Sesiones(), silencioso())},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			caso.handler.ServeHTTP(rec, e069Peticion(`{"locale":"en-US"}`, `"v1"`))
			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("esperado 503, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			if cache := rec.Header().Get("Cache-Control"); cache != "private, no-store" {
				t.Fatalf("tambien en 503 la respuesta es privada, obtenido %q", cache)
			}
			if store.actualizaciones != 0 {
				t.Fatal("un 503 no puede escribir")
			}
		})
	}
}

// TestE069HTTPAislamiento: el titular B no puede modificar ni observar las preferencias de A.
func TestE069HTTPAislamiento(t *testing.T) {
	store := newE068HTTPStore()
	sembrarPreferenciasHTTP(store, 1, "en-US", "America/New_York")

	req := e004Peticion(http.MethodPatch, "/api/v1/users/me/preferences", "jwt-sujeto-ajeno", `{"locale":"es-MX"}`)
	req.Header.Set("If-Match", `"v1"`)
	rec := httptest.NewRecorder()
	e069HTTPHandler(store).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	var cuerpo map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("respuesta no es JSON: %v", err)
	}
	if cuerpo["locale"] != "es-MX" || cuerpo["timezone"] != "America/Mexico_City" {
		t.Fatalf("B debe recibir sus propios defaults: %v", cuerpo)
	}
	if store.items[e004HTTPUsuario].Locale != "en-US" || store.items[e004HTTPUsuario].Timezone != "America/New_York" {
		t.Fatal("la peticion de B no puede modificar el item de A")
	}
	if store.items[e004HTTPUsuario].Version != 1 {
		t.Fatal("la version de A no puede cambiar")
	}
}
