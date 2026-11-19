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
		ID:     "E039",
		Metodo: http.MethodPost,
		Ruta:   "/goals",
		Montar: montarE039,
	})
}

// montarE039 conecta la creación de metas de ahorro (E039).
//
// Fail-closed: sin persistencia el almacén falla cerrado y el handler responde 503; sin
// proveedor de tokens JWT o sin autorizador de sesiones el handler también responde 503 y
// nunca autoriza sólo con la firma. No se registran dependencias simuladas.
//
// La ruta sólo toca `USER#<userId>` (`GOAL#<goalId>` e `IDEM#...`), claves que la política IAM
// vigente ya autoriza: no se crean recursos, tablas ni índices nuevos.
func montarE039(d *dependenciasOleada2) {
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
		logger.Warn("E039: sin persistencia; el endpoint respondera 503",
			slog.String("motivo", "sin_cliente_dynamodb_o_tabla"))
	}
	if d.Tokens == nil || d.Authorizer == nil {
		logger.Warn("E039: componentes de autenticacion incompletos; el endpoint respondera 503",
			slog.String("motivo", "sin_credenciales_o_autorizador"))
	}

	handler := nexumhttp.NewGoalsCreateHandler(servicio, d.Tokens, d.Authorizer, logger)
	d.Router.HandleFunc(http.MethodPost, "/goals", handler.ServeHTTP)
}
