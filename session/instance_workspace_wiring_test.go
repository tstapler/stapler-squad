package session

import (
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
)

// fakeSwitchWorkspaceProcessManager is a ProcessManager double that records the
// kill/restart calls SwitchWorkspace makes, so the test needs no real tmux.
// Embeds a nil ProcessManager so any unanticipated call panics loudly.
type fakeSwitchWorkspaceProcessManager struct {
	ProcessManager
	mu     sync.Mutex
	closes int
	starts int
	alive  bool
}

func (f *fakeSwitchWorkspaceProcessManager) counts() (closes, starts int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closes, f.starts
}

func (f *fakeSwitchWorkspaceProcessManager) WireLaunch(LaunchSpec) {}

func (f *fakeSwitchWorkspaceProcessManager) AppendLaunchEnv(...string) {}

func (f *fakeSwitchWorkspaceProcessManager) ResetExitOnce() {}

func (f *fakeSwitchWorkspaceProcessManager) SetOnExitCallback(func(string)) {}

func (f *fakeSwitchWorkspaceProcessManager) RestoreWithWorkDir(string) error { return nil }

func (f *fakeSwitchWorkspaceProcessManager) GetPTY() (*os.File, error) {
	return nil, errors.New("no pty in fake")
}

func (f *fakeSwitchWorkspaceProcessManager) HasSession() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.alive
}

func (f *fakeSwitchWorkspaceProcessManager) IsAlive() bool { return f.HasSession() }

func (f *fakeSwitchWorkspaceProcessManager) GetPanePID() (int32, error) {
	return 0, errors.New("no pane in fake")
}

func (f *fakeSwitchWorkspaceProcessManager) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closes++
	f.alive = false
	return nil
}

func (f *fakeSwitchWorkspaceProcessManager) Start(string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.starts++
	f.alive = true
	return nil
}

// TestSwitchWorkspace_DoesNotDeadlockOnStartCall is a regression test ensuring
// SwitchWorkspace does not deadlock when it needs to call Start() internally.
//
// Prior to Epic 4, SwitchWorkspace held i.stateMutex.Lock() across its entire body
// including calls to i.Start(false), which also acquires i.stateMutex — a reentrant
// deadlock on a non-reentrant mutex.
//
// After Epic 4, both SwitchWorkspace and Start route through the actor mailbox via
// sendSyncErr, eliminating stateMutex from the hot path. The test is re-targeted to
// use *LiveInstance so the actor goroutine is present and the test reflects real
// production usage.
//
// The instance has no VCS in its temp directory, so VCS detection fails and
// SwitchWorkspace takes the SessionTypeDirectory fallback path, exercising the
// startLocked call inside switchWorkspaceLocked.
func TestSwitchWorkspace_DoesNotDeadlockOnStartCall(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping integration test that restarts a session via a fake process manager")
	}

	instance, cleanup, err := NewTestInstance(t, "switch-workspace-deadlock-regression").
		WithSessionType(domain.SessionTypeDirectory).
		Build()
	require.NoError(t, err)
	if cleanup != nil {
		defer func() { _ = cleanup() }()
	}

	// Wrap in LiveInstance so the actor goroutine is present (production-realistic).
	li := NewLiveInstance(instance)
	defer li.Stop()

	// Swap the real tmux backend for a fake: the restart path would otherwise
	// shell out to real tmux, whose wall time (several seconds, more under
	// package-wide load) is what made this test flaky against its 10s bound.
	fakePM := &fakeSwitchWorkspaceProcessManager{alive: true}
	instance.SetProcessManagerForTest(fakePM)

	// Simulate an already-running session so SwitchWorkspace's guard passes.
	instance.MarkStarted()
	instance.Status = domain.Active

	done := make(chan struct{})
	go func() {
		defer close(done)
		// No VCS in the temp dir, so SwitchWorkspace takes the directory-fallback
		// path and calls startLocked from inside the actor command.
		_, _ = li.SwitchWorkspace(WorkspaceSwitchRequest{
			Type:   SwitchTypeRevision,
			Target: "some-branch",
		})
	}()

	// The timeout should only fire on a real deadlock: with the fake backend the
	// call returns in well under a second (VCS detection dominates).
	select {
	case <-done:
		// Result/error are not asserted: they depend on VCS detection of the temp
		// dir. Kill/restart happen exactly once on either path (checked below).
	case <-time.After(10 * time.Second):
		t.Fatal("SwitchWorkspace did not return within 10s - actor deadlock in switchWorkspaceLocked (see instance_workspace.go)")
	}

	closes, starts := fakePM.counts()
	assert.Equal(t, 1, closes, "the old session must be killed once")
	assert.Equal(t, 1, starts, "startLocked must restart the session once, inside the actor")
}
