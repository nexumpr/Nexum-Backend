package main

import (
	"net/http"

	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

func init() {
	registrarOleada2(montajeOleada2{
		ID:     "E030",
		Metodo: http.MethodPatch,
		Ruta:   "/movements/{movementId}",
		Montar: montarE030,
	})
}

// montarE030 conecta `PATCH /api/v1/movements/{movementId}` (E030).
//
// El `If-Match` obligatorio lo exige el handler: sin él responde 428 y con una versión obsoleta
// 412, de modo que montar esta ruta no habilita una escritura sin precondición.
func montarE030(d *dependenciasOleada2) {
	if d == nil || d.Router == nil {
		return
	}
	servicio, tokens, autorizador, limitador, logger := dependenciasMovimientos(d)
	handler := nexumhttp.NewMovementHandler(servicio, tokens, autorizador, limitador, logger)
	d.Router.HandleFunc(http.MethodPatch, "/movements/{movementId}", handler.ServeHTTP)
}
