package session

import (
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

func TestHistoryLinker_FileEvent_should_LeaveSessionParked_When_OwnerIsLive(t *testing.T) {
	t.Parallel()
	var calls atomic.Int64
	linker, waiting := newOwnerAndUnlinked(t, Running, false, &calls)
	linker.backoffs[waiting.Title] = &sessionBackoff{parked: true}

	linker.handleHistoryFileEvent("/projects/p/conv-u.jsonl")

	assert.Zero(t, calls.Load(), "live owner's own append needs no rescan")
	require.NotNil(t, linker.backoffs[waiting.Title])
	assert.True(t, linker.backoffs[waiting.Title].parked, "intended: parked session stays parked")
}

// Nobody is waiting for a link, so a non-live owner's write needs no rescan.
func TestHistoryLinker_FileEvent_should_SkipRescan_When_OwnerNotLiveButNoSessionIsUnlinked(t *testing.T) {
	t.Parallel()
	var calls atomic.Int64
	linker := NewHistoryLinker(NewHistoryFileDetectorWithHomeDir(&mockProcessInspector{}, t.TempDir()), nil)
	owner := &Instance{Title: "owner", Path: t.TempDir(), Status: Stopped}
	owner.processManager = panePIDCountingPM{calls: &calls}
	owner.SetHistoryInfo("conv-u", "/projects/p/conv-u.jsonl")
	linker.SetInstances([]*Instance{owner})

	linker.handleHistoryFileEvent("/projects/p/conv-u.jsonl")

	assert.Zero(t, calls.Load())
}
