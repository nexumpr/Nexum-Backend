package main

import (
	"log/slog"
	"net/http"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appPrivacy "github.com/nexumpr/Nexum-Backend/internal/application/privacy"
	"github.com/nexumpr/Nexum-Backend/internal/infrastructure/security"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

func init() {
	registrarOleada1(montajeOleada{
		ID:     "E017",
		Metodo: http.MethodPost,
		Ruta:   "/users/me/consents",
		Montar: montarE017,
	})
}

// montarE017 conecta el registro append-only de decisiones de consentimiento (E017).
//
// Fail-closed: sin persistencia, sin proveedor de tokens o sin autorizador de sesiones, el
// handler responde 503 y no se registra nada. No se usan dependencias simuladas ni un almacén
// en memoria como sustituto de DynamoDB.
//
// La escritura sólo toca `USER#<id>/CONSENT#*`, una clave que la política IAM vigente ya
// autoriza (`USER#*`). No se crean recursos ni permisos nuevos, no se modifica E016 y no se
// reescriben los consentimientos históricos de E001.
func montarE017(d *dependenciasOleada) {
	if d == nil || d.Router == nil {
		return
	}
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}

	var api dynamoAdapter.ConsentWriteAPI
	if d.Dynamo != nil {
		api = d.Dynamo
	}
	store := dynamoAdapter.NewConsentWriteStore(api, d.Tabla)

	var verificadorTokens ports.TokenProvider
	proveedorTokens, errTokens := security.LoadTokenProvider()
	if errTokens != nil {
		logger.Error("E017: clave de firma JWT no disponible; el endpoint respondera 503",
			slog.String("error_class", "signing_key_unavailable"))
	} else if proveedorTokens != nil {
		verificadorTokens = proveedorTokens
	}

	// La sesión autoritativa es la misma que usan E004 y E016: una sesión revocada en otro nodo
	// deja de autorizar en el acto.
	var autorizadorSesiones ports.SessionAuthorizer
	if d.Dynamo != nil && d.Tabla != "" {
		autorizadorSesiones = dynamoAdapter.NewLogoutStore(d.Dynamo, d.Tabla)
	}

	var servicio *appPrivacy.ConsentsWriteService
	if d.Dynamo != nil && d.Tabla != "" {
		servicio = appPrivacy.NewConsentsWriteService(store)
	}

	if servicio == nil || verificadorTokens == nil || autorizadorSesiones == nil {
		logger.Warn("E017: componentes incompletos; el endpoint respondera 503",
			slog.String("motivo", "sin_persistencia_credenciales_o_autorizador"))
	}

	handler := nexumhttp.NewConsentsWriteHandler(servicio, verificadorTokens, autorizadorSesiones, logger)
	d.Router.HandleFunc(http.MethodPost, "/users/me/consents", handler.ServeHTTP)
}
