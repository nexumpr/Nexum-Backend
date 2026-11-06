package middleware

import (
	"context"
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
func AuthenticateBearer(tokenProvider ports.TokenProvider) Middleware {
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
				if err == ports.ErrTokenExpired {
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

			// Inyectar claims verificados en el contexto (A-050 §2.6)
			ctx := context.WithValue(r.Context(), UserIDKey, claims.Subject)
			ctx = context.WithValue(ctx, SessionIDKey, claims.SessionID)
			ctx = context.WithValue(ctx, RolesKey, claims.Roles)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
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

