package session

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newOwnerAndUnlinked(t *testing.T, ownerStatus Status, archived bool, calls *atomic.Int64) (*HistoryLinker, *Instance) {
	t.Helper()
	linker := NewHistoryLinker(NewHistoryFileDetectorWithHomeDir(&mockProcessInspector{}, t.TempDir()), nil)
	owner := &Instance{Title: "owner", Path: t.TempDir(), Status: ownerStatus}
	if archived {
		at := time.Now()
		owner.ArchivedAt = &at
	}
	owner.SetHistoryInfo("conv-u", "/projects/p/conv-u.jsonl")
	waiting := &Instance{Title: "waiting", Path: t.TempDir(), Status: Running}
	waiting.processManager = panePIDCountingPM{calls: calls}
	linker.SetInstances([]*Instance{owner, waiting})
	return linker, waiting
}

// The owner of the file is not running, so a write to it may be the live
// session that resumed the conversation: an unlinked session must still get its
// rescan (the pane-PID fast path is the only thing that can link it).
func TestHistoryLinker_FileEvent_should_Rescan_When_ClaimingOwnerIsNotLiveAndASessionIsUnlinked(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		status   Status
		archived bool
	}{
		{"paused", Paused, false},
		{"stopped", Stopped, false},
		{"hibernated", Hibernated, false},
		{"archived", Running, true},
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
	linker, waiting := newOwnerAndUnlinked(t, Paused, false, &calls)
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
	linker, waiting := newOwnerAndUnlinked(t, Running, false, &calls)
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
	linker, waiting := newOwnerAndUnlinked(t, Running, false, &calls)
	waiting.Status = Paused

	linker.handleHistoryFileEvent("/projects/p/conv-u.jsonl")

	assert.Zero(t, calls.Load())
}

// A non-live owner proves nothing even when no session is unlinked: a linked
// live session could have resumed into the owner's conversation.
func TestHistoryLinker_FileEvent_should_Rescan_When_OwnerNotLiveAndNoSessionIsUnlinked(t *testing.T) {
	t.Parallel()
	var calls atomic.Int64
	linker := NewHistoryLinker(NewHistoryFileDetectorWithHomeDir(&mockProcessInspector{}, t.TempDir()), nil)
	owner := &Instance{Title: "owner", Path: t.TempDir(), Status: Stopped}
	owner.processManager = panePIDCountingPM{calls: &calls}
	owner.SetHistoryInfo("conv-u", "/projects/p/conv-u.jsonl")
	linker.SetInstances([]*Instance{owner})

	linker.handleHistoryFileEvent("/projects/p/conv-u.jsonl")

	assert.Positive(t, calls.Load())
}

// End to end through a real fsnotify watcher: an append to a live owner's file
// skips the scan, a brand-new conversation file triggers it.
func TestHistoryLinker_Watcher_should_SkipForLiveOwnerAndScanForNewFile(t *testing.T) {
	t.Parallel()
	var calls atomic.Int64
	dir := t.TempDir()
	linker := NewHistoryLinker(NewHistoryFileDetectorWithHomeDir(&mockProcessInspector{}, t.TempDir()), nil)
	owner := &Instance{Title: "owner", Path: t.TempDir(), Status: Running}
	owner.processManager = panePIDCountingPM{calls: &calls}
	owner.SetHistoryInfo("conv-u", filepath.Join(dir, "conv-u.jsonl"))
	linker.SetInstances([]*Instance{owner})

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
