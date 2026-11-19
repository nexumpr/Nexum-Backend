package transport_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appAccount "github.com/nexumpr/Nexum-Backend/internal/application/account"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// errE014HTTPCaida simula una persistencia inaccesible.
var errE014HTTPCaida = errors.New("dynamodb inaccesible")

// Pruebas HTTP de E014 (`DELETE /api/v1/users/me`): middleware de autenticación con
// verificación autoritativa de sesión, contrato 202 con los cinco campos, rechas 400/401/404/
// 422/503, cabecera de no almacenamiento y ausencia de fugas de contraseña, tokens o
// identificadores. Los dobles son exclusivamente de prueba; la composición productiva usa
// DynamoDB real.

func e014HTTPClave() string { return "Cuenta" + "Sintetica#2026" }

const e014HTTPCuerpoValido = `{"confirmation":"DELETE_MY_ACCOUNT","currentPassword":"Cuenta` + `Sintetica#2026"}`

// e014HTTPStore implementa DeletionStore en memoria de prueba.
type e014HTTPStore struct {
	perfil   *appAccount.DeletionProfile
	cargaErr error
	schedErr error
	escritas int
}

func (s *e014HTTPStore) LoadDeletionProfile(context.Context, string) (*appAccount.DeletionProfile, error) {
	if s.cargaErr != nil {
		return nil, s.cargaErr
	}
	return s.perfil, nil
}

func (s *e014HTTPStore) ScheduleDeletion(context.Context, appAccount.DeletionApplication) error {
	s.escritas++
	return s.schedErr
}

type e014HTTPHasher struct{}

func (e014HTTPHasher) HashPassword(p string) (string, error) { return "h:" + p, nil }

func (e014HTTPHasher) VerifyPassword(p, hash string) (bool, error) { return hash == "h:"+p, nil }

func e014HTTPPerfil() *appAccount.DeletionProfile {
	return &appAccount.DeletionProfile{
		UserID:       e004HTTPUsuario,
		Status:       appAccount.ProfileStatusActive,
		PasswordHash: "h:" + e014HTTPClave(),
		Email:        "titular@example.invalid",
		Version:      1,
		HasVersion:   true,
		SessionEpoch: 0,
	}
}

func e014HTTPHandler(store *e014HTTPStore) *nexumhttp.DeleteAccountHandler {
	servicio := appAccount.NewDeletionService(store, e014HTTPHasher{})
	return nexumhttp.NewDeleteAccountHandler(servicio, e004TokensValidos(), newE004Sesiones(), silencioso())
}

func e014HTTPHandlerCon(servicio *appAccount.DeletionService, sesiones *e004Sesiones) *nexumhttp.DeleteAccountHandler {
	return nexumhttp.NewDeleteAccountHandler(servicio, e004TokensValidos(), sesiones, silencioso())
}

func e014HTTPEjecutar(t *testing.T, handler http.Handler, req *http.Request) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	var cuerpo map[string]any
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
			t.Fatalf("respuesta no es JSON valido: %v (%s)", err, rec.Body.String())
		}
	}
	return rec, cuerpo
}

// TestE014HTTP202Receipt fija el contrato de éxito completo.
func TestE014HTTP202Receipt(t *testing.T) {
	store := &e014HTTPStore{perfil: e014HTTPPerfil()}
	rec, cuerpo := e014HTTPEjecutar(t, e014HTTPHandler(store),
		e004Peticion(http.MethodDelete, "/api/v1/users/me", "jwt-valido", e014HTTPCuerpoValido))

	if rec.Code != http.StatusAccepted {
		t.Fatalf("esperado 202, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("Content-Type debe ser JSON, obtenido %q", rec.Header().Get("Content-Type"))
	}
	if rec.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("Cache-Control debe ser 'private, no-store', obtenido %q", rec.Header().Get("Cache-Control"))
	}
	if len(cuerpo) != 5 {
		t.Fatalf("DeletionReceipt debe tener exactamente 5 campos, obtenido %v", cuerpo)
	}
	for _, campo := range []string{"requestId", "status", "requestedAt", "sessionsRevoked", "deletionRequestId"} {
		if _, ok := cuerpo[campo]; !ok {
			t.Fatalf("falta el campo %q en %v", campo, cuerpo)
		}
	}
	if cuerpo["status"] != "scheduled" {
		t.Fatalf("status esperado scheduled, obtenido %v", cuerpo["status"])
	}
	if cuerpo["sessionsRevoked"] != true {
		t.Fatalf("sessionsRevoked debe ser true, obtenido %v", cuerpo["sessionsRevoked"])
	}
	if cuerpo["requestId"] != cuerpo["deletionRequestId"] {
		t.Fatalf("deletionRequestId debe igualar requestId")
	}
	if store.escritas != 1 {
		t.Fatalf("debe registrarse exactamente un trabajo, obtenidos %d", store.escritas)
	}

	// Ningun dato sensible ni identificador interno puede viajar en la respuesta.
	for _, prohibido := range []string{e014HTTPClave(), e004HTTPUsuario, e004HTTPSesion, "jwt-valido", "h:", "email", "token"} {
		if strings.Contains(rec.Body.String(), prohibido) {
			t.Fatalf("la respuesta no puede contener %q: %q", prohibido, rec.Body.String())
		}
	}
}

