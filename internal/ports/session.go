package ports

import (
	"context"
	"errors"
)

var (
	// ErrSessionUnauthorized indica que la sesión referenciada por el token no es utilizable:
	// no existe, no pertenece al sujeto del JWT, fue revocada (por logout, por reutilización
	// de refresh o por borrado de cuenta) o ya expiró.
	//
	// Es un error único y deliberadamente indistinguible: el middleware responde 401 sin
	// revelar si la sesión existe, a quién pertenece o por qué dejó de ser válida.
	ErrSessionUnauthorized = errors.New("sesion no autorizada")

	// ErrSessionUnavailable indica que no fue posible determinar el estado autoritativo de la
	// sesión (DynamoDB inaccesible, tabla no configurada o error de servicio). El middleware
	// falla cerrado con 503: nunca deja pasar una petición sin verificación de revocación.
	ErrSessionUnavailable = errors.New("estado de sesion no disponible")
)

// SessionAuthorizer verifica el estado autoritativo de la sesión asociada a un access token
// ya validado criptográficamente (E004, GOV-06).
//
// Contrato:
//   - Recibe exclusivamente `sub`, `sid` y `epo` extraídos de claims verificados; nunca datos
//     del cuerpo, cabeceras libres o parámetros controlados por el cliente.
//   - Devuelve nil sólo si la sesión existe, pertenece al sujeto, está ACTIVE, no ha expirado
//     y la época de revocación (`epo` del token frente a `sessionEpoch` del perfil) coincide.
//   - Devuelve ErrSessionUnauthorized para cualquier estado no utilizable, incluida una época
//     desalineada tras un cambio de contraseña (A6, RFC-016 §17.3).
//   - Devuelve ErrSessionUnavailable cuando no puede determinarse el estado.
//
// La revocación debe ser observable de inmediato por cualquier instancia: la implementación
// autoritativa es DynamoDB con lectura consistente. Una caché puede ayudar al rendimiento,
// pero nunca puede autorizar una sesión que ya fue revocada (A-080 §2.6).
type SessionAuthorizer interface {
	AuthorizeSession(ctx context.Context, userID, sessionID string, epoch int64) error
}
