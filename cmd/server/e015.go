package main

import (
	"net/http"

	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

func init() {
	registrarOleada1(montajeOleada{
		ID:     "E015",
		Metodo: http.MethodGet,
		Ruta:   "/privacy/notices/current",
		Montar: montarE015,
	})
}

// montarE015 conecta el aviso de privacidad público. No depende de DynamoDB,
// Redis, JWT ni de un repositorio externo: el contenido es un recurso versionado
// del propio binario.
func montarE015(d *dependenciasOleada) {
	if d == nil || d.Router == nil {
		return
	}
	d.Router.HandleFunc(http.MethodGet, "/privacy/notices/current", nexumhttp.NewPrivacyNoticeHandler(d.Logger).ServeHTTP)
}
