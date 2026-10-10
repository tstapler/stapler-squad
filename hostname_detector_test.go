package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/envtest"
	"github.com/tstapler/stapler-squad/server"
	serverauth "github.com/tstapler/stapler-squad/server/auth"
	"github.com/tstapler/stapler-squad/server/middleware"
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
		validateFn:   func(context.Context, string) bool { return true },
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

	cycle0 := d.redetect(ctx, TriggerStartup)
	if !reflect.DeepEqual(d.networks["10.0.0.5"], []string{"host.example.local"}) {
		t.Fatalf("cycle 0: networks[10.0.0.5] = %v", d.networks["10.0.0.5"])
	}
	if hn := srv.GetHostnames(); !slices.Contains(hn, "host.example.local") {
		t.Fatalf("cycle 0: SetHostnames should include host.example.local, got %v", hn)
	}
	if cycle0.PrevCount != 0 {
		t.Fatalf("cycle 0: PrevCount = %d, want 0 (nothing known before startup)", cycle0.PrevCount)
	}
	if cycle0.NewCount != 1 {
		t.Fatalf("cycle 0: NewCount = %d, want 1", cycle0.NewCount)
	}
	if !reflect.DeepEqual(cycle0.Added, []string{"host.example.local"}) {
		t.Fatalf("cycle 0: Added = %v, want [host.example.local]", cycle0.Added)
	}

	cycle = 1
	cycle1 := d.redetect(ctx, TriggerTimer)
	if cycle1.PrevCount != 1 {
		t.Fatalf("cycle 1: PrevCount = %d, want 1", cycle1.PrevCount)
	}
	if cycle1.NewCount != 2 {
		t.Fatalf("cycle 1: NewCount = %d, want 2 (host2.example.local newly added)", cycle1.NewCount)
	}
	if !reflect.DeepEqual(cycle1.Added, []string{"host2.example.local"}) {
		t.Fatalf("cycle 1: Added = %v, want [host2.example.local]", cycle1.Added)
	}
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
	cycle2 := d.redetect(ctx, TriggerTimer)
	if !reflect.DeepEqual(cycle2.Added, []string{"other.example.local"}) {
		t.Fatalf("cycle 2: Added = %v, want [other.example.local] (transient failure should not permanently exclude the IP)", cycle2.Added)
	}
	if !reflect.DeepEqual(d.networks["10.0.0.9"], []string{"other.example.local"}) {
		t.Fatalf("cycle 2: networks[10.0.0.9] = %v, want [other.example.local] (transient failure should not permanently exclude the IP)", d.networks["10.0.0.9"])
	}
	if hn := srv.GetHostnames(); !slices.Contains(hn, "other.example.local") {
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
		if slices.Contains(cert.SANs, hostname) {
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
	validateFn    func(context.Context, string) bool
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
		validateFn:    func(context.Context, string) bool { return false },
	})

	cycleResult := d.redetect(context.Background(), TriggerStartup)

	if slices.Contains(cycleResult.Added, spoofed) {
		t.Fatalf("unverified hostname %q must not appear in redetectCycle.Added, got %v", spoofed, cycleResult.Added)
	}
	if slices.Contains(d.networks["10.0.0.5"], spoofed) {
		t.Fatalf("unverified hostname %q must not be merged into d.networks, got %v", spoofed, d.networks["10.0.0.5"])
	}
	if hn := srv.GetHostnames(); !slices.Contains(hn, spoofed) {
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
		validateFn:    func(context.Context, string) bool { return true },
	})

	cycleResult := d.redetect(context.Background(), TriggerStartup)

	if !slices.Contains(cycleResult.Added, verified) {
		t.Fatalf("verified hostname %q should appear in redetectCycle.Added, got %v", verified, cycleResult.Added)
	}
	if !slices.Contains(d.networks["10.0.0.9"], verified) {
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
		validateFn: func(context.Context, string) bool { return true },
	})

	cycleResult := d.redetect(context.Background(), TriggerStartup) // must not panic

	if !slices.Contains(cycleResult.Added, verified) {
		t.Fatalf("expected %q in redetectCycle.Added even with nil Handler/CertStore, got %v", verified, cycleResult.Added)
	}
	if !slices.Contains(d.networks["10.0.0.9"], verified) {
		t.Fatalf("expected %q merged into d.networks even with nil Handler/CertStore, got %v", verified, d.networks["10.0.0.9"])
	}
	if hn := srv.GetHostnames(); !slices.Contains(hn, verified) {
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
		validateFn:    func(context.Context, string) bool { return true },
	})

	cycleResult := d.redetect(context.Background(), TriggerStartup)

	if !slices.Contains(cycleResult.Added, verified) {
		t.Fatalf("expected %q in redetectCycle.Added even though cert issuance failed afterward, got %v", verified, cycleResult.Added)
	}
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
		validateFn:    func(context.Context, string) bool { return true },
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

