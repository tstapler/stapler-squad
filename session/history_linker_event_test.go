package session

import (
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// panePIDCountingPM counts GetPanePID calls; the embedded nil ProcessManager
// is never touched because correlateSession only reaches GetPanePID here.
type panePIDCountingPM struct {
	ProcessManager
	calls *atomic.Int64
}

func (p panePIDCountingPM) GetPanePID() (int32, error) {
	p.calls.Add(1)
	return 0, assert.AnError // no live pane: Detect is skipped, path fallback finds nothing
}

func newLinkedInstances(t *testing.T, n int, calls *atomic.Int64) []*Instance {
	t.Helper()
	insts := make([]*Instance, 0, n)
	for i := 0; i < n; i++ {
		title := "linked-" + string(rune('a'+i%26)) + string(rune('a'+i/26))
		inst := &Instance{Title: title, Path: t.TempDir(), Status: Running}
		inst.processManager = panePIDCountingPM{calls: calls}
		inst.SetHistoryInfo("uuid-"+title, "/projects/p/uuid-"+title+".jsonl")
		insts = append(insts, inst)
	}
	return insts
}

// A write to an already-linked conversation's own JSONL (the common event: every
// appended message) cannot introduce a new conversation, so it must not trigger
// the per-session pane-PID lookups of a force rescan.
func TestHistoryLinker_FileEvent_should_SkipRescan_When_FileBelongsToLinkedSession(t *testing.T) {
	t.Parallel()
	const n = 50
	var calls atomic.Int64
	linker := NewHistoryLinker(NewHistoryFileDetectorWithHomeDir(&mockProcessInspector{}, t.TempDir()), nil)
	insts := newLinkedInstances(t, n, &calls)
	linker.SetInstances(insts)

	linker.handleHistoryFileEvent(filepath.Join("/projects/p", "uuid-"+insts[7].Title+".jsonl"))

	assert.Zero(t, calls.Load(), "known conversation file must not trigger GetPanePID")
}

// An unknown file (new conversation, e.g. after /clear) still force-rescans, so
// UUID changes are detected as before.
func TestHistoryLinker_FileEvent_should_Rescan_When_FileIsUnclaimed(t *testing.T) {
	t.Parallel()
	const n = 5
	var calls atomic.Int64
	linker := NewHistoryLinker(NewHistoryFileDetectorWithHomeDir(&mockProcessInspector{}, t.TempDir()), nil)
	linker.SetInstances(newLinkedInstances(t, n, &calls))

	linker.handleHistoryFileEvent("/projects/p/brand-new-conversation.jsonl")

	require.Equal(t, int64(n), calls.Load(), "unclaimed file must force-rescan every session")
}

func TestHistoryLinker_FileEvent_should_RunExtraCallbacks_When_RescanSkipped(t *testing.T) {
	t.Parallel()
	var calls atomic.Int64
	linker := NewHistoryLinker(NewHistoryFileDetectorWithHomeDir(&mockProcessInspector{}, t.TempDir()), nil)
	insts := newLinkedInstances(t, 1, &calls)
	linker.SetInstances(insts)
	var got atomic.Value
	linker.RegisterFileCallback(func(p string) { got.Store(p) })

	path := "/projects/p/uuid-" + insts[0].Title + ".jsonl"
	linker.handleHistoryFileEvent(path)

	assert.Equal(t, path, got.Load())
}
