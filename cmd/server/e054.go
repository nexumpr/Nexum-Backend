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
		ID:     "E054",
		Metodo: http.MethodGet,
		Ruta:   "/analytics/spending-patterns",
		Montar: montarE054,
	})
}

// montarE054 conecta `GET /api/v1/analytics/spending-patterns` (E054).
// Reutiliza la Query acotada por usuario/mes compartida con E052 y E053, sin Scan.
func montarE054(d *dependenciasOleada4) {
	if d == nil || d.Router == nil {
		return
	}
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	cliente, ok := oleada4DynamoCliente(d)
	if !ok || cliente == nil || d.Tabla == "" {
		logger.Warn("E054: persistencia no configurada; el endpoint respondera 503",
			slog.String("motivo", "dynamodb_no_configurado"))
	}

	servicio := appAnalytics.NewService(dynamoAdapter.NewMovementsStore(cliente, d.Tabla))
	limitador := limitadorOleada4(d)
	handler := nexumhttp.NewAnalyticsPatternsHandler(servicio, d.Tokens, d.Authorizer, limitador, logger)
	d.Router.HandleFunc(http.MethodGet, "/analytics/spending-patterns", handler.ServeHTTP)
}
