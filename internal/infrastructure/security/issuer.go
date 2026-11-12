package security

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
)

// Variables de entorno con el material criptográfico (bundle RFC-004 → backend.env).
const (
	// EnvJWTPrivatePEMB64 contiene el PEM de la clave privada RSA en Base64.
	EnvJWTPrivatePEMB64 = "JWT_PRIVATE_KEY_PEM_B64"
	// EnvJWTKeyID identifica la clave vigente (claim kid).
	EnvJWTKeyID = "JWT_KEY_ID"
)

// ErrSigningKeyUnavailable indica que falta material criptográfico válido: el login debe
// fallar cerrado con 503 y no emitir tokens ficticios.
var ErrSigningKeyUnavailable = errors.New("clave de firma JWT no disponible")

// LoadTokenProvider construye el proveedor RS256 desde el entorno. La clave se decodifica
// sólo en memoria: nunca se escribe en disco ni se registra en logs.
func LoadTokenProvider() (*RSATokenProvider, error) {
	encoded := strings.TrimSpace(os.Getenv(EnvJWTPrivatePEMB64))
	if encoded == "" {
		return nil, fmt.Errorf("%w: falta %s", ErrSigningKeyUnavailable, EnvJWTPrivatePEMB64)
	}
	pemBytes, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("%w: base64 invalido", ErrSigningKeyUnavailable)
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil || !strings.Contains(block.Type, "PRIVATE KEY") {
		return nil, fmt.Errorf("%w: PEM invalido", ErrSigningKeyUnavailable)
	}

	var clave *rsa.PrivateKey
	if parsed, errPKCS8 := x509.ParsePKCS8PrivateKey(block.Bytes); errPKCS8 == nil {
		typed, ok := parsed.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("%w: la clave no es RSA", ErrSigningKeyUnavailable)
		}
		clave = typed
	} else {
		parsed, errPKCS1 := x509.ParsePKCS1PrivateKey(block.Bytes)
		if errPKCS1 != nil {
			return nil, fmt.Errorf("%w: PEM no parseable", ErrSigningKeyUnavailable)
		}
		clave = parsed
	}

	if err := clave.Validate(); err != nil {
		return nil, fmt.Errorf("%w: clave invalida", ErrSigningKeyUnavailable)
	}
	if clave.N.BitLen() < 2048 {
		return nil, fmt.Errorf("%w: RSA < 2048 bits", ErrSigningKeyUnavailable)
	}

	keyID := strings.TrimSpace(os.Getenv(EnvJWTKeyID))
	return NewRSATokenProvider(clave, keyID)
}

// RSAAccessTokenIssuer implementa appIdentity.AccessTokenIssuer con RS256 y 900 s de vida.
type RSAAccessTokenIssuer struct {
	provider *RSATokenProvider
}

// NewRSAAccessTokenIssuer envuelve un proveedor ya construido.
func NewRSAAccessTokenIssuer(provider *RSATokenProvider) *RSAAccessTokenIssuer {
	return &RSAAccessTokenIssuer{provider: provider}
}

// IssueAccessToken emite el JWT y devuelve también el jti persistido en la sesión.
func (i *RSAAccessTokenIssuer) IssueAccessToken(ctx context.Context, userID, sessionID string, now time.Time) (string, string, error) {
	if i == nil || i.provider == nil {
		return "", "", ErrSigningKeyUnavailable
	}
	jtiBytes := make([]byte, 16)
	if _, err := rand.Read(jtiBytes); err != nil {
		return "", "", fmt.Errorf("jti: %w", err)
	}
	jti := base64.RawURLEncoding.EncodeToString(jtiBytes)

	issued := now.UTC()
	token, err := i.provider.GenerateAccessToken(ctx, ports.TokenClaims{
		Subject:   userID,
		SessionID: sessionID,
		IssuedAt:  issued,
		ExpiresAt: issued.Add(appIdentity.AccessTokenTTL),
		JTI:       jti,
	})
	if err != nil {
		return "", "", err
	}
	return token, jti, nil
}
