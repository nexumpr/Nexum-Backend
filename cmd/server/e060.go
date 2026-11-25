package main

import (
	"net/http"

	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

func init() {
	registrarOleada4(montajeOleada4{
		ID:     "E060",
		Metodo: http.MethodDelete,
		Ruta:   "/devices/push-tokens/{deviceRegistrationId}",
		Montar: montarE060,
	})
}

// montarE060 conecta `DELETE /api/v1/devices/push-tokens/{deviceRegistrationId}`
// (E060). Comparte almacén y sellador con E059; la baja es condicional por
// usuario e identificador y retira el token cifrado de forma permanente.
func montarE060(d *dependenciasOleada4) {
	if d == nil || d.Router == nil {
		return
	}
	logger := loggerPushDevices(d)
	servicio := servicioPushDevices(d, logger)
	limitador := limitadorOleada4(d)
	handler := nexumhttp.NewPushDeviceDeleteHandler(servicio, d.Tokens, d.Authorizer, limitador, logger)
	d.Router.HandleFunc(http.MethodDelete, "/devices/push-tokens/{deviceRegistrationId}", handler.ServeHTTP)
}
