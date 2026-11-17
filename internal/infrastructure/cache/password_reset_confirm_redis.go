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

// scriptCuota atomiza INCR + PEXPIRE en una sola operación: si el proceso se interrumpe
// entre ambas, la clave nunca queda sin vencimiento (cuota permanente). Misma semántica
// fail-closed que E005: cualquier error de Redis se propaga al handler (503).
const scriptCuota = `
local n = redis.call("INCR", KEYS[1])
if n == 1 then
  redis.call("PEXPIRE", KEYS[1], ARGV[1])
end
return n`

// ResetConfirmLimiter aplica la cuota de E006 (confirmación de cambio de contraseña) en
// Redis distribuido: 10 intentos por IP cada 15 minutos. Fail-closed: si Redis falla, el
// endpoint responde 503; nunca se degrada a "sin límite".
type ResetConfirmLimiter struct {
	client  *redis.Client
	hmacKey []byte
}

// NewResetConfirmLimiter construye el limitador. Sin cliente o sin clave HMAC no se crea.
func NewResetConfirmLimiter(client *redis.Client, hmacKey []byte) (*ResetConfirmLimiter, error) {
	if client == nil || len(hmacKey) == 0 {
		return nil, errors.New("limitador de confirmacion no configurable")
	}
	return &ResetConfirmLimiter{client: client, hmacKey: hmacKey}, nil
}

// AllowResetConfirm consume una unidad de cuota para la IP indicada. La IP se seudonimiza
// con HMAC-SHA256: la clave de Redis no contiene direcciones en claro.
func (l *ResetConfirmLimiter) AllowResetConfirm(ctx context.Context, ip string) (bool, time.Duration, error) {
	const (
		limite  = int64(10)
		ventana = 15 * time.Minute
	)
	if l == nil || l.client == nil || ip == "" {
		return false, 0, errors.New("limitador no disponible")
	}

	mac := hmac.New(sha256.New, l.hmacKey)
	mac.Write([]byte("pwreset-confirm:" + ip))
	clave := "nexum:v1:pwreset:confirm:" + hex.EncodeToString(mac.Sum(nil))[:32]

	n, err := l.client.Eval(ctx, scriptCuota, []string{clave}, ventana.Milliseconds()).Int64()
	if err != nil {
		return false, 0, fmt.Errorf("cuota de confirmacion: %w", err)
	}
	if n > limite {
		return false, ventana, nil
	}
	return true, 0, nil
}
