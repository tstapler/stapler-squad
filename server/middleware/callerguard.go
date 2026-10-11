package middleware

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// GuardProfile selects the verdict a LocalWriteGuard applies to one procedure.
type GuardProfile int

const (
	// ProfileProbe is ProbeProgram's verdict, unchanged: POST only, a loopback-bound
	// listener, and a loopback or allowed Host and Origin.
	ProfileProbe GuardProfile = iota
	// ProfileRebinding is the rebinding gate: POST only plus RebindingVerdict. It has
	// no loopback-bound condition, so a verified LAN-hostname browser keeps working
	// on a wildcard bind.
	ProfileRebinding
)

// LocalWriteGuardConfig supplies each profile's inputs as functions evaluated per
// request.
type LocalWriteGuardConfig struct {
	Probe     ProbeGuardConfig
	Rebinding VerdictConfig
}

// LocalWriteGuard protects the procedures in procedures (full, /api-prefixed
// request paths, each with its profile) on the unauthenticated listener. Every
// other path passes through untouched.
func LocalWriteGuard(procedures map[string]GuardProfile, cfg LocalWriteGuardConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			profile, ok := procedures[r.URL.Path]
			if !ok {
				next.ServeHTTP(w, r)
				return
			}
			if reason, status := guardVerdict(profile, r, cfg); reason != "" {
				log.Warn("guarded procedure request rejected", "procedure", r.URL.Path, "reason", reason, "host", clipForLog(r.Host),
					"origin", clipForLog(r.Header.Get("Origin")), "remote_addr", r.RemoteAddr)
				http.Error(w, http.StatusText(status), status)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func guardVerdict(profile GuardProfile, r *http.Request, cfg LocalWriteGuardConfig) (reason string, status int) {
	switch profile {
	case ProfileRebinding:
		if r.Method != http.MethodPost {
			return "method_not_post", http.StatusMethodNotAllowed
		}
		// The procedure guard is installed only on a chain without auth, so the
		// Host test always applies here.
		if reason := RebindingVerdict(CallerFacts{Host: r.Host, Origin: r.Header.Get("Origin")}, cfg.Rebinding); reason != "" {
			return reason, http.StatusForbidden
		}
		return "", 0
	default:
		return probeGuardVerdict(r, cfg.Probe)
	}
}

// CallerFacts are the raw request facts a verdict reads.
type CallerFacts struct {
	Host         string
	Origin       string
	PeerAddr     string
	Proxied      bool
	AuthRequired bool
}

// VerdictConfig supplies the rebinding gate's inputs as functions evaluated per
// request, so values published after construction are honored.
type VerdictConfig struct {
	// VerifiedHosts is the detector-verified hostname set; never the raw detected list.
	VerifiedHosts func() []string
	// ListenAddr is the listener's address, such as "localhost:8543" or ":8543".
	ListenAddr func() string
	// AllowedOrigins are the configured CORS origins.
	AllowedOrigins func() []string
	// LocalIPs lists this machine's interface addresses; nil uses a briefly cached
	// net.InterfaceAddrs.
	LocalIPs func() []net.IP
}

// RebindingVerdict is the rebinding gate. It returns "" to allow, or a reason.
//
// The Host must be a loopback name, a verified hostname, an IP literal that is
// the listener's or a local interface's own address, or the host of a
// non-wildcard listen address; any other IP literal and any unverified name is
// refused. When f.AuthRequired the Host test is skipped (the auth cookie is the
// boundary and the remote Host is a LAN or Tailscale name) and only the Origin
// test applies: an Origin must be loopback, configured, or same-origin with Host.
func RebindingVerdict(f CallerFacts, cfg VerdictConfig) string {
	if !f.AuthRequired {
		host := NormalizeHost(f.Host)
		switch {
		case host == "":
			return "host_missing"
		case !rebindingHostAllowed(host, cfg):
			return "host_not_allowed"
		}
	}
	if f.Origin != "" && !rebindingOriginAllowed(f, cfg.AllowedOrigins) {
		return "origin_not_allowed"
	}
	return ""
}

// LocalCallerVerdict refuses a non-loopback peer, or any request that carries a
// proxy header (a local reverse proxy connects from loopback), unless auth is
// required. It returns "" to allow, or a reason.
func LocalCallerVerdict(f CallerFacts) string {
	switch {
	case f.AuthRequired:
		return ""
	case !PeerIsLoopback(f.PeerAddr):
		return "peer_not_loopback"
	case f.Proxied:
		return "proxy_header"
	}
	return ""
}

// NormalizeHost lowercases hostport, strips the port and IPv6 brackets, and
// strips one trailing dot.
func NormalizeHost(hostport string) string {
	h := strings.ToLower(strings.TrimSpace(hostport))
	if host, _, err := net.SplitHostPort(h); err == nil {
		h = host
	} else {
		h = strings.Trim(h, "[]")
	}
	return strings.TrimSuffix(h, ".")
}

func isWildcardHost(host string) bool {
	if host == "" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsUnspecified()
}

func rebindingHostAllowed(host string, cfg VerdictConfig) bool {
	if IsLoopbackHostname(host) {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ipIsListenerOrLocal(ip, cfg)
	}
	if cfg.VerifiedHosts != nil {
		for _, v := range cfg.VerifiedHosts() {
			if NormalizeHost(v) == host {
				return true
			}
		}
	}
	if cfg.ListenAddr != nil {
		if lh := NormalizeHost(cfg.ListenAddr()); !isWildcardHost(lh) && lh == host {
			return true
		}
	}
	return false
}

func ipIsListenerOrLocal(ip net.IP, cfg VerdictConfig) bool {
	if cfg.ListenAddr != nil {
		if lh := NormalizeHost(cfg.ListenAddr()); !isWildcardHost(lh) {
			if lip := net.ParseIP(lh); lip != nil && lip.Equal(ip) {
				return true
			}
		}
	}
	localIPs := cfg.LocalIPs
	if localIPs == nil {
		localIPs = cachedInterfaceIPs
	}
	for _, l := range localIPs() {
		if l.Equal(ip) {
			return true
		}
	}
	return false
}

func rebindingOriginAllowed(f CallerFacts, allowed func() []string) bool {
	if originAllowed(f.Origin, allowed) {
		return true
	}
	// An Origin is same-origin when its authority equals the request Host.
	if i := strings.Index(f.Origin, "://"); i > 0 {
		return strings.EqualFold(f.Origin[i+3:], f.Host)
	}
	return false
}

const interfaceIPsTTL = 30 * time.Second

var (
	ifaceMu    sync.Mutex
	ifaceIPs   []net.IP
	ifaceIPsAt time.Time
)

// cachedInterfaceIPs returns this machine's interface addresses, refreshed at
// most every interfaceIPsTTL.
func cachedInterfaceIPs() []net.IP {
	ifaceMu.Lock()
	defer ifaceMu.Unlock()
	if ifaceIPs != nil && time.Since(ifaceIPsAt) < interfaceIPsTTL {
		return ifaceIPs
	}
	var ips []net.IP
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, a := range addrs {
			switch v := a.(type) {
			case *net.IPNet:
				ips = append(ips, v.IP)
			case *net.IPAddr:
				ips = append(ips, v.IP)
			}
		}
	}
	ifaceIPs, ifaceIPsAt = ips, time.Now()
	if ifaceIPs == nil {
		ifaceIPs = []net.IP{}
	}
	return ifaceIPs
}

// proxyHeaders mark a request that passed through a reverse proxy or tunnel. A
// local proxy connects from loopback, so the socket alone cannot tell a proxied
// remote caller from a local one.
var proxyHeaders = []string{
	"Forwarded", "X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto", "X-Forwarded-Prefix",
	"X-Original-Forwarded-For", "X-Real-Ip", "X-Client-Ip", "True-Client-Ip", "Cf-Connecting-Ip",
	"Cdn-Loop", "Via",
}

// proxyHeaderPrefixes match whole header families, such as Tailscale Serve's.
var proxyHeaderPrefixes = []string{"Tailscale-"}

// ProxyHeaderName returns the name of a proxy header present in h, or "".
func ProxyHeaderName(h http.Header) string {
	for name, values := range h {
		if len(values) > 0 && isProxyHeader(name) {
			return name
		}
	}
	return ""
}

// HasProxyHeader reports whether h carries any proxy or tunnel header.
func HasProxyHeader(h http.Header) bool {
	return ProxyHeaderName(h) != ""
}

func isProxyHeader(name string) bool {
	name = http.CanonicalHeaderKey(name)
	for _, p := range proxyHeaders {
		if name == http.CanonicalHeaderKey(p) {
			return true
		}
	}
	for _, prefix := range proxyHeaderPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// PeerIsLoopback reports whether a "host:port" socket peer address is loopback.
func PeerIsLoopback(peerAddr string) bool {
	ap, err := netip.ParseAddrPort(peerAddr)
	return err == nil && ap.Addr().Unmap().IsLoopback()
}
