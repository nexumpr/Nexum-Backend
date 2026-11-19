package main

import (
	"net/http"

	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

func init() {
	registrarOleada2(montajeOleada2{
		ID:     "E034",
		Metodo: http.MethodGet,
		Ruta:   "/recurring-payments/{recurringPaymentId}",
		Montar: montarE034,
	})
}

// montarE034 conecta `GET /api/v1/recurring-payments/{recurringPaymentId}` (E034).
//
// Lee `USER#<id>/RECURRING#<id>` con consistencia fuerte y responde el recurso con su `ETag`.
// No crea recursos, no usa `Scan` y no amplía IAM: la partición `USER#*` ya está autorizada
// por la política del runtime. La ruta es una plantilla del router: `recurringPaymentId` se
// expone al handler con `SetPathValue`, nunca por query ni por cabecera.
func montarE034(d *dependenciasOleada2) {
	if d == nil || d.Router == nil {
		return
	}
	servicio, tokens, autorizador, limitador, logger := dependenciasRecurrentes(d)
	handler := nexumhttp.NewRecurringGetHandler(servicio, tokens, autorizador, limitador, logger)
	d.Router.HandleFunc(http.MethodGet, "/recurring-payments/{recurringPaymentId}", handler.ServeHTTP)
}
