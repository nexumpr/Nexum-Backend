package cache

import (
	"context"
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

// LoginRateLimiter implementa appIdentity/http.LoginRateLimiter con dos cuotas simultáneas.
// Cada cuota usa AuthRateLimiter, que incrementa y fija TTL de forma atómica en Redis y
// seudonimiza el identificador con SHA-256: ni el email ni la IP se guardan en claro.
type LoginRateLimiter struct {
	porIP    *AuthRateLimiter
	porEmail *AuthRateLimiter
}

// NewLoginRateLimiter construye el limitador dual. Con cliente nil ambas cuotas fallan
// cerradas (503), de modo que el login nunca se ejecuta sin limitación.
func NewLoginRateLimiter(client *redis.Client) *LoginRateLimiter {
	return &LoginRateLimiter{
		porIP:    NewAuthRateLimiter(client, loginIPMax, loginWindow),
		porEmail: NewAuthRateLimiter(client, loginEmailMax, loginWindow),
	}
}

// AllowLogin evalúa primero la cuota por IP y después la cuota por email normalizado.
// Devuelve el Retry-After de la cuota que haya bloqueado.
func (l *LoginRateLimiter) AllowLogin(ctx context.Context, ip, email string) (bool, time.Duration, error) {
	if l == nil || l.porIP == nil || l.porEmail == nil {
		return false, 0, ErrAuthRateLimitFailClosed
	}

	identificadorIP := strings.TrimSpace(ip)
	if identificadorIP == "" {
		identificadorIP = "unknown"
	}
	permitido, retry, err := l.porIP.Allow(ctx, claseLoginIP, identificadorIP)
	if err != nil {
		return false, 0, err
	}
	if !permitido {
		return false, retry, nil
	}

	permitido, retry, err = l.porEmail.Allow(ctx, claseLoginEmail, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return false, 0, err
	}
	if !permitido {
		return false, retry, nil
	}
	return true, 0, nil
}
