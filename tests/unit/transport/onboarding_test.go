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

// Pruebas HTTP de E018 (`GET /api/v1/users/me/onboarding`): middleware de
// autenticación con sesión autoritativa, contrato exacto de `Onboarding`,
// 404 sin estado, aislamiento por `sub` y fail-closed 503. Los dobles son
// exclusivamente de prueba; la composición productiva usa DynamoDB real.

const (
	e018HTTPUsuarioA = "11111111-2222-4333-8444-555555555555"
	e018HTTPUsuarioB = "99999999-8888-4777-8666-555555555555"
	e018HTTPSesionA  = "018f90ef-08e5-7a68-b09f-0000000000e1"
	e018HTTPSesionB  = "018f90ef-08e5-7a68-b09f-0000000000e2"
)

var e018HTTPAhora = time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)

// e018HTTPAlmacen es el puerto de persistencia en memoria, aislado por usuario.
type e018HTTPAlmacen struct {
	mu        sync.Mutex
	registros map[string]*appOnboarding.Record
	fallo     error
	usuarios  []string
}

func newE018HTTPAlmacen() *e018HTTPAlmacen {
	return &e018HTTPAlmacen{registros: map[string]*appOnboarding.Record{}}
}

func (a *e018HTTPAlmacen) GetOnboarding(_ context.Context, userID string) (*appOnboarding.Record, bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.usuarios = append(a.usuarios, userID)
	if a.fallo != nil {
		return nil, false, a.fallo
	}
	registro, ok := a.registros[userID]
	if !ok {
		return nil, false, nil
	}
	copia := *registro
	return &copia, true, nil
}

func e018HTTPHandler(almacen appOnboarding.Store) *internalhttp.OnboardingHandler {
	var servicio *appOnboarding.Service
	if almacen != nil {
		servicio = appOnboarding.NewService(almacen).WithClock(func() time.Time { return e018HTTPAhora })
	}
	return internalhttp.NewOnboardingHandler(servicio, e004TokensValidos(), newE004Sesiones(), silencioso())
}

// e018HTTPTokens devuelve tokens con los sujetos de las dos cuentas de prueba.
func e018HTTPTokens() *e004Tokens {
	return &e004Tokens{emitidos: map[string]ports.TokenClaims{
		"jwt-a": tokenClaims(e018HTTPUsuarioA, e018HTTPSesionA, e018HTTPAhora),
		"jwt-b": tokenClaims(e018HTTPUsuarioB, e018HTTPSesionB, e018HTTPAhora),
	}}
}

// TestE018HTTPAutenticacion cubre 401: sin Bearer, esquema inválido, token
// desconocido, expirado y sesión revocada en el estado autoritativo.
func TestE018HTTPAutenticacion(t *testing.T) {
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
			handler := internalhttp.NewOnboardingHandler(
				appOnboarding.NewService(newE018HTTPAlmacen()),
				e004TokensValidos(),
				sesiones,
				silencioso(),
			)
			req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me/onboarding", nil)
			if caso.cabecera != "" {
				req.Header.Set("Authorization", caso.cabecera)
			}
			rec := httptest.NewRecorder()
			handler.ServeGet(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("esperado 401, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			if cuerpo := rec.Body.String(); !strings.Contains(cuerpo, "\"error\"") {
				t.Fatalf("debe devolver ErrorEnvelope, obtenido %q", cuerpo)
			}
		})
	}
}

// TestE018HTTP200EsquemaExacto cubre el contrato de éxito: siete campos, booleanos
// presentes, `completedSteps` como array (nunca null) y `completedAt` null explícito.
func TestE018HTTP200EsquemaExacto(t *testing.T) {
	almacen := newE018HTTPAlmacen()
	almacen.registros[e018HTTPUsuarioA] = &appOnboarding.Record{
		CompletedSteps: []string{"profile"},
		Version:        2,
		HasVersion:     true,
		MicrophoneRaw:  "granted",
	}
	handler := e018HTTPHandler(almacen)

	rec := httptest.NewRecorder()
	handler.ServeGet(rec, e004Peticion(http.MethodGet, "/api/v1/users/me/onboarding", "jwt-valido", ""))

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
	if !ok {
		t.Fatalf("completedSteps debe ser un array JSON, obtenido %#v", cuerpo["completedSteps"])
	}
	if len(pasos) != 1 || pasos[0] != "profile" {
		t.Fatalf("completedSteps esperado [profile], obtenido %v", pasos)
	}
	if valor, existe := cuerpo["completedAt"]; !existe || valor != nil {
		t.Fatalf("completedAt debe emitirse como null, obtenido %#v", valor)
	}
	for _, campo := range []string{"done", "microphone", "notifications", "configured"} {
		if _, ok := cuerpo[campo].(bool); !ok {
			t.Fatalf("%s debe ser booleano, obtenido %#v", campo, cuerpo[campo])
		}
	}
	if cuerpo["microphone"] != true || cuerpo["notifications"] != false {
		t.Fatalf("permisos proyectados incorrectamente: %v", cuerpo)
	}
	if version, ok := cuerpo["version"].(float64); !ok || version != 2 {
		t.Fatalf("version esperada 2, obtenido %#v", cuerpo["version"])
	}
}

// TestE018HTTP200Vacio: un estado sin pasos se serializa `[]` y nunca `null`.
func TestE018HTTP200Vacio(t *testing.T) {
	almacen := newE018HTTPAlmacen()
	almacen.registros[e018HTTPUsuarioA] = &appOnboarding.Record{Version: 1, HasVersion: true}
	handler := e018HTTPHandler(almacen)

	rec := httptest.NewRecorder()
	handler.ServeGet(rec, e004Peticion(http.MethodGet, "/api/v1/users/me/onboarding", "jwt-valido", ""))

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
		t.Fatalf("completedSteps debe estar vacio, obtenido %v", cuerpo.CompletedSteps)
	}
}

