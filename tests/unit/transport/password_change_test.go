package transport_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// Pruebas HTTP de E007 (`POST /api/v1/auth/password/change`): middleware de autenticación con
// verificación autoritativa de sesión, contrato exacto `200 Accepted {status, requestId}`,
// rechazos 400/401/409/422/429/503 y ausencia de fugas de contraseñas, tokens o `changedAt`.
// Los dobles son exclusivamente de prueba; la composición productiva usa DynamoDB real.

const e007CuerpoValido = `{"currentPassword":"OldPassphrase#2026","newPassword":"NewPassphrase#2026"}`

var e007UUIDv4 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// e007Limiter registra la clave con la que se evalúa la cuota.
type e007Limiter struct {
	allow  bool
	retry  time.Duration
	err    error
	claves []string
}

func (l *e007Limiter) Permitir(_ context.Context, clave string) (bool, time.Duration, error) {
	l.claves = append(l.claves, clave)
	if l.err != nil {
		return false, 0, l.err
	}
	return l.allow, l.retry, nil
}

// e007Store es el puerto PasswordChangeStore en memoria de prueba.
type e007Store struct {
	profile  *appIdentity.PasswordChangeProfile
	applyErr error
	aplicado *appIdentity.PasswordChangeApplication
}

func (s *e007Store) LoadPasswordChangeProfile(context.Context, string) (*appIdentity.PasswordChangeProfile, error) {
	return s.profile, nil
}

func (s *e007Store) ApplyPasswordChange(_ context.Context, apply appIdentity.PasswordChangeApplication) error {
	s.aplicado = &apply
	return s.applyErr
}

// e007Hasher es determinista y barato: el flujo con Argon2id real se cubre en
// tests/unit/application.
type e007Hasher struct{}

func (e007Hasher) HashPassword(password string) (string, error) { return "hash:" + password, nil }

func (e007Hasher) VerifyPassword(password, hash string) (bool, error) {
	return hash == "hash:"+password, nil
}

func e007PerfilActivo() *appIdentity.PasswordChangeProfile {
	return &appIdentity.PasswordChangeProfile{
		UserID:       e004HTTPUsuario,
		Status:       appIdentity.StatusActive,
		PasswordHash: "hash:OldPassphrase#2026",
		Version:      1,
		HasVersion:   true,
		SessionEpoch: 0,
	}
}

func e007Handler(store *e007Store, limiter nexumhttp.PasswordChangeLimiter) *nexumhttp.PasswordChangeHandler {
	return e007HandlerConSesiones(store, limiter, newE004Sesiones())
}

func e007HandlerConSesiones(
	store *e007Store,
	limiter nexumhttp.PasswordChangeLimiter,
	sesiones *e004Sesiones,
) *nexumhttp.PasswordChangeHandler {
	servicio := appIdentity.NewPasswordChangeService(store, e007Hasher{})
	return nexumhttp.NewPasswordChangeHandler(servicio, limiter, e004TokensValidos(), sesiones, silencioso())
}

func e007Peticion(token, cuerpo string) *http.Request {
	return e004Peticion(http.MethodPost, "/api/v1/auth/password/change", token, cuerpo)
}

