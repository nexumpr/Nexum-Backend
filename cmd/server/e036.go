package main

import (
	"net/http"

	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

func init() {
	registrarOleada2(montajeOleada2{
		ID:     "E036",
		Metodo: http.MethodDelete,
		Ruta:   "/recurring-payments/{recurringPaymentId}",
		Montar: montarE036,
	})
}

// montarE036 conecta `DELETE /api/v1/recurring-payments/{recurringPaymentId}` (E036).
//
// Da de baja la regla (`active = false`) sin borrar el ítem ni ningún pago: el historial debe
// seguir siendo consultable. No usa `Scan` ni `DeleteItem` y no amplía IAM: la partición
// `USER#*` ya está autorizada por la política del runtime.
func montarE036(d *dependenciasOleada2) {
	if d == nil || d.Router == nil {
		return
	}
	servicio, tokens, autorizador, limitador, logger := dependenciasRecurrentes(d)
	handler := nexumhttp.NewRecurringDeleteHandler(servicio, tokens, autorizador, limitador, logger)
	d.Router.HandleFunc(http.MethodDelete, "/recurring-payments/{recurringPaymentId}", handler.ServeHTTP)
}
