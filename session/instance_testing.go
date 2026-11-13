package session

import (
	"os"
	"sync"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/tymux"
)

// NewStartedInstanceForTest returns an Instance that reports itself started and
// routes all terminal I/O through pm, so tests outside this package (e.g.
// server/mcp handler tests) can observe SendKeys traffic without a real tmux
// session. The testing.TB parameter keeps it out of production call sites; the
// Instance is otherwise zero-valued, so only terminal I/O methods are safe.
func NewStartedInstanceForTest(tb testing.TB, title string, pm ProcessManager) *Instance {
	tb.Helper()
	inst := &Instance{Title: title}
	inst.processManager = pm
	inst.started.Store(true)
	return inst
}

// NewUnstartedActiveInstanceForTest is NewStartedInstanceForTest for an Active Instance whose
// Start() has not run (a boot-loaded record awaiting its first lazy restore).
func NewUnstartedActiveInstanceForTest(tb testing.TB, title string, pm ProcessManager) *Instance {
	tb.Helper()
	inst := &Instance{Title: title}
	inst.processManager = pm
	SetStatusForTest(tb, inst, domain.Active, nil)
	return inst
}

// HoldHibernationTeardownForTest blocks resume startup until the returned release runs.
func HoldHibernationTeardownForTest(tb testing.TB, inst *Instance) func() {
	tb.Helper()
	inst.hibernateTeardownWG.Add(1)
	var once sync.Once
	release := func() { once.Do(inst.hibernateTeardownWG.Done) }
	tb.Cleanup(release)
	return release
}

// SetTymuxManagerFactoryForTest makes every tymux-backed Instance built until tb ends use
// the manager fn returns, so tests outside this package need no live tymuxd. Not safe
// alongside parallel tests that build tymux Instances.
func SetTymuxManagerFactoryForTest(tb testing.TB, fn func() tymux.TymuxManager) {
	tb.Helper()
	orig := newTymuxSessionFn
	newTymuxSessionFn = fn
	tb.Cleanup(func() { newTymuxSessionFn = orig })
}

// SetStatusForTest changes inst's Status and republishes its snapshot, bypassing the
// state machine, so tests outside this package can model a status change (e.g. a
// retry) on a zero-valued Instance. archivedAt, when non-nil, also marks it archived.
func SetStatusForTest(tb testing.TB, inst *Instance, status domain.Status, archivedAt *time.Time) {
	tb.Helper()
	inst.mu.Lock()
	defer inst.mu.Unlock()
	inst.Status = status
	inst.ArchivedAt = archivedAt
	inst.snapshot.Store(buildSnapshot(inst))
}

// SetHiddenForTest flips inst's Hidden flag and republishes its snapshot.
func SetHiddenForTest(tb testing.TB, inst *Instance, hidden bool) {
	tb.Helper()
	inst.mu.Lock()
	defer inst.mu.Unlock()
	inst.Hidden = hidden
	inst.snapshot.Store(buildSnapshot(inst))
}

// hotRestorablePM is a live backing process whose attach succeeds; the embedded nil
// ProcessManager means only the methods Start(false)'s attach path uses are safe.
type hotRestorablePM struct{ ProcessManager }

func (hotRestorablePM) WireLaunch(LaunchSpec)                       {}
func (hotRestorablePM) AppendLaunchEnv(...string)                   {}
func (hotRestorablePM) ResetExitOnce()                              {}
func (hotRestorablePM) SetOnExitCallback(func(string))              {}
func (hotRestorablePM) IsAlive() bool                               { return true }
func (hotRestorablePM) HasSession() bool                            { return true }
func (hotRestorablePM) HasLiveSessionNoCache() bool                 { return true }
func (hotRestorablePM) RestoreWithWorkDir(string) error             { return nil }
func (hotRestorablePM) GetPTY() (*os.File, error)                   { return nil, nil } //nolint:nilnil
func (hotRestorablePM) GetSessionIdentifier() string                { return "hot-restorable" }
func (hotRestorablePM) GetCurrentWorkingDirectory() (string, error) { return "/tmp", nil }

// NewHotRestorableInstanceForTest returns an Instance in a terminal status whose
// backing process is alive and attaches cleanly, so HotRestoreIfLive can reach
// HotRestoreCleared from outside this package.
func NewHotRestorableInstanceForTest(tb testing.TB, title string, status domain.Status) *Instance {
	tb.Helper()
	inst := &Instance{Title: title, Status: status, Path: tb.TempDir()}
	inst.processManager = hotRestorablePM{}
	return inst
}
