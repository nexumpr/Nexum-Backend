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

// Pruebas HTTP de E012 (`GET /api/v1/users/me`): middleware de autenticación con sesión
// autoritativa, esquema exacto de `UserProfile`, aislamiento entre sujetos y fallo cerrado.
// Los dobles son exclusivamente de prueba; la composición productiva usa DynamoDB real.

var e012HTTPAhora = time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)

// e012HTTPStore devuelve perfiles preparados por sujeto y registra a quién se consultó.
type e012HTTPStore struct {
	registros map[string]*appProfile.Record
	consultas []string
	fallo     error
}

func (s *e012HTTPStore) LoadProfile(_ context.Context, userID string) (*appProfile.Record, error) {
	s.consultas = append(s.consultas, userID)
	if s.fallo != nil {
		return nil, s.fallo
	}
	return s.registros[userID], nil
}

func e012HTTPRegistro(userID, email string, version int64) *appProfile.Record {
	return &appProfile.Record{
		UserID:        userID,
		Email:         email,
		DisplayName:   "Usuario Ejemplo",
		EmailVerified: true,
		CreatedAt:     e012HTTPAhora.Add(-24 * time.Hour),
		HasCreatedAt:  true,
		Version:       version,
		HasVersion:    true,
	}
}

func e012HTTPHandler(store appProfile.Store, sesiones ports.SessionAuthorizer) *internalhttp.ProfileHandler {
	var servicio *appProfile.Service
	if store != nil {
		servicio = appProfile.NewService(store)
	}
	return internalhttp.NewProfileHandler(servicio, e004TokensValidos(), sesiones, silencioso())
}

// TestE012HTTPEsquemaExacto cubre T01: 200 con exactamente los seis campos del contrato y
// `Cache-Control: private, no-store`.
func TestE012HTTPEsquemaExacto(t *testing.T) {
	store := &e012HTTPStore{registros: map[string]*appProfile.Record{
		e004HTTPUsuario: e012HTTPRegistro(e004HTTPUsuario, "ejemplo@example.invalid", 2),
	}}
	rec := httptest.NewRecorder()
	e012HTTPHandler(store, newE004Sesiones()).ServeHTTP(rec, e004Peticion(http.MethodGet, "/api/v1/users/me", "jwt-valido", ""))

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
	claves := make(map[string]bool, len(cuerpo))
	for clave := range cuerpo {
		claves[clave] = true
	}
	for _, requerida := range []string{"id", "email", "displayName", "emailVerified", "createdAt", "version"} {
		if !claves[requerida] {
			t.Fatalf("falta el campo requerido %q en %v", requerida, cuerpo)
		}
	}
	if len(claves) != 6 {
		t.Fatalf("UserProfile es un objeto cerrado de 6 campos, se obtuvo %v", cuerpo)
	}
	if cuerpo["id"] != e004HTTPUsuario {
		t.Fatalf("id inesperado: %v", cuerpo["id"])
	}
	if cuerpo["emailVerified"] != true {
		t.Fatalf("emailVerified debe emitirse siempre: %v", cuerpo["emailVerified"])
	}
	if cuerpo["createdAt"] != e012HTTPAhora.Add(-24*time.Hour).Format(time.RFC3339) {
		t.Fatalf("createdAt inesperado: %v", cuerpo["createdAt"])
	}
	if cuerpo["version"] != float64(2) {
		t.Fatalf("version inesperada: %v", cuerpo["version"])
	}
	// Ningún atributo interno del item aparece en el cuerpo.
	for _, prohibida := range []string{"passwordHash", "sessionEpoch", "privacyNoticeVersion", "status"} {
		if claves[prohibida] {
			t.Fatalf("el cuerpo no debe incluir %q", prohibida)
		}
	}
}

// TestE012HTTPAutenticacion cubre T02/T07: sin Bearer, esquema inválido y token expirado.
func TestE012HTTPAutenticacion(t *testing.T) {
	store := &e012HTTPStore{registros: map[string]*appProfile.Record{}}
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
			rec := httptest.NewRecorder()
			peticion := e004Peticion(http.MethodGet, "/api/v1/users/me", "", "")
			if caso.cabecera != "" {
				peticion.Header.Set("Authorization", caso.cabecera)
			}
			e012HTTPHandler(store, newE004Sesiones()).ServeHTTP(rec, peticion)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("esperado 401, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
		})
	}
	if len(store.consultas) != 0 {
		t.Fatalf("no debe consultarse la persistencia sin token valido: %v", store.consultas)
	}
}

