package main

import (
	"log/slog"
	"net/http"
	"time"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appContrib "github.com/nexumpr/Nexum-Backend/internal/application/contributions"
	cacheinfra "github.com/nexumpr/Nexum-Backend/internal/infrastructure/cache"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// cuotaPreviewAportacionesPorMinuto es la cuota distribuida por usuario de E043. Sin Redis el
// limitador es nil y no se emite 429 (no se simula una cuota inexistente).
const cuotaPreviewAportacionesPorMinuto int64 = 60

func init() {
	registrarOleada3(montajeOleada3{
		ID:     "E043",
		Metodo: http.MethodPost,
		Ruta:   "/goal-contributions/preview",
		Montar: montarE043,
	})
}

// montarE043 conecta la previsualizacion de aportaciones (E043).
//
// Fail-closed: sin cliente DynamoDB o sin tabla el almacen falla cerrado y el handler responde
// 503; sin proveedor de tokens JWT o sin autorizador de sesiones tambien responde 503 y nunca
// autoriza solo con la firma. E043 no escribe: solo lee las metas afectadas por `GetItem` con
// consistencia fuerte, claves `USER#<id>` que la politica IAM vigente ya autoriza. No se crean
// recursos, tablas ni indices nuevos y no se usa `Scan`.
func montarE043(d *dependenciasOleada3) {
	if d == nil || d.Router == nil {
		return
	}
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	cliente, ok := oleada3DynamoCliente(d)
	if !ok || cliente == nil || d.Tabla == "" {
		logger.Warn("E043: persistencia no configurada; el endpoint respondera 503",
			slog.String("motivo", "dynamodb_no_configurado"))
	}
	servicio := appContrib.NewService(dynamoAdapter.NewContributionsStore(cliente, d.Tabla))
	var limitador nexumhttp.LimitadorRuta
	if d.Redis != nil {
		limitador = cacheinfra.NewAuthRateLimiter(d.Redis, cuotaPreviewAportacionesPorMinuto, time.Minute)
	}
	handler := nexumhttp.NewContributionPreviewHandler(servicio, d.Tokens, d.Authorizer, limitador, logger)
	d.Router.HandleFunc(http.MethodPost, "/goal-contributions/preview", handler.ServeHTTP)
}
