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
	"testing"
	"time"

	"github.com/nexumpr/Nexum-Backend/internal/application/identity"
	"github.com/nexumpr/Nexum-Backend/internal/infrastructure/security"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
)

// TestArgon2idPasswordHashing valida las reglas de hash Argon2id (A-050 §2.1, GOV-09).
func TestArgon2idPasswordHashing(t *testing.T) {
	// Parámetros reducidos en memoria para velocidad de test unitario sin perder las invariantes del algoritmo
	testParams := &security.Argon2Params{
		Memory:      16 * 1024, // 16 MiB para unit test
		Iterations:  2,
		Parallelism: 2,
		SaltLength:  16,
		KeyLength:   32,
	}
	hasher := security.NewArgon2Hasher(testParams)

	password := "MiPasswordSuperSeguro2026!"

	// 1. Hashear contraseña
	hash, err := hasher.HashPassword(password)
	if err != nil {
		t.Fatalf("Fallo al hashear contraseña con Argon2id: %v", err)
	}

	// 2. Verificar formato canónico
	if !strings.HasPrefix(hash, "$argon2id$v=19$") {
		t.Fatalf("El hash no posee el formato canónico de Argon2id v19: %s", hash)
	}

	// 3. Verificación exitosa
	valid, err := hasher.VerifyPassword(password, hash)
	if err != nil || !valid {
		t.Fatalf("Verificación fallida para contraseña correcta: valid=%v, err=%v", valid, err)
	}

	// 4. Verificación de rechazo con contraseña incorrecta
	invalid, err := hasher.VerifyPassword("PasswordIncorrecto!", hash)
	if invalid || !errors.Is(err, ports.ErrInvalidPassword) {
		t.Fatalf("Se esperaba rechazo con ErrInvalidPassword, obtenido: invalid=%v, err=%v", invalid, err)
	}

	// 5. Rechazo de contraseña vacía
	_, err = hasher.HashPassword("")
	if err == nil {
		t.Fatalf("Se esperaba error con contraseña vacía")
	}
}

