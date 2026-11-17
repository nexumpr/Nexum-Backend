package main

import (
	"log/slog"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// montarE009 conecta la revocacion de una sesion propia (E009).
//
// Fail-closed: sin persistencia el almacen falla cerrado y el handler responde 503; sin
// proveedor de tokens o sin autorizador de sesiones el handler tambien responde 503 y nunca
// autoriza solo con la firma. No se registran dependencias simuladas.
//
// El proveedor de tokens y el autorizador se reciben ya construidos: son los mismos que usan
// E004 y E008 (sesion autoritativa en DynamoDB con consistencia fuerte), de modo que una
// revocacion hecha en un nodo deja de autorizar en el otro en el acto.
func montarE009(
	router registradorHTTP,
	clienteDynamo *dynamodb.Client,
	tokens ports.TokenProvider,
	sessions ports.SessionAuthorizer,
	tabla string,
	logger *slog.Logger,
) {
	if logger == nil {
		logger = slog.Default()
	}

	var api dynamoAdapter.SessionsAPI
	if clienteDynamo != nil {
		api = clienteDynamo
	}
	store := dynamoAdapter.NewSessionsStore(api, tabla)
	servicio := appIdentity.NewSessionRevokeService(store)

	if api == nil || tabla == "" {
		logger.Warn("E009: sin persistencia; el endpoint respondera 503",
			slog.String("motivo", "sin_cliente_dynamodb_o_tabla"))
	}
	if tokens == nil || sessions == nil {
		logger.Warn("E009: componentes de autenticacion incompletos; el endpoint respondera 503",
			slog.String("motivo", "sin_credenciales_autorizador_o_persistencia"))
	}

	handler := nexumhttp.NewSessionRevokeHandler(servicio, tokens, sessions, logger)
	router.HandleFunc(http.MethodDelete, "/auth/sessions/{sessionId}", handler.ServeHTTP)
}
