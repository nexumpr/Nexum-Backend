package platform_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nexumpr/Nexum-Backend/internal/ports"
)

// mockNoSQLRepository es un doble en memoria que satisface el puerto NoSQLRepository (A-010 §2.3, GOV-04).
type mockNoSQLRepository struct {
	store map[string]map[string]any
}

func newMockNoSQLRepository() *mockNoSQLRepository {
	return &mockNoSQLRepository{store: make(map[string]map[string]any)}
}

func (m *mockNoSQLRepository) GetItem(ctx context.Context, tableName string, key map[string]any) (map[string]any, error) {
	pk := key["PK"].(string)
	if item, exists := m.store[pk]; exists {
		return item, nil
	}
	return nil, ports.ErrItemNotFound
}

func (m *mockNoSQLRepository) PutItem(ctx context.Context, tableName string, item map[string]any, cond *ports.ConditionExpression) error {
	pk := item["PK"].(string)
	m.store[pk] = item
	return nil
}

func (m *mockNoSQLRepository) UpdateItem(ctx context.Context, tableName string, key map[string]any, updateExpr string, exprValues map[string]any, cond *ports.ConditionExpression) error {
	pk := key["PK"].(string)
	if item, exists := m.store[pk]; exists {
		for k, v := range exprValues {
			item[k] = v
		}
		return nil
	}
	return ports.ErrItemNotFound
}

func (m *mockNoSQLRepository) DeleteItem(ctx context.Context, tableName string, key map[string]any, cond *ports.ConditionExpression) error {
	pk := key["PK"].(string)
	delete(m.store, pk)
	return nil
}

func (m *mockNoSQLRepository) Query(ctx context.Context, tableName string, input ports.QueryInput) (*ports.QueryResult, error) {
	var items []map[string]any
	for _, item := range m.store {
		items = append(items, item)
	}
	return &ports.QueryResult{Items: items}, nil
}

// mockDistributedCache es un doble en memoria que satisface el puerto DistributedCache (A-010 §2.4, GOV-05).
type mockDistributedCache struct {
	cache map[string][]byte
}

func newMockDistributedCache() *mockDistributedCache {
	return &mockDistributedCache{cache: make(map[string][]byte)}
}

func (m *mockDistributedCache) Get(ctx context.Context, key string) ([]byte, error) {
	if val, ok := m.cache[key]; ok {
		return val, nil
	}
	return nil, ports.ErrCacheMiss
}

func (m *mockDistributedCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	m.cache[key] = value
	return nil
}

func (m *mockDistributedCache) Delete(ctx context.Context, key string) error {
	delete(m.cache, key)
	return nil
}

func (m *mockDistributedCache) AcquireLock(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	lockKey := "lock:" + key
	if _, exists := m.cache[lockKey]; exists {
		return false, ports.ErrLockNotAcquired
	}
	m.cache[lockKey] = []byte("locked")
	return true, nil
}

func (m *mockDistributedCache) ReleaseLock(ctx context.Context, key string) error {
	lockKey := "lock:" + key
	delete(m.cache, lockKey)
	return nil
}

func TestNoSQLAndCachePortContracts(t *testing.T) {
	ctx := context.Background()

	t.Run("Contrato NoSQLRepository", func(t *testing.T) {
		var repo ports.NoSQLRepository = newMockNoSQLRepository()

		item := map[string]any{"PK": "USER#100", "Email": "usuario@nexum.com"}
		if err := repo.PutItem(ctx, "NexumCore", item, nil); err != nil {
			t.Fatalf("PutItem fallo: %v", err)
		}

		retrieved, err := repo.GetItem(ctx, "NexumCore", map[string]any{"PK": "USER#100"})
		if err != nil {
			t.Fatalf("GetItem fallo: %v", err)
		}
		if retrieved["Email"] != "usuario@nexum.com" {
			t.Errorf("email recuperado incorrecto: %v", retrieved["Email"])
		}

		_, err = repo.GetItem(ctx, "NexumCore", map[string]any{"PK": "USER#INEXISTENTE"})
		if !errors.Is(err, ports.ErrItemNotFound) {
			t.Errorf("se esperaba ErrItemNotFound, se obtuvo: %v", err)
		}
	})

	t.Run("Contrato DistributedCache", func(t *testing.T) {
		var cache ports.DistributedCache = newMockDistributedCache()

		if err := cache.Set(ctx, "session:123", []byte("valid_token"), 5*time.Minute); err != nil {
			t.Fatalf("Set fallo: %v", err)
		}

		val, err := cache.Get(ctx, "session:123")
		if err != nil || string(val) != "valid_token" {
			t.Fatalf("Get fallo o retorno valor incorrecto: %v", err)
		}

		acquired, err := cache.AcquireLock(ctx, "user:100", 1*time.Minute)
		if err != nil || !acquired {
			t.Fatalf("se esperaba adquirir el lock: %v", err)
		}

		// Intento recurrente de adquirir el mismo lock debe fallar
		_, err = cache.AcquireLock(ctx, "user:100", 1*time.Minute)
		if !errors.Is(err, ports.ErrLockNotAcquired) {
			t.Errorf("se esperaba ErrLockNotAcquired, se obtuvo: %v", err)
		}

		if err := cache.ReleaseLock(ctx, "user:100"); err != nil {
			t.Fatalf("ReleaseLock fallo: %v", err)
		}
	})
}