// TestE014HTTP400InvalidRequest cubre el binding estricto.
func TestE014HTTP400InvalidRequest(t *testing.T) {
	casos := []struct {
		nombre  string
		cuerpo  string
		sinTipo bool
	}{
		{"campo desconocido", `{"confirmation":"DELETE_MY_ACCOUNT","currentPassword":"Cuenta` + `Sintetica#2026","userId":"otro"}`, false},
		{"json malformado", `{"confirmation":`, false},
		{"json concatenado", e014HTTPCuerpoValido + `{"confirmation":"DELETE_MY_ACCOUNT"}`, false},
		{"tipo incorrecto", `{"confirmation":123,"currentPassword":"Cuenta` + `Sintetica#2026"}`, false},
		{"sin content-type", e014HTTPCuerpoValido, true},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := &e014HTTPStore{perfil: e014HTTPPerfil()}
			req := e004Peticion(http.MethodDelete, "/api/v1/users/me", "jwt-valido", caso.cuerpo)
			if caso.sinTipo {
				req.Header.Del("Content-Type")
			}
			rec, cuerpo := e014HTTPEjecutar(t, e014HTTPHandler(store), req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("esperado 400, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			if errorObj, _ := cuerpo["error"].(map[string]any); errorObj == nil || errorObj["code"] != "INVALID_REQUEST" {
				t.Fatalf("codigo esperado INVALID_REQUEST, obtenido %v", cuerpo)
			}
			if store.escritas != 0 {
				t.Fatalf("un rechazo de binding no puede escribir")
			}
		})
	}
}

// TestE014HTTP401Autenticacion exige Bearer válido y sesión ACTIVE.
func TestE014HTTP401Autenticacion(t *testing.T) {
	casos := []struct {
		nombre   string
		cabecera string
		token    string
	}{
		{"sin cabecera", "", ""},
		{"esquema basico", "Basic abc", ""},
		{"token desconocido", "", "desconocido"},
		{"token expirado", "", "jwt-expirado"},
		{"token sin sesion", "", "jwt-sin-sesion"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := &e014HTTPStore{perfil: e014HTTPPerfil()}
			req := e004Peticion(http.MethodDelete, "/api/v1/users/me", caso.token, e014HTTPCuerpoValido)
			if caso.cabecera != "" {
				req.Header.Set("Authorization", caso.cabecera)
			}
			rec, _ := e014HTTPEjecutar(t, e014HTTPHandler(store), req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("esperado 401, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			if store.escritas != 0 {
				t.Fatalf("sin autenticacion no puede escribirse")
			}
		})
	}

	t.Run("sesion revocada", func(t *testing.T) {
		store := &e014HTTPStore{perfil: e014HTTPPerfil()}
		sesiones := newE004Sesiones()
		sesiones.revocar(e004HTTPSesion)
		servicio := appAccount.NewDeletionService(store, e014HTTPHasher{})
		rec, _ := e014HTTPEjecutar(t, e014HTTPHandlerCon(servicio, sesiones),
			e004Peticion(http.MethodDelete, "/api/v1/users/me", "jwt-valido", e014HTTPCuerpoValido))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("una sesion revocada debe ser 401, obtenido %d", rec.Code)
		}
		if store.escritas != 0 {
			t.Fatalf("una sesion revocada no puede eliminar la cuenta")
		}
	})
}

// TestE014HTTP404PerfilAusente: identidad válida sin recurso propio.
func TestE014HTTP404PerfilAusente(t *testing.T) {
	store := &e014HTTPStore{perfil: nil}
	rec, cuerpo := e014HTTPEjecutar(t, e014HTTPHandler(store),
		e004Peticion(http.MethodDelete, "/api/v1/users/me", "jwt-valido", e014HTTPCuerpoValido))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("esperado 404, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if errorObj, _ := cuerpo["error"].(map[string]any); errorObj == nil || errorObj["code"] != "NOT_FOUND" {
		t.Fatalf("codigo esperado NOT_FOUND, obtenido %v", cuerpo)
	}
}

