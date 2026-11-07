package memory

import (
	"context"
	"sync"
	"time"

	"github.com/nexumpr/Nexum-Backend/internal/ports"
)

type cacheEntry struct {
	value     []byte
	expiresAt time.Time
}

// MemoryCache es una implementación en memoria y segura para concurrencia de ports.DistributedCache (A-020 §2.2, AC-A-020-03).
// Permite coordinar caché y locks en pruebas locales simulando el comportamiento de Redis compartido entre 2 nodos.
type MemoryCache struct {
	mu    sync.RWMutex
	items map[string]cacheEntry
	locks map[string]time.Time
}

// NewMemoryCache inicializa una nueva instancia de la caché en memoria.
func NewMemoryCache() *MemoryCache {
	return &MemoryCache{
		items: make(map[string]cacheEntry),
		locks: make(map[string]time.Time),
	}
}

// Get recupera los bytes almacenados bajo una clave si no han expirado.
func (c *MemoryCache) Get(ctx context.Context, key string) ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.items[key]
	if !ok {
		return nil, ports.ErrCacheMiss
	}

	if time.Now().After(entry.expiresAt) {
		return nil, ports.ErrCacheMiss
	}

	copied := make([]byte, len(entry.value))
	copy(copied, entry.value)
	return copied, nil
}

// Set almacena un valor bajo una clave con un tiempo de vida (TTL).
func (c *MemoryCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	copied := make([]byte, len(value))
	copy(copied, value)

	c.items[key] = cacheEntry{
		value:     copied,
		expiresAt: time.Now().Add(ttl),
	}
	return nil
}

// Delete remueve una clave de la caché.
func (c *MemoryCache) Delete(ctx context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.items, key)
	return nil
}

// AcquireLock adquiere un lock de exclusión mutua temporal.
func (c *MemoryCache) AcquireLock(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	if exp, exists := c.locks[key]; exists && now.Before(exp) {
		return false, ports.ErrLockNotAcquired
	}

	c.locks[key] = now.Add(ttl)
	return true, nil
}

// ReleaseLock libera un lock previamente adquirido.
func (c *MemoryCache) ReleaseLock(ctx context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.locks, key)
	return nil
}
