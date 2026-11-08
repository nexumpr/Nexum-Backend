package security_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nexumpr/Nexum-Backend/internal/adapters/memory"
	infraCache "github.com/nexumpr/Nexum-Backend/internal/infrastructure/cache"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
)

// Caracteriza un defecto de A-030: el middleware HTTP cuenta en memoria por
// instancia y confía en X-Forwarded-For sin validar el proxy. No corrige A-030.
func TestCaracterizaDefectoHTTPRateLimitLocalYConfiaXFF(t *testing.T) {
	cacheCompartida := memory.NewMemoryCache()
	nodoA := middleware.NewRateLimiter(cacheCompartida, 2, time.Minute)
	nodoB := middleware.NewRateLimiter(cacheCompartida, 2, time.Minute)
	handlerA := nodoA.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
	handlerB := nodoB.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))

	permitidas := 0
	for _, handler := range []http.Handler{handlerA, handlerB} {
		for i := 0; i < 2; i++ {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
			req.RemoteAddr = "10.0.0.1:1234"
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code == http.StatusOK {
				permitidas++
			}
		}
	}
	// Dos instancias con el mismo caché permiten 2 cada una: no hay coordinación.
	if permitidas != 4 {
		t.Fatalf("se esperaba comportamiento local (4 permitidas), obtenido %d", permitidas)
	}

	// Rotar X-Forwarded-For evade el límite de la misma IP real.
	limiter := middleware.NewRateLimiter(cacheCompartida, 1, time.Minute)
	handler := limiter.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
	evadidas := 0
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
		req.RemoteAddr = "10.0.0.9:1234"
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("203.0.113.%d", i+1))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code == http.StatusOK {
			evadidas++
		}
	}
	if evadidas != 5 {
		t.Fatalf("se esperaba evasión por XFF (5 permitidas), obtenido %d", evadidas)
	}
}

// Demuestra coordinación simulada del limitador de A-080 mediante caché compartida.
// NO demuestra Redis real ni dos instancias AWS.
func TestInfraRateLimitCoordinaPorCacheCompartida(t *testing.T) {
	compartida := infraCache.NewMemoryDistributedCache()
	nodoA := infraCache.NewRateLimiter(compartida)
	nodoB := infraCache.NewRateLimiter(compartida)
	ctx := context.Background()

	permitidas := 0
	denegadas := 0
	for i := 0; i < 4; i++ {
		limiter := nodoA
		if i%2 == 1 {
			limiter = nodoB
		}
		allowed, _, _, err := limiter.CheckLimit(ctx, "auth", "10.0.0.1", 2, time.Minute, true)
		if err != nil && !errors.Is(err, infraCache.ErrRateLimitExceeded) {
			t.Fatalf("error inesperado: %v", err)
		}
		if allowed && err == nil {
			permitidas++
		} else {
			denegadas++
		}
	}
	if permitidas != 2 || denegadas != 2 {
		t.Errorf("con caché compartida se esperaban 2/2; obtenido %d permitidas y %d denegadas", permitidas, denegadas)
	}
}