// TestHostnameRedetectInterval_DefaultWhenUnset covers plan.md Task 4.2.1a:
// an unset env var is normal, not a parse failure, so it silently falls back
// to defaultHostnameRedetectInterval with no warning.
func TestHostnameRedetectInterval_DefaultWhenUnset(t *testing.T) {
	buf := captureLogWarn(t)

	got := hostnameRedetectInterval()

	if got != defaultHostnameRedetectInterval {
		t.Fatalf("hostnameRedetectInterval() = %v, want default %v", got, defaultHostnameRedetectInterval)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected no log.Warn for an unset env var, got: %s", buf.String())
	}
}

// TestHostnameRedetectInterval_ParsesOverride covers the env var successfully
// overriding the default.
func TestHostnameRedetectInterval_ParsesOverride(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_HOSTNAME_REDETECT_INTERVAL", "2m")

	if got, want := hostnameRedetectInterval(), 2*time.Minute; got != want {
		t.Fatalf("hostnameRedetectInterval() = %v, want %v", got, want)
	}
}

// TestHostnameRedetectInterval_FallsBackOnParseError covers an unparseable
// value: it must fall back to the default (never fail startup) and log a
// warning naming the offending value.
func TestHostnameRedetectInterval_FallsBackOnParseError(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_HOSTNAME_REDETECT_INTERVAL", "banana")
	buf := captureLogWarn(t)

	got := hostnameRedetectInterval()

	if got != defaultHostnameRedetectInterval {
		t.Fatalf("hostnameRedetectInterval() = %v, want default %v on parse failure", got, defaultHostnameRedetectInterval)
	}
	if !strings.Contains(buf.String(), "banana") {
		t.Fatalf("expected log.Warn to name the offending value %q, got: %s", "banana", buf.String())
	}
}

// ---------------------------------------------------------------------------
// Verified hostname set (Task 1.8d; T-RP-65, T-RP-70, T-RP-71, T-RP-72)
// ---------------------------------------------------------------------------

// verifiedTestDetector builds a detector through NewHostnameDetector (so the
// boot seeding runs) and then swaps in fakes for discovery and ownership.
func verifiedTestDetector(srv *server.Server, cfg HostnameDetectorConfig,
	detect func(context.Context) []string, resolve func(context.Context, string) []string,
	validate func(context.Context, string) bool) *HostnameDetector {
	cfg.Srv = srv
	cfg.ValidateFn = validate
	d := NewHostnameDetector(cfg)
	d.detectFn = detect
	d.resolveFn = resolve
	d.certPublisher = nil
	return d
}

func detectIPs(ips ...string) func(context.Context) []string {
	return func(context.Context) []string { return ips }
}

// T-RP-65: a name resolveAndValidate returned as unverified is in GetHostnames()
// but not in GetVerifiedHostnames(), and the verdict rejects it.
func TestVerdict_ShouldRejectAHostnameThatResolveAndValidateReturnedAsUnverified_WhenTheRealDetectorRunsWithAStubResolver(t *testing.T) {
	srv := &server.Server{}
	validate := func(_ context.Context, name string) bool { return name == "good.lan" }
	resolve := func(_ context.Context, _ string) []string { return []string{"good.lan", "attacker.example"} }
	d := verifiedTestDetector(srv, HostnameDetectorConfig{}, detectIPs("10.0.0.5"), resolve, validate)

	d.redetect(context.Background(), TriggerManual)

	if !slices.Contains(srv.GetHostnames(), "attacker.example") {
		t.Fatalf("precondition: the unverified name must be in GetHostnames(), got %v", srv.GetHostnames())
	}
	if got := srv.GetVerifiedHostnames(); !reflect.DeepEqual(got, []string{"good.lan"}) {
		t.Fatalf("GetVerifiedHostnames = %v, want [good.lan]", got)
	}
	cfg := srv.LocalWriteVerdictConfig()
	if reason := middleware.RebindingVerdict(middleware.CallerFacts{Host: "attacker.example:8543"}, cfg); reason != "host_not_allowed" {
		t.Fatalf("verdict for the unverified name = %q, want host_not_allowed", reason)
	}
	if reason := middleware.RebindingVerdict(middleware.CallerFacts{Host: "good.lan:8543"}, cfg); reason != "" {
		t.Fatalf("verdict for the verified name = %q, want allow", reason)
	}
}

