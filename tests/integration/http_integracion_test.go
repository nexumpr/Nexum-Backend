package integration_test

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	infratelemetry "github.com/nexumpr/Nexum-Backend/internal/infrastructure/telemetry"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
	pkgtelemetry "github.com/nexumpr/Nexum-Backend/pkg/telemetry"
)

// cadenaDeIntegracion reproduce exactamente la composicion de NewHTTPServer
// (Trace -> RequestLog -> Recovery -> SecurityHeaders -> CORS) sobre el Router real.
// Se replica aqui porque HTTPServer no expone su handler; el job `integracion`
// ademas arranca el binario real y comprueba las mismas rutas por TCP, de modo que
// una divergencia en la composicion se detecta por el camino de produccion.
func cadenaDeIntegracion() (*nexumhttp.Router, http.Handler) {
	logger := pkgtelemetry.NewJSONLogger(io.Discard, slog.LevelError)
	metricas := infratelemetry.NewMetrics()

	router := nexumhttp.NewRouter()
	router.RegisterMetrics(metricas)

	health := nexumhttp.NewHealthRegistry(2 * time.Second)
	router.RegisterHealth(health)

	router.HandleFunc(http.MethodPost, "/api/v1/prueba", func(w http.ResponseWriter, r *http.Request) {
		var carga struct {
			Importe string `json:"importe"`
		}
		if err := nexumhttp.BindJSON(w, r, &carga); err != nil {
			codigo := http.StatusBadRequest
			if strings.Contains(err.Error(), "excede el limite") {
				codigo = http.StatusRequestEntityTooLarge
			}
			httputil.WriteError(w, codigo, "INVALID_BODY", err.Error())
			return
		}
		_ = httputil.WriteJSON(w, http.StatusOK, map[string]string{"estado": "ok"})
	})

	handler := middleware.Chain(
		router,
		middleware.TraceMiddleware(),
		middleware.RequestLogger(middleware.RequestLogOptions{
			Logger:        logger,
			Service:       "nexum-backend",
			Environment:   "integracion",
			InstanceID:    "integracion",
			Metrics:       metricas,
			RouteResolver: router.TemplateFor,
		}),
		middleware.Recovery(logger),
		middleware.SecurityHeaders(false),
		middleware.CORS(middleware.DefaultCORSOptions()),
	)
	return router, handler
}

func servidorDeIntegracion(t *testing.T) *httptest.Server {
	t.Helper()
	_, handler := cadenaDeIntegracion()
	servidor := httptest.NewServer(handler)
	servidor.Client().Timeout = 10 * time.Second
	t.Cleanup(servidor.Close)
	return servidor
}

func pedir(t *testing.T, cliente *http.Client, metodo, url string, cuerpo io.Reader, cabeceras map[string]string) *http.Response {
	t.Helper()
	peticion, err := http.NewRequest(metodo, url, cuerpo)
	if err != nil {
		t.Fatalf("no se pudo construir la peticion: %v", err)
	}
	for clave, valor := range cabeceras {
		peticion.Header.Set(clave, valor)
	}
	respuesta, err := cliente.Do(peticion)
	if err != nil {
		t.Fatalf("la peticion %s %s fallo: %v", metodo, url, err)
	}
	t.Cleanup(func() { _ = respuesta.Body.Close() })
	return respuesta
}

func TestIntegracionSondasYErroresEstructurales(t *testing.T) {
	servidor := servidorDeIntegracion(t)
	cliente := servidor.Client()

	casos := []struct {
		nombre   string
		metodo   string
		ruta     string
		codigo   int
		contiene string
	}{
		{"liveness", http.MethodGet, "/api/v1/health/live", http.StatusOK, "UP"},
		{"readiness sin dependencias", http.MethodGet, "/api/v1/health/ready", http.StatusServiceUnavailable, "DOWN"},
		{"ruta inexistente", http.MethodGet, "/api/v1/no-existe", http.StatusNotFound, "RESOURCE_NOT_FOUND"},
		{"metodo no permitido", http.MethodDelete, "/api/v1/health/live", http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED"},
	}

	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			respuesta := pedir(t, cliente, caso.metodo, servidor.URL+caso.ruta, nil, nil)
			if respuesta.StatusCode != caso.codigo {
				t.Fatalf("se esperaba %d y se obtuvo %d", caso.codigo, respuesta.StatusCode)
			}
			cuerpo, err := io.ReadAll(respuesta.Body)
			if err != nil {
				t.Fatalf("no se pudo leer el cuerpo: %v", err)
			}
			if !strings.Contains(string(cuerpo), caso.contiene) {
				t.Fatalf("el cuerpo no contiene %q: %s", caso.contiene, string(cuerpo))
			}
		})
	}

	respuesta := pedir(t, cliente, http.MethodDelete, servidor.URL+"/api/v1/health/live", nil, nil)
	if respuesta.Header.Get("Allow") == "" {
		t.Fatal("405 sin cabecera Allow")
	}
}

