package session

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/launchcmd"
	"github.com/tstapler/stapler-squad/session/tymux"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

// seamFakeTymux is a tymux.TymuxManager that records identity-relevant calls.
type seamFakeTymux struct {
	tymux.TymuxManager // nil; any unlisted method must not be reached
	id                 string
	gotStoredID        string
	closeCalls         int
}

func (f *seamFakeTymux) SetStoredSessionID(id string)       { f.gotStoredID = id }
func (f *seamFakeTymux) SetSessionLabel(string)             {}
func (f *seamFakeTymux) SetCommand(launchcmd.LaunchCommand) {}
func (f *seamFakeTymux) GetSessionIdentifier() string       { return f.id }
func (f *seamFakeTymux) PersistedSessionID() string {
	if f.id != "" {
		return f.id
	}
	return f.gotStoredID
}
func (f *seamFakeTymux) HasSession() bool { return false }
func (f *seamFakeTymux) Close() error     { f.closeCalls++; return nil }

// useSeamFake routes every new tymux manager to fake for the test.
func useSeamFake(t *testing.T) *seamFakeTymux {
	t.Helper()
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	fake := &seamFakeTymux{}
	orig := newTymuxSessionFn
	newTymuxSessionFn = func() tymux.TymuxManager { return fake }
	t.Cleanup(func() { newTymuxSessionFn = orig })
	return fake
}

// A restored but not yet started Instance must re-serialize the daemon id it
// was restored with; otherwise a re-save drops ownership of a live session.
func TestSeam_UnstartedRestoredInstanceKeepsStoredDaemonID(t *testing.T) {
	useSeamFake(t)
	data := restoredPausedData(t, "unstarted")
	data.Backend = BackendTymux
	data.TymuxSessionID = "d-77"

	inst, err := FromInstanceDataDeferred(data)
	require.NoError(t, err)

	assert.Equal(t, "d-77", inst.ToInstanceData().TymuxSessionID)
}

// KillSession on a restored-but-unstarted tymux record (e.g. deleting a Stopped
// session after a restart) must release its daemon session, not leak it.
func TestSeam_KillSessionReleasesDaemonSessionOfUnrestoredRecord(t *testing.T) {
	fake := useSeamFake(t)
	data := restoredPausedData(t, "kill-unrestored")
	data.Backend = BackendTymux
	data.TymuxSessionID = "d-78"
	inst, err := FromInstanceDataDeferred(data)
	require.NoError(t, err)

	require.NoError(t, inst.KillSession())

	assert.Equal(t, 1, fake.closeCalls)
}

// An old record with no backend pin but a daemon id ran on tymux; a process-wide
// default that is now tmux must not route it to a tmux backend and drop the id.
func TestSeam_UnpinnedRecordWithDaemonIDRestoresOnTymux(t *testing.T) {
	fake := useSeamFake(t)
	data := restoredPausedData(t, "unpinned-with-id")
	data.TymuxSessionID = "d-79"

	inst, err := FromInstanceDataDeferred(data)
	require.NoError(t, err)

	assert.Equal(t, BackendTymux, inst.Backend)
	assert.Equal(t, "d-79", fake.gotStoredID)
	assert.Equal(t, BackendTymux, inst.ToInstanceData().Backend)
}

// A newly built Instance records the backend it actually resolved, as the
// CreateSession path already does, so later default changes cannot move it.
// Restored unpinned records deliberately keep following the live default.
func TestSeam_NewInstancePinsResolvedBackend(t *testing.T) {
	useSeamFake(t)
	created, err := NewInstance(InstanceOptions{Title: "pin-new", Path: t.TempDir(), Program: "claude"})
	require.NoError(t, err)
	want := processManagerBackendKey(created.processManager)

	assert.Equal(t, want, created.Backend)
	assert.Equal(t, want, created.ToInstanceData().Backend)
}

// A daemon id on a record pinned to another backend is ignored, never routed to
// tymux, and not re-persisted.
func TestSeam_StaleDaemonIDOnTmuxBackendIsDropped(t *testing.T) {
	fake := useSeamFake(t)
	data := restoredPausedData(t, "tmux-with-stale-id")
	data.Backend = BackendTmux
	data.TymuxSessionID = "stale"

	inst, err := FromInstanceDataDeferred(data)
	require.NoError(t, err)

	assert.Empty(t, inst.ToInstanceData().TymuxSessionID)
	assert.Empty(t, fake.gotStoredID, "the tymux manager must never be constructed for a tmux record")
}

