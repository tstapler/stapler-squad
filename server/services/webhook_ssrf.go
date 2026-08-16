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

// lookupIPAddr is the DNS resolution function resolveAndValidateCallbackHost uses —
// a package-level var (not threaded through as a parameter/interface) so tests can
// substitute a fake resolver to prove DNS-rebinding protection (a resolver returning
// different answers across calls) without adding a resolver abstraction for a check
// exercised in exactly one code path.
var lookupIPAddr = (&net.Resolver{}).LookupIPAddr

// resolveAndValidateCallbackHost resolves rawURL's host and validates every returned
// address, returning the first validated IP for the caller to pin its dial to
// (webhook-triggers verify follow-ups AC8 — closes the DNS-rebinding window between
// validation and the actual HTTP dial: see CallbackDispatcher.attempt). ValidateCallbackURL
// is a thin wrapper over this for callers that only need the error.
func resolveAndValidateCallbackHost(ctx context.Context, rawURL string) (net.IP, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, errors.New("callback URL is not a valid URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("callback URL scheme must be http or https")
	}
	host := parsed.Hostname()
	if host == "" {
		return nil, errors.New("callback URL has no host")
	}

	addrs, err := lookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve callback host: %w", err)
	}
	if len(addrs) == 0 {
		return nil, errors.New("callback host did not resolve to any address")
	}
	// Any disallowed address in the result set rejects the whole target — a mixed
	// safe/unsafe answer must not let the caller pick only the safe one, since a
	// rebinding attacker controls which address the dial would actually reach.
	for _, addr := range addrs {
		if err := checkDisallowedCallbackIP(addr.IP); err != nil {
			return nil, err
		}
	}
	// Only the first validated address is ever dialed (CallbackDispatcher pins to
	// exactly this IP) — a multi-A/AAAA-record host whose first address is transiently
	// unreachable hard-fails this attempt instead of falling back to the next address
	// the way an unpinned dialer would. Accepted tradeoff for closing the DNS-rebinding
	// window; revisit if multi-address callback targets become common.
	return addrs[0].IP, nil
}

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
	_, err := resolveAndValidateCallbackHost(ctx, rawURL)
	return err
}

// cgnatBlock is RFC 6598 shared address space (100.64.0.0/10) — used by several
// cloud providers (AWS Lambda ENIs, various CNI/CGNAT setups) for internal-only
// addressing. Not covered by net.IP.IsPrivate(), which only implements RFC 1918 +
// IPv6 ULA, so it's checked explicitly (sdd:6-verify security review).
var cgnatBlock = func() *net.IPNet {
	_, n, err := net.ParseCIDR("100.64.0.0/10")
	if err != nil {
		panic(err) // unreachable: the literal is a valid CIDR
	}
	return n
}()

// checkDisallowedCallbackIP rejects loopback, unspecified, link-local, private-range,
// CGNAT, and the cloud-metadata address.
func checkDisallowedCallbackIP(ip net.IP) error {
	if ip.Equal(net.ParseIP(cloudMetadataIP)) {
		return errors.New("callback host resolves to the cloud metadata address")
	}
	// IsUnspecified (0.0.0.0 / ::): on Linux, connect() to 0.0.0.0 is treated by the
	// kernel as connecting to localhost — a documented SSRF technique for bypassing
	// exactly this kind of allow/deny-list filter (sdd:6-verify security review).
	if ip.IsUnspecified() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsPrivate() || cgnatBlock.Contains(ip) {
		return errors.New("callback host resolves to a disallowed address range")
	}
	return nil
}
