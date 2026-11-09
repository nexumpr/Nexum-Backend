package telemetry

import (
	"fmt"
	"net/http"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

type metricKey struct {
	Method string
	Route  string
	Status int
}

// Metrics mantiene contadores y latencias acotadas por ruta plantilla y estado.
// No usa identificadores de usuario ni rutas crudas (cardinalidad limitada).
const maxSeries = 500

type Metrics struct {
	mu        sync.Mutex
	requests  map[metricKey]uint64
	durations map[metricKey]float64
	dropped   uint64
}

// NewMetrics crea un registro de métricas vacío.
func NewMetrics() *Metrics {
	return &Metrics{
		requests:  make(map[metricKey]uint64),
		durations: make(map[metricKey]float64),
	}
}

// Observe registra una petición con su ruta plantilla y duración.
func (m *Metrics) Observe(method, route string, status int, latency time.Duration) {
	if m == nil {
		return
	}
	key := metricKey{Method: strings.ToUpper(method), Route: route, Status: status}
	m.mu.Lock()
	if _, existe := m.requests[key]; !existe && len(m.requests) >= maxSeries {
		m.dropped++
		m.mu.Unlock()
		return
	}
	m.requests[key]++
	m.durations[key] += latency.Seconds()
	m.mu.Unlock()
}

// Handler expone métricas en formato Prometheus. Sólo responde a loopback (agente local).
func (m *Metrics) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !loopback(r.RemoteAddr) {
			http.Error(w, "no autorizado", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		var lineas []string
		m.mu.Lock()
		claves := make([]metricKey, 0, len(m.requests))
		for key := range m.requests {
			claves = append(claves, key)
		}
		sort.Slice(claves, func(i, j int) bool {
			if claves[i].Route != claves[j].Route {
				return claves[i].Route < claves[j].Route
			}
			if claves[i].Method != claves[j].Method {
				return claves[i].Method < claves[j].Method
			}
			return claves[i].Status < claves[j].Status
		})
		lineas = append(lineas, "# TYPE nexum_http_requests_total counter")
		for _, key := range claves {
			lineas = append(lineas, fmt.Sprintf(
				`nexum_http_requests_total{method=%q,route=%q,status="%d"} %d`,
				key.Method, key.Route, key.Status, m.requests[key],
			))
		}
		lineas = append(lineas, "# TYPE nexum_http_request_duration_seconds summary")
		for _, key := range claves {
			etiquetas := fmt.Sprintf(`method=%q,route=%q,status="%d"`, key.Method, key.Route, key.Status)
			lineas = append(lineas, fmt.Sprintf("nexum_http_request_duration_seconds_sum{%s} %f", etiquetas, m.durations[key]))
			lineas = append(lineas, fmt.Sprintf("nexum_http_request_duration_seconds_count{%s} %d", etiquetas, m.requests[key]))
		}
		m.mu.Unlock()
		lineas = append(lineas,
			"# TYPE nexum_metrics_series gauge",
			fmt.Sprintf("nexum_metrics_series %d", len(claves)),
			"# TYPE nexum_metrics_series_dropped_total counter",
			fmt.Sprintf("nexum_metrics_series_dropped_total %d", m.dropped),
			"# TYPE nexum_go_goroutines gauge",
			fmt.Sprintf("nexum_go_goroutines %d", runtime.NumGoroutine()),
			"# TYPE nexum_go_mem_alloc_bytes gauge",
			fmt.Sprintf("nexum_go_mem_alloc_bytes %d", memAllocBytes()),
		)
		_, _ = w.Write([]byte(strings.Join(lineas, "\n") + "\n"))
	})
}

func memAllocBytes() uint64 {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return stats.Alloc
}

func loopback(remote string) bool {
	host := remote
	if indice := strings.LastIndex(remote, ":"); indice > 0 {
		host = remote[:indice]
	}
	host = strings.Trim(host, "[]")
	return host == "127.0.0.1" || host == "::1" || host == "localhost"
}
