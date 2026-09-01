package jules

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zalando/go-keyring"
)

// swapKeyringSeams overrides the package-level keyringGet/Set/Delete test
// seams for the duration of the calling test, restoring the originals via
// t.Cleanup. Tests in this file must not run in parallel with each other --
// the seams are package-level vars, not per-instance -- so none of them
// calls t.Parallel().
func swapKeyringSeams(t *testing.T, get func(string, string) (string, error), set func(string, string, string) error, del func(string, string) error) {
	t.Helper()
	origGet, origSet, origDelete := keyringGet, keyringSet, keyringDelete
	t.Cleanup(func() {
		keyringGet, keyringSet, keyringDelete = origGet, origSet, origDelete
	})
	if get != nil {
		keyringGet = get
	}
	if set != nil {
		keyringSet = set
	}
	if del != nil {
		keyringDelete = del
	}
}

// warnHandler is a minimal slog.Handler that counts Warn-level records
// matching a given message, for asserting "logged exactly once" without
// pulling in a full log-capture library.
type warnHandler struct {
	mu      sync.Mutex
	message string
	count   int
}

func (h *warnHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *warnHandler) Handle(_ context.Context, r slog.Record) error {
	if r.Level == slog.LevelWarn && r.Message == h.message {
		h.mu.Lock()
		h.count++
		h.mu.Unlock()
	}
	return nil
}

func (h *warnHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *warnHandler) WithGroup(string) slog.Handler      { return h }

func (h *warnHandler) Count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.count
}

// installWarnHandler installs a warnHandler as the slog default for the
// duration of the calling test, restoring the prior default via t.Cleanup.
func installWarnHandler(t *testing.T, message string) *warnHandler {
	t.Helper()
	h := &warnHandler{message: message}
	prev := slog.Default()
	slog.SetDefault(slog.New(h))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return h
}

// waitForCount polls get every 2ms until it returns want or timeout elapses,
// failing the test if it never reaches want. Used to synchronize against a
// background probe goroutine's log write without a dedicated done channel.
func waitForCount(t *testing.T, timeout time.Duration, get func() int, want int) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if got := get(); got >= want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for count to reach %d (last was %d)", want, get())
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func TestKeyringTokenSource_APIKey_should_RoundTripThroughKeyring_When_SetThenRead(t *testing.T) {
	store := map[string]string{}
	var gotSetService, gotSetKey string

	swapKeyringSeams(t,
		func(service, key string) (string, error) {
			v, ok := store[service+"/"+key]
			if !ok {
				return "", keyring.ErrNotFound
			}
			return v, nil
		},
		func(service, key, value string) error {
			gotSetService, gotSetKey = service, key
			store[service+"/"+key] = value
			return nil
		},
		func(service, key string) error {
			delete(store, service+"/"+key)
			return nil
		},
	)

	s := NewKeyringTokenSource()
	if err := s.SetJulesAPIKey(context.Background(), JulesAPIKey("AIzaSyD-EXAMPLE")); err != nil {
		t.Fatalf("SetJulesAPIKey: %v", err)
	}

	if gotSetService != "stapler-squad-jules" {
		t.Fatalf("wrote to service %q, want %q (not GitHub's \"stapler-squad\" or SSH's \"stapler-squad-ssh\")", gotSetService, "stapler-squad-jules")
	}
	if gotSetKey != keychainAccount {
		t.Fatalf("wrote to account %q, want %q", gotSetKey, keychainAccount)
	}

	got, err := s.APIKey(context.Background())
	if err != nil {
		t.Fatalf("APIKey: %v", err)
	}
	if got != JulesAPIKey("AIzaSyD-EXAMPLE") {
		t.Fatalf("APIKey = %q, want %q", got, "AIzaSyD-EXAMPLE")
	}
}

