package main

import (
	"net/http"

	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

func init() {
	registrarOleada2(montajeOleada2{
		ID:     "E031",
		Metodo: http.MethodDelete,
		Ruta:   "/movements/{movementId}",
		Montar: montarE031,
	})
}

// montarE031 conecta `DELETE /api/v1/movements/{movementId}` (E031).
//
// Responde 204 sin cuerpo. Un movimiento ligado a una aportación confirmada no se borra: el caso
// de uso devuelve 409 y la reversa corresponde a la operación de metas.
func montarE031(d *dependenciasOleada2) {
	if d == nil || d.Router == nil {
		return
	}
	servicio, tokens, autorizador, limitador, logger := dependenciasMovimientos(d)
	handler := nexumhttp.NewMovementHandler(servicio, tokens, autorizador, limitador, logger)
	d.Router.HandleFunc(http.MethodDelete, "/movements/{movementId}", handler.ServeHTTP)
}
