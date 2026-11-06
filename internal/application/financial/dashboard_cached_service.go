package financial

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	domainCache "github.com/nexumpr/Nexum-Backend/internal/domain/cache"
	infraCache "github.com/nexumpr/Nexum-Backend/internal/infrastructure/cache"
)

// UserDashboardData representa el resumen financiero agregado presentado al usuario (A-080 §2.3, E051).
type UserDashboardData struct {
	UserID         string    `json:"userId"`
	AvailableCents int64     `json:"availableCents"`
	TotalSavedCents int64    `json:"totalSavedCents"`
	ActiveGoals    int       `json:"activeGoals"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// DataFetcher función abstracta que recupera los datos desde la fuente de verdad (DynamoDB).
type DataFetcher func(ctx context.Context, userID string) (*UserDashboardData, error)

// DashboardCachedService coordina lecturas eficientes con Read-Through cache e invalidación inmediata (A-080 §2.3, §2.4).
type DashboardCachedService struct {
	cache   *infraCache.MemoryDistributedCache
	fetcher DataFetcher
	env     string
	ttl     time.Duration
}

// NewDashboardCachedService inicializa el servicio con política de 30 segundos de TTL (A-080 §2.3: 20-60s).
func NewDashboardCachedService(cache *infraCache.MemoryDistributedCache, fetcher DataFetcher, env string) *DashboardCachedService {
	if env == "" {
		env = "local"
	}
	return &DashboardCachedService{
		cache:   cache,
		fetcher: fetcher,
		env:     env,
		ttl:     30 * time.Second,
	}
}

// GetUserDashboard recupera el dashboard con estrategia Read-Through.
// Garantiza que la clave sea única por usuario (AC-A-080-02: User A no comparte caché con User B).
func (s *DashboardCachedService) GetUserDashboard(ctx context.Context, userID string, dataVersion int64) (*UserDashboardData, bool, error) {
	cacheKey := domainCache.UserCacheKey(s.env, userID, "dashboard", "", dataVersion)

	// 1. Intentar lectura en caché
	cachedJSON, hit, err := s.cache.Get(ctx, cacheKey)
	if err == nil && hit {
		var data UserDashboardData
		if jsonErr := json.Unmarshal([]byte(cachedJSON), &data); jsonErr == nil {
			return &data, true, nil // Cache Hit
		}
	}

	// 2. Cache Miss o fallo de caché: consultar fuente de verdad (DynamoDB)
	freshData, fetchErr := s.fetcher(ctx, userID)
	if fetchErr != nil {
		return nil, false, fmt.Errorf("error al obtener dashboard desde almacenamiento: %w", fetchErr)
	}

	// 3. Poblar caché de forma asíncrona/no bloqueante
	if serialized, err := json.Marshal(freshData); err == nil {
		_ = s.cache.Set(ctx, cacheKey, string(serialized), s.ttl)
	}

	return freshData, false, nil // Cache Miss
}

// InvalidateUserCache elimina de inmediato todas las proyecciones cacheadas del usuario tras una mutación financiera (A-080 §2.4).
func (s *DashboardCachedService) InvalidateUserCache(ctx context.Context, userID string) error {
	prefix := domainCache.UserCachePrefix(s.env, userID)
	return s.cache.DeletePrefix(ctx, prefix)
}
