package executor

import (
	"context"
	"errors"
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock Executor ---

type mockExecutor struct {
	runFunc            func(cmd *exec.Cmd) error
	outputFunc         func(cmd *exec.Cmd) ([]byte, error)
	combinedOutputFunc func(cmd *exec.Cmd) ([]byte, error)
}

func (m *mockExecutor) Run(cmd *exec.Cmd) error {
	if m.runFunc != nil {
		return m.runFunc(cmd)
	}
	return nil
}

func (m *mockExecutor) Output(cmd *exec.Cmd) ([]byte, error) {
	if m.outputFunc != nil {
		return m.outputFunc(cmd)
	}
	return []byte("ok"), nil
}

func (m *mockExecutor) CombinedOutput(cmd *exec.Cmd) ([]byte, error) {
	if m.combinedOutputFunc != nil {
		return m.combinedOutputFunc(cmd)
	}
	if m.outputFunc != nil {
		return m.outputFunc(cmd)
	}
	return []byte("ok"), nil
}

// --- Mock Clock ---

type mockClock struct {
	mu  sync.Mutex
	now time.Time
}

func newMockClock(t time.Time) *mockClock {
	return &mockClock{now: t}
}

func (c *mockClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *mockClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// --- Helpers ---

var errSimulated = errors.New("simulated failure")

func failingExecutor() *mockExecutor {
	return &mockExecutor{
		runFunc: func(cmd *exec.Cmd) error {
			return errSimulated
		},
		outputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return nil, errSimulated
		},
	}
}

func succeedingExecutor() *mockExecutor {
	return &mockExecutor{
		runFunc: func(cmd *exec.Cmd) error {
			return nil
		},
		outputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("success"), nil
		},
	}
}

func dummyCmd(name string, args ...string) *exec.Cmd {
	return exec.CommandContext(context.Background(), name, args...)
}

// --- commandClass tests ---

