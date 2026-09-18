package main

import (
	"context"
	"fmt"
	"net/http/httptest"
	"reflect"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/envtest"
	"github.com/tstapler/stapler-squad/server"
	serverauth "github.com/tstapler/stapler-squad/server/auth"
	"go.uber.org/goleak"
)

// waitFor receives from ch, failing the test if timeout elapses first. Used
// in place of a real sleep to synchronize on a fake detectFn/resolveFn
// having run, per this repo's deterministic-fast-tests skill.
func waitFor[T any](t *testing.T, ch <-chan T, timeout time.Duration, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for %s", what)
		var zero T
		return zero
	}
}

// countingDetectFn returns a detectFn that increments calls (atomically) and
// signals cycles on every invocation, for tests that only need to know a
// cycle ran, not what it detected.
func countingDetectFn(calls *int32, cycles chan<- struct{}) func(context.Context) []string {
	return func(ctx context.Context) []string {
		atomic.AddInt32(calls, 1)
		cycles <- struct{}{}
		return nil
	}
}

// newTestDetector builds a detector with validateFn defaulting to
// accept-everything -- Epic 2.1's tests predate the Epic 2.2 validation
// gate and assert on d.networks/SetHostnames content that assumed every
// resolved hostname was merged unconditionally. Tests that need to exercise
// the gate itself construct a HostnameDetector literal directly instead of
// using this helper.
func newTestDetector(detectFn func(context.Context) []string, resolveFn func(context.Context, string) []string,
	tick <-chan time.Time, events <-chan struct{}) *HostnameDetector {
	return &HostnameDetector{
		srv:          &server.Server{},
		detectFn:     detectFn,
		resolveFn:    resolveFn,
		cycleTimeout: 5 * time.Second,
		networks:     map[string][]string{},
		tick:         tick,
		events:       events,
		manual:       make(chan chan redetectCycle),
		done:         make(chan struct{}),
		validateFn:   func(string) bool { return true },
	}
}

func noopResolveFn(ctx context.Context, ip string) []string { return nil }

func TestHostnameDetector_Run_StartupCycleRunsImmediately(t *testing.T) {
	defer goleak.VerifyNone(t)

	cycles := make(chan struct{}, 10)
	var calls int32
	d := newTestDetector(countingDetectFn(&calls, cycles), noopResolveFn, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())

	go d.Run(ctx)
	waitFor(t, cycles, time.Second, "startup cycle")

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected 1 call after startup, got %d", got)
	}

	cancel()
	waitFor(t, (<-chan struct{})(d.done), time.Second, "Run to return")
}

func TestHostnameDetector_Run_TickTriggersAnotherCycle(t *testing.T) {
	defer goleak.VerifyNone(t)

	cycles := make(chan struct{}, 10)
	var calls int32
	tick := make(chan time.Time)
	d := newTestDetector(countingDetectFn(&calls, cycles), noopResolveFn, tick, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go d.Run(ctx)
	waitFor(t, cycles, time.Second, "startup cycle")

	tick <- time.Now()
	waitFor(t, cycles, time.Second, "tick-triggered cycle")

	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("expected 2 calls (startup + tick), got %d", got)
	}

	cancel()
	waitFor(t, (<-chan struct{})(d.done), time.Second, "Run to return")
}

func TestHostnameDetector_Run_EventTriggersAnotherCycle(t *testing.T) {
	defer goleak.VerifyNone(t)

	cycles := make(chan struct{}, 10)
	var calls int32
	events := make(chan struct{})
	d := newTestDetector(countingDetectFn(&calls, cycles), noopResolveFn, nil, events)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go d.Run(ctx)
	waitFor(t, cycles, time.Second, "startup cycle")

	events <- struct{}{}
	waitFor(t, cycles, time.Second, "event-triggered cycle")

	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("expected 2 calls (startup + event), got %d", got)
	}

	cancel()
	waitFor(t, (<-chan struct{})(d.done), time.Second, "Run to return")
}

func TestHostnameDetector_Run_StopsOnContextCancel(t *testing.T) {
	defer goleak.VerifyNone(t)

	d := newTestDetector(func(ctx context.Context) []string { return nil }, noopResolveFn, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())

	go d.Run(ctx)
	cancel()

	waitFor(t, (<-chan struct{})(d.done), time.Second, "Run to return after ctx cancel")
}

