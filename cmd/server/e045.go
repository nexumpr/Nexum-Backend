package main

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appContrib "github.com/nexumpr/Nexum-Backend/internal/application/contributions"
	cacheinfra "github.com/nexumpr/Nexum-Backend/internal/infrastructure/cache"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

func init() {
	registrarOleada3(montajeOleada3{
		ID:     "E045",
		Metodo: http.MethodGet,
		Ruta:   "/goal-contributions",
		Montar: montarE045,
	})
}

// dependenciasAportaciones construye el servicio de aportaciones y el limitador que comparten
// E045, E046 y E047. Reutiliza el cliente DynamoDB, la tabla, Redis y la clave HMAC ya
// existentes en el arranque: no se crean recursos, tablas, índices ni secretos nuevos.
//
// Fail-closed:
//   - sin cliente DynamoDB o sin tabla el almacén queda nulo y toda operación responde 503;
//   - sin clave HMAC base el listado de E045 responde 503 en lugar de paginar sin firmar;
//   - la clave de cursor se deriva con separación de dominio (`e045`), de modo que los cursores
//     de otros endpoints no son intercambiables.
func dependenciasAportaciones(d *dependenciasOleada3, logger *slog.Logger) (*appContrib.Service, nexumhttp.LimitadorRuta) {
	if d == nil {
		return nil, nil
	}
	if logger == nil {
		logger = slog.Default()
	}

	var store appContrib.Store
	if cliente, ok := oleada3DynamoCliente(d); ok && cliente != nil && strings.TrimSpace(d.Tabla) != "" {
		store = dynamoAdapter.NewContributionsStore(cliente, d.Tabla)
	} else {
		logger.Warn("contributions: sin persistencia; los endpoints responderan 503",
			slog.String("motivo", "sin_cliente_dynamodb_o_tabla"))
	}

	servicio := appContrib.NewService(store)
	claveBase, errClave := cacheinfra.LoadLoginHMACKey()
	if errClave != nil {
		logger.Error("contributions: clave HMAC base no disponible; el listado responderá 503",
			slog.String("error_class", "cursor_key_unavailable"))
	} else if secreto := httputil.DeriveCursorKey(claveBase, "e045"); len(secreto) > 0 {
		servicio = servicio.WithCursorKey(secreto)
	}

	var limitador nexumhttp.LimitadorRuta
	if d.Redis != nil {
		limitador = cacheinfra.NewAuthRateLimiter(d.Redis, nexumhttp.CuotaAportacionesPorMinuto, time.Minute)
	}
	return servicio, limitador
}

// montarE045 conecta el listado paginado de aportaciones propias (E045).
//
// La consulta se resuelve con `Query` sobre `USER#<sub>` ordenada newest-first; nunca `Scan` y
// nunca la partición de otro usuario. El cursor es opaco, firmado con HMAC y ligado al usuario y
// a la firma exacta de filtros.
func montarE045(d *dependenciasOleada3) {
	if d == nil || d.Router == nil {
		return
	}
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	servicio, limitador := dependenciasAportaciones(d, logger)
	if d.Tokens == nil || d.Authorizer == nil {
		logger.Warn("E045: componentes de autenticacion incompletos; el endpoint respondera 503",
			slog.String("motivo", "sin_credenciales_o_autorizador"))
	}
	handler := nexumhttp.NewContributionListHandler(servicio, d.Tokens, d.Authorizer, limitador, logger)
	d.Router.HandleFunc(http.MethodGet, "/goal-contributions", handler.ServeHTTP)
}