func TestCommandClass(t *testing.T) {
	tests := []struct {
		name     string
		cmd      *exec.Cmd
		expected string
	}{
		{
			name:     "nil command",
			cmd:      nil,
			expected: "unknown",
		},
		{
			name:     "simple command",
			cmd:      exec.CommandContext(context.Background(), "git"),
			expected: "git",
		},
		{
			name:     "command with subcommand",
			cmd:      exec.CommandContext(context.Background(), "git", "diff"),
			expected: "git-diff",
		},
		{
			name:     "command with flags before subcommand",
			cmd:      exec.CommandContext(context.Background(), "git", "--no-pager", "log"),
			expected: "git-log",
		},
		{
			name:     "command with only flags",
			cmd:      exec.CommandContext(context.Background(), "ls", "-la", "-R"),
			expected: "ls",
		},
		{
			name:     "tmux capture-pane",
			cmd:      exec.CommandContext(context.Background(), "tmux", "capture-pane", "-t", "session"),
			expected: "tmux-capture-pane",
		},
		{
			name:     "full path binary",
			cmd:      exec.CommandContext(context.Background(), "/usr/bin/git", "status"),
			expected: "git-status",
		},
		{
			name:     "tmux with -L socket flag before subcommand",
			cmd:      exec.CommandContext(context.Background(), "tmux", "-L", "mysocket", "list-sessions", "-F", "#{session_name}"),
			expected: "tmux-list-sessions",
		},
		{
			name:     "tmux new-session with -L socket flag",
			cmd:      exec.CommandContext(context.Background(), "tmux", "-L", "mysocket", "new-session", "-d", "-s", "myname"),
			expected: "tmux-new-session",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := commandClass(tc.cmd)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// --- CircuitState String tests ---

func TestCircuitStateString(t *testing.T) {
	assert.Equal(t, "CLOSED", CircuitClosed.String())
	assert.Equal(t, "OPEN", CircuitOpen.String())
	assert.Equal(t, "HALF-OPEN", CircuitHalfOpen.String())
	assert.Equal(t, "UNKNOWN", CircuitState(99).String())
}

// --- DefaultCircuitBreakerConfig tests ---

func TestDefaultCircuitBreakerConfig(t *testing.T) {
	cfg := DefaultCircuitBreakerConfig()
	assert.Equal(t, 3, cfg.FailureThreshold)
	assert.Equal(t, 30*time.Second, cfg.RecoveryTimeout)
	assert.Equal(t, 5*time.Minute, cfg.MaxRecoveryTimeout)
}

// --- Core state transition tests (table-driven) ---

func TestCircuitBreakerStateTransitions(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, cbe *CircuitBreakerExecutor, clock *mockClock)
	}{
		{
			name: "Closed to Open: consecutive failures trip the breaker",
			setup: func(t *testing.T, cbe *CircuitBreakerExecutor, clock *mockClock) {
				// 3 consecutive failures should trip the breaker (threshold = 3).
				for i := 0; i < 3; i++ {
					err := cbe.Run(dummyCmd("git", "diff"))
					assert.ErrorIs(t, err, errSimulated, "failure %d should propagate", i+1)
				}

				// The 4th call should be rejected by the open breaker.
				err := cbe.Run(dummyCmd("git", "diff"))
				assert.ErrorIs(t, err, ErrCircuitOpen, "breaker should be open after 3 failures")

				// Verify state via snapshot.
				snaps := cbe.AllBreakers()
				snap, ok := snaps["git-diff"]
				require.True(t, ok, "should have a breaker for git-diff")
				assert.Equal(t, CircuitOpen, snap.State)
				assert.Equal(t, 3, snap.ConsecutiveFailures)
			},
		},
		{
			name: "Open rejects calls with ErrCircuitOpen",
			setup: func(t *testing.T, cbe *CircuitBreakerExecutor, clock *mockClock) {
				// Trip the breaker.
				for i := 0; i < 3; i++ {
					_ = cbe.Run(dummyCmd("git", "status"))
				}

				// Multiple calls should all be rejected immediately.
				for i := 0; i < 5; i++ {
					err := cbe.Run(dummyCmd("git", "status"))
					assert.ErrorIs(t, err, ErrCircuitOpen, "call %d should be rejected", i+1)
				}
			},
		},
		{
			name: "Open to Half-Open: recovery timeout allows one probe",
			setup: func(t *testing.T, cbe *CircuitBreakerExecutor, clock *mockClock) {
				// Trip the breaker.
				for i := 0; i < 3; i++ {
					_ = cbe.Run(dummyCmd("git", "diff"))
				}

				// Verify open.
				err := cbe.Run(dummyCmd("git", "diff"))
				assert.ErrorIs(t, err, ErrCircuitOpen)

				// Advance clock past recovery timeout.
				clock.Advance(31 * time.Second)

				// Swap delegate to a succeeding executor so the probe succeeds.
				cbe.delegate = succeedingExecutor()

				// The next call should be allowed as a half-open probe.
				err = cbe.Run(dummyCmd("git", "diff"))
				assert.NoError(t, err, "probe should be allowed after recovery timeout")
			},
		},
		{
			name: "Half-Open to Closed: successful probe resets breaker",
			setup: func(t *testing.T, cbe *CircuitBreakerExecutor, clock *mockClock) {
				// Trip the breaker.
				for i := 0; i < 3; i++ {
					_ = cbe.Run(dummyCmd("git", "log"))
				}

				// Advance past recovery.
				clock.Advance(31 * time.Second)

				// Swap to succeeding executor.
				cbe.delegate = succeedingExecutor()

				// Probe succeeds -> should transition to CLOSED.
				err := cbe.Run(dummyCmd("git", "log"))
				assert.NoError(t, err)

				// Verify breaker is now CLOSED.
				snaps := cbe.AllBreakers()
				snap := snaps["git-log"]
				assert.Equal(t, CircuitClosed, snap.State)
				assert.Equal(t, 0, snap.ConsecutiveFailures)

				// Subsequent calls should work normally.
				err = cbe.Run(dummyCmd("git", "log"))
				assert.NoError(t, err)
			},
		},
		{
			name: "Half-Open to Open: failed probe re-opens breaker",
			setup: func(t *testing.T, cbe *CircuitBreakerExecutor, clock *mockClock) {
				// Trip the breaker.
				for i := 0; i < 3; i++ {
					_ = cbe.Run(dummyCmd("git", "push"))
				}

				// Advance past recovery.
				clock.Advance(31 * time.Second)

				// Delegate still fails -- probe will fail.
				err := cbe.Run(dummyCmd("git", "push"))
				assert.ErrorIs(t, err, errSimulated, "probe should execute and fail")

				// Breaker should be OPEN again.
				snaps := cbe.AllBreakers()
				snap := snaps["git-push"]
				assert.Equal(t, CircuitOpen, snap.State)

				// Calls should be rejected.
				err = cbe.Run(dummyCmd("git", "push"))
				assert.ErrorIs(t, err, ErrCircuitOpen)
			},
		},
		{
			name: "Closed stays Closed: intermittent failures do not trip",
			setup: func(t *testing.T, cbe *CircuitBreakerExecutor, clock *mockClock) {
				// Alternate: fail, succeed, fail, succeed, fail, succeed.
				// Consecutive failures never reach threshold of 3.
				fail := failingExecutor()
				succeed := succeedingExecutor()

				for i := 0; i < 3; i++ {
					cbe.delegate = fail
					_ = cbe.Run(dummyCmd("git", "fetch"))

					cbe.delegate = succeed
					err := cbe.Run(dummyCmd("git", "fetch"))
					assert.NoError(t, err)
				}

				// Breaker should still be CLOSED.
				snaps := cbe.AllBreakers()
				snap := snaps["git-fetch"]
				assert.Equal(t, CircuitClosed, snap.State)
				assert.Equal(t, 0, snap.ConsecutiveFailures)
			},
		},
		{
			name: "Command-class isolation: independent breakers for different classes",
			setup: func(t *testing.T, cbe *CircuitBreakerExecutor, clock *mockClock) {
				// Trip the breaker for "git-diff" only.
				for i := 0; i < 3; i++ {
					_ = cbe.Run(dummyCmd("git", "diff"))
				}

				// git-diff should be open.
				err := cbe.Run(dummyCmd("git", "diff"))
				assert.ErrorIs(t, err, ErrCircuitOpen)

				// git-status should still work (independent breaker).
				cbe.delegate = succeedingExecutor()
				err = cbe.Run(dummyCmd("git", "status"))
				assert.NoError(t, err)

				// tmux-capture-pane should also work.
				err = cbe.Run(dummyCmd("tmux", "capture-pane"))
				assert.NoError(t, err)

				// Verify independent states.
				snaps := cbe.AllBreakers()
				assert.Equal(t, CircuitOpen, snaps["git-diff"].State)
				assert.Equal(t, CircuitClosed, snaps["git-status"].State)
				assert.Equal(t, CircuitClosed, snaps["tmux-capture-pane"].State)
			},
		},
		{
			name: "Config validation: custom thresholds respected",
			setup: func(t *testing.T, _ *CircuitBreakerExecutor, clock *mockClock) {
				// Create a new executor with custom config.
				customCfg := CircuitBreakerConfig{
					FailureThreshold: 5,
					RecoveryTimeout:  10 * time.Second,
				}
				customCBE := NewCircuitBreakerExecutorWithClock(failingExecutor(), customCfg, clock)

				// 4 failures should NOT trip (threshold is 5).
				for i := 0; i < 4; i++ {
					err := customCBE.Run(dummyCmd("git", "pull"))
					assert.ErrorIs(t, err, errSimulated)
				}
				snaps := customCBE.AllBreakers()
				assert.Equal(t, CircuitClosed, snaps["git-pull"].State)

				// 5th failure trips it.
				_ = customCBE.Run(dummyCmd("git", "pull"))
				snaps = customCBE.AllBreakers()
				assert.Equal(t, CircuitOpen, snaps["git-pull"].State)

				// 10s recovery (not 30s default).
				clock.Advance(11 * time.Second)
				customCBE.delegate = succeedingExecutor()
				err := customCBE.Run(dummyCmd("git", "pull"))
				assert.NoError(t, err, "should recover after custom timeout")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clock := newMockClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
			cfg := CircuitBreakerConfig{
				FailureThreshold: 3,
				RecoveryTimeout:  30 * time.Second,
			}
			cbe := NewCircuitBreakerExecutorWithClock(failingExecutor(), cfg, clock)
			tc.setup(t, cbe, clock)
		})
	}
}

// --- Output method tests ---

func TestCircuitBreakerExecutor_Output(t *testing.T) {
	clock := newMockClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	cfg := CircuitBreakerConfig{
		FailureThreshold: 2,
		RecoveryTimeout:  10 * time.Second,
	}

	t.Run("Output succeeds through closed breaker", func(t *testing.T) {
		cbe := NewCircuitBreakerExecutorWithClock(succeedingExecutor(), cfg, clock)
		out, err := cbe.Output(dummyCmd("git", "log"))
		assert.NoError(t, err)
		assert.Equal(t, []byte("success"), out)
	})

	t.Run("Output returns ErrCircuitOpen when breaker is open", func(t *testing.T) {
		cbe := NewCircuitBreakerExecutorWithClock(failingExecutor(), cfg, clock)

		// Trip breaker (threshold=2).
		for i := 0; i < 2; i++ {
			_, _ = cbe.Output(dummyCmd("git", "show"))
		}

		out, err := cbe.Output(dummyCmd("git", "show"))
		assert.ErrorIs(t, err, ErrCircuitOpen)
		assert.Nil(t, out)
	})

	t.Run("Output probe recovers breaker", func(t *testing.T) {
		cbe := NewCircuitBreakerExecutorWithClock(failingExecutor(), cfg, clock)

		// Trip breaker.
		for i := 0; i < 2; i++ {
			_, _ = cbe.Output(dummyCmd("git", "blame"))
		}

		// Advance past recovery.
		clock.Advance(11 * time.Second)
		cbe.delegate = succeedingExecutor()

		out, err := cbe.Output(dummyCmd("git", "blame"))
		assert.NoError(t, err)
		assert.Equal(t, []byte("success"), out)

		// Verify closed.
		snaps := cbe.AllBreakers()
		assert.Equal(t, CircuitClosed, snaps["git-blame"].State)
	})
}

// --- Concurrent access test ---

func TestCircuitBreakerExecutor_ConcurrentAccess(t *testing.T) {
	clock := newMockClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	cfg := CircuitBreakerConfig{
		FailureThreshold: 3,
		RecoveryTimeout:  30 * time.Second,
	}
	cbe := NewCircuitBreakerExecutorWithClock(succeedingExecutor(), cfg, clock)

	const numGoroutines = 50
	const numCallsPerGoroutine = 20
	var wg sync.WaitGroup
	var successCount atomic.Int64
	var errorCount atomic.Int64

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			// Use a mix of command classes.
			cmds := [][]string{
				{"git", "diff"},
				{"git", "status"},
				{"tmux", "capture-pane"},
			}
			for j := 0; j < numCallsPerGoroutine; j++ {
				cmdArgs := cmds[j%len(cmds)]
				cmd := dummyCmd(cmdArgs[0], cmdArgs[1:]...)
				err := cbe.Run(cmd)
				if err == nil {
					successCount.Add(1)
				} else {
					errorCount.Add(1)
				}
			}
		}(i)
	}

	wg.Wait()

	// All calls should succeed since the delegate always succeeds.
	total := numGoroutines * numCallsPerGoroutine
	assert.Equal(t, int64(total), successCount.Load(), "all calls should succeed")
	assert.Equal(t, int64(0), errorCount.Load(), "no errors expected")

	// All breakers should be in CLOSED state.
	snaps := cbe.AllBreakers()
	for class, snap := range snaps {
		assert.Equal(t, CircuitClosed, snap.State, "breaker %s should be CLOSED", class)
	}
}

