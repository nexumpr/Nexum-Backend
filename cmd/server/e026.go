package main

import (
	"log/slog"
	"net/http"
	"time"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appSummary "github.com/nexumpr/Nexum-Backend/internal/application/financialsummary"
	cacheinfra "github.com/nexumpr/Nexum-Backend/internal/infrastructure/cache"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

const cuotaResumenFinancieroPorMinuto int64 = 120

func init() {
	registrarOleada3(montajeOleada3{
		ID:     "E026",
		Metodo: http.MethodGet,
		Ruta:   "/financial-summary",
		Montar: montarE026,
	})
}

// montarE026 conecta el resumen financiero. Sin DynamoDB o tabla el almacén queda nulo y el
// handler responde 503: nunca se inventan totales.
func montarE026(d *dependenciasOleada3) {
	if d == nil || d.Router == nil {
		return
	}
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	cliente, ok := oleada3DynamoCliente(d)
	if !ok || cliente == nil || d.Tabla == "" {
		logger.Warn("E026: persistencia no configurada; el endpoint respondera 503",
			slog.String("motivo", "dynamodb_no_configurado"))
	}
	servicio := appSummary.NewService(dynamoAdapter.NewFinancialSummaryStore(cliente, d.Tabla))
	var limitador nexumhttp.LimitadorRuta
	if d.Redis != nil {
		limitador = cacheinfra.NewAuthRateLimiter(d.Redis, cuotaResumenFinancieroPorMinuto, time.Minute)
	}
	handler := nexumhttp.NewFinancialSummaryHandler(servicio, d.Tokens, d.Authorizer, limitador, logger)
	d.Router.HandleFunc(http.MethodGet, "/financial-summary", handler.ServeHTTP)
}
