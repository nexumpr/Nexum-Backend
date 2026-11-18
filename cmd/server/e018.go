package main

import (
	"log/slog"
	"net/http"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appOnboarding "github.com/nexumpr/Nexum-Backend/internal/application/onboarding"
	"github.com/nexumpr/Nexum-Backend/internal/infrastructure/security"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

func init() {
	registrarOleada1(montajeOleada{
		ID:     "E018",
		Metodo: http.MethodGet,
		Ruta:   "/users/me/onboarding",
		Montar: montarE018,
	})
}

// montarE018 conecta la lectura del estado de onboarding del titular (E018).
//
// La ruta exige JWT RS256 y sesión autoritativa: se reutiliza el mismo proveedor de
// tokens y el mismo autorizador de sesiones de E004/E008, sin claves ni almacenes
// nuevos. Fail-closed: sin persistencia, sin firma o sin autorizador, el handler
// responde 503 en lugar de servir un estado ficticio.
func montarE018(d *dependenciasOleada) {
	handler := nuevoHandlerOnboarding(d)
	d.Router.HandleFunc(http.MethodGet, "/users/me/onboarding", handler.ServeGet)
}

// nuevoHandlerOnboarding compone el handler de E018/E019 con las dependencias reales
// ya construidas en cmd/server/main.go. No crea recursos AWS, claves ni conexiones:
// sólo envuelve el cliente DynamoDB y el material JWT existentes.
func nuevoHandlerOnboarding(d *dependenciasOleada) *nexumhttp.OnboardingHandler {
	if d == nil {
		return nexumhttp.NewOnboardingHandler(nil, nil, nil, slog.Default())
	}

	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}

	var api dynamoAdapter.OnboardingAPI
	if d.Dynamo != nil {
		api = d.Dynamo
	}
	store := dynamoAdapter.NewOnboardingStore(api, d.Tabla)
	servicio := appOnboarding.NewService(store)

	// El autorizador reutiliza el almacén real de E004, que exige el subconjunto
	// `DynamoAPI` (GetItem + TransactWriteItems) del mismo cliente.
	var apiAutorizador dynamoAdapter.DynamoAPI
	if d.Dynamo != nil {
		apiAutorizador = d.Dynamo
	}

	var tokens ports.TokenProvider
	proveedor, errTokens := security.LoadTokenProvider()
	if errTokens != nil {
		logger.Error("E018/E019: clave de firma JWT no disponible; el endpoint respondera 503",
			slog.String("error_class", "signing_key_unavailable"))
	} else if proveedor != nil {
		tokens = proveedor
	}

	// El autorizador es el almacén real de E004: lee la sesión autoritativa con
	// consistencia fuerte, de modo que una sesión revocada en otro nodo deja de
	// autorizar en el acto.
	var autorizador ports.SessionAuthorizer
	if apiAutorizador != nil && d.Tabla != "" {
		autorizador = dynamoAdapter.NewLogoutStore(apiAutorizador, d.Tabla)
	}

	if api == nil || d.Tabla == "" || tokens == nil || autorizador == nil {
		logger.Warn("E018/E019: componentes incompletos; el endpoint respondera 503",
			slog.String("motivo", "sin_persistencia_credenciales_o_autorizador"))
	}

	return nexumhttp.NewOnboardingHandler(servicio, tokens, autorizador, logger)
}
