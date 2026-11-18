package main

import (
	"log/slog"
	"net/http"
	"os"

	appPrivacy "github.com/nexumpr/Nexum-Backend/internal/application/privacy"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

func init() {
	registrarOleada1(montajeOleada{
		ID:     "E015",
		Metodo: http.MethodGet,
		Ruta:   "/privacy/notices/current",
		Montar: montarE015,
	})
}

// montarE015 conecta el aviso de privacidad público. La fecha efectiva se lee
// una sola vez de E015_EFFECTIVE_AT (bundle RFC-004 del release); no se calcula
// por petición. Si falta o es inválida, el handler responde 503 y el smoke test
// de promoción debe bloquear la release.
func montarE015(d *dependenciasOleada) {
	if d == nil || d.Router == nil {
		return
	}
	proveedor, err := appPrivacy.ProveedorDesdeEntorno(os.Getenv)
	if err != nil {
		if d.Logger != nil {
			d.Logger.Error("E015: fecha efectiva no configurada; el endpoint respondera 503",
				slog.String("variable", appPrivacy.EffectiveAtEnvVar),
				slog.String("error_class", "effective_at_unavailable"))
		}
		proveedor = appPrivacy.Proveedor{}
	}
	d.Router.HandleFunc(http.MethodGet, "/privacy/notices/current", nexumhttp.NewPrivacyNoticeHandler(proveedor, d.Logger).ServeHTTP)
}