func e007Ejecutar(t *testing.T, handler http.Handler, req *http.Request) (*httptest.ResponseRecorder, map[string]any) {
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

// TestE007HTTP200Accepted fija el contrato de éxito: exactamente `status` y `requestId`.
func TestE007HTTP200Accepted(t *testing.T) {
	store := &e007Store{profile: e007PerfilActivo()}
	limiter := &e007Limiter{allow: true}
	handler := e007Handler(store, limiter)

	rec, cuerpo := e007Ejecutar(t, handler, e007Peticion("jwt-valido", e007CuerpoValido))
	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if cuerpo["status"] != "accepted" {
		t.Fatalf("status esperado accepted, obtenido %v", cuerpo["status"])
	}
	id, _ := cuerpo["requestId"].(string)
	if !e007UUIDv4.MatchString(id) {
		t.Fatalf("requestId debe ser UUID v4, obtenido %q", id)
	}
	if len(cuerpo) != 2 {
		t.Fatalf("Accepted solo admite status y requestId, obtenido %v", cuerpo)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("la respuesta no puede cachearse, Cache-Control=%q", rec.Header().Get("Cache-Control"))
	}

	if store.aplicado == nil {
		t.Fatalf("debe aplicarse la transaccion")
	}
	// La titularidad viene del claim `sub`, no del cuerpo.
	if store.aplicado.UserID != e004HTTPUsuario {
		t.Fatalf("el userId debe venir del token, obtenido %q", store.aplicado.UserID)
	}
	if store.aplicado.PasswordHash != "hash:NewPassphrase#2026" {
		t.Fatalf("el hash aplicado no corresponde a la contrasena nueva: %q", store.aplicado.PasswordHash)
	}
	if store.aplicado.ExpectedSessionEpoch != 0 || store.aplicado.NextSessionEpoch != 1 {
		t.Fatalf("la epoca debe incrementarse 0->1, obtenido %d->%d",
			store.aplicado.ExpectedSessionEpoch, store.aplicado.NextSessionEpoch)
	}

	// La cuota se evalúa una vez y con el identificador autenticado.
	if len(limiter.claves) != 1 || limiter.claves[0] != e004HTTPUsuario {
		t.Fatalf("la cuota debe evaluarse con el userId autenticado, obtenidas %v", limiter.claves)
	}
}

// TestE007HTTPAutenticacion exige Bearer válido y sesión ACTIVE con época vigente.
func TestE007HTTPAutenticacion(t *testing.T) {
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
			store := &e007Store{profile: e007PerfilActivo()}
			req := e007Peticion(caso.token, e007CuerpoValido)
			if caso.cabecera != "" {
				req.Header.Set("Authorization", caso.cabecera)
			}
			rec, cuerpo := e007Ejecutar(t, e007Handler(store, &e007Limiter{allow: true}), req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("esperado 401, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			errorObj, _ := cuerpo["error"].(map[string]any)
			if errorObj == nil {
				t.Fatalf("debe devolver ErrorEnvelope, obtenido %v", cuerpo)
			}
			if store.aplicado != nil {
				t.Fatalf("sin autenticacion no puede aplicarse la transaccion")
			}
		})
	}

	// Sesión revocada en el estado compartido: 401 y el caso de uso no participa.
	t.Run("sesion revocada", func(t *testing.T) {
		store := &e007Store{profile: e007PerfilActivo()}
		sesiones := newE004Sesiones()
		sesiones.revocar(e004HTTPSesion)
		rec, _ := e007Ejecutar(t, e007HandlerConSesiones(store, &e007Limiter{allow: true}, sesiones),
			e007Peticion("jwt-valido", e007CuerpoValido))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("una sesion revocada debe ser 401, obtenido %d", rec.Code)
		}
		if store.aplicado != nil {
			t.Fatalf("una sesion revocada no puede cambiar la contrasena")
		}
	})
}

