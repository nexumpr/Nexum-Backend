package main

import (
	"log/slog"
	"net/http"
	"time"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appNotifications "github.com/nexumpr/Nexum-Backend/internal/application/notifications"
	cacheinfra "github.com/nexumpr/Nexum-Backend/internal/infrastructure/cache"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

func init() {
	registrarOleada4(montajeOleada4{
		ID:     "E061",
		Metodo: http.MethodGet,
		Ruta:   "/notifications",
		Montar: montarE061,
	})
}

// montarE061 conecta `GET /api/v1/notifications` (E061) sobre la tabla única.
// La clave de cursores es obligatoria y estable: sin ella no se sirve el listado.
func montarE061(d *dependenciasOleada4) {
	if d == nil || d.Router == nil {
		return
	}
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	cliente, ok := oleada4DynamoCliente(d)
	if !ok || cliente == nil || d.Tabla == "" {
		logger.Warn("E061: persistencia no configurada; el endpoint respondera 503",
			slog.String("motivo", "dynamodb_no_configurado"))
	}
	clave := d.CursorKey
	if len(clave) == 0 {
		logger.Warn("E061: clave de cursor no configurada; el endpoint respondera 503",
			slog.String("motivo", "cursor_key_no_configurada"))
	}

	store := dynamoAdapter.NewNotificationsStore(cliente, d.Tabla)
	servicio := appNotifications.NewService(store, clave)
	var limitador nexumhttp.LimitadorRuta
	if d.Redis != nil {
		limitador = cacheinfra.NewAuthRateLimiter(d.Redis, cuotaAnalyticsPorMinuto, time.Minute)
	}
	handler := nexumhttp.NewNotificationsHandler(servicio, d.Tokens, d.Authorizer, limitador, clave, logger)
	d.Router.HandleFunc(http.MethodGet, "/notifications", handler.ServeHTTP)
}
