package main

import (
	"log/slog"
	"net/http"

	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

func init() {
	registrarOleada3(montajeOleada3{
		ID:     "E046",
		Metodo: http.MethodGet,
		Ruta:   "/goal-contributions/{contributionId}",
		Montar: montarE046,
	})
}

// montarE046 conecta la consulta de una aportación propia (E046).
//
// Resolución O(1) por el localizador interno dentro de `USER#<sub>`: un recurso ajeno y uno
// inexistente producen el mismo 404. No se usa `Scan` ni se consulta la partición de otro
// usuario, y la lectura sólo toca claves que la política IAM vigente ya autoriza.
func montarE046(d *dependenciasOleada3) {
	if d == nil || d.Router == nil {
		return
	}
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	servicio, limitador := dependenciasAportaciones(d, logger)
	if d.Tokens == nil || d.Authorizer == nil {
		logger.Warn("E046: componentes de autenticacion incompletos; el endpoint respondera 503",
			slog.String("motivo", "sin_credenciales_o_autorizador"))
	}
	handler := nexumhttp.NewContributionGetHandler(servicio, d.Tokens, d.Authorizer, limitador, logger)
	d.Router.HandleFunc(http.MethodGet, "/goal-contributions/{contributionId}", handler.ServeHTTP)
}