// TestE018HTTP200Completado: con los cuatro pasos se emite la marca RFC3339.
func TestE018HTTP200Completado(t *testing.T) {
	marca := e018HTTPAhora.Add(-24 * time.Hour)
	almacen := newE018HTTPAlmacen()
	almacen.registros[e018HTTPUsuarioA] = &appOnboarding.Record{
		CompletedSteps: []string{"profile", "income", "privacy", "permissions"},
		Version:        9,
		HasVersion:     true,
		CompletedAt:    &marca,
	}
	handler := e018HTTPHandler(almacen)

	rec := httptest.NewRecorder()
	handler.ServeGet(rec, e004Peticion(http.MethodGet, "/api/v1/users/me/onboarding", "jwt-valido", ""))

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	var cuerpo struct {
		Done        bool    `json:"done"`
		CompletedAt *string `json:"completedAt"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("respuesta no es JSON: %v", err)
	}
	if !cuerpo.Done {
		t.Fatalf("done debe ser true con los cuatro pasos")
	}
	if cuerpo.CompletedAt == nil {
		t.Fatalf("completedAt no debe ser null con done=true")
	}
	if _, err := time.Parse(time.RFC3339, *cuerpo.CompletedAt); err != nil {
		t.Fatalf("completedAt debe ser RFC3339, obtenido %q", *cuerpo.CompletedAt)
	}
}

// TestE018HTTP404: sin estado persistido el endpoint responde 404.
func TestE018HTTP404(t *testing.T) {
	handler := e018HTTPHandler(newE018HTTPAlmacen())
	rec := httptest.NewRecorder()
	handler.ServeGet(rec, e004Peticion(http.MethodGet, "/api/v1/users/me/onboarding", "jwt-valido", ""))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("esperado 404, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "NOT_FOUND") {
		t.Fatalf("el error debe ser NOT_FOUND, obtenido %q", rec.Body.String())
	}
}

// TestE018HTTPFallaCerrado cubre el 503: sin servicio, sin proveedor de tokens, sin
// autorizador o con DynamoDB caído no se sirve estado.
func TestE018HTTPFallaCerrado(t *testing.T) {
	caido := newE018HTTPAlmacen()
	caido.fallo = errors.New("dynamodb no disponible")

	casos := []struct {
		nombre  string
		handler *internalhttp.OnboardingHandler
	}{
		{"sin servicio", internalhttp.NewOnboardingHandler(nil, e004TokensValidos(), newE004Sesiones(), silencioso())},
		{"sin tokens", internalhttp.NewOnboardingHandler(appOnboarding.NewService(newE018HTTPAlmacen()), nil, newE004Sesiones(), silencioso())},
		{"sin autorizador", internalhttp.NewOnboardingHandler(appOnboarding.NewService(newE018HTTPAlmacen()), e004TokensValidos(), nil, silencioso())},
		{"almacen caido", e018HTTPHandler(caido)},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			caso.handler.ServeGet(rec, e004Peticion(http.MethodGet, "/api/v1/users/me/onboarding", "jwt-valido", ""))
			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("esperado 503, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
		})
	}
}

// TestE018HTTPAislamiento: el estado se resuelve por el `sub` del JWT, no por un
// identificador del cliente. B no observa el estado de A.
func TestE018HTTPAislamiento(t *testing.T) {
	almacen := newE018HTTPAlmacen()
	almacen.registros[e018HTTPUsuarioA] = &appOnboarding.Record{
		CompletedSteps: []string{"profile", "income", "privacy", "permissions"},
		Version:        4,
		HasVersion:     true,
	}
	handler := internalhttp.NewOnboardingHandler(
		appOnboarding.NewService(almacen),
		e018HTTPTokens(),
		e004SesionesAmpliada{},
		silencioso(),
	)

	recA := httptest.NewRecorder()
	handler.ServeGet(recA, e004Peticion(http.MethodGet, "/api/v1/users/me/onboarding", "jwt-a", ""))
	if recA.Code != http.StatusOK {
		t.Fatalf("A debe leer su estado, obtenido %d (%s)", recA.Code, recA.Body.String())
	}

	recB := httptest.NewRecorder()
	handler.ServeGet(recB, e004Peticion(http.MethodGet, "/api/v1/users/me/onboarding", "jwt-b", ""))
	if recB.Code != http.StatusNotFound {
		t.Fatalf("B no debe observar el estado de A, obtenido %d (%s)", recB.Code, recB.Body.String())
	}
	if strings.Contains(recA.Body.String(), e018HTTPUsuarioA) || strings.Contains(recB.Body.String(), e018HTTPUsuarioB) {
		t.Fatalf("la respuesta no debe contener identificadores de usuario")
	}
}

// e004SesionesAmpliada autoriza cualquier usuario/sesión de las cuentas de prueba
// de E018 sin reutilizar la lista cerrada de E004.
type e004SesionesAmpliada struct{}

func (e004SesionesAmpliada) AuthorizeSession(_ context.Context, userID, sessionID string, _ int64) error {
	if userID == e018HTTPUsuarioA && sessionID == e018HTTPSesionA {
		return nil
	}
	if userID == e018HTTPUsuarioB && sessionID == e018HTTPSesionB {
		return nil
	}
	return ports.ErrSessionUnauthorized
}
