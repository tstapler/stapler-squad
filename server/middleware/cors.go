package middleware

import (
	"net/http"
)

// allowedCORSHeaders are the headers permitted in CORS requests.
const allowedCORSHeaders = "Content-Type, Connect-Protocol-Version, Connect-Timeout-Ms, Authorization"

// CORS middleware adds Cross-Origin Resource Sharing headers.
// Uses the request Origin so it works for both localhost development and
// remote access (the client and server share the same origin in production).
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			// Same-origin request or no browser – skip CORS headers.
			next.ServeHTTP(w, r)
			return
		}

		// Echo the request origin back so credentials work correctly.
		// Access-Control-Allow-Origin must be a specific origin (not "*") when
		// Access-Control-Allow-Credentials is "true".
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, "+http.MethodOptions)
		w.Header().Set("Access-Control-Allow-Headers", allowedCORSHeaders)
		w.Header().Set("Access-Control-Expose-Headers", "Connect-Protocol-Version, Connect-Timeout-Ms")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Max-Age", "86400") // 24 hours
		w.Header().Set("Vary", "Origin")

		// Handle preflight OPTIONS request
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// CORSWithOrigins creates CORS middleware with specific allowed origins.
// Use this in production with your actual domain.
func CORSWithOrigins(allowedOrigins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Check if origin is allowed
			allowed := false
			for _, o := range allowedOrigins {
				if o == origin {
					allowed = true
					break
				}
			}

			if allowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, "+http.MethodOptions)
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Connect-Protocol-Version, Connect-Timeout-Ms")
				w.Header().Set("Access-Control-Expose-Headers", "Connect-Protocol-Version, Connect-Timeout-Ms")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Max-Age", "86400")
			}

			// Handle preflight OPTIONS request
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
