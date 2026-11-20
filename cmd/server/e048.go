package main

import (
	"log/slog"
	"net/http"
	"time"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appBudget "github.com/nexumpr/Nexum-Backend/internal/application/budgetset"
	cacheinfra "github.com/nexumpr/Nexum-Backend/internal/infrastructure/cache"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

const cuotaPresupuestosPorMinuto int64 = 120

func init() {
	registrarOleada3(montajeOleada3{
		ID:     "E048",
		Metodo: http.MethodGet,
		Ruta:   "/budgets",
		Montar: montarE048,
	})
}

// montarE048 conecta el listado de presupuestos. Sin cliente DynamoDB o tabla responde 503.
func montarE048(d *dependenciasOleada3) {
	if d == nil || d.Router == nil {
		return
	}
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	cliente, ok := oleada3DynamoCliente(d)
	if !ok || cliente == nil || d.Tabla == "" {
		logger.Warn("E048: persistencia no configurada; el endpoint respondera 503",
			slog.String("motivo", "dynamodb_no_configurado"))
	}
	servicio := appBudget.NewService(dynamoAdapter.NewBudgetSetStore(cliente, d.Tabla))
	if base, err := cacheinfra.LoadLoginHMACKey(); err != nil || len(base) == 0 {
		logger.Warn("E048: clave HMAC base no disponible; el listado respondera 503",
			slog.String("motivo", "cursor_key_unavailable"))
	} else if secreto := httputil.DeriveCursorKey(base, "e048"); len(secreto) > 0 {
		servicio = servicio.WithCursorKey(secreto)
	}
	var limitador nexumhttp.LimitadorRuta
	if d.Redis != nil {
		limitador = cacheinfra.NewAuthRateLimiter(d.Redis, cuotaPresupuestosPorMinuto, time.Minute)
	}
	handler := nexumhttp.NewBudgetsListHandler(servicio, d.Tokens, d.Authorizer, limitador, logger)
	d.Router.HandleFunc(http.MethodGet, "/budgets", handler.ServeHTTP)
}
