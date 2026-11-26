package search

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/claudehistory"
)

// cancelAfterCtx reports context.Canceled once Err has been called n times,
// which cancels a sync deterministically partway through its session loop.
type cancelAfterCtx struct {
	context.Context
	left atomic.Int64
}

func newCancelAfterCtx(n int64) *cancelAfterCtx {
	c := &cancelAfterCtx{Context: context.Background()}
	c.left.Store(n)
	return c
}

func (c *cancelAfterCtx) Err() error {
	if c.left.Add(-1) < 0 {
		return context.Canceled
	}
	return nil
}

func writeHistory(t *testing.T, sessions int) *claudehistory.ClaudeSessionHistory {
	t.Helper()
	path := filepath.Join(t.TempDir(), "history.jsonl")
	var data []byte
	for i := 0; i < sessions; i++ {
		data = append(data, fmt.Appendf(nil, `{"display":"hello %d","timestamp":%d,"project":"/repo","sessionId":"sess-%d"}`+"\n", i, 1700000000000+int64(i), i)...)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	hist, err := claudehistory.NewClaudeSessionHistory(path)
	if err != nil {
		t.Fatal(err)
	}
	return hist
}

// Not parallel: t.Setenv keeps conversation-file lookup inside an empty HOME.
func TestIncrementalSyncContext_should_ReturnPromptlyAndRebuildLater_When_CancelledMidFullBuild(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const sessions = 50
	hist := writeHistory(t, sessions)
	engine := NewSearchEngine()

	_, err := engine.IncrementalSyncContext(newCancelAfterCtx(10), hist)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if engine.GetSyncMetadata() != nil {
		t.Error("sync metadata must be cleared after a cancelled full build so the next sync rebuilds")
	}

	res, err := engine.IncrementalSync(hist)
	if err != nil {
		t.Fatalf("follow-up sync: %v", err)
	}
	if !res.WasFullRebuild {
		t.Error("follow-up sync after a cancelled full build must be a full rebuild")
	}
}

func TestIncrementalSyncContext_should_ReturnContextError_When_AlreadyCancelled(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	hist := writeHistory(t, 5)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := NewSearchEngine().IncrementalSyncContext(ctx, hist); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

// Not parallel: t.Setenv keeps conversation-file lookup inside an empty HOME.
// With an up-to-date index there is no per-session loop to notice the cancel,
// so only the up-front check can return the error.
func TestIncrementalSyncContext_should_ReturnContextError_When_AlreadyCancelledAndNothingChanged(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	hist := writeHistory(t, 3)
	engine := NewSearchEngine()
	if _, err := engine.IncrementalSync(hist); err != nil {
		t.Fatal(err)
	}
	// An empty HOME indexes nothing, so mark every session as up to date by hand.
	md := engine.GetSyncMetadata()
	for _, entry := range hist.GetAll() {
		md.Sessions[entry.ID] = &SessionIndexMetadata{SessionID: entry.ID, UpdatedAt: entry.UpdatedAt, MessageCount: entry.MessageCount}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := engine.IncrementalSyncContext(ctx, hist); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

// Not parallel: t.Setenv keeps conversation-file lookup inside an empty HOME.
func TestIncrementalSyncContext_should_RefreshTotalsAndStayIncremental_When_CancelledMidIncrementalPass(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	hist := writeHistory(t, 5)
	engine := NewSearchEngine()
	if _, err := engine.IncrementalSync(hist); err != nil {
		t.Fatal(err)
	}
	// Three indexed sessions that have since vanished from history: the pass
	// removes them, then is cancelled in the additions loop.
	md := engine.GetSyncMetadata()
	for _, id := range []string{"gone-1", "gone-2", "gone-3"} {
		md.Sessions[id] = &SessionIndexMetadata{SessionID: id, DocCount: 2}
	}
	md.TotalSessions, md.TotalDocuments = 3, 6
	md.LastIncrementalSync = time.Time{}

	_, err := engine.IncrementalSyncContext(newCancelAfterCtx(1), hist)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	got := engine.GetSyncMetadata()
	if got == nil {
		t.Fatal("a cancelled incremental pass must keep the sync metadata")
	}
	if got.TotalSessions != len(got.Sessions) || got.TotalDocuments != 0 {
		t.Errorf("TotalSessions=%d TotalDocuments=%d with %d tracked sessions: totals must match the removals already applied",
			got.TotalSessions, got.TotalDocuments, len(got.Sessions))
	}
	if got.LastIncrementalSync.IsZero() {
		t.Error("a cancelled incremental pass that applied removals must stamp LastIncrementalSync")
	}
	res, err := engine.IncrementalSync(hist)
	if err != nil {
		t.Fatalf("follow-up sync: %v", err)
	}
	if res.WasFullRebuild {
		t.Error("follow-up sync after a cancelled incremental pass must not be a full rebuild")
	}
}

// Not parallel: t.Setenv keeps conversation-file lookup inside an empty HOME.
// The ctx budget of 1 is spent by the up-front check, so the cancel lands in
// the updates loop (no session is added).
func TestIncrementalSyncContext_should_StampLastIncrementalSync_When_CancelledInUpdatesLoop(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	hist := writeHistory(t, 3)
	engine := NewSearchEngine()
	if _, err := engine.IncrementalSync(hist); err != nil {
		t.Fatal(err)
	}
	md := engine.GetSyncMetadata()
	for _, entry := range hist.GetAll() {
		md.Sessions[entry.ID] = &SessionIndexMetadata{SessionID: entry.ID, UpdatedAt: entry.UpdatedAt, MessageCount: entry.MessageCount + 1}
	}
	md.LastIncrementalSync = time.Time{}

	_, err := engine.IncrementalSyncContext(newCancelAfterCtx(1), hist)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if engine.GetSyncMetadata().LastIncrementalSync.IsZero() {
		t.Error("a pass cancelled in the updates loop must stamp LastIncrementalSync")
	}
}

// Not parallel: t.Setenv keeps conversation-file lookup inside an empty HOME.
// A pre-cancelled rebuild must still clear the metadata, so the up-front ctx
// check may only guard the incremental path.
func TestIncrementalSyncContext_should_ClearMetadata_When_RebuildStartsWithCancelledCtx(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	hist := writeHistory(t, 3)
	engine := NewSearchEngine()
	if _, err := engine.IncrementalSync(hist); err != nil {
		t.Fatal(err)
	}
	engine.GetSyncMetadata().Version = -1 // next sync takes the full-rebuild path
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := engine.IncrementalSyncContext(ctx, hist); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if engine.GetSyncMetadata() != nil {
		t.Error("a cancelled rebuild must clear the sync metadata so the next sync rebuilds")
	}
}