// --- Half-Open serialization test (BUG-003 prevention) ---

func TestCircuitBreakerExecutor_HalfOpenSerializesProbes(t *testing.T) {
	clock := newMockClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	cfg := CircuitBreakerConfig{
		FailureThreshold: 3,
		RecoveryTimeout:  30 * time.Second,
	}

	// The probe executor blocks until we release it.
	probeStarted := make(chan struct{}, 1)
	probeRelease := make(chan struct{})
	var probeCount atomic.Int64

	blockingExec := &mockExecutor{
		runFunc: func(cmd *exec.Cmd) error {
			probeCount.Add(1)
			probeStarted <- struct{}{}
			<-probeRelease
			return nil // success
		},
	}

	cbe := NewCircuitBreakerExecutorWithClock(failingExecutor(), cfg, clock)

	// Trip the breaker.
	for i := 0; i < 3; i++ {
		_ = cbe.Run(dummyCmd("git", "diff"))
	}

	// Advance past recovery timeout.
	clock.Advance(31 * time.Second)

	// Switch to blocking executor for the probe.
	cbe.delegate = blockingExec

	// Launch the probe goroutine.
	var probeErr error
	var probeDone sync.WaitGroup
	probeDone.Add(1)
	go func() {
		defer probeDone.Done()
		probeErr = cbe.Run(dummyCmd("git", "diff"))
	}()

	// Wait for the probe to start executing.
	<-probeStarted

	// While the probe is in flight, launch concurrent requests.
	// They should all be rejected since exactly one probe is allowed.
	const concurrentAttempts = 10
	rejectedCount := atomic.Int64{}
	var attemptWg sync.WaitGroup
	attemptWg.Add(concurrentAttempts)
	for i := 0; i < concurrentAttempts; i++ {
		go func() {
			defer attemptWg.Done()
			err := cbe.Run(dummyCmd("git", "diff"))
			if errors.Is(err, ErrCircuitOpen) {
				rejectedCount.Add(1)
			}
		}()
	}

	attemptWg.Wait()

	// All concurrent attempts should have been rejected.
	assert.Equal(t, int64(concurrentAttempts), rejectedCount.Load(),
		"all concurrent attempts during HALF-OPEN probe should be rejected")

	// Only one probe should have started.
	assert.Equal(t, int64(1), probeCount.Load(), "exactly one probe should execute")

	// Release the probe.
	close(probeRelease)
	probeDone.Wait()

	assert.NoError(t, probeErr, "probe should succeed")

	// Breaker should now be CLOSED.
	snaps := cbe.AllBreakers()
	assert.Equal(t, CircuitClosed, snaps["git-diff"].State)
}

