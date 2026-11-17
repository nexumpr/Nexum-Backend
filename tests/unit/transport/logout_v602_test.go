package transport_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
)

// Pruebas HTTP de E004 (`POST /api/v1/auth/logout`): middleware de autenticación, verificación
// autoritativa de sesión, contrato 204 sin cuerpo y traducción de errores. Los dobles son
// exclusivamente de prueba; la composición productiva usa DynamoDB real.

const (
	e004HTTPUsuario  = "11111111-2222-4333-8444-555555555555"
	e004HTTPSesion   = "018f90ef-08e5-7a68-b09f-0000000000c1"
	e004HTTPSesionB  = "018f90ef-08e5-7a68-b09f-0000000000c2"
	e004HTTPOtroUser = "99999999-8888-4777-8666-555555555555"
)

var e004HTTPAhora = time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)

// e004Tokens implementa ports.TokenProvider con tokens literales.
type e004Tokens struct {
	emitidos map[string]ports.TokenClaims
	errores  map[string]error
}

func (p *e004Tokens) GenerateAccessToken(context.Context, ports.TokenClaims) (string, error) {
	return "emitido", nil
}

func (p *e004Tokens) GenerateRefreshToken(context.Context) (string, error) { return "refresh", nil }

func (p *e004Tokens) ValidateAccessToken(_ context.Context, token string) (*ports.TokenClaims, error) {
	if err, ok := p.errores[token]; ok {
		return nil, err
	}
	claims, ok := p.emitidos[token]
	if !ok {
		return nil, ports.ErrInvalidToken
	}
	copia := claims
	return &copia, nil
}

// e004Sesiones es el autorizador de sesión: refleja el estado autoritativo.
type e004Sesiones struct {
	mu        sync.Mutex
	revocada  map[string]bool
	fallo     error
	consultas int
}

func newE004Sesiones() *e004Sesiones {
	return &e004Sesiones{revocada: map[string]bool{}}
}

func (s *e004Sesiones) AuthorizeSession(_ context.Context, userID, sessionID string, _ int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.consultas++
	if s.fallo != nil {
		return s.fallo
	}
	if userID != e004HTTPUsuario && userID != e004HTTPOtroUser {
		return ports.ErrSessionUnauthorized
	}
	if s.revocada[sessionID] {
		return ports.ErrSessionUnauthorized
	}
	if sessionID != e004HTTPSesion && sessionID != e004HTTPSesionB {
		return ports.ErrSessionUnauthorized
	}
	return nil
}

func (s *e004Sesiones) revocar(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revocada[sessionID] = true
}

// TestE004SesionesRevocacionAfectaAlAutorizador garantiza que el estado revocado del doble
// produce 401 en el autorizador, de modo que el helper no es código muerto.
func TestE004SesionesRevocacionAfectaAlAutorizador(t *testing.T) {
	sesiones := newE004Sesiones()
	if err := sesiones.AuthorizeSession(context.Background(), e004HTTPUsuario, e004HTTPSesion, 0); err != nil {
		t.Fatalf("una sesion activa debe autorizar: %v", err)
	}
	sesiones.revocar(e004HTTPSesion)
	if err := sesiones.AuthorizeSession(context.Background(), e004HTTPUsuario, e004HTTPSesion, 0); !errors.Is(err, ports.ErrSessionUnauthorized) {
		t.Fatalf("una sesion revocada debe rechazarse, obtenido %v", err)
	}
}

