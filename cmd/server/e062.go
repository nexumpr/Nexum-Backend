package main

import (
	"log/slog"
	"net/http"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appNotifications "github.com/nexumpr/Nexum-Backend/internal/application/notifications"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

func init() {
	registrarOleada4(montajeOleada4{
		ID:     "E062",
		Metodo: http.MethodPatch,
		Ruta:   "/notifications/{notificationId}/read",
		Montar: montarE062,
	})
}

// montarE062 conecta `PATCH /api/v1/notifications/{notificationId}/read` (E062).
// Comparte el adaptador real de DynamoDB con E061/E063; la transición atómica
// y la idempotencia las garantiza la condición `#isRead = false`.
func montarE062(d *dependenciasOleada4) {
	if d == nil || d.Router == nil {
		return
	}
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	cliente, ok := oleada4DynamoCliente(d)
	if !ok || cliente == nil || d.Tabla == "" {
		logger.Warn("E062: persistencia no configurada; el endpoint respondera 503",
			slog.String("motivo", "dynamodb_no_configurado"))
	}
	clave := d.CursorKey
	if len(clave) == 0 {
		logger.Warn("E062: clave de cursor no configurada; el endpoint respondera 503",
			slog.String("motivo", "cursor_key_no_configurada"))
	}

	store := dynamoAdapter.NewNotificationsStore(cliente, d.Tabla)
	servicio := appNotifications.NewService(store, clave)
	limitador := limitadorOleada4(d)
	handler := nexumhttp.NewNotificationsHandler(servicio, d.Tokens, d.Authorizer, limitador, clave, logger)
	d.Router.HandleFunc(http.MethodPatch, "/notifications/{notificationId}/read", handler.ServeHTTP)
}
