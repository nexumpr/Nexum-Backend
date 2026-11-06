package cache_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	appFinancial "github.com/nexumpr/Nexum-Backend/internal/application/financial"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
	infraCache "github.com/nexumpr/Nexum-Backend/internal/infrastructure/cache"
	infraDynamo "github.com/nexumpr/Nexum-Backend/internal/infrastructure/dynamo"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// TestTwoInstancesDistributedRateLimitAndInvalidation verifica AC-A-080-01:
// Dos instancias (Nodo A y Nodo B) comparten el rate limit y la invalidación a través de la caché distribuida.
func TestTwoInstancesDistributedRateLimitAndInvalidation(t *testing.T) {
	ctx := context.Background()

	// Caché distribuida compartida (simula Redis cluster entre múltiples nodos Go)
	sharedCache := infraCache.NewMemoryDistributedCache()

	// Rate limiters independientes en cada nodo compartiendo el mismo almacén
	nodeALimiter := infraCache.NewRateLimiter(sharedCache)
	nodeBLimiter := infraCache.NewRateLimiter(sharedCache)

	accountID := "user-login-audit@nexum.finance"
	maxRequests := int64(5)
	window := 1 * time.Minute

	// 1. Nodo A procesa 3 intentos de login
	for i := 1; i <= 3; i++ {
		allowed, rem, _, err := nodeALimiter.CheckLimit(ctx, "auth_login", accountID, maxRequests, window, true)
		if err != nil || !allowed {
			t.Fatalf("Nodo A: Intento %d debió ser permitido, got err=%v", i, err)
		}
		if rem != maxRequests-int64(i) {
			t.Fatalf("Nodo A: Restante incorrecto: got %d, want %d", rem, maxRequests-int64(i))
		}
	}

	// 2. Nodo B procesa 2 intentos adicionales (agotando el cupo global de 5)
	for i := 4; i <= 5; i++ {
		allowed, _, _, err := nodeBLimiter.CheckLimit(ctx, "auth_login", accountID, maxRequests, window, true)
		if err != nil || !allowed {
			t.Fatalf("Nodo B: Intento %d debió ser permitido, got err=%v", i, err)
		}
	}

	// 3. Intento 6 en Nodo B: DEBE SER RECHAZADO con 429 Too Many Requests (AC-A-080-01)
	allowed, _, retryAfter, err := nodeBLimiter.CheckLimit(ctx, "auth_login", accountID, maxRequests, window, true)
	if allowed || !errors.Is(err, infraCache.ErrRateLimitExceeded) {
		t.Fatalf("AC-A-080-01 Fallido: Nodo B no reconoció el límite agotado por Nodo A. err=%v", err)
	}
	if retryAfter <= 0 {
		t.Fatalf("Se esperaba cabecera Retry-After positiva")
	}

	// 4. Verificación de invalidación compartida:
	fetchCount := 0
	mockFetcher := func(ctx context.Context, userID string) (*appFinancial.UserDashboardData, error) {
		fetchCount++
		return &appFinancial.UserDashboardData{
			UserID:          userID,
			AvailableCents:  150000,
			TotalSavedCents: 50000,
			ActiveGoals:     2,
			UpdatedAt:       time.Now().UTC(),
		}, nil
	}

	serviceA := appFinancial.NewDashboardCachedService(sharedCache, mockFetcher, "test")
	serviceB := appFinancial.NewDashboardCachedService(sharedCache, mockFetcher, "test")

	userID := "usr-shared-inv-01"

	// a) Nodo A consulta -> Cache Miss (fetchCount = 1)
	_, hitA, _ := serviceA.GetUserDashboard(ctx, userID, 1)
	if hitA || fetchCount != 1 {
		t.Fatalf("Nodo A debió reportar Cache Miss en primera llamada")
	}

	// b) Nodo B consulta -> Cache Hit (fetchCount sigue en 1)
	_, hitB, _ := serviceB.GetUserDashboard(ctx, userID, 1)
	if !hitB || fetchCount != 1 {
		t.Fatalf("Nodo B debió aprovechar la respuesta cacheada por Nodo A")
	}

	// c) Nodo A invalida la caché del usuario tras una mutación financiera
	if err := serviceA.InvalidateUserCache(ctx, userID); err != nil {
		t.Fatalf("Error al invalidar caché en Nodo A: %v", err)
	}

	// d) Nodo B consulta nuevamente -> Debe ser Cache Miss y refrescar (fetchCount = 2, AC-A-080-01)
	_, hitAfterB, _ := serviceB.GetUserDashboard(ctx, userID, 1)
	if hitAfterB || fetchCount != 2 {
		t.Fatalf("AC-A-080-01 Fallido: Nodo B sirvió caché vieja después de invalidación en Nodo A")
	}

	t.Logf("AC-A-080-01 Verificado: Rate limit e invalidación compartidos exactamente entre Nodo A y Nodo B.")
}

