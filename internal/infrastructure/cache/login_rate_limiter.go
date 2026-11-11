package cache

import (
	"context"
	"fmt"
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
)

// scriptLoginDual evalua AMBAS cuotas en una sola operacion atomica del servidor Redis:
// incrementa los dos contadores, fija su TTL y decide en el mismo instante. Asi no existe
// una ventana en la que una cuota se haya consumido y la otra no se haya evaluado.
//
// Claves: KEYS[1] = cuota por IP, KEYS[2] = cuota por email.
// Argumentos: ARGV[1] = maximo por IP, ARGV[2] = maximo por email, ARGV[3] = ventana (s).
// Devuelve {permitido (1/0), retry_after_segundos, cuota_que_bloquea (0/1/2)}.
//
// La instancia desplegada (I-050) es Redis OSS de un solo nodo, no Cluster: las dos claves
// pueden participar en el mismo script. Si en el futuro se migrara a Cluster, ambas claves
// tendrian que compartir hash slot (mismo prefijo con hash tags).
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

// loginRateLimitKey construye la clave Redis de una cuota de login con el identificador
// seudonimizado (nunca el valor en claro) y una clase propia, separada de las de E001.
func loginRateLimitKey(clase, identificador string) string {
	return fmt.Sprintf("nexum:v1:ratelimit:%s:%s", sanitizeRateLimitClass(clase), hashRateLimitIdentifier(identificador))
}

// LoginRateLimiter implementa la interfaz del handler con una decisión conjunta y atómica.
// Los identificadores se seudonimizan antes de construir la clave: ni el email ni la IP
// se almacenan en claro en Redis.
type LoginRateLimiter struct {
	client *redis.Client
	script *redis.Script
}

// NewLoginRateLimiter construye el limitador dual. Con cliente nil, AllowLogin falla
// cerrado (503): el login nunca se ejecuta sin limitación.
func NewLoginRateLimiter(client *redis.Client) *LoginRateLimiter {
	return &LoginRateLimiter{client: client, script: redis.NewScript(scriptLoginDual)}
}

// AllowLogin evalúa las dos cuotas en una única ejecución atómica del script.
func (l *LoginRateLimiter) AllowLogin(ctx context.Context, ip, email string) (bool, time.Duration, error) {
	if l == nil || l.client == nil {
		return false, 0, ErrAuthRateLimitFailClosed
	}

	ipLimpia := strings.TrimSpace(ip)
	if ipLimpia == "" {
		ipLimpia = "unknown"
	}
	claveIP := loginRateLimitKey(claseLoginIP, ipLimpia)
	claveEmail := loginRateLimitKey(claseLoginEmail, strings.ToLower(strings.TrimSpace(email)))
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
