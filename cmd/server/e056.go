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

func init() {
	registrarOleada4(montajeOleada4{
		ID:     "E056",
		Metodo: http.MethodGet,
		Ruta:   "/analytics/savings-streak",
		Montar: montarE056,
	})
}

// montarE056 conecta `GET /api/v1/analytics/savings-streak` (E056). Reutiliza el
// adaptador de aportaciones: la racha se calcula sobre contribuciones activas,
// nunca sobre una suma duplicada de movimientos vinculados.
func montarE056(d *dependenciasOleada4) {
	if d == nil || d.Router == nil {
		return
	}
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	cliente, ok := oleada4DynamoCliente(d)
	if !ok || cliente == nil || d.Tabla == "" {
		logger.Warn("E056: persistencia no configurada; el endpoint respondera 503",
			slog.String("motivo", "dynamodb_no_configurado"))
	}

	store := dynamoAdapter.NewContributionsStore(cliente, d.Tabla)
	servicio := appAnalytics.NewService(nil).WithSavings(store, store)
	var limitador nexumhttp.LimitadorRuta
	if d.Redis != nil {
		limitador = cacheinfra.NewAuthRateLimiter(d.Redis, cuotaAnalyticsPorMinuto, time.Minute)
	}
	handler := nexumhttp.NewAnalyticsSavingsHandler(servicio, d.Tokens, d.Authorizer, limitador, logger)
	d.Router.HandleFunc(http.MethodGet, "/analytics/savings-streak", handler.ServeHTTP)
}