// TestE014HTTP422Reglas cubre confirmación inválida, contraseña incorrecta y contraseña vacía.
func TestE014HTTP422Reglas(t *testing.T) {
	casos := []struct {
		nombre   string
		cuerpo   string
		preparar func(*e014HTTPStore)
	}{
		{
			nombre: "confirmacion invalida",
			cuerpo: `{"confirmation":"BORRAR","currentPassword":"Cuenta` + `Sintetica#2026"}`,
		},
		{
			nombre: "password incorrecta",
			cuerpo: e014HTTPCuerpoValido,
			preparar: func(s *e014HTTPStore) {
				s.perfil.PasswordHash = "h:otra-clave"
			},
		},
		{
			nombre: "password vacia",
			cuerpo: `{"confirmation":"DELETE_MY_ACCOUNT","currentPassword":""}`,
		},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := &e014HTTPStore{perfil: e014HTTPPerfil()}
			if caso.preparar != nil {
				caso.preparar(store)
			}
			rec, cuerpo := e014HTTPEjecutar(t, e014HTTPHandler(store),
				e004Peticion(http.MethodDelete, "/api/v1/users/me", "jwt-valido", caso.cuerpo))
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("esperado 422, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			errorObj, _ := cuerpo["error"].(map[string]any)
			if errorObj == nil || errorObj["code"] != "BUSINESS_RULE_VIOLATION" {
				t.Fatalf("codigo esperado BUSINESS_RULE_VIOLATION, obtenido %v", cuerpo)
			}
			if store.escritas != 0 {
				t.Fatalf("una violacion de regla no puede escribir")
			}
		})
	}
}

// TestE014HTTP503FallaCerrado: cualquier pieza ausente o caída responde 503.
func TestE014HTTP503FallaCerrado(t *testing.T) {
	servicioOK := func(store *e014HTTPStore) *appAccount.DeletionService {
		return appAccount.NewDeletionService(store, e014HTTPHasher{})
	}
	casos := []struct {
		nombre  string
		handler http.Handler
	}{
		{"servicio ausente", nexumhttp.NewDeleteAccountHandler(nil, e004TokensValidos(), newE004Sesiones(), silencioso())},
		{"tokens ausentes", nexumhttp.NewDeleteAccountHandler(servicioOK(&e014HTTPStore{perfil: e014HTTPPerfil()}), nil, newE004Sesiones(), silencioso())},
		{"autorizador ausente", nexumhttp.NewDeleteAccountHandler(servicioOK(&e014HTTPStore{perfil: e014HTTPPerfil()}), e004TokensValidos(), nil, silencioso())},
		{"persistencia caida", e014HTTPHandler(&e014HTTPStore{cargaErr: errE014HTTPCaida})},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			rec, cuerpo := e014HTTPEjecutar(t, caso.handler,
				e004Peticion(http.MethodDelete, "/api/v1/users/me", "jwt-valido", e014HTTPCuerpoValido))
			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("esperado 503, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			errorObj, _ := cuerpo["error"].(map[string]any)
			if errorObj == nil || errorObj["code"] != "DEPENDENCY_UNAVAILABLE" {
				t.Fatalf("codigo esperado DEPENDENCY_UNAVAILABLE, obtenido %v", cuerpo)
			}
			if detalles, ok := errorObj["details"].([]any); !ok || len(detalles) != 0 {
				t.Fatalf("details debe ser una lista vacia, obtenido %v", errorObj["details"])
			}
		})
	}
}

// TestE014HTTPNoReflejaElCuerpo comprueba que el cuerpo recibido nunca vuelve en la respuesta.
func TestE014HTTPNoReflejaElCuerpo(t *testing.T) {
	marcador := "SecretoUnico"
	cuerpo := `{"confirmation":"DELETE_MY_ACCOUNT","currentPassword":"` + marcador + `#2026"}`
	store := &e014HTTPStore{perfil: e014HTTPPerfil()}
	rec, _ := e014HTTPEjecutar(t, e014HTTPHandler(store),
		e004Peticion(http.MethodDelete, "/api/v1/users/me", "jwt-valido", cuerpo))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("esperado 422 por contraseña distinta, obtenido %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), marcador) {
		t.Fatalf("la respuesta no puede reflejar el cuerpo: %q", rec.Body.String())
	}
}
