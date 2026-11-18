package transport_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appProfile "github.com/nexumpr/Nexum-Backend/internal/application/profile"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// Pruebas HTTP de E013 (`PATCH /api/v1/users/me`): If-Match obligatorio (428/400/412),
// cuerpo estricto (400), validación de dominio (422), conflicto de reintento (409), ETag de
// salida y fallo cerrado. Los dobles son exclusivamente de prueba.

var e013HTTPAhora = time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)

// e013HTTPStore dobla lectura y escritura con estado en memoria.
type e013HTTPStore struct {
	registros      map[string]*appProfile.Record
	falloLectura   error
	falloEscritura error
	casFallidos    int
	escrituras     []appProfile.UpdateInput
}

func (s *e013HTTPStore) LoadProfile(_ context.Context, userID string) (*appProfile.Record, error) {
	if s.falloLectura != nil {
		return nil, s.falloLectura
	}
	return s.registros[userID], nil
}

func (s *e013HTTPStore) UpdateProfile(_ context.Context, input appProfile.UpdateInput) (*appProfile.Record, error) {
	s.escrituras = append(s.escrituras, input)
	if s.falloEscritura != nil {
		return nil, s.falloEscritura
	}
	if s.casFallidos > 0 {
		s.casFallidos--
		return nil, appProfile.ErrProfileCASFailed
	}
	actual := s.registros[input.UserID]
	if actual == nil || e013HTTPVersion(actual) != input.ExpectedVersion {
		return nil, appProfile.ErrProfileCASFailed
	}
	copia := *actual
	copia.DisplayName = input.DisplayName
	copia.Version = e013HTTPVersion(actual) + 1
	copia.HasVersion = true
	s.registros[input.UserID] = &copia
	return &copia, nil
}

func e013HTTPVersion(record *appProfile.Record) int64 {
	if record == nil || !record.HasVersion || record.Version < 1 {
		return 1
	}
	return record.Version
}

func e013HTTPRegistro(userID string, version int64) *appProfile.Record {
	return &appProfile.Record{
		UserID:        userID,
		Email:         "ejemplo@example.invalid",
		DisplayName:   "Usuario Ejemplo",
		EmailVerified: true,
		CreatedAt:     e013HTTPAhora.Add(-48 * time.Hour),
		HasCreatedAt:  true,
		Version:       version,
		HasVersion:    true,
	}
}

func e013HTTPHandler(store *e013HTTPStore, sesiones ports.SessionAuthorizer) *internalhttp.ProfileHandler {
	var servicio *appProfile.Service
	if store != nil {
		servicio = appProfile.NewService(store).WithUpdateStore(store)
	}
	return internalhttp.NewProfileHandler(servicio, e004TokensValidos(), sesiones, silencioso())
}

// e013HTTPHandlerSoloLectura compone el handler sin puerto de escritura (equivalente a un
// montaje que sólo habilitó E012).
func e013HTTPHandlerSoloLectura(store *e013HTTPStore, sesiones ports.SessionAuthorizer) *internalhttp.ProfileHandler {
	return internalhttp.NewProfileHandler(appProfile.NewService(store), e004TokensValidos(), sesiones, silencioso())
}

// e013Peticion construye la petición con `If-Match` cuando se indica un valor.
func e013Peticion(etag, cuerpo string) *http.Request {
	req := e004Peticion(http.MethodPatch, "/api/v1/users/me", "jwt-valido", cuerpo)
	if etag != "" {
		req.Header.Set("If-Match", etag)
	}
	return req
}

func e013Cuerpo(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var cuerpo map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("respuesta no es JSON: %v (%s)", err, rec.Body.String())
	}
	return cuerpo
}

func e013CodigoDeError(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	cuerpo := e013Cuerpo(t, rec)
	sobre, ok := cuerpo["error"].(map[string]any)
	if !ok {
		t.Fatalf("se esperaba el sobre ErrorEnvelope: %s", rec.Body.String())
	}
	codigo, _ := sobre["code"].(string)
	return codigo
}