// TestMultiUserCacheIsolation verifica AC-A-080-02:
// El acceso entre usuarios no comparte respuestas cacheadas (aislamiento multi-cuenta estricto).
func TestMultiUserCacheIsolation(t *testing.T) {
	ctx := context.Background()
	sharedCache := infraCache.NewMemoryDistributedCache()

	userBalances := map[string]int64{
		"usr-alice": 125000, // $1,250.00
		"usr-bob":   850000, // $8,500.00
	}

	fetcher := func(ctx context.Context, userID string) (*appFinancial.UserDashboardData, error) {
		bal := userBalances[userID]
		return &appFinancial.UserDashboardData{
			UserID:         userID,
			AvailableCents: bal,
		}, nil
	}

	service := appFinancial.NewDashboardCachedService(sharedCache, fetcher, "test")

	// 1. Alice consulta y su resultado queda cacheado
	aliceData, hitAlice, _ := service.GetUserDashboard(ctx, "usr-alice", 1)
	if hitAlice || aliceData.AvailableCents != 125000 {
		t.Fatalf("Datos de Alice incorrectos")
	}

	// 2. Bob consulta el mismo recurso 'dashboard':
	// REGLA AC-A-080-02: Bob NO debe recibir el caché de Alice bajo ninguna circunstancia
	bobData, hitBob, _ := service.GetUserDashboard(ctx, "usr-bob", 1)
	if hitBob {
		t.Fatalf("AC-A-080-02 Violado: Bob recibió un Cache Hit proveniente de otra cuenta")
	}
	if bobData.AvailableCents != 850000 {
		t.Fatalf("AC-A-080-02 Violado: Bob recibió saldo incorrecto (%d != 850000)", bobData.AvailableCents)
	}

	// 3. Segunda consulta de Alice debe ser Hit con sus propios datos
	aliceSecond, hitSecond, _ := service.GetUserDashboard(ctx, "usr-alice", 1)
	if !hitSecond || aliceSecond.AvailableCents != 125000 {
		t.Fatalf("Segunda consulta de Alice fallida")
	}

	t.Logf("AC-A-080-02 Verificado: Aislamiento absoluto de caché entre cuentas de usuario.")
}

