package http

import (
	"net/http"
	"strings"
	"sync"

	"github.com/nexumpr/Nexum-Backend/internal/infrastructure/telemetry"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// Router gestiona el enrutamiento versionado de la API (/api/v1) y códigos de estado REST (A-030 §2.1, AC-A-030-01).
type Router struct {
	mu        sync.RWMutex
	routes    map[string]map[string]http.Handler // [path][method]handler
	isReady   bool
	probeMux  *http.ServeMux
	apiPrefix string
	health    *HealthRegistry
	metrics   *telemetry.Metrics
}

// NewRouter crea un enrutador con soporte estricto de métodos, 404 no enumerable y 405 Method Not Allowed.
func NewRouter() *Router {
	r := &Router{
		routes:    make(map[string]map[string]http.Handler),
		isReady:   true,
		probeMux:  http.NewServeMux(),
		apiPrefix: "/api/v1",
	}

	// Probes de salud (Liveness y Readiness exigidos en A-030 §2.7, GOV-03)
	r.probeMux.HandleFunc("/health", func(w http.ResponseWriter, req *http.Request) {
		_ = httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "UP"})
	})
	r.probeMux.HandleFunc("/ready", func(w http.ResponseWriter, req *http.Request) {
		r.mu.RLock()
		ready := r.isReady
		r.mu.RUnlock()

		if ready {
			_ = httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "READY"})
		} else {
			httputil.WriteError(w, http.StatusServiceUnavailable, "DRAINING", "servidor en proceso de apagado suave")
		}
	})

	return r
}

// RegisterHealth conecta el HealthRegistry de A-090 al router: expone las rutas
// normativas /api/v1/health/live y /api/v1/health/ready y hace que los probes
// heredados /health y /ready reflejen el mismo estado real.
func (rt *Router) RegisterHealth(hr *HealthRegistry) {
	rt.mu.Lock()
	rt.health = hr
	rt.mu.Unlock()

	rt.HandleFunc(http.MethodGet, "/health/live", hr.LiveHandler())
	rt.HandleFunc(http.MethodGet, "/health/ready", hr.ReadyHandler())
}

// RegisterMetrics conecta el registro de métricas y expone /metrics (sólo loopback).
func (rt *Router) RegisterMetrics(m *telemetry.Metrics) {
	rt.mu.Lock()
	rt.metrics = m
	rt.mu.Unlock()
}

// healthRegistry devuelve el registro conectado, si existe.
func (rt *Router) healthRegistry() *HealthRegistry {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return rt.health
}

// SetReady permite cambiar el estado del probe heredado de readiness cuando no
// hay HealthRegistry conectado (usado en graceful shutdown).
func (rt *Router) SetReady(ready bool) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.isReady = ready
}

// HandleFunc registra un handler para un método y ruta específicos bajo /api/v1.
func (rt *Router) HandleFunc(method, pattern string, handler http.HandlerFunc) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	cleanPattern := pattern
	if !strings.HasPrefix(cleanPattern, rt.apiPrefix) {
		cleanPattern = rt.apiPrefix + cleanPattern
	}

	if _, ok := rt.routes[cleanPattern]; !ok {
		rt.routes[cleanPattern] = make(map[string]http.Handler)
	}
	rt.routes[cleanPattern][strings.ToUpper(method)] = handler
}

// ServeHTTP implementa http.Handler resolviendo rutas, 405 y 404.
func (rt *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// 1. Resolver probes heredados del balanceador (/health, /ready).
	// Con HealthRegistry conectado reflejan el estado real de A-090; sin él se
	// conserva el comportamiento previo de A-030.
	if path == "/metrics" {
		rt.mu.RLock()
		m := rt.metrics
		rt.mu.RUnlock()
		if m == nil {
			httputil.WriteError(w, http.StatusNotFound, "RESOURCE_NOT_FOUND", "el recurso solicitado no existe")
			return
		}
		m.Handler().ServeHTTP(w, r)
		return
	}
	if path == "/health" || path == "/ready" {
		if hr := rt.healthRegistry(); hr != nil {
			if path == "/health" {
				hr.LiveHandler().ServeHTTP(w, r)
			} else {
				hr.ReadyHandler().ServeHTTP(w, r)
			}
			return
		}
		rt.probeMux.ServeHTTP(w, r)
		return
	}

	rt.mu.RLock()
	methodsMap, pathExists := rt.routes[path]
	rt.mu.RUnlock()

	// 2. Si la ruta existe, validar el método solicitado (AC-A-030-01)
	if pathExists {
		handler, methodAllowed := methodsMap[r.Method]
		if methodAllowed {
			handler.ServeHTTP(w, r)
			return
		}

		// Método no permitido: emitir 405 y cabecera Allow con métodos soportados
		var allowedMethods []string
		for m := range methodsMap {
			allowedMethods = append(allowedMethods, m)
		}
		w.Header().Set("Allow", strings.Join(allowedMethods, ", "))
		httputil.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "el metodo HTTP no esta permitido para este recurso")
		return
	}

	// 3. Ruta no encontrada: emitir 404 estructurado no enumerable
	httputil.WriteError(w, http.StatusNotFound, "RESOURCE_NOT_FOUND", "el recurso solicitado no existe")
}
