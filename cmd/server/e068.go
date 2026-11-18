package main

import (
	"log/slog"
	"net/http"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appPreferences "github.com/nexumpr/Nexum-Backend/internal/application/preferences"
	"github.com/nexumpr/Nexum-Backend/internal/infrastructure/security"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

func init() {
	registrarOleada1(montajeOleada{
		ID:     "E068",
		Metodo: http.MethodGet,
		Ruta:   "/users/me/preferences",
		Montar: montarE068,
	})
}

// montarE068 conecta la consulta de preferencias personales (E068).
//
// Fail-closed: sin persistencia, sin proveedor de tokens JWT o sin autorizador de sesiones, el
// handler responde 503 y no se sirve nada. La ruta reutiliza el cliente DynamoDB, la tabla y el
// logger ya construidos en main; no crea recursos ni clientes nuevos.
func montarE068(d *dependenciasOleada) {
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}

	var api dynamoAdapter.PreferencesAPI
	if d.Dynamo != nil {
		api = d.Dynamo
	}

	var servicio *appPreferences.Service
	if api != nil && d.Tabla != "" {
		servicio = appPreferences.NewService(dynamoAdapter.NewPreferencesStore(api, d.Tabla))
	}

	var tokens ports.TokenProvider
	if proveedor, err := security.LoadTokenProvider(); err != nil {
		logger.Error("E068: clave de firma JWT no disponible; el endpoint respondera 503",
			slog.String("error_class", "signing_key_unavailable"))
	} else if proveedor != nil {
		tokens = proveedor
	}

	// Sesión autoritativa: el mismo almacén real de E004/E007/E008. Una sesión revocada en otro
	// nodo deja de autorizar de inmediato (lectura consistente en DynamoDB).
	var autorizador ports.SessionAuthorizer
	if d.Dynamo != nil && d.Tabla != "" {
		autorizador = dynamoAdapter.NewLogoutStore(d.Dynamo, d.Tabla)
	}

	if servicio == nil || tokens == nil || autorizador == nil {
		logger.Warn("E068: componentes incompletos; el endpoint respondera 503",
			slog.String("motivo", "sin_persistencia_credenciales_o_autorizador"))
	}

	handler := nexumhttp.NewPreferencesHandler(servicio, tokens, autorizador, logger)
	d.Router.HandleFunc(http.MethodGet, "/users/me/preferences", handler.ServeHTTP)
}
