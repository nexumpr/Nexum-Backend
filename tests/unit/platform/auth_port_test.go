package platform_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nexumpr/Nexum-Backend/internal/ports"
)

// mockPasswordHasher es un doble de prueba que cumple con el puerto PasswordHasher (A-010 §2.5).
type mockPasswordHasher struct{}

func (m *mockPasswordHasher) HashPassword(password string) (string, error) {
	if password == "" {
		return "", errors.New("la contrasena no puede estar vacia")
	}
	return "argon2id$v=19$m=65536,t=3,p=4$fake_salt$fake_hash", nil
}

func (m *mockPasswordHasher) VerifyPassword(password, hash string) (bool, error) {
	if password == "" || hash == "" {
		return false, ports.ErrInvalidPassword
	}
	if password == "CorrectPassword123!" {
		return true, nil
	}
	return false, ports.ErrInvalidPassword
}

// mockTokenProvider es un doble de prueba que cumple con el puerto TokenProvider (A-010 §2.5, GOV-06).
type mockTokenProvider struct{}

func (m *mockTokenProvider) GenerateAccessToken(ctx context.Context, claims ports.TokenClaims) (string, error) {
	if claims.Subject == "" {
		return "", errors.New("el claim subject (sub) es obligatorio")
	}
	return "mock.jwt.token", nil
}

func (m *mockTokenProvider) ValidateAccessToken(ctx context.Context, tokenString string) (*ports.TokenClaims, error) {
	if tokenString == "mock.jwt.token" {
		return &ports.TokenClaims{
			Subject:   "usr-100",
			SessionID: "sess-abc",
			IssuedAt:  time.Now().Add(-1 * time.Minute),
			ExpiresAt: time.Now().Add(15 * time.Minute),
			Roles:     []string{"USER"},
		}, nil
	}
	if tokenString == "expired.jwt.token" {
		return nil, ports.ErrTokenExpired
	}
	return nil, ports.ErrInvalidToken
}

func (m *mockTokenProvider) GenerateRefreshToken(ctx context.Context) (string, error) {
	return "mock-refresh-token-high-entropy", nil
}

func TestAuthPortContracts(t *testing.T) {
	ctx := context.Background()
	var hasher ports.PasswordHasher = &mockPasswordHasher{}
	var tokenProvider ports.TokenProvider = &mockTokenProvider{}

	t.Run("Contrato PasswordHasher", func(t *testing.T) {
		hash, err := hasher.HashPassword("CorrectPassword123!")
		if err != nil {
			t.Fatalf("error inesperado al hashear contrasena: %v", err)
		}
		if hash == "" {
			t.Fatal("el hash no debe estar vacio")
		}

		valid, err := hasher.VerifyPassword("CorrectPassword123!", hash)
		if err != nil || !valid {
			t.Fatalf("se esperaba verificacion exitosa, obtuvo valid=%t, err=%v", valid, err)
		}

		invalid, err := hasher.VerifyPassword("WrongPassword!", hash)
		if invalid || !errors.Is(err, ports.ErrInvalidPassword) {
			t.Fatalf("se esperaba rechazo con ErrInvalidPassword, obtuvo valid=%t, err=%v", invalid, err)
		}
	})

	t.Run("Contrato TokenProvider", func(t *testing.T) {
		claims := ports.TokenClaims{
			Subject:   "usr-100",
			SessionID: "sess-abc",
			IssuedAt:  time.Now(),
			ExpiresAt: time.Now().Add(15 * time.Minute),
		}

		token, err := tokenProvider.GenerateAccessToken(ctx, claims)
		if err != nil || token == "" {
			t.Fatalf("fallo al generar token: %v", err)
		}

		parsed, err := tokenProvider.ValidateAccessToken(ctx, token)
		if err != nil {
			t.Fatalf("fallo al validar token valido: %v", err)
		}
		if parsed.Subject != "usr-100" {
			t.Errorf("subject esperado 'usr-100', obtenido '%s'", parsed.Subject)
		}

		_, err = tokenProvider.ValidateAccessToken(ctx, "expired.jwt.token")
		if !errors.Is(err, ports.ErrTokenExpired) {
			t.Errorf("se esperaba ErrTokenExpired, obtenido: %v", err)
		}

		_, err = tokenProvider.ValidateAccessToken(ctx, "tampered.jwt.token")
		if !errors.Is(err, ports.ErrInvalidToken) {
			t.Errorf("se esperaba ErrInvalidToken, obtenido: %v", err)
		}
	})
}
