package main

import (
	"log/slog"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/redis/go-redis/v9"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	cacheinfra "github.com/nexumpr/Nexum-Backend/internal/infrastructure/cache"
	"github.com/nexumpr/Nexum-Backend/internal/infrastructure/security"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// montarE006 conecta el cambio real de contraseña (RFC-016 §17.3, A6).
//
// Fail-closed: sin persistencia o sin la clave HMAC de cuotas (Redis), el handler
// responde 503. La transacción toca exclusivamente `USER#<id>/PROFILE` y
// `RESET#<sha256(token)>/RESET`, ambas ya autorizadas por la política IAM vigente.
func montarE006(
	router registradorHTTP,
	clienteDynamo *dynamodb.Client,
	clienteRedis *redis.Client,
	tabla string,
	logger *slog.Logger,
) {
	var store *dynamoAdapter.PasswordResetConfirmStore
	if clienteDynamo != nil && tabla != "" {
		store = dynamoAdapter.NewPasswordResetConfirmStore(clienteDynamo, tabla)
	} else {
		store = dynamoAdapter.NewPasswordResetConfirmStore(nil, "")
	}

	servicio := appIdentity.NewPasswordResetConfirmService(store, security.NewArgon2Hasher(nil))

	var limitador nexumhttp.PasswordResetConfirmLimiter
	if clienteRedis != nil {
		if claveHMAC, err := cacheinfra.LoadLoginHMACKey(); err == nil {
			limitador, _ = cacheinfra.NewResetConfirmLimiter(clienteRedis, claveHMAC)
		}
	}
	if limitador == nil {
		logger.Warn("E006: limitador no configurable; el endpoint respondera 503",
			slog.String("motivo", "sin_redis_o_clave_hmac"))
	}

	handler := nexumhttp.NewPasswordResetConfirmHandler(servicio, limitador, logger)
	router.HandleFunc(http.MethodPost, "/auth/password/reset", handler.ServeHTTP)
}
