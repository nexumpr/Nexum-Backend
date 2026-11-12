package transport_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	"github.com/nexumpr/Nexum-Backend/internal/infrastructure/cache"
	"github.com/nexumpr/Nexum-Backend/internal/infrastructure/security"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// ---------- Política de IP confiable (RFC-013 §4.5) ----------

func TestClientIPPolicy(t *testing.T) {
	casos := []struct {
		nombre    string
		forwarded string
		remote    string
		esperado  string
	}{
		{"un solo salto (CloudFront sustituye)", "203.0.113.7", "10.42.1.10:443", "203.0.113.7"},
		{"cadena con prefijo falsificado", "1.2.3.4, 203.0.113.7, 130.176.10.20", "10.42.1.10:443", "203.0.113.7"},
		{"dos saltos sin spoof", "203.0.113.7, 130.176.10.20", "10.42.1.10:443", "203.0.113.7"},
		{"ipv6 del visor", "2001:db8::1, 130.176.10.20", "10.42.1.10:443", "2001:db8::1"},
		{"ipv4 mapeada en ipv6", "::ffff:203.0.113.7, 130.176.10.20", "10.42.1.10:443", "203.0.113.7"},
		{"cadena vacia usa el peer", "", "198.51.100.9:5555", "198.51.100.9"},
		{"cadena malformada rechazada", "no-es-ip, 130.176.10.20", "198.51.100.9:5555", ""},
		{"cadena demasiado larga", strings.Repeat("203.0.113.7,", 40), "198.51.100.9:5555", ""},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			cuerpo := `{"email":"usuario.prueba@example.com","password":"ExamplePassphrase#2026"}`
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(cuerpo))
			req.Header.Set("Content-Type", "application/json")
			req.RemoteAddr = caso.remote
			if caso.forwarded != "" {
				req.Header.Set("X-Forwarded-For", caso.forwarded)
			}
			rec := httptest.NewRecorder()
			// La función es interna al paquete de transporte; se ejercita a través del
			// handler mediante un limitador que captura la IP recibida.
			limitador := &capturaLimiter{allow: true}
			handler := internalhttp.NewLoginHandler(nuevoServicioLogin(t), limitador, silencioso())
			handler.ServeHTTP(rec, req)
			if caso.esperado == "" {
				if rec.Code != http.StatusBadRequest {
					t.Fatalf("cadena no confiable debe rechazarse con 400, obtenido %d", rec.Code)
				}
				return
			}
			if limitador.ip != caso.esperado {
				t.Fatalf("IP esperada %q, obtenida %q", caso.esperado, limitador.ip)
			}
		})
	}
}

// ---------- Keyring RSA (RFC-013 §4.2) ----------

func TestKeyringRSA(t *testing.T) {
	ctx := context.Background()
	activa, _ := rsa.GenerateKey(rand.Reader, 2048)
	anterior, _ := rsa.GenerateKey(rand.Reader, 2048)
	ajena, _ := rsa.GenerateKey(rand.Reader, 2048)

	proveedor, err := security.NewRSATokenProvider(activa, "kid-activo")
	if err != nil {
		t.Fatalf("proveedor: %v", err)
	}
	if err := proveedor.AddVerificationKey("kid-anterior", &anterior.PublicKey); err != nil {
		t.Fatalf("keyring anterior: %v", err)
	}

	ahora := time.Now().UTC()
	token, err := proveedor.GenerateAccessToken(ctx, tokenClaims("user-1", "sess-1", ahora))
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}
	if _, err := proveedor.ValidateAccessToken(ctx, token); err != nil {
		t.Fatalf("validar con clave activa: %v", err)
	}

	// Token emitido antes de la rotación: kid anterior reconocido por el keyring.
	previo, _ := security.NewRSATokenProvider(anterior, "kid-anterior")
	tokenPrevio, _ := previo.GenerateAccessToken(ctx, tokenClaims("user-1", "sess-1", ahora))
	if _, err := proveedor.ValidateAccessToken(ctx, tokenPrevio); err != nil {
		t.Fatalf("validar con clave anterior: %v", err)
	}

	// kid desconocido.
	desconocido, _ := security.NewRSATokenProvider(ajena, "kid-ajeno")
	tokenAjeno, _ := desconocido.GenerateAccessToken(ctx, tokenClaims("user-1", "sess-1", ahora))
	if _, err := proveedor.ValidateAccessToken(ctx, tokenAjeno); err == nil {
		t.Fatal("un kid desconocido debe rechazarse")
	}

	// Firma manipulada con el mismo kid activo.
	partes := strings.Split(token, ".")
	if len(partes) != 3 {
		t.Fatalf("token inesperado: %q", token)
	}
	alterado := partes[0] + "." + partes[1] + "." + partes[2][:len(partes[2])-2] + "AA"
	if _, err := proveedor.ValidateAccessToken(ctx, alterado); err == nil {
		t.Fatal("una firma alterada debe rechazarse")
	}

	// kid vacío y clave inválida en el keyring.
	if err := proveedor.AddVerificationKey("", &anterior.PublicKey); err == nil {
		t.Fatal("kid vacio debe rechazarse")
	}
	if err := proveedor.AddVerificationKey("kid-x", nil); err == nil {
		t.Fatal("clave nula debe rechazarse")
	}
	if err := proveedor.AddVerificationKey("kid-anterior", &ajena.PublicKey); err == nil {
		t.Fatal("kid ya registrado con otra clave debe rechazarse")
	}
}

