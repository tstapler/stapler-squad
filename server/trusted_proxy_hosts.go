package server

import (
	"net"
	"regexp"
	"strings"

	"github.com/tstapler/stapler-squad/log"
)

// TrustedProxyHostsEnv lists hostnames a same-box reverse proxy forwards to the
// loopback listener (comma-separated), e.g. a dev-workspace app proxy. The proxy
// must authenticate callers itself: the listener behind it is unauthenticated.
const TrustedProxyHostsEnv = "STAPLER_SQUAD_TRUSTED_PROXY_HOSTS"

var dnsLabelPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// ParseTrustedProxyHosts splits raw on commas and keeps entries that are plain
// DNS names: no scheme, port, path, wildcard, IP literal or "localhost". Rejected
// entries are logged and returned, never silently trusted.
func ParseTrustedProxyHosts(raw string) (valid, rejected []string) {
	for _, entry := range strings.Split(raw, ",") {
		host := strings.ToLower(strings.TrimSpace(entry))
		if host == "" {
			continue
		}
		if validTrustedProxyHost(host) {
			valid = append(valid, host)
			continue
		}
		log.Warn("Rejected invalid "+TrustedProxyHostsEnv+" entry", "entry", host)
		rejected = append(rejected, host)
	}
	return valid, rejected
}

func validTrustedProxyHost(host string) bool {
	if len(host) > 253 || host == "localhost" || net.ParseIP(host) != nil {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if !dnsLabelPattern.MatchString(label) {
			return false
		}
	}
	return true
}

// SetTrustedProxyHosts replaces the trusted proxy hostname set.
func (s *Server) SetTrustedProxyHosts(hosts []string) {
	cp := append([]string(nil), hosts...)
	s.trustedProxyHosts.Store(&cp)
}

func (s *Server) trustedProxyHostList() []string {
	if p := s.trustedProxyHosts.Load(); p != nil {
		return *p
	}
	return nil
}

// guardHosts is the Host allowlist of the local-listener guards: the detected set
// plus trusted proxy hosts. GetHostnames itself stays unchanged because it also
// feeds TLS SANs and QR codes.
func (s *Server) guardHosts() []string {
	return append(append([]string(nil), s.GetHostnames()...), s.trustedProxyHostList()...)
}

// guardVerifiedHosts is the write guard's verified-host set plus trusted proxy hosts.
func (s *Server) guardVerifiedHosts() []string {
	return append(append([]string(nil), s.GetVerifiedHostnames()...), s.trustedProxyHostList()...)
}

// guardOrigins is the configured origin set plus an https origin per trusted proxy host.
func (s *Server) guardOrigins() []string {
	out := append([]string(nil), s.GetOrigins()...)
	for _, h := range s.trustedProxyHostList() {
		out = append(out, "https://"+h)
	}
	return out
}
