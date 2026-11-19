package main

import (
	"net/http"

	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

func init() {
	registrarOleada2(montajeOleada2{
		ID:     "E035",
		Metodo: http.MethodPatch,
		Ruta:   "/recurring-payments/{recurringPaymentId}",
		Montar: montarE035,
	})
}

// montarE035 conecta `PATCH /api/v1/recurring-payments/{recurringPaymentId}` (E035).
//
// Modifica la regla con compare-and-swap por versión: `If-Match` es obligatorio y la escritura
// sólo se aplica si la versión persistida coincide. No crea recursos, no usa `Scan` y no amplía
// IAM: la partición `USER#*` ya está autorizada por la política del runtime.
func montarE035(d *dependenciasOleada2) {
	if d == nil || d.Router == nil {
		return
	}
	servicio, tokens, autorizador, limitador, logger := dependenciasRecurrentes(d)
	handler := nexumhttp.NewRecurringPatchHandler(servicio, tokens, autorizador, limitador, logger)
	d.Router.HandleFunc(http.MethodPatch, "/recurring-payments/{recurringPaymentId}", handler.ServeHTTP)
}
