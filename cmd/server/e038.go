package main

import (
	"log/slog"
	"net/http"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appGoals "github.com/nexumpr/Nexum-Backend/internal/application/goals"
	cacheinfra "github.com/nexumpr/Nexum-Backend/internal/infrastructure/cache"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

func init() {
	registrarOleada2(montajeOleada2{
		ID:     "E038",
		Metodo: http.MethodGet,
		Ruta:   "/goals",
		Montar: montarE038,
	})
}

// montarE038 conecta el listado paginado de metas propias (E038).
//
// Fail-closed: sin persistencia, sin proveedor de tokens, sin autorizador de sesiones o sin
// clave de firma de cursor, el handler responde 503 y no se lista nada. No se registran
// dependencias simuladas ni se degrada la firma del cursor a un valor por defecto.
//
// La clave de firma de cursor se deriva con HMAC-SHA256 y separación de dominio a partir de la
// clave HMAC ya existente en el bundle RFC-004 (la misma que usan las cuotas de login y el
// cursor de E008). Así no se introduce un secreto nuevo —lo que exigiría un Gate de
// infraestructura— y las claves de dominios distintos no son intercambiables.
func montarE038(d *dependenciasOleada2) {
	if d == nil || d.Router == nil {
		return
	}
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}

	var api dynamoAdapter.GoalsAPI
	if cliente, ok := oleada2DynamoCliente(d); ok {
		api = cliente
	}
	store := dynamoAdapter.NewGoalsStore(api, d.Tabla)

	servicio := appGoals.NewService(store)
	claveBase, errClave := cacheinfra.LoadLoginHMACKey()
	if errClave != nil {
		logger.Error("E038: clave HMAC base no disponible; el endpoint respondera 503",
			slog.String("error_class", "cursor_key_unavailable"))
	} else if secreto := httputil.DeriveCursorKey(claveBase, "e038"); len(secreto) > 0 {
		servicio = servicio.WithCursorSecret(secreto)
	}

	if api == nil || d.Tabla == "" {
		logger.Warn("E038: sin persistencia; el endpoint respondera 503",
			slog.String("motivo", "sin_cliente_dynamodb_o_tabla"))
	}
	if d.Tokens == nil || d.Authorizer == nil {
		logger.Warn("E038: componentes de autenticacion incompletos; el endpoint respondera 503",
			slog.String("motivo", "sin_credenciales_o_autorizador"))
	}
	if errClave != nil {
		logger.Warn("E038: sin clave de cursor; el endpoint respondera 503",
			slog.String("motivo", "cursor_key_unavailable"))
	}

	handler := nexumhttp.NewGoalsListHandler(servicio, d.Tokens, d.Authorizer, logger)
	d.Router.HandleFunc(http.MethodGet, "/goals", handler.ServeHTTP)
}
