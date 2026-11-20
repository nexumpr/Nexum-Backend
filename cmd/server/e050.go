package main

import (
	"log/slog"
	"net/http"
	"time"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appUsage "github.com/nexumpr/Nexum-Backend/internal/application/budgetusage"
	cacheinfra "github.com/nexumpr/Nexum-Backend/internal/infrastructure/cache"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

func init() {
	registrarOleada3(montajeOleada3{
		ID:     "E050",
		Metodo: http.MethodGet,
		Ruta:   "/budgets/usage",
		Montar: montarE050,
	})
}

// montarE050 conecta el uso presupuestal. Lee el BUDGETSET y suma movimientos expense reales.
func montarE050(d *dependenciasOleada3) {
	if d == nil || d.Router == nil {
		return
	}
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	cliente, ok := oleada3DynamoCliente(d)
	if !ok || cliente == nil || d.Tabla == "" {
		logger.Warn("E050: persistencia no configurada; el endpoint respondera 503",
			slog.String("motivo", "dynamodb_no_configurado"))
	}
	servicio := appUsage.NewService(dynamoAdapter.NewBudgetUsageStore(cliente, d.Tabla))
	var limitador nexumhttp.LimitadorRuta
	if d.Redis != nil {
		limitador = cacheinfra.NewAuthRateLimiter(d.Redis, 120, time.Minute)
	}
	handler := nexumhttp.NewBudgetUsageHandler(servicio, d.Tokens, d.Authorizer, limitador, logger)
	d.Router.HandleFunc(http.MethodGet, "/budgets/usage", handler.ServeHTTP)
}