// ---------- Clave HMAC del limitador (RFC-013 §4.4) ----------

func TestLoginHMACKeyLoading(t *testing.T) {
	casos := []struct {
		nombre   string
		valor    string
		esperaOk bool
	}{
		{"clave de 32 bytes", base64.StdEncoding.EncodeToString(make([]byte, 32)), true},
		{"clave de 16 bytes (insuficiente)", base64.StdEncoding.EncodeToString(make([]byte, 16)), false},
		{"base64 invalido", "no-es-base64!!", false},
		{"ausente", "", false},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			t.Setenv(cache.EnvLoginHMACKeyB64, caso.valor)
			clave, err := cache.LoadLoginHMACKey()
			if caso.esperaOk {
				if err != nil || len(clave) < 32 {
					t.Fatalf("se esperaba clave valida, err=%v len=%d", err, len(clave))
				}
				return
			}
			if err == nil {
				t.Fatal("se esperaba error de configuracion")
			}
		})
	}
}

func TestLoginRateLimiterFallaCerradoSinClave(t *testing.T) {
	limitador := cache.NewLoginRateLimiter(nil, nil)
	if _, _, err := limitador.AllowLogin(context.Background(), "203.0.113.7", "usuario@example.com"); err == nil {
		t.Fatal("sin Redis o sin clave HMAC el limitador debe fallar cerrado")
	}
}

// ---------- Handler HTTP de E002 ----------

func TestLoginHandlerContrato(t *testing.T) {
	casos := []struct {
		nombre   string
		cuerpo   string
		limitado bool
		estado   int
	}{
		{"minimo valido", `{"email":"usuario.prueba@example.com","password":"ExamplePassphrase#2026"}`, true, http.StatusOK},
		{"completo valido", `{"email":"usuario.prueba@example.com","password":"ExamplePassphrase#2026","deviceId":"installation-1","deviceName":"Android","platform":"android"}`, true, http.StatusOK},
		{"email invalido", `{"email":"sin-arroba","password":"ExamplePassphrase#2026"}`, true, http.StatusBadRequest},
		{"password vacia", `{"email":"usuario.prueba@example.com","password":""}`, true, http.StatusBadRequest},
		{"plataforma no admitida", `{"email":"usuario.prueba@example.com","password":"ExamplePassphrase#2026","platform":"ios"}`, true, http.StatusBadRequest},
		{"propiedad desconocida", `{"email":"usuario.prueba@example.com","password":"ExamplePassphrase#2026","userId":"x"}`, true, http.StatusBadRequest},
		{"json malformado", `{"email":`, true, http.StatusBadRequest},
		{"credenciales invalidas", `{"email":"usuario.prueba@example.com","password":"OtraFrase#2026"}`, true, http.StatusUnauthorized},
		{"cuota excedida", `{"email":"usuario.prueba@example.com","password":"ExamplePassphrase#2026"}`, false, http.StatusTooManyRequests},
	}

	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			limitador := &capturaLimiter{allow: caso.limitado, retryAfter: 30 * time.Second}
			handler := internalhttp.NewLoginHandler(nuevoServicioLogin(t), limitador, silencioso())
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(caso.cuerpo))
			req.Header.Set("Content-Type", "application/json")
			req.RemoteAddr = "10.42.1.10:443"
			req.Header.Set("X-Forwarded-For", "203.0.113.7, 130.176.10.20")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != caso.estado {
				t.Fatalf("estado esperado %d, obtenido %d: %s", caso.estado, rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "no-store") {
				t.Fatalf("Cache-Control esperado con no-store, obtenido %q", got)
			}
			if caso.estado == http.StatusTooManyRequests {
				if rec.Header().Get("Retry-After") == "" {
					t.Fatal("429 debe incluir Retry-After")
				}
			}
			if caso.estado == http.StatusOK {
				var respuesta map[string]any
				if err := json.Unmarshal(rec.Body.Bytes(), &respuesta); err != nil {
					t.Fatalf("respuesta no es JSON: %v", err)
				}
				if respuesta["tokenType"] != "Bearer" {
					t.Fatalf("tokenType inesperado: %v", respuesta["tokenType"])
				}
				if respuesta["expiresIn"] != float64(900) {
					t.Fatalf("expiresIn inesperado: %v", respuesta["expiresIn"])
				}
				sesion, _ := respuesta["session"].(map[string]any)
				if sesion == nil || sesion["id"] != respuesta["sessionId"] {
					t.Fatalf("session.id debe igualar sessionId: %v", respuesta)
				}
				usuario, _ := respuesta["user"].(map[string]any)
				if usuario == nil || usuario["id"] != "47622cbc-36ec-4b30-9b8a-d9c26833a395" {
					t.Fatalf("user real esperado: %v", respuesta["user"])
				}
				onboarding, _ := respuesta["onboarding"].(map[string]any)
				if onboarding == nil || onboarding["configured"] != false {
					t.Fatalf("onboarding real esperado: %v", respuesta["onboarding"])
				}
			}
			if caso.estado >= 400 && strings.Contains(rec.Body.String(), "ExamplePassphrase") {
				t.Fatal("el error no debe reflejar la contrasena")
			}
		})
	}
}

