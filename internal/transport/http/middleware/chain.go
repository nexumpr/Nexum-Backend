package middleware

import "net/http"

// Middleware representa una función decoradora de http.Handler (A-030 §2.3).
type Middleware func(http.Handler) http.Handler

// Chain encadena múltiples middlewares en orden secuencial de ejecución:
// Chain(handler, m1, m2, m3) ejecutará m1 -> m2 -> m3 -> handler.
func Chain(h http.Handler, middlewares ...Middleware) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}
