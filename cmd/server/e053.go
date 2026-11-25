package main

import (
	"log/slog"
	"net/http"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appAnalytics "github.com/nexumpr/Nexum-Backend/internal/application/analytics"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

func init() {
	registrarOleada4(montajeOleada4{
		ID:     "E053",
		Metodo: http.MethodGet,
		Ruta:   "/analytics/categories",
		Montar: montarE053,
	})
}

// montarE053 conecta `GET /api/v1/analytics/categories` (E053).
//
// Reutiliza el mismo almacén y la misma cuota familiar de analítica que E052: una sola
// Query acotada a `USER#<sub>` y al rango del mes, sin Scan ni índice nuevo. Sin cliente
// DynamoDB o tabla, el handler responde 503 en lugar de devolver cifras inventadas.
func montarE053(d *dependenciasOleada4) {
	if d == nil || d.Router == nil {
		return
	}
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	cliente, ok := oleada4DynamoCliente(d)
	if !ok || cliente == nil || d.Tabla == "" {
		logger.Warn("E053: persistencia no configurada; el endpoint respondera 503",
			slog.String("motivo", "dynamodb_no_configurado"))
	}

	servicio := appAnalytics.NewService(dynamoAdapter.NewMovementsStore(cliente, d.Tabla))
	limitador := limitadorOleada4(d)
	handler := nexumhttp.NewAnalyticsCategoriesHandler(servicio, d.Tokens, d.Authorizer, limitador, logger)
	d.Router.HandleFunc(http.MethodGet, "/analytics/categories", handler.ServeHTTP)
}
