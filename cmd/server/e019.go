package main

import (
	"net/http"
)

func init() {
	registrarOleada1(montajeOleada{
		ID:     "E019",
		Metodo: http.MethodPatch,
		Ruta:   "/users/me/onboarding",
		Montar: montarE019,
	})
}

// montarE019 conecta la actualización del progreso de onboarding del titular
// (E019, `PATCH /api/v1/users/me/onboarding`).
//
// Comparte la composición de E018 (`nuevoHandlerOnboarding`): mismo proveedor de
// tokens, mismo autorizador de sesiones y el mismo almacén sobre la tabla única.
// No crea claves, almacenes ni recursos nuevos y no toca `PROFILE` ni la
// autenticación. Fail-closed: sin persistencia de escritura, sin firma o sin
// autorizador, el handler responde 503 en lugar de aceptar un cambio que no se
// guardó.
func montarE019(d *dependenciasOleada) {
	if d == nil || d.Router == nil {
		return
	}
	handler := nuevoHandlerOnboarding(d)
	d.Router.HandleFunc(http.MethodPatch, "/users/me/onboarding", handler.ServePatch)
}
