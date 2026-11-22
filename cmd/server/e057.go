package main

import (
	"log/slog"
	"net/http"
	"time"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appPrefs "github.com/nexumpr/Nexum-Backend/internal/application/notificationprefs"
	cacheinfra "github.com/nexumpr/Nexum-Backend/internal/infrastructure/cache"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

func init() {
	registrarOleada4(montajeOleada4{
		ID:     "E057",
		Metodo: http.MethodGet,
		Ruta:   "/notification-preferences",
		Montar: montarE057,
	})
}

// montarE057 conecta `GET /api/v1/notification-preferences` (E057). Reutiliza
// el adaptador real de DynamoDB sobre la tabla única y materializa los defaults
// del contrato cuando el titular aún no tiene preferencias registradas.
func montarE057(d *dependenciasOleada4) {
	if d == nil || d.Router == nil {
		return
	}
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	cliente, ok := oleada4DynamoCliente(d)
	if !ok || cliente == nil || d.Tabla == "" {
		logger.Warn("E057: persistencia no configurada; el endpoint respondera 503",
			slog.String("motivo", "dynamodb_no_configurado"))
	}

	store := dynamoAdapter.NewNotificationPreferencesStore(cliente, d.Tabla)
	servicio := appPrefs.NewService(store)
	var limitador nexumhttp.LimitadorRuta
	if d.Redis != nil {
		limitador = cacheinfra.NewAuthRateLimiter(d.Redis, cuotaAnalyticsPorMinuto, time.Minute)
	}
	handler := nexumhttp.NewNotificationPreferencesHandler(servicio, d.Tokens, d.Authorizer, limitador, logger)
	d.Router.HandleFunc(http.MethodGet, "/notification-preferences", handler.ServeHTTP)
}
