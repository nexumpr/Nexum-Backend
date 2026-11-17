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
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// montarE007 conecta el cambio de contraseña de una sesión activa (E007, época A6/RFC-018).
//
// Fail-closed: sin persistencia, sin material JWT, sin autorizador de sesión o sin la clave
// HMAC de cuotas (Redis), el handler responde 503. La transacción toca exclusivamente
// `USER#<id>/PROFILE`, ya autorizada por la política IAM vigente: no hay recursos nuevos.
func montarE007(
	router registradorHTTP,
	clienteDynamo *dynamodb.Client,
	clienteRedis *redis.Client,
	tabla string,
	logger *slog.Logger,
) {
	var store *dynamoAdapter.PasswordChangeStore
	if clienteDynamo != nil && tabla != "" {
		store = dynamoAdapter.NewPasswordChangeStore(clienteDynamo, tabla)
	} else {
		store = dynamoAdapter.NewPasswordChangeStore(nil, "")
	}

	servicio := appIdentity.NewPasswordChangeService(store, security.NewArgon2Hasher(nil))

	var limitador nexumhttp.PasswordChangeLimiter
	if clienteRedis != nil {
		if claveHMAC, err := cacheinfra.LoadLoginHMACKey(); err == nil {
			limitador, _ = cacheinfra.NewPasswordChangeLimiter(clienteRedis, claveHMAC)
		}
	}
	if limitador == nil {
		logger.Warn("E007: limitador no configurable; el endpoint respondera 503",
			slog.String("motivo", "sin_redis_o_clave_hmac"))
	}

	// La ruta exige Bearer RS256 y estado autoritativo de la sesión en DynamoDB: sin
	// proveedor de tokens o sin autorizador el endpoint responde 503 en vez de autorizar
	// sólo con la firma (mismo criterio que E004).
	var verificadorTokens ports.TokenProvider
	proveedorTokens, errTokens := security.LoadTokenProvider()
	if errTokens != nil {
		logger.Warn("E007: clave de firma JWT no disponible; el endpoint respondera 503",
			slog.String("motivo", "signing_key_unavailable"))
	} else if proveedorTokens != nil {
		verificadorTokens = proveedorTokens
	}

	var autorizadorSesiones ports.SessionAuthorizer
	if clienteDynamo != nil && tabla != "" {
		autorizadorSesiones = dynamoAdapter.NewLogoutStore(clienteDynamo, tabla)
	}

	handler := nexumhttp.NewPasswordChangeHandler(servicio, limitador, verificadorTokens, autorizadorSesiones, logger)
	router.HandleFunc(http.MethodPost, "/auth/password/change", handler.ServeHTTP)
}