// TestHostnameDetector_Redetect_AddOnlyMergesNetworks covers the acceptance
// criteria in plan.md Story 2.1.1: resolveFn is re-run for every currently
// known IP on every cycle (not just newly-seen ones), and a transient
// resolution failure on first sighting never permanently excludes an IP.
func TestHostnameDetector_Redetect_AddOnlyMergesNetworks(t *testing.T) {
	srv := &server.Server{}

	var cycle int
	detectFn := func(ctx context.Context) []string {
		if cycle == 0 {
			return []string{"10.0.0.5"}
		}
		return []string{"10.0.0.5", "10.0.0.9"}
	}
	resolveFn := func(ctx context.Context, ip string) []string {
		switch {
		case ip == "10.0.0.5" && cycle == 0:
			return []string{"host.example.local"}
		case ip == "10.0.0.5" && cycle == 1:
			// Re-resolved on a later cycle: gains a second hostname.
			return []string{"host.example.local", "host2.example.local"}
		case ip == "10.0.0.9" && cycle == 0:
			// Not detected yet this cycle.
			return nil
		case ip == "10.0.0.9" && cycle == 1:
			// Newly detected this cycle; first resolution attempt fails
			// transiently (simulating a DNS hiccup right after discovery).
			return nil
		case ip == "10.0.0.9" && cycle == 2:
			// Later cycle: the earlier transient failure did not
			// permanently exclude this IP from being re-resolved.
			return []string{"other.example.local"}
		}
		return nil
	}

	d := newTestDetector(detectFn, resolveFn, nil, nil)
	d.srv = srv

	ctx := context.Background()

	d.redetect(ctx, TriggerStartup)
	if !reflect.DeepEqual(d.networks["10.0.0.5"], []string{"host.example.local"}) {
		t.Fatalf("cycle 0: networks[10.0.0.5] = %v", d.networks["10.0.0.5"])
	}
	if hn := srv.GetHostnames(); !containsString(hn, "host.example.local") {
		t.Fatalf("cycle 0: SetHostnames should include host.example.local, got %v", hn)
	}

	cycle = 1
	d.redetect(ctx, TriggerTimer)
	got := append([]string(nil), d.networks["10.0.0.5"]...)
	sort.Strings(got)
	want := []string{"host.example.local", "host2.example.local"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("cycle 1: networks[10.0.0.5] = %v, want %v", got, want)
	}
	if _, ok := d.networks["10.0.0.9"]; !ok {
		t.Fatalf("cycle 1: 10.0.0.9 should be a known key even though resolution failed")
	}
	if len(d.networks["10.0.0.9"]) != 0 {
		t.Fatalf("cycle 1: 10.0.0.9 should have no hostnames yet, got %v", d.networks["10.0.0.9"])
	}

	cycle = 2
	d.redetect(ctx, TriggerTimer)
	if !reflect.DeepEqual(d.networks["10.0.0.9"], []string{"other.example.local"}) {
		t.Fatalf("cycle 2: networks[10.0.0.9] = %v, want [other.example.local] (transient failure should not permanently exclude the IP)", d.networks["10.0.0.9"])
	}
	if hn := srv.GetHostnames(); !containsString(hn, "other.example.local") {
		t.Fatalf("cycle 2: SetHostnames should include other.example.local, got %v", hn)
	}
}

// TestHostnameDetector_Redetect_TimesOutBoundedByCycleTimeout covers Task
// 2.1.1b-timeout: a resolveFn that ignores cancellation entirely would hang
// Run's whole loop forever; one that respects ctx.Done() (as
// resolveLANHostnames's threaded-through subprocess/DNS calls do) lets
// redetect return within cycleTimeout instead.
func TestHostnameDetector_Redetect_TimesOutBoundedByCycleTimeout(t *testing.T) {
	const cycleTimeout = 200 * time.Millisecond

	d := newTestDetector(
		func(ctx context.Context) []string { return []string{"10.0.0.5"} },
		func(ctx context.Context, ip string) []string {
			<-ctx.Done()
			return nil
		},
		nil, nil,
	)
	d.cycleTimeout = cycleTimeout

	start := time.Now()
	done := make(chan redetectCycle, 1)
	go func() { done <- d.redetect(context.Background(), TriggerStartup) }()

	result := waitFor(t, done, cycleTimeout+2*time.Second, "redetect to return after cycleTimeout")
	elapsed := time.Since(start)

	if elapsed > cycleTimeout+time.Second {
		t.Fatalf("redetect took %v, expected roughly cycleTimeout (%v) plus a small margin", elapsed, cycleTimeout)
	}
	if result.Trigger != TriggerStartup {
		t.Fatalf("expected Trigger=TriggerStartup even on timeout, got %v", result.Trigger)
	}
}

