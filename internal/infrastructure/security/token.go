package security

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nexumpr/Nexum-Backend/internal/ports"
)

const (
	// DefaultIssuer identificador canónico del emisor del token (A-050 §2.2).
	DefaultIssuer = "https://api.nexum.finance"
	// DefaultAudience audiencia destino válida para clientes móviles y API.
	DefaultAudience = "nexum-client"
	// DefaultKeyID identificador de la clave pública para rotación de claves (kid).
	DefaultKeyID = "nexum-rsa-key-v1"
)

// JWTHeader representa la cabecera estándar de un JWT RS256.
type JWTHeader struct {
	Algorithm string `json:"alg"`
	Type      string `json:"typ"`
	KeyID     string `json:"kid"`
}

// JWTPayload representa las reclamaciones completas registradas y privadas.
type JWTPayload struct {
	Issuer    string   `json:"iss"`
	Subject   string   `json:"sub"`
	Audience  string   `json:"aud"`
	SessionID string   `json:"sid"`
	IssuedAt  int64    `json:"iat"`
	ExpiresAt int64    `json:"exp"`
	Roles     []string `json:"roles,omitempty"`
	JWTID     string   `json:"jti"`
}

// RSATokenProvider implementa el puerto ports.TokenProvider con firmas RSA-SHA256 (RS256).
type RSATokenProvider struct {
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	keyID      string
	issuer     string
	audience   string
}

// NewRSATokenProvider crea un proveedor de tokens con par de claves RSA provisto.
func NewRSATokenProvider(privateKey *rsa.PrivateKey, keyID string) (*RSATokenProvider, error) {
	if privateKey == nil {
		return nil, errors.New("la clave privada RSA es obligatoria")
	}
	if keyID == "" {
		keyID = DefaultKeyID
	}
	return &RSATokenProvider{
		privateKey: privateKey,
		publicKey:  &privateKey.PublicKey,
		keyID:      keyID,
		issuer:     DefaultIssuer,
		audience:   DefaultAudience,
	}, nil
}

// GenerateAccessToken emite un JWT firmado digitalmente con RS256.
func (p *RSATokenProvider) GenerateAccessToken(ctx context.Context, claims ports.TokenClaims) (string, error) {
	if claims.Subject == "" {
		return "", errors.New("el claim subject (sub) es obligatorio")
	}

	now := time.Now().UTC()
	iat := claims.IssuedAt
	if iat.IsZero() {
		iat = now
	}

	exp := claims.ExpiresAt
	if exp.IsZero() {
		exp = iat.Add(15 * time.Minute)
	}

	// Generar JTI aleatorio de 16 bytes
	jtiBytes := make([]byte, 16)
	if _, err := rand.Read(jtiBytes); err != nil {
		return "", fmt.Errorf("error al generar jti: %w", err)
	}
	jti := base64.RawURLEncoding.EncodeToString(jtiBytes)

	header := JWTHeader{
		Algorithm: "RS256",
		Type:      "JWT",
		KeyID:     p.keyID,
	}

	payload := JWTPayload{
		Issuer:    p.issuer,
		Subject:   claims.Subject,
		Audience:  p.audience,
		SessionID: claims.SessionID,
		IssuedAt:  iat.Unix(),
		ExpiresAt: exp.Unix(),
		Roles:     claims.Roles,
		JWTID:     jti,
	}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)

	signingInput := headerB64 + "." + payloadB64

	hasher := sha256.New()
	hasher.Write([]byte(signingInput))
	digest := hasher.Sum(nil)

	signature, err := rsa.SignPKCS1v15(rand.Reader, p.privateKey, crypto.SHA256, digest)
	if err != nil {
		return "", fmt.Errorf("error al firmar token con clave privada: %w", err)
	}

	sigB64 := base64.RawURLEncoding.EncodeToString(signature)
	return signingInput + "." + sigB64, nil
}

// ValidateAccessToken verifica la firma RS256, rechaza algoritmos no permitidos (ej: 'none')
// y comprueba la expiración temporal.
func (p *RSATokenProvider) ValidateAccessToken(ctx context.Context, tokenString string) (*ports.TokenClaims, error) {
	if tokenString == "" {
		return nil, ports.ErrInvalidToken
	}

	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, ports.ErrInvalidToken
	}

	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, ports.ErrInvalidToken
	}

	var header JWTHeader
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return nil, ports.ErrInvalidToken
	}

	// Rechazo estricto de cualquier algoritmo distinto a RS256 (A-050 §2.2)
	if header.Algorithm != "RS256" {
		return nil, fmt.Errorf("%w: algoritmo '%s' no permitido (solo RS256)", ports.ErrInvalidToken, header.Algorithm)
	}

	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ports.ErrInvalidToken
	}

	var payload JWTPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, ports.ErrInvalidToken
	}

	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, ports.ErrInvalidToken
	}

	signingInput := parts[0] + "." + parts[1]
	hasher := sha256.New()
	hasher.Write([]byte(signingInput))
	digest := hasher.Sum(nil)

	if err := rsa.VerifyPKCS1v15(p.publicKey, crypto.SHA256, digest, sigBytes); err != nil {
		return nil, ports.ErrInvalidToken
	}

	now := time.Now().UTC().Unix()
	if now >= payload.ExpiresAt {
		return nil, ports.ErrTokenExpired
	}

	return &ports.TokenClaims{
		Subject:   payload.Subject,
		SessionID: payload.SessionID,
		IssuedAt:  time.Unix(payload.IssuedAt, 0).UTC(),
		ExpiresAt: time.Unix(payload.ExpiresAt, 0).UTC(),
		Roles:     payload.Roles,
	}, nil
}

// GenerateRefreshToken genera un token opaco aleatorio de 256 bits (32 bytes) codificado en URL-safe Base64 (A-050 §2.3).
func (p *RSATokenProvider) GenerateRefreshToken(ctx context.Context) (string, error) {
	bytes := make([]byte, 32) // 256-bit
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("error al generar entropía para refresh token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