// --- AllBreakers snapshot test ---

func TestCircuitBreakerExecutor_AllBreakers(t *testing.T) {
	clock := newMockClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	cfg := CircuitBreakerConfig{
		FailureThreshold: 3,
		RecoveryTimeout:  30 * time.Second,
	}
	cbe := NewCircuitBreakerExecutorWithClock(succeedingExecutor(), cfg, clock)

	// No breakers initially.
	snaps := cbe.AllBreakers()
	assert.Empty(t, snaps)

	// Create some breakers.
	_ = cbe.Run(dummyCmd("git", "diff"))
	_ = cbe.Run(dummyCmd("git", "status"))
	_ = cbe.Run(dummyCmd("tmux", "capture-pane"))

	snaps = cbe.AllBreakers()
	assert.Len(t, snaps, 3)
	assert.Contains(t, snaps, "git-diff")
	assert.Contains(t, snaps, "git-status")
	assert.Contains(t, snaps, "tmux-capture-pane")

	for _, snap := range snaps {
		assert.Equal(t, CircuitClosed, snap.State)
		assert.Equal(t, 0, snap.ConsecutiveFailures)
		assert.Equal(t, cfg.FailureThreshold, snap.Config.FailureThreshold)
		assert.Equal(t, cfg.RecoveryTimeout, snap.Config.RecoveryTimeout)
	}
}

