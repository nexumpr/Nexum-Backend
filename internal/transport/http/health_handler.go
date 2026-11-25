package http

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
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

// HealthServiceName identifica el servicio en la respuesta contractual.
const HealthServiceName = "nexum-backend"

// HealthResponse es el esquema `Health` del contrato (E072/E073). Tiene
// `additionalProperties: false`: no admite detalles operativos ni estados de
// dependencias. El detalle vive únicamente en los logs del proceso.
type HealthResponse struct {
	Status    string `json:"status"`
	Service   string `json:"service"`
	Timestamp string `json:"timestamp"`
}

func healthResponse() HealthResponse {
	return HealthResponse{
		Status:    "ok",
		Service:   HealthServiceName,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
}

func writeHealthResponse(w http.ResponseWriter, status int) {
	w.Header().Set("Cache-Control", "no-store")
	_ = httputil.WriteJSON(w, status, healthResponse())
}

// LiveHandler responde 200 con el esquema `Health` mientras el proceso HTTP
// está en ejecución. No consulta DynamoDB, Redis ni ninguna dependencia
// (A-090) y no expone su estado.
func (hr *HealthRegistry) LiveHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeHealthResponse(w, http.StatusOK)
	}
}

// ReadyHandler verifica cada dependencia registrada con timeout acotado y
// responde con el esquema contractual:
//
//   - 200 `{"status":"ok","service":"nexum-backend","timestamp":...}`.
//   - 503 `ErrorEnvelope` con `code=DEPENDENCY_UNAVAILABLE` y `traceId` cuando
//     el nodo no puede atender.
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

		noDisponible := func() {
			w.Header().Set("Cache-Control", "no-store")
			_ = httputil.WriteErrorEnvelope(w, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "solicitud no completada", traceID(r), nil)
		}

		if apagando {
			noDisponible()
			return
		}

		if len(deps) == 0 {
			noDisponible()
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

		pendientes := len(deps)
		expirado := false
		fallidas := make([]string, 0, len(deps))
		for pendientes > 0 {
			select {
			case res := <-resultados:
				if !res.ok {
					fallidas = append(fallidas, res.nombre)
				}
				pendientes--
			case <-ctx.Done():
				// Alguna comprobación ignoró el contexto: no se bloquea la sonda.
				expirado = true
				pendientes = 0
			}
		}

		if expirado {
			slog.Default().WarnContext(r.Context(), "readiness agotó el tiempo de comprobación",
				slog.String("trace_id", traceID(r)))
			noDisponible()
			return
		}
		if len(fallidas) > 0 {
			// Sólo el nombre lógico de la dependencia: nunca el error del
			// proveedor, que puede contener endpoints o credenciales.
			slog.Default().WarnContext(r.Context(), "readiness con dependencias degradadas",
				slog.Any("dependencias", fallidas),
				slog.String("trace_id", traceID(r)))
			noDisponible()
			return
		}
		writeHealthResponse(w, http.StatusOK)
	}
}
