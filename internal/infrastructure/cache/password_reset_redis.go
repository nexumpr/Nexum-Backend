package cache

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// PasswordResetLimiter aplica las cuotas de E005 (por correo y por IP) en Redis
// distribuido. Fail-closed: si Redis falla, devuelve error y el endpoint responde 503.
type PasswordResetLimiter struct {
	client  *redis.Client
	hmacKey []byte
	id      string
}

// NewPasswordResetLimiter construye el limitador. Sin clave HMAC no se crea.
func NewPasswordResetLimiter(client *redis.Client, hmacKey []byte) (*PasswordResetLimiter, error) {
	if client == nil || len(hmacKey) == 0 {
		return nil, errors.New("limitador de recuperacion no configurable")
	}
	sufijo := make([]byte, 4)
	if _, err := rand.Read(sufijo); err != nil {
		return nil, err
	}
	return &PasswordResetLimiter{client: client, hmacKey: hmacKey, id: hex.EncodeToString(sufijo)}, nil
}

func (l *PasswordResetLimiter) identificador(clase, valor string) string {
	mac := hmac.New(sha256.New, l.hmacKey)
	mac.Write([]byte(clase + ":" + valor))
	return "nexum:v1:pwreset:" + clase + ":" + hex.EncodeToString(mac.Sum(nil))[:32]
}

// Permitir consume una cuota atómica en Redis. La clave indica el ámbito:
// "email:<correo>" (3 por 15 min) o "ip:<direccion>" (10 por 15 min).
func (l *PasswordResetLimiter) Permitir(ctx context.Context, clave string) (bool, error) {
	if l == nil || l.client == nil {
		return false, errors.New("limitador no disponible")
	}
	limite := int64(3)
	if len(clave) >= 3 && clave[:3] == "ip:" {
		limite = 10
	}
	return l.consumir(ctx, l.identificador("cuota", clave), limite, 15*time.Minute)
}

func (l *PasswordResetLimiter) consumir(ctx context.Context, clave string, limite int64, ventana time.Duration) (bool, error) {
	n, err := l.client.Incr(ctx, clave).Result()
	if err != nil {
		return false, fmt.Errorf("cuota de recuperacion: %w", err)
	}
	if n == 1 {
		if err := l.client.Expire(ctx, clave, ventana).Err(); err != nil {
			return false, fmt.Errorf("ventana de cuota: %w", err)
		}
	}
	return n <= limite, nil
}

// scriptRenovar extiende el TTL solo si el valor sigue siendo del solicitante.
const scriptRenovar = `
if redis.call("get", KEYS[1]) == ARGV[1] then
  return redis.call("expire", KEYS[1], ARGV[2])
end
return 0`

// scriptLiberar borra la clave solo si pertenece al solicitante.
const scriptLiberar = `
if redis.call("get", KEYS[1]) == ARGV[1] then
  return redis.call("del", KEYS[1])
end
return 0`

// WorkerElector implementa el liderazgo del worker con SET NX + renovación de TTL.
// El lease NO sustituye la reclamación atómica por trabajo.
type WorkerElector struct {
	client *redis.Client
	clave  string
	id     string
}

// NewWorkerElector construye el elector. El identificador distingue instancias.
func NewWorkerElector(client *redis.Client, clave string) (*WorkerElector, error) {
	if client == nil || clave == "" {
		return nil, errors.New("elector de worker no configurable")
	}
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return &WorkerElector{client: client, clave: clave, id: hex.EncodeToString(b)}, nil
}

// Adquirir devuelve true si esta instancia es líder. Renueva el TTL cuando ya lo es y
// respeta el lease ajeno cuando no lo es.
func (e *WorkerElector) Adquirir(ctx context.Context, _ string, ttl time.Duration) (bool, error) {
	if e == nil || e.client == nil {
		return false, errors.New("elector no disponible")
	}
	ok, err := e.client.SetNX(ctx, e.clave, e.id, ttl).Result()
	if err != nil {
		return false, fmt.Errorf("lease de worker: %w", err)
	}
	if ok {
		return true, nil
	}
	// Renovacion atomica: solo el propietario extiende el TTL (sin GET/EXPIRE).
	renovado, err := e.client.Eval(ctx, scriptRenovar, []string{e.clave}, e.id, int(ttl.Seconds())).Int()
	if err != nil {
		return false, fmt.Errorf("renovacion de lease: %w", err)
	}
	return renovado == 1, nil
}

// Liberar cede el liderazgo solo si sigue siendo propio.
func (e *WorkerElector) Liberar(ctx context.Context) {
	if e == nil || e.client == nil {
		return
	}
	// Liberacion atomica: solo borra si sigue siendo propio (sin GET/DEL).
	_, _ = e.client.Eval(ctx, scriptLiberar, []string{e.clave}, e.id).Int()
}
