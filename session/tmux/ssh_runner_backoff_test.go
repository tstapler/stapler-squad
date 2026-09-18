package tmux

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeClock is a manually-advanced clock for deterministic backoff timing
// tests, mirroring executor/circuit_breaker_test.go's approach for the
// pattern this type reuses.
type fakeClock struct {
	now time.Time
}

func (c *fakeClock) Now() time.Time          { return c.now }
func (c *fakeClock) Advance(d time.Duration) { c.now = c.now.Add(d) }

func newFakeClock() *fakeClock { return &fakeClock{now: time.Unix(0, 0)} }

// TestSSHBackoff_ClosedAllowsAttempts verifies a fresh backoff starts
// CLOSED and allows attempts immediately.
func TestSSHBackoff_ClosedAllowsAttempts(t *testing.T) {
	b := newSSHBackoffWithClock(sshBackoffConfig{FailureThreshold: 3, BaseInterval: time.Second, MaxInterval: time.Minute}, newFakeClock())
	if err := b.allowAttempt(); err != nil {
		t.Fatalf("allowAttempt() on a fresh backoff = %v, want nil", err)
	}
	if got := b.State(); got != sshCircuitClosed {
		t.Errorf("State() = %v, want CLOSED", got)
	}
}

// TestSSHBackoff_TripsOpenAfterThresholdFailures verifies the circuit opens
// exactly at FailureThreshold consecutive failures, not before.
func TestSSHBackoff_TripsOpenAfterThresholdFailures(t *testing.T) {
	clk := newFakeClock()
	b := newSSHBackoffWithClock(sshBackoffConfig{FailureThreshold: 3, BaseInterval: time.Second, MaxInterval: time.Minute}, clk)

	for i := 0; i < 2; i++ {
		if err := b.allowAttempt(); err != nil {
			t.Fatalf("attempt %d: allowAttempt() = %v, want nil (threshold not yet reached)", i, err)
		}
		b.recordResult(false)
		if got := b.State(); got != sshCircuitClosed {
			t.Fatalf("attempt %d: State() = %v, want CLOSED (only %d of 3 failures recorded)", i, got, i+1)
		}
	}

	// Third consecutive failure trips the circuit.
	if err := b.allowAttempt(); err != nil {
		t.Fatalf("allowAttempt() before 3rd failure = %v, want nil", err)
	}
	b.recordResult(false)
	if got := b.State(); got != sshCircuitOpen {
		t.Fatalf("State() after 3rd consecutive failure = %v, want OPEN", got)
	}

	// Immediately after tripping, further attempts are rejected without
	// blocking or dialing again -- the "not a tight retry loop" behavior.
	if err := b.allowAttempt(); !errors.Is(err, ErrSSHCircuitOpen) {
		t.Errorf("allowAttempt() immediately after trip = %v, want ErrSSHCircuitOpen", err)
	}
}

// TestSSHBackoff_IntervalDoublesOnRepeatedHalfOpenFailures verifies the
// recovery wait increases (exponential backoff) each time a HALF-OPEN probe
// fails, rather than staying constant.
func TestSSHBackoff_IntervalDoublesOnRepeatedHalfOpenFailures(t *testing.T) {
	clk := newFakeClock()
	cfg := sshBackoffConfig{FailureThreshold: 1, BaseInterval: time.Second, MaxInterval: time.Hour}
	b := newSSHBackoffWithClock(cfg, clk)

	// Trip the circuit open.
	if err := b.allowAttempt(); err != nil {
		t.Fatalf("allowAttempt() = %v, want nil", err)
	}
	b.recordResult(false)
	if got := b.State(); got != sshCircuitOpen {
		t.Fatalf("State() = %v, want OPEN", got)
	}

	var lastInterval time.Duration
	for i := 0; i < 3; i++ {
		b.mu.Lock()
		interval := b.effectiveInterval()
		b.mu.Unlock()
		if i > 0 && interval <= lastInterval {
			t.Errorf("probe %d: effectiveInterval() = %v, want > previous %v (exponential backoff)", i, interval, lastInterval)
		}
		lastInterval = interval

		clk.Advance(interval)
		if err := b.allowAttempt(); err != nil {
			t.Fatalf("probe %d: allowAttempt() after advancing past interval = %v, want nil (HALF-OPEN probe allowed)", i, err)
		}
		if got := b.State(); got != sshCircuitHalfOpen {
			t.Fatalf("probe %d: State() = %v, want HALF-OPEN", i, got)
		}
		b.recordResult(false) // fail the probe, reopening with a longer wait
		if got := b.State(); got != sshCircuitOpen {
			t.Fatalf("probe %d: State() after failed probe = %v, want OPEN", i, got)
		}
	}
}

