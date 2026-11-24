package search

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/claudehistory"
)

// cancelAfterCtx cancels itself once Err has been called n times, so a sync is
// interrupted at a fixed point in its session loop with no timing involved.
type cancelAfterCtx struct {
	context.Context
	cancel context.CancelFunc
	left   atomic.Int64
}

func newCancelAfterCtx(n int64) *cancelAfterCtx {
	inner, cancel := context.WithCancel(context.Background())
	c := &cancelAfterCtx{Context: inner, cancel: cancel}
	c.left.Store(n)
	return c
}

func (c *cancelAfterCtx) Err() error {
	if c.left.Add(-1) < 0 {
		c.cancel()
	}
	return c.Context.Err()
}

// fakeClaudeHome points HOME at a temp dir and returns the history file path.
// Not for parallel tests: it uses t.Setenv.
func fakeClaudeHome(t *testing.T) (historyPath string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".claude", "projects", "p"), 0o755); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(home, ".claude", "history.jsonl")
}

// writeSessions makes sessions [0,total) exist: one history line and one
// one-message conversation file each, so each session indexes one document.
func writeSessions(t *testing.T, historyPath string, total int) {
	t.Helper()
	var hist []byte
	for i := 0; i < total; i++ {
		id := fmt.Sprintf("sess-%03d", i)
		ts := time.Date(2026, 1, 1, 0, 0, i, 0, time.UTC)
		line, _ := json.Marshal(map[string]any{"display": "hello world", "timestamp": ts.UnixMilli(), "project": "/repo", "sessionId": id})
		hist = append(hist, append(line, '\n')...)

		conv, _ := json.Marshal(map[string]any{
			"type": "user", "uuid": id + "-u", "sessionId": id, "timestamp": ts.Format(time.RFC3339), "cwd": "/repo",
			"message": map[string]any{"role": "user", "content": "hello world " + id},
		})
		convPath := filepath.Join(filepath.Dir(historyPath), "projects", "p", id+".jsonl")
		if err := os.WriteFile(convPath, append(conv, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(historyPath, hist, 0o600); err != nil {
		t.Fatal(err)
	}
}

func hitCount(t *testing.T, e *SearchEngine) int {
	t.Helper()
	res, err := e.Search("hello", SearchOptions{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	return res.TotalMatches
}

func TestIncrementalSyncContext_should_StopEarlyAndRebuildLater_When_CancelledMidFullBuild(t *testing.T) {
	histPath := fakeClaudeHome(t)
	const sessions = 50
	writeSessions(t, histPath, sessions)
	hist, err := claudehistory.NewClaudeSessionHistory(histPath)
	if err != nil {
		t.Fatal(err)
	}
	engine := NewSearchEngine()

	_, err = engine.IncrementalSyncContext(newCancelAfterCtx(10), hist)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if got := hitCount(t, engine); got == 0 || got >= sessions {
		t.Errorf("hits after cancel = %d, want a partial index (0 < n < %d): the build must have run, then stopped early", got, sessions)
	}
	if engine.GetSyncMetadata() != nil {
		t.Error("sync metadata must be cleared after a cancelled full build so the next sync rebuilds")
	}

	res, err := engine.IncrementalSync(hist)
	if err != nil {
		t.Fatalf("follow-up sync: %v", err)
	}
	if !res.WasFullRebuild || res.SessionsAdded != sessions {
		t.Errorf("follow-up: fullRebuild=%v added=%d, want full rebuild of %d", res.WasFullRebuild, res.SessionsAdded, sessions)
	}
	if got := hitCount(t, engine); got != sessions {
		t.Errorf("hits after follow-up = %d, want %d", got, sessions)
	}
}

func TestIncrementalSyncContext_should_KeepMetadataConsistentAndResume_When_CancelledMidIncrementalPass(t *testing.T) {
	histPath := fakeClaudeHome(t)
	const initial, added = 5, 6
	writeSessions(t, histPath, initial)
	hist, err := claudehistory.NewClaudeSessionHistory(histPath)
	if err != nil {
		t.Fatal(err)
	}
	engine := NewSearchEngine()
	if _, err := engine.IncrementalSync(hist); err != nil {
		t.Fatalf("initial sync: %v", err)
	}
	writeSessions(t, histPath, initial+added)
	if err := hist.Reload(); err != nil {
		t.Fatal(err)
	}

	_, err = engine.IncrementalSyncContext(newCancelAfterCtx(4), hist)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	meta := engine.GetSyncMetadata()
	if meta == nil {
		t.Fatal("metadata must survive a cancelled incremental pass")
	}
	done := len(meta.Sessions) - initial // how many new sessions were indexed before the cancel
	if done <= 0 || done >= added {
		t.Fatalf("sessions indexed before cancel = %d, want 0 < n < %d: the pass must have started and stopped early", done, added)
	}
	if meta.TotalSessions != initial+done {
		t.Errorf("TotalSessions = %d, want %d (aggregates refreshed on cancel)", meta.TotalSessions, initial+done)
	}
	if got := hitCount(t, engine); got != initial+done {
		t.Errorf("hits after cancel = %d, want %d (index matches metadata)", got, initial+done)
	}

	res, err := engine.IncrementalSync(hist)
	if err != nil {
		t.Fatalf("resume sync: %v", err)
	}
	if res.WasFullRebuild || res.SessionsAdded != added-done {
		t.Errorf("resume: fullRebuild=%v added=%d, want incremental pass adding %d", res.WasFullRebuild, res.SessionsAdded, added-done)
	}
	if got := hitCount(t, engine); got != initial+added {
		t.Errorf("hits after resume = %d, want %d", got, initial+added)
	}
}

func TestIncrementalSyncContext_should_ReturnContextError_When_AlreadyCancelled(t *testing.T) {
	t.Parallel()
	hist, err := claudehistory.NewClaudeSessionHistory(filepath.Join(t.TempDir(), "history.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := NewSearchEngine().IncrementalSyncContext(ctx, hist); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}
