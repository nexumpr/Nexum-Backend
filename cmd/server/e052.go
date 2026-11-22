package main

import (
	"log/slog"
	"net/http"
	"time"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appAnalytics "github.com/nexumpr/Nexum-Backend/internal/application/analytics"
	cacheinfra "github.com/nexumpr/Nexum-Backend/internal/infrastructure/cache"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// cuotaAnalyticsPorMinuto acota E052–E056 por usuario. Redis compartido da consistencia
// entre las dos instancias; sin Redis no se simula una cuota.
const cuotaAnalyticsPorMinuto int64 = 60

func init() {
	registrarOleada4(montajeOleada4{
		ID:     "E052",
		Metodo: http.MethodGet,
		Ruta:   "/analytics/insights",
		Montar: montarE052,
	})
}

// montarE052 conecta `GET /api/v1/analytics/insights` (E052).
//
// Reutiliza el almacén real de movimientos: la consulta es `Query` sobre `USER#<sub>` con
// cotas de fecha, sin `Scan` ni índice nuevo. Sin cliente DynamoDB o tabla, el handler
// responde 503 en lugar de devolver insights calculados con datos inexistentes.
func montarE052(d *dependenciasOleada4) {
	if d == nil || d.Router == nil {
		return
	}
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	cliente, ok := oleada4DynamoCliente(d)
	if !ok || cliente == nil || d.Tabla == "" {
		logger.Warn("E052: persistencia no configurada; el endpoint respondera 503",
			slog.String("motivo", "dynamodb_no_configurado"))
	}

	servicio := appAnalytics.NewService(dynamoAdapter.NewMovementsStore(cliente, d.Tabla))
	var limitador nexumhttp.LimitadorRuta
	if d.Redis != nil {
		limitador = cacheinfra.NewAuthRateLimiter(d.Redis, cuotaAnalyticsPorMinuto, time.Minute)
	}
	handler := nexumhttp.NewAnalyticsInsightsHandler(servicio, d.Tokens, d.Authorizer, limitador, logger)
	d.Router.HandleFunc(http.MethodGet, "/analytics/insights", handler.ServeHTTP)
}
