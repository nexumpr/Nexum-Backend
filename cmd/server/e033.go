package main

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appRecurring "github.com/nexumpr/Nexum-Backend/internal/application/recurring"
	cacheinfra "github.com/nexumpr/Nexum-Backend/internal/infrastructure/cache"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

func init() {
	registroOleada2Global.registrar(montajeOleada2{
		ID:     "E033",
		Metodo: http.MethodPost,
		Ruta:   "/recurring-payments",
		Montar: montarE033,
	})
}

// dependenciasRecurrentes agrupa lo que necesitan E032–E036. No crea clientes: reutiliza
// DynamoDB, Redis, el material JWT y el autorizador de sesión ya construidos en `run()`.
//
// Fail-closed: si falta la persistencia, el proveedor de tokens o el autorizador de sesión,
// los campos quedan nulos y el handler responde 503. La cuota distribuida (Redis) es
// opcional: sin ella se omite y 429 no se emite, en lugar de simularse.
//
// La clave de firma de cursor sólo la necesita E032; se deriva con HMAC-SHA256 y separación
// de dominio a partir de la clave HMAC ya presente en el bundle RFC-004 (la misma que usan
// las cuotas de login). Así no se introduce un secreto nuevo —lo que exigiría un Gate de
// infraestructura— ni se reutiliza literalmente la clave de cuotas.
func dependenciasRecurrentes(d *dependenciasOleada2) (
	*appRecurring.Service,
	ports.TokenProvider,
	ports.SessionAuthorizer,
	nexumhttp.LimitadorRuta,
	*slog.Logger,
) {
	logger := slog.Default()
	if d != nil && d.Logger != nil {
		logger = d.Logger
	}

	var cliente *dynamodb.Client
	if d != nil {
		if concreto, ok := d.Dynamo.(*dynamodb.Client); ok && concreto != nil {
			cliente = concreto
		}
	}
	var tabla string
	if d != nil {
		tabla = d.Tabla
	}

	var store *dynamoAdapter.RecurringStore
	if cliente != nil && tabla != "" {
		store = dynamoAdapter.NewRecurringStore(cliente, tabla)
	} else {
		store = dynamoAdapter.NewRecurringStore(nil, "")
		logger.Warn("recurrentes: persistencia no configurada; los endpoints responderan 503",
			slog.String("motivo", "dynamodb_no_configurado"))
	}

	var claveCursor []byte
	claveBase, errClave := cacheinfra.LoadLoginHMACKey()
	if errClave != nil {
		logger.Warn("recurrentes: clave HMAC base no disponible; E032 respondera 503",
			slog.String("motivo", "cursor_key_unavailable"))
	} else {
		claveCursor = httputil.DeriveCursorKey(claveBase, "e032")
	}

	servicio := appRecurring.NewService(store, claveCursor)

	var limitador nexumhttp.LimitadorRuta
	if d != nil && d.Redis != nil {
		limitador = cacheinfra.NewAuthRateLimiter(d.Redis, cuotaOleada1PorMinuto, time.Minute)
	}

	var tokens ports.TokenProvider
	var autorizador ports.SessionAuthorizer
	if d != nil {
		tokens = d.Tokens
		autorizador = d.Authorizer
	}
	if tokens == nil || autorizador == nil {
		logger.Warn("recurrentes: credenciales o autorizador ausentes; los endpoints responderan 503",
			slog.String("motivo", "sin_credenciales_o_autorizador"))
	}

	return servicio, tokens, autorizador, limitador, logger
}

// montarE033 conecta `POST /api/v1/recurring-payments` (E033).
//
// Escribe `USER#<id>/RECURRING#<id>` y el registro de idempotencia en UNA transacción, sin
// `Scan`, sin tabla ni índice nuevos y sin ampliar IAM: la clave ya está autorizada por la
// política del runtime (`LeadingKeys` `USER#*`). No genera ningún pago ni movimiento.
func montarE033(d *dependenciasOleada2) {
	if d == nil || d.Router == nil {
		return
	}
	servicio, tokens, autorizador, limitador, logger := dependenciasRecurrentes(d)
	handler := nexumhttp.NewRecurringCreateHandler(servicio, tokens, autorizador, limitador, logger)
	d.Router.HandleFunc(http.MethodPost, "/recurring-payments", handler.ServeHTTP)
}