// --- NewCircuitBreakerExecutor (real clock) ---

func TestNewCircuitBreakerExecutor_UsesRealClock(t *testing.T) {
	cbe := NewCircuitBreakerExecutor(succeedingExecutor(), DefaultCircuitBreakerConfig())
	assert.NotNil(t, cbe)
	assert.NotNil(t, cbe.clock)

	// Should work with real time.
	err := cbe.Run(dummyCmd("echo", "hello"))
	assert.NoError(t, err)
}

// --- Reset / ResetAll tests ---

func TestCircuitBreakerExecutor_Reset_FromOpen(t *testing.T) {
	clock := newMockClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	cfg := CircuitBreakerConfig{
		FailureThreshold: 2,
		RecoveryTimeout:  30 * time.Second,
	}
	cbe := NewCircuitBreakerExecutorWithClock(failingExecutor(), cfg, clock)

	// Trip breaker to OPEN.
	for i := 0; i < 2; i++ {
		_ = cbe.Run(dummyCmd("tmux", "list-sessions"))
	}
	require.Equal(t, CircuitOpen, cbe.AllBreakers()["tmux-list-sessions"].State)

	// Reset brings it back to CLOSED.
	cbe.Reset()
	snap := cbe.AllBreakers()["tmux-list-sessions"]
	assert.Equal(t, CircuitClosed, snap.State)
	assert.Equal(t, 0, snap.ConsecutiveFailures)
	assert.False(t, snap.LastStateChange.IsZero())
}