func TestLoginHandlerBodyDemasiadoGrande(t *testing.T) {
	limitador := &capturaLimiter{allow: true}
	handler := internalhttp.NewLoginHandler(nuevoServicioLogin(t), limitador, silencioso())
	cuerpo := `{"email":"usuario.prueba@example.com","password":"` + strings.Repeat("a", 300*1024) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(cuerpo))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("cuerpo > 256 KiB debe rechazarse, obtenido %d", rec.Code)
	}
}

// ---------- dobles ----------

type capturaLimiter struct {
	allow      bool
	retryAfter time.Duration
	err        error
	ip         string
	email      string
}

func (c *capturaLimiter) AllowLogin(_ context.Context, ip, email string) (bool, time.Duration, error) {
	c.ip = ip
	c.email = email
	if c.err != nil {
		return false, 0, c.err
	}
	return c.allow, c.retryAfter, nil
}

type loginStoreDoble struct {
	identidad  *appIdentity.StoredIdentity
	onboarding bool
}

func (d *loginStoreDoble) LookupIdentity(context.Context, string) (*appIdentity.StoredIdentity, error) {
	return d.identidad, nil
}

func (d *loginStoreDoble) OnboardingConfigured(context.Context, string) (bool, error) {
	return d.onboarding, nil
}

func (d *loginStoreDoble) CreateSession(context.Context, appIdentity.SessionRecord) error { return nil }

type emisorDoble struct{}

func (emisorDoble) IssueAccessToken(context.Context, string, string, time.Time) (string, string, error) {
	return "jwt-de-prueba", "jti-de-prueba", nil
}

type hasherDoble struct{}

func (hasherDoble) HashPassword(password string) (string, error) { return "hashed:" + password, nil }
func (hasherDoble) VerifyPassword(password, hash string) (bool, error) {
	return hash == "hashed:"+password, nil
}

func nuevoServicioLogin(t *testing.T) *appIdentity.LoginService {
	t.Helper()
	store := &loginStoreDoble{
		identidad: &appIdentity.StoredIdentity{
			UserID:        "47622cbc-36ec-4b30-9b8a-d9c26833a395",
			Email:         "usuario.prueba@example.com",
			DisplayName:   "Usuario de Prueba Nexum",
			PasswordHash:  "hashed:ExamplePassphrase#2026",
			EmailVerified: false,
			Status:        "ACTIVE",
		},
		onboarding: false,
	}
	return appIdentity.NewLoginService(store, hasherDoble{}, emisorDoble{})
}

func silencioso() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

func tokenClaims(userID, sessionID string, ahora time.Time) ports.TokenClaims {
	return ports.TokenClaims{
		Subject:   userID,
		SessionID: sessionID,
		IssuedAt:  ahora,
		ExpiresAt: ahora.Add(900 * time.Second),
	}
}