// A persisted stale id is cleared when a record is saved without one (backend
// migration, or a session that was closed).
func TestSeam_EntRepositoryClearsDaemonIDOnUpdate(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()
	data := createTestSession("clear-id")
	data.TymuxSessionID = "daemon-1"
	require.NoError(t, repo.Create(ctx, data))

	data.TymuxSessionID = ""
	require.NoError(t, repo.Update(ctx, data))

	got, err := repo.Get(ctx, data.Title)
	require.NoError(t, err)
	assert.Empty(t, got.TymuxSessionID)
}

// Copy audit: every Instance-deriving path (fork, import, backlog, MCP) builds a
// fresh Instance from InstanceOptions. If InstanceOptions ever gains a daemon-id
// field, a copy could carry one Instance's session into another.
func TestSeam_InstanceOptionsCannotCarryDaemonIdentity(t *testing.T) {
	typ := reflect.TypeOf(InstanceOptions{})
	for i := 0; i < typ.NumField(); i++ {
		name := strings.ToLower(typ.Field(i).Name)
		assert.False(t, strings.Contains(name, "tymux") || strings.Contains(name, "daemon"),
			"InstanceOptions.%s could copy daemon session identity between Instances", typ.Field(i).Name)
	}
}

func TestSeam_NewInstanceStartsWithoutDaemonIdentity(t *testing.T) {
	useSeamFake(t)
	inst, err := NewInstance(InstanceOptions{Title: "fresh", Path: t.TempDir(), Program: "claude", Backend: BackendTymux})
	require.NoError(t, err)
	assert.Empty(t, inst.ToInstanceData().TymuxSessionID)
}

type seamProbeTymux struct {
	seamFakeTymux
	live bool
}

func (f *seamProbeTymux) StoredSessionLive() bool { return f.live }

// A liveness shadow (FromInstanceDataDeferred) has no pane, but its stored id
// still tells whether the real session runs; otherwise backlog treats a live
// tymux session as dead and spawns a duplicate into its worktree.
func TestSeam_ShadowLivenessUsesStoredDaemonSession(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	probe := &seamProbeTymux{live: true}
	orig := newTymuxSessionFn
	newTymuxSessionFn = func() tymux.TymuxManager { return probe }
	t.Cleanup(func() { newTymuxSessionFn = orig })
	data := restoredPausedData(t, "shadow-live")
	data.Backend = BackendTymux
	data.TymuxSessionID = "d-90"
	shadow, err := FromInstanceDataDeferred(data)
	require.NoError(t, err)

	assert.True(t, shadow.IsLiveOrStoredSessionLive())
	probe.live = false
	assert.False(t, shadow.IsLiveOrStoredSessionLive())
}

// The stored-session probe is for shadow liveness lookups only; the stream
// paths ask IsBackendProcessAlive whether a pane is attached and must get false
// for a not-yet-restored view so they restore it.
func TestSeam_IsBackendProcessAliveIgnoresStoredSessionOfUnrestoredView(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	probe := &seamProbeTymux{live: true}
	orig := newTymuxSessionFn
	newTymuxSessionFn = func() tymux.TymuxManager { return probe }
	t.Cleanup(func() { newTymuxSessionFn = orig })
	data := restoredPausedData(t, "unrestored-stream")
	data.Backend = BackendTymux
	data.TymuxSessionID = "d-91"
	view, err := FromInstanceDataDeferred(data)
	require.NoError(t, err)

	assert.False(t, view.IsBackendProcessAlive())
	assert.True(t, view.IsLiveOrStoredSessionLive())
}

// failingStartTymux fails every Start, as a transient CreateSession error would.
type failingStartTymux struct{ seamFakeTymux }

func (f *failingStartTymux) Start(string) error             { return errors.New("create failed") }
func (f *failingStartTymux) IsAlive() bool                  { return false }
func (f *failingStartTymux) ResetExitOnce()                 {}
func (f *failingStartTymux) SetOnExitCallback(func(string)) {}
func (f *failingStartTymux) AppendEnv(...string)            {}
func (f *failingStartTymux) GetPTY() (*os.File, error)      { return nil, errors.New("no pty") }

