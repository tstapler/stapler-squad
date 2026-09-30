package middleware

import (
	"net/http"
	"path"
	"strings"
)

// AuthValidator is the minimal interface the auth middleware needs from the
// session manager.
type AuthValidator interface {
	ValidateAuthSession(token string) bool
}

// AuthOption customises Auth.
type AuthOption func(*authConfig)

type authConfig struct {
	selfAuthenticated func(path string) bool
}

// WithSelfAuthenticatedPaths lets paths that authenticate every request themselves (with a
// credential the passkey session manager knows nothing about) through without a passkey
// session. match is evaluated per request, so routes registered after the middleware is built
// are honoured, and it is only ever asked about a canonical path: one that is already clean and
// carries no special escaping, so "..", "//" and %-encoded tricks can never be made to look like
// an exempt route. Everything the handler behind such a path does is that handler's
// responsibility to authenticate; this option grants nothing beyond skipping the passkey check.
func WithSelfAuthenticatedPaths(match func(path string) bool) AuthOption {
	return func(c *authConfig) { c.selfAuthenticated = match }
}

// isCanonicalPath reports whether r's path is in canonical form and so safe to match
// literally against an exemption list.
func isCanonicalPath(r *http.Request) bool {
	return r.URL.RawPath == "" && path.Clean(r.URL.Path) == r.URL.Path
}

func (c authConfig) skipsPasskey(r *http.Request) bool {
	return c.selfAuthenticated != nil && isCanonicalPath(r) && c.selfAuthenticated(r.URL.Path)
}

// Auth returns middleware that enforces authentication on all non-exempt paths.
// When auth is nil (auth disabled), the middleware is a no-op pass-through.
func Auth(validator AuthValidator, opts ...AuthOption) func(http.Handler) http.Handler {
	var cfg authConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	return func(next http.Handler) http.Handler {
		if validator == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Always allow auth endpoints and static assets needed before login.
			if isExempt(r.URL.Path) || cfg.skipsPasskey(r) {
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
