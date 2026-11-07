package cache

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	CachePrefix = "nexum:v1"
	DefaultEnv  = "local"
)

var (
	// hmacSecret clave de hashing para ofuscar PII y userIds en claves de caché (A-080 §2.2).
	hmacSecret = []byte("nexum-cache-key-hmac-sha256-secret-seed")
)

// HashIdentifier genera un HMAC-SHA256 hex seguro de un identificador de usuario para evitar PII en Redis (A-080 §2.2).
func HashIdentifier(identifier string) string {
	mac := hmac.New(sha256.New, hmacSecret)
	mac.Write([]byte(strings.TrimSpace(identifier)))
	return hex.EncodeToString(mac.Sum(nil))[:16] // primeros 16 caracteres hex (suficiente entropía)
}

// UserCacheKey construye una clave canónica con formato:
// nexum:v1:<env>:<user-hash>:<resource>:<params-hash>:<version> (A-080 §2.2)
func UserCacheKey(env, userID, resource, params string, dataVersion int64) string {
	if env == "" {
		env = DefaultEnv
	}
	userHash := HashIdentifier(userID)

	paramsHash := "default"
	if params != "" {
		h := sha256.Sum256([]byte(params))
		paramsHash = hex.EncodeToString(h[:])[:8]
	}

	return fmt.Sprintf("%s:%s:%s:%s:%s:v%d", CachePrefix, env, userHash, resource, paramsHash, dataVersion)
}

// UserCachePrefix genera el prefijo para invalidar selectivamente todos los recursos en caché de un usuario.
func UserCachePrefix(env, userID string) string {
	if env == "" {
		env = DefaultEnv
	}
	userHash := HashIdentifier(userID)
	return fmt.Sprintf("%s:%s:%s:", CachePrefix, env, userHash)
}

// PublicCatalogCacheKey genera claves de caché para catálogos públicos compartidos (ej: categorías, divisas).
func PublicCatalogCacheKey(env, catalogName string, version int64) string {
	if env == "" {
		env = DefaultEnv
	}
	return fmt.Sprintf("%s:%s:public:catalog:%s:v%d", CachePrefix, env, catalogName, version)
}

// RateLimitKey genera la clave atómica de Token Bucket en Redis por clase y hash de sujeto (A-080 §2.5).
func RateLimitKey(endpointClass, identifier string) string {
	idHash := HashIdentifier(identifier)
	return fmt.Sprintf("nexum:rl:%s:%s", endpointClass, idHash)
}
