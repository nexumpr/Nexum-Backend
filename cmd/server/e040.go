package main

import (
	"log/slog"
	"net/http"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appGoals "github.com/nexumpr/Nexum-Backend/internal/application/goals"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

func init() {
	registrarOleada2(montajeOleada2{
		ID:     "E040",
		Metodo: http.MethodGet,
		Ruta:   "/goals/{goalId}",
		Montar: montarE040,
	})
}

// montarE040 conecta la consulta de una meta propia (E040).
//
// Fail-closed: sin persistencia el almacén falla cerrado y el handler responde 503; sin
// proveedor de tokens JWT o sin autorizador de sesiones el handler también responde 503 y nunca
// autoriza sólo con la firma.
//
// La lectura sólo toca `USER#<userId>`/`GOAL#<goalId>`, una clave que la política IAM vigente ya
// autoriza: no se crean recursos, tablas ni índices nuevos.
func montarE040(d *dependenciasOleada2) {
	if d == nil || d.Router == nil {
		return
	}
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}

	var api dynamoAdapter.GoalsAPI
	if cliente, ok := oleada2DynamoCliente(d); ok {
		api = cliente
	}
	store := dynamoAdapter.NewGoalsStore(api, d.Tabla)
	servicio := appGoals.NewService(store)

	if api == nil || d.Tabla == "" {
		logger.Warn("E040: sin persistencia; el endpoint respondera 503",
			slog.String("motivo", "sin_cliente_dynamodb_o_tabla"))
	}
	if d.Tokens == nil || d.Authorizer == nil {
		logger.Warn("E040: componentes de autenticacion incompletos; el endpoint respondera 503",
			slog.String("motivo", "sin_credenciales_o_autorizador"))
	}

	handler := nexumhttp.NewGoalGetHandler(servicio, d.Tokens, d.Authorizer, logger)
	d.Router.HandleFunc(http.MethodGet, "/goals/{goalId}", handler.ServeHTTP)
}
