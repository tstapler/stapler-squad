package session

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/streamhub"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// TestInstanceStartControlMode_should_NeverProduceTwoOwners_When_100GoroutinesRaceHubRegistryConcurrently
// is validation.md's REQ-8 integration test (Story 3.1.2), exercised against
// the real *session.Instance.StartControlMode path instead of only against
// streamhub.StreamOwnershipLock in isolation
// (streamhub.TestAcquireOwnershipLock_should_ReturnSameLockInstance_When_CalledWithSameSessionName
// and friends cover that). Before this project's fix,
// streamhub.AcquireOwnershipLock was only ever called from
// server/services/connectrpc_websocket.go — Instance.StartControlMode itself
// never touched it, so any direct caller of StartControlMode bypassed
// mutual exclusion entirely. This test proves the fixed
// Instance.StartControlMode (session/instance_tmux.go) now shares the same
// *streamhub.StreamOwnershipLock instance, and the same real critical
// section (AcquireAndResolve/AcquireAndResolveExpecting), that
// HubRegistry.GetOrCreate (server/services/connectrpc_websocket.go) uses in
// production — session/streamhub can't be imported from a
// server/services-level fake here (that would be the reverse of this
// project's one-way session -> session/streamhub dependency and doesn't
// exist anyway), so the hub-creation side of the race is simulated with the
// exact same primitive GetOrCreate itself calls,
// AcquireOwnershipLock(name).AcquireAndResolveExpecting(true, PathHubOwned, ...).
//
// Package session cannot import server/services (server/services already
// imports package session; the reverse would be an import cycle), so this
// test simulates HubRegistry.GetOrCreate's ownership half directly via
// streamhub.AcquireOwnershipLock.AcquireAndResolveExpecting — the exact
// primitive GetOrCreate itself calls — rather than the full hub type.
func TestInstanceStartControlMode_should_NeverProduceTwoOwners_When_100GoroutinesRaceHubRegistryConcurrently(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available")
	}

	name := fmt.Sprintf("ssq-ownership-race-%d-%d", os.Getpid(), time.Now().UnixNano())
	tmuxSession, cleanup := tmux.NewTmuxSessionWithPrefixAndCleanup(name, "sleep 60", "staplersquad_test_")
	t.Cleanup(func() {
		if err := cleanup(); err != nil {
			t.Logf("tmux cleanup for %s: %v", name, err)
		}
	})
	if err := tmuxSession.Start(t.TempDir()); err != nil {
		t.Fatalf("failed to start real tmux session %s: %v", name, err)
	}

	inst := &Instance{Title: name}
	_ = inst.pm() // lazily initializes processManager as *TmuxBackend before SetTmuxSession
	inst.SetTmuxSession(tmuxSession)
	t.Cleanup(func() { _ = inst.StopControlMode() })

	tmuxName := inst.GetTmuxSessionName()
	if tmuxName == "" {
		t.Fatalf("expected Instance.GetTmuxSessionName() to be non-empty after SetTmuxSession")
	}

	const n = 100
	var wg sync.WaitGroup
	var startErrs, hubErrs, hubWins, legacyStarts atomic.Int64

	// Each StartControlMode call is NOT paired with an immediate concurrent
	// StopControlMode here (unlike an earlier version of this test): pairing
	// them surfaced a genuine, pre-existing data race in
	// session/tmux/control_mode.go's own refcounting (processControlModeLine's
	// write vs StopControlMode's read at control_mode.go:487/210, with no
	// streamhub code on either side of it) when many Start/Stop pairs
	// interleave concurrently against one real tmux session — filed as
	// BUG-095, out of scope for this project's ownership-lock fix. Calling
	// Stop once via t.Cleanup after every Start has completed (below) still
	// fully exercises the ownership-lock invariant this test targets
	// (StartControlMode and a concurrent simulated HubRegistry.GetOrCreate
	// never both proceed as if they owned the session) without also
	// exercising that unrelated refcounting race.
	for i := 0; i < n; i++ {
		wg.Add(2)
		// One goroutine calls the real Instance.StartControlMode() — the
		// legacy per-connection entry point, now wired to
		// streamhub.AcquireOwnershipLock per Story 3.1.2's fix.
		go func() {
			defer wg.Done()
			if err := inst.StartControlMode(); err != nil {
				startErrs.Add(1)
				return
			}
			legacyStarts.Add(1)
		}()
		// The other simulates HubRegistry.GetOrCreate's ownership check —
		// the exact call GetOrCreate makes before ever creating a hub.
		go func() {
			defer wg.Done()
			err := streamhub.AcquireOwnershipLock(tmuxName).AcquireAndResolveExpecting(true, streamhub.PathHubOwned, func() error {
				hubWins.Add(1)
				return nil
			})
			if err != nil {
				hubErrs.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := startErrs.Load(); got != 0 {
		t.Errorf("Instance.StartControlMode must never fail here (refcounted, safe under either resolved path): got %d errors", got)
	}
	if got := legacyStarts.Load(); got != n {
		t.Errorf("expected all %d StartControlMode calls to succeed, got %d", n, got)
	}

	// The sticky resolution is whatever the very first Resolve call (from
	// either side) observed — the real invariant under test is that it is
	// singular and that the losing side never proceeded as if it had won.
	resolved := streamhub.AcquireOwnershipLock(tmuxName).Resolve(true)
	switch resolved {
	case streamhub.PathHubOwned:
		if got := hubWins.Load(); got != n {
			t.Errorf("resolved PathHubOwned: expected all %d simulated hub-creation attempts to win, got %d (hubErrs=%d) — a losing attempt that still ran fn would mean two owners existed", n, got, hubErrs.Load())
		}
		if got := hubErrs.Load(); got != 0 {
			t.Errorf("resolved PathHubOwned: expected 0 ErrOwnershipResolvedToOtherPath, got %d", got)
		}
	case streamhub.PathLegacyPerConnection:
		if got := hubErrs.Load(); got != n {
			t.Errorf("resolved PathLegacyPerConnection: expected all %d simulated hub-creation attempts to be refused with ErrOwnershipResolvedToOtherPath, got %d refused (hubWins=%d) — a hub-creation attempt that ran fn anyway would mean two owners existed", n, got, hubWins.Load())
		}
		if got := hubWins.Load(); got != 0 {
			t.Errorf("resolved PathLegacyPerConnection: expected 0 simulated hub-creation wins, got %d", got)
		}
	default:
		t.Fatalf("unexpected resolved StreamPath: %v", resolved)
	}
}