// A restart whose Start fails (a transient create error) must not kill the
// running agent its stored id names: the cleanup closes only what the failed
// attempt attached or created.
func TestSeam_FailedStartOfUnrestoredRecordDoesNotKillStoredSession(t *testing.T) {
	fake := &failingStartTymux{}
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	orig := newTymuxSessionFn
	newTymuxSessionFn = func() tymux.TymuxManager { return fake }
	t.Cleanup(func() { newTymuxSessionFn = orig })
	data := restoredPausedData(t, "failed-start-keeps-session")
	data.Backend = BackendTymux
	data.TymuxSessionID = "d-80"
	inst, err := FromInstanceDataDeferred(data)
	require.NoError(t, err)

	require.Error(t, inst.Start(false))

	assert.Zero(t, fake.closeCalls, "a failed Start must not close the stored daemon session")
	assert.Equal(t, "d-80", inst.ToInstanceData().TymuxSessionID, "the record keeps its id")
}

// releasingTymux reports an adopted session and records release/label calls.
type releasingTymux struct {
	seamFakeTymux
	attached bool
	released int
	label    string
}

func (f *releasingTymux) HasSession() bool            { return f.attached }
func (f *releasingTymux) SetSessionLabel(l string)    { f.label = l }
func (f *releasingTymux) ReleaseAdoptedSession() bool { f.released++; return true }

func restoredTymuxWith(t *testing.T, fake tymux.TymuxManager, title string) *Instance {
	t.Helper()
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	orig := newTymuxSessionFn
	newTymuxSessionFn = func() tymux.TymuxManager { return fake }
	t.Cleanup(func() { newTymuxSessionFn = orig })
	data := restoredPausedData(t, title)
	data.Backend = BackendTymux
	data.TymuxSessionID = "d-81"
	inst, err := FromInstanceDataDeferred(data)
	require.NoError(t, err)
	return inst
}

// A failed Start that attached a session already running before the attempt releases it instead
// of killing the agent.
func TestSeam_FailedStartReleasesAdoptedSessionInsteadOfKillingIt(t *testing.T) {
	fake := &releasingTymux{attached: true}
	inst := restoredTymuxWith(t, fake, "failed-start-adopted")

	require.NoError(t, inst.cleanupAfterFailedStart(false))

	assert.Equal(t, 1, fake.released)
	assert.Zero(t, fake.closeCalls, "an adopted session must not be killed")
}

// Tymux identifies a record by its label, so a rename must reach the backend.
func TestSeam_RenameAndSetTitleDirectUpdateTheProcessLabel(t *testing.T) {
	fake := &releasingTymux{}
	inst := restoredTymuxWith(t, fake, "label-before")

	require.NoError(t, inst.Rename("label-renamed"))
	assert.Equal(t, "label-renamed", fake.label)

	inst.SetTitleDirect("label-rollback")
	assert.Equal(t, "label-rollback", fake.label)
}

// Rename must not call into the process manager while holding i.mu: pm() takes pmMu, and
// updateProcessManagerForRestart takes pmMu and then i.mu when migrating backends (ABBA).
func TestSeam_RenameDoesNotHoldInstanceLockWhileTakingPMLock(t *testing.T) {
	inst := restoredTymuxWith(t, &releasingTymux{}, "lock-order-before")
	inst.pmMu.Lock() // as the migrating branch holds it before taking i.mu
	released := false
	t.Cleanup(func() {
		if !released {
			inst.pmMu.Unlock() // let a failed run's Rename finish instead of hanging
		}
	})
	done := make(chan error, 1)
	go func() { done <- inst.Rename("lock-order-after") }()

	// Rename publishes the new title before it reaches the process manager, so once it is
	// visible Rename is past the update and about to block on (or already past) pmMu.
	wait.RequireEventually(t, func() bool { return inst.Snapshot().Title == "lock-order-after" },
		5*time.Second, time.Millisecond, "Rename never started")
	wait.RequireEventually(t, func() bool {
		if inst.mu.TryLock() {
			inst.mu.Unlock()
			return true
		}
		return false
	}, 5*time.Second, time.Millisecond, "Rename holds i.mu while blocked on pmMu")

	released = true
	inst.pmMu.Unlock()
	require.NoError(t, <-done)
}