// newTestWAHandler builds a real *serverauth.Handler seeded with one RPID,
// isolated from any other test's on-disk credential/session state via
// envtest.NewIsolatedStateDir. hostnameValidator is nil throughout Epic 2.2's
// tests -- HostnameDetector's own validateFn is the thing under test, not
// webauthnForHost's independent reactive-validation path.
func newTestWAHandler(t *testing.T, seedRPID string) *serverauth.Handler {
	t.Helper()
	envtest.NewIsolatedStateDir(t)

	store, err := serverauth.NewCredentialStore()
	if err != nil {
		t.Fatalf("NewCredentialStore: %v", err)
	}
	sessions := serverauth.NewSessionManager(t.TempDir() + "/auth-sessions.json")
	t.Cleanup(sessions.Close) // stop the cleanup goroutine so goleak checks elsewhere in this binary don't see it

	h, err := serverauth.NewHandler([]string{seedRPID}, []string{"https://" + seedRPID}, store, sessions, nil)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return h
}

// hostnameHasRPID reports whether h has hostname registered as a usable
// RPID, asserted indirectly through the same webauthnForHost path a real
// registration ceremony would use (BeginRegistration), per plan.md Task
// 2.2.1c's suggested indirect-assertion strategy -- Handler's rpIDs field
// is unexported and this test lives in package main, not package auth.
func hostnameHasRPID(h *serverauth.Handler, hostname string) bool {
	r := httptest.NewRequest("POST", "https://"+hostname+"/webauthn/register/begin", nil)
	r.Host = hostname
	_, _, _, err := h.BeginRegistration(r)
	return err == nil
}

// certStoreHasSAN reports whether certStore's current cert map contains
// hostname in any network's SANs.
func certStoreHasSAN(certStore *server.NetworkCertStore, hostname string) bool {
	for _, cert := range certStore.Load() {
		if containsString(cert.SANs, hostname) {
			return true
		}
	}
	return false
}

// gateTestDetectorConfig is Epic 2.2's test-only counterpart to
// HostnameDetectorConfig, letting each validation-gate test override just
// the collaborators it cares about (waHandler, certStore, certPublisher,
// validateFn) instead of repeating every other HostnameDetector field.
type gateTestDetectorConfig struct {
	srv           *server.Server
	detectFn      func(context.Context) []string
	resolveFn     func(context.Context, string) []string
	waHandler     *serverauth.Handler
	certStore     *server.NetworkCertStore
	certPublisher func(map[string][]string) (string, map[string]*server.NetworkCert, error)
	validateFn    func(string) bool
}

func newGateTestDetector(cfg gateTestDetectorConfig) *HostnameDetector {
	return &HostnameDetector{
		srv:           cfg.srv,
		detectFn:      cfg.detectFn,
		resolveFn:     cfg.resolveFn,
		cycleTimeout:  5 * time.Second,
		networks:      map[string][]string{},
		manual:        make(chan chan redetectCycle),
		done:          make(chan struct{}),
		waHandler:     cfg.waHandler,
		certStore:     cfg.certStore,
		certPublisher: cfg.certPublisher,
		validateFn:    cfg.validateFn,
	}
}

