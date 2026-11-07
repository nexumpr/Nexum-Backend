package transport_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// puertoLibre reserva y libera un puerto efímero para la prueba.
func puertoLibre(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("no se pudo reservar puerto: %v", err)
	}
	puerto := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		t.Fatalf("no se pudo liberar puerto: %v", err)
	}
	return puerto
}

func loggerSilencioso() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

// clienteSinKeepAlive evita conexiones ociosas que retrasarían Shutdown.
func clienteSinKeepAlive() *http.Client {
	return &http.Client{
		Timeout:   2 * time.Second,
		Transport: &http.Transport{DisableKeepAlives: true},
	}
}

func statusDe(t *testing.T, cliente *http.Client, url string) int {
	t.Helper()
	resp, err := cliente.Get(url)
	if err != nil {
		return 0
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	return resp.StatusCode
}

// esperarArranque sondea hasta que el listener responde o vence el plazo.
func esperarArranque(t *testing.T, cliente *http.Client, url string) int {
	t.Helper()
	for intento := 0; intento < 100; intento++ {
		if codigo := statusDe(t, cliente, url); codigo != 0 {
			return codigo
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("el servidor no respondió en %s", url)
	return 0
}

// TestRegresionD01_ServidorArrancaYAtiende reproduce el defecto D-01: antes el
// ejecutable terminaba sin abrir listener. Aquí se exige un socket real.
func TestRegresionD01_ServidorArrancaYAtiende(t *testing.T) {
	puerto := puertoLibre(t)
	cfg := nexumhttp.DefaultServerConfig()
	cfg.Port = puerto

	srv := nexumhttp.NewHTTPServer(cfg, nexumhttp.NewRouter(), loggerSilencioso())
	srv.Router().HandleFunc(http.MethodGet, "/ping", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"status":"ok"}`)); err != nil {
			t.Errorf("escritura de respuesta: %v", err)
		}
	})

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start() }()

	base := fmt.Sprintf("http://127.0.0.1:%d", puerto)
	cliente := clienteSinKeepAlive()

	if codigo := esperarArranque(t, cliente, base+"/health"); codigo != http.StatusOK {
		t.Fatalf("/health = %d, se esperaba 200", codigo)
	}
	if codigo := statusDe(t, cliente, base+"/api/v1/ping"); codigo != http.StatusOK {
		t.Fatalf("/api/v1/ping = %d, se esperaba 200", codigo)
	}
	if codigo := statusDe(t, cliente, base+"/api/v1/desconocida"); codigo != http.StatusNotFound {
		t.Fatalf("/api/v1/desconocida = %d, se esperaba 404", codigo)
	}

	req, err := http.NewRequest(http.MethodPost, base+"/api/v1/ping", nil)
	if err != nil {
		t.Fatalf("construir POST: %v", err)
	}
	respPost, err := cliente.Do(req)
	if err != nil {
		t.Fatalf("POST /api/v1/ping: %v", err)
	}
	_, _ = io.Copy(io.Discard, respPost.Body)
	_ = respPost.Body.Close()
	if respPost.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST /api/v1/ping = %d, se esperaba 405", respPost.StatusCode)
	}

	// Apagado controlado: debe terminar y liberar el puerto.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("Start devolvió error inesperado: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("el servidor no terminó tras Shutdown")
	}

	if conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", puerto), 200*time.Millisecond); err == nil {
		_ = conn.Close()
		t.Fatal("el puerto sigue escuchando después del shutdown")
	}
}

// TestPuertoOcupadoFallaExplicitamente exige un error real, no un éxito falso.
// Se ocupa 0.0.0.0:puerto (no 127.0.0.1) porque en Windows ambas direcciones no
// colisionan y el servidor podría arrancar y bloquear la prueba.
func TestPuertoOcupadoFallaExplicitamente(t *testing.T) {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("no se pudo ocupar el puerto: %v", err)
	}
	defer func() { _ = l.Close() }()

	puerto := l.Addr().(*net.TCPAddr).Port
	cfg := nexumhttp.DefaultServerConfig()
	cfg.Port = puerto

	srv := nexumhttp.NewHTTPServer(cfg, nexumhttp.NewRouter(), loggerSilencioso())

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start() }()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("Start debía fallar con el puerto ocupado y devolvió nil")
		}
	case <-time.After(3 * time.Second):
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = srv.Shutdown(ctx)
		cancel()
		t.Fatal("Start no detectó el puerto ocupado en 3 s")
	}
}