func TestKeyringTokenSource_APIKey_should_ReturnErrJulesNotConfigured_When_KeyringEmpty(t *testing.T) {
	swapKeyringSeams(t,
		func(string, string) (string, error) { return "", keyring.ErrNotFound },
		nil, nil,
	)

	s := NewKeyringTokenSource()
	_, err := s.APIKey(context.Background())
	if !errors.Is(err, ErrJulesNotConfigured) {
		t.Fatalf("APIKey error = %v, want errors.Is(err, ErrJulesNotConfigured)", err)
	}
}

func TestKeyringTokenSource_APIKey_should_ReturnWithinFiveSecondsOnFirstCall_When_SecretServiceHangs(t *testing.T) {
	block := make(chan struct{})
	done := make(chan struct{})

	swapKeyringSeams(t,
		func(string, string) (string, error) {
			<-block
			close(done)
			return "never-seen", nil
		},
		nil, nil,
	)
	// Registered after swapKeyringSeams, so it runs BEFORE that helper's
	// restore-the-seam-vars cleanup (t.Cleanup is LIFO): unblocks and waits
	// for the leaked op goroutine (raceKeyringOp's documented non-cancellable
	// gap -- it keeps running past the timeout until the stub returns) to
	// finish its single read of the keyringGet var before that var is
	// restored, so the race detector doesn't see a write racing that read.
	t.Cleanup(func() {
		close(block)
		<-done
	})

	// Test-injected timeout stands in for the production 5s budget so this
	// test runs fast; the same timeout-raced code path is exercised either way.
	s := NewKeyringTokenSource(withKeyringTimeout(200 * time.Millisecond))

	start := time.Now()
	_, err := s.APIKey(context.Background())
	elapsed := time.Since(start)

	if elapsed > time.Second {
		t.Fatalf("first call took %s, want it bounded near the injected keyring timeout", elapsed)
	}
	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
}