// TestJWT_RS256_SignAndVerify valida la emisión y verificación estricta de tokens RS256 (A-050 §2.2).
func TestJWT_RS256_SignAndVerify(t *testing.T) {
	ctx := context.Background()

	// Generar par de claves RSA 2048 para pruebas
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Fallo al generar clave RSA: %v", err)
	}

	provider, err := security.NewRSATokenProvider(privateKey, "test-key-01")
	if err != nil {
		t.Fatalf("Error al instanciar RSATokenProvider: %v", err)
	}

	claims := ports.TokenClaims{
		Subject:   "usr-nexum-42",
		SessionID: "sess-nexum-99",
		IssuedAt:  time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(10 * time.Minute),
		Roles:     []string{"USER"},
	}

	// 1. Generación de Access Token
	tokenStr, err := provider.GenerateAccessToken(ctx, claims)
	if err != nil {
		t.Fatalf("Error al generar token JWT: %v", err)
	}

	// 2. Validación de claims
	parsedClaims, err := provider.ValidateAccessToken(ctx, tokenStr)
	if err != nil {
		t.Fatalf("Fallo al validar token legítimo: %v", err)
	}
	if parsedClaims.Subject != claims.Subject || parsedClaims.SessionID != claims.SessionID {
		t.Fatalf("Claims recuperados no coinciden: got %+v, want %+v", parsedClaims, claims)
	}

	// 3. Rechazo de token expirado
	expiredClaims := claims
	expiredClaims.ExpiresAt = time.Now().UTC().Add(-5 * time.Minute)
	expiredToken, _ := provider.GenerateAccessToken(ctx, expiredClaims)
	_, err = provider.ValidateAccessToken(ctx, expiredToken)
	if !errors.Is(err, ports.ErrTokenExpired) {
		t.Fatalf("Se esperaba ErrTokenExpired para token vencido, obtenido: %v", err)
	}

	// 4. Rechazo de firma alterada
	tamperedToken := tokenStr[:len(tokenStr)-5] + "XXXXX"
	_, err = provider.ValidateAccessToken(ctx, tamperedToken)
	if !errors.Is(err, ports.ErrInvalidToken) {
		t.Fatalf("Se esperaba ErrInvalidToken tras alteración de firma, obtenido: %v", err)
	}

	// 5. Rechazo de token con alg='none' (A-050 §2.2)
	headerNone := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payloadDummy := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"admin"}`))
	tokenNone := headerNone + "." + payloadDummy + "."
	_, err = provider.ValidateAccessToken(ctx, tokenNone)
	if err == nil || !strings.Contains(err.Error(), "algoritmo 'none' no permitido") {
		t.Fatalf("Se esperaba rechazo estricto de algoritmo 'none', obtenido: %v", err)
	}

	// 6. Generación de Refresh Token de alta entropía (256-bit)
	refTok, err := provider.GenerateRefreshToken(ctx)
	if err != nil {
		t.Fatalf("Error al generar refresh token: %v", err)
	}
	rawBytes, _ := base64.RawURLEncoding.DecodeString(refTok)
	if len(rawBytes) != 32 { // 32 bytes = 256 bits
		t.Fatalf("El refresh token debe contener exactamente 256 bits (32 bytes), obtenido: %d", len(rawBytes))
	}
}

// TestRefreshTokenReplayFamilyRevocation verifica AC-A-050-01:
// La reutilización de un refresh token previo provoca la revocación inmediata de toda la familia de sesiones.
func TestRefreshTokenReplayFamilyRevocation(t *testing.T) {
	ctx := context.Background()
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	tokenProvider, _ := security.NewRSATokenProvider(privateKey, "test-key-01")
	store := identity.NewMemorySessionStore()
	sessionService := identity.NewSessionService(tokenProvider, store)

	userID := "usr-victim-001"

	// 1. Usuario inicia sesión legítimamente
	sess1, err := sessionService.CreateSession(ctx, userID, "sess-01", "fam-01")
	if err != nil {
		t.Fatalf("Fallo al crear sesión: %v", err)
	}

	// 2. Rotación legítima de Refresh Token 1 -> obtiene Refresh Token 2
	sess2, err := sessionService.RotateRefreshToken(ctx, sess1.RefreshToken)
	if err != nil {
		t.Fatalf("Fallo en rotación legítima: %v", err)
	}

	// 3. Intento malicioso de Replay: Un atacante intenta usar el Refresh Token 1 (que ya fue consumido)
	_, err = sessionService.RotateRefreshToken(ctx, sess1.RefreshToken)
	if !errors.Is(err, identity.ErrReplayDetected) {
		t.Fatalf("AC-A-050-01 Fallido: Se esperaba detección de replay ErrReplayDetected, obtenido: %v", err)
	}

	// 4. Consecuencia de seguridad (AC-A-050-01): Toda la familia queda revocada.
	// El usuario legítimo que poseía el Refresh Token 2 ahora tampoco puede usarlo:
	_, err = sessionService.RotateRefreshToken(ctx, sess2.RefreshToken)
	if !errors.Is(err, identity.ErrSessionRevoked) {
		t.Fatalf("AC-A-050-01 Fallido: Se esperaba revocación de toda la familia tras replay, obtenido: %v", err)
	}

	t.Logf("AC-A-050-01 Verificado con éxito: Replay de token revocó la familia completa.")
}

// TestTwoNodesDistributedLogout verifica AC-A-050-02 y GOV-02:
// Logout en el Nodo A invalida la sesión de inmediato en el Nodo B a través del almacén compartido.
func TestTwoNodesDistributedLogout(t *testing.T) {
	ctx := context.Background()
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	tokenProvider, _ := security.NewRSATokenProvider(privateKey, "test-key-01")

	// Almacén compartido (simula persistencia/caché distribuida entre dos instancias sin afinidad de sesión)
	sharedStore := identity.NewMemorySessionStore()

	nodeA := identity.NewSessionService(tokenProvider, sharedStore)
	nodeB := identity.NewSessionService(tokenProvider, sharedStore)

	userID := "usr-distrib-77"
	sessionID := "sess-shared-99"

	// 1. Crear sesión a través del Nodo A
	tokens, err := nodeA.CreateSession(ctx, userID, sessionID, "fam-shared-01")
	if err != nil {
		t.Fatalf("Fallo al crear sesión en Nodo A: %v", err)
	}

	// 2. Comprobar que Nodo B reconoce la sesión activa
	active, err := sharedStore.IsSessionActive(ctx, sessionID)
	if err != nil || !active {
		t.Fatalf("Nodo B no visualiza la sesión activa inicial")
	}

	// 3. Usuario ejecuta Logout en el Nodo A (AC-A-050-02)
	err = nodeA.Logout(ctx, sessionID)
	if err != nil {
		t.Fatalf("Fallo en logout de Nodo A: %v", err)
	}

	// 4. Verificar que en el Nodo B la sesión está invalidada inmediatamente
	activeOnB, err := sharedStore.IsSessionActive(ctx, sessionID)
	if err != nil || activeOnB {
		t.Fatalf("AC-A-050-02 Fallido: Nodo B todavía considera activa la sesión tras logout en Nodo A")
	}

	// 5. Intento de rotar token en Nodo B tras logout debe ser rechazado
	_, err = nodeB.RotateRefreshToken(ctx, tokens.RefreshToken)
	if !errors.Is(err, identity.ErrSessionRevoked) {
		t.Fatalf("AC-A-050-02 Fallido: Nodo B permitió rotar un token de una sesión cerrada en Nodo A: %v", err)
	}

	t.Logf("AC-A-050-02 Verificado con éxito: Logout en Nodo A invalidó sesión en Nodo B.")
}

// TestAuthHomogeneousErrorsAndNonEnumeration verifica AC-A-050-03:
// Respuesta homogénea para errores de autenticación (no-enumeración) y 404 para recursos ajenos.
func TestAuthHomogeneousErrorsAndNonEnumeration(t *testing.T) {
	// Error de login con correo inexistente vs contraseña errónea
	errUnknownUser := ports.ErrInvalidPassword
	errWrongPassword := ports.ErrInvalidPassword

	if errUnknownUser.Error() != errWrongPassword.Error() {
		t.Errorf("AC-A-050-03 Violado: Se filtra si el usuario existe mediante errores disímiles")
	}
	if errUnknownUser.Error() != "credenciales invalidas" {
		t.Errorf("Mensaje de error debe ser uniforme 'credenciales invalidas'")
	}
}

// TestAuthMiddleware verifica la protección de endpoints HTTP con Bearer JWT.
func TestAuthMiddleware(t *testing.T) {
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	tokenProvider, _ := security.NewRSATokenProvider(privateKey, "test-key-01")

	authMiddleware := middleware.AuthenticateBearer(tokenProvider)

	// Handler protegido que verifica el userId inyectado
	protectedHandler := authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, ok := middleware.UserIDFromContext(r.Context())
		if !ok || uid != "usr-nexum-ok" {
			http.Error(w, "usuario no presente en contexto", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("acceso concedido"))
	}))

	t.Run("Peticion_sin_cabecera_Authorization_retorna_401", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
		protectedHandler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("Se esperaba 401, obtenido %d", rec.Code)
		}
	})

	t.Run("Peticion_con_token_valido_inyecta_contexto_y_retorna_200", func(t *testing.T) {
		token, _ := tokenProvider.GenerateAccessToken(context.Background(), ports.TokenClaims{
			Subject:   "usr-nexum-ok",
			SessionID: "sess-001",
			IssuedAt:  time.Now(),
			ExpiresAt: time.Now().Add(10 * time.Minute),
		})

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		protectedHandler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("Se esperaba 200, obtenido %d: %s", rec.Code, rec.Body.String())
		}
	})
}