// TestSSHBackoff_IntervalCappedAtMaxInterval verifies exponential backoff
// stops growing once it reaches MaxInterval.
func TestSSHBackoff_IntervalCappedAtMaxInterval(t *testing.T) {
	clk := newFakeClock()
	cfg := sshBackoffConfig{FailureThreshold: 1, BaseInterval: time.Second, MaxInterval: 4 * time.Second}
	b := newSSHBackoffWithClock(cfg, clk)

	_ = b.allowAttempt()
	b.recordResult(false) // OPEN, interval = 1s

	for i := 0; i < 5; i++ {
		b.mu.Lock()
		interval := b.effectiveInterval()
		b.mu.Unlock()
		clk.Advance(interval)
		_ = b.allowAttempt()  // -> HALF-OPEN
		b.recordResult(false) // fail -> OPEN again, doubles (capped)
	}

	b.mu.Lock()
	got := b.effectiveInterval()
	b.mu.Unlock()
	if got != cfg.MaxInterval {
		t.Errorf("effectiveInterval() after repeated failures = %v, want capped at MaxInterval %v", got, cfg.MaxInterval)
	}
}

// TestSSHBackoff_SuccessResetsToClosedAndZeroesBackoff verifies a
// successful probe fully resets the circuit, including the backoff
// schedule (not just the state).
func TestSSHBackoff_SuccessResetsToClosedAndZeroesBackoff(t *testing.T) {
	clk := newFakeClock()
	cfg := sshBackoffConfig{FailureThreshold: 1, BaseInterval: time.Second, MaxInterval: time.Minute}
	b := newSSHBackoffWithClock(cfg, clk)

	_ = b.allowAttempt()
	b.recordResult(false) // OPEN

	clk.Advance(cfg.BaseInterval)
	if err := b.allowAttempt(); err != nil {
		t.Fatalf("allowAttempt() = %v, want nil (HALF-OPEN probe)", err)
	}
	b.recordResult(true) // probe succeeds

	if got := b.State(); got != sshCircuitClosed {
		t.Fatalf("State() after successful probe = %v, want CLOSED", got)
	}
	b.mu.Lock()
	interval := b.effectiveInterval()
	b.mu.Unlock()
	if interval != cfg.BaseInterval {
		t.Errorf("effectiveInterval() after reset = %v, want BaseInterval %v (backoff cleared)", interval, cfg.BaseInterval)
	}
}

// TestSSHRunner_CircuitOpen_SurfacesWithoutBlocking is Story 2.1.2's
// SSHRunner-level acceptance criterion: after a connection drop, the next
// Run call retries with backoff (not immediate retry) and surfaces
// ErrSSHCircuitOpen after the configured max-retry threshold, without
// blocking the caller indefinitely. Simulated by shutting the test sshd
// down after an initial successful dial, so every subsequent redial attempt
// fails fast with connection-refused.
func TestSSHRunner_CircuitOpen_SurfacesWithoutBlocking(t *testing.T) {
	srv := startTestSSHServer(t)
	cfg := newTestClientConfig(t, srv.HostKey)

	backoffCfg := sshBackoffConfig{FailureThreshold: 3, BaseInterval: 50 * time.Millisecond, MaxInterval: time.Second}
	runner := newTestSSHRunner(t, "flaky-remote", srv.Addr, cfg, withSSHBackoffConfig(backoffCfg))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Establish and then kill the connection so the pool's Client.Wait()
	// watcher evicts the dead entry, forcing the next Run to redial.
	if _, err := runner.Run(ctx, "", "echo", "-n", "ok"); err != nil {
		t.Fatalf("initial Run() error: %v", err)
	}
	client, ok := runner.pool.Peek(runner.target.Name)
	if !ok {
		t.Fatal("expected a pooled client after the initial Run()")
	}
	_ = client.Close()
	waitForPoolEviction(t, runner.pool, runner.target.Name)

	// Now take the server down entirely so every redial attempt fails fast
	// (connection refused), driving the backoff toward circuit-open.
	if err := srv.server.Close(); err != nil {
		t.Fatalf("failed to close test server: %v", err)
	}
	_ = srv.listener.Close()

	var lastErr error
	for i := 0; i < backoffCfg.FailureThreshold; i++ {
		_, lastErr = runner.Run(ctx, "", "echo", "-n", "unreachable")
		if lastErr == nil {
			t.Fatalf("attempt %d: Run() succeeded against a closed server, want an error", i)
		}
		if errors.Is(lastErr, ErrSSHCircuitOpen) {
			t.Fatalf("attempt %d: circuit opened after only %d failures, want exactly %d", i, i+1, backoffCfg.FailureThreshold)
		}
	}

	// One more attempt, right after the threshold is reached: the circuit
	// must now be open, and the call must return fast (no dial attempt,
	// no blocking) rather than trying (and failing slowly) again.
	start := time.Now()
	_, err := runner.Run(ctx, "", "echo", "-n", "unreachable")
	elapsed := time.Since(start)
	if !errors.Is(err, ErrSSHCircuitOpen) {
		t.Fatalf("Run() after %d consecutive failures = %v, want ErrSSHCircuitOpen", backoffCfg.FailureThreshold, err)
	}
	if elapsed > 200*time.Millisecond {
		t.Errorf("Run() with an open circuit took %v, want near-instant (no dial attempt)", elapsed)
	}
}

// waitForPoolEviction polls until the pool no longer has an entry for name,
// bounded so a stuck watcher goroutine fails the test instead of hanging it.
func waitForPoolEviction(t *testing.T, pool *SSHClientPool, name string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := pool.Peek(name); !ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("pool entry for %q was not evicted within the deadline", name)
}
