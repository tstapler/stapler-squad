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

	start := time.Now()
	_, err := engine.IncrementalSyncContext(newCancelAfterCtx(10), hist)
	elapsed := time.Since(start)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if elapsed > time.Second {
		t.Errorf("cancelled sync took %v, want well under the shutdown deadline", elapsed)
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
