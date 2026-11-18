package transport_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	appOnboarding "github.com/nexumpr/Nexum-Backend/internal/application/onboarding"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// Pruebas HTTP de E019 (`PATCH /api/v1/users/me/onboarding`): DTO estricto con
// `completedSteps` obligatorio, booleanos opcionales con punteros, contrato
// `Onboarding` completo, normalización con 422, conflicto de versión con 409 y
// fail-closed 503. Los dobles son exclusivamente de prueba.

const e019RutaOnboarding = "/api/v1/users/me/onboarding"

var e019HTTPAhora = time.Date(2026, 11, 18, 20, 0, 0, 0, time.UTC)

// e019HTTPAlmacen es `Store` + `Writer` en memoria con CAS por versión y banderas
// de fallo para cubrir 409/503.
type e019HTTPAlmacen struct {
	mu        sync.Mutex
	registros map[string]*appOnboarding.Record

	falloLectura      error
	falloEscritura    error
	conflictoSiempre  bool
	intentosEscritura int
}

func newE019HTTPAlmacen() *e019HTTPAlmacen {
	return &e019HTTPAlmacen{registros: map[string]*appOnboarding.Record{}}
}

func copiarRegistroE019HTTP(registro *appOnboarding.Record) *appOnboarding.Record {
	if registro == nil {
		return nil
	}
	copia := *registro
	copia.CompletedSteps = append([]string{}, registro.CompletedSteps...)
	if registro.CompletedAt != nil {
		marca := *registro.CompletedAt
		copia.CompletedAt = &marca
	}
	return &copia
}

func (a *e019HTTPAlmacen) GetOnboarding(_ context.Context, userID string) (*appOnboarding.Record, bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.falloLectura != nil {
		return nil, false, a.falloLectura
	}
	registro, ok := a.registros[userID]
	if !ok {
		return nil, false, nil
	}
	return copiarRegistroE019HTTP(registro), true, nil
}

func (a *e019HTTPAlmacen) CreateOnboarding(_ context.Context, userID string, write appOnboarding.Write) (*appOnboarding.Record, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.intentosEscritura++
	if a.falloEscritura != nil {
		return nil, a.falloEscritura
	}
	if a.conflictoSiempre {
		return nil, appOnboarding.ErrConflict
	}
	if _, existe := a.registros[userID]; existe {
		return nil, appOnboarding.ErrConflict
	}
	registro := registroDesdeWriteE019HTTP(write, 1)
	a.registros[userID] = registro
	return copiarRegistroE019HTTP(registro), nil
}

func (a *e019HTTPAlmacen) UpdateOnboarding(_ context.Context, userID string, write appOnboarding.Write) (*appOnboarding.Record, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.intentosEscritura++
	if a.falloEscritura != nil {
		return nil, a.falloEscritura
	}
	if a.conflictoSiempre {
		return nil, appOnboarding.ErrConflict
	}
	actual, existe := a.registros[userID]
	if !existe {
		return nil, appOnboarding.ErrConflict
	}
	if actual.HasVersion != write.HasVersion || (write.HasVersion && actual.Version != write.ExpectedVersion) {
		return nil, appOnboarding.ErrConflict
	}
	version := int64(1)
	if actual.HasVersion {
		version = actual.Version + 1
	}
	registro := registroDesdeWriteE019HTTP(write, version)
	a.registros[userID] = registro
	return copiarRegistroE019HTTP(registro), nil
}

func registroDesdeWriteE019HTTP(write appOnboarding.Write, version int64) *appOnboarding.Record {
	registro := &appOnboarding.Record{
		CompletedSteps:   append([]string{}, write.CompletedSteps...),
		Version:          version,
		HasVersion:       true,
		MicrophoneRaw:    write.MicrophoneRaw,
		NotificationsRaw: write.NotificationsRaw,
		Configured:       write.Configured,
	}
	if write.CompletedAt != nil && !write.CompletedAt.IsZero() {
		marca := write.CompletedAt.UTC()
		registro.CompletedAt = &marca
	}
	return registro
}

func e019HTTPHandler(almacen appOnboarding.Store) *internalhttp.OnboardingHandler {
	var servicio *appOnboarding.Service
	if almacen != nil {
		servicio = appOnboarding.NewService(almacen).WithClock(func() time.Time { return e019HTTPAhora })
	}
	return internalhttp.NewOnboardingHandler(servicio, e018HTTPTokens(), e004SesionesAmpliada{}, silencioso())
}