// T-RP-70: names seeded through InitialNetworks as startRemoteAccess seeds them
// are validated, not trusted; literals and localhost are excluded by rule.
func TestVerifiedHostnames_ShouldExcludeASeededUnverifiedNameAnIpLiteralAndLocalhost_WhenInitialNetworksIsSeededAsStartRemoteAccessSeedsItAndValidateFnRejectsOrAcceptsEverything(t *testing.T) {
	seed := map[string][]string{"10.0.0.5": {"10.0.0.5", "ptr.attacker.example", "localhost"}}
	noResolve := func(context.Context, string) []string { return nil }

	t.Run("validateFn rejects everything", func(t *testing.T) {
		srv := &server.Server{}
		d := verifiedTestDetector(srv, HostnameDetectorConfig{InitialNetworks: seed}, detectIPs(), noResolve,
			func(context.Context, string) bool { return false })
		d.redetect(context.Background(), TriggerManual)
		d.redetect(context.Background(), TriggerManual)
		if got := srv.GetVerifiedHostnames(); len(got) != 0 {
			t.Fatalf("verified set = %v, want empty", got)
		}
		if !slices.Contains(srv.GetHostnames(), "ptr.attacker.example") {
			t.Fatalf("the seeded name is expected in GetHostnames(), got %v", srv.GetHostnames())
		}
	})
	t.Run("validateFn accepts everything", func(t *testing.T) {
		srv := &server.Server{}
		d := verifiedTestDetector(srv, HostnameDetectorConfig{InitialNetworks: seed}, detectIPs(), noResolve,
			func(context.Context, string) bool { return true })
		d.redetect(context.Background(), TriggerManual)
		if got := srv.GetVerifiedHostnames(); !reflect.DeepEqual(got, []string{"ptr.attacker.example"}) {
			t.Fatalf("verified set = %v, want only the validated name (literal and localhost excluded by rule)", got)
		}
	})
	t.Run("a literal and localhost are excluded from InitialVerified too", func(t *testing.T) {
		srv := &server.Server{}
		verifiedTestDetector(srv, HostnameDetectorConfig{InitialVerified: []string{"10.0.0.5", "localhost", "ok.lan"}},
			detectIPs(), noResolve, func(context.Context, string) bool { return true })
		if got := srv.GetVerifiedHostnames(); !reflect.DeepEqual(got, []string{"ok.lan"}) {
			t.Fatalf("seeded verified set = %v, want [ok.lan]", got)
		}
	})
}