func TestCircuitBreakerExecutor_Reset_FromHalfOpen(t *testing.T) {
	clock := newMockClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	cfg := CircuitBreakerConfig{
		FailureThreshold: 2,
		RecoveryTimeout:  10 * time.Second,
	}
	cbe := NewCircuitBreakerExecutorWithClock(failingExecutor(), cfg, clock)

	// Trip breaker to OPEN.
	for i := 0; i < 2; i++ {
		_ = cbe.Run(dummyCmd("tmux", "list-sessions"))
	}

	// Advance past recovery timeout to allow HALF-OPEN transition.
	clock.Advance(11 * time.Second)

	// One probe attempt transitions to HALF-OPEN.
	_ = cbe.Run(dummyCmd("tmux", "list-sessions"))
	require.Equal(t, CircuitOpen, cbe.AllBreakers()["tmux-list-sessions"].State) // failed probe → back to OPEN

	// Reset while in OPEN should still restore to CLOSED.
	cbe.Reset()
	snap := cbe.AllBreakers()["tmux-list-sessions"]
	assert.Equal(t, CircuitClosed, snap.State)
}

func TestCircuitBreakerRegistry_ResetAll(t *testing.T) {
	clock := newMockClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	cfg := CircuitBreakerConfig{
		FailureThreshold: 2,
		RecoveryTimeout:  30 * time.Second,
	}

	// Create two executors and register them in a local registry.
	registry := &CircuitBreakerRegistry{
		executors: make(map[string]*CircuitBreakerExecutor),
	}

	cbe1 := NewCircuitBreakerExecutorWithClock(failingExecutor(), cfg, clock)
	cbe2 := NewCircuitBreakerExecutorWithClock(failingExecutor(), cfg, clock)
	registry.Register("tmux-session1", cbe1)
	registry.Register("tmux-session2", cbe2)

	// Trip breakers in both executors.
	for i := 0; i < 2; i++ {
		_ = cbe1.Run(dummyCmd("tmux", "list-sessions"))
		_ = cbe2.Run(dummyCmd("tmux", "has-session"))
	}
	require.Equal(t, CircuitOpen, cbe1.AllBreakers()["tmux-list-sessions"].State)
	require.Equal(t, CircuitOpen, cbe2.AllBreakers()["tmux-has-session"].State)

	// ResetAll restores both executors.
	registry.ResetAll()

	snap1 := cbe1.AllBreakers()["tmux-list-sessions"]
	snap2 := cbe2.AllBreakers()["tmux-has-session"]
	assert.Equal(t, CircuitClosed, snap1.State, "executor 1 breaker should be CLOSED after ResetAll")
	assert.Equal(t, CircuitClosed, snap2.State, "executor 2 breaker should be CLOSED after ResetAll")
}

