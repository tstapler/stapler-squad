package services

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
)

// cloudMetadataIP is the well-known cloud-provider instance-metadata address
// (AWS/GCP/Azure/DigitalOcean all use it). Already covered by IsLinkLocalUnicast
// since 169.254.0.0/16 is link-local, but checked explicitly first so the
// rejection reason is unambiguous, per AC11's wording.
const cloudMetadataIP = "169.254.169.254"

// ValidateCallbackURL rejects an outbound-callback target that could be used for
// SSRF: non-http(s) schemes, and any resolved IP that is loopback, link-local, or
// private-range (including the cloud-metadata address).
//
// Called from two places (AC11's two halves): CallbackConfigService.UpdateCallbackConfig
// at config-save time, and CallbackDispatcher inside its per-attempt retry loop at
// send time. A single check at save time is not sufficient — DNS can change between
// save and any later delivery attempt (TOCTOU / DNS-rebinding, plan.md pitfalls §5) —
// so this function must be called again on every delivery attempt, not cached from the
// save-time check.
//
// Deliberately does not echo rawURL (or the parsed host) back into its error messages
// beyond what's needed for an operator to understand a save-time rejection — callers
// that log a validation failure (CallbackDispatcher) must still avoid logging the URL
// itself per the redaction requirement; this function keeps that easy by never
// constructing an error that contains the full URL (which could carry embedded
// credentials).
func ValidateCallbackURL(ctx context.Context, rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return errors.New("callback URL is not a valid URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("callback URL scheme must be http or https")
	}
	host := parsed.Hostname()
	if host == "" {
		return errors.New("callback URL has no host")
	}

	resolver := &net.Resolver{}
	addrs, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("failed to resolve callback host: %w", err)
	}
	if len(addrs) == 0 {
		return errors.New("callback host did not resolve to any address")
	}
	for _, addr := range addrs {
		if err := checkDisallowedCallbackIP(addr.IP); err != nil {
			return err
		}
	}
	return nil
}

// checkDisallowedCallbackIP rejects loopback, link-local, private-range, and the
// cloud-metadata address.
func checkDisallowedCallbackIP(ip net.IP) error {
	if ip.Equal(net.ParseIP(cloudMetadataIP)) {
		return errors.New("callback host resolves to the cloud metadata address")
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsPrivate() {
		return errors.New("callback host resolves to a disallowed address range")
	}
	return nil
}

// pinnedDialer builds the DialContext function CallbackDispatcher's http.Client
// uses to actually open the TCP connection for an outbound callback POST.
//
// ValidateCallbackURL (above) resolves the callback hostname and validates the
// resulting IPs, but that validation is inert on its own: the stdlib
// http.Transport performs its OWN independent DNS resolution at dial time,
// completely disconnected from whatever ValidateCallbackURL just checked. A
// DNS-rebinding attacker (authoritative DNS with TTL=0) can return a public IP
// for the validation lookup and a private/loopback/cloud-metadata IP moments
// later for the transport's own dial-time lookup — the validated IP is never
// the IP that's actually connected to.
//
// pinnedDialer closes that gap by making dial time the ONLY resolution in the
// critical path: it re-resolves the host itself (via lookupIP), re-validates
// every resolved IP with checkDisallowedCallbackIP (the exact same blocklist
// ValidateCallbackURL uses — not duplicated), and then dials the validated IP
// directly by address, never the original hostname. Since the IP that was
// checked and the IP that gets connected to are now the same value from the
// same lookup, there is no window for a second DNS answer to differ from the
// first.
//
// lookupIP and dial are seams so tests can simulate a rebinding lookup and
// assert dial is never reached for a disallowed IP, without needing real DNS
// or a non-loopback listener.
type pinnedDialer struct {
	lookupIP func(ctx context.Context, host string) ([]net.IP, error)
	dial     func(ctx context.Context, network, addr string) (net.Conn, error)
}

// newPinnedDialer builds a pinnedDialer backed by real DNS resolution and a
// real net.Dialer — the production wiring used by NewCallbackDispatcher.
func newPinnedDialer() *pinnedDialer {
	resolver := &net.Resolver{}
	dialer := &net.Dialer{}
	return &pinnedDialer{
		lookupIP: func(ctx context.Context, host string) ([]net.IP, error) {
			addrs, err := resolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			ips := make([]net.IP, len(addrs))
			for i, a := range addrs {
				ips[i] = a.IP
			}
			return ips, nil
		},
		dial: dialer.DialContext,
	}
}

// DialContext implements the signature required by http.Transport.DialContext.
// addr is "host:port" — the target the transport wants to connect to for a
// given request's URL. If host is already an IP literal, no DNS is involved at
// all and the literal itself is validated directly. Otherwise every resolved
// IP is validated, and dialing rejects the whole attempt if ANY resolved IP is
// disallowed (rather than trying to cherry-pick a safe one) — the same
// fail-closed posture ValidateCallbackURL takes.
func (p *pinnedDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("callback dial target is not host:port: %w", err)
	}

	var ips []net.IP
	if literal := net.ParseIP(host); literal != nil {
		ips = []net.IP{literal}
	} else {
		ips, err = p.lookupIP(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve callback host: %w", err)
		}
		if len(ips) == 0 {
			return nil, errors.New("callback host did not resolve to any address")
		}
	}

	for _, ip := range ips {
		if err := checkDisallowedCallbackIP(ip); err != nil {
			return nil, err
		}
	}

	// Dial the validated IP directly — never the original hostname — so the
	// connection is pinned to the exact address just checked.
	return p.dial(ctx, network, net.JoinHostPort(ips[0].String(), port))
}