// TestE007HTTPRechazosDelContrato cubre 400/409/422/429/503 con códigos exactos.
func TestE007HTTPRechazosDelContrato(t *testing.T) {
	casos := []struct {
		nombre    string
		cuerpo    string
		espera    int
		codigo    string
		preparar  func(*e007Store, *e007Limiter)
		sinTipo   bool
		esperaHdr string
	}{
		{nombre: "campo desconocido", cuerpo: `{"currentPassword":"OldPassphrase#2026","newPassword":"NewPassphrase#2026","userId":"otro"}`, espera: http.StatusBadRequest, codigo: "INVALID_REQUEST"},
		{nombre: "json malformado", cuerpo: `{"currentPassword":`, espera: http.StatusBadRequest, codigo: "INVALID_REQUEST"},
		{nombre: "json concatenado", cuerpo: e007CuerpoValido + `{"currentPassword":"x"}`, espera: http.StatusBadRequest, codigo: "INVALID_REQUEST"},
		{nombre: "tipo incorrecto", cuerpo: `{"currentPassword":123,"newPassword":"NewPassphrase#2026"}`, espera: http.StatusBadRequest, codigo: "INVALID_REQUEST"},
		{nombre: "sin content-type", cuerpo: e007CuerpoValido, espera: http.StatusBadRequest, codigo: "INVALID_REQUEST", sinTipo: true},
		{nombre: "actual vacia", cuerpo: `{"currentPassword":"","newPassword":"NewPassphrase#2026"}`, espera: http.StatusUnprocessableEntity, codigo: "BUSINESS_RULE_VIOLATION"},
		{nombre: "nueva corta", cuerpo: `{"currentPassword":"OldPassphrase#2026","newPassword":"corta"}`, espera: http.StatusUnprocessableEntity, codigo: "BUSINESS_RULE_VIOLATION"},
		{nombre: "actual incorrecta", cuerpo: e007CuerpoValido, espera: http.StatusUnprocessableEntity, codigo: "BUSINESS_RULE_VIOLATION", preparar: func(s *e007Store, _ *e007Limiter) {
			s.profile.PasswordHash = "hash:OtraPassphrase#2026"
		}},
		{nombre: "conflicto de concurrencia", cuerpo: e007CuerpoValido, espera: http.StatusConflict, codigo: "CONFLICT", preparar: func(s *e007Store, _ *e007Limiter) {
			s.applyErr = appIdentity.ErrPasswordChangeConflict
		}},
		{nombre: "cuota agotada", cuerpo: e007CuerpoValido, espera: http.StatusTooManyRequests, codigo: "RATE_LIMITED", esperaHdr: "900", preparar: func(_ *e007Store, l *e007Limiter) {
			l.allow = false
			l.retry = 15 * time.Minute
		}},
		{nombre: "limitador caido", cuerpo: e007CuerpoValido, espera: http.StatusServiceUnavailable, codigo: "DEPENDENCY_UNAVAILABLE", preparar: func(_ *e007Store, l *e007Limiter) {
			l.err = errors.New("redis caido")
		}},
		{nombre: "persistencia caida", cuerpo: e007CuerpoValido, espera: http.StatusServiceUnavailable, codigo: "DEPENDENCY_UNAVAILABLE", preparar: func(s *e007Store, _ *e007Limiter) {
			s.applyErr = appIdentity.ErrPasswordChangeUnavailable
		}},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := &e007Store{profile: e007PerfilActivo()}
			limiter := &e007Limiter{allow: true}
			if caso.preparar != nil {
				caso.preparar(store, limiter)
			}
			req := e007Peticion("jwt-valido", caso.cuerpo)
			if caso.sinTipo {
				req.Header.Del("Content-Type")
			}
			rec, cuerpo := e007Ejecutar(t, e007Handler(store, limiter), req)
			if rec.Code != caso.espera {
				t.Fatalf("esperado %d, obtenido %d (%s)", caso.espera, rec.Code, rec.Body.String())
			}
			errorObj, _ := cuerpo["error"].(map[string]any)
			if errorObj == nil || errorObj["code"] != caso.codigo {
				t.Fatalf("codigo esperado %q, obtenido %v", caso.codigo, cuerpo)
			}
			if caso.esperaHdr != "" && rec.Header().Get("Retry-After") != caso.esperaHdr {
				t.Fatalf("Retry-After esperado %q, obtenido %q", caso.esperaHdr, rec.Header().Get("Retry-After"))
			}
			if caso.espera != http.StatusOK && store.aplicado != nil {
				// La cuota y los rechazos previos ocurren antes de la transacción; sólo el
				// conflicto y el fallo de persistencia llegan a emitirla (sin escribir).
				if caso.codigo != "CONFLICT" && caso.codigo != "DEPENDENCY_UNAVAILABLE" {
					t.Fatalf("un rechazo no puede aplicar la transaccion")
				}
			}
		})
	}
}

// TestE007HTTPFallaCerradoSinComponentes: cualquier pieza ausente responde 503.
func TestE007HTTPFallaCerradoSinComponentes(t *testing.T) {
	req := func() *http.Request { return e007Peticion("jwt-valido", e007CuerpoValido) }

	casos := []struct {
		nombre  string
		handler http.Handler
	}{
		{"servicio ausente", nexumhttp.NewPasswordChangeHandler(nil, &e007Limiter{allow: true}, e004TokensValidos(), newE004Sesiones(), silencioso())},
		{"hasher ausente", nexumhttp.NewPasswordChangeHandler(
			appIdentity.NewPasswordChangeService(&e007Store{profile: e007PerfilActivo()}, nil),
			&e007Limiter{allow: true}, e004TokensValidos(), newE004Sesiones(), silencioso())},
		{"limitador ausente", nexumhttp.NewPasswordChangeHandler(
			appIdentity.NewPasswordChangeService(&e007Store{profile: e007PerfilActivo()}, e007Hasher{}),
			nil, e004TokensValidos(), newE004Sesiones(), silencioso())},
		{"tokens ausentes", nexumhttp.NewPasswordChangeHandler(
			appIdentity.NewPasswordChangeService(&e007Store{profile: e007PerfilActivo()}, e007Hasher{}),
			&e007Limiter{allow: true}, nil, newE004Sesiones(), silencioso())},
		{"autorizador ausente", nexumhttp.NewPasswordChangeHandler(
			appIdentity.NewPasswordChangeService(&e007Store{profile: e007PerfilActivo()}, e007Hasher{}),
			&e007Limiter{allow: true}, e004TokensValidos(), nil, silencioso())},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			rec, cuerpo := e007Ejecutar(t, caso.handler, req())
			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("esperado 503, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			errorObj, _ := cuerpo["error"].(map[string]any)
			if errorObj == nil || errorObj["code"] != "DEPENDENCY_UNAVAILABLE" {
				t.Fatalf("codigo esperado DEPENDENCY_UNAVAILABLE, obtenido %v", cuerpo)
			}
			// Un error de dependencia no filtra detalles internos.
			if detalles, ok := errorObj["details"].([]any); !ok || len(detalles) != 0 {
				t.Fatalf("details debe ser una lista vacia, obtenido %v", errorObj["details"])
			}
		})
	}
}