func TestCircuitBreakerRegistry_Unregister(t *testing.T) {
	clock := newMockClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	cfg := DefaultCircuitBreakerConfig()

	registry := &CircuitBreakerRegistry{
		executors: make(map[string]*CircuitBreakerExecutor),
	}
	cbe := NewCircuitBreakerExecutorWithClock(failingExecutor(), cfg, clock)
	registry.Register("key", cbe)

	// Run a command through the executor to create a breaker entry.
	_ = cbe.Run(dummyCmd("tmux", "list-sessions"))

	// AllBreakers prefixes keys as "<execKey>/<breakerKey>", so after running
	// "tmux list-sessions" through the executor registered as "key", we expect
	// the composite key "key/tmux-list-sessions" to be present.
	snapsBefore := registry.AllBreakers()
	require.Contains(t, snapsBefore, "key/tmux-list-sessions",
		"breaker should appear under executor key before unregister")

	registry.Unregister("key")

	// After unregister the executor's breakers are no longer aggregated.
	snapsAfter := registry.AllBreakers()
	_, found := snapsAfter["key/tmux-list-sessions"]
	assert.False(t, found, "unregistered executor breakers should not appear in AllBreakers")
	assert.Empty(t, snapsAfter, "AllBreakers should be empty after unregistering the only executor")
}

// TestCircuitBreakerExecutor_IsFailure verifies that a custom IsFailure classifier controls
// which errors count as circuit breaker failures.
// TestCircuitBreakerExponentialBackoff verifies that successive failed HALF-OPEN probes
// double the recovery timeout up to MaxRecoveryTimeout.
func TestCircuitBreakerExponentialBackoff(t *testing.T) {
	clock := newMockClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	cfg := CircuitBreakerConfig{
		FailureThreshold:   3,
		RecoveryTimeout:    30 * time.Second,
		MaxRecoveryTimeout: 5 * time.Minute,
	}
	cbe := NewCircuitBreakerExecutorWithClock(failingExecutor(), cfg, clock)

	// Trip the breaker (consecutiveOpenTrips=0, effectiveTimeout=30s).
	for i := 0; i < 3; i++ {
		_ = cbe.Run(dummyCmd("git", "diff"))
	}
	snap := cbe.AllBreakers()["git-diff"]
	require.Equal(t, CircuitOpen, snap.State)
	assert.Equal(t, 0, snap.ConsecutiveOpenTrips)
	assert.Equal(t, 30*time.Second, snap.EffectiveRecovery)

	// First probe fails: effectiveTimeout doubles to 60s.
	clock.Advance(31 * time.Second) // past 30s
	_ = cbe.Run(dummyCmd("git", "diff"))
	snap = cbe.AllBreakers()["git-diff"]
	require.Equal(t, CircuitOpen, snap.State)
	assert.Equal(t, 1, snap.ConsecutiveOpenTrips)
	assert.Equal(t, 60*time.Second, snap.EffectiveRecovery)

	// 30s after re-open: still not enough (need 60s).
	clock.Advance(30 * time.Second)
	err := cbe.Run(dummyCmd("git", "diff"))
	assert.ErrorIs(t, err, ErrCircuitOpen, "30s < 60s backoff: should still be open")

	// Second probe fails: effectiveTimeout doubles to 120s.
	clock.Advance(31 * time.Second) // 61s since re-open → allowed
	_ = cbe.Run(dummyCmd("git", "diff"))
	snap = cbe.AllBreakers()["git-diff"]
	require.Equal(t, CircuitOpen, snap.State)
	assert.Equal(t, 2, snap.ConsecutiveOpenTrips)
	assert.Equal(t, 120*time.Second, snap.EffectiveRecovery)

	// Third probe fails: effectiveTimeout would be 240s but capped at 5min=300s (no cap yet).
	clock.Advance(121 * time.Second)
	_ = cbe.Run(dummyCmd("git", "diff"))
	snap = cbe.AllBreakers()["git-diff"]
	require.Equal(t, CircuitOpen, snap.State)
	assert.Equal(t, 3, snap.ConsecutiveOpenTrips)
	assert.Equal(t, 240*time.Second, snap.EffectiveRecovery)

	// Fourth probe fails: effectiveTimeout would be 480s → capped at 300s (5min).
	clock.Advance(241 * time.Second)
	_ = cbe.Run(dummyCmd("git", "diff"))
	snap = cbe.AllBreakers()["git-diff"]
	require.Equal(t, CircuitOpen, snap.State)
	assert.Equal(t, 4, snap.ConsecutiveOpenTrips)
	assert.Equal(t, 5*time.Minute, snap.EffectiveRecovery, "timeout should be capped at MaxRecoveryTimeout")

	// Subsequent failures stay at the cap.
	clock.Advance(301 * time.Second)
	_ = cbe.Run(dummyCmd("git", "diff"))
	snap = cbe.AllBreakers()["git-diff"]
	assert.Equal(t, 5*time.Minute, snap.EffectiveRecovery, "cap should persist")

	// Successful probe resets all backoff state.
	clock.Advance(301 * time.Second)
	cbe.delegate = succeedingExecutor()
	err = cbe.Run(dummyCmd("git", "diff"))
	assert.NoError(t, err)
	snap = cbe.AllBreakers()["git-diff"]
	assert.Equal(t, CircuitClosed, snap.State)
	assert.Equal(t, 0, snap.ConsecutiveOpenTrips)
	assert.Equal(t, 30*time.Second, snap.EffectiveRecovery, "base timeout restored after close")
}