func TestIntegracionCabecerasDeSeguridadYTrazabilidad(t *testing.T) {
	servidor := servidorDeIntegracion(t)
	cliente := servidor.Client()

	respuesta := pedir(t, cliente, http.MethodGet, servidor.URL+"/api/v1/health/live", nil, map[string]string{
		"traceparent": "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
	})

	if respuesta.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("falta X-Content-Type-Options: nosniff")
	}
	if respuesta.Header.Get("X-Frame-Options") != "DENY" {
		t.Fatal("falta X-Frame-Options: DENY")
	}
	if respuesta.Header.Get("Cache-Control") == "" {
		t.Fatal("falta Cache-Control")
	}
	if respuesta.Header.Get(infratelemetry.RequestIDHeader) == "" {
		t.Fatalf("falta la cabecera de correlacion %s", infratelemetry.RequestIDHeader)
	}
	if respuesta.Header.Get(infratelemetry.TraceParentHeader) == "" {
		t.Fatalf("falta la cabecera %s en la respuesta", infratelemetry.TraceParentHeader)
	}
}

func TestIntegracionCORSConAllowlist(t *testing.T) {
	servidor := servidorDeIntegracion(t)
	cliente := servidor.Client()

	permitido := pedir(t, cliente, http.MethodOptions, servidor.URL+"/api/v1/prueba", nil, map[string]string{
		"Origin":                        "https://app.nexum.finance",
		"Access-Control-Request-Method": "POST",
	})
	if permitido.StatusCode != http.StatusNoContent {
		t.Fatalf("preflight permitido: se esperaba 204 y se obtuvo %d", permitido.StatusCode)
	}
	if permitido.Header.Get("Access-Control-Allow-Origin") != "https://app.nexum.finance" {
		t.Fatal("el origen permitido no se refleja en Access-Control-Allow-Origin")
	}

	rechazado := pedir(t, cliente, http.MethodOptions, servidor.URL+"/api/v1/prueba", nil, map[string]string{
		"Origin":                        "https://origen-no-autorizado.example",
		"Access-Control-Request-Method": "POST",
	})
	if rechazado.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("un origen fuera de la allowlist recibio Access-Control-Allow-Origin")
	}
}

func TestIntegracionCuerpoExcesivo(t *testing.T) {
	servidor := servidorDeIntegracion(t)
	cliente := servidor.Client()

	grande := strings.Repeat("a", 2*1024*1024)
	respuesta := pedir(t, cliente, http.MethodPost, servidor.URL+"/api/v1/prueba",
		strings.NewReader(`{"importe":"`+grande+`"}`),
		map[string]string{"Content-Type": "application/json"})

	if respuesta.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("se esperaba 413 y se obtuvo %d", respuesta.StatusCode)
	}
}

func TestIntegracionCampoDesconocidoRechazado(t *testing.T) {
	servidor := servidorDeIntegracion(t)
	cliente := servidor.Client()

	respuesta := pedir(t, cliente, http.MethodPost, servidor.URL+"/api/v1/prueba",
		strings.NewReader(`{"importe":"10.00","campo_extra":"x"}`),
		map[string]string{"Content-Type": "application/json"})

	if respuesta.StatusCode != http.StatusBadRequest {
		t.Fatalf("se esperaba 400 y se obtuvo %d", respuesta.StatusCode)
	}
}

func TestIntegracionReadyDegradadoTrasApagado(t *testing.T) {
	router, handler := cadenaDeIntegracion()
	servidor := httptest.NewServer(handler)
	t.Cleanup(servidor.Close)
	servidor.Client().Timeout = 10 * time.Second

	antes := pedir(t, servidor.Client(), http.MethodGet, servidor.URL+"/api/v1/health/ready", nil, nil)
	if antes.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("readiness sin dependencias: se esperaba 503 y se obtuvo %d", antes.StatusCode)
	}

	router.SetReady(false)
	despues := pedir(t, servidor.Client(), http.MethodGet, servidor.URL+"/ready", nil, nil)
	if despues.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("probe heredado en apagado: se esperaba 503 y se obtuvo %d", despues.StatusCode)
	}
}

func TestIntegracionConcurrencia(t *testing.T) {
	servidor := servidorDeIntegracion(t)
	cliente := servidor.Client()

	const (
		obreros     = 40
		iteraciones = 15
	)

	var espera sync.WaitGroup
	errores := make(chan string, obreros*iteraciones)

	for obrero := 0; obrero < obreros; obrero++ {
		espera.Add(1)
		go func(id int) {
			defer espera.Done()
			for iteracion := 0; iteracion < iteraciones; iteracion++ {
				ruta := "/api/v1/health/live"
				esperado := http.StatusOK
				if iteracion%2 == 1 {
					ruta = "/api/v1/no-existe"
					esperado = http.StatusNotFound
				}
				respuesta, err := cliente.Get(servidor.URL + ruta)
				if err != nil {
					errores <- fmt.Sprintf("obrero %d: %v", id, err)
					continue
				}
				_ = respuesta.Body.Close()
				if respuesta.StatusCode != esperado {
					errores <- fmt.Sprintf("obrero %d: %s devolvio %d, se esperaba %d", id, ruta, respuesta.StatusCode, esperado)
				}
				if respuesta.StatusCode >= 500 {
					errores <- fmt.Sprintf("obrero %d: 5xx inesperado en %s", id, ruta)
				}
			}
		}(obrero)
	}

	espera.Wait()
	close(errores)
	for mensaje := range errores {
		t.Error(mensaje)
	}
}
