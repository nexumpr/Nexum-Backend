package main

import (
	"log/slog"
	"net/http"

	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

func init() {
	registrarOleada3(montajeOleada3{
		ID:     "E047",
		Metodo: http.MethodPost,
		Ruta:   "/goal-contributions/{contributionId}/reverse",
		Montar: montarE047,
	})
}

// montarE047 conecta la reversión exacta de una aportación propia (E047).
//
// La operación exige `Idempotency-Key`, revierte aportación y metas en una sola transacción
// condicionada y nunca borra historia financiera: el estado pasa a `reversed` y la versión se
// incrementa. Una segunda reversión responde 409; una repetición idéntica devuelve la misma
// respuesta con `X-Idempotent-Replay: true`.
func montarE047(d *dependenciasOleada3) {
	if d == nil || d.Router == nil {
		return
	}
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	servicio, limitador := dependenciasAportaciones(d, logger)
	if d.Tokens == nil || d.Authorizer == nil {
		logger.Warn("E047: componentes de autenticacion incompletos; el endpoint respondera 503",
			slog.String("motivo", "sin_credenciales_o_autorizador"))
	}
	handler := nexumhttp.NewContributionReverseHandler(servicio, d.Tokens, d.Authorizer, limitador, logger)
	d.Router.HandleFunc(http.MethodPost, "/goal-contributions/{contributionId}/reverse", handler.ServeHTTP)
}
