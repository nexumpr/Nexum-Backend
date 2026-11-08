package security_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nexumpr/Nexum-Backend/internal/adapters/memory"
	financial "github.com/nexumpr/Nexum-Backend/internal/application/financial"
	identity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	"github.com/nexumpr/Nexum-Backend/internal/infrastructure/security"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	transport "github.com/nexumpr/Nexum-Backend/internal/transport/http"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
)

func proveedor(t *testing.T) *security.RSATokenProvider {
	t.Helper()
	clave, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("no se pudo generar la clave RSA: %v", err)
	}
	provider, err := security.NewRSATokenProvider(clave, "test-kid")
	if err != nil {
		t.Fatalf("no se pudo crear el proveedor: %v", err)
	}
	return provider
}

func TestJWTRechazaFirmaManipuladaYAlgNone(t *testing.T) {
	provider := proveedor(t)
	ctx := context.Background()
	valido, err := provider.GenerateAccessToken(ctx, ports.TokenClaims{
		Subject:   "usuario-a",
		SessionID: "sess-1",
		IssuedAt:  time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(5 * time.Minute),
	})
	if err != nil {
		t.Fatalf("no se pudo emitir el token: %v", err)
	}
	partes := strings.Split(valido, ".")
	if len(partes) != 3 {
		t.Fatalf("token inesperado: %q", valido)
	}

	// alg=none con la firma original: debe rechazarse.
	cabeceraNone := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	sinFirma := cabeceraNone + "." + partes[1] + "." + partes[2]
	if _, err := provider.ValidateAccessToken(ctx, sinFirma); err == nil {
		t.Errorf("un token con alg=none no debe validarse")
	}

	// Payload manipulado conservando la firma: debe rechazarse.
	payload := []byte(`{"sub":"usuario-atacante","sid":"sess-1","exp":9999999999}`)
	manipulado := partes[0] + "." + base64.RawURLEncoding.EncodeToString(payload) + "." + partes[2]
	if _, err := provider.ValidateAccessToken(ctx, manipulado); err == nil {
		t.Errorf("un payload manipulado no debe validarse")
	}
}

func TestJWTExpiradoSeRechaza(t *testing.T) {
	provider := proveedor(t)
	expirado, err := provider.GenerateAccessToken(context.Background(), ports.TokenClaims{
		Subject:   "usuario-a",
		SessionID: "sess-1",
		IssuedAt:  time.Now().UTC().Add(-10 * time.Minute),
		ExpiresAt: time.Now().UTC().Add(-1 * time.Minute),
	})
	if err != nil {
		t.Fatalf("no se pudo emitir el token expirado: %v", err)
	}
	if _, err := provider.ValidateAccessToken(context.Background(), expirado); !errors.Is(err, ports.ErrTokenExpired) {
		t.Errorf("se esperaba ErrTokenExpired, obtenido %v", err)
	}
}

func TestAuthMiddlewareRechazaSinTokenYConTokenInvalido(t *testing.T) {
	provider := proveedor(t)
	handler := middleware.AuthenticateBearer(provider)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, cabecera := range []string{"", "Bearer no-es-un-jwt", "Basic abc"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
		if cabecera != "" {
			req.Header.Set("Authorization", cabecera)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("cabecera %q: se esperaba 401, obtenido %d", cabecera, rec.Code)
		}
	}
}

func TestAuthMiddlewareInyectaUsuarioVerificado(t *testing.T) {
	provider := proveedor(t)
	token, err := provider.GenerateAccessToken(context.Background(), ports.TokenClaims{
		Subject:   "usuario-a",
		SessionID: "sess-1",
		IssuedAt:  time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(5 * time.Minute),
		Roles:     []string{"user"},
	})
	if err != nil {
		t.Fatalf("no se pudo emitir el token: %v", err)
	}
	var usuario, sesion string
	handler := middleware.AuthenticateBearer(provider)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		usuario, _ = middleware.UserIDFromContext(r.Context())
		sesion, _ = middleware.SessionIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || usuario != "usuario-a" || sesion != "sess-1" {
		t.Errorf("contexto inesperado: status=%d usuario=%q sesion=%q", rec.Code, usuario, sesion)
	}
}

func TestSesionLogoutYReplayRevocan(t *testing.T) {
	provider := proveedor(t)
	store := identity.NewMemorySessionStore()
	svc := identity.NewSessionService(provider, store)
	ctx := context.Background()

	sesion, err := svc.CreateSession(ctx, "usuario-a", "sess-1", "fam-1")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := svc.Logout(ctx, sesion.SessionID); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if activa, _ := store.IsSessionActive(ctx, sesion.SessionID); activa {
		t.Errorf("la sesion debe quedar inactiva tras logout")
	}
	if _, err := svc.RotateRefreshToken(ctx, sesion.RefreshToken); err == nil {
		t.Errorf("no se debe rotar un refresh token de una sesion cerrada")
	}

	sesion2, err := svc.CreateSession(ctx, "usuario-b", "sess-2", "fam-2")
	if err != nil {
		t.Fatalf("CreateSession 2: %v", err)
	}
	rotada, err := svc.RotateRefreshToken(ctx, sesion2.RefreshToken)
	if err != nil {
		t.Fatalf("primera rotacion: %v", err)
	}
	if _, err := svc.RotateRefreshToken(ctx, sesion2.RefreshToken); !errors.Is(err, identity.ErrReplayDetected) {
		t.Errorf("se esperaba replay detectado, obtenido %v", err)
	}
	// La familia queda revocada: el token rotado tampoco puede volver a usarse.
	if _, err := svc.RotateRefreshToken(ctx, rotada.RefreshToken); !errors.Is(err, identity.ErrSessionRevoked) && !errors.Is(err, identity.ErrSessionNotFound) {
		t.Errorf("el token de la familia revocada no debe rotar, obtenido %v", err)
	}
}

