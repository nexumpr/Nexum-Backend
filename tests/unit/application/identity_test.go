package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nexumpr/Nexum-Backend/internal/adapters/memory"
	"github.com/nexumpr/Nexum-Backend/internal/application/identity"
	domain "github.com/nexumpr/Nexum-Backend/internal/domain/identity"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
)

// mockSimpleHasher implementa ports.PasswordHasher para los tests de aplicacion.
type mockSimpleHasher struct{}

func (h *mockSimpleHasher) HashPassword(password string) (string, error) {
	return "hashed:" + password, nil
}

func (h *mockSimpleHasher) VerifyPassword(password, hash string) (bool, error) {
	return hash == "hashed:"+password, nil
}

// mockSimpleTokenProvider implementa ports.TokenProvider.
type mockSimpleTokenProvider struct{}

func (p *mockSimpleTokenProvider) GenerateAccessToken(ctx context.Context, claims ports.TokenClaims) (string, error) {
	return "jwt-token-for-" + claims.Subject, nil
}

func (p *mockSimpleTokenProvider) ValidateAccessToken(ctx context.Context, tokenString string) (*ports.TokenClaims, error) {
	return &ports.TokenClaims{
		Subject:   "usr-test",
		SessionID: "sess-test",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}, nil
}

func (p *mockSimpleTokenProvider) GenerateRefreshToken(ctx context.Context) (string, error) {
	return "refresh-token-entropy", nil
}

// TestTwoInstancesStatelessIdentity verifica los criterios críticos de A-020:
// 1. AC-A-020-02: Dos instancias independientes atienden la misma identidad desde persistencia compartida.
// 2. A-020 §2.1 y §2.7: Backend es completamente stateless (sin afinidad de sesiones).
// 3. AC-A-020-03: Pruebas unitarias sin dependencia de la nube.
func TestTwoInstancesStatelessIdentity(t *testing.T) {
	ctx := context.Background()
	tableName := "NexumCore"

	// Almacenamiento compartido común simulando DynamoDB y Redis
	sharedRepo := memory.NewMemoryRepository()
	sharedCache := memory.NewMemoryCache()
	hasher := &mockSimpleHasher{}
	tokenProvider := &mockSimpleTokenProvider{}

	// Nodo A y Nodo B (simulan las 2 instancias independientes exigidas por GOV-02)
	nodeA := identity.NewIdentityService(sharedRepo, sharedCache, hasher, tokenProvider, tableName)
	nodeB := identity.NewIdentityService(sharedRepo, sharedCache, hasher, tokenProvider, tableName)

	// 1. Registro de usuario a través del Nodo A
	registerInput := identity.RegisterInput{
		ID:       "usr-cluster-01",
		Email:    "cliente@nexum.com",
		Password: "PasswordSeguro2026!",
	}

	createdUser, err := nodeA.RegisterUser(ctx, registerInput)
	if err != nil {
		t.Fatalf("Nodo A fallo al registrar usuario: %v", err)
	}
	if createdUser.Email != "cliente@nexum.com" {
		t.Errorf("email retornado incorrecto: %s", createdUser.Email)
	}

	// 2. Intento de registro duplicado a través del Nodo B -> DEBE FALLAR
	_, err = nodeB.RegisterUser(ctx, registerInput)
	if err == nil {
		t.Fatal("Nodo B debio rechazar el registro de un email ya existente")
	}
	if !errors.Is(err, identity.ErrUserAlreadyExists) {
		t.Errorf("se esperaba ErrUserAlreadyExists, se obtuvo: %v", err)
	}

	// 3. Autenticación exitosa a través del Nodo B (demuestra statelessness y verdad compartida)
	authResult, err := nodeB.Authenticate(ctx, "usr-cluster-01", "PasswordSeguro2026!")
	if err != nil {
		t.Fatalf("Nodo B debio autenticar al usuario registrado en Nodo A: %v", err)
	}
	if authResult.AccessToken != "jwt-token-for-usr-cluster-01" {
		t.Errorf("token inesperado emitido por Nodo B: %s", authResult.AccessToken)
	}

	// 4. Intento de autenticación con contraseña incorrecta en Nodo A
	_, err = nodeA.Authenticate(ctx, "usr-cluster-01", "PasswordErroneo")
	if !errors.Is(err, ports.ErrInvalidPassword) {
		t.Errorf("se esperaba ErrInvalidPassword, obtenido: %v", err)
	}

	// 5. Inactivación / Borrado de cuenta (GOV-07)
	// Obtenemos el item para marcarlo DELETED
	userItem, err := sharedRepo.GetItem(ctx, tableName, map[string]any{"PK": "USER#usr-cluster-01"})
	if err != nil {
		t.Fatalf("error al obtener usuario: %v", err)
	}
	userItem["Status"] = string(domain.StatusDeleted)
	_ = sharedRepo.PutItem(ctx, tableName, userItem, nil)

	// Nodo B intenta autenticar a la cuenta eliminada -> DEBE RECHAZARSE con ErrUserDeleted
	_, err = nodeB.Authenticate(ctx, "usr-cluster-01", "PasswordSeguro2026!")
	if !errors.Is(err, domain.ErrUserDeleted) {
		t.Errorf("Nodo B debio rechazar al usuario eliminado con ErrUserDeleted, obtenido: %v", err)
	}
}
