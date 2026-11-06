package cache

import (
	"context"
	"errors"
	"time"

	domainCache "github.com/nexumpr/Nexum-Backend/internal/domain/cache"
)

var (
	// ErrRateLimitExceeded se retorna cuando se superan las peticiones permitidas en la ventana (HTTP 429).
	ErrRateLimitExceeded = errors.New("limite de peticiones excedido (429 Too Many Requests)")

	// ErrAuthRateLimitFailClosed se retorna en endpoints de auth si Redis no está disponible (fail-closed, 503).
	ErrAuthRateLimitFailClosed = errors.New("servicio de autenticacion temporalmente no disponible por seguridad (fail-closed)")
)

// RateLimiter gestiona la limitación de tasa coordinada a través de Redis distribuido (A-080 §2.5, AC-A-080-01).
type RateLimiter struct {
	cache *MemoryDistributedCache
}

// NewRateLimiter inicializa el limitador sobre el caché provisto.
func NewRateLimiter(cache *MemoryDistributedCache) *RateLimiter {
	return &RateLimiter{cache: cache}
}

// CheckLimit verifica si una acción dentro de una clase de endpoint está permitida para un sujeto (IP o usuario).
// failClosed: si es true (ej: login), ante fallo de Redis deniega el acceso con 503 para evitar bypass (A-080 §2.5).
//             si es false (ej: GET catálogos), degrada permitiendo la petición (fail-open).
func (rl *RateLimiter) CheckLimit(
	ctx context.Context,
	endpointClass string,
	identifier string,
	maxRequests int64,
	window time.Duration,
	failClosed bool,
) (allowed bool, remaining int64, retryAfter time.Duration, err error) {
	key := domainCache.RateLimitKey(endpointClass, identifier)

	count, cacheErr := rl.cache.IncrementAtomic(ctx, key, window)
	if cacheErr != nil {
		if failClosed {
			// En endpoints sensibles como autenticación, fail-closed previene ataques de fuerza bruta
			return false, 0, window, ErrAuthRateLimitFailClosed
		}
		// En endpoints de lectura, degradar permitiendo la solicitud
		return true, maxRequests, 0, nil
	}

	if count > maxRequests {
		return false, 0, window, ErrRateLimitExceeded
	}

	rem := maxRequests - count
	if rem < 0 {
		rem = 0
	}
	return true, rem, 0, nil
}
