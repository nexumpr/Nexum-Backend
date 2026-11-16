package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

type contextKey string

const (
	// UserIDKey clave de contexto para el identificador del usuario autenticado (sub).
	UserIDKey contextKey = "nexum_user_id"
	// SessionIDKey clave de contexto para el identificador de sesión activa (sid).
	SessionIDKey contextKey = "nexum_session_id"
	// RolesKey clave de contexto para los roles del usuario.
	RolesKey contextKey = "nexum_roles"
)

// AuthenticateBearer crea un middleware HTTP que verifica tokens Bearer JWT RS256 (A-050 §2.2).
//
// Cuando se proporciona un `ports.SessionAuthorizer`, después de validar la firma y la
// expiración se comprueba el estado autoritativo de la sesión (E004): una sesión revocada deja
// de autorizar en el acto, en cualquier instancia. Sin autorizador el comportamiento es el
// heredado (sólo criptografía), de modo que los llamadores existentes siguen compilando.
//
// Política de fallo: si el estado de la sesión no puede determinarse, la respuesta es 503. Un
// error de dependencia nunca se degrada a "sesión válida".
func AuthenticateBearer(tokenProvider ports.TokenProvider, authorizers ...ports.SessionAuthorizer) Middleware {
	var authorizer ports.SessionAuthorizer
	if len(authorizers) > 0 {
		authorizer = authorizers[0]
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				_ = httputil.WriteErrorEnvelope(
					w,
					http.StatusUnauthorized,
					"UNAUTHORIZED",
					"cabecera de autorizacion requerida",
					r.Header.Get("X-Trace-ID"),
					nil,
				)
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				_ = httputil.WriteErrorEnvelope(
					w,
					http.StatusUnauthorized,
					"INVALID_TOKEN_FORMAT",
					"formato de autorizacion invalido: se requiere 'Bearer <token>'",
					r.Header.Get("X-Trace-ID"),
					nil,
				)
				return
			}

			tokenStr := strings.TrimSpace(parts[1])
			claims, err := tokenProvider.ValidateAccessToken(r.Context(), tokenStr)
			if err != nil {
				code := "INVALID_TOKEN"
				if errors.Is(err, ports.ErrTokenExpired) {
					code = "TOKEN_EXPIRED"
				}
				_ = httputil.WriteErrorEnvelope(
					w,
					http.StatusUnauthorized,
					code,
					"credenciales invalidas o token expirado",
					r.Header.Get("X-Trace-ID"),
					nil,
				)
				return
			}
			// Un JWT verificado debe identificar sujeto y sesión: sin ambos no hay recurso al
			// que aplicar autorización por propietario (R-031).
			if claims == nil || strings.TrimSpace(claims.Subject) == "" || strings.TrimSpace(claims.SessionID) == "" {
				_ = httputil.WriteErrorEnvelope(
					w,
					http.StatusUnauthorized,
					"INVALID_TOKEN",
					"credenciales invalidas o token expirado",
					r.Header.Get("X-Trace-ID"),
					nil,
				)
				return
			}

			if authorizer != nil {
				if errSesion := authorizer.AuthorizeSession(r.Context(), claims.Subject, claims.SessionID); errSesion != nil {
					if errors.Is(errSesion, ports.ErrSessionUnauthorized) {
						_ = httputil.WriteErrorEnvelope(
							w,
							http.StatusUnauthorized,
							"UNAUTHORIZED",
							"sesion no valida",
							r.Header.Get("X-Trace-ID"),
							nil,
						)
						return
					}
					// Incluye ErrSessionUnavailable y cualquier error no clasificado: se falla
					// cerrado con 503 y no se revela el motivo.
					_ = httputil.WriteErrorEnvelope(
						w,
						http.StatusServiceUnavailable,
						"DEPENDENCY_UNAVAILABLE",
						"servicio no disponible",
						r.Header.Get("X-Trace-ID"),
						nil,
					)
					return
				}
			}

			// Inyectar claims verificados en el contexto (A-050 §2.6)
			ctx := context.WithValue(r.Context(), UserIDKey, claims.Subject)
			ctx = context.WithValue(ctx, SessionIDKey, claims.SessionID)
			ctx = context.WithValue(ctx, RolesKey, claims.Roles)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AuthenticateBearerWithSession exige un autorizador de sesión (E004).
//
// Es la forma obligatoria para rutas protegidas de producción: si el autorizador falta, la
// ruta responde 503 en lugar de autorizar sin comprobar revocación. Nunca degrada a la
// verificación criptográfica sola.
func AuthenticateBearerWithSession(tokenProvider ports.TokenProvider, authorizer ports.SessionAuthorizer) Middleware {
	if authorizer == nil {
		return func(http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = httputil.WriteErrorEnvelope(
					w,
					http.StatusServiceUnavailable,
					"DEPENDENCY_UNAVAILABLE",
					"servicio no disponible",
					r.Header.Get("X-Trace-ID"),
					nil,
				)
			})
		}
	}
	return AuthenticateBearer(tokenProvider, authorizer)
}

// UserIDFromContext extrae el userId autenticado de forma segura del contexto.
func UserIDFromContext(ctx context.Context) (string, bool) {
	val, ok := ctx.Value(UserIDKey).(string)
	return val, ok
}

// SessionIDFromContext extrae el sessionId de la sesión activa del contexto.
func SessionIDFromContext(ctx context.Context) (string, bool) {
	val, ok := ctx.Value(SessionIDKey).(string)
	return val, ok
}