func e019Peticion(cuerpo string) *http.Request {
	return e004Peticion(http.MethodPatch, e019RutaOnboarding, "jwt-a", cuerpo)
}

func e019Cuerpo(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var cuerpo map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("respuesta no es JSON: %v (%s)", err, rec.Body.String())
	}
	return cuerpo
}

// TestE019HTTP200EsquemaExacto cubre el contrato de éxito: siete campos, booleanos
// presentes sin omitempty, `completedSteps` como array y `completedAt` null.
func TestE019HTTP200EsquemaExacto(t *testing.T) {
	almacen := newE019HTTPAlmacen()
	almacen.registros[e018HTTPUsuarioA] = &appOnboarding.Record{
		CompletedSteps: []string{"profile"},
		Version:        2,
		HasVersion:     true,
		MicrophoneRaw:  "pending",
	}
	handler := e019HTTPHandler(almacen)

	rec := httptest.NewRecorder()
	handler.ServePatch(rec, e019Peticion(`{"completedSteps":["profile","income"],"microphone":true}`))

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if cache := rec.Header().Get("Cache-Control"); !strings.Contains(cache, "private") || !strings.Contains(cache, "no-store") {
		t.Fatalf("Cache-Control debe ser 'private, no-store', obtenido %q", cache)
	}

	cuerpo := e019Cuerpo(t, rec)
	claves := make([]string, 0, len(cuerpo))
	for clave := range cuerpo {
		claves = append(claves, clave)
	}
	sort.Strings(claves)
	esperadas := []string{"completedAt", "completedSteps", "configured", "done", "microphone", "notifications", "version"}
	if strings.Join(claves, ",") != strings.Join(esperadas, ",") {
		t.Fatalf("campos de la respuesta %v, esperados %v", claves, esperadas)
	}
	pasos, ok := cuerpo["completedSteps"].([]any)
	if !ok || len(pasos) != 2 || pasos[0] != "profile" || pasos[1] != "income" {
		t.Fatalf("completedSteps esperado [profile income], obtenido %#v", cuerpo["completedSteps"])
	}
	for _, campo := range []string{"done", "microphone", "notifications", "configured"} {
		if _, ok := cuerpo[campo].(bool); !ok {
			t.Fatalf("%s debe ser booleano, obtenido %#v", campo, cuerpo[campo])
		}
	}
	if cuerpo["microphone"] != true {
		t.Fatalf("microphone=true debe proyectarse true: %v", cuerpo)
	}
	if cuerpo["notifications"] != false || cuerpo["configured"] != false {
		t.Fatalf("los booleanos no enviados deben emitirse false: %v", cuerpo)
	}
	if valor, existe := cuerpo["completedAt"]; !existe || valor != nil {
		t.Fatalf("completedAt debe emitirse como null, obtenido %#v", valor)
	}
	if version, ok := cuerpo["version"].(float64); !ok || version != 3 {
		t.Fatalf("version esperada 3, obtenido %#v", cuerpo["version"])
	}

	persistido := almacen.registros[e018HTTPUsuarioA]
	if persistido.MicrophoneRaw != "granted" {
		t.Fatalf("microphone=true debe persistir 'granted', obtenido %q", persistido.MicrophoneRaw)
	}
}

