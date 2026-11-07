package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nexumpr/Nexum-Backend/internal/system"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
	"github.com/nexumpr/Nexum-Backend/pkg/telemetry"
	"github.com/nexumpr/Nexum-Backend/pkg/types"
)

// shutdownTimeout es el tiempo máximo de drenado al recibir SIGINT/SIGTERM (A-030 §2.7).
const shutdownTimeout = 15 * time.Second

// run compone y ejecuta el runtime HTTP hasta que el contexto se cancela o el
// listener falla. Es la función que main() invoca, y existe para que la prueba
// de regresión ejercite el mismo camino que producción (defecto D-01).
func run(ctx context.Context, cfg nexumhttp.ServerConfig, logger *slog.Logger) error {
	router := nexumhttp.NewRouter()
	srv := nexumhttp.NewHTTPServer(cfg, router, logger)

	// Start bloquea hasta que el listener falla o se cierra; el canal transporta
	// el motivo para distinguir un fallo real de un cierre esperado.
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("fallo al iniciar el servidor HTTP: %w", err)
	case <-ctx.Done():
		logger.InfoContext(context.Background(), "Señal de terminación recibida; iniciando apagado controlado (A-030)")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}

	// El listener debe haber terminado; se propaga cualquier error distinto del
	// cierre esperado para no ocultar fallos de escucha.
	if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("el servidor terminó con error: %w", err)
	}

	return nil
}

func main() {
	logger := telemetry.NewJSONLogger(os.Stdout, slog.LevelInfo)
	baseCtx := context.Background()

	// Señales de terminación: Ctrl+C en local y SIGTERM en Linux (systemd/ASG).
	ctx, stop := signal.NotifyContext(baseCtx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	envStr := os.Getenv("APP_ENV")
	var meta types.SystemMetadata

	if envStr == "PRODUCTION" {
		meta = system.DefaultProductionMetadata()
	} else {
		meta = system.DefaultLocalMetadata()
	}

	sysCtx, err := system.NewSystemContext(meta)
	if err != nil {
		logger.ErrorContext(baseCtx, "Error crítico al inicializar contexto de sistema (A-000)", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// Mensaje deliberadamente sin afirmar disponibilidad: el listener todavía no
	// existe. El éxito de escucha sólo se refleja en el log de apagado o en las
	// sondas; un fallo posterior se registra como error (defecto D-01 observación 2).
	logger.InfoContext(ctx, "Contexto de sistema inicializado (A-000); se inicia el listener HTTP (A-030)",
		slog.String("app_name", sysCtx.Metadata.AppName),
		slog.String("version", sysCtx.Metadata.Version),
		slog.String("spec_version", sysCtx.Metadata.SpecVersion),
		slog.String("environment", string(sysCtx.Metadata.Environment)),
		slog.Int("min_nodes", sysCtx.Metadata.MinNodeCount),
		slog.Bool("tls_required", sysCtx.Metadata.TLSRequired),
		slog.String("nosql_provider", sysCtx.Metadata.NoSQLProvider),
		slog.String("cache_provider", sysCtx.Metadata.CacheProvider),
	)

	if err := run(ctx, nexumhttp.DefaultServerConfig(), logger); err != nil {
		logger.ErrorContext(baseCtx, "El servidor HTTP terminó con error", slog.String("error", err.Error()))
		os.Exit(1)
	}

	logger.InfoContext(baseCtx, "Apagado completado correctamente")
}
