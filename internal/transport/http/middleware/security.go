package middleware

import "net/http"

// SecurityHeaders inyecta cabeceras de endurecimiento HTTP (A-030 §2.4).
func SecurityHeaders(tlsEnabled bool) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("X-XSS-Protection", "0")
			h.Set("Cache-Control", "no-store, no-cache, must-revalidate")
			h.Set("Pragma", "no-cache")

			if tlsEnabled {
				h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}

			next.ServeHTTP(w, r)
		})
	}
}