// TestE013HTTPIfMatchObligatorio cubre 428 sin cabecera y 400 con formatos no válidos.
func TestE013HTTPIfMatchObligatorio(t *testing.T) {
	store := &e013HTTPStore{registros: map[string]*appProfile.Record{
		e004HTTPUsuario: e013HTTPRegistro(e004HTTPUsuario, 3),
	}}
	handler := e013HTTPHandler(store, newE004Sesiones())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, e013Peticion("", `{"displayName":"Nombre Nuevo"}`))
	if rec.Code != http.StatusPreconditionRequired {
		t.Fatalf("sin If-Match debe ser 428, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if codigo := e013CodigoDeError(t, rec); codigo != "PRECONDITION_REQUIRED" {
		t.Fatalf("codigo inesperado: %q", codigo)
	}

	for _, etag := range []string{"v3", `"3"`, `"v0"`, `"vx"`, `"v-2"`, `""`, `*`} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, e013Peticion(etag, `{"displayName":"Nombre Nuevo"}`))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("If-Match %q debe ser 400, obtenido %d (%s)", etag, rec.Code, rec.Body.String())
		}
		if codigo := e013CodigoDeError(t, rec); codigo != "INVALID_REQUEST" {
			t.Fatalf("codigo inesperado para %q: %q", etag, codigo)
		}
	}
	if len(store.escrituras) != 0 {
		t.Fatalf("una precondicion invalida no debe escribir: %v", store.escrituras)
	}
}

// TestE013HTTPVersionDesactualizada cubre 412.
func TestE013HTTPVersionDesactualizada(t *testing.T) {
	store := &e013HTTPStore{registros: map[string]*appProfile.Record{
		e004HTTPUsuario: e013HTTPRegistro(e004HTTPUsuario, 3),
	}}
	handler := e013HTTPHandler(store, newE004Sesiones())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, e013Peticion(`"v2"`, `{"displayName":"Nombre Nuevo"}`))
	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("esperado 412, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if codigo := e013CodigoDeError(t, rec); codigo != "VERSION_CONFLICT" {
		t.Fatalf("codigo inesperado: %q", codigo)
	}
	if len(store.escrituras) != 0 {
		t.Fatalf("una version desactualizada no debe escribir: %v", store.escrituras)
	}
}

// TestE013HTTPNombreInvalido cubre 422 con detalle del campo.
func TestE013HTTPNombreInvalido(t *testing.T) {
	store := &e013HTTPStore{registros: map[string]*appProfile.Record{
		e004HTTPUsuario: e013HTTPRegistro(e004HTTPUsuario, 3),
	}}
	handler := e013HTTPHandler(store, newE004Sesiones())

	for _, cuerpo := range []string{`{"displayName":""}`, `{"displayName":"   "}`, `{"displayName":"` + strings.Repeat("á", 101) + `"}`} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, e013Peticion(`"v3"`, cuerpo))
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("cuerpo %q debe ser 422, obtenido %d (%s)", cuerpo, rec.Code, rec.Body.String())
		}
		sobre := e013Cuerpo(t, rec)["error"].(map[string]any)
		if sobre["code"] != "BUSINESS_RULE_VIOLATION" {
			t.Fatalf("codigo inesperado: %v", sobre["code"])
		}
		detalles, _ := sobre["details"].([]any)
		if len(detalles) == 0 {
			t.Fatalf("422 debe incluir details: %s", rec.Body.String())
		}
		detalle, _ := detalles[0].(map[string]any)
		if detalle["field"] != "displayName" {
			t.Fatalf("el detalle debe senalar displayName: %v", detalle)
		}
	}
	if len(store.escrituras) != 0 {
		t.Fatalf("un nombre invalido no debe escribir: %v", store.escrituras)
	}
}

// TestE013HTTPCuerpoEstricto cubre 400 por campo desconocido y por JSON invalido.
func TestE013HTTPCuerpoEstricto(t *testing.T) {
	store := &e013HTTPStore{registros: map[string]*appProfile.Record{
		e004HTTPUsuario: e013HTTPRegistro(e004HTTPUsuario, 3),
	}}
	handler := e013HTTPHandler(store, newE004Sesiones())

	casos := []string{
		`{"displayName":"Nombre Nuevo","email":"otro@example.invalid"}`,
		`{"displayName":"Nombre Nuevo","version":9}`,
		`{"displayName":`,
		`{"displayName":"Nombre Nuevo"}{"displayName":"Otro"}`,
		`{"displayName":null}`,
		`null`,
		`{"displayName":42}`,
	}
	for _, cuerpo := range casos {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, e013Peticion(`"v3"`, cuerpo))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("cuerpo %q debe ser 400, obtenido %d (%s)", cuerpo, rec.Code, rec.Body.String())
		}
		if codigo := e013CodigoDeError(t, rec); codigo != "INVALID_REQUEST" {
			t.Fatalf("codigo inesperado: %q", codigo)
		}
	}
	if len(store.escrituras) != 0 {
		t.Fatalf("un cuerpo invalido no debe escribir: %v", store.escrituras)
	}
}

