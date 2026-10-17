package middleware

import "net/http"

// BodyLimit caps the request body of one procedure path (as the wrapped handler sees
// it, after any prefix strip). Reading past max fails with *http.MaxBytesError, which
// connect-go reports as ResourceExhausted before the RPC handler runs. Other paths pass untouched.
func BodyLimit(path string, max int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == path {
				r.Body = http.MaxBytesReader(w, r.Body, max)
			}
			next.ServeHTTP(w, r)
		})
	}
}
