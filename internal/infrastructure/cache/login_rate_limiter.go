package cache

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Cuotas de prelogin definidas en POLITICAS_TRANSVERSALES_H06.md: 5/min/IP y 10/min/email.
const (
	loginIPMax      int64 = 5
	loginEmailMax   int64 = 10
	loginWindow           = time.Minute
	claseLoginIP          = "auth_login_ip"
	claseLoginEmail       = "auth_login_email"

	// EnvLoginHMACKeyB64 nombre de la variable de entorno con la clave HMAC del login.
	// Se provisiona por el bundle privado RFC-004 junto al resto de configuración.
	EnvLoginHMACKeyB64 = "LOGIN_RATE_LIMIT_HMAC_KEY_B64"

	// loginHMACMinBytes es la entropía mínima aceptada para la clave HMAC (256 bits).
	loginHMACMinBytes = 32
)

// ErrLoginRateLimitKeyUnavailable indica que falta la clave HMAC: el login debe fallar
// cerrado (503) y nunca degradar a un hash sin secreto.
var ErrLoginRateLimitKeyUnavailable = errors.New("clave HMAC del limitador de login no disponible")

// scriptLoginDual evalua AMBAS cuotas en una sola operación atómica del servidor Redis:
// incrementa los dos contadores, fija su TTL y decide en el mismo instante.
//
// Claves: KEYS[1] = cuota por IP, KEYS[2] = cuota por email.
// Argumentos: ARGV[1] = máximo por IP, ARGV[2] = máximo por email, ARGV[3] = ventana (s).
// Devuelve {permitido (1/0), retry_after_segundos, cuota_que_bloquea (0/1/2)}.
//
// La instancia desplegada (I-050) es Redis OSS de un nodo, no Cluster: las dos claves
// pueden participar en el mismo script.
const scriptLoginDual = `
local ipCount = redis.call('INCR', KEYS[1])
if ipCount == 1 then
  redis.call('EXPIRE', KEYS[1], ARGV[3])
end
local emailCount = redis.call('INCR', KEYS[2])
if emailCount == 1 then
  redis.call('EXPIRE', KEYS[2], ARGV[3])
end
if ipCount > tonumber(ARGV[1]) then
  local ttlIP = redis.call('TTL', KEYS[1])
  if ttlIP < 1 then ttlIP = tonumber(ARGV[3]) end
  return {0, ttlIP, 1}
end
if emailCount > tonumber(ARGV[2]) then
  local ttlEmail = redis.call('TTL', KEYS[2])
  if ttlEmail < 1 then ttlEmail = tonumber(ARGV[3]) end
  return {0, ttlEmail, 2}
end
return {1, 0, 0}
`

// LoadLoginHMACKey lee la clave HMAC del entorno (Base64) y exige al menos 256 bits.
// No hay valor por defecto ni derivación desde otra clave: sin material válido el
// limitador falla cerrado.
func LoadLoginHMACKey() ([]byte, error) {
	codificada := strings.TrimSpace(os.Getenv(EnvLoginHMACKeyB64))
	if codificada == "" {
		return nil, fmt.Errorf("%w: falta %s", ErrLoginRateLimitKeyUnavailable, EnvLoginHMACKeyB64)
	}
	clave, err := base64.StdEncoding.DecodeString(codificada)
	if err != nil {
		return nil, fmt.Errorf("%w: base64 invalido", ErrLoginRateLimitKeyUnavailable)
	}
	if len(clave) < loginHMACMinBytes {
		return nil, fmt.Errorf("%w: se requieren al menos %d bytes", ErrLoginRateLimitKeyUnavailable, loginHMACMinBytes)
	}
	return clave, nil
}

// LoginRateLimiter implementa la interfaz del handler con una decisión conjunta y atómica.
// Los identificadores se derivan con HMAC-SHA256 y una clave secreta: sin la clave no se
// puede reconstruir el identificador a partir del email o de la IP.
type LoginRateLimiter struct {
	client *redis.Client
	script *redis.Script
	key    []byte
}

// NewLoginRateLimiter construye el limitador dual. Con cliente nil o clave HMAC ausente
// AllowLogin falla cerrado (503): el login nunca se ejecuta sin limitación.
func NewLoginRateLimiter(client *redis.Client, hmacKey []byte) *LoginRateLimiter {
	clave := make([]byte, len(hmacKey))
	copy(clave, hmacKey)
	return &LoginRateLimiter{client: client, script: redis.NewScript(scriptLoginDual), key: clave}
}

// loginIdentifier deriva el identificador seudonimizado con HMAC-SHA256 (RFC-013).
func (l *LoginRateLimiter) loginIdentifier(clase, valor string) string {
	mac := hmac.New(sha256.New, l.key)
	mac.Write([]byte(clase))
	mac.Write([]byte("|"))
	mac.Write([]byte(strings.TrimSpace(valor)))
	return hex.EncodeToString(mac.Sum(nil))
}

// AllowLogin evalúa las dos cuotas en una única ejecución atómica del script.
func (l *LoginRateLimiter) AllowLogin(ctx context.Context, ip, email string) (bool, time.Duration, error) {
	if l == nil || l.client == nil {
		return false, 0, ErrAuthRateLimitFailClosed
	}
	if len(l.key) < loginHMACMinBytes {
		// Sin clave HMAC válida no hay seudonimización segura: fail-closed explícito.
		return false, 0, ErrLoginRateLimitKeyUnavailable
	}

	ipLimpia := strings.TrimSpace(ip)
	if ipLimpia == "" {
		ipLimpia = "unknown"
	}
	claveIP := loginRateLimitKey(claseLoginIP, l.loginIdentifier(claseLoginIP, ipLimpia))
	claveEmail := loginRateLimitKey(claseLoginEmail, l.loginIdentifier(claseLoginEmail, strings.ToLower(strings.TrimSpace(email))))
	ventanaSegundos := int64(loginWindow / time.Second)

	res, err := l.script.Run(ctx, l.client, []string{claveIP, claveEmail},
		loginIPMax, loginEmailMax, ventanaSegundos).Slice()
	if err != nil {
		return false, 0, fmt.Errorf("%w: %v", ErrAuthRateLimitFailClosed, err)
	}
	if len(res) != 3 {
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

// loginRateLimitKey construye la clave Redis con el identificador ya seudonimizado.
func loginRateLimitKey(clase, identificador string) string {
	return fmt.Sprintf("nexum:v1:ratelimit:%s:%s", sanitizeRateLimitClass(clase), identificador)
}
