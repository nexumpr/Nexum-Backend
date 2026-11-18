package cache

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// GetOrLoadJSON devuelve el JSON cacheado de una clave pública. Si Redis no está
// configurado, falla o no hay entrada, ejecuta load() y guarda el resultado con
// TTL. El fallo de caché nunca impide servir contenido público (fail-open).
//
// No debe usarse para datos de usuario: las claves públicas no llevan identidad.
func GetOrLoadJSON(
	ctx context.Context,
	client *redis.Client,
	key string,
	ttl time.Duration,
	load func() ([]byte, error),
) ([]byte, error) {
	if load == nil {
		return nil, errors.New("carga de contenido nula")
	}
	if client != nil && key != "" {
		if datos, err := client.Get(ctx, key).Bytes(); err == nil && len(datos) > 0 {
			return datos, nil
		}
	}

	datos, err := load()
	if err != nil {
		return nil, err
	}
	if client != nil && key != "" && ttl > 0 {
		// Best-effort: un fallo al escribir la caché no invalida la respuesta.
		_ = client.Set(ctx, key, datos, ttl).Err()
	}
	return datos, nil
}
