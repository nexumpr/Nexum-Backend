package http

import (
	"net/http"

	appGoals "github.com/nexumpr/Nexum-Backend/internal/application/goals"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
)

// Utilidades compartidas por los handlers de lectura de metas (E038 y E040).
//
// Se mantienen en un único lugar para que ambas rutas exijan EXACTAMENTE la misma cadena de
// autenticación: Bearer RS256 verificado y estado autoritativo de la sesión en DynamoDB. Una
// divergencia entre las dos rutas sería un fallo de autorización, no un detalle de estilo.

// cadenaMetas compone la autenticación con estado autoritativo de sesión. Devuelve nil cuando
// falta un componente obligatorio: el handler responde 503 en lugar de autorizar sólo con la
// firma o servir datos sin persistencia.
func cadenaMetas(
	service *appGoals.Service,
	tokens ports.TokenProvider,
	sessions ports.SessionAuthorizer,
	serve http.HandlerFunc,
) http.Handler {
	if service == nil || tokens == nil || sessions == nil {
		return nil
	}
	return middleware.AuthenticateBearerWithSession(tokens, sessions)(http.HandlerFunc(serve))
}
