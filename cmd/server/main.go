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

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	cacheinfra "github.com/nexumpr/Nexum-Backend/internal/infrastructure/cache"
	healthinfra "github.com/nexumpr/Nexum-Backend/internal/infrastructure/health"
	"github.com/nexumpr/Nexum-Backend/internal/infrastructure/security"
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

	// A-090: registro de salud conectado al runtime real. No se registran
	// dependencias todavía (DynamoDB/Redis se integran en I-040/I-050); por
	// diseño readiness devuelve 503 hasta que exista una comprobación real.
	health := nexumhttp.NewHealthRegistry(2 * time.Second)
	router.RegisterHealth(health)

	// I-040/I-050: dependencias REALES. Si una no esta configurada no se registra y
	// readiness responde 503 (no se declara disponibilidad sin evidencia).
	cfgDeps := healthinfra.DesdeEntorno()
	// Ambas dependencias son OBLIGATORIAS: si falta configuracion o el cliente no se
	// puede construir, se registra una comprobacion que falla y readiness responde 503.
	clienteDynamo, errDynamo := healthinfra.ClienteDynamoDB(ctx, cfgDeps)
	if errDynamo != nil {
		logger.Error("dynamodb no inicializable; readiness devolvera 503", slog.String("error", errDynamo.Error()))
		causa := errDynamo
		health.RegisterDependency("dynamodb", func(context.Context) error { return causa })
	} else {
		tabla := cfgDeps.TablaDynamoDB
		health.RegisterDependency("dynamodb", func(ctx context.Context) error {
			return healthinfra.ComprobarDynamoDB(ctx, clienteDynamo, tabla)
		})
	}
	clienteRedis, errRedis := healthinfra.ClienteRedis(cfgDeps)
	if errRedis != nil {
		logger.Error("redis no inicializable; readiness devolvera 503", slog.String("error", errRedis.Error()))
		causa := errRedis
		health.RegisterDependency("redis", func(context.Context) error { return causa })
	} else {
		health.RegisterDependency("redis", func(ctx context.Context) error {
			return healthinfra.ComprobarRedis(ctx, clienteRedis)
		})
		defer func() { _ = clienteRedis.Close() }()
	}

	// H08-E / E001: registro real. La persistencia y el rate limit son dependencias
	// obligatorias; si faltan, el endpoint responde 503 (fail-closed). El almacen en
	// memoria solo existe en pruebas y nunca se compone aquí.
	var registrationStore *dynamoAdapter.RegistrationStore
	if clienteDynamo != nil && cfgDeps.TablaDynamoDB != "" {
		registrationStore = dynamoAdapter.NewRegistrationStore(clienteDynamo, cfgDeps.TablaDynamoDB)
	} else {
		registrationStore = dynamoAdapter.NewRegistrationStore(nil, "")
	}
	registerService := appIdentity.NewRegisterService(registrationStore, security.NewArgon2Hasher(nil))

	var authRateLimiter nexumhttp.RegisterRateLimiter
	if clienteRedis != nil {
		authRateLimiter = cacheinfra.NewAuthRateLimiter(clienteRedis, 5, 15*time.Minute)
	}
	registerHandler := nexumhttp.NewRegisterHandler(registerService, authRateLimiter, logger)
	router.HandleFunc(http.MethodPost, "/auth/register", registerHandler.ServeHTTP)

	// H08-E / E002 V6.0.2 (RFC-013): login con sesión persistida y JWT RS256.
	// Si falta la clave de firma o la persistencia, el endpoint responde 503 (fail-closed):
	// nunca se emiten tokens ficticios.
	sessionStore := dynamoAdapter.NewSessionStore(clienteDynamo, cfgDeps.TablaDynamoDB)

	var tokenIssuer appIdentity.AccessTokenIssuer
	proveedorTokens, errTokens := security.LoadTokenProvider()
	if errTokens != nil {
		logger.Error("login: clave de firma JWT no disponible; E002 devolvera 503",
			slog.String("error_class", "signing_key_unavailable"))
	} else {
		tokenIssuer = security.NewRSAAccessTokenIssuer(proveedorTokens)
	}

	// El limitador de login exige una clave HMAC propia (RFC-013). Sin ella el endpoint
	// responde 503 y E001 sigue funcionando: no se degrada a un hash sin secreto.
	claveHMAC, errHMAC := cacheinfra.LoadLoginHMACKey()
	if errHMAC != nil {
		logger.Error("login: clave HMAC del limitador no disponible; E002 devolvera 503",
			slog.String("error_class", "login_hmac_key_unavailable"))
	}
	var loginLimiter nexumhttp.LoginRateLimiter
	if clienteRedis != nil {
		loginLimiter = cacheinfra.NewLoginRateLimiter(clienteRedis, claveHMAC)
	}

	loginService := appIdentity.NewLoginService(sessionStore, security.NewArgon2Hasher(nil), tokenIssuer)
	loginHandler := nexumhttp.NewLoginHandler(loginService, loginLimiter, logger)
	router.HandleFunc(http.MethodPost, "/auth/login", loginHandler.ServeHTTP)

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

	// A-090: readiness pasa a 503 antes del drenado para retirar el destino.
	health.BeginShutdown()

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
