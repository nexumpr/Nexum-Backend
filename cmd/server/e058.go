package main

import (
	"log/slog"
	"net/http"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appPrefs "github.com/nexumpr/Nexum-Backend/internal/application/notificationprefs"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

func init() {
	registrarOleada4(montajeOleada4{
		ID:     "E058",
		Metodo: http.MethodPut,
		Ruta:   "/notification-preferences",
		Montar: montarE058,
	})
}

// montarE058 conecta `PUT /api/v1/notification-preferences` (E058). Comparte
// almacén y servicio con E057; el handler despacha el método y aplica el CAS
// interno por versión del adaptador DynamoDB.
func montarE058(d *dependenciasOleada4) {
	if d == nil || d.Router == nil {
		return
	}
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	cliente, ok := oleada4DynamoCliente(d)
	if !ok || cliente == nil || d.Tabla == "" {
		logger.Warn("E058: persistencia no configurada; el endpoint respondera 503",
			slog.String("motivo", "dynamodb_no_configurado"))
	}

	store := dynamoAdapter.NewNotificationPreferencesStore(cliente, d.Tabla)
	servicio := appPrefs.NewService(store)
	limitador := limitadorOleada4(d)
	handler := nexumhttp.NewNotificationPreferencesHandler(servicio, d.Tokens, d.Authorizer, limitador, logger)
	d.Router.HandleFunc(http.MethodPut, "/notification-preferences", handler.ServeHTTP)
}