// TestE004MiddlewareRechazaJWTDeSesionRevocada es la prueba decisiva del middleware: un JWT
// criptográficamente válido cuya sesión fue revocada en el estado compartido NO llega al
// handler protegido. No se apoya en el caso de uso de logout, que aquí no participa.
func TestE004MiddlewareRechazaJWTDeSesionRevocada(t *testing.T) {
	sesiones := newE004Sesiones()
	tokens := e004TokensValidos()

	invocado := false
	protegido := middleware.AuthenticateBearerWithSession(tokens, sesiones)(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			invocado = true
			w.WriteHeader(http.StatusNoContent)
		}),
	)

	// 1. Sesión ACTIVE: el middleware deja pasar y el handler se ejecuta.
	activa := httptest.NewRecorder()
	protegido.ServeHTTP(activa, e004Peticion(http.MethodPost, "/protegido", "jwt-valido", ""))
	if activa.Code != http.StatusNoContent || !invocado {
		t.Fatalf("sesion activa debe alcanzar el handler: code=%d invocado=%v", activa.Code, invocado)
	}

	// 2. El otro nodo revoca la sesión en el estado compartido (equivalente a un logout ya
	//    confirmado). El mismo JWT sigue siendo válido criptográficamente.
	sesiones.revocar(e004HTTPSesion)
	invocado = false

	revocada := httptest.NewRecorder()
	protegido.ServeHTTP(revocada, e004Peticion(http.MethodPost, "/protegido", "jwt-valido", ""))
	if revocada.Code != http.StatusUnauthorized {
		t.Fatalf("el middleware debe rechazar con 401 la sesion revocada, obtenido %d", revocada.Code)
	}
	if invocado {
		t.Fatalf("el handler protegido NO debe ejecutarse con una sesion revocada")
	}
	if corpo := revocada.Body.String(); strings.Contains(corpo, e004HTTPSesion) || strings.Contains(corpo, "jwt-valido") {
		t.Fatalf("la respuesta no debe revelar identificadores: %q", corpo)
	}

	// 3. Una sesión distinta del mismo usuario sigue pasando: la revocación es por sesión.
	otra := httptest.NewRecorder()
	protegido.ServeHTTP(otra, e004Peticion(http.MethodPost, "/protegido", "jwt-otra-sesion", ""))
	if otra.Code != http.StatusNoContent || !invocado {
		t.Fatalf("la segunda sesion debe seguir autorizando: code=%d invocado=%v", otra.Code, invocado)
	}
}

// TestE004MiddlewareFallaCerrado cubre T14 en el middleware: sin poder determinar el estado,
// no hay paso (503) y el handler no se ejecuta.
func TestE004MiddlewareFallaCerrado(t *testing.T) {
	sesiones := newE004Sesiones()
	sesiones.fallo = ports.ErrSessionUnavailable

	invocado := false
	protegido := middleware.AuthenticateBearerWithSession(e004TokensValidos(), sesiones)(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			invocado = true
			w.WriteHeader(http.StatusNoContent)
		}),
	)

	rec := httptest.NewRecorder()
	protegido.ServeHTTP(rec, e004Peticion(http.MethodPost, "/protegido", "jwt-valido", ""))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("estado de sesion indeterminable debe ser 503, obtenido %d", rec.Code)
	}
	if invocado {
		t.Fatalf("el handler no debe ejecutarse si no se pudo verificar la sesion")
	}
}

// e004Almacen es el puerto LogoutStore en memoria de prueba.
type e004Almacen struct {
	mu             sync.Mutex
	sesiones       map[string]*appIdentity.LogoutSession
	falloEscritura error
}

func newE004Almacen() *e004Almacen {
	return &e004Almacen{sesiones: map[string]*appIdentity.LogoutSession{
		e004HTTPSesion: {
			UserID:      e004HTTPUsuario,
			SessionID:   e004HTTPSesion,
			RefreshHash: "digest-c1",
			Status:      appIdentity.StatusActive,
			ExpiresAt:   e004HTTPAhora.Add(30 * 24 * time.Hour),
			HasExpiry:   true,
			Version:     1,
			HasVersion:  true,
		},
		e004HTTPSesionB: {
			UserID:      e004HTTPUsuario,
			SessionID:   e004HTTPSesionB,
			RefreshHash: "digest-c2",
			Status:      appIdentity.StatusActive,
			ExpiresAt:   e004HTTPAhora.Add(30 * 24 * time.Hour),
			HasExpiry:   true,
			Version:     1,
			HasVersion:  true,
		},
	}}
}

func (a *e004Almacen) LoadLogoutSession(_ context.Context, _, sessionID string) (*appIdentity.LogoutSession, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	sesion, ok := a.sesiones[sessionID]
	if !ok {
		return nil, nil
	}
	copia := *sesion
	return &copia, nil
}

