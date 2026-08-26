package auth

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func newTestHandler(t *testing.T, rpIDs []string, hostnameValidator func(string) bool) *Handler {
	t.Helper()
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())

	store, err := NewCredentialStore()
	if err != nil {
		t.Fatalf("NewCredentialStore: %v", err)
	}
	sessions := NewSessionManager(filepath.Join(t.TempDir(), "auth-sessions.json"))

	origins := []string{"https://" + rpIDs[0]}
	h, err := NewHandler(rpIDs, origins, store, sessions, hostnameValidator)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return h
}

func Test_webauthnForHost_should_MatchConfiguredRPID_When_HostSuffixMatches(t *testing.T) {
	h := newTestHandler(t, []string{"onyx.local"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "onyx.local"

	if _, err := h.webauthnForHost(req); err != nil {
		t.Fatalf("webauthnForHost: %v", err)
	}
}

func Test_webauthnForHost_should_Reject_When_HostUnknownAndNoValidator(t *testing.T) {
	h := newTestHandler(t, []string{"onyx.local"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "netflix1.staplerhome.internal"

	if _, err := h.webauthnForHost(req); err == nil {
		t.Fatal("expected error for unknown host with no hostnameValidator, got nil")
	}
}

// Test_webauthnForHost_should_RegisterNewRPID_When_HostnameValidatorApproves is
// the regression test for the staleness bug: hostname discovery at server
// startup (main.go's detectLANIPs/resolveLANHostnames) is one-shot and can
// miss a hostname that isn't resolvable yet at that instant (e.g. Wi-Fi/DHCP
// still coming up when launchd starts the process). Once the network
// settles, the exact same hostname is legitimately resolvable, but the old
// code kept the initial rpIDs list frozen for the life of the process and
// permanently rejected registrations from that host. This verifies the
// fallback path accepts and remembers a newly-approved hostname instead.
func Test_webauthnForHost_should_RegisterNewRPID_When_HostnameValidatorApproves(t *testing.T) {
	const newHost = "netflix1.staplerhome.internal"
	h := newTestHandler(t, []string{"onyx.local"}, func(hostname string) bool {
		return hostname == newHost
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = newHost

	wa, err := h.webauthnForHost(req)
	if err != nil {
		t.Fatalf("webauthnForHost: %v", err)
	}
	if wa == nil {
		t.Fatal("expected non-nil WebAuthn instance for validated hostname")
	}

	h.mu.RLock()
	_, known := h.webauthn[newHost]
	rpIDs := append([]string(nil), h.rpIDs...)
	h.mu.RUnlock()
	if !known {
		t.Fatalf("expected %s to be registered in webauthn map after validation", newHost)
	}
	found := false
	for _, rpID := range rpIDs {
		if rpID == newHost {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected %s to be appended to rpIDs, got %v", newHost, rpIDs)
	}

	// A second request for the same host must reuse the cached instance
	// rather than erroring or re-registering.
	if _, err := h.webauthnForHost(req); err != nil {
		t.Fatalf("second webauthnForHost call: %v", err)
	}
}

// Test_webauthnForHost_should_IncludeMatchingOrigin_When_RegisteringNewRPID is
// the regression test for the origin-mismatch bug: go-webauthn requires an
// exact origin match (protocol/client.go's IsOriginInHaystack has no
// suffix/wildcard support), so a dynamically-registered rpID whose
// RPOrigins never gained an entry for its own hostname would select a valid
// WebAuthn instance here but fail every real ceremony's origin check. This
// verifies the new instance's RPOrigins actually contains an origin for the
// newly-approved hostname, reusing the scheme/port of the configured origins.
func Test_webauthnForHost_should_IncludeMatchingOrigin_When_RegisteringNewRPID(t *testing.T) {
	const newHost = "netflix1.staplerhome.internal"
	h := newTestHandler(t, []string{"onyx.local"}, func(hostname string) bool {
		return hostname == newHost
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = newHost + ":8444"

	wa, err := h.webauthnForHost(req)
	if err != nil {
		t.Fatalf("webauthnForHost: %v", err)
	}

	wantOrigin := "https://" + newHost
	found := false
	for _, o := range wa.Config.RPOrigins {
		if o == wantOrigin {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected RPOrigins to contain %q, got %v", wantOrigin, wa.Config.RPOrigins)
	}
}

func Test_webauthnForHost_should_Reject_When_HostnameValidatorDenies(t *testing.T) {
	h := newTestHandler(t, []string{"onyx.local"}, func(hostname string) bool {
		return false
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "attacker.example.com"

	if _, err := h.webauthnForHost(req); err == nil {
		t.Fatal("expected error when hostnameValidator denies the hostname, got nil")
	}
}

// Test_webauthnForHost_should_SkipValidator_When_HostnameInNegativeCache is the
// regression test for the negative-cache guard: without it, an attacker (or a
// misbehaving client) sending the same never-resolvable Host header repeatedly
// would trigger a real DNS lookup on every single request. This verifies a
// second attempt for a hostname that already failed validation short-circuits
// without calling hostnameValidator again.
func Test_webauthnForHost_should_SkipValidator_When_HostnameInNegativeCache(t *testing.T) {
	calls := 0
	h := newTestHandler(t, []string{"onyx.local"}, func(hostname string) bool {
		calls++
		return false
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "bad.example.com"
	req.RemoteAddr = "10.0.0.1:5555"

	if _, err := h.webauthnForHost(req); err == nil {
		t.Fatal("expected error on first (validator-denied) attempt")
	}
	if calls != 1 {
		t.Fatalf("expected exactly 1 validator call after first attempt, got %d", calls)
	}

	if _, err := h.webauthnForHost(req); err == nil {
		t.Fatal("expected error on second attempt (negative cache hit)")
	}
	if calls != 1 {
		t.Fatalf("expected validator NOT called again due to negative cache, got %d calls", calls)
	}
}

// Test_webauthnForHost_should_RateLimitPerSourceIP_When_ManyDistinctHostnames
// is the regression test for the per-source-IP rate limiter: the negative
// cache alone doesn't stop an attacker who varies the Host header on every
// request (a fresh cache key each time), which would otherwise still force
// an unbounded number of DNS lookups from a single source. This verifies
// requests from one IP get rate limited once the burst is exhausted, without
// ever calling hostnameValidator for the throttled attempts.
func Test_webauthnForHost_should_RateLimitPerSourceIP_When_ManyDistinctHostnames(t *testing.T) {
	calls := 0
	h := newTestHandler(t, []string{"onyx.local"}, func(hostname string) bool {
		calls++
		return true
	})

	rateLimited := 0
	for i := 0; i < ipLimiterBurst+5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Host = fmt.Sprintf("host-%d.example.com", i)
		req.RemoteAddr = "10.0.0.2:5555"

		if _, err := h.webauthnForHost(req); err != nil {
			rateLimited++
		}
	}

	if rateLimited == 0 {
		t.Fatal("expected some requests to be rate limited once the per-IP burst was exhausted")
	}
	if calls > ipLimiterBurst {
		t.Fatalf("expected at most %d validator calls (the burst size), got %d", ipLimiterBurst, calls)
	}
}
