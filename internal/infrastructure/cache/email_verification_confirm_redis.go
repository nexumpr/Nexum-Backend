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

// EmailVerificationConfirmLimiter aplica la cuota de E011 (confirmacion de verificacion) en
// Redis distribuido: 10 intentos por IP cada 15 minutos.
//
// Fail-closed: si Redis falla, el handler responde 503; nunca se degrada a "sin limite". La IP
// se seudonimiza con HMAC-SHA256: la clave de Redis no contiene direcciones en claro.
type EmailVerificationConfirmLimiter struct {
	client  *redis.Client
	hmacKey []byte
}

// NewEmailVerificationConfirmLimiter construye el limitador. Sin cliente o sin clave HMAC no
// se crea.
func NewEmailVerificationConfirmLimiter(client *redis.Client, hmacKey []byte) (*EmailVerificationConfirmLimiter, error) {
	if client == nil || len(hmacKey) == 0 {
		return nil, errors.New("limitador de confirmacion de verificacion no configurable")
	}
	return &EmailVerificationConfirmLimiter{client: client, hmacKey: hmacKey}, nil
}

// AllowEmailVerificationConfirm consume una unidad de cuota para la IP indicada. El script Lua
// atomiza INCR y PEXPIRE, de modo que una interrupcion nunca deja la cuota sin vencimiento.
func (l *EmailVerificationConfirmLimiter) AllowEmailVerificationConfirm(ctx context.Context, ip string) (bool, time.Duration, error) {
	const (
		limite  = int64(10)
		ventana = 15 * time.Minute
	)
	if l == nil || l.client == nil || ip == "" {
		return false, 0, errors.New("limitador de confirmacion no disponible")
	}

	mac := hmac.New(sha256.New, l.hmacKey)
	mac.Write([]byte("email-verify:confirm:" + ip))
	clave := "nexum:v1:emailverify:confirm:" + hex.EncodeToString(mac.Sum(nil))[:32]

	n, err := l.client.Eval(ctx, scriptCuota, []string{clave}, ventana.Milliseconds()).Int64()
	if err != nil {
		return false, 0, fmt.Errorf("cuota de confirmacion de verificacion: %w", err)
	}
	if n > limite {
		return false, ventana, nil
	}
	return true, 0, nil
}