// TestE013HTTPVacioNoEscribe cubre `{}`: 200, seis campos, ETag de la versión vigente y
// ninguna escritura.
func TestE013HTTPVacioNoEscribe(t *testing.T) {
	store := &e013HTTPStore{registros: map[string]*appProfile.Record{
		e004HTTPUsuario: e013HTTPRegistro(e004HTTPUsuario, 3),
	}}
	handler := e013HTTPHandler(store, newE004Sesiones())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, e013Peticion(`"v3"`, `{}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if etag := rec.Header().Get("ETag"); etag != `"v3"` {
		t.Fatalf("ETag inesperado: %q", etag)
	}
	cuerpo := e013Cuerpo(t, rec)
	if len(cuerpo) != 6 {
		t.Fatalf("UserProfile es un objeto cerrado de 6 campos: %v", cuerpo)
	}
	if cuerpo["version"] != float64(3) {
		t.Fatalf("la version no debe incrementarse: %v", cuerpo["version"])
	}
	if len(store.escrituras) != 0 {
		t.Fatalf("{} no debe escribir: %v", store.escrituras)
	}
}

// TestE013HTTPActualizaYDevuelveETag cubre el camino feliz, incluido If-Match débil.
func TestE013HTTPActualizaYDevuelveETag(t *testing.T) {
	store := &e013HTTPStore{registros: map[string]*appProfile.Record{
		e004HTTPUsuario: e013HTTPRegistro(e004HTTPUsuario, 3),
	}}
	handler := e013HTTPHandler(store, newE004Sesiones())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, e013Peticion(`W/"v3"`, `{"displayName":"Nombre Nuevo"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if etag := rec.Header().Get("ETag"); etag != `"v4"` {
		t.Fatalf("ETag inesperado: %q", etag)
	}
	if cache := rec.Header().Get("Cache-Control"); !strings.Contains(cache, "private") || !strings.Contains(cache, "no-store") {
		t.Fatalf("Cache-Control debe ser 'private, no-store', obtenido %q", cache)
	}
	cuerpo := e013Cuerpo(t, rec)
	claves := make(map[string]bool, len(cuerpo))
	for clave := range cuerpo {
		claves[clave] = true
	}
	for _, requerida := range []string{"id", "email", "displayName", "emailVerified", "createdAt", "version"} {
		if !claves[requerida] {
			t.Fatalf("falta el campo requerido %q en %v", requerida, cuerpo)
		}
	}
	if len(cuerpo) != 6 {
		t.Fatalf("UserProfile es un objeto cerrado de 6 campos: %v", cuerpo)
	}
	if cuerpo["displayName"] != "Nombre Nuevo" || cuerpo["version"] != float64(4) {
		t.Fatalf("respuesta inesperada: %v", cuerpo)
	}
	for _, prohibida := range []string{"passwordHash", "sessionEpoch", "privacyNoticeVersion", "status"} {
		if claves[prohibida] {
			t.Fatalf("el cuerpo no debe incluir %q", prohibida)
		}
	}
	if len(store.escrituras) != 1 || store.escrituras[0].ExpectedVersion != 3 {
		t.Fatalf("debe escribirse una vez con la version de If-Match: %v", store.escrituras)
	}
}

// TestE013HTTPReintentoAgotado cubre 409 cuando la carrera persiste.
func TestE013HTTPReintentoAgotado(t *testing.T) {
	store := &e013HTTPStore{
		registros:   map[string]*appProfile.Record{e004HTTPUsuario: e013HTTPRegistro(e004HTTPUsuario, 3)},
		casFallidos: 5,
	}
	handler := e013HTTPHandler(store, newE004Sesiones())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, e013Peticion(`"v3"`, `{"displayName":"Nombre Nuevo"}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("esperado 409, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if codigo := e013CodigoDeError(t, rec); codigo != "CONFLICT" {
		t.Fatalf("codigo inesperado: %q", codigo)
	}
}

// TestE013HTTPFallaCerrado: sin servicio, sin puerto de escritura, con la sesión o la
// persistencia caídas, responde 503 y no escribe.
func TestE013HTTPFallaCerrado(t *testing.T) {
	casos := []struct {
		nombre  string
		handler *internalhttp.ProfileHandler
	}{
		{"sin servicio", e013HTTPHandler(nil, newE004Sesiones())},
		{"sin puerto de escritura", e013HTTPHandlerSoloLectura(
			&e013HTTPStore{registros: map[string]*appProfile.Record{}}, newE004Sesiones())},
		{"sin autorizador de sesiones", e013HTTPHandler(
			&e013HTTPStore{registros: map[string]*appProfile.Record{}}, nil)},
		{"estado de sesion indisponible", func() *internalhttp.ProfileHandler {
			sesiones := newE004Sesiones()
			sesiones.fallo = ports.ErrSessionUnavailable
			return e013HTTPHandler(&e013HTTPStore{}, sesiones)
		}()},
		{"persistencia de lectura caida", e013HTTPHandler(&e013HTTPStore{
			falloLectura: errors.New("dynamodb no disponible"),
		}, newE004Sesiones())},
		{"persistencia de escritura caida", e013HTTPHandler(&e013HTTPStore{
			registros:      map[string]*appProfile.Record{e004HTTPUsuario: e013HTTPRegistro(e004HTTPUsuario, 3)},
			falloEscritura: errors.New("dynamodb no disponible"),
		}, newE004Sesiones())},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			caso.handler.ServeHTTP(rec, e013Peticion(`"v3"`, `{"displayName":"Nombre Nuevo"}`))
			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("esperado 503, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			if codigo := e013CodigoDeError(t, rec); codigo != "DEPENDENCY_UNAVAILABLE" {
				t.Fatalf("codigo inesperado: %q", codigo)
			}
		})
	}
}

// TestE013HTTPAutenticacion: sin Bearer, token desconocido, expirado o sin sesión no se
// escribe nada.
func TestE013HTTPAutenticacion(t *testing.T) {
	store := &e013HTTPStore{registros: map[string]*appProfile.Record{
		e004HTTPUsuario: e013HTTPRegistro(e004HTTPUsuario, 3),
	}}
	handler := e013HTTPHandler(store, newE004Sesiones())

	casos := []struct {
		nombre   string
		cabecera string
	}{
		{"sin cabecera", ""},
		{"esquema basico", "Basic abc"},
		{"token desconocido", "Bearer desconocido"},
		{"token expirado", "Bearer jwt-expirado"},
		{"sin sesion", "Bearer jwt-sin-sesion"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			peticion := e013Peticion(`"v3"`, `{"displayName":"Nombre Nuevo"}`)
			peticion.Header.Del("Authorization")
			if caso.cabecera != "" {
				peticion.Header.Set("Authorization", caso.cabecera)
			}
			handler.ServeHTTP(rec, peticion)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("esperado 401, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
		})
	}
	if len(store.escrituras) != 0 {
		t.Fatalf("sin autenticacion no debe escribirse: %v", store.escrituras)
	}
}

// TestE013HTTPMetodoNoPermitido: el handler anuncia GET y PATCH, no otros verbos.
func TestE013HTTPMetodoNoPermitido(t *testing.T) {
	store := &e013HTTPStore{registros: map[string]*appProfile.Record{
		e004HTTPUsuario: e013HTTPRegistro(e004HTTPUsuario, 3),
	}}
	rec := httptest.NewRecorder()
	e013HTTPHandler(store, newE004Sesiones()).ServeHTTP(rec, e004Peticion(http.MethodDelete, "/api/v1/users/me", "jwt-valido", ""))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("esperado 405, obtenido %d", rec.Code)
	}
	allow := rec.Header().Get("Allow")
	if !strings.Contains(allow, http.MethodGet) || !strings.Contains(allow, http.MethodPatch) {
		t.Fatalf("Allow debe anunciar GET y PATCH, obtuvo %q", allow)
	}
}

// TestE013HTTPPerfilInexistente cubre 404 sin escribir ni crear el perfil.
func TestE013HTTPPerfilInexistente(t *testing.T) {
	store := &e013HTTPStore{registros: map[string]*appProfile.Record{}}
	rec := httptest.NewRecorder()
	e013HTTPHandler(store, newE004Sesiones()).ServeHTTP(rec, e013Peticion(`"v1"`, `{"displayName":"Nombre Nuevo"}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("esperado 404, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if codigo := e013CodigoDeError(t, rec); codigo != "NOT_FOUND" {
		t.Fatalf("codigo inesperado: %q", codigo)
	}
	if len(store.escrituras) != 0 {
		t.Fatalf("no debe escribir sin perfil: %v", store.escrituras)
	}
}