// TestHostnameDetector_Redetect_UnverifiedHostnameNeverReachesRPIDOrTLS is
// the security-critical adversarial test for Story 2.2.1: a hostname that
// fails validateFn (e.g. a spoofed mDNS/PTR answer that doesn't actually
// forward-resolve to this host) must never become a trusted WebAuthn RPID
// or land in a TLS certificate's SAN list, even though it's still allowed
// to reach Server.hostnames as internal bookkeeping.
func TestHostnameDetector_Redetect_UnverifiedHostnameNeverReachesRPIDOrTLS(t *testing.T) {
	const seedRPID = "seed.local"
	const spoofed = "evil-onyx.local"

	h := newTestWAHandler(t, seedRPID)
	certStore := server.NewNetworkCertStore(nil)
	srv := &server.Server{}

	d := newGateTestDetector(gateTestDetectorConfig{
		srv:           srv,
		detectFn:      func(context.Context) []string { return []string{"10.0.0.5"} },
		resolveFn:     func(context.Context, string) []string { return []string{spoofed} },
		waHandler:     h,
		certStore:     certStore,
		certPublisher: server.EnsureNetworkTLSCerts,
		validateFn:    func(string) bool { return false },
	})

	d.redetect(context.Background(), TriggerStartup)

	if containsString(d.networks["10.0.0.5"], spoofed) {
		t.Fatalf("unverified hostname %q must not be merged into d.networks, got %v", spoofed, d.networks["10.0.0.5"])
	}
	if hn := srv.GetHostnames(); !containsString(hn, spoofed) {
		t.Fatalf("unverified hostname %q should still reach Server.hostnames as bookkeeping, got %v", spoofed, hn)
	}
	if hostnameHasRPID(h, spoofed) {
		t.Fatalf("unverified hostname %q must never become a trusted RPID", spoofed)
	}
	if certStoreHasSAN(certStore, spoofed) {
		t.Fatalf("unverified hostname %q must never appear in a TLS cert's SANs", spoofed)
	}
}

// TestHostnameDetector_Redetect_VerifiedHostnameReachesRPIDAndTLS mirrors the
// adversarial test above with validateFn approving the hostname: it must
// reach both the WebAuthn RPID set and the TLS SAN set.
func TestHostnameDetector_Redetect_VerifiedHostnameReachesRPIDAndTLS(t *testing.T) {
	const seedRPID = "seed.local"
	const verified = "netflix1.newwifi.local"

	h := newTestWAHandler(t, seedRPID)
	certStore := server.NewNetworkCertStore(nil)
	srv := &server.Server{}

	d := newGateTestDetector(gateTestDetectorConfig{
		srv:           srv,
		detectFn:      func(context.Context) []string { return []string{"10.0.0.9"} },
		resolveFn:     func(context.Context, string) []string { return []string{verified} },
		waHandler:     h,
		certStore:     certStore,
		certPublisher: server.EnsureNetworkTLSCerts,
		validateFn:    func(string) bool { return true },
	})

	d.redetect(context.Background(), TriggerStartup)

	if !containsString(d.networks["10.0.0.9"], verified) {
		t.Fatalf("verified hostname %q should be merged into d.networks, got %v", verified, d.networks["10.0.0.9"])
	}
	if !hostnameHasRPID(h, verified) {
		t.Fatalf("verified hostname %q should have been registered as a trusted RPID", verified)
	}
	if !certStoreHasSAN(certStore, verified) {
		t.Fatalf("verified hostname %q should appear in a TLS cert's SANs, got %v", verified, certStore.Load())
	}
}

// TestHostnameDetector_Redetect_NilHandlerAndCertStoreDoNotPanic covers
// Story 2.2.1's third acceptance criterion: remote access disabled (nil
// Handler/CertStore) must not panic when a new, verified hostname is found,
// and still keeps Server.hostnames current.
func TestHostnameDetector_Redetect_NilHandlerAndCertStoreDoNotPanic(t *testing.T) {
	const verified = "netflix1.newwifi.local"
	srv := &server.Server{}

	d := newGateTestDetector(gateTestDetectorConfig{
		srv:        srv,
		detectFn:   func(context.Context) []string { return []string{"10.0.0.9"} },
		resolveFn:  func(context.Context, string) []string { return []string{verified} },
		validateFn: func(string) bool { return true },
	})

	d.redetect(context.Background(), TriggerStartup) // must not panic

	if !containsString(d.networks["10.0.0.9"], verified) {
		t.Fatalf("expected %q merged into d.networks even with nil Handler/CertStore, got %v", verified, d.networks["10.0.0.9"])
	}
	if hn := srv.GetHostnames(); !containsString(hn, verified) {
		t.Fatalf("expected Server.hostnames to include %q, got %v", verified, hn)
	}
}