// TestE012HTTPAislamientoEntreSujetos cubre T03: cada token lee su propio perfil.
func TestE012HTTPAislamientoEntreSujetos(t *testing.T) {
	store := &e012HTTPStore{registros: map[string]*appProfile.Record{
		e004HTTPUsuario:  e012HTTPRegistro(e004HTTPUsuario, "titular@example.invalid", 1),
		e004HTTPOtroUser: e012HTTPRegistro(e004HTTPOtroUser, "ajeno@example.invalid", 3),
	}}
	handler := e012HTTPHandler(store, newE004Sesiones())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, e004Peticion(http.MethodGet, "/api/v1/users/me", "jwt-valido", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("el titular debe obtener 200, obtuvo %d", rec.Code)
	}
	recOtro := httptest.NewRecorder()
	handler.ServeHTTP(recOtro, e004Peticion(http.MethodGet, "/api/v1/users/me", "jwt-sujeto-ajeno", ""))
	if recOtro.Code != http.StatusOK {
		t.Fatalf("el segundo sujeto debe obtener su propio perfil, obtuvo %d", recOtro.Code)
	}
	if !strings.Contains(recOtro.Body.String(), "ajeno@example.invalid") {
		t.Fatalf("el segundo sujeto debe leer su correo, obtuvo %s", recOtro.Body.String())
	}
	if strings.Contains(recOtro.Body.String(), "titular@example.invalid") {
		t.Fatal("la respuesta de un sujeto no puede contener el perfil de otro")
	}
	if len(store.consultas) != 2 || store.consultas[0] != e004HTTPUsuario || store.consultas[1] != e004HTTPOtroUser {
		t.Fatalf("la persistencia debe recibir cada sujeto verificado: %v", store.consultas)
	}
}

// TestE012HTTPPerfilInexistente responde 404 sin enumerar.
func TestE012HTTPPerfilInexistente(t *testing.T) {
	store := &e012HTTPStore{registros: map[string]*appProfile.Record{}}
	rec := httptest.NewRecorder()
	e012HTTPHandler(store, newE004Sesiones()).ServeHTTP(rec, e004Peticion(http.MethodGet, "/api/v1/users/me", "jwt-valido", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("esperado 404, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "NOT_FOUND") {
		t.Fatalf("el sobre debe usar NOT_FOUND: %s", rec.Body.String())
	}
}

// TestE012HTTPFallaCerrado cubre la ausencia de componentes y un fallo de persistencia.
func TestE012HTTPFallaCerrado(t *testing.T) {
	casos := []struct {
		nombre   string
		handler  *internalhttp.ProfileHandler
		esperado int
	}{
		{
			nombre:   "sin autorizador de sesiones",
			handler:  e012HTTPHandler(&e012HTTPStore{}, nil),
			esperado: http.StatusServiceUnavailable,
		},
		{
			nombre:   "sin servicio",
			handler:  e012HTTPHandler(nil, newE004Sesiones()),
			esperado: http.StatusServiceUnavailable,
		},
		{
			nombre: "estado de sesion indisponible",
			handler: func() *internalhttp.ProfileHandler {
				sesiones := newE004Sesiones()
				sesiones.fallo = ports.ErrSessionUnavailable
				return e012HTTPHandler(&e012HTTPStore{}, sesiones)
			}(),
			esperado: http.StatusServiceUnavailable,
		},
		{
			nombre: "persistencia caida",
			handler: e012HTTPHandler(&e012HTTPStore{
				fallo: errors.New("dynamodb no disponible"),
			}, newE004Sesiones()),
			esperado: http.StatusServiceUnavailable,
		},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			caso.handler.ServeHTTP(rec, e004Peticion(http.MethodGet, "/api/v1/users/me", "jwt-valido", ""))
			if rec.Code != caso.esperado {
				t.Fatalf("esperado %d, obtenido %d (%s)", caso.esperado, rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "DEPENDENCY_UNAVAILABLE") {
				t.Fatalf("el sobre debe usar DEPENDENCY_UNAVAILABLE: %s", rec.Body.String())
			}
		})
	}
}

// TestE012HTTPMetodoNoImplementado garantiza que el handler no sirve verbos no previstos en
// esta unidad, aunque el router ya los rechace por su cuenta.
func TestE012HTTPMetodoNoImplementado(t *testing.T) {
	store := &e012HTTPStore{registros: map[string]*appProfile.Record{
		e004HTTPUsuario: e012HTTPRegistro(e004HTTPUsuario, "ejemplo@example.invalid", 1),
	}}
	rec := httptest.NewRecorder()
	e012HTTPHandler(store, newE004Sesiones()).ServeHTTP(rec, e004Peticion(http.MethodDelete, "/api/v1/users/me", "jwt-valido", ""))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("esperado 405, obtenido %d", rec.Code)
	}
	if allow := rec.Header().Get("Allow"); !strings.Contains(allow, http.MethodGet) {
		t.Fatalf("Allow debe anunciar GET, obtuvo %q", allow)
	}
}
