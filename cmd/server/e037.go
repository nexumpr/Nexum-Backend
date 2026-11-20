package main

import (
	"log/slog"
	"net/http"
	"time"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appRecurring "github.com/nexumpr/Nexum-Backend/internal/application/recurring"
	cacheinfra "github.com/nexumpr/Nexum-Backend/internal/infrastructure/cache"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// cuotaPagosRecurrentesPorMinuto es la cuota distribuida por usuario de E037. Sin Redis el
// limitador es nil y no se emite 429 (no se simula una cuota inexistente).
const cuotaPagosRecurrentesPorMinuto int64 = 60

func init() {
	registrarOleada3(montajeOleada3{
		ID:     "E037",
		Metodo: http.MethodPost,
		Ruta:   "/recurring-payments/{recurringPaymentId}/payments",
		Montar: montarE037,
	})
}

// montarE037 conecta el pago de una regla recurrente. Sin cliente DynamoDB o sin tabla el
// servicio queda con almacén nulo y el handler responde 503: nunca se escribe sin persistencia.
func montarE037(d *dependenciasOleada3) {
	if d == nil || d.Router == nil {
		return
	}
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	cliente, ok := oleada3DynamoCliente(d)
	if !ok || cliente == nil || d.Tabla == "" {
		logger.Warn("E037: persistencia no configurada; el endpoint respondera 503",
			slog.String("motivo", "dynamodb_no_configurado"))
	}
	servicio := appRecurring.NewPaymentService(dynamoAdapter.NewRecurringPaymentStore(cliente, d.Tabla))
	var limitador nexumhttp.LimitadorRuta
	if d.Redis != nil {
		limitador = cacheinfra.NewAuthRateLimiter(d.Redis, cuotaPagosRecurrentesPorMinuto, time.Minute)
	}
	handler := nexumhttp.NewRecurringPaymentHandler(servicio, d.Tokens, d.Authorizer, limitador, logger)
	d.Router.HandleFunc(http.MethodPost, "/recurring-payments/{recurringPaymentId}/payments", handler.ServeHTTP)
}
