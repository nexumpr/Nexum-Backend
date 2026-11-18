package main

import (
	"log/slog"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/redis/go-redis/v9"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	cacheinfra "github.com/nexumpr/Nexum-Backend/internal/infrastructure/cache"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// montarE011 conecta la confirmacion real de verificacion de correo (E011).
//
// Ruta PUBLICA (sin Bearer): la autoridad es el token de un solo uso. Fail-closed: sin
// persistencia o sin la clave HMAC de cuotas (Redis), el endpoint responde 503. La transaccion
// toca exclusivamente `USER#<id>/PROFILE` y `ONE_USE#<sha256(token)>/VERIFY`, ambos ya
// autorizados por la politica IAM vigente: no crea recursos nuevos.
func montarE011(
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

	servicio := appIdentity.NewEmailVerificationConfirmService(store)

	var limitador nexumhttp.EmailVerificationConfirmLimiter
	if clienteRedis != nil {
		if claveHMAC, err := cacheinfra.LoadLoginHMACKey(); err == nil {
			limitador, _ = cacheinfra.NewEmailVerificationConfirmLimiter(clienteRedis, claveHMAC)
		}
	}
	if limitador == nil {
		logger.Warn("E011: limitador no configurable; el endpoint respondera 503",
			slog.String("motivo", "sin_redis_o_clave_hmac"))
	}

	handler := nexumhttp.NewEmailVerificationConfirmHandler(servicio, limitador, logger)
	router.HandleFunc(http.MethodPost, "/auth/email-verification/confirm", handler.ServeHTTP)
}