func (a *e004Almacen) RevokeLogoutSession(_ context.Context, rev appIdentity.LogoutRevocation) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.falloEscritura != nil {
		return a.falloEscritura
	}
	sesion := a.sesiones[rev.SessionID]
	if sesion == nil {
		return nil
	}
	if rev.HasVersion && sesion.Version != rev.ExpectedVersion {
		return appIdentity.ErrLogoutConflict
	}
	sesion.Status = appIdentity.StatusRevoked
	sesion.Version++
	return nil
}

func e004Handler(almacen *e004Almacen, sesiones *e004Sesiones, tokens *e004Tokens) *internalhttp.LogoutHandler {
	servicio := appIdentity.NewLogoutService(almacen).
		WithClock(func() time.Time { return e004HTTPAhora })
	return internalhttp.NewLogoutHandler(servicio, tokens, sesiones, silencioso())
}

func e004TokensValidos() *e004Tokens {
	return &e004Tokens{emitidos: map[string]ports.TokenClaims{
		"jwt-valido":       tokenClaims(e004HTTPUsuario, e004HTTPSesion, e004HTTPAhora),
		"jwt-otra-sesion":  tokenClaims(e004HTTPUsuario, e004HTTPSesionB, e004HTTPAhora),
		"jwt-sin-sesion":   {Subject: e004HTTPUsuario},
		"jwt-sujeto-ajeno": tokenClaims(e004HTTPOtroUser, e004HTTPSesion, e004HTTPAhora),
	}, errores: map[string]error{
		"jwt-expirado": ports.ErrTokenExpired,
	}}
}

func e004Peticion(metodo, ruta, token, cuerpo string) *http.Request {
	var lector *strings.Reader
	if cuerpo == "" {
		lector = strings.NewReader("")
	} else {
		lector = strings.NewReader(cuerpo)
	}
	req := httptest.NewRequest(metodo, ruta, lector)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if cuerpo != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

// TestE004HTTPAutenticacion cubre T07: sin Bearer, esquema inválido, token inválido y expirado.
func TestE004HTTPAutenticacion(t *testing.T) {
	casos := []struct {
		nombre   string
		cabecera string
		esperado int
	}{
		{"sin cabecera", "", http.StatusUnauthorized},
		{"esquema basico", "Basic abc", http.StatusUnauthorized},
		{"token desconocido", "Bearer desconocido", http.StatusUnauthorized},
		{"token expirado", "Bearer jwt-expirado", http.StatusUnauthorized},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			handler := e004Handler(newE004Almacen(), newE004Sesiones(), e004TokensValidos())
			req := e004Peticion(http.MethodPost, "/api/v1/auth/logout", "", "")
			if caso.cabecera != "" {
				req.Header.Set("Authorization", caso.cabecera)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != caso.esperado {
				t.Fatalf("esperado %d, obtenido %d", caso.esperado, rec.Code)
			}
			if corpo := rec.Body.String(); !strings.Contains(corpo, "\"error\"") {
				t.Fatalf("debe devolver ErrorEnvelope, obtenido %q", corpo)
			}
		})
	}
}

// TestE004HTTPLogout204SinCuerpo cubre T02/T03: token válido, sesión ACTIVE y 204 sin contenido.
func TestE004HTTPLogout204SinCuerpo(t *testing.T) {
	sesiones := newE004Sesiones()
	handler := e004Handler(newE004Almacen(), sesiones, e004TokensValidos())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, e004Peticion(http.MethodPost, "/api/v1/auth/logout", "jwt-valido", ""))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("esperado 204, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("204 no debe llevar cuerpo, obtenido %q", rec.Body.String())
	}
	if cache := rec.Header().Get("Cache-Control"); !strings.Contains(cache, "no-store") {
		t.Fatalf("Cache-Control debe incluir no-store, obtenido %q", cache)
	}
	if sesiones.consultas != 1 {
		t.Fatalf("la sesion debe verificarse exactamente una vez, obtenidas %d", sesiones.consultas)
	}
}