// TestE019HTTP200Vacio: `completedSteps: []` es válido y la respuesta emite `[]`,
// nunca `null`.
func TestE019HTTP200Vacio(t *testing.T) {
	almacen := newE019HTTPAlmacen()
	almacen.registros[e018HTTPUsuarioA] = &appOnboarding.Record{
		CompletedSteps: []string{"profile", "income"},
		Version:        4,
		HasVersion:     true,
	}
	handler := e019HTTPHandler(almacen)

	rec := httptest.NewRecorder()
	handler.ServePatch(rec, e019Peticion(`{"completedSteps":[]}`))

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	var cuerpo struct {
		CompletedSteps []string `json:"completedSteps"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("respuesta no es JSON: %v", err)
	}
	if cuerpo.CompletedSteps == nil {
		t.Fatalf("completedSteps debe ser [] y no null")
	}
	if len(cuerpo.CompletedSteps) != 0 {
		t.Fatalf("completedSteps debe quedar vacio, obtenido %v", cuerpo.CompletedSteps)
	}
}

// TestE019HTTP200Completado: los cuatro pasos marcan `done` y emiten la marca
// RFC3339.
func TestE019HTTP200Completado(t *testing.T) {
	almacen := newE019HTTPAlmacen()
	almacen.registros[e018HTTPUsuarioA] = &appOnboarding.Record{
		CompletedSteps: []string{"profile"},
		Version:        1,
		HasVersion:     true,
	}
	handler := e019HTTPHandler(almacen)

	rec := httptest.NewRecorder()
	handler.ServePatch(rec, e019Peticion(`{"completedSteps":["permissions","privacy","income","profile"]}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d (%s)", rec.Code, rec.Body.String())
	}

	var cuerpo struct {
		CompletedSteps []string `json:"completedSteps"`
		Done           bool     `json:"done"`
		CompletedAt    *string  `json:"completedAt"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("respuesta no es JSON: %v", err)
	}
	if !cuerpo.Done || cuerpo.CompletedAt == nil {
		t.Fatalf("cuatro pasos deben marcar done y completedAt: %+v", cuerpo)
	}
	if _, err := time.Parse(time.RFC3339, *cuerpo.CompletedAt); err != nil {
		t.Fatalf("completedAt debe ser RFC3339, obtenido %q", *cuerpo.CompletedAt)
	}
	if want := []string{"profile", "income", "privacy", "permissions"}; strings.Join(cuerpo.CompletedSteps, ",") != strings.Join(want, ",") {
		t.Fatalf("orden canonico esperado %v, obtenido %v", want, cuerpo.CompletedSteps)
	}
}

// TestE019HTTP400 cubre las solicitudes inválidas: cuerpo ausente, `null`, campo
// obligatorio omitido, campo desconocido, tipo incorrecto, JSON roto y
// Content-Type no JSON.
func TestE019HTTP400(t *testing.T) {
	casos := []struct {
		nombre      string
		cuerpo      string
		contentType string
	}{
		{"cuerpo vacio", "", "application/json"},
		{"completedSteps null", `{"completedSteps":null}`, "application/json"},
		{"completedSteps ausente", `{"microphone":true}`, "application/json"},
		{"campo desconocido", `{"completedSteps":[],"extra":1}`, "application/json"},
		{"tipo incorrecto", `{"completedSteps":"profile"}`, "application/json"},
		{"json roto", `{"completedSteps":`, "application/json"},
		{"content type no json", `{"completedSteps":[]}`, "text/plain"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			handler := e019HTTPHandler(newE019HTTPAlmacen())
			req := httptest.NewRequest(http.MethodPatch, e019RutaOnboarding, strings.NewReader(caso.cuerpo))
			req.Header.Set("Authorization", "Bearer jwt-a")
			if caso.contentType != "" {
				req.Header.Set("Content-Type", caso.contentType)
			}
			rec := httptest.NewRecorder()
			handler.ServePatch(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("esperado 400, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "INVALID_REQUEST") {
				t.Fatalf("el error debe ser INVALID_REQUEST, obtenido %q", rec.Body.String())
			}
		})
	}
}

// TestE019HTTP401: sin Bearer, esquema inválido, token desconocido, expirado o
// sesión revocada no se escribe nada.
func TestE019HTTP401(t *testing.T) {
	casos := []struct {
		nombre   string
		cabecera string
		revocar  bool
	}{
		{"sin cabecera", "", false},
		{"esquema basico", "Basic abc", false},
		{"token desconocido", "Bearer desconocido", false},
		{"token expirado", "Bearer jwt-expirado", false},
		{"sesion revocada", "Bearer jwt-valido", true},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			sesiones := newE004Sesiones()
			if caso.revocar {
				sesiones.revocar(e004HTTPSesion)
			}
			almacen := newE019HTTPAlmacen()
			handler := internalhttp.NewOnboardingHandler(
				appOnboarding.NewService(almacen),
				e004TokensValidos(),
				sesiones,
				silencioso(),
			)
			req := httptest.NewRequest(http.MethodPatch, e019RutaOnboarding, strings.NewReader(`{"completedSteps":[]}`))
			if caso.cabecera != "" {
				req.Header.Set("Authorization", caso.cabecera)
			}
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			handler.ServePatch(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("esperado 401, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			if almacen.intentosEscritura != 0 {
				t.Fatalf("una peticion sin identidad no puede escribir, intentos=%d", almacen.intentosEscritura)
			}
		})
	}
}

// TestE019HTTP422: un paso fuera del catálogo es una violación de negocio.
func TestE019HTTP422(t *testing.T) {
	handler := e019HTTPHandler(newE019HTTPAlmacen())
	for _, cuerpo := range []string{
		`{"completedSteps":["otro"]}`,
		`{"completedSteps":["profile","desconocido"]}`,
		`{"completedSteps":[""]}`,
	} {
		rec := httptest.NewRecorder()
		handler.ServePatch(rec, e019Peticion(cuerpo))
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("esperado 422 para %s, obtenido %d (%s)", cuerpo, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "BUSINESS_RULE_VIOLATION") {
			t.Fatalf("el error debe ser BUSINESS_RULE_VIOLATION, obtenido %q", rec.Body.String())
		}
	}
}

// TestE019HTTP409: un conflicto de versión persistente no se oculta como éxito.
func TestE019HTTP409(t *testing.T) {
	almacen := newE019HTTPAlmacen()
	almacen.conflictoSiempre = true
	almacen.registros[e018HTTPUsuarioA] = &appOnboarding.Record{Version: 2, HasVersion: true}

	rec := httptest.NewRecorder()
	e019HTTPHandler(almacen).ServePatch(rec, e019Peticion(`{"completedSteps":["profile"]}`))

	if rec.Code != http.StatusConflict {
		t.Fatalf("esperado 409, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "CONFLICT") {
		t.Fatalf("el error debe ser CONFLICT, obtenido %q", rec.Body.String())
	}
	if almacen.intentosEscritura != 2 {
		t.Fatalf("debe haber dos intentos (reintento unico), hay %d", almacen.intentosEscritura)
	}
}

// TestE019HTTP503 cubre el fail-closed: sin servicio, sin tokens, sin autorizador
// o con persistencia caída no se acepta un cambio que no se guardó.
func TestE019HTTP503(t *testing.T) {
	lecturaCaida := newE019HTTPAlmacen()
	lecturaCaida.falloLectura = errors.New("dynamodb no disponible")

	escrituraCaida := newE019HTTPAlmacen()
	escrituraCaida.registros[e018HTTPUsuarioA] = &appOnboarding.Record{Version: 1, HasVersion: true}
	escrituraCaida.falloEscritura = errors.New("dynamodb no disponible")

	casos := []struct {
		nombre  string
		handler *internalhttp.OnboardingHandler
	}{
		{"sin servicio", internalhttp.NewOnboardingHandler(nil, e018HTTPTokens(), e004SesionesAmpliada{}, silencioso())},
		{"sin tokens", internalhttp.NewOnboardingHandler(appOnboarding.NewService(newE019HTTPAlmacen()), nil, e004SesionesAmpliada{}, silencioso())},
		{"sin autorizador", internalhttp.NewOnboardingHandler(appOnboarding.NewService(newE019HTTPAlmacen()), e018HTTPTokens(), nil, silencioso())},
		{"lectura caida", e019HTTPHandler(lecturaCaida)},
		{"escritura caida", e019HTTPHandler(escrituraCaida)},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			caso.handler.ServePatch(rec, e019Peticion(`{"completedSteps":[]}`))
			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("esperado 503, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "DEPENDENCY_UNAVAILABLE") {
				t.Fatalf("el error debe ser DEPENDENCY_UNAVAILABLE, obtenido %q", rec.Body.String())
			}
		})
	}
}

// TestE019HTTPFalseExplicito: `false` explícito se persiste como `denied` y no se
// confunde con un campo ausente.
func TestE019HTTPFalseExplicito(t *testing.T) {
	almacen := newE019HTTPAlmacen()
	almacen.registros[e018HTTPUsuarioA] = &appOnboarding.Record{
		CompletedSteps:   []string{"profile"},
		Version:          2,
		HasVersion:       true,
		MicrophoneRaw:    "granted",
		NotificationsRaw: "granted",
		Configured:       true,
	}

	rec := httptest.NewRecorder()
	e019HTTPHandler(almacen).ServePatch(rec, e019Peticion(`{"completedSteps":["profile"],"microphone":false,"notifications":false,"configured":false}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d (%s)", rec.Code, rec.Body.String())
	}

	cuerpo := e019Cuerpo(t, rec)
	for _, campo := range []string{"microphone", "notifications", "configured"} {
		if cuerpo[campo] != false {
			t.Fatalf("%s=false explicito debe emitirse false, obtenido %#v", campo, cuerpo[campo])
		}
	}
	persistido := almacen.registros[e018HTTPUsuarioA]
	if persistido.MicrophoneRaw != "denied" || persistido.NotificationsRaw != "denied" || persistido.Configured {
		t.Fatalf("false explicito debe persistir 'denied'/false: %+v", persistido)
	}
}

// TestE019HTTPAusenteConserva: un PATCH que no envía booleanos no los degrada.
func TestE019HTTPAusenteConserva(t *testing.T) {
	almacen := newE019HTTPAlmacen()
	almacen.registros[e018HTTPUsuarioA] = &appOnboarding.Record{
		CompletedSteps:   []string{"profile"},
		Version:          2,
		HasVersion:       true,
		MicrophoneRaw:    "granted",
		NotificationsRaw: "granted",
		Configured:       true,
	}

	rec := httptest.NewRecorder()
	e019HTTPHandler(almacen).ServePatch(rec, e019Peticion(`{"completedSteps":["profile","income"]}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	cuerpo := e019Cuerpo(t, rec)
	if cuerpo["microphone"] != true || cuerpo["notifications"] != true || cuerpo["configured"] != true {
		t.Fatalf("los booleanos ausentes deben conservarse, obtenido %v", cuerpo)
	}
}

// TestE019HTTPAislamiento: la escritura se resuelve por el `sub` del JWT y nunca
// por un identificador del cliente. B no observa ni modifica el estado de A.
func TestE019HTTPAislamiento(t *testing.T) {
	almacen := newE019HTTPAlmacen()
	almacen.registros[e018HTTPUsuarioA] = &appOnboarding.Record{
		CompletedSteps: []string{"profile"},
		Version:        1,
		HasVersion:     true,
	}
	almacen.registros[e018HTTPUsuarioB] = &appOnboarding.Record{
		CompletedSteps: []string{"income"},
		Version:        5,
		HasVersion:     true,
	}
	handler := e019HTTPHandler(almacen)

	rec := httptest.NewRecorder()
	handler.ServePatch(rec, e019Peticion(`{"completedSteps":["profile","income","privacy","permissions"]}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("A debe poder escribir su estado, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), e018HTTPUsuarioA) || strings.Contains(rec.Body.String(), e018HTTPUsuarioB) {
		t.Fatalf("la respuesta no debe contener identificadores de usuario")
	}

	b := almacen.registros[e018HTTPUsuarioB]
	if b.Version != 5 || strings.Join(b.CompletedSteps, ",") != "income" {
		t.Fatalf("el estado de B no debe cambiar: version=%d pasos=%v", b.Version, b.CompletedSteps)
	}
	a := almacen.registros[e018HTTPUsuarioA]
	if a.Version != 2 || len(a.CompletedSteps) != 4 {
		t.Fatalf("el estado de A debe reflejar su propia escritura: version=%d pasos=%v", a.Version, a.CompletedSteps)
	}
}

// TestE019HTTPAutorizadorRechazaNoEscribe verifica que el handler no autoriza sólo
// con la firma cuando el estado de sesión rechaza: el 401 impide cualquier
// escritura.
func TestE019HTTPAutorizadorRechazaNoEscribe(t *testing.T) {
	almacen := newE019HTTPAlmacen()
	almacen.registros[e018HTTPUsuarioA] = &appOnboarding.Record{Version: 1, HasVersion: true}
	handler := internalhttp.NewOnboardingHandler(
		appOnboarding.NewService(almacen),
		e018HTTPTokens(),
		autorizadorRechazaE019{},
		silencioso(),
	)
	rec := httptest.NewRecorder()
	handler.ServePatch(rec, e019Peticion(`{"completedSteps":[]}`))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("esperado 401, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if almacen.intentosEscritura != 0 {
		t.Fatalf("no debe escribirse con la sesion no autorizada")
	}
}

type autorizadorRechazaE019 struct{}

func (autorizadorRechazaE019) AuthorizeSession(context.Context, string, string, int64) error {
	return ports.ErrSessionUnauthorized
}
