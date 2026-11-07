package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// Las pruebas viven en el paquete main para ejercitar `run`, el mismo camino de
// composición que usa `main()`. Si alguien desconecta Router/HTTPServer del
// punto de entrada, estas pruebas fallan (defecto D-01, observación 1).

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

func clienteSinKeepAlive() *http.Client {
	return &http.Client{
		Timeout:   2 * time.Second,
		Transport: &http.Transport{DisableKeepAlives: true},
	}
}

func statusDe(cliente *http.Client, url string) int {
	resp, err := cliente.Get(url)
	if err != nil {
		return 0
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	return resp.StatusCode
}

// TestRunArrancaListenerYApaga ejerce la composición real: arranque, HTTP y
// apagado por cancelación de contexto.
func TestRunArrancaListenerYApaga(t *testing.T) {
	puerto := puertoLibre(t)
	cfg := nexumhttp.DefaultServerConfig()
	cfg.Port = puerto

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- run(ctx, cfg, logger) }()

	cliente := clienteSinKeepAlive()
	base := fmt.Sprintf("http://127.0.0.1:%d", puerto)

	var ultimo int
	for intento := 0; intento < 100; intento++ {
		if ultimo = statusDe(cliente, base+"/health"); ultimo != 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if ultimo != http.StatusOK {
		t.Fatalf("/health = %d, se esperaba 200 (listener no arrancó por run)", ultimo)
	}
	if codigo := statusDe(cliente, base+"/api/v1/desconocida"); codigo != http.StatusNotFound {
		t.Fatalf("/api/v1/desconocida = %d, se esperaba 404", codigo)
	}

	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("run devolvió error tras cancelar: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("run no terminó tras cancelar el contexto")
	}

	if conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", puerto), 200*time.Millisecond); err == nil {
		_ = conn.Close()
		t.Fatal("el puerto sigue escuchando después del shutdown")
	}

	salida := buf.String()
	if strings.Contains(strings.ToLower(salida), "éxito") || strings.Contains(strings.ToLower(salida), "exito") {
		t.Fatalf("el log de arranque afirma éxito antes de escuchar: %s", salida)
	}
	if !strings.Contains(salida, "A-030") {
		t.Fatalf("el log de arranque no identifica la spec A-030: %s", salida)
	}
}

// TestRunFallaConPuertoOcupado exige que la composición real propague el error
// de escucha en vez de declarar éxito.
func TestRunFallaConPuertoOcupado(t *testing.T) {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("no se pudo ocupar el puerto: %v", err)
	}
	defer func() { _ = l.Close() }()

	cfg := nexumhttp.DefaultServerConfig()
	cfg.Port = l.Addr().(*net.TCPAddr).Port

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	errCh := make(chan error, 1)
	go func() { errCh <- run(context.Background(), cfg, logger) }()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("run debía devolver error con el puerto ocupado")
		}
		if !strings.Contains(err.Error(), "servidor HTTP") {
			t.Fatalf("error poco informativo: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run no detectó el puerto ocupado en 5 s")
	}

	// El logger no debe contener ningún JSON con campos sensibles.
	for _, prohibido := range []string{"AKIA", "BEGIN PRIVATE", "password", "token"} {
		if strings.Contains(buf.String(), prohibido) {
			t.Fatalf("log con dato sensible: %s", prohibido)
		}
	}
	_ = json.Valid(buf.Bytes()) // el handler JSON debe emitir líneas válidas
}
