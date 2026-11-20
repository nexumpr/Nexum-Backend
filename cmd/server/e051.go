package main

import (
	"log/slog"
	"net/http"
	"time"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appDashboard "github.com/nexumpr/Nexum-Backend/internal/application/dashboard"
	appSummary "github.com/nexumpr/Nexum-Backend/internal/application/financialsummary"
	cacheinfra "github.com/nexumpr/Nexum-Backend/internal/infrastructure/cache"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

const cuotaDashboardPorMinuto int64 = 120

func init() {
	registrarOleada3(montajeOleada3{
		ID:     "E051",
		Metodo: http.MethodGet,
		Ruta:   "/dashboard",
		Montar: montarE051,
	})
}

// montarE051 conecta el dashboard sin caché Redis. Usa E026 para el resumen y consultas
// directas para metas activas y movimientos recientes.
func montarE051(d *dependenciasOleada3) {
	if d == nil || d.Router == nil {
		return
	}
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	cliente, ok := oleada3DynamoCliente(d)
	if !ok || cliente == nil || d.Tabla == "" {
		logger.Warn("E051: persistencia no configurada; el endpoint respondera 503",
			slog.String("motivo", "dynamodb_no_configurado"))
	}
	summary := appSummary.NewService(dynamoAdapter.NewFinancialSummaryStore(cliente, d.Tabla))
	dashboard := appDashboard.NewService(summary, dynamoAdapter.NewDashboardStore(cliente, d.Tabla))
	var limitador nexumhttp.LimitadorRuta
	if d.Redis != nil {
		limitador = cacheinfra.NewAuthRateLimiter(d.Redis, cuotaDashboardPorMinuto, time.Minute)
	}
	handler := nexumhttp.NewDashboardHandler(dashboard, d.Tokens, d.Authorizer, limitador, logger)
	d.Router.HandleFunc(http.MethodGet, "/dashboard", handler.ServeHTTP)
}
