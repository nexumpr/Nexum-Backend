package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	cryptoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/crypto"
	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	sesAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/ses"
	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	cacheinfra "github.com/nexumpr/Nexum-Backend/internal/infrastructure/cache"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
	"github.com/redis/go-redis/v9"
)

// registradorHTTP es el subconjunto del router que necesita el montaje de E005.
type registradorHTTP interface {
	HandleFunc(method, path string, handler http.HandlerFunc)
}

// montarE005 conecta el flujo real de recuperacion de contrasena (RFC-016 17.2).
//
// Fail-closed: sin sellador (A9), sin remitente/base publica, sin Redis o sin
// persistencia, el handler responde 503 y no se acepta trabajo que nadie podria
// entregar. No se registran dependencias simuladas.
func montarE005(
	ctx context.Context,
	router registradorHTTP,
	clienteDynamo *dynamodb.Client,
	clienteRedis *redis.Client,
	tabla string,
	logger *slog.Logger,
) (worker *appIdentity.ResetWorker, cerrar func()) {

	var (
		servicio *appIdentity.PasswordResetService
		buscar   appIdentity.BuscarCuenta
	)

	sellador, errSellador := cryptoAdapter.NuevoSealer(os.Getenv("PASSWORD_RESET_SEAL_KEY_B64"))
	remitente := os.Getenv("NEXUM_SES_FROM_EMAIL")
	basePublica := os.Getenv("NEXUM_PUBLIC_BASE_URL")

	if clienteDynamo != nil && tabla != "" {
		store := dynamoAdapter.NewPasswordResetStore(clienteDynamo, tabla)
		lookup := dynamoAdapter.NewIdentityLookup(clienteDynamo, tabla)
		buscar = lookup.BuscarPorEmail

		var limitador *cacheinfra.PasswordResetLimiter
		if clienteRedis != nil {
			if claveHMAC, err := cacheinfra.LoadLoginHMACKey(); err == nil {
				limitador, _ = cacheinfra.NewPasswordResetLimiter(clienteRedis, claveHMAC)
			}
		}

		if errSellador == nil && remitente != "" && basePublica != "" && limitador != nil {
			cfgSES, errCfg := config.LoadDefaultConfig(ctx, config.WithRegion(regionSES()))
			if errCfg != nil {
				logger.Error("E005: no se pudo configurar SES; el endpoint respondera 503",
					slog.String("error_class", "ses_config"))
			} else {
				correo := sesAdapter.NewMailer(sesv2.NewFromConfig(cfgSES), remitente, basePublica)
				servicio = appIdentity.NewPasswordResetService(store, correo, sellador, limitador, logger)
				if clienteRedis != nil {
					if elector, errE := cacheinfra.NewWorkerElector(clienteRedis, "nexum:v1:worker:pwreset:leader"); errE == nil {
						worker = appIdentity.NewResetWorker(store, correo, sellador, elector, logger)
					}
				}
			}
		}
	}

	if servicio == nil {
		logger.Warn("E005: componentes incompletos; el endpoint respondera 503",
			slog.String("motivo", "sin_sellador_remitente_redis_o_persistencia"))
	}

	handler := nexumhttp.NewPasswordForgotHandler(servicio, buscar, logger)
	router.HandleFunc(http.MethodPost, "/auth/password/forgot", handler.ServeHTTP)

	if worker != nil {
		go worker.Run(ctx)
	}

	return worker, func() {
		if worker != nil {
			logger.Info("E005: worker de recuperacion detenido")
		}
	}
}

// regionSES devuelve la region de SES, con us-east-2 como valor del proyecto.
func regionSES() string {
	if r := os.Getenv("AWS_REGION"); r != "" {
		return r
	}
	return "us-east-2"
}
