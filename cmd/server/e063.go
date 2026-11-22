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
		ID:     "E063",
		Metodo: http.MethodPost,
		Ruta:   "/notifications/read-all",
		Montar: montarE063,
	})
}

// montarE063 conecta `POST /api/v1/notifications/read-all` (E063). El 202 se
// emite sólo después de persistir el registro READALL y procesar todas las
// páginas pendientes del titular; no hay acknowledgements falsos ni Scan.
func montarE063(d *dependenciasOleada4) {
	if d == nil || d.Router == nil {
		return
	}
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	cliente, ok := oleada4DynamoCliente(d)
	if !ok || cliente == nil || d.Tabla == "" {
		logger.Warn("E063: persistencia no configurada; el endpoint respondera 503",
			slog.String("motivo", "dynamodb_no_configurado"))
	}
	clave := d.CursorKey
	if len(clave) == 0 {
		logger.Warn("E063: clave de cursor no configurada; el endpoint respondera 503",
			slog.String("motivo", "cursor_key_no_configurada"))
	}

	store := dynamoAdapter.NewNotificationsStore(cliente, d.Tabla)
	servicio := appNotifications.NewService(store, clave)
	var limitador nexumhttp.LimitadorRuta
	if d.Redis != nil {
		limitador = cacheinfra.NewAuthRateLimiter(d.Redis, cuotaAnalyticsPorMinuto, time.Minute)
	}
	handler := nexumhttp.NewNotificationsHandler(servicio, d.Tokens, d.Authorizer, limitador, clave, logger)
	d.Router.HandleFunc(http.MethodPost, "/notifications/read-all", handler.ServeHTTP)
}
