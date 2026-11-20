package main

import (
	"log/slog"
	"net/http"
	"time"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appGoals "github.com/nexumpr/Nexum-Backend/internal/application/goals"
	cacheinfra "github.com/nexumpr/Nexum-Backend/internal/infrastructure/cache"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

func init() {
	registrarOleada3(montajeOleada3{
		ID:     "E042",
		Metodo: http.MethodDelete,
		Ruta:   "/goals/{goalId}",
		Montar: montarE042,
	})
}

// montarE042 conecta el borrado logico de metas. Sin DynamoDB o tabla responde 503.
func montarE042(d *dependenciasOleada3) {
	if d == nil || d.Router == nil {
		return
	}
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	cliente, ok := oleada3DynamoCliente(d)
	if !ok || cliente == nil || d.Tabla == "" {
		logger.Warn("E042: persistencia no configurada; el endpoint respondera 503",
			slog.String("motivo", "dynamodb_no_configurado"))
	}
	servicio := appGoals.NewService(dynamoAdapter.NewGoalsStore(cliente, d.Tabla))
	var limitador nexumhttp.LimitadorRuta
	if d.Redis != nil {
		limitador = cacheinfra.NewAuthRateLimiter(d.Redis, 60, time.Minute)
	}
	handler := nexumhttp.NewGoalDeleteHandler(servicio, d.Tokens, d.Authorizer, limitador, logger)
	d.Router.HandleFunc(http.MethodDelete, "/goals/{goalId}", handler.ServeHTTP)
}
