package importer

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session"
)

func newOwnerAndUnlinked(t *testing.T, ownerStatus session.Status, archived bool, calls *atomic.Int64) (*HistoryLinker, *session.Instance) {
	t.Helper()
	linker := NewHistoryLinker(session.NewHistoryFileDetectorWithHomeDir(&mockProcessInspector{}, t.TempDir()), nil)
	owner := &session.Instance{Title: "owner", Path: t.TempDir(), Status: ownerStatus}
	if archived {
		at := time.Now()
		owner.ArchivedAt = &at
	}
	owner.SetHistoryInfo("conv-u", "/projects/p/conv-u.jsonl")
	waiting := &session.Instance{Title: "waiting", Path: t.TempDir(), Status: session.Active, Program: "claude"}
	waiting.SetProcessManagerForTest(panePIDCountingPM{calls: calls})
	linker.SetInstances([]*session.Instance{owner, waiting})
	return linker, waiting
}

// The owner of the file is not running, so a write to it may be the live
// session that resumed the conversation: an unlinked session must still get its
// rescan (the pane-PID fast path is the only thing that can link it).
func TestHistoryLinker_FileEvent_should_Rescan_When_ClaimingOwnerIsNotLiveAndASessionIsUnlinked(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		status   session.Status
		archived bool
	}{
		{"paused", session.Paused, false},
		{"stopped", session.Stopped, false},
		{"hibernated", session.Hibernated, false},
		{"archived", session.Active, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int64
			linker, _ := newOwnerAndUnlinked(t, tc.status, tc.archived, &calls)

			linker.handleHistoryFileEvent("/projects/p/conv-u.jsonl")

			assert.Positive(t, calls.Load(), "unlinked session must be rescanned")
		})
	}
}

func TestHistoryLinker_FileEvent_should_UnparkSession_When_OwnerNotLive(t *testing.T) {
	t.Parallel()
	var calls atomic.Int64
	linker, waiting := newOwnerAndUnlinked(t, session.Paused, false, &calls)
	linker.backoffs[waiting.Title] = &sessionBackoff{parked: true}

	linker.handleHistoryFileEvent("/projects/p/conv-u.jsonl")

	bo := linker.backoffs[waiting.Title]
	assert.True(t, bo == nil || !bo.parked, "ScanAll must unpark the waiting session")
}

// A live owner appending is not enough to skip while a live session still
// awaits a link: that session may be the one that resumed the conversation, and
// ScanAll is what unparks it.
func TestHistoryLinker_FileEvent_should_UnparkWaitingSession_When_OwnerIsLive(t *testing.T) {
	t.Parallel()
	var calls atomic.Int64
	linker, waiting := newOwnerAndUnlinked(t, session.Active, false, &calls)
	linker.backoffs[waiting.Title] = &sessionBackoff{parked: true}

	linker.handleHistoryFileEvent("/projects/p/conv-u.jsonl")

	assert.Positive(t, calls.Load(), "waiting session must be rescanned")
	bo := linker.backoffs[waiting.Title]
	assert.True(t, bo == nil || !bo.parked, "ScanAll must unpark the waiting session")
}

// An unlinked session that is not live cannot be waiting for a link, so it must
// not defeat the skip.
func TestHistoryLinker_FileEvent_should_SkipRescan_When_OnlyUnlinkedSessionIsNotLive(t *testing.T) {
	t.Parallel()
	var calls atomic.Int64
	linker, waiting := newOwnerAndUnlinked(t, session.Active, false, &calls)
	waiting.Status = session.Paused

	linker.handleHistoryFileEvent("/projects/p/conv-u.jsonl")

	assert.Zero(t, calls.Load())
}

// A live unlinked session that cannot link (not a Claude program) must not keep
// every append rescanning forever.
func TestHistoryLinker_FileEvent_should_SkipRescan_When_OnlyUnlinkedLiveSessionIsNotClaude(t *testing.T) {
	t.Parallel()
	var calls atomic.Int64
	linker, waiting := newOwnerAndUnlinked(t, session.Active, false, &calls)
	waiting.Program = "aider --model x"

	linker.handleHistoryFileEvent("/projects/p/conv-u.jsonl")

	assert.Zero(t, calls.Load())
}

// An archived session is not live, so being unlinked does not make it await a
// link.
func TestHistoryLinker_FileEvent_should_SkipRescan_When_OnlyUnlinkedSessionIsArchived(t *testing.T) {
	t.Parallel()
	var calls atomic.Int64
	linker, waiting := newOwnerAndUnlinked(t, session.Active, false, &calls)
	at := time.Now()
	waiting.ArchivedAt = &at

	linker.handleHistoryFileEvent("/projects/p/conv-u.jsonl")

	assert.Zero(t, calls.Load())
}

