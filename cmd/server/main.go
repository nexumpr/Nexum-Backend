package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/nexumpr/Nexum-Backend/internal/system"
	"github.com/nexumpr/Nexum-Backend/pkg/telemetry"
	"github.com/nexumpr/Nexum-Backend/pkg/types"
)

func main() {
	logger := telemetry.NewJSONLogger(os.Stdout, slog.LevelInfo)
	ctx := context.Background()

	envStr := os.Getenv("APP_ENV")
	var meta types.SystemMetadata

	if envStr == "PRODUCTION" {
		meta = system.DefaultProductionMetadata()
	} else {
		meta = system.DefaultLocalMetadata()
	}

	sysCtx, err := system.NewSystemContext(meta)
	if err != nil {
		logger.ErrorContext(ctx, "Error crítico al inicializar contexto de sistema (A-000)", slog.String("error", err.Error()))
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
}
