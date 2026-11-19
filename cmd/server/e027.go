package main

import (
	"net/http"

	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

func init() {
	registrarOleada2(montajeOleada2{
		ID:     "E027",
		Metodo: http.MethodGet,
		Ruta:   "/movements",
		Montar: montarE027,
	})
}

// montarE027 conecta `GET /api/v1/movements` (E027).
//
// Reutiliza exactamente las mismas dependencias que E028 (almacén DynamoDB, material JWT,
// autorizador de sesión, cuota y clave de firma de cursores): no se construye un segundo cliente
// ni se introduce un secreto nuevo. Sin clave de cursor el listado responde 503 en lugar de
// paginar sin firmar.
func montarE027(d *dependenciasOleada2) {
	if d == nil || d.Router == nil {
		return
	}
	servicio, tokens, autorizador, limitador, logger := dependenciasMovimientos(d)
	handler := nexumhttp.NewMovementsHandler(servicio, tokens, autorizador, limitador, logger)
	d.Router.HandleFunc(http.MethodGet, "/movements", handler.ServeHTTP)
}