// TestCursorPaginationTenThousandItemsNoScan verifica AC-A-080-03:
// 10,000 movimientos paginados deterministamente mediante cursor opaco sin Scan ni offset.
func TestCursorPaginationTenThousandItemsNoScan(t *testing.T) {
	ctx := context.Background()
	table := infraDynamo.NewDynamoTable()
	userID := "usr-scale-10k"

	totalItems := 10000
	pageSize := 100 // 100 páginas de 100 elementos

	baseTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// 1. Poblar 10,000 movimientos deterministas bajo la misma partición USER#<id>
	for i := 0; i < totalItems; i++ {
		movID := fmt.Sprintf("mov-%06d", i)
		occurredAt := baseTime.Add(time.Duration(i) * time.Minute)
		pk, sk, _, _ := domainDynamo.MovementKeys(userID, movID, "ANT", occurredAt)

		item := domainDynamo.NewDynamoItem(pk, sk, "MOVEMENT", map[string]any{
			"movementId":  movID,
			"amountCents": int64(100 + (i % 500)),
		})
		_ = table.PutItem(ctx, item, false, nil)
	}

	// 2. Paginar los 10,000 elementos mediante Query particionado con límites (sin Scan ni offset)
	retrievedCount := 0
	pagesCount := 0
	prefix := domainDynamo.PrefixMovement

	secret := []byte("signing-key-for-cursor-pagination-32bytes")

	var lastSK string
	for {
		pagesCount++
		// Query directo sobre la partición del usuario acotado por prefijo y límite (AC-A-080-03)
		items, err := table.QueryPartition(ctx, domainDynamo.PrefixUser+userID, prefix, lastSK, true, pageSize)
		if err != nil {
			t.Fatalf("Fallo en Query de paginación en página %d: %v", pagesCount, err)
		}

		if len(items) == 0 {
			break
		}

		retrievedCount += len(items)
		lastItem := items[len(items)-1]

		if len(items) < pageSize {
			// Última página alcanzada
			break
		}

		// Generar cursor opaco firmado para la siguiente página (A-080 §2.6)
		cursor, err := httputil.GenerateCursor(secret, userID, "SK", lastItem.SK, time.Now().UTC(), 1*time.Hour)
		if err != nil {
			t.Fatalf("Error al generar cursor opaco: %v", err)
		}

		// Validar cursor firmado en el siguiente salto
		payload, err := httputil.ValidateCursor(secret, cursor, userID)
		if err != nil || payload.LastID != lastItem.SK {
			t.Fatalf("Cursor opaco inválido o alterado en salto de página: %v", err)
		}

		lastSK = payload.LastID
		if pagesCount > 105 {
			t.Fatalf("Bucle infinito detectado en paginación")
		}
	}

	if retrievedCount != totalItems {
		t.Fatalf("AC-A-080-03 Fallido: Se esperaban %d elementos paginados, recuperados: %d en %d páginas", totalItems, retrievedCount, pagesCount)
	}

	t.Logf("AC-A-080-03 Verificado: 10,000 movimientos paginados deterministamente sin Scan ni offset.")
}

// TestFailClosedOnAuthRateLimit verifica la política fail-closed en autenticación (A-080 §2.5).
func TestFailClosedOnAuthRateLimit(t *testing.T) {
	ctx := context.Background()
	cache := infraCache.NewMemoryDistributedCache()
	limiter := infraCache.NewRateLimiter(cache)

	// Simular indisponibilidad de Redis (caída de red)
	cache.SetAvailability(false)

	// 1. Endpoint sensible (login) con failClosed=true -> DEBE DENEGAR con 503 (A-080 §2.5)
	allowed, _, _, err := limiter.CheckLimit(ctx, "auth_login", "attacker-ip", 5, 1*time.Minute, true)
	if allowed || !errors.Is(err, infraCache.ErrAuthRateLimitFailClosed) {
		t.Fatalf("Se esperaba fallo fail-closed ErrAuthRateLimitFailClosed ante caída de Redis, got allowed=%v, err=%v", allowed, err)
	}

	// 2. Endpoint ordinario (lectura de catálogo) con failClosed=false -> DEBE DEGRADAR permitiendo acceso (fail-open)
	allowedDegrade, _, _, errDegrade := limiter.CheckLimit(ctx, "public_catalog", "client-ip", 100, 1*time.Minute, false)
	if !allowedDegrade || errDegrade != nil {
		t.Fatalf("Se esperaba degradación suave (fail-open) para lectura pública, got allowed=%v, err=%v", allowedDegrade, errDegrade)
	}
}
