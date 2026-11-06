package http

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// HealthCheckFunc evalúa el estado de una dependencia con timeout acotado.
type HealthCheckFunc func(ctx context.Context) error

// HealthRegistry gestiona las sondas de liveness y readiness del backend.
type HealthRegistry struct {
	mu           sync.RWMutex
	dependencies map[string]HealthCheckFunc
	timeout      time.Duration
}

// NewHealthRegistry crea un registro de salud con timeout por comprobación.
func NewHealthRegistry(timeout time.Duration) *HealthRegistry {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return &HealthRegistry{
		dependencies: make(map[string]HealthCheckFunc),
		timeout:      timeout,
	}
}

// RegisterDependency añade una dependencia crítica para la sonda de readiness.
func (hr *HealthRegistry) RegisterDependency(name string, check HealthCheckFunc) {
	hr.mu.Lock()
	defer hr.mu.Unlock()
	hr.dependencies[name] = check
}

// LiveHandler responde siempre 200 OK si el proceso HTTP está en ejecución (sin consultar dependencias).
func (hr *HealthRegistry) LiveHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "UP",
		})
	}
}

// ReadyResponse estructura la respuesta de la sonda readiness sin exponer secretos.
type ReadyResponse struct {
	Status       string            `json:"status"`
	Dependencies map[string]string `json:"dependencies"`
}

// ReadyHandler verifica concurrentemente cada dependencia registrada con timeout acotado.
// Si alguna dependencia crítica falla, retorna HTTP 503 Service Unavailable (AC-A-090-02).
func (hr *HealthRegistry) ReadyHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hr.mu.RLock()
		deps := make(map[string]HealthCheckFunc, len(hr.dependencies))
		for k, v := range hr.dependencies {
			deps[k] = v
		}
		hr.mu.RUnlock()

		ctx, cancel := context.WithTimeout(r.Context(), hr.timeout)
		defer cancel()

		type checkResult struct {
			name   string
			status string
			err    error
		}

		results := make(chan checkResult, len(deps))
		var wg sync.WaitGroup

		for name, check := range deps {
			wg.Add(1)
			go func(depName string, fn HealthCheckFunc) {
				defer wg.Done()
				err := fn(ctx)
				if err != nil {
					results <- checkResult{name: depName, status: "DOWN", err: err}
				} else {
					results <- checkResult{name: depName, status: "UP", err: nil}
				}
			}(name, check)
		}

		wg.Wait()
		close(results)

		depStatuses := make(map[string]string, len(deps))
		allHealthy := true

		for res := range results {
			depStatuses[res.name] = res.status
			if res.err != nil {
				allHealthy = false
			}
		}

		resp := ReadyResponse{
			Dependencies: depStatuses,
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if allHealthy {
			resp.Status = "UP"
			w.WriteHeader(http.StatusOK)
		} else {
			resp.Status = "DOWN"
			w.WriteHeader(http.StatusServiceUnavailable) // 503
		}

		_ = json.NewEncoder(w).Encode(resp)
	}
}

