package main

import (
	"log/slog"
	"net/http"

	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

func init() {
	registrarOleada1(montajeOleada{
		ID:     "E025",
		Metodo: http.MethodPut,
		Ruta:   "/income",
		Montar: montarE025,
	})
}

// montarE025 conecta `PUT /api/v1/income` (E025).
//
// Reutiliza exactamente las mismas dependencias que E024 (`dependenciasIngreso`): la misma
// tabla, el mismo material JWT, el mismo autorizador de sesión y la misma cuota. Escribe
// `USER#<id>/INCOME` con una única escritura condicional por versión, con `If-Match`
// obligatorio; no toca `PROFILE`, ni la autenticación, ni las sesiones, y no amplía IAM.
func montarE025(d *dependenciasOleada) {
	if d == nil || d.Router == nil {
		return
	}
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}

	servicio, tokens, sesiones, limitador := dependenciasIngreso(d, logger)
	handler := nexumhttp.NewIncomePutHandler(servicio, tokens, sesiones, limitador, logger)
	d.Router.HandleFunc(http.MethodPut, "/income", handler.ServeHTTP)
}
