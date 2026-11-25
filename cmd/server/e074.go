package main

import (
	"net/http"

	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

func init() {
	registrarOleada5(montajeOleada5{
		ID: "E074", Metodo: http.MethodGet, Ruta: "/demo/bootstrap", Montar: montarE074,
	})
	registrarOleada5(montajeOleada5{
		ID: "E075", Metodo: http.MethodPost, Ruta: "/voice/parse", Montar: montarE075,
	})
}

func montarE074(d *dependenciasOleada5) {
	if d == nil || d.Router == nil {
		return
	}
	logger := loggerOleada5(d)
	handler := nexumhttp.NewDemoBootstrapHandler(
		d.DemoEnabled, d.Tokens, d.Authorizer, limitadorRecordatoriosOleada5(d), logger,
	)
	d.Router.HandleFunc(http.MethodGet, "/demo/bootstrap", handler.ServeHTTP)
}

func montarE075(d *dependenciasOleada5) {
	if d == nil || d.Router == nil {
		return
	}
	logger := loggerOleada5(d)
	handler := nexumhttp.NewVoiceParseHandler(
		d.Tokens, d.Authorizer, limitadorRecordatoriosOleada5(d), logger,
	)
	d.Router.HandleFunc(http.MethodPost, "/voice/parse", handler.ServeHTTP)
}
