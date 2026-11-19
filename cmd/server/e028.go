package main

import (
	"log/slog"
	"net/http"
	"time"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
	cacheinfra "github.com/nexumpr/Nexum-Backend/internal/infrastructure/cache"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// cuotaMovimientosPorMinuto es la cuota distribuida por usuario de la familia de movimientos.
// Es la misma cifra que usa la Oleada 1 y sólo se aplica si Redis está compuesto: sin Redis el
// limitador es nil y no se emite 429 (no se simula una cuota inexistente).
const cuotaMovimientosPorMinuto int64 = 120

func init() {
	registrarOleada2(montajeOleada2{
		ID:     "E028",
		Metodo: http.MethodPost,
		Ruta:   "/movements",
		Montar: montarE028,
	})
}

// dependenciasMovimientos construye lo que comparten E027/E028 (y reutilizarán E029-E031): el
// servicio con el almacén DynamoDB real, el material JWT, el autorizador de sesión y el
// limitador, todos ya construidos por `main`.
//
// Fail-closed: sin cliente DynamoDB o sin tabla, el almacén queda nulo y el handler responde 503.
// Sin la clave HMAC base no se puede derivar la clave de firma de cursores y el listado responde
// 503: nunca se emiten cursores sin firmar ni se acepta uno no verificado.
func dependenciasMovimientos(
	d *dependenciasOleada2,
) (*appMovements.Service, ports.TokenProvider, ports.SessionAuthorizer, nexumhttp.LimitadorRuta, *slog.Logger) {
	if d == nil {
		return nil, nil, nil, nil, slog.Default()
	}
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}

	var api dynamoAdapter.MovementsAPI
	cliente, okCliente := oleada2DynamoCliente(d)
	if okCliente && cliente != nil && d.Tabla != "" {
		api = cliente
	} else {
		logger.Warn("movimientos: persistencia no configurada; los endpoints responderan 503",
			slog.String("motivo", "dynamodb_no_configurado"))
	}

	servicio := appMovements.NewService(dynamoAdapter.NewMovementsStore(api, d.Tabla))

	claveBase, errClave := cacheinfra.LoadLoginHMACKey()
	if errClave != nil || len(claveBase) == 0 {
		logger.Warn("movimientos: clave HMAC base no disponible; el listado respondera 503",
			slog.String("motivo", "cursor_key_unavailable"))
	} else if secreto := httputil.DeriveCursorKey(claveBase, "e027"); len(secreto) > 0 {
		// Clave derivada con separación de dominio: no se introduce un secreto nuevo y la clave
		// de cursores no es utilizable como la de cuotas ni al revés.
		servicio = servicio.WithCursorKey(secreto)
	}

	var limitador nexumhttp.LimitadorRuta
	if d.Redis != nil {
		limitador = cacheinfra.NewAuthRateLimiter(d.Redis, cuotaMovimientosPorMinuto, time.Minute)
	}

	return servicio, d.Tokens, d.Authorizer, limitador, logger
}

// montarE028 conecta `POST /api/v1/movements` (E028).
//
// La ruta exige Bearer RS256 y estado autoritativo de la sesión en DynamoDB: sin proveedor de
// tokens o sin autorizador el handler responde 503 en vez de autorizar sólo con la firma.
func montarE028(d *dependenciasOleada2) {
	if d == nil || d.Router == nil {
		return
	}
	servicio, tokens, autorizador, limitador, logger := dependenciasMovimientos(d)
	handler := nexumhttp.NewMovementsHandler(servicio, tokens, autorizador, limitador, logger)
	d.Router.HandleFunc(http.MethodPost, "/movements", handler.ServeHTTP)
}