func TestKeyringTokenSource_APIKey_should_BoundConcurrentHungKeychainProbesToOne_When_FiftyCallsRaceDuringOutage(t *testing.T) {
	block := make(chan struct{})
	// Exactly two op goroutines call keyringGet and block on <-block during
	// this test: the first (synchronous) call, and the single background
	// probe. done is sized for both so the cleanup below can wait for each
	// one's single read of the keyringGet var to finish before that var is
	// restored -- see the sibling hang test for why this ordering matters.
	done := make(chan struct{}, 2)

	swapKeyringSeams(t,
		func(string, string) (string, error) {
			<-block
			done <- struct{}{}
			return "resumed-value", nil
		},
		nil, nil,
	)
	t.Cleanup(func() {
		close(block)
		<-done
		<-done
	})

	probeStarted := make(chan struct{}, 8)
	s := NewKeyringTokenSource(
		withKeyringTimeout(50*time.Millisecond),
		withCircuitCooldown(time.Hour), // stays open for the whole 50-call burst below
		withProbeStartHook(func() { probeStarted <- struct{}{} }),
	)

	// First call: synchronous, times out, opens the circuit.
	if _, err := s.APIKey(context.Background()); err == nil {
		t.Fatal("expected first call to return a timeout error")
	}

	// 50 more calls simulate 50 poll ticks / HTTP requests during the outage.
	for i := 0; i < 50; i++ {
		start := time.Now()
		_, err := s.APIKey(context.Background())
		elapsed := time.Since(start)

		if elapsed > 20*time.Millisecond {
			t.Fatalf("call %d took %s while circuit open, want a non-blocking return", i, elapsed)
		}
		if !errors.Is(err, ErrJulesKeychainPaused) {
			t.Fatalf("call %d error = %v, want errors.Is(err, ErrJulesKeychainPaused)", i, err)
		}
	}

	// Exactly one probe goroutine should have started across all 51 calls.
	select {
	case <-probeStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("expected exactly one probe to start, got none")
	}
	select {
	case <-probeStarted:
		t.Fatal("expected exactly one probe, got a second")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestKeyringTokenSource_APIKey_should_ServeFromCacheWithoutReResolving_When_CalledWithinTTL(t *testing.T) {
	var calls int32
	swapKeyringSeams(t,
		func(string, string) (string, error) {
			atomic.AddInt32(&calls, 1)
			return "cached-value", nil
		},
		nil, nil,
	)

	s := NewKeyringTokenSource(withCacheTTL(time.Hour))

	for i := 0; i < 10; i++ {
		got, err := s.APIKey(context.Background())
		if err != nil {
			t.Fatalf("call %d: APIKey: %v", i, err)
		}
		if got != JulesAPIKey("cached-value") {
			t.Fatalf("call %d: APIKey = %q, want %q", i, got, "cached-value")
		}
	}

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("underlying keyring Get invoked %d times, want exactly 1", got)
	}
}

func TestKeyringTokenSource_APIKey_should_ReturnErrJulesKeychainPausedAndLogOnce_When_CircuitOpen(t *testing.T) {
	handler := installWarnHandler(t, "jules keychain paused")

	swapKeyringSeams(t,
		func(string, string) (string, error) { return "", errors.New("still down") },
		nil, nil,
	)

	now := time.Now()
	s := NewKeyringTokenSource(withClock(func() time.Time { return now }))
	s.stateMu.Lock()
	s.circuitOpenUntil = now.Add(time.Hour)
	s.stateMu.Unlock()

	for i := 0; i < 5; i++ {
		_, err := s.APIKey(context.Background())
		if !errors.Is(err, ErrJulesKeychainPaused) {
			t.Fatalf("call %d error = %v, want errors.Is(err, ErrJulesKeychainPaused)", i, err)
		}
		if !errors.Is(err, ErrJulesNotConfigured) {
			t.Fatalf("call %d error = %v, want errors.Is(err, ErrJulesNotConfigured) (existing feature-off handling must still apply)", i, err)
		}
	}

	// The single background probe (launched by the first of the 5 calls)
	// fails against the stub above and logs once -- wait for it to land.
	waitForCount(t, time.Second, handler.Count, 1)
	if got := handler.Count(); got != 1 {
		t.Fatalf("\"jules keychain paused\" Warn records = %d across 5 calls, want exactly 1", got)
	}
}

func TestKeyringTokenSource_APIKey_should_ReopenForOneProbeAndCloseOnSuccess_When_CooldownElapses(t *testing.T) {
	var clockMu sync.Mutex
	current := time.Now()
	now := func() time.Time {
		clockMu.Lock()
		defer clockMu.Unlock()
		return current
	}
	advance := func(d time.Duration) {
		clockMu.Lock()
		current = current.Add(d)
		clockMu.Unlock()
	}

	var calls int32
	swapKeyringSeams(t,
		func(string, string) (string, error) {
			atomic.AddInt32(&calls, 1)
			return "reopened-value", nil
		},
		nil, nil,
	)

	s := NewKeyringTokenSource(withClock(now), withCircuitCooldown(time.Second), withCacheTTL(time.Hour))
	s.stateMu.Lock()
	s.circuitOpenUntil = now().Add(time.Second)
	s.stateMu.Unlock()

	advance(2 * time.Second) // past the cooldown boundary

	key, err := s.APIKey(context.Background())
	if err != nil {
		t.Fatalf("APIKey after cooldown: %v", err)
	}
	if key != JulesAPIKey("reopened-value") {
		t.Fatalf("APIKey = %q, want %q", key, "reopened-value")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("underlying keyring Get invoked %d times for the reopen probe, want exactly 1", got)
	}

	s.stateMu.Lock()
	circuitOpen := !s.circuitOpenUntil.IsZero()
	s.stateMu.Unlock()
	if circuitOpen {
		t.Fatal("expected circuit to be closed after the probe succeeded")
	}

	// A subsequent call within the new TTL hits the cache: zero further calls.
	key2, err := s.APIKey(context.Background())
	if err != nil {
		t.Fatalf("APIKey (cached): %v", err)
	}
	if key2 != JulesAPIKey("reopened-value") {
		t.Fatalf("APIKey (cached) = %q, want %q", key2, "reopened-value")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("underlying keyring Get invoked %d times after the cache should have served the second call, want exactly 1", got)
	}
}
