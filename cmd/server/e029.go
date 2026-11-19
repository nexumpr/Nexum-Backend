package main

import (
	"net/http"

	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

func init() {
	registrarOleada2(montajeOleada2{
		ID:     "E029",
		Metodo: http.MethodGet,
		Ruta:   "/movements/{movementId}",
		Montar: montarE029,
	})
}

// montarE029 conecta `GET /api/v1/movements/{movementId}` (E029).
//
// Reutiliza las dependencias de la familia de movimientos (almacén DynamoDB, material JWT,
// autorizador de sesión y cuota). El identificador llega por el patrón `{movementId}`, que el
// router expone con `SetPathValue`; nunca por query ni por cabecera.
func montarE029(d *dependenciasOleada2) {
	if d == nil || d.Router == nil {
		return
	}
	servicio, tokens, autorizador, limitador, logger := dependenciasMovimientos(d)
	handler := nexumhttp.NewMovementHandler(servicio, tokens, autorizador, limitador, logger)
	d.Router.HandleFunc(http.MethodGet, "/movements/{movementId}", handler.ServeHTTP)
}
