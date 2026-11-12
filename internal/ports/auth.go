package ports

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrInvalidPassword indica que la contraseña proporcionada no coincide con el hash almacenado.
	ErrInvalidPassword = errors.New("credenciales invalidas")

	// ErrInvalidToken indica que la firma, formato o estructura del token no es válida.
	ErrInvalidToken = errors.New("token invalido o con firma no verificada")

	// ErrTokenExpired indica que el token superó su tiempo de expiración (exp).
	ErrTokenExpired = errors.New("el token ha expirado")
)

// TokenClaims representa la estructura de reclamaciones de seguridad contenidas en un JWT RS256.
type TokenClaims struct {
	Subject   string    `json:"sub"`
	SessionID string    `json:"sid"`
	IssuedAt  time.Time `json:"iat"`
	ExpiresAt time.Time `json:"exp"`
	Roles     []string  `json:"roles,omitempty"`
	// JTI permite que el emisor fije el identificador único del token y lo persista con la
	// sesión (E002). Si llega vacío, el proveedor genera uno aleatorio.
	JTI string `json:"jti,omitempty"`
}

// PasswordHasher abstrae el algoritmo de derivación de claves para contraseñas (A-010 §2.5: Argon2id).
type PasswordHasher interface {
	// HashPassword genera un hash seguro con sal determinista y parámetros Argon2id aprobados.
	HashPassword(password string) (string, error)

	// VerifyPassword compara en tiempo constante una contraseña en texto plano contra un hash.
	VerifyPassword(password, hash string) (bool, error)
}

// TokenProvider abstrae la emisión y validación de tokens de sesión JWT firmados con RS256 (A-010 §2.5, GOV-06).
type TokenProvider interface {
	// GenerateAccessToken emite un token JWT firmado asimétricamente para acceso a la API.
	GenerateAccessToken(ctx context.Context, claims TokenClaims) (string, error)

	// ValidateAccessToken verifica la firma criptográfica y expiración del token retornando sus reclamaciones.
	ValidateAccessToken(ctx context.Context, tokenString string) (*TokenClaims, error)

	// GenerateRefreshToken genera un token criptográfico de alta entropía para rotación de sesión.
	GenerateRefreshToken(ctx context.Context) (string, error)
}
