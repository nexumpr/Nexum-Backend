package main

import (
	"net/http"

	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

func init() {
	registrarOleada1(montajeOleada{
		ID:     "E071",
		Metodo: http.MethodGet,
		Ruta:   "/education/tips",
		Montar: montarE071,
	})
}

// montarE071 conecta los consejos educativos publicos. Reutiliza el handler de
// contenido y la cuota por IP declarada en e070.go: no crea dependencias ni
// ventanas nuevas.
func montarE071(d *dependenciasOleada) {
	if d == nil || d.Router == nil {
		return
	}
	handler := nexumhttp.NewTipsHandler(d.Redis, d.Logger)
	d.Router.HandleFunc(http.MethodGet, "/education/tips",
		nexumhttp.LimitarPorIP(limitadorContenido(d.Redis), claseCuotaContenido, handler.ServeHTTP))
}
