package main

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/redis/go-redis/v9"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	cacheinfra "github.com/nexumpr/Nexum-Backend/internal/infrastructure/cache"
	"github.com/nexumpr/Nexum-Backend/internal/infrastructure/security"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// cuotaSesionesPorMinuto es la cuota D-429 para las rutas de sesiones (120 req/min por usuario),
// aplicada con clases separadas para listar (`sessions`) y revocar (`sessions_revoke`).
const cuotaSesionesPorMinuto int64 = 120

// montarE008 conecta el listado paginado de sesiones propias (E008).
//
// Fail-closed: sin persistencia, sin proveedor de tokens, sin autorizador de sesiones o sin
// clave de firma de cursor, el handler responde 503 y no se lista nada. No se registran
// dependencias simuladas ni se degrada la firma del cursor a un valor por defecto.
//
// La clave de firma de cursor se deriva con HMAC-SHA256 y separación de dominio a partir de la
// clave HMAC ya existente en el bundle RFC-004 (la misma que usan las cuotas de login). Así no
// se introduce un secreto nuevo —lo que exigiría un Gate de infraestructura— y la clave de
// cursores no es utilizable como la de cuotas ni viceversa.
func montarE008(
	router registradorHTTP,
	clienteDynamo *dynamodb.Client,
	clienteRedis *redis.Client,
	tabla string,
	logger *slog.Logger,
) {
	if logger == nil {
		logger = slog.Default()
	}

	var api dynamoAdapter.SessionsAPI
	if clienteDynamo != nil {
		api = clienteDynamo
	}
	store := dynamoAdapter.NewSessionsStore(api, tabla)

	var tokens ports.TokenProvider
	proveedor, errTokens := security.LoadTokenProvider()
	if errTokens != nil {
		logger.Error("E008: clave de firma JWT no disponible; el endpoint respondera 503",
			slog.String("error_class", "signing_key_unavailable"))
	} else if proveedor != nil {
		tokens = proveedor
	}

	// El autorizador de sesiones es el mismo almacén real de E004: lee la sesión autoritativa
	// con consistencia fuerte, de modo que una sesión revocada en otro nodo deja de autorizar
	// en el acto.
	var autorizador ports.SessionAuthorizer
	if api != nil && tabla != "" {
		autorizador = dynamoAdapter.NewLogoutStore(api, tabla)
	}

	var servicio *appIdentity.SessionsService
	claveBase, errClave := cacheinfra.LoadLoginHMACKey()
	if errClave != nil {
		logger.Error("E008: clave HMAC base no disponible; el endpoint respondera 503",
			slog.String("error_class", "cursor_key_unavailable"))
	} else if secreto := httputil.DeriveCursorKey(claveBase, "e008"); len(secreto) > 0 {
		servicio = appIdentity.NewSessionsService(store, secreto)
	}

	if servicio == nil || tokens == nil || autorizador == nil {
		logger.Warn("E008: componentes incompletos; el endpoint respondera 503",
			slog.String("motivo", "sin_persistencia_credenciales_autorizador_o_clave_cursor"))
	}

	var limitador nexumhttp.LimitadorRuta
	if clienteRedis != nil {
		limitador = cacheinfra.NewAuthRateLimiter(clienteRedis, cuotaSesionesPorMinuto, time.Minute)
	}
	handler := nexumhttp.NewSessionsHandler(servicio, tokens, autorizador, limitador, logger)
	router.HandleFunc(http.MethodGet, "/auth/sessions", handler.ServeHTTP)
}
