package session

import (
	"os"
	"sync"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/launchcmd"
	"github.com/tstapler/stapler-squad/session/queue"
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

// NewPausedTestInstance returns a minimal Paused, started Instance ready for Storage.AddInstance, for tests of
// packages that sit above session (and cannot set the unexported started flag themselves).
func NewPausedTestInstance(title string) *Instance {
	inst := &Instance{
		Title:     title,
		Path:      "/tmp/test",
		Status:    domain.Paused,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	inst.started.Store(true)
	return inst
}

// FinishInstanceForTest publishes the initial snapshot of an Instance built from a struct literal,
// so tests of packages above session (e.g. session/prpoller) can call Snapshot()/AttachPR on it.
func FinishInstanceForTest(i *Instance) { FinishInstanceConstruction(i) }

// SetProcessManagerForTest swaps the instance's pane backend so tests in packages that import session can
// stand in a fake without reaching the unexported field.
func (i *Instance) SetProcessManagerForTest(pm ProcessManager) { i.processManager = pm }

// WithProcessManagerForTest swaps the pane backend and returns i, so a struct literal can seed it inline.
func (i *Instance) WithProcessManagerForTest(pm ProcessManager) *Instance {
	i.processManager = pm
	return i
}

// ProcessManagerForTest returns the raw pane backend, unlike pm(), which takes pmMu.
func (i *Instance) ProcessManagerForTest() ProcessManager { return i.processManager }

// SetStartedForTest sets the started flag; MarkStarted covers the true case.
func (i *Instance) SetStartedForTest(v bool) { i.started.Store(v) }

// WithReviewQueueForTest sets the review queue and returns i, for seeding a struct literal inline.
func (i *Instance) WithReviewQueueForTest(q *queue.ReviewQueue) *Instance {
	i.reviewQueue = q
	return i
}

// WithWorktreeForTest sets the git worktree (nil for a directory session) and returns i, for seeding a struct literal inline.
func (i *Instance) WithWorktreeForTest(w *git.GitWorktree) *Instance {
	i.gitManager.Worktree = w
	return i
}

// WithClaudeSessionForTest sets the Claude session data and returns i, for seeding a struct literal inline.
func (i *Instance) WithClaudeSessionForTest(s *domain.ClaudeSessionData) *Instance {
	i.SetClaudeSession(s)
	return i
}

// WithHistoryDetectorForTest sets the conversation-history detector and returns i, for seeding a struct literal inline.
func (i *Instance) WithHistoryDetectorForTest(d *HistoryFileDetector) *Instance {
	i.historyDetector = d
	return i
}

// WithConversationClearedAtForTest records when the conversation was last cleared and returns i.
func (i *Instance) WithConversationClearedAtForTest(at time.Time) *Instance {
	i.claudeSessionMu.Lock()
	defer i.claudeSessionMu.Unlock()
	i.conversationClearedAt = at
	return i
}

// UpdateProcessManagerForRestartForTest runs the backend swap Restart performs.
func (i *Instance) UpdateProcessManagerForRestartForTest(migrating bool, backendOverride domain.ProcessManagerBackend, command launchcmd.LaunchCommand) error {
	return i.updateProcessManagerForRestart(migrating, backendOverride, command)
}

// MutateForTest runs fn under i.mu and republishes the snapshot, so a test can edit exported fields
// the way an actor setter would without reaching the unexported lock and snapshot.
func (i *Instance) MutateForTest(fn func(*Instance)) {
	i.mu.Lock()
	defer i.mu.Unlock()
	fn(i)
	i.snapshot.Store(buildSnapshot(i))
}

// RecoverFromStaleResumeForTest runs the stale-resume recovery a failed resume exit triggers.
func (i *Instance) RecoverFromStaleResumeForTest() { i.recoverFromStaleResume() }

// CleanupAfterFailedStartForTest runs the failed-start cleanup path.
func (i *Instance) CleanupAfterFailedStartForTest(firstTimeSetup bool) error {
	return i.cleanupAfterFailedStart(firstTimeSetup)
}

// WireTmuxSessionForTest installs the launch command on the current backend, as Start does.
func (i *Instance) WireTmuxSessionForTest(command launchcmd.LaunchCommand) {
	i.wireTmuxSession(command)
}

// InitTmuxSessionForTest rebuilds the pane backend session, as Start does.
func (i *Instance) InitTmuxSessionForTest() { i.initTmuxSession() }

// InitTagManagerForTest attaches the tag manager NewInstance would have installed.
func (i *Instance) InitTagManagerForTest() { i.tagManager = NewTagManager(&i.Tags) }

// AddStoppedShellForTest registers a stopped companion shell, creating the registry first.
func (i *Instance) AddStoppedShellForTest(sh *Shell) {
	i.initShellRegistry()
	i.shells.AddStopped(sh)
}

// SetPRPersistPendingForTest marks a PR set change as awaiting the next full save.
func (i *Instance) SetPRPersistPendingForTest(v bool) { i.prPersistPending.Store(v) }

// SetDestroyedForTest sets the destroyed flag without running Destroy.
func (i *Instance) SetDestroyedForTest(v bool) { i.destroyed.Store(v) }

// HibernateReasonForTest returns the recorded hibernation reason.
func (i *Instance) HibernateReasonForTest() string { return i.hibernateReason }

// HoldPMLockForTest takes pmMu, as a backend migration does before taking i.mu, and returns its release.
func (i *Instance) HoldPMLockForTest() (release func()) {
	i.pmMu.Lock()
	return i.pmMu.Unlock
}

// InstanceLockFreeForTest reports whether i.mu can be taken right now.
func (i *Instance) InstanceLockFreeForTest() bool {
	if i.mu.TryLock() {
		i.mu.Unlock()
		return true
	}
	return false
}

// AcquiredAtForTest returns when the lease was granted.
func (l *HeldLease) AcquiredAtForTest() time.Time { return l.acquiredAt }

// ContextErrForTest returns the actor context's error, non-nil once the actor was stopped.
func (l *LiveInstance) ContextErrForTest() error { return l.ctx.Err() }

// PublishSnapshotForTest republishes the snapshot without taking i.mu, for a test that edits fields
// before the instance is shared.
func (i *Instance) PublishSnapshotForTest() { i.snapshot.Store(buildSnapshot(i)) }

// PMForTest calls the lock-taking pm() accessor, so a test can observe pmMu contention.
func (i *Instance) PMForTest() ProcessManager { return i.pm() }

// SetTerminalViewersForTest sets the attached-viewer count.
func (i *Instance) SetTerminalViewersForTest(n int64) { i.terminalViewers.Store(n) }

// TerminalViewersForTest returns the attached-viewer count.
func (i *Instance) TerminalViewersForTest() int64 { return i.terminalViewers.Load() }

// RetryInFlightForTest reports whether a restart claimed the retry slot.
func (i *Instance) RetryInFlightForTest() bool { return i.retryInFlight.Load() }

// StartHotAttachForTest runs the attach-only Start that HotRestoreIfLive performs.
func (i *Instance) StartHotAttachForTest() error { return i.startHotAttach() }

// HotAttachingForTest reports whether a hot attach is in progress.
func (i *Instance) HotAttachingForTest() bool { return i.hotAttaching.Load() }

// ResolvePRForTest resolves ref (nil for the active PR) the way the PR actions do.
func (i *Instance) ResolvePRForTest(ref *domain.PRRef) (domain.PRRef, error) { return i.resolvePR(ref) }

// CancelForTest cancels the actor context without stopping the actor, to model a dead actor.
func (l *LiveInstance) CancelForTest() { l.cancel() }

// ProbeForTest runs safeProbe, the panic-recovering probe wrapper.
func (e *WorktreeEnricher) ProbeForTest(path string) (*WorktreeInfo, error) { return e.safeProbe(path) }

// QueueProcessedForTest returns how many probe tasks the worker pool finished.
func (e *WorktreeEnricher) QueueProcessedForTest() int64 { return int64(e.queue.Stats().Processed) }

// QueuePendingForTest returns how many probe tasks are waiting.
func (e *WorktreeEnricher) QueuePendingForTest() int { return e.queue.Pending() }

// ResultCountForTest returns the number of cached results.
func (e *WorktreeEnricher) ResultCountForTest() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.results)
}

// DriverGoroutinesForTest returns the live driver goroutine count.
func (i *Instance) DriverGoroutinesForTest() int {
	i.driverMu.Lock()
	defer i.driverMu.Unlock()
	return i.driverGoroutines
}

// SetStatusForTest sets the inferred status, so a test can drive the status poller without a pi subprocess.
func (s *PiStatusSource) SetStatusForTest(status detection.DetectedStatus) {
	s.status.Store(int32(status))
}
