package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/redis/go-redis/v9"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	sesAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/ses"
	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	cacheinfra "github.com/nexumpr/Nexum-Backend/internal/infrastructure/cache"
	"github.com/nexumpr/Nexum-Backend/internal/infrastructure/security"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// montarE010 conecta la solicitud real de verificacion de correo (E010).
//
// Fail-closed: sin persistencia, sin remitente SES, sin Redis (cuota), sin proveedor JWT o
// sin autorizador de sesiones, el endpoint responde 503 y no se envia nada. Reutiliza la
// tabla unica, el namespace IAM `ONE_USE#*` ya autorizado y la politica SES vigente
// (`ses:SendEmail` sobre la identidad con `ses:FromAddress` fijo): no crea recursos nuevos.
func montarE010(
	router registradorHTTP,
	clienteDynamo *dynamodb.Client,
	clienteRedis *redis.Client,
	tabla string,
	logger *slog.Logger,
) {
	var store *dynamoAdapter.EmailVerificationStore
	if clienteDynamo != nil && tabla != "" {
		store = dynamoAdapter.NewEmailVerificationStore(clienteDynamo, tabla)
	} else {
		store = dynamoAdapter.NewEmailVerificationStore(nil, "")
	}

	// La cuota se seudonimiza con la misma clave HMAC de cuotas ya desplegada (RFC-013).
	var limitador ports.Limitador
	if clienteRedis != nil {
		if claveHMAC, err := cacheinfra.LoadLoginHMACKey(); err == nil {
			limitador, _ = cacheinfra.NewEmailVerificationLimiter(clienteRedis, claveHMAC)
		}
	}
	if limitador == nil {
		logger.Warn("E010: limitador no configurable; el endpoint respondera 503",
			slog.String("motivo", "sin_redis_o_clave_hmac"))
	}

	// El remitente es obligatorio. Se declara como interfaz para no pasar un puntero nil
	// tipado (que pareceria disponible).
	var correo appIdentity.EmailVerificationMailer
	if remitente := strings.TrimSpace(os.Getenv("NEXUM_SES_FROM_EMAIL")); remitente != "" {
		cfgSES, errCfg := config.LoadDefaultConfig(context.Background(), config.WithRegion(regionSES()))
		if errCfg != nil {
			logger.Error("E010: no se pudo configurar SES; el endpoint respondera 503",
				slog.String("error_class", "ses_config"))
		} else {
			// Cliente HTTP propio: sin reutilizar conexiones ociosas y con timeouts
			// cortos, para que un socket muerto no consuma el deadline completo del envio.
			cfgSES.HTTPClient = sesHTTPClient()
			correo = sesAdapter.NewMailer(sesv2.NewFromConfig(cfgSES), remitente)
		}
	} else {
		logger.Warn("E010: sin remitente SES; el endpoint respondera 503",
			slog.String("motivo", "sin_remitente"))
	}

	servicio := appIdentity.NewEmailVerificationRequestService(store, correo, limitador, logger)
	if !servicio.Disponible() {
		logger.Warn("E010: componentes incompletos; el endpoint respondera 503",
			slog.String("motivo", "sin_persistencia_correo_o_cuota"))
	}

	// La ruta exige Bearer JWT RS256 y estado autoritativo de la sesion (E004). Sin material
	// de firma o sin tabla, el middleware falla cerrado con 503.
	var verificador ports.TokenProvider
	if proveedor, err := security.LoadTokenProvider(); err == nil {
		verificador = proveedor
	} else {
		logger.Error("E010: clave de firma JWT no disponible; el endpoint respondera 503",
			slog.String("error_class", "signing_key_unavailable"))
	}
	var dynamoAPI dynamoAdapter.DynamoAPI
	if clienteDynamo != nil {
		dynamoAPI = clienteDynamo
	}
	autorizador := dynamoAdapter.NewLogoutStore(dynamoAPI, tabla)

	handler := nexumhttp.NewEmailVerificationRequestHandler(servicio, verificador, autorizador, logger)
	router.HandleFunc(http.MethodPost, "/auth/email-verification/request", handler.ServeHTTP)
}