func TestCircuitBreakerExecutor_IsFailure(t *testing.T) {
	softErr := errors.New("soft error: no sessions")
	hardErr := errors.New("hard error: no server running")

	config := CircuitBreakerConfig{
		FailureThreshold: 3,
		RecoveryTimeout:  30 * time.Second,
		// Only treat hard errors as real failures
		IsFailure: func(class string, output []byte, err error) bool {
			if err == nil {
				return false
			}
			return err == hardErr
		},
	}

	mock := &mockExecutor{}
	cbe := NewCircuitBreakerExecutor(mock, config)
	cmd := exec.CommandContext(context.Background(), "tmux", "list-sessions")

	t.Run("soft errors do not trip the breaker", func(t *testing.T) {
		mock.combinedOutputFunc = func(_ *exec.Cmd) ([]byte, error) {
			return nil, softErr
		}
		// Exceed the failure threshold with soft errors
		for i := 0; i < config.FailureThreshold+2; i++ {
			_, err := cbe.CombinedOutput(cmd)
			require.Equal(t, softErr, err, "should pass through the original error")
		}
		// Breaker should remain closed — soft errors don't count
		snap := cbe.AllBreakers()["tmux-list-sessions"]
		assert.Equal(t, CircuitClosed, snap.State, "breaker should stay closed for soft errors")
		assert.Equal(t, 0, snap.ConsecutiveFailures, "soft errors should not accumulate failures")
	})

	t.Run("hard errors trip the breaker after threshold", func(t *testing.T) {
		mock.combinedOutputFunc = func(_ *exec.Cmd) ([]byte, error) {
			return nil, hardErr
		}
		for i := 0; i < config.FailureThreshold; i++ {
			_, _ = cbe.CombinedOutput(cmd)
		}
		snap := cbe.AllBreakers()["tmux-list-sessions"]
		assert.Equal(t, CircuitOpen, snap.State, "breaker should open after hard error threshold")
	})
}
