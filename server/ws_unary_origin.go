package server

import (
	"net"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/idna"
)

// wsUnaryRemoteOriginPolicy is the upgrade origin rule for /api/ws/unary on the
// remote listener (ADR-002 item 7). It reads the origin and hostname sets lazily so
// it tracks SetOrigins/SetHostnames after startup, as hostGuardConfig does.
func (s *Server) wsUnaryRemoteOriginPolicy() func(*http.Request) bool {
	return func(r *http.Request) bool {
		return remoteUnaryOriginAllowed(r, s.GetOrigins(), s.GetHostnames())
	}
}

// remoteUnaryOriginAllowed is stricter than isAllowedOrigin (any HTTPS origin): the
// remote listener has no HostGuard and the cs_auth cookie is SameSite-scoped, not
// origin-scoped, so this check is the primary CSWSH control. All must hold:
//   - an Origin that parses to https with no userinfo, path or query;
//   - it equals a configured origin, or equals the request's own Host whose hostname
//     is verified (blocks a DNS-rebound name that merely points at this server);
//   - no wildcard or suffix matching.
//
// A missing Origin is accepted only from a non-browser client: Bearer token, no Cookie.
func remoteUnaryOriginAllowed(r *http.Request, origins, hostnames []string) bool {
	raw := r.Header.Get("Origin")
	if raw == "" {
		return strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") && r.Header.Get("Cookie") == ""
	}
	origin, ok := normalizeHTTPSOrigin(raw)
	if !ok {
		return false
	}
	for _, configured := range origins {
		if c, ok := normalizeHTTPSOrigin(configured); ok && c == origin {
			return true
		}
	}
	return sameOriginAsVerifiedHost(r, origin, hostnames)
}

func sameOriginAsVerifiedHost(r *http.Request, origin string, hostnames []string) bool {
	hostPort, ok := normalizeHostPort(r.Host)
	if !ok || hostPort != strings.TrimPrefix(origin, "https://") {
		return false
	}
	name, _, err := net.SplitHostPort(hostPort)
	if err != nil {
		return false
	}
	for _, h := range hostnames {
		if v, err := idna.Lookup.ToASCII(h); err == nil && strings.EqualFold(v, name) {
			return true
		}
	}
	return false
}

// normalizeHTTPSOrigin returns "https://<ascii-host>:<port>" (lowercase, default
// port 443 made explicit), or false for anything that is not a bare https origin.
func normalizeHTTPSOrigin(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Scheme, "https") || u.User != nil ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return "", false
	}
	hostPort, ok := normalizeHostPort(u.Host)
	if !ok {
		return "", false
	}
	return "https://" + hostPort, true
}

// normalizeHostPort lowercases, converts the host to ASCII (punycode; this also
// rejects wildcards and other invalid labels) and makes the port explicit.
func normalizeHostPort(hostport string) (string, bool) {
	host, port := hostport, "443"
	if h, p, err := net.SplitHostPort(hostport); err == nil {
		host, port = h, p
	} else if strings.Contains(hostport, ":") && !strings.HasPrefix(hostport, "[") {
		return "", false
	}
	host = strings.Trim(host, "[]")
	if host == "" || port == "" {
		return "", false
	}
	if ip := net.ParseIP(host); ip != nil {
		return net.JoinHostPort(ip.String(), port), true
	}
	ascii, err := idna.Lookup.ToASCII(host)
	if err != nil || ascii == "" {
		return "", false
	}
	return net.JoinHostPort(strings.ToLower(ascii), port), true
}
