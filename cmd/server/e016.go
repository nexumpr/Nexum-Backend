package main

import (
	"log/slog"
	"net/http"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appPrivacy "github.com/nexumpr/Nexum-Backend/internal/application/privacy"
	cacheinfra "github.com/nexumpr/Nexum-Backend/internal/infrastructure/cache"
	"github.com/nexumpr/Nexum-Backend/internal/infrastructure/security"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

func init() {
	registrarOleada1(montajeOleada{
		ID:     "E016",
		Metodo: http.MethodGet,
		Ruta:   "/users/me/consents",
		Montar: montarE016,
	})
}

// montarE016 conecta el listado paginado del historial de consentimientos propios (E016).
//
// Fail-closed: sin persistencia, sin proveedor de tokens, sin autorizador de sesiones o sin
// clave de firma de cursor, el handler responde 503 y no se lista nada. No se registran
// dependencias simuladas ni se degrada la firma del cursor a un valor por defecto.
//
// La clave de firma de cursor se deriva con HMAC-SHA256 y separación de dominio a partir de la
// clave HMAC ya existente en el bundle RFC-004. Así no se introduce un secreto nuevo —lo que
// exigiría un Gate de infraestructura— y la clave de cursores no es utilizable como la de
// cuotas ni viceversa. El dominio `e016` impide reutilizar un cursor de otro listado.
//
// La lectura sólo toca `USER#<id>/CONSENT#*`, una clave que la política IAM vigente ya autoriza
// (`USER#*`). No se crean recursos ni permisos nuevos.
func montarE016(d *dependenciasOleada) {
	if d == nil || d.Router == nil {
		return
	}
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}

	var api dynamoAdapter.ConsentsAPI
	if d.Dynamo != nil {
		api = d.Dynamo
	}
	store := dynamoAdapter.NewConsentsStore(api, d.Tabla)

	var verificadorTokens ports.TokenProvider
	proveedorTokens, errTokens := security.LoadTokenProvider()
	if errTokens != nil {
		logger.Error("E016: clave de firma JWT no disponible; el endpoint respondera 503",
			slog.String("error_class", "signing_key_unavailable"))
	} else if proveedorTokens != nil {
		verificadorTokens = proveedorTokens
	}

	// La sesión autoritativa es la misma que usa E004: una sesión revocada en otro nodo deja de
	// autorizar en el acto.
	var autorizadorSesiones ports.SessionAuthorizer
	if d.Dynamo != nil && d.Tabla != "" {
		autorizadorSesiones = dynamoAdapter.NewLogoutStore(d.Dynamo, d.Tabla)
	}

	var servicio *appPrivacy.ConsentsService
	claveBase, errClave := cacheinfra.LoadLoginHMACKey()
	if errClave != nil {
		logger.Error("E016: clave HMAC base no disponible; el endpoint respondera 503",
			slog.String("error_class", "cursor_key_unavailable"))
	} else if secreto := httputil.DeriveCursorKey(claveBase, "e016"); len(secreto) > 0 {
		servicio = appPrivacy.NewConsentsService(store, secreto)
	}

	if servicio == nil || verificadorTokens == nil || autorizadorSesiones == nil {
		logger.Warn("E016: componentes incompletos; el endpoint respondera 503",
			slog.String("motivo", "sin_persistencia_credenciales_autorizador_o_clave_cursor"))
	}

	handler := nexumhttp.NewConsentsHandler(servicio, verificadorTokens, autorizadorSesiones, logger)
	d.Router.HandleFunc(http.MethodGet, "/users/me/consents", handler.ServeHTTP)
}
