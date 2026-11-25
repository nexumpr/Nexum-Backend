package main

import (
	"log/slog"
	"net/http"

	dynamoLocal "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appPush "github.com/nexumpr/Nexum-Backend/internal/application/pushdevices"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

func init() {
	registrarOleada4(montajeOleada4{
		ID:     "E059",
		Metodo: http.MethodPost,
		Ruta:   "/devices/push-tokens",
		Montar: montarE059,
	})
}

// montarE059 conecta `POST /api/v1/devices/push-tokens` (E059). El token se
// sella con el `PushSealer` inyectado; si falta DynamoDB o el sellador, el
// handler responde 503 sin escribir nada.
func montarE059(d *dependenciasOleada4) {
	if d == nil || d.Router == nil {
		return
	}
	logger := loggerPushDevices(d)
	servicio := servicioPushDevices(d, logger)
	limitador := limitadorOleada4(d)
	handler := nexumhttp.NewPushDevicesHandler(servicio, d.Tokens, d.Authorizer, limitador, logger)
	d.Router.HandleFunc(http.MethodPost, "/devices/push-tokens", handler.ServeHTTP)
}

// servicioPushDevices construye el caso de uso compartido por E059 y E060 sobre
// la tabla única existente. No crea clientes ni recursos AWS.
func servicioPushDevices(d *dependenciasOleada4, logger *slog.Logger) *appPush.Service {
	cliente, ok := oleada4DynamoCliente(d)
	if !ok || cliente == nil || d.Tabla == "" {
		logger.Warn("E059/E060: persistencia no configurada; el endpoint respondera 503",
			slog.String("motivo", "dynamodb_no_configurado"))
	}
	store := dynamoLocal.NewPushDevicesStore(cliente, d.Tabla)
	sealer := d.PushSealer
	if sealer == nil {
		logger.Warn("E059/E060: cifrado push no configurado; el endpoint respondera 503",
			slog.String("motivo", "push_sealer_no_configurado"))
	}
	return appPush.NewService(store, sealer)
}

func loggerPushDevices(d *dependenciasOleada4) *slog.Logger {
	if d != nil && d.Logger != nil {
		return d.Logger
	}
	return slog.Default()
}
