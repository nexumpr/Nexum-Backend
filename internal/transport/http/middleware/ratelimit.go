package middleware

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// RateLimiter aplica limitación de tasa por ventana fija (A-030 §2.3, AC-A-030-01).
// Emite código 429 Too Many Requests y cabecera Retry-After cuando se supera el límite.
type RateLimiter struct {
	mu          sync.Mutex
	cache       ports.DistributedCache
	maxRequests int
	window      time.Duration
	localCounts map[string]int
	localExpiry map[string]time.Time
}

// NewRateLimiter crea una nueva instancia del limitador de tasa.
func NewRateLimiter(cache ports.DistributedCache, maxRequests int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		cache:       cache,
		maxRequests: maxRequests,
		window:      window,
		localCounts: make(map[string]int),
		localExpiry: make(map[string]time.Time),
	}
}

// Middleware retorna la función decoradora HTTP para limitar tasa por IP de cliente.
func (rl *RateLimiter) Middleware() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clientIP := r.RemoteAddr
			if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
				clientIP = forwarded
			}

			key := fmt.Sprintf("ratelimit:%s", clientIP)
			ctx := r.Context()

			allowed, retryAfter := rl.allow(ctx, key)
			if !allowed {
				w.Header().Set("Retry-After", fmt.Sprintf("%d", int(retryAfter.Seconds())))
				httputil.WriteError(w, http.StatusTooManyRequests, "RATE_LIMIT_EXCEEDED", "se ha superado el limite de solicitudes permitidas")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func (rl *RateLimiter) allow(ctx context.Context, key string) (bool, time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	expiry, exists := rl.localExpiry[key]
	if !exists || now.After(expiry) {
		rl.localCounts[key] = 1
		rl.localExpiry[key] = now.Add(rl.window)
		return true, 0
	}

	if rl.localCounts[key] >= rl.maxRequests {
		return false, expiry.Sub(now)
	}

	rl.localCounts[key]++
	return true, 0
}

