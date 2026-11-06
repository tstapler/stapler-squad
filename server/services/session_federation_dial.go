package services

import (
	"fmt"
	"net"
	"net/http"
	"syscall"
)

// federationAllowLoopbackDials lets tests dial httptest servers; never set in production.
var federationAllowLoopbackDials = false

// errForbiddenPeerAddress is returned when a peer address resolves to a range
// federation must never dial.
var errForbiddenPeerAddress = fmt.Errorf("federation: peer address resolves to a forbidden range")

// forbiddenPeerIP reports whether ip is loopback, link-local (incl. cloud
// metadata 169.254.169.254), unspecified or multicast. RFC-1918 is deliberately
// allowed: same-LAN peers are the supported topology (ADR-002).
func forbiddenPeerIP(ip net.IP) bool {
	if federationAllowLoopbackDials && ip.IsLoopback() {
		return false
	}
	return ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified()
}

// guardPeerDial runs after DNS resolution on the actual socket address, so a
// hostname that resolves (or rebinds) to a forbidden IP is refused at connect time.
func guardPeerDial(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return errForbiddenPeerAddress
	}
	ip := net.ParseIP(host)
	if ip == nil || forbiddenPeerIP(ip) {
		return errForbiddenPeerAddress
	}
	return nil
}

// federationTransport is peerTLSTransport with the dial-layer SSRF guard.
func federationTransport() *http.Transport {
	t := peerTLSTransport()
	t.DialContext = (&net.Dialer{Timeout: peerFetchTimeout, Control: guardPeerDial}).DialContext
	t.TLSHandshakeTimeout = peerFetchTimeout
	t.Proxy = nil // a proxy would dial on our behalf and bypass the guard
	return t
}
