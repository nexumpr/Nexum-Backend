package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

var authRateLimitScript = redis.NewScript(`
local count = redis.call('INCR', KEYS[1])
if count == 1 then
  redis.call('EXPIRE', KEYS[1], ARGV[1])
end
local ttl = redis.call('TTL', KEYS[1])
if ttl < 0 then
  ttl = tonumber(ARGV[1])
end
return {count, ttl}
`)

// AuthRateLimiter aplica ventanas fijas por clase de endpoint e identificador usando
// Redis real. El identificador (IP o usuario) se hashea para no almacenar PII en claves.
//
// Cada clase usa su propio contador: un usuario agotado en `notifications` no consume
// la cuota de `analytics` ni la de `push`. `porClase` permite fijar un límite distinto
// por dominio; las clases ausentes usan `max`.
type AuthRateLimiter struct {
	client   *redis.Client
	max      int64
	porClase map[string]int64
	window   time.Duration
}

// NewAuthRateLimiter construye el limitador con un límite único para todas las clases.
// Si el cliente es nil, Allow falla cerrado.
func NewAuthRateLimiter(client *redis.Client, max int64, window time.Duration) *AuthRateLimiter {
	return NewAuthRateLimiterPorClase(client, max, nil, window)
}

// NewAuthRateLimiterPorClase construye el limitador con un mapa explícito de límites
// por clase. Las claves se normalizan igual que la clave de Redis; un límite no
// positivo se ignora y la clase cae al valor por defecto.
func NewAuthRateLimiterPorClase(client *redis.Client, porDefecto int64, porClase map[string]int64, window time.Duration) *AuthRateLimiter {
	if porDefecto <= 0 {
		porDefecto = 5
	}
	if window <= 0 {
		window = 15 * time.Minute
	}
	limpios := make(map[string]int64, len(porClase))
	for clase, limite := range porClase {
		if limite <= 0 {
			continue
		}
		limpios[sanitizeRateLimitClass(clase)] = limite
	}
	return &AuthRateLimiter{
		client:   client,
		max:      porDefecto,
		porClase: limpios,
		window:   window,
	}
}

// limitePorClase resuelve el máximo aplicable a la clase. Es determinista y no
// consulta Redis: el aislamiento real lo da la clave, no este valor.
func (l *AuthRateLimiter) limitePorClase(class string) int64 {
	if l == nil {
		return 0
	}
	if limite, ok := l.porClase[sanitizeRateLimitClass(class)]; ok {
		return limite
	}
	return l.max
}

// claveRateLimit deriva la clave de Redis. Incluye la clase y el hash del sujeto:
// dos dominios o dos usuarios nunca comparten contador.
func claveRateLimit(class, identifier string) string {
	return fmt.Sprintf("nexum:v1:ratelimit:%s:%s", sanitizeRateLimitClass(class), hashRateLimitIdentifier(identifier))
}

// Allow incrementa el contador de forma atomica y devuelve si la peticion cabe en la ventana.
func (l *AuthRateLimiter) Allow(ctx context.Context, class, identifier string) (bool, time.Duration, error) {
	if l == nil || l.client == nil {
		return false, 0, errors.New("limitador de autenticacion no configurado")
	}

	key := claveRateLimit(class, identifier)
	maximo := l.limitePorClase(class)
	seconds := int64(l.window / time.Second)
	if seconds <= 0 {
		seconds = 1
	}

	result, err := authRateLimitScript.Run(ctx, l.client, []string{key}, seconds).Result()
	if err != nil {
		return false, 0, fmt.Errorf("redis rate limit: %w", err)
	}

	values, ok := result.([]interface{})
	if !ok || len(values) != 2 {
		return false, 0, errors.New("respuesta inesperada del script de rate limit")
	}
	count, okCount := values[0].(int64)
	ttl, okTTL := values[1].(int64)
	if !okCount || !okTTL {
		return false, 0, errors.New("respuesta invalida del script de rate limit")
	}

	if count > maximo {
		if ttl <= 0 {
			ttl = seconds
		}
		return false, time.Duration(ttl) * time.Second, nil
	}

	return true, 0, nil
}

func sanitizeRateLimitClass(class string) string {
	clean := strings.TrimSpace(class)
	if clean == "" {
		return "default"
	}
	var b strings.Builder
	for _, r := range clean {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '-':
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "default"
	}
	return b.String()
}

func hashRateLimitIdentifier(identifier string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(identifier)))
	return hex.EncodeToString(sum[:])
}