// TestE007HTTPRespuestaSinDatosSensibles comprueba que ni el éxito ni los errores exponen
// contraseñas, tokens, identificadores de sesión ni marcas de cambio.
func TestE007HTTPRespuestaSinDatosSensibles(t *testing.T) {
	store := &e007Store{profile: e007PerfilActivo()}
	rec, _ := e007Ejecutar(t, e007Handler(store, &e007Limiter{allow: true}),
		e007Peticion("jwt-valido", e007CuerpoValido))
	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d", rec.Code)
	}

	for _, prohibido := range []string{
		"accessToken", "refreshToken", "changedAt", "passwordHash", "expiresIn",
		"sessionId", "token", "OldPassphrase#2026", "NewPassphrase#2026",
		e004HTTPUsuario, e004HTTPSesion,
	} {
		if strings.Contains(rec.Body.String(), prohibido) {
			t.Fatalf("la respuesta no puede contener %q: %q", prohibido, rec.Body.String())
		}
	}

	// Tampoco en el caso de error más informativo (conflicto).
	store.applyErr = appIdentity.ErrPasswordChangeConflict
	recConflicto, _ := e007Ejecutar(t, e007Handler(store, &e007Limiter{allow: true}),
		e007Peticion("jwt-valido", e007CuerpoValido))
	if recConflicto.Code != http.StatusConflict {
		t.Fatalf("esperado 409, obtenido %d", recConflicto.Code)
	}
	for _, prohibido := range []string{"OldPassphrase#2026", "NewPassphrase#2026", e004HTTPUsuario, e004HTTPSesion} {
		if strings.Contains(recConflicto.Body.String(), prohibido) {
			t.Fatalf("el error no puede contener %q: %q", prohibido, recConflicto.Body.String())
		}
	}

	// El cuerpo recibido nunca se refleja en la respuesta.
	eco := `{"currentPassword":"SecretoUnico#2026","newPassword":"OtroSecretoUnico#2026"}`
	recEco, _ := e007Ejecutar(t, e007Handler(&e007Store{profile: e007PerfilActivo()}, &e007Limiter{allow: true}),
		e007Peticion("jwt-valido", eco))
	if strings.Contains(recEco.Body.String(), "SecretoUnico#2026") {
		t.Fatalf("la respuesta no puede reflejar el cuerpo: %q", recEco.Body.String())
	}
}

// TestE007HTTPCuerpoGrandeRechazado acota el tamaño antes de derivar Argon2id.
func TestE007HTTPCuerpoGrandeRechazado(t *testing.T) {
	grande := `{"currentPassword":"` + strings.Repeat("a", int(nexumhttp.PasswordChangeMaxRequestBodyBytes)) + `","newPassword":"NewPassphrase#2026"}`
	store := &e007Store{profile: e007PerfilActivo()}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password/change", bytes.NewBufferString(grande))
	req.Header.Set("Authorization", "Bearer jwt-valido")
	req.Header.Set("Content-Type", "application/json")

	rec, _ := e007Ejecutar(t, e007Handler(store, &e007Limiter{allow: true}), req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("un cuerpo por encima del limite debe ser 400, obtenido %d", rec.Code)
	}
	if store.aplicado != nil {
		t.Fatalf("un cuerpo rechazado no puede aplicar la transaccion")
	}
}
