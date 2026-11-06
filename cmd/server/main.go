package main

import (
	"context"
	"errors"
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

	logger.InfoContext(ctx, "Servidor Nexum-Backend inicializado con éxito",
		slog.String("app_name", sysCtx.Metadata.AppName),
		slog.String("version", sysCtx.Metadata.Version),
		slog.String("spec_version", sysCtx.Metadata.SpecVersion),
		slog.String("environment", string(sysCtx.Metadata.Environment)),
		slog.Int("min_nodes", sysCtx.Metadata.MinNodeCount),
		slog.Bool("tls_required", sysCtx.Metadata.TLSRequired),
		slog.String("nosql_provider", sysCtx.Metadata.NoSQLProvider),
		slog.String("cache_provider", sysCtx.Metadata.CacheProvider),
	)

	router := nexumhttp.NewRouter()
	srv := nexumhttp.NewHTTPServer(nexumhttp.DefaultServerConfig(), router, logger)

	// Start bloquea hasta que el listener falla o se cierra; el canal transporta
	// el motivo para distinguir un fallo real de un cierre esperado.
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			logger.InfoContext(ctx, "Servidor HTTP cerrado sin error")
			return
		}
		logger.ErrorContext(ctx, "Fallo al iniciar el servidor HTTP", slog.String("error", err.Error()))
		os.Exit(1)
	case <-ctx.Done():
		logger.InfoContext(baseCtx, "Señal de terminación recibida; iniciando apagado controlado (A-030)")
	}

	shutdownCtx, cancel := context.WithTimeout(baseCtx, shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.ErrorContext(baseCtx, "Graceful shutdown falló", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// El listener debe haber terminado; se propaga cualquier error distinto del
	// cierre esperado para no ocultar fallos de escucha.
	if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.ErrorContext(baseCtx, "El servidor terminó con error", slog.String("error", err.Error()))
		os.Exit(1)
	}

	logger.InfoContext(baseCtx, "Apagado completado correctamente")
}
