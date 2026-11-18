package http

import (
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/nexumpr/Nexum-Backend/internal/infrastructure/telemetry"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// Router gestiona el enrutamiento versionado de la API (/api/v1) y códigos de estado REST (A-030 §2.1, AC-A-030-01).
//
// Soporta dos clases de ruta, con prioridad absoluta de la literal:
//   - literal: coincidencia exacta de `r.URL.Path` (comportamiento heredado de A-030);
//   - plantilla: segmentos con `{nombre}`, donde cada parámetro casa exactamente UN segmento
//     no vacío. El valor decodificado se expone al handler con `r.SetPathValue`.
//
// Una ruta literal siempre gana a una plantilla, de modo que registrar
// `/api/v1/auth/sessions/{sessionId}` no altera ninguna ruta existente.
type Router struct {
	mu        sync.RWMutex
	routes    map[string]map[string]http.Handler // [path][method]handler
	templates []*routeTemplate                   // plantillas con parámetros, en orden de registro
	isReady   bool
	probeMux  *http.ServeMux
	apiPrefix string
	health    *HealthRegistry
	metrics   *telemetry.Metrics
}

// routeTemplate es una ruta registrada con al menos un parámetro `{nombre}`.
type routeTemplate struct {
	template string
	segments []routeSegment
	methods  map[string]http.Handler
}

// routeSegment es un segmento de plantilla: o literal exacto, o parámetro nombrado.
type routeSegment struct {
	literal string
	param   string
}

// match comprueba si la ruta concreta casa con la plantilla y devuelve los parámetros
// extraídos. Un parámetro nunca cruza `/` ni acepta el segmento vacío.
func (t *routeTemplate) match(path string) (map[string]string, bool) {
	segmentos := splitPath(path)
	if len(segmentos) != len(t.segments) {
		return nil, false
	}
	valores := make(map[string]string, len(t.segments))
	for i, segmento := range t.segments {
		actual := segmentos[i]
		if segmento.param == "" {
			if actual != segmento.literal {
				return nil, false
			}
			continue
		}
		if actual == "" {
			return nil, false
		}
		decodificado, err := url.PathUnescape(actual)
		if err != nil || decodificado == "" || strings.Contains(decodificado, "/") {
			// Un segmento manipulado (`%2F`) no se acepta como valor de parámetro: se evita
			// que un identificador con barra codificada se interprete como dos segmentos.
			return nil, false
		}
		valores[segmento.param] = decodificado
	}
	return valores, true
}

// splitPath divide una ruta absoluta en segmentos no vacíos, ignorando la barra inicial.
func splitPath(path string) []string {
	limpio := strings.TrimPrefix(path, "/")
	if limpio == "" {
		return nil
	}
	return strings.Split(limpio, "/")
}

// parseTemplate convierte un patrón con `{param}` en segmentos. Devuelve false si el patrón
// no es válido (paréntesis sin cerrar, parámetro vacío o parámetro con `/`).
func parseTemplate(pattern string) ([]routeSegment, bool) {
	segmentos := splitPath(pattern)
	if len(segmentos) == 0 {
		return nil, false
	}
	resultado := make([]routeSegment, 0, len(segmentos))
	for _, segmento := range segmentos {
		if !strings.Contains(segmento, "{") {
			if strings.Contains(segmento, "}") {
				return nil, false
			}
			resultado = append(resultado, routeSegment{literal: segmento})
			continue
		}
		// Sólo se admite la forma completa `{nombre}`: un prefijo literal dentro del mismo
		// segmento haría ambiguo el reparto y no lo necesita ninguna ruta del contrato.
		if len(segmento) < 3 || segmento[0] != '{' || segmento[len(segmento)-1] != '}' {
			return nil, false
		}
		nombre := segmento[1 : len(segmento)-1]
		if nombre == "" || strings.ContainsAny(nombre, "/{}") {
			return nil, false
		}
		resultado = append(resultado, routeSegment{param: nombre})
	}
	return resultado, true
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
// TemplateFor devuelve la plantilla registrada para una ruta conocida o /unknown.
// Las rutas no registradas nunca devuelven segmentos arbitrarios (PII/cardinalidad).
func (rt *Router) TemplateFor(path string) string {
	switch path {
	case "/health", "/ready", "/metrics":
		return path
	}
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	if _, existe := rt.routes[path]; existe {
		return path
	}
	// Las rutas con parámetros devuelven SIEMPRE la plantilla registrada, nunca el valor
	// concreto: la etiqueta de métrica no puede convertirse en un identificador de sesión.
	for _, plantilla := range rt.templates {
		if _, ok := plantilla.match(path); ok {
			return plantilla.template
		}
	}
	return "/unknown"
}

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
//
// Si el patrón contiene `{param}`, se registra como plantilla parametrizada. Una ruta literal
// registrada con el mismo texto siempre tiene prioridad en la resolución.
func (rt *Router) HandleFunc(method, pattern string, handler http.HandlerFunc) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	cleanPattern := pattern
	if !strings.HasPrefix(cleanPattern, rt.apiPrefix) {
		cleanPattern = rt.apiPrefix + cleanPattern
	}

	if !strings.Contains(cleanPattern, "{") {
		if _, ok := rt.routes[cleanPattern]; !ok {
			rt.routes[cleanPattern] = make(map[string]http.Handler)
		}
		rt.routes[cleanPattern][strings.ToUpper(method)] = handler
		return
	}

	segmentos, ok := parseTemplate(cleanPattern)
	if !ok {
		// Patrón inválido: se ignora el registro en lugar de crear una ruta ambigua. La ruta
		// responderá 404, que es el fallo cerrado correcto.
		return
	}
	for _, plantilla := range rt.templates {
		if plantilla.template == cleanPattern {
			plantilla.methods[strings.ToUpper(method)] = handler
			return
		}
	}
	rt.templates = append(rt.templates, &routeTemplate{
		template: cleanPattern,
		segments: segmentos,
		methods:  map[string]http.Handler{strings.ToUpper(method): handler},
	})
}

// matchTemplate busca la primera plantilla registrada que case con la ruta. Devuelve los
// métodos disponibles y los parámetros extraídos.
func (rt *Router) matchTemplate(path string) (map[string]http.Handler, map[string]string) {
	for _, plantilla := range rt.templates {
		if valores, ok := plantilla.match(path); ok {
			return plantilla.methods, valores
		}
	}
	return nil, nil
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
	var (
		templateMethods map[string]http.Handler
		templateValues  map[string]string
	)
	if !pathExists {
		// Prioridad absoluta de la ruta literal: la plantilla sólo se evalúa si no existe
		// una ruta literal con ese texto exacto.
		templateMethods, templateValues = rt.matchTemplate(path)
	}
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

	// 2 bis. Ruta parametrizada: mismo tratamiento de 405 y `Allow`, y los valores del
	// path se exponen al handler mediante `SetPathValue` (nunca por query ni por cabecera).
	if templateMethods != nil {
		handler, methodAllowed := templateMethods[r.Method]
		if !methodAllowed {
			var allowedMethods []string
			for m := range templateMethods {
				allowedMethods = append(allowedMethods, m)
			}
			w.Header().Set("Allow", strings.Join(allowedMethods, ", "))
			httputil.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "el metodo HTTP no esta permitido para este recurso")
			return
		}
		for nombre, valor := range templateValues {
			r.SetPathValue(nombre, valor)
		}
		handler.ServeHTTP(w, r)
		return
	}

	// 3. Ruta no encontrada: emitir 404 estructurado no enumerable
	httputil.WriteError(w, http.StatusNotFound, "RESOURCE_NOT_FOUND", "el recurso solicitado no existe")
}