// TestE004HTTPSegundoLogout401 cubre T05: repetir el cierre con el mismo JWT devuelve 401.
func TestE004HTTPSegundoLogout401(t *testing.T) {
	almacen := newE004Almacen()
	handler := e004Handler(almacen, newE004Sesiones(), e004TokensValidos())

	primera := httptest.NewRecorder()
	handler.ServeHTTP(primera, e004Peticion(http.MethodPost, "/api/v1/auth/logout", "jwt-valido", ""))
	if primera.Code != http.StatusNoContent {
		t.Fatalf("primer cierre debe ser 204, obtenido %d", primera.Code)
	}

	segunda := httptest.NewRecorder()
	handler.ServeHTTP(segunda, e004Peticion(http.MethodPost, "/api/v1/auth/logout", "jwt-valido", ""))
	if segunda.Code != http.StatusUnauthorized {
		t.Fatalf("segundo cierre debe ser 401, obtenido %d (%s)", segunda.Code, segunda.Body.String())
	}
}

// TestE004HTTPBodyNoPermitido cubre T08: el contrato no admite cuerpo, ni siquiera `{}`.
func TestE004HTTPBodyNoPermitido(t *testing.T) {
	for _, cuerpo := range []string{"{}", `{"allDevices":true}`, `{"refreshToken":"x"}`, "null"} {
		handler := e004Handler(newE004Almacen(), newE004Sesiones(), e004TokensValidos())
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, e004Peticion(http.MethodPost, "/api/v1/auth/logout", "jwt-valido", cuerpo))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("cuerpo %q debe rechazarse con 400, obtenido %d", cuerpo, rec.Code)
		}
		if corpo := rec.Body.String(); !strings.Contains(corpo, "body_not_allowed") {
			t.Fatalf("el detalle debe indicar body_not_allowed, obtenido %q", corpo)
		}
	}
}

// TestE004HTTPTokenSinSesion cubre T06: un JWT verificado sin `sid` no identifica sesión.
func TestE004HTTPTokenSinSesion(t *testing.T) {
	handler := e004Handler(newE004Almacen(), newE004Sesiones(), e004TokensValidos())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, e004Peticion(http.MethodPost, "/api/v1/auth/logout", "jwt-sin-sesion", ""))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("token sin sid debe ser 401, obtenido %d", rec.Code)
	}
}

// TestE004HTTPSesionDeOtroSujeto cubre la propiedad en el borde HTTP.
func TestE004HTTPSesionDeOtroSujeto(t *testing.T) {
	almacen := newE004Almacen()
	handler := e004Handler(almacen, newE004Sesiones(), e004TokensValidos())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, e004Peticion(http.MethodPost, "/api/v1/auth/logout", "jwt-sujeto-ajeno", ""))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("un sub ajeno no puede cerrar la sesion: %d", rec.Code)
	}
	// La sesión del titular sigue activa.
	sesion, _ := almacen.LoadLogoutSession(context.Background(), e004HTTPUsuario, e004HTTPSesion)
	if sesion == nil || sesion.Status != appIdentity.StatusActive {
		t.Fatalf("la sesion del titular no debe modificarse")
	}
}

// TestE004HTTPDependenciaIndisponible cubre T13/T14 en HTTP: 503, nunca 204.
func TestE004HTTPDependenciaIndisponible(t *testing.T) {
	sesiones := newE004Sesiones()
	sesiones.fallo = ports.ErrSessionUnavailable
	handler := e004Handler(newE004Almacen(), sesiones, e004TokensValidos())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, e004Peticion(http.MethodPost, "/api/v1/auth/logout", "jwt-valido", ""))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("sesion no verificable debe ser 503, obtenido %d", rec.Code)
	}

	// Fallo de escritura: tampoco 204.
	almacen := newE004Almacen()
	almacen.falloEscritura = errors.New("dynamodb inaccesible")
	handlerEscritura := e004Handler(almacen, newE004Sesiones(), e004TokensValidos())
	recEscritura := httptest.NewRecorder()
	handlerEscritura.ServeHTTP(recEscritura, e004Peticion(http.MethodPost, "/api/v1/auth/logout", "jwt-valido", ""))
	if recEscritura.Code != http.StatusServiceUnavailable {
		t.Fatalf("fallo de persistencia debe ser 503, obtenido %d", recEscritura.Code)
	}
}

