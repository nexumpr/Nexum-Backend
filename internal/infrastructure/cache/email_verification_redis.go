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

// EmailVerificationLimiter aplica la cuota de E010 (solicitud de verificacion de correo) en
// Redis distribuido: 3 solicitudes por usuario cada 15 minutos.
//
// Fail-closed: si Redis falla, el servicio devuelve error y el endpoint responde 503; nunca
// se degrada a "sin limite". La clave de Redis no contiene el userId en claro: se seudonimiza
// con HMAC-SHA256 usando la misma clave de cuotas ya desplegada.
type EmailVerificationLimiter struct {
	client  *redis.Client
	hmacKey []byte
}

// NewEmailVerificationLimiter construye el limitador. Sin cliente o sin clave HMAC no se crea.
func NewEmailVerificationLimiter(client *redis.Client, hmacKey []byte) (*EmailVerificationLimiter, error) {
	if client == nil || len(hmacKey) == 0 {
		return nil, errors.New("limitador de verificacion de correo no configurable")
	}
	return &EmailVerificationLimiter{client: client, hmacKey: hmacKey}, nil
}

// identificador deriva la clave de cuota sin exponer el identificador original.
func (l *EmailVerificationLimiter) identificador(clase, valor string) string {
	mac := hmac.New(sha256.New, l.hmacKey)
	mac.Write([]byte("email-verify:" + clase + ":" + valor))
	return "nexum:v1:emailverify:" + clase + ":" + hex.EncodeToString(mac.Sum(nil))[:32]
}

// Permitir consume una unidad de cuota para el ámbito indicado ("user:<userId>").
//
// Implementa ports.Limitador, el mismo puerto que usa E005: el script Lua atomiza INCR y
// PEXPIRE, de modo que una interrupción nunca deja la cuota sin vencimiento.
// Límite: 3 solicitudes por usuario cada 15 minutos.
func (l *EmailVerificationLimiter) Permitir(ctx context.Context, clave string) (bool, error) {
	const (
		limite  = int64(3)
		ventana = 15 * time.Minute
	)
	if l == nil || l.client == nil || clave == "" {
		return false, errors.New("limitador de verificacion no disponible")
	}

	n, err := l.client.Eval(ctx, scriptCuota, []string{l.identificador("user", clave)}, ventana.Milliseconds()).Int64()
	if err != nil {
		return false, fmt.Errorf("cuota de verificacion: %w", err)
	}
	return n <= limite, nil
}
