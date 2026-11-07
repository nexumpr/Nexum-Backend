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

// HealthRegistry gestiona las sondas de liveness y readiness del backend
// (A-090): liveness sólo indica proceso vivo; readiness verifica dependencias
// esenciales con timeout acotado y devuelve 503 cuando no puede atender.
type HealthRegistry struct {
	mu           sync.RWMutex
	dependencies map[string]HealthCheckFunc
	timeout      time.Duration
	shuttingDown bool
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

// BeginShutdown marca el registro como no listo: readiness pasa a 503 antes del
// drenado para que el balanceador retire el destino (A-090).
func (hr *HealthRegistry) BeginShutdown() {
	hr.mu.Lock()
	defer hr.mu.Unlock()
	hr.shuttingDown = true
}

// LiveHandler responde 200 si el proceso HTTP está en ejecución.
// No consulta DynamoDB, Redis ni ninguna dependencia (A-090).
func (hr *HealthRegistry) LiveHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "UP",
		})
	}
}

// ReadyResponse estructura la respuesta de readiness sin exponer detalles
// internos ni mensajes de error de las dependencias.
type ReadyResponse struct {
	Status       string            `json:"status"`
	Dependencies map[string]string `json:"dependencies"`
}

// ReadyHandler verifica cada dependencia registrada con timeout acotado.
//
// Reglas (AC-A-090-02):
//   - 503 durante el apagado.
//   - 503 si no hay ninguna dependencia crítica registrada: no se declara
//     disponibilidad sin evidencia.
//   - 503 si alguna dependencia falla o no responde dentro del timeout.
//   - 200 sólo si todas las dependencias registradas responden correctamente.
func (hr *HealthRegistry) ReadyHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hr.mu.RLock()
		deps := make(map[string]HealthCheckFunc, len(hr.dependencies))
		for name, check := range hr.dependencies {
			deps[name] = check
		}
		apagando := hr.shuttingDown
		timeout := hr.timeout
		hr.mu.RUnlock()

		responder := func(code int, resp ReadyResponse) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(code)
			_ = json.NewEncoder(w).Encode(resp)
		}

		if apagando {
			responder(http.StatusServiceUnavailable, ReadyResponse{
				Status:       "DOWN",
				Dependencies: map[string]string{},
			})
			return
		}

		if len(deps) == 0 {
			responder(http.StatusServiceUnavailable, ReadyResponse{
				Status:       "DOWN",
				Dependencies: map[string]string{},
			})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()

		type resultado struct {
			nombre string
			ok     bool
		}
		resultados := make(chan resultado, len(deps))

		for nombre, check := range deps {
			go func(nombre string, fn HealthCheckFunc) {
				if err := fn(ctx); err != nil {
					resultados <- resultado{nombre: nombre, ok: false}
					return
				}
				resultados <- resultado{nombre: nombre, ok: true}
			}(nombre, check)
		}

		estados := make(map[string]string, len(deps))
		for nombre := range deps {
			estados[nombre] = "UNKNOWN"
		}

		pendientes := len(deps)
		expirado := false
		for pendientes > 0 {
			select {
			case res := <-resultados:
				if res.ok {
					estados[res.nombre] = "UP"
				} else {
					estados[res.nombre] = "DOWN"
				}
				pendientes--
			case <-ctx.Done():
				// Alguna comprobación ignoró el contexto: no se bloquea la sonda.
				expirado = true
				pendientes = 0
			}
		}

		todoOK := !expirado
		for _, estado := range estados {
			if estado != "UP" {
				todoOK = false
			}
		}

		if todoOK {
			responder(http.StatusOK, ReadyResponse{Status: "UP", Dependencies: estados})
			return
		}
		responder(http.StatusServiceUnavailable, ReadyResponse{Status: "DOWN", Dependencies: estados})
	}
}
