package middleware

import (
	"net/http"
	"strings"
)

// AuthValidator is the minimal interface the auth middleware needs from the
// session manager.
type AuthValidator interface {
	ValidateAuthSession(token string) bool
}

// Auth returns middleware that enforces authentication on all non-exempt paths.
// When auth is nil (auth disabled), the middleware is a no-op pass-through.
func Auth(validator AuthValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if validator == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Always allow auth endpoints and static assets needed before login.
			if isExempt(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			if !isAuthenticated(r, validator) {
				// API call → 401 JSON
				if isAPIPath(r.URL.Path) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusUnauthorized)
					_, _ = w.Write([]byte(`{"error":"unauthorized"}`)) //nolint:errcheck
					return
				}
				// Browser navigations → redirect to login page
				http.Redirect(w, r, "/login", http.StatusFound)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// isExempt returns true for paths that must be accessible before login.
var exemptPrefixes = []string{
	"/auth/",   // all auth endpoints
	"/login",   // login page and assets
	"/health",  // health check
	"/_next/",  // Next.js build assets
	"/favicon", // browser tab icon
}

// exemptPeerPaths are host-to-host gossip endpoints. Peers are other
// stapler-squad instances with no passkey session, so they can never pass
// isAuthenticated; each handler authenticates the payload itself (Ed25519
// signature + TOFU-pinned key). Matched exactly, not by prefix, so nothing
// else under /internal/ is opened up. Paths must equal
// session.ClaimAdvertisementEndpointPath and session.ClaimLookupEndpointPath
// (asserted in auth_test.go).
//
// The claim-lookup GET carries no signature: it reveals whether a given
// external URL is claimed, and by whom, to anyone who can reach the remote port.
//
// /internal/host-advertisement (session.AdvertisementEndpointPath) is not
// listed: exempting it is a separate decision from the claim endpoints.
var exemptPeerPaths = map[string]struct{}{
	"/internal/claim-advertisement": {},
	"/internal/claim-lookup":        {},
}

func isExempt(path string) bool {
	if _, ok := exemptPeerPaths[path]; ok {
		return true
	}
	for _, prefix := range exemptPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func isAPIPath(path string) bool {
	return strings.HasPrefix(path, "/api/")
}

func isAuthenticated(r *http.Request, validator AuthValidator) bool {
	// Cookie
	if cookie, err := r.Cookie("cs_auth"); err == nil && cookie.Value != "" {
		if validator.ValidateAuthSession(cookie.Value) {
			return true
		}
	}
	// Bearer token (API/headless clients)
	auth := r.Header.Get("Authorization")
	if len(auth) > 7 && auth[:7] == "Bearer " {
		if validator.ValidateAuthSession(auth[7:]) {
			return true
		}
	}
	return false
}
