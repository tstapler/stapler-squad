package middleware

import (
	"net/http"

	"github.com/tstapler/stapler-squad/log"
)

// HostGuardConfig supplies the guard's inputs as functions evaluated per request.
type HostGuardConfig struct {
	AllowedOrigins func() []string
	AllowedHosts   func() []string
	// ExemptPaths are matched exactly and skip the guard (e.g. "/health").
	ExemptPaths []string
}

// HostGuard rejects any request whose Host is not loopback or explicitly
// allowed, and any request carrying an Origin that is not loopback or allowed.
// It is the DNS-rebinding and same-box-reverse-proxy boundary for the
// unauthenticated local listener: the listener sees a loopback RemoteAddr for
// both, but only the Host/Origin reveal the foreign party. Applies to every
// method and path, including WebSocket upgrades.
func HostGuard(cfg HostGuardConfig) func(http.Handler) http.Handler {
	exempt := make(map[string]struct{}, len(cfg.ExemptPaths))
	for _, p := range cfg.ExemptPaths {
		exempt[p] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := exempt[r.URL.Path]; ok {
				next.ServeHTTP(w, r)
				return
			}
			origin := r.Header.Get("Origin")
			reason := ""
			switch {
			case !hostAllowed(hostnameOf(r.Host), cfg.AllowedHosts):
				reason = "host_not_allowed"
			case origin != "" && !originAllowed(origin, cfg.AllowedOrigins):
				reason = "origin_not_allowed"
			}
			if reason != "" {
				log.Warn("local listener request rejected", "path", r.URL.Path, "reason", reason, "host", clipForLog(r.Host),
					"origin", clipForLog(origin), "remote_addr", r.RemoteAddr)
				http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