// T-RP-71: re-verification every cycle, two consecutive false results to drop,
// a lookup cut by the cycle deadline is a timeout and keeps the previous state.
func TestVerifiedHostnames_ShouldDropANameThatFailsReverificationAndReadmitItWhenItPassesAgainAndKeepUncheckedNamesWhenTheCycleTimesOut_WhenCyclesRunWithAFakeResolverAndValidateFn(t *testing.T) {
	srv := &server.Server{}
	result := map[string]bool{"a.lan": true, "b.lan": true}
	var calls []string
	var cancelOn string
	var cancel context.CancelFunc
	validate := func(_ context.Context, name string) bool {
		calls = append(calls, name)
		if name == cancelOn {
			cancel()
			return false
		}
		return result[name]
	}
	d := verifiedTestDetector(srv, HostnameDetectorConfig{InitialVerified: []string{"a.lan", "b.lan"}},
		detectIPs(), func(context.Context, string) []string { return nil }, validate)

	// Non-empty from construction, before any cycle ran, and matched on a normalized Host.
	if got := srv.GetVerifiedHostnames(); !reflect.DeepEqual(got, []string{"a.lan", "b.lan"}) {
		t.Fatalf("seeded set = %v", got)
	}
	if reason := middleware.RebindingVerdict(middleware.CallerFacts{Host: "A.Lan.:8543"}, srv.LocalWriteVerdictConfig()); reason != "" {
		t.Fatalf("normalized Host rejected: %q", reason)
	}

	cycle := func() { d.redetect(context.Background(), TriggerManual) }

	result["a.lan"] = false
	cycle()
	if got := srv.GetVerifiedHostnames(); !slices.Contains(got, "a.lan") {
		t.Fatalf("one false result must not drop a name, got %v", got)
	}
	cycle()
	if got := srv.GetVerifiedHostnames(); !reflect.DeepEqual(got, []string{"b.lan"}) {
		t.Fatalf("two consecutive false results must drop a.lan, got %v", got)
	}
	result["a.lan"] = true
	cycle()
	if got := srv.GetVerifiedHostnames(); !reflect.DeepEqual(got, []string{"a.lan", "b.lan"}) {
		t.Fatalf("a name that passes again must be re-admitted, got %v", got)
	}

	// The cycle deadline expires during a.lan's lookup: both names keep their state.
	var ctx context.Context
	ctx, cancel = context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cancelOn, calls = "a.lan", nil
	d.redetect(ctx, TriggerManual)
	if !reflect.DeepEqual(calls, []string{"a.lan"}) {
		t.Fatalf("lookups after the deadline = %v, want only the one in flight", calls)
	}
	if got := srv.GetVerifiedHostnames(); !reflect.DeepEqual(got, []string{"a.lan", "b.lan"}) {
		t.Fatalf("a timed-out cycle must keep previous state, got %v", got)
	}
	// The cut lookup did not count as a miss: one real false result still keeps a.lan.
	cancelOn, result["a.lan"] = "", false
	cycle()
	if got := srv.GetVerifiedHostnames(); !slices.Contains(got, "a.lan") {
		t.Fatalf("a deadline-cut false must not count toward the drop, got %v", got)
	}
}

// T-RP-72: with the loop disabled the set is the boot verifiedHostnames, static.
func TestVerifiedHostnames_ShouldBeSeededAtBootFromVerifiedHostnamesOnlyAndStayStatic_WhenTheDetectorIsDisabled(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_HOSTNAME_REDETECT_DISABLE", "true")
	srv := &server.Server{}
	ra := &remoteAccessResult{
		Networks:          map[string][]string{"10.0.0.5": {"10.0.0.5", "raw.ptr.example", "ok.lan"}},
		VerifiedHostnames: []string{"ok.lan"},
	}
	var goCalls int
	startHostnameDetector(http.NewServeMux(), srv, ra,
		func() (NetworkChangeSource, error) { return nil, fmt.Errorf("unused when disabled") },
		func(string, func(context.Context)) { goCalls++ },
		func(string, func(context.Context) error) {})

	if goCalls != 0 {
		t.Fatalf("Run goroutine started %d times, want 0", goCalls)
	}
	if got := srv.GetVerifiedHostnames(); !reflect.DeepEqual(got, []string{"ok.lan"}) {
		t.Fatalf("verified set = %v, want only the boot-verified names (raw candidates excluded)", got)
	}
}

func TestRecomputeVerified_ShouldCountOneMissPerNamePerCycle_WhenANameIsListedUnderSeveralSources(t *testing.T) {
	srv := &server.Server{}
	calls := 0
	validate := func(context.Context, string) bool { calls++; return false }
	d := verifiedTestDetector(srv, HostnameDetectorConfig{InitialVerified: []string{"a.lan"}},
		detectIPs(), func(context.Context, string) []string { return nil }, validate)
	d.networks = map[string][]string{"10.0.0.1": {"a.lan"}, "10.0.0.2": {"A.lan."}}

	d.recomputeVerified(context.Background())
	if calls != 1 {
		t.Fatalf("validateFn calls = %d, want 1 per distinct name", calls)
	}
	if got := srv.GetVerifiedHostnames(); !slices.Contains(got, "a.lan") {
		t.Fatalf("one false cycle must not drop a boot-verified name, got %v", got)
	}
	d.recomputeVerified(context.Background())
	if got := srv.GetVerifiedHostnames(); slices.Contains(got, "a.lan") {
		t.Fatalf("two consecutive false cycles must drop it, got %v", got)
	}
}
