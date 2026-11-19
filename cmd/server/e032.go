package main

import (
	"net/http"

	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

func init() {
	registrarOleada2(montajeOleada2{
		ID:     "E032",
		Metodo: http.MethodGet,
		Ruta:   "/recurring-payments",
		Montar: montarE032,
	})
}

// montarE032 conecta `GET /api/v1/recurring-payments` (E032).
//
// Consulta la partición del titular por prefijo `RECURRING#` con filtros de servidor y cursor
// opaco firmado. No usa `Scan`, no escribe nada y no amplía IAM: la partición `USER#*` ya está
// autorizada por la política del runtime. La clave de firma de cursor se deriva del bundle
// RFC-004 con separación de dominio; sin ella el endpoint responde 503.
func montarE032(d *dependenciasOleada2) {
	if d == nil || d.Router == nil {
		return
	}
	servicio, tokens, autorizador, limitador, logger := dependenciasRecurrentes(d)
	handler := nexumhttp.NewRecurringListHandler(servicio, tokens, autorizador, limitador, logger)
	d.Router.HandleFunc(http.MethodGet, "/recurring-payments", handler.ServeHTTP)
}
