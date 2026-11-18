package main

import (
	"log/slog"
	"net/http"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appProfile "github.com/nexumpr/Nexum-Backend/internal/application/profile"
	"github.com/nexumpr/Nexum-Backend/internal/infrastructure/security"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

func init() {
	registrarOleada1(montajeOleada{
		ID:     "E013",
		Metodo: http.MethodPatch,
		Ruta:   "/users/me",
		Montar: montarE013,
	})
}

// montarE013 conecta la modificación del perfil propio (E013).
//
// Fail-closed, con el mismo criterio que E012: sin persistencia, sin proveedor de tokens JWT
// o sin autorizador de sesiones, el handler responde 503 y no se modifica ningún perfil. No
// se registran dependencias simuladas ni se reutiliza un almacén en memoria: la composición
// productiva usa DynamoDB.
//
// La escritura sólo toca `USER#<id>/PROFILE`, una clave que la política IAM vigente ya
// autoriza (`USER#*`). No se crean recursos ni permisos nuevos; la cuota distribuida (Redis)
// pertenece a una unidad con Gate propio y no se simula aquí.
func montarE013(d *dependenciasOleada) {
	if d == nil || d.Router == nil {
		return
	}
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}

	var lecturaAPI dynamoAdapter.ProfileAPI
	var escrituraAPI dynamoAdapter.ProfileUpdateAPI
	if d.Dynamo != nil {
		lecturaAPI = d.Dynamo
		escrituraAPI = d.Dynamo
	}
	lectura := dynamoAdapter.NewProfileStore(lecturaAPI, d.Tabla)
	escritura := dynamoAdapter.NewProfileUpdateStore(escrituraAPI, d.Tabla)
	servicio := appProfile.NewService(lectura).WithUpdateStore(escritura)

	// La ruta exige Bearer RS256 y estado autoritativo de la sesión en DynamoDB: sin
	// proveedor de tokens o sin autorizador el endpoint responde 503 en vez de autorizar
	// sólo con la firma (mismo criterio que E004/E012).
	var verificadorTokens ports.TokenProvider
	proveedorTokens, errTokens := security.LoadTokenProvider()
	if errTokens != nil {
		logger.Warn("E013: clave de firma JWT no disponible; el endpoint respondera 503",
			slog.String("motivo", "signing_key_unavailable"))
	} else if proveedorTokens != nil {
		verificadorTokens = proveedorTokens
	}

	var autorizadorSesiones ports.SessionAuthorizer
	if d.Dynamo != nil && d.Tabla != "" {
		autorizadorSesiones = dynamoAdapter.NewLogoutStore(d.Dynamo, d.Tabla)
	}

	if verificadorTokens == nil || autorizadorSesiones == nil || d.Dynamo == nil || d.Tabla == "" {
		logger.Warn("E013: componentes incompletos; el endpoint respondera 503",
			slog.String("motivo", "sin_persistencia_credenciales_o_autorizador"))
	}

	handler := nexumhttp.NewProfileHandler(servicio, verificadorTokens, autorizadorSesiones, logger)
	d.Router.HandleFunc(http.MethodPatch, "/users/me", handler.ServeHTTP)
}
