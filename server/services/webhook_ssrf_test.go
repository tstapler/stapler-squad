package services

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withFakeResolver temporarily replaces the package-level lookupIPAddr var (used by
// resolveAndValidateCallbackHost) with fn, restoring the original on cleanup.
func withFakeResolver(t *testing.T, fn func(ctx context.Context, host string) ([]net.IPAddr, error)) {
	t.Helper()
	orig := lookupIPAddr
	lookupIPAddr = fn
	t.Cleanup(func() { lookupIPAddr = orig })
}

// Test host literals: net.Resolver.LookupIPAddr resolves an already-numeric host
// without a real DNS lookup, so these are deterministic and network-independent.
func TestValidateCallbackURL_RejectsNonHTTPScheme(t *testing.T) {
	err := ValidateCallbackURL(context.Background(), "ftp://8.8.8.8/")
	assert.Error(t, err)
}

func TestValidateCallbackURL_RejectsMalformedURL(t *testing.T) {
	err := ValidateCallbackURL(context.Background(), "://not a url")
	assert.Error(t, err)
}

func TestValidateCallbackURL_RejectsLoopback(t *testing.T) {
	for _, raw := range []string{"http://127.0.0.1/", "http://localhost/", "http://[::1]/"} {
		t.Run(raw, func(t *testing.T) {
			err := ValidateCallbackURL(context.Background(), raw)
			assert.Error(t, err, "expected loopback target to be rejected")
		})
	}
}

func TestValidateCallbackURL_RejectsLinkLocal(t *testing.T) {
	err := ValidateCallbackURL(context.Background(), "http://169.254.1.1/")
	assert.Error(t, err)
}

func TestValidateCallbackURL_RejectsCloudMetadataAddress(t *testing.T) {
	// AC11 calls this address out explicitly, even though it's already covered by
	// the link-local check above — assert it independently so a future refactor of
	// the link-local branch can't silently stop covering it.
	err := ValidateCallbackURL(context.Background(), "http://169.254.169.254/latest/meta-data/")
	assert.Error(t, err)
}

// TestValidateCallbackURL_RejectsUnspecifiedAddress proves the fix for the SSRF gap
// found during sdd:6-verify's security review: on Linux, connect() to 0.0.0.0 (or ::)
// is treated by the kernel as connecting to localhost — an attacker who controls DNS
// for the callback host's domain could return 0.0.0.0 and sail past a filter that only
// checks loopback/link-local/private ranges.
func TestValidateCallbackURL_RejectsUnspecifiedAddress(t *testing.T) {
	for _, raw := range []string{"http://0.0.0.0/", "http://[::]/"} {
		t.Run(raw, func(t *testing.T) {
			err := ValidateCallbackURL(context.Background(), raw)
			assert.Error(t, err, "expected the unspecified address to be rejected")
		})
	}
}

// TestValidateCallbackURL_RejectsCGNATRange proves the fix for the second SSRF gap
// found during review: net.IP.IsPrivate() only covers RFC 1918 + IPv6 ULA, not RFC
// 6598 shared address space (100.64.0.0/10), which several cloud providers use for
// internal-only addressing.
func TestValidateCallbackURL_RejectsCGNATRange(t *testing.T) {
	err := ValidateCallbackURL(context.Background(), "http://100.64.0.1/")
	assert.Error(t, err)
}

func TestValidateCallbackURL_RejectsPrivateRanges(t *testing.T) {
	for _, raw := range []string{
		"http://10.0.0.5/",
		"http://172.16.5.5/",
		"http://192.168.1.1/",
	} {
		t.Run(raw, func(t *testing.T) {
			err := ValidateCallbackURL(context.Background(), raw)
			assert.Error(t, err, "expected private-range target to be rejected")
		})
	}
}

func TestValidateCallbackURL_AcceptsPublicAddress(t *testing.T) {
	// 8.8.8.8 (Google DNS) is a stable, well-known public IP literal — used here
	// purely as "not loopback/link-local/private", no actual network call is made
	// for a POST, and LookupIPAddr on an IP literal doesn't touch the network either.
	err := ValidateCallbackURL(context.Background(), "https://8.8.8.8/hook")
	assert.NoError(t, err)
}

func TestValidateCallbackURL_RejectsEmptyHost(t *testing.T) {
	err := ValidateCallbackURL(context.Background(), "http:///path")
	assert.Error(t, err)
}

// TestResolveAndValidateCallbackHost_RejectsMixedSafetyResultSet proves AC8's edge
// case (research/pitfalls.md): a hostname resolving to BOTH a public and a private
// address must be rejected outright — a caller must never be able to pick only the
// "safe" address from a mixed result set, since a rebinding attacker controls which
// address the actual dial would reach.
func TestResolveAndValidateCallbackHost_RejectsMixedSafetyResultSet(t *testing.T) {
	withFakeResolver(t, func(ctx context.Context, host string) ([]net.IPAddr, error) {
		return []net.IPAddr{
			{IP: net.ParseIP("8.8.8.8")},  // public — would pass alone
			{IP: net.ParseIP("10.0.0.5")}, // private — must sink the whole result
		}, nil
	})

	_, err := resolveAndValidateCallbackHost(context.Background(), "http://multi-answer.example/")
	assert.Error(t, err, "a mixed safe/unsafe DNS answer must reject the target entirely")
}

// TestResolveAndValidateCallbackHost_ReturnsTheValidatedIP proves the IP
// CallbackDispatcher pins its dial to (AC8) is genuinely the one that was checked, not
// a placeholder.
func TestResolveAndValidateCallbackHost_ReturnsTheValidatedIP(t *testing.T) {
	withFakeResolver(t, func(ctx context.Context, host string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
	})

	ip, err := resolveAndValidateCallbackHost(context.Background(), "http://single-answer.example/")
	require.NoError(t, err)
	assert.True(t, net.ParseIP("8.8.8.8").Equal(ip))
}

// TestResolveAndValidateCallbackHost_RejectsWhenSecondLookupWouldDiffer documents the
// DNS-rebinding threat model AC8 closes: resolveAndValidateCallbackHost itself only
// calls the resolver once per invocation (proven here — a resolver that returns a
// disallowed address on a hypothetical "second" call is never reached), and
// CallbackDispatcher.attempt pins its dial to the single IP this call returns rather
// than re-resolving at dial time (proven directly against the real dial path by
// TestCallbackDispatcher_Attempt_DialsThePinnedIP_NotTheURLHost in
// callback_dispatcher_test.go).
func TestResolveAndValidateCallbackHost_RejectsWhenSecondLookupWouldDiffer(t *testing.T) {
	calls := 0
	withFakeResolver(t, func(ctx context.Context, host string) ([]net.IPAddr, error) {
		calls++
		if calls == 1 {
			return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
		}
		return []net.IPAddr{{IP: net.ParseIP("169.254.169.254")}}, nil // rebound to metadata IP
	})

	ip, err := resolveAndValidateCallbackHost(context.Background(), "http://rebinding.example/")
	require.NoError(t, err)
	assert.True(t, net.ParseIP("8.8.8.8").Equal(ip))
	assert.Equal(t, 1, calls, "a single validation call must resolve exactly once, not re-resolve internally")
}
