package cache

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// PasswordChangeLimiter aplica la cuota de E007 (cambio de contraseña de una sesión activa)
// en Redis distribuido: 5 intentos por usuario cada 15 minutos.
//
// La clave se seudonimiza con HMAC-SHA256: Redis nunca almacena el `userId` en claro y sin la
// clave secreta no se puede reconstruirlo. Fail-closed: si Redis falla, el endpoint responde
// 503; nunca se degrada a "sin límite" (A-080 §2.5).
//
// Reutiliza `scriptCuota`, que atomiza INCR + PEXPIRE: una interrupción entre ambas nunca deja
// una clave sin vencimiento (cuota permanente).
type PasswordChangeLimiter struct {
	client  *redis.Client
	hmacKey []byte
}

// NewPasswordChangeLimiter construye el limitador. Sin cliente o sin clave HMAC no se crea.
func NewPasswordChangeLimiter(client *redis.Client, hmacKey []byte) (*PasswordChangeLimiter, error) {
	if client == nil || len(hmacKey) == 0 {
		return nil, errors.New("limitador de cambio de contrasena no configurable")
	}
	return &PasswordChangeLimiter{client: client, hmacKey: hmacKey}, nil
}

// Permitir consume una unidad de cuota para el usuario indicado. El segundo valor es el
// tiempo de espera sugerido cuando la cuota está agotada.
func (l *PasswordChangeLimiter) Permitir(ctx context.Context, clave string) (bool, time.Duration, error) {
	const (
		limite  = int64(5)
		ventana = 15 * time.Minute
	)
	if l == nil || l.client == nil || len(l.hmacKey) == 0 || clave == "" {
		return false, 0, errors.New("limitador no disponible")
	}

	mac := hmac.New(sha256.New, l.hmacKey)
	mac.Write([]byte("pwchange:" + clave))
	claveRedis := "nexum:v1:pwchange:" + hex.EncodeToString(mac.Sum(nil))[:32]

	n, err := l.client.Eval(ctx, scriptCuota, []string{claveRedis}, ventana.Milliseconds()).Int64()
	if err != nil {
		return false, 0, fmt.Errorf("cuota de cambio de contrasena: %w", err)
	}
	if n > limite {
		return false, ventana, nil
	}
	return true, 0, nil
}
