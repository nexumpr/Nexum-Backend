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
		ID:     "E055",
		Metodo: http.MethodGet,
		Ruta:   "/analytics/recommendations",
		Montar: montarE055,
	})
}

// montarE055 conecta `GET /api/v1/analytics/recommendations` (E055). El modelo
// K-Means se carga desde el artefacto incrustado verificado; no hay Python, AWS
// ni proveedor de pago en la ruta de petición.
func montarE055(d *dependenciasOleada4) {
	if d == nil || d.Router == nil {
		return
	}
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	cliente, ok := oleada4DynamoCliente(d)
	if !ok || cliente == nil || d.Tabla == "" {
		logger.Warn("E055: persistencia no configurada; el endpoint respondera 503",
			slog.String("motivo", "dynamodb_no_configurado"))
	}

	servicio := appAnalytics.NewService(dynamoAdapter.NewMovementsStore(cliente, d.Tabla))
	limitador := limitadorOleada4(d)
	handler := nexumhttp.NewAnalyticsRecommendationsHandler(servicio, d.Tokens, d.Authorizer, limitador, logger)
	d.Router.HandleFunc(http.MethodGet, "/analytics/recommendations", handler.ServeHTTP)
}