// TestE004HTTPRevocacionVisibleEntreInstancias cubre T15 en el borde HTTP: dos handlers
// independientes que comparten estado autoritativo (una sesión, una tabla).
func TestE004HTTPRevocacionVisibleEntreInstancias(t *testing.T) {
	almacen := newE004Almacen()
	sesiones := newE004Sesiones()
	nodoA := e004Handler(almacen, sesiones, e004TokensValidos())
	nodoB := e004Handler(almacen, sesiones, e004TokensValidos())

	if rec := httptest.NewRecorder(); func() int {
		nodoB.ServeHTTP(rec, e004Peticion(http.MethodPost, "/api/v1/auth/logout", "jwt-valido", ""))
		return rec.Code
	}() != http.StatusNoContent {
		t.Fatalf("el nodo B debe poder cerrar una sesion activa")
	}

	// El nodo A ya no acepta el mismo token: la revocación es compartida.
	recA := httptest.NewRecorder()
	nodoA.ServeHTTP(recA, e004Peticion(http.MethodPost, "/api/v1/auth/logout", "jwt-valido", ""))
	if recA.Code != http.StatusUnauthorized {
		t.Fatalf("el nodo A debe ver la revocacion hecha por B, obtenido %d", recA.Code)
	}
}

// TestE004HTTPSinAutorizadorFallaCerrado cubre la corrección B: sin verificador de sesión el
// endpoint no autoriza "sólo con la firma"; responde 503.
func TestE004HTTPSinAutorizadorFallaCerrado(t *testing.T) {
	servicio := appIdentity.NewLogoutService(newE004Almacen()).
		WithClock(func() time.Time { return e004HTTPAhora })

	handler := internalhttp.NewLogoutHandler(servicio, e004TokensValidos(), nil, silencioso())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, e004Peticion(http.MethodPost, "/api/v1/auth/logout", "jwt-valido", ""))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("sin autorizador de sesion debe ser 503, obtenido %d", rec.Code)
	}

	handlerSinTokens := internalhttp.NewLogoutHandler(servicio, nil, newE004Sesiones(), silencioso())
	recSinTokens := httptest.NewRecorder()
	handlerSinTokens.ServeHTTP(recSinTokens, e004Peticion(http.MethodPost, "/api/v1/auth/logout", "jwt-valido", ""))
	if recSinTokens.Code != http.StatusServiceUnavailable {
		t.Fatalf("sin proveedor de tokens debe ser 503, obtenido %d", recSinTokens.Code)
	}
}

// TestE004HTTPErroresSinDatosSensibles cubre T20: ni tokens, ni ids de sesión, ni usuario.
func TestE004HTTPErroresSinDatosSensibles(t *testing.T) {
	almacen := newE004Almacen()
	sesiones := newE004Sesiones()
	handler := e004Handler(almacen, sesiones, e004TokensValidos())

	casos := []struct {
		nombre string
		token  string
		cuerpo string
	}{
		{"token invalido", "desconocido", ""},
		{"body rechazado", "jwt-valido", `{"refreshToken":"secreto-sintetico"}`},
		{"sesion ajena", "jwt-sujeto-ajeno", ""},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, e004Peticion(http.MethodPost, "/api/v1/auth/logout", caso.token, caso.cuerpo))
			corpo := rec.Body.String()
			for _, prohibido := range []string{
				"jwt-valido", "secreto-sintetico", "digest-c1", e004HTTPSesion, e004HTTPUsuario,
				"desconocido\"",
			} {
				if strings.Contains(corpo, prohibido) && prohibido != "desconocido\"" {
					t.Fatalf("la respuesta no debe contener %q: %q", prohibido, corpo)
				}
			}
		})
	}
}
