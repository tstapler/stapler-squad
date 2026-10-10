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
			if isExempt(r) {
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
// isAuthenticated; each handler authenticates the payload itself. Matched
// exactly, not by prefix, so nothing else under /internal/ is opened up. Paths
// must equal session.AdvertisementEndpointPath,
// session.ClaimAdvertisementEndpointPath and session.ClaimLookupEndpointPath
// (asserted in auth_test.go).
//
//   - /internal/host-advertisement is how a peer enrols in HostRegistry
//     (Ed25519 signature, TOFU-pinned key). Without the exemption no peer can
//     ever enrol, the registry stays empty and claim gossip cannot work between
//     hosts. Enrolment is open to anyone who can reach the port (the accepted
//     same-LAN threat model of ADR-002); the handler bounds body size and the
//     registry bounds entry count.
//   - /internal/claim-advertisement accepts only signed claims from enrolled
//     hosts (ClaimIndex.RecordClaim).
//   - /internal/claim-lookup requires a signed request from an enrolled host
//     (ClaimIndex.AuthorizeClaimLookup).
var exemptPeerPaths = map[string]struct{}{
	"/internal/host-advertisement":  {},
	"/internal/claim-advertisement": {},
	"/internal/claim-lookup":        {},
}

func isExempt(r *http.Request) bool {
	if isGenericWebhookDelivery(r) {
		return true
	}
	if _, ok := exemptPeerPaths[r.URL.Path]; ok {
		return true
	}
	for _, prefix := range exemptPrefixes {
		if strings.HasPrefix(r.URL.Path, prefix) {
			return true
		}
	}
	return false
}

func isGenericWebhookDelivery(r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}
	slug, ok := strings.CutPrefix(r.URL.Path, "/webhooks/")
	return ok && slug != "" && slug != "." && slug != ".." && !strings.Contains(slug, "/") && !strings.EqualFold(slug, "github")
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