// A parked Claude session still counts as waiting: the rescan is what unparks it.
func TestHistoryLinker_FileEvent_should_UnparkParkedClaudeSession_When_OwnerIsLive(t *testing.T) {
	t.Parallel()
	var calls atomic.Int64
	linker, waiting := newOwnerAndUnlinked(t, session.Active, false, &calls)
	linker.backoffs[waiting.Title] = &sessionBackoff{parked: true, consecutiveMisses: historyLinkerParkThreshold}

	linker.handleHistoryFileEvent("/projects/p/conv-u.jsonl")

	assert.Positive(t, calls.Load())
	bo := linker.backoffs[waiting.Title]
	assert.True(t, bo == nil || !bo.parked)
}

// An archived owner is not live even though its Status is Active: nothing
// proves the writer is that owner, so the rescan runs with nobody else waiting.
func TestHistoryLinker_FileEvent_should_Rescan_When_ArchivedActiveOwnerAndNoSessionIsUnlinked(t *testing.T) {
	t.Parallel()
	var calls atomic.Int64
	linker := NewHistoryLinker(session.NewHistoryFileDetectorWithHomeDir(&mockProcessInspector{}, t.TempDir()), nil)
	at := time.Now()
	owner := &session.Instance{Title: "owner", Path: t.TempDir(), Status: session.Active, ArchivedAt: &at}
	owner.SetProcessManagerForTest(panePIDCountingPM{calls: &calls})
	owner.SetHistoryInfo("conv-u", "/projects/p/conv-u.jsonl")
	linker.SetInstances([]*session.Instance{owner})

	linker.handleHistoryFileEvent("/projects/p/conv-u.jsonl")

	assert.Positive(t, calls.Load())
}

// Mixed fleet: 49 live linked Claude sessions plus one live aider session that
// can never link. An append to a linked session's file still costs nothing.
func TestHistoryLinker_FileEvent_should_SkipRescan_When_FleetIsLinkedClaudePlusOneAider(t *testing.T) {
	t.Parallel()
	var calls atomic.Int64
	linker := NewHistoryLinker(session.NewHistoryFileDetectorWithHomeDir(&mockProcessInspector{}, t.TempDir()), nil)
	insts := newLinkedInstances(t, 49, &calls)
	for _, inst := range insts {
		inst.Program = "claude"
	}
	aider := &session.Instance{Title: "aider", Path: t.TempDir(), Status: session.Active, Program: "aider"}
	aider.SetProcessManagerForTest(panePIDCountingPM{calls: &calls})
	linker.SetInstances(append(insts, aider))

	linker.handleHistoryFileEvent("/projects/p/uuid-" + insts[3].Title + ".jsonl")

	assert.Zero(t, calls.Load(), "0 GetPanePID calls per append")
}

// A non-live owner proves nothing even when no session is unlinked: a linked
// live session could have resumed into the owner's conversation.
func TestHistoryLinker_FileEvent_should_Rescan_When_OwnerNotLiveAndNoSessionIsUnlinked(t *testing.T) {
	t.Parallel()
	var calls atomic.Int64
	linker := NewHistoryLinker(session.NewHistoryFileDetectorWithHomeDir(&mockProcessInspector{}, t.TempDir()), nil)
	owner := &session.Instance{Title: "owner", Path: t.TempDir(), Status: session.Stopped}
	owner.SetProcessManagerForTest(panePIDCountingPM{calls: &calls})
	owner.SetHistoryInfo("conv-u", "/projects/p/conv-u.jsonl")
	linker.SetInstances([]*session.Instance{owner})

	linker.handleHistoryFileEvent("/projects/p/conv-u.jsonl")

	assert.Positive(t, calls.Load())
}

// End to end through a real fsnotify watcher: an append to a live owner's file
// skips the scan, a brand-new conversation file triggers it.
func TestHistoryLinker_Watcher_should_SkipForLiveOwnerAndScanForNewFile(t *testing.T) {
	t.Parallel()
	var calls atomic.Int64
	dir := t.TempDir()
	linker := NewHistoryLinker(session.NewHistoryFileDetectorWithHomeDir(&mockProcessInspector{}, t.TempDir()), nil)
	owner := &session.Instance{Title: "owner", Path: t.TempDir(), Status: session.Active}
	owner.SetProcessManagerForTest(panePIDCountingPM{calls: &calls})
	owner.SetHistoryInfo("conv-u", filepath.Join(dir, "conv-u.jsonl"))
	linker.SetInstances([]*session.Instance{owner})

	var seen atomic.Value
	linker.RegisterFileCallback(func(p string) { seen.Store(filepath.Base(p)) })
	watcher := NewHistoryFileWatcher(dir, linker.handleHistoryFileEvent)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel(); <-watcher.Stopped() })
	require.NoError(t, watcher.Start(ctx))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "conv-u.jsonl"), []byte("{}\n"), 0o600))
	require.Eventually(t, func() bool { return seen.Load() == "conv-u.jsonl" }, 5*time.Second, 5*time.Millisecond)
	assert.Zero(t, calls.Load(), "append to a live owner's file must not rescan")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "conv-new.jsonl"), []byte("{}\n"), 0o600))
	require.Eventually(t, func() bool { return calls.Load() > 0 }, 5*time.Second, 5*time.Millisecond)
}
