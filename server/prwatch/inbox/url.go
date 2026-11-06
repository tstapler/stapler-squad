package inbox

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// ErrNonLoopbackURL means the inbox URL could send the bearer token off-host.
var ErrNonLoopbackURL = errors.New("inbox url must be http(s) on 127.0.0.1, ::1 or localhost")

// ParseLoopbackURL accepts only http(s) URLs whose host is localhost or a
// loopback IP literal, with no userinfo. It performs no network I/O.
func ParseLoopbackURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %q", ErrNonLoopbackURL, raw)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Opaque != "" {
		return nil, fmt.Errorf("%w: %q", ErrNonLoopbackURL, raw)
	}
	host := u.Hostname()
	if !strings.EqualFold(host, "localhost") {
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return nil, fmt.Errorf("%w: %q", ErrNonLoopbackURL, raw)
		}
	}
	return u, nil
}

// Resolver is the subset of *net.Resolver the dialer needs; tests inject a fake.
type Resolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

// DialFunc matches http.Transport.DialContext.
type DialFunc func(ctx context.Context, network, addr string) (net.Conn, error)

// LoopbackDialer returns a DialFunc that refuses any address that is not
// loopback. A hostname must resolve only to loopback addresses, and the
// connection goes to the validated IP so a second lookup cannot differ.
func LoopbackDialer(resolver Resolver, dialer *net.Dialer) DialFunc {
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	if dialer == nil {
		dialer = &net.Dialer{}
	}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		ip := net.ParseIP(host)
		if ip == nil {
			addrs, err := resolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			if len(addrs) == 0 {
				return nil, fmt.Errorf("%w: %q resolved to no addresses", ErrNonLoopbackURL, host)
			}
			for _, a := range addrs {
				if !a.IP.IsLoopback() {
					return nil, fmt.Errorf("%w: %q resolved to %s", ErrNonLoopbackURL, host, a.IP)
				}
			}
			ip = addrs[0].IP
		} else if !ip.IsLoopback() {
			return nil, fmt.Errorf("%w: %s", ErrNonLoopbackURL, ip)
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
	}
}
