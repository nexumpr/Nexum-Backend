package ports

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrCacheMiss se produce cuando una clave solicitada no existe o expiró en la caché distribuida.
	ErrCacheMiss = errors.New("clave no encontrada en cache distribuida (cache miss)")

	// ErrLockNotAcquired se produce cuando no es posible adquirir un lock distribuido antes del timeout.
	ErrLockNotAcquired = errors.New("no se pudo adquirir el lock distribuido")
)

// DistributedCache define el puerto para operaciones de caché distribuida en Redis (A-010 §2.4, GOV-05).
// Se emplea para almacenamiento temporal compartido entre los 2 nodos, control de concurrencia y rate limiting.
type DistributedCache interface {
	// Get recupera los bytes almacenados bajo una clave. Retorna ErrCacheMiss si no existe.
	Get(ctx context.Context, key string) ([]byte, error)

	// Set almacena un valor bajo una clave con un tiempo de vida (TTL) determinado.
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error

	// Delete elimina explícitamente una clave de la caché (invalidación).
	Delete(ctx context.Context, key string) error

	// AcquireLock intenta adquirir un lock distribuido mutualmente excluyente con TTL de seguridad.
	AcquireLock(ctx context.Context, key string, ttl time.Duration) (bool, error)

	// ReleaseLock libera un lock distribuido previamente adquirido.
	ReleaseLock(ctx context.Context, key string) error
}
