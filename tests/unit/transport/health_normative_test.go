package transport_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// Pruebas de A-090: rutas normativas de salud, readiness realista y estado
// durante el apagado. Las dependencias son funciones controladas: esto NO
// declara disponibilidad real de DynamoDB ni de Redis.

func decodificar(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	var cuerpo map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&cuerpo); err != nil {
		t.Fatalf("respuesta no es JSON válido: %v", err)
	}
	return cuerpo
}

func servidorConSalud(t *testing.T, hr *nexumhttp.HealthRegistry) (string, func()) {
	t.Helper()
	puerto := puertoLibre(t)
	cfg := nexumhttp.DefaultServerConfig()
	cfg.Port = puerto

	router := nexumhttp.NewRouter()
	router.RegisterHealth(hr)
	srv := nexumhttp.NewHTTPServer(cfg, router, loggerSilencioso())

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start() }()

	base := fmt.Sprintf("http://127.0.0.1:%d", puerto)
	cliente := clienteSinKeepAlive()
	for intento := 0; intento < 100; intento++ {
		if statusDe(t, cliente, base+"/api/v1/health/live") != 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	cerrar := func() {
		hr.BeginShutdown()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		select {
		case <-errCh:
		case <-time.After(5 * time.Second):
			t.Error("el servidor no terminó tras Shutdown")
		}
	}
	return base, cerrar
}

func TestRutasNormativasDeSalud(t *testing.T) {
	hr := nexumhttp.NewHealthRegistry(200 * time.Millisecond)
	hr.RegisterDependency("dynamodb", func(ctx context.Context) error { return nil })

	base, cerrar := servidorConSalud(t, hr)
	defer cerrar()
	cliente := clienteSinKeepAlive()

	if codigo := statusDe(t, cliente, base+"/api/v1/health/live"); codigo != http.StatusOK {
		t.Fatalf("live = %d, se esperaba 200", codigo)
	}
	if codigo := statusDe(t, cliente, base+"/api/v1/health/ready"); codigo != http.StatusOK {
		t.Fatalf("ready con dependencia sana = %d, se esperaba 200", codigo)
	}
	// Los probes heredados reflejan el mismo estado real.
	if codigo := statusDe(t, cliente, base+"/ready"); codigo != http.StatusOK {
		t.Fatalf("/ready heredado = %d, se esperaba 200", codigo)
	}
}

func TestLiveNoDependeDeServicios(t *testing.T) {
	hr := nexumhttp.NewHealthRegistry(50 * time.Millisecond)
	hr.RegisterDependency("dynamodb", func(ctx context.Context) error { return errors.New("caido") })

	base, cerrar := servidorConSalud(t, hr)
	defer cerrar()
	cliente := clienteSinKeepAlive()

	if codigo := statusDe(t, cliente, base+"/api/v1/health/live"); codigo != http.StatusOK {
		t.Fatalf("live con dependencia caída = %d; liveness no debe depender de servicios", codigo)
	}
	if codigo := statusDe(t, cliente, base+"/api/v1/health/ready"); codigo != http.StatusServiceUnavailable {
		t.Fatalf("ready con dependencia caída = %d, se esperaba 503", codigo)
	}
}

func TestReadySinDependenciasDevuelve503(t *testing.T) {
	hr := nexumhttp.NewHealthRegistry(50 * time.Millisecond)

	base, cerrar := servidorConSalud(t, hr)
	defer cerrar()
	cliente := clienteSinKeepAlive()

	if codigo := statusDe(t, cliente, base+"/api/v1/health/ready"); codigo != http.StatusServiceUnavailable {
		t.Fatalf("ready sin dependencias = %d; no se declara disponibilidad sin evidencia", codigo)
	}
}

func TestReadyConDependenciaLentaNoBloquea(t *testing.T) {
	hr := nexumhttp.NewHealthRegistry(100 * time.Millisecond)
	hr.RegisterDependency("redis", func(ctx context.Context) error {
		time.Sleep(2 * time.Second) // ignora el contexto a propósito
		return nil
	})

	base, cerrar := servidorConSalud(t, hr)
	defer cerrar()
	cliente := clienteSinKeepAlive()

	inicio := time.Now()
	codigo := statusDe(t, cliente, base+"/api/v1/health/ready")
	transcurrido := time.Since(inicio)

	if codigo != http.StatusServiceUnavailable {
		t.Fatalf("ready con dependencia lenta = %d, se esperaba 503", codigo)
	}
	if transcurrido > 900*time.Millisecond {
		t.Fatalf("ready tardó %v; debe respetar el timeout acotado", transcurrido)
	}
}

func TestReadyDuranteApagadoDevuelve503(t *testing.T) {
	hr := nexumhttp.NewHealthRegistry(100 * time.Millisecond)
	hr.RegisterDependency("dynamodb", func(ctx context.Context) error { return nil })

	base, cerrar := servidorConSalud(t, hr)
	defer cerrar()
	cliente := clienteSinKeepAlive()

	if codigo := statusDe(t, cliente, base+"/api/v1/health/ready"); codigo != http.StatusOK {
		t.Fatalf("ready inicial = %d, se esperaba 200", codigo)
	}

	hr.BeginShutdown()

	if codigo := statusDe(t, cliente, base+"/api/v1/health/ready"); codigo != http.StatusServiceUnavailable {
		t.Fatalf("ready en apagado = %d, se esperaba 503", codigo)
	}
}

func TestReadyNoFiltraDetallesInternos(t *testing.T) {
	hr := nexumhttp.NewHealthRegistry(100 * time.Millisecond)
	hr.RegisterDependency("dynamodb", func(ctx context.Context) error {
		return errors.New("credenciales internas filtradas: usuario=admin host=10.0.0.5")
	})

	base, cerrar := servidorConSalud(t, hr)
	defer cerrar()
	cliente := clienteSinKeepAlive()

	resp, err := cliente.Get(base + "/api/v1/health/ready")
	if err != nil {
		t.Fatalf("GET ready: %v", err)
	}
	cuerpo := decodificar(t, resp)
	texto, _ := json.Marshal(cuerpo)
	for _, prohibido := range []string{"credenciales", "admin", "10.0.0.5", "usuario="} {
		if strings.Contains(string(texto), prohibido) {
			t.Fatalf("la respuesta filtra detalle interno %q: %s", prohibido, texto)
		}
	}
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("ready con error = %d, se esperaba 503", resp.StatusCode)
	}
}

func TestReadyNoDependeDelMetodoDelCliente(t *testing.T) {
	hr := nexumhttp.NewHealthRegistry(50 * time.Millisecond)
	hr.RegisterDependency("dynamodb", func(ctx context.Context) error { return nil })

	base, cerrar := servidorConSalud(t, hr)
	defer cerrar()

	// POST sobre una ruta GET registrada debe responder 405 con Allow.
	cliente := clienteSinKeepAlive()
	req, err := http.NewRequest(http.MethodPost, base+"/api/v1/health/live", nil)
	if err != nil {
		t.Fatalf("construir POST: %v", err)
	}
	resp, err := cliente.Do(req)
	if err != nil {
		t.Fatalf("POST live: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST live = %d, se esperaba 405", resp.StatusCode)
	}
	if resp.Header.Get("Allow") == "" {
		t.Fatal("405 sin cabecera Allow")
	}
}

var _ = net.DialTimeout
