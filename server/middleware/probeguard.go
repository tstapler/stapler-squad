package middleware

import (
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/tstapler/stapler-squad/log"
)

// ProbeGuardConfig supplies the guard's inputs as functions evaluated per
// request, so hostnames and origins published after construction are honored.
type ProbeGuardConfig struct {
	// LoopbackBound reports whether the listener is bound to a loopback address.
	LoopbackBound  func() bool
	AllowedOrigins func() []string
	AllowedHosts   func() []string
}

// ProbeGuard protects the single procedure at procedurePath (the full,
// /api-prefixed request path) on the unauthenticated listener: POST only, a
// loopback-bound listener, and a Host and Origin that are loopback or
// explicitly allowed. The Host check is what stops DNS rebinding, which CORS
// does not. Every other path passes through untouched.
func ProbeGuard(procedurePath string, cfg ProbeGuardConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != procedurePath {
				next.ServeHTTP(w, r)
				return
			}
			if reason, status := probeGuardVerdict(r, cfg); reason != "" {
				log.Warn("ProbeProgram request rejected", "reason", reason, "host", r.Host,
					"origin", r.Header.Get("Origin"), "remote_addr", r.RemoteAddr)
				http.Error(w, http.StatusText(status), status)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func probeGuardVerdict(r *http.Request, cfg ProbeGuardConfig) (reason string, status int) {
	switch {
	case r.Method != http.MethodPost:
		return "method_not_post", http.StatusMethodNotAllowed
	case cfg.LoopbackBound == nil || !cfg.LoopbackBound():
		return "listener_not_loopback", http.StatusForbidden
	case !hostAllowed(hostnameOf(r.Host), cfg.AllowedHosts):
		return "host_not_allowed", http.StatusForbidden
	}
	if origin := r.Header.Get("Origin"); origin != "" && !originAllowed(origin, cfg.AllowedOrigins) {
		return "origin_not_allowed", http.StatusForbidden
	}
	return "", 0
}

// IsLoopbackHostname reports whether host is localhost or a loopback IP literal.
func IsLoopbackHostname(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// hostnameOf strips an optional port and IPv6 brackets from a Host or listen address.
func hostnameOf(hostport string) string {
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		return h
	}
	return strings.Trim(hostport, "[]")
}

func hostAllowed(host string, allowed func() []string) bool {
	if IsLoopbackHostname(host) {
		return true
	}
	if allowed == nil {
		return false
	}
	for _, a := range allowed() {
		if strings.EqualFold(hostnameOf(a), host) {
			return true
		}
	}
	return false
}

func originAllowed(origin string, allowed func() []string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	if IsLoopbackHostname(u.Hostname()) {
		return true
	}
	if allowed == nil {
		return false
	}
	for _, a := range allowed() {
		if strings.EqualFold(a, origin) {
			return true
		}
	}
	return false
}

// ListenAddrIsLoopback reports whether a listen address such as
// "localhost:8543" or "127.0.0.1:0" binds only to loopback. An empty host
// (":8543") and wildcard addresses bind every interface and are not loopback.
func ListenAddrIsLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	return IsLoopbackHostname(host)
}