func TestIdempotenciaConcurrenteYConflictos(t *testing.T) {
	store := financial.NewMemoryIdempotencyStore()
	ctx := context.Background()
	const intentos = 20
	var wg sync.WaitGroup
	var mu sync.Mutex
	proceden, enCurso := 0, 0
	for i := 0; i < intentos; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.LockOrGet(ctx, "clave-1234567890", "usuario-a", "movimiento", "hash-a", time.Minute)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				proceden++
			case errors.Is(err, financial.ErrOperationInProgress):
				enCurso++
			}
		}()
	}
	wg.Wait()
	if proceden != 1 || enCurso != intentos-1 {
		t.Errorf("se esperaba 1 procede y %d en curso; obtenido %d/%d", intentos-1, proceden, enCurso)
	}
	if err := store.SaveResponse(ctx, "clave-1234567890", "usuario-a", "movimiento", 201, "{}"); err != nil {
		t.Fatalf("SaveResponse: %v", err)
	}
	rec, err := store.LockOrGet(ctx, "clave-1234567890", "usuario-a", "movimiento", "hash-a", time.Minute)
	if err != nil || rec == nil || !rec.IsCompleted {
		t.Errorf("se esperaba registro completado; rec=%v err=%v", rec, err)
	}
	if _, err := store.LockOrGet(ctx, "clave-1234567890", "usuario-a", "movimiento", "hash-b", time.Minute); !errors.Is(err, financial.ErrIdempotencyConflict) {
		t.Errorf("misma clave con otro payload debe dar 409, obtenido %v", err)
	}
	// Aislamiento por usuario: la misma clave de otro usuario no colisiona.
	if _, err := store.LockOrGet(ctx, "clave-1234567890", "usuario-b", "movimiento", "hash-a", time.Minute); err != nil {
		t.Errorf("la clave debe aislarse por usuario, obtenido %v", err)
	}
}

func TestCORSRechazaOrigenNoAutorizado(t *testing.T) {
	opts := middleware.DefaultCORSOptions()
	handler := middleware.CORS(opts)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Origin", "https://atacante.example")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("un origen no autorizado no debe recibir ACAO")
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Origin", "https://app.nexum.finance")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Header().Get("Access-Control-Allow-Origin") != "https://app.nexum.finance" {
		t.Errorf("el origen autorizado debe reflejarse")
	}
	if rec.Header().Get("Access-Control-Allow-Origin") == "*" {
		t.Errorf("no se debe usar comodin")
	}

	req = httptest.NewRequest(http.MethodOptions, "/api/v1/me", nil)
	req.Header.Set("Origin", "https://app.nexum.finance")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("preflight debe responder 204, obtenido %d", rec.Code)
	}
}

func TestCabecerasDeSeguridad(t *testing.T) {
	handler := middleware.SecurityHeaders(true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/me", nil))
	for cabecera, valor := range map[string]string{
		"X-Content-Type-Options":    "nosniff",
		"X-Frame-Options":           "DENY",
		"Strict-Transport-Security": "max-age=31536000; includeSubDomains",
	} {
		if rec.Header().Get(cabecera) != valor {
			t.Errorf("%s: esperado %q, obtenido %q", cabecera, valor, rec.Header().Get(cabecera))
		}
	}
}

func TestBodyGrandeSeRechaza(t *testing.T) {
	cuerpo := `{"campo":"` + strings.Repeat("a", 2*1024*1024) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/movimientos", strings.NewReader(cuerpo))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	var destino struct {
		Campo string `json:"campo"`
	}
	err := transport.BindJSON(rec, req, &destino)
	if err == nil {
		t.Errorf("un cuerpo > 1MB debe rechazarse")
	}
}

func TestRateLimiterDevuelve429ConRetryAfter(t *testing.T) {
	cacheCompartida := memory.NewMemoryCache()
	rl := middleware.NewRateLimiter(cacheCompartida, 2, time.Minute)
	handler := rl.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	codigos := make([]int, 0, 3)
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
		req.RemoteAddr = "10.0.0.1:1234"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		codigos = append(codigos, rec.Code)
		if rec.Code == http.StatusTooManyRequests && rec.Header().Get("Retry-After") == "" {
			t.Errorf("429 debe incluir Retry-After")
		}
	}
	if codigos[2] != http.StatusTooManyRequests {
		t.Errorf("la tercera peticion debe dar 429, obtenido %v", codigos)
	}
}
