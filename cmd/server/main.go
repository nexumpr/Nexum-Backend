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
	_ "time/tzdata"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	cacheinfra "github.com/nexumpr/Nexum-Backend/internal/infrastructure/cache"
	healthinfra "github.com/nexumpr/Nexum-Backend/internal/infrastructure/health"
	"github.com/nexumpr/Nexum-Backend/internal/infrastructure/security"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
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

	// H08-E / E003: rotacion de refresh token sobre las sesiones REALES de E002.
	// Reutiliza el mismo proveedor RS256 y la misma clave HMAC de cuotas; no crea claves,
	// sesiones paralelas ni almacenamiento en memoria. Sin persistencia o sin firma, el
	// endpoint responde 503 (fail-closed).
	refreshStore := dynamoAdapter.NewRefreshStore(clienteDynamo, cfgDeps.TablaDynamoDB)
	refreshService := appIdentity.NewRefreshService(refreshStore, tokenIssuer)
	var refreshLimiter nexumhttp.RefreshRateLimiter
	if clienteRedis != nil {
		refreshLimiter = cacheinfra.NewRefreshRateLimiter(clienteRedis, claveHMAC)
	}
	refreshHandler := nexumhttp.NewRefreshHandler(refreshService, refreshLimiter, logger)
	router.HandleFunc(http.MethodPost, "/auth/refresh", refreshHandler.ServeHTTP)

	// H08-E / E004: cierre de sesion real sobre las sesiones de E002/E003. La ruta exige JWT
	// RS256 y estado autoritativo de la sesion en DynamoDB (lectura consistente): un token de
	// una sesion revocada deja de autorizar en el acto, en cualquier instancia.
	// Se pasa un interface nil cuando falta el cliente para fallar cerrado con 503 en lugar de
	// operar sobre un puntero nil.
	var dynamoAPI dynamoAdapter.DynamoAPI
	if clienteDynamo != nil {
		dynamoAPI = clienteDynamo
	}
	logoutStore := dynamoAdapter.NewLogoutStore(dynamoAPI, cfgDeps.TablaDynamoDB)
	logoutService := appIdentity.NewLogoutService(logoutStore)
	var verificadorTokens ports.TokenProvider
	if errTokens == nil && proveedorTokens != nil {
		verificadorTokens = proveedorTokens
	}
	logoutHandler := nexumhttp.NewLogoutHandler(logoutService, verificadorTokens, logoutStore, logger)
	router.HandleFunc(http.MethodPost, "/auth/logout", logoutHandler.ServeHTTP)

	// H08-E / E005: recuperacion de contrasena. SES real, persistentcia DynamoDB,
	// cuotas y liderazgo Redis y worker durable. Fail-closed: si falta A9 (clave de
	// sellado), remitente/base publica o Redis, el endpoint responde 503.
	_, cerrarE005 := montarE005(ctx, router, clienteDynamo, clienteRedis, cfgDeps.TablaDynamoDB, logger)
	defer cerrarE005()

	// H08-E / E006: confirmacion de restablecimiento con token de un solo uso y
	// revocacion por epoca (A6). Reutiliza la tabla, el hasher Argon2id, el material JWT
	// y Redis ya existentes; sin IAM ni recursos nuevos.
	montarE006(router, clienteDynamo, clienteRedis, cfgDeps.TablaDynamoDB, logger)
	// E011: montaje de la oleada E007-E011 (RFC-019).
	montarE011(router, clienteDynamo, clienteRedis, cfgDeps.TablaDynamoDB, logger)
	// E010: montaje de la oleada E007-E011 (RFC-019).
	montarE010(router, clienteDynamo, clienteRedis, cfgDeps.TablaDynamoDB, logger)
	// E010: montaje de la oleada E007-E011 (RFC-019).
	// E009: montaje de la oleada E007-E011 (RFC-019).
	montarE009(router, clienteDynamo, clienteRedis, verificadorTokens, logoutStore, cfgDeps.TablaDynamoDB, logger)
	// E008: montaje de la oleada E007-E011 (RFC-019).
	montarE008(router, clienteDynamo, clienteRedis, cfgDeps.TablaDynamoDB, logger)
	// E007: montaje de la oleada E007-E011 (RFC-019).
	montarE007(router, clienteDynamo, clienteRedis, cfgDeps.TablaDynamoDB, logger)

	// H08-E / Oleada 1 (RFC-019 E-1): registro determinista de E012..E071.
	// Sin endpoints registrados no añade ninguna ruta. El andamiaje es válido con
	// cero endpoints y no contiene handlers ficticios.
	montarOleada1(&dependenciasOleada{
		Router: router,
		Dynamo: clienteDynamo,
		Redis:  clienteRedis,
		Tabla:  cfgDeps.TablaDynamoDB,
		Logger: logger,
	})

	// H08-E / Oleada 2 (RFC-019 E-3, Gate H08E_OLEADA_2 aprobado): 15 endpoints.
	// El registro es cerrado y falla en el arranque si un ID no esta autorizado.
	depsOleada2 := &dependenciasOleada2{
		Router:     router,
		Dynamo:     dynamoAPI,
		Redis:      clienteRedis,
		Tabla:      cfgDeps.TablaDynamoDB,
		Logger:     logger,
		Tokens:     verificadorTokens,
		Authorizer: logoutStore,
	}

	// H08-E / E014: el worker de purga es requisito para aceptar solicitudes. Se arranca
	// ANTES de montar las rutas: si no puede iniciar, E014 responde 503 en vez de emitir
	// un 202 que nadie ejecutaria. El resto de endpoints no se ve afectado.
	cerrarPurga, purgaActiva, errPurga := iniciarPurgaE014(ctx, depsOleada2)
	if errPurga != nil {
		logger.ErrorContext(ctx, "E014: worker de purga no iniciado; el endpoint respondera 503",
			slog.String("error_class", "purga_no_iniciada"))
	} else {
		depsOleada2.PurgaLista = true
		depsOleada2.PurgaActiva = purgaActiva
		defer cerrarPurga()
	}

	montarOleada2(depsOleada2)

	// Oleada 3: el registro cerrado valida IDs, métodos y rutas antes de montar. Los
	// endpoints de esta oleada comparten las mismas dependencias reales.
	depsOleada3 := &dependenciasOleada3{
		Router:     router,
		Dynamo:     dynamoAPI,
		Redis:      clienteRedis,
		Tabla:      cfgDeps.TablaDynamoDB,
		Logger:     logger,
		Tokens:     verificadorTokens,
		Authorizer: logoutStore,
	}
	montarOleada3(depsOleada3)

	// Oleada 4: registro cerrado E052–E063. Hereda las mismas dependencias reales y
	// sólo monta los E-ID con implementación presente en esta rama.
	depsOleada4 := &dependenciasOleada4{
		Router:     router,
		Dynamo:     dynamoAPI,
		Redis:      clienteRedis,
		Tabla:      cfgDeps.TablaDynamoDB,
		Logger:     logger,
		Tokens:     verificadorTokens,
		Authorizer: logoutStore,
		PushSealer: pushSealerOleada4(),
		CursorKey:  claveCursorDesdeEntorno(),
	}
	depsOleada4.Preferencias = preferenciasWorkerOleada4(depsOleada4)
	montarOleada4(depsOleada4)

	// Oleada 5: registro cerrado E021/E064-E067/E074/E075. E072/E073 ya están
	// montados por RegisterHealth y no se duplican aquí. Sólo se registran los
	// E-ID con implementación presente; sin dependencias reales responden 503.
	depsOleada5 := &dependenciasOleada5{
		Router:      router,
		Dynamo:      dynamoAPI,
		Redis:       clienteRedis,
		Tabla:       cfgDeps.TablaDynamoDB,
		Logger:      logger,
		Tokens:      verificadorTokens,
		Authorizer:  logoutStore,
		CursorKey:   claveCursorDesdeEntorno(),
		DemoEnabled: demoHabilitadoOleada5(),
	}
	montarOleada5(depsOleada5)

	// E063: el worker durable procesa la cola READALL fuera de la petición. Sin
	// persistencia no se arranca; el endpoint seguiría aceptando trabajo que nadie
	// ejecutaría, por lo que el fallo se registra y no se simula.
	if cerrarReadAll, errReadAll := iniciarReadAllWorkerOleada4(ctx, depsOleada4); errReadAll != nil {
		logger.WarnContext(ctx, "E063: worker durable no iniciado",
			slog.String("clase", "READALL_WORKER_NO_INICIADO"))
	} else {
		defer cerrarReadAll()
	}

	// Emisor FCM real: se arranca sólo si existe credencial de servidor válida en
	// el secret store/entorno. Sin ella el worker queda inerte y no se afirma E2E.
	cfgFCM, errFCM := configuracionFCMOleada4DesdeEntorno()
	if errFCM != nil {
		logger.WarnContext(ctx, "FCM Oleada 4: credencial no inyectada; worker no iniciado",
			slog.String("clase", "FCM_CREDENCIAL_AUSENTE"))
	} else {
		cerrarFCM, errWorker := iniciarFCMWorkerOleada4(ctx, depsOleada4, cfgFCM)
		if errWorker != nil {
			logger.WarnContext(ctx, "FCM Oleada 4: worker no iniciado",
				slog.String("clase", "FCM_WORKER_NO_INICIADO"))
		} else {
			defer cerrarFCM()
		}
	}

	// Scheduler durable de recordatorios (Oleada 5): recupera ocurrencias vencidas,
	// las encola en el outbox push y avanza la siguiente. Sin DynamoDB/tabla no se
	// inicia; el resto del servidor sigue disponible.
	cerrarScheduler, errScheduler := iniciarSchedulerOleada5(ctx, depsOleada5)
	if errScheduler != nil {
		logger.WarnContext(ctx, "scheduler Oleada 5: no iniciado",
			slog.String("clase", "SCHEDULER_NO_INICIADO"))
	} else {
		defer cerrarScheduler()
	}

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
