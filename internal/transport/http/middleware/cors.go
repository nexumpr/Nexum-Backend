package middleware

import (
	"net/http"
	"strings"
)

// CORSOptions define la política de Cross-Origin Resource Sharing (A-030 §2.5).
type CORSOptions struct {
	AllowedOrigins []string
	AllowedMethods []string
	AllowedHeaders []string
	MaxAgeSeconds  string
}

// DefaultCORSOptions genera una configuración segura para producción y local.
func DefaultCORSOptions() CORSOptions {
	return CORSOptions{
		AllowedOrigins: []string{"http://localhost:3000", "https://app.nexum.finance"},
		AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Content-Type", "Authorization", "X-Request-ID", "X-Correlation-ID"},
		MaxAgeSeconds:  "86400",
	}
}

// CORS aplica validación estricta de orígenes y resuelve peticiones preflight OPTIONS.
func CORS(opts CORSOptions) Middleware {
	allowedMap := make(map[string]bool, len(opts.AllowedOrigins))
	for _, o := range opts.AllowedOrigins {
		allowedMap[strings.TrimSpace(o)] = true
	}

	methodsStr := strings.Join(opts.AllowedMethods, ", ")
	headersStr := strings.Join(opts.AllowedHeaders, ", ")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Si el origen está en la allowlist, reflejarlo de forma explícita (no wildcard *)
			if origin != "" && allowedMap[origin] {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", methodsStr)
				w.Header().Set("Access-Control-Allow-Headers", headersStr)
				w.Header().Set("Access-Control-Max-Age", opts.MaxAgeSeconds)
				w.Header().Set("Vary", "Origin")
			}

			// Manejo de preflight OPTIONS (A-030 §2.5, AC-A-030-01)
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