// TestHostnameDetector_Redetect_CertIssuanceFailureDoesNotRollbackRPID
// covers Story 2.2.1's fourth acceptance criterion: a cert-issuance failure
// in the same cycle a hostname was newly verified and registered must not
// roll back that RPID registration, and must skip certStore.Store for that
// cycle only.
func TestHostnameDetector_Redetect_CertIssuanceFailureDoesNotRollbackRPID(t *testing.T) {
	const seedRPID = "seed.local"
	const verified = "netflix1.newwifi.local"

	h := newTestWAHandler(t, seedRPID)
	certStore := server.NewNetworkCertStore(nil)
	srv := &server.Server{}

	failingPublisher := func(map[string][]string) (string, map[string]*server.NetworkCert, error) {
		return "", nil, fmt.Errorf("simulated cert issuance failure")
	}

	d := newGateTestDetector(gateTestDetectorConfig{
		srv:           srv,
		detectFn:      func(context.Context) []string { return []string{"10.0.0.9"} },
		resolveFn:     func(context.Context, string) []string { return []string{verified} },
		waHandler:     h,
		certStore:     certStore,
		certPublisher: failingPublisher,
		validateFn:    func(string) bool { return true },
	})

	d.redetect(context.Background(), TriggerStartup)

	if !hostnameHasRPID(h, verified) {
		t.Fatalf("RegisterHostname for %q must not be rolled back when cert issuance fails afterward", verified)
	}
	if certStore.Load() != nil {
		t.Fatalf("certStore.Store must not be called this cycle when cert issuance failed, got %v", certStore.Load())
	}
}

// TestHostnameDetector_Redetect_RepeatedNoOpCyclesAreStable covers
// requirements.md's idempotency success metric: two consecutive no-change
// cycles must leave d.networks byte-identical, the second cycle's
// redetectCycle must report NewCount == PrevCount with an empty Added list,
// and downstream publication (RegisterHostname/EnsureNetworkTLSCerts) must
// not be redundantly re-invoked for already-known, unchanged state.
func TestHostnameDetector_Redetect_RepeatedNoOpCyclesAreStable(t *testing.T) {
	const seedRPID = "seed.local"
	const verified = "netflix1.newwifi.local"

	h := newTestWAHandler(t, seedRPID)
	certStore := server.NewNetworkCertStore(nil)
	srv := &server.Server{}

	var publisherCalls int32
	countingPublisher := func(networks map[string][]string) (string, map[string]*server.NetworkCert, error) {
		atomic.AddInt32(&publisherCalls, 1)
		return server.EnsureNetworkTLSCerts(networks)
	}

	d := newGateTestDetector(gateTestDetectorConfig{
		srv:           srv,
		detectFn:      func(context.Context) []string { return []string{"10.0.0.9"} },
		resolveFn:     func(context.Context, string) []string { return []string{verified} },
		waHandler:     h,
		certStore:     certStore,
		certPublisher: countingPublisher,
		validateFn:    func(string) bool { return true },
	})

	first := d.redetect(context.Background(), TriggerStartup)
	networksAfterFirst := map[string][]string{}
	for ip, hostnames := range d.networks {
		networksAfterFirst[ip] = append([]string(nil), hostnames...)
	}
	if calls := atomic.LoadInt32(&publisherCalls); calls != 1 {
		t.Fatalf("expected certPublisher called once after first cycle, got %d", calls)
	}
	if len(first.Added) == 0 {
		t.Fatalf("expected first cycle to report Added, got none")
	}

	second := d.redetect(context.Background(), TriggerTimer)

	if !reflect.DeepEqual(networksAfterFirst, d.networks) {
		t.Fatalf("d.networks changed on a no-op second cycle: before=%v after=%v", networksAfterFirst, d.networks)
	}
	if second.NewCount != second.PrevCount {
		t.Fatalf("expected second cycle's NewCount == PrevCount (no change), got NewCount=%d PrevCount=%d", second.NewCount, second.PrevCount)
	}
	if len(second.Added) != 0 {
		t.Fatalf("expected second cycle's Added to be empty, got %v", second.Added)
	}
	if calls := atomic.LoadInt32(&publisherCalls); calls != 1 {
		t.Fatalf("expected certPublisher NOT re-invoked on no-op second cycle, call count = %d", calls)
	}
}
