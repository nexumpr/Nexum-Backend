package cache

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	// ErrCacheUnavailable simula la caída de Redis para pruebas de resiliencia y degradación (A-080 §2.5, §2.8).
	ErrCacheUnavailable = errors.New("servicio de cache Redis no disponible")
)

type cacheItem struct {
	value     string
	expiresAt time.Time
}

// MemoryDistributedCache emula un cluster de Redis distribuido en memoria, con soporte para
// TTLs, operaciones atómicas de incremento e invalidaciones por prefijo (A-080 §2.1, §2.4).
type MemoryDistributedCache struct {
	mu          sync.RWMutex
	store       map[string]cacheItem
	isAvailable bool

	// Métricas de observabilidad (A-080 §2.8)
	hits   atomic.Int64
	misses atomic.Int64
}

// NewMemoryDistributedCache inicializa una instancia de caché concurrente.
func NewMemoryDistributedCache() *MemoryDistributedCache {
	return &MemoryDistributedCache{
		store:       make(map[string]cacheItem),
		isAvailable: true,
	}
}

// SetAvailability permite simular fallas de conexión o particiones de red con Redis.
func (c *MemoryDistributedCache) SetAvailability(available bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.isAvailable = available
}

func (c *MemoryDistributedCache) Get(ctx context.Context, key string) (string, bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.isAvailable {
		c.misses.Add(1)
		return "", false, ErrCacheUnavailable
	}

	item, exists := c.store[key]
	if !exists {
		c.misses.Add(1)
		return "", false, nil
	}

	if !item.expiresAt.IsZero() && time.Now().UTC().After(item.expiresAt) {
		c.misses.Add(1)
		return "", false, nil
	}

	c.hits.Add(1)
	return item.value, true, nil
}

func (c *MemoryDistributedCache) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.isAvailable {
		return ErrCacheUnavailable
	}

	var exp time.Time
	if ttl > 0 {
		exp = time.Now().UTC().Add(ttl)
	}

	c.store[key] = cacheItem{
		value:     value,
		expiresAt: exp,
	}
	return nil
}

func (c *MemoryDistributedCache) Delete(ctx context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.isAvailable {
		return ErrCacheUnavailable
	}

	delete(c.store, key)
	return nil
}

// DeletePrefix invalida de forma eficiente todas las claves que coinciden con un prefijo (A-080 §2.4).
func (c *MemoryDistributedCache) DeletePrefix(ctx context.Context, prefix string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.isAvailable {
		return ErrCacheUnavailable
	}

	for k := range c.store {
		if strings.HasPrefix(k, prefix) {
			delete(c.store, k)
		}
	}
	return nil
}

// IncrementAtomic realiza un incremento atómico tipo INCR con ventana de expiración en Redis (A-080 §2.5).
func (c *MemoryDistributedCache) IncrementAtomic(ctx context.Context, key string, window time.Duration) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.isAvailable {
		return 0, ErrCacheUnavailable
	}

	now := time.Now().UTC()
	item, exists := c.store[key]

	var count int64 = 1
	var exp time.Time = now.Add(window)

	if exists && (item.expiresAt.IsZero() || now.Before(item.expiresAt)) {
		// Incrementar valor existente
		var current int64
		for _, ch := range item.value {
			current = current*10 + int64(ch-'0')
		}
		count = current + 1
		exp = item.expiresAt
	}

	// Guardar como string
	valStr := string(intToBytes(count))
	c.store[key] = cacheItem{
		value:     valStr,
		expiresAt: exp,
	}

	return count, nil
}

// Stats retorna las métricas de hit ratio y volumen (A-080 §2.8).
func (c *MemoryDistributedCache) Stats() (hits int64, misses int64, hitRatio float64) {
	h := c.hits.Load()
	m := c.misses.Load()
	total := h + m
	ratio := 0.0
	if total > 0 {
		ratio = float64(h) / float64(total)
	}
	return h, m, ratio
}

func intToBytes(n int64) []byte {
	if n == 0 {
		return []byte("0")
	}
	var b []byte
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		b = append([]byte{byte('0' + (n % 10))}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return b
}
