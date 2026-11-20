package main

import (
	"log/slog"
	"net/http"
	"time"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appBudget "github.com/nexumpr/Nexum-Backend/internal/application/budgetset"
	cacheinfra "github.com/nexumpr/Nexum-Backend/internal/infrastructure/cache"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

func init() {
	registrarOleada3(montajeOleada3{
		ID:     "E049",
		Metodo: http.MethodPut,
		Ruta:   "/budgets",
		Montar: montarE049,
	})
}

// montarE049 conecta el reemplazo de presupuestos. Usa el mismo almacén que E048; el CAS lo
// aplica el adaptador con la versión leída de forma consistente.
func montarE049(d *dependenciasOleada3) {
	if d == nil || d.Router == nil {
		return
	}
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	cliente, ok := oleada3DynamoCliente(d)
	if !ok || cliente == nil || d.Tabla == "" {
		logger.Warn("E049: persistencia no configurada; el endpoint respondera 503",
			slog.String("motivo", "dynamodb_no_configurado"))
	}
	servicio := appBudget.NewService(dynamoAdapter.NewBudgetSetStore(cliente, d.Tabla))
	var limitador nexumhttp.LimitadorRuta
	if d.Redis != nil {
		limitador = cacheinfra.NewAuthRateLimiter(d.Redis, 60, time.Minute)
	}
	handler := nexumhttp.NewBudgetsReplaceHandler(servicio, d.Tokens, d.Authorizer, limitador, logger)
	d.Router.HandleFunc(http.MethodPut, "/budgets", handler.ServeHTTP)
}
