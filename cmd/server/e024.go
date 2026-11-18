package main

import (
	"log/slog"
	"net/http"
	"time"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	cacheinfra "github.com/nexumpr/Nexum-Backend/internal/infrastructure/cache"
	"github.com/nexumpr/Nexum-Backend/internal/infrastructure/security"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

func init() {
	registrarOleada1(montajeOleada{
		ID:     "E024",
		Metodo: http.MethodGet,
		Ruta:   "/income",
		Montar: montarE024,
	})
}

// dependenciasIngreso agrupa lo que necesitan E024 y E025. No crea instancias nuevas:
// reutiliza DynamoDB, Redis, el material JWT y el autorizador de sesión ya construidos.
//
// Fail-closed: si falta persistencia, el proveedor de tokens o el autorizador de sesión, los
// campos quedan nulos y el handler responde 503. La cuota distribuida (Redis) es opcional:
// sin ella se omite y 429 no se emite, en lugar de simularse.
func dependenciasIngreso(
	d *dependenciasOleada,
	logger *slog.Logger,
) (*appIdentity.IncomeService, ports.TokenProvider, ports.SessionAuthorizer, nexumhttp.LimitadorRuta) {
	var store *dynamoAdapter.IncomeStore
	if d.Dynamo != nil && d.Tabla != "" {
		store = dynamoAdapter.NewIncomeStore(d.Dynamo, d.Tabla)
	} else {
		store = dynamoAdapter.NewIncomeStore(nil, "")
		logger.Warn("income: persistencia no configurada; E024/E025 responderan 503",
			slog.String("motivo", "dynamodb_no_configurado"))
	}
	servicio := appIdentity.NewIncomeService(store)

	// La ruta exige Bearer RS256 y estado autoritativo de la sesión en DynamoDB: sin
	// proveedor de tokens o sin autorizador se responde 503 en vez de autorizar sólo con la
	// firma (mismo criterio que E004/E007).
	var verificadorTokens ports.TokenProvider
	proveedorTokens, errTokens := security.LoadTokenProvider()
	if errTokens != nil {
		logger.Warn("income: clave de firma JWT no disponible; E024/E025 responderan 503",
			slog.String("motivo", "signing_key_unavailable"))
	} else if proveedorTokens != nil {
		verificadorTokens = proveedorTokens
	}

	var autorizadorSesiones ports.SessionAuthorizer
	var dynamoAPI dynamoAdapter.DynamoAPI
	if d.Dynamo != nil && d.Tabla != "" {
		dynamoAPI = d.Dynamo
		autorizadorSesiones = dynamoAdapter.NewLogoutStore(dynamoAPI, d.Tabla)
	}

	var limitador nexumhttp.LimitadorRuta
	if d.Redis != nil {
		limitador = cacheinfra.NewAuthRateLimiter(d.Redis, cuotaOleada1PorMinuto, time.Minute)
	}

	return servicio, verificadorTokens, autorizadorSesiones, limitador
}

// montarE024 conecta `GET /api/v1/income` (E024).
//
// Lee `USER#<id>/INCOME` con consistencia fuerte. No escribe nada, no usa Scan y no amplía
// IAM: la clave ya está autorizada por la política del runtime (`LeadingKeys` `USER#*`).
func montarE024(d *dependenciasOleada) {
	if d == nil || d.Router == nil {
		return
	}
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}

	servicio, tokens, sesiones, limitador := dependenciasIngreso(d, logger)
	handler := nexumhttp.NewIncomeGetHandler(servicio, tokens, sesiones, limitador, logger)
	d.Router.HandleFunc(http.MethodGet, "/income", handler.ServeHTTP)
}
