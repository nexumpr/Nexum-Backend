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
		ID:     "E069",
		Metodo: http.MethodPatch,
		Ruta:   "/users/me/preferences",
		Montar: montarE069,
	})
}

// montarE069 conecta la modificación de preferencias personales (E069).
//
// Fail-closed: sin persistencia, sin proveedor de tokens JWT o sin autorizador de sesiones, el
// handler responde 503 y no se escribe nada. La ruta reutiliza el cliente DynamoDB, la tabla y
// el logger ya construidos en main; no crea recursos ni clientes nuevos. Comparte ruta con E068
// (GET) y se registra por método, sin duplicar el handler de lectura.
func montarE069(d *dependenciasOleada) {
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
		logger.Error("E069: clave de firma JWT no disponible; el endpoint respondera 503",
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
		logger.Warn("E069: componentes incompletos; el endpoint respondera 503",
			slog.String("motivo", "sin_persistencia_credenciales_o_autorizador"))
	}

	handler := nexumhttp.NewPreferencesPatchHandler(servicio, tokens, autorizador, logger)
	d.Router.HandleFunc(http.MethodPatch, "/users/me/preferences", handler.ServeHTTP)
}
