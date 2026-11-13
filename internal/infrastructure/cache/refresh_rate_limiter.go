package cache

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Cuotas de E003 (refresh). El token opaco es la credencial de la operación: la cuota por
// IP acota el abuso distribuido y la cuota por digest evita martillear un mismo token.
// Ambos contadores se evalúan en un único script atómico del servidor Redis.
const (
	refreshIPMax       int64 = 30
	refreshDigestMax   int64 = 12
	refreshWindow            = time.Minute
	claseRefreshIP           = "auth_refresh_ip"
	claseRefreshDigest       = "auth_refresh_token"
)

// ErrRefreshRateLimitKeyUnavailable indica que falta la clave HMAC: el endpoint falla
// cerrado (503) y nunca degrada a un identificador sin secreto.
var ErrRefreshRateLimitKeyUnavailable = errors.New("clave HMAC del limitador de refresh no disponible")

// scriptRefreshDual evalúa las dos cuotas de refresh en una sola operación atómica.
//
// Claves: KEYS[1] = cuota por IP, KEYS[2] = cuota por digest del token presentado.
// Argumentos: ARGV[1] = máximo por IP, ARGV[2] = máximo por token, ARGV[3] = ventana (s).
// Devuelve {permitido (1/0), retry_after_segundos}.
const scriptRefreshDual = `
local ipCount = redis.call('INCR', KEYS[1])
if ipCount == 1 then
  redis.call('EXPIRE', KEYS[1], ARGV[3])
end
local tokenCount = redis.call('INCR', KEYS[2])
if tokenCount == 1 then
  redis.call('EXPIRE', KEYS[2], ARGV[3])
end
if ipCount > tonumber(ARGV[1]) then
  local ttlIP = redis.call('TTL', KEYS[1])
  if ttlIP < 1 then ttlIP = tonumber(ARGV[3]) end
  return {0, ttlIP}
end
if tokenCount > tonumber(ARGV[2]) then
  local ttlToken = redis.call('TTL', KEYS[2])
  if ttlToken < 1 then ttlToken = tonumber(ARGV[3]) end
  return {0, ttlToken}
end
return {1, 0}
`

// RefreshRateLimiter aplica las dos cuotas de E003 con Redis real.
type RefreshRateLimiter struct {
	client *redis.Client
	script *redis.Script
	key    []byte
}

// NewRefreshRateLimiter construye el limitador. Con cliente nil o clave HMAC insuficiente,
// AllowRefresh falla cerrado (503): E003 nunca se ejecuta sin limitación.
func NewRefreshRateLimiter(client *redis.Client, hmacKey []byte) *RefreshRateLimiter {
	clave := make([]byte, len(hmacKey))
	copy(clave, hmacKey)
	return &RefreshRateLimiter{client: client, script: redis.NewScript(scriptRefreshDual), key: clave}
}

// refreshIdentifier deriva el identificador seudonimizado con HMAC-SHA256. Nunca se guarda
// en Redis la IP ni el digest del token en claro.
func (l *RefreshRateLimiter) refreshIdentifier(clase, valor string) string {
	mac := hmac.New(sha256.New, l.key)
	mac.Write([]byte(clase))
	mac.Write([]byte("|"))
	mac.Write([]byte(strings.TrimSpace(valor)))
	return hex.EncodeToString(mac.Sum(nil))
}

// AllowRefresh evalúa la cuota por IP y la cuota por digest en una ejecución atómica.
func (l *RefreshRateLimiter) AllowRefresh(ctx context.Context, ip, tokenDigest string) (bool, time.Duration, error) {
	if l == nil || l.client == nil {
		return false, 0, ErrAuthRateLimitFailClosed
	}
	if len(l.key) < loginHMACMinBytes {
		return false, 0, ErrRefreshRateLimitKeyUnavailable
	}

	ipLimpia := strings.TrimSpace(ip)
	if ipLimpia == "" {
		ipLimpia = "unknown"
	}
	claveIP := loginRateLimitKey(claseRefreshIP, l.refreshIdentifier(claseRefreshIP, ipLimpia))
	claveToken := loginRateLimitKey(claseRefreshDigest, l.refreshIdentifier(claseRefreshDigest, tokenDigest))
	ventanaSegundos := int64(refreshWindow / time.Second)

	res, err := l.script.Run(ctx, l.client, []string{claveIP, claveToken},
		refreshIPMax, refreshDigestMax, ventanaSegundos).Slice()
	if err != nil {
		return false, 0, fmt.Errorf("%w: %v", ErrAuthRateLimitFailClosed, err)
	}
	if len(res) != 2 {
		return false, 0, ErrAuthRateLimitFailClosed
	}

	permitido, ok := res[0].(int64)
	if !ok {
		return false, 0, ErrAuthRateLimitFailClosed
	}
	if permitido == 1 {
		return true, 0, nil
	}

	retry, _ := res[1].(int64)
	if retry < 1 {
		retry = ventanaSegundos
	}
	return false, time.Duration(retry) * time.Second, nil
}
