package importer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

func TestHistoryFileWatcher_FiresOnJSONLCreate(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	var mu sync.Mutex
	var received []string

	watcher := NewHistoryFileWatcher(tmpDir, func(filePath string) {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, filePath)
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := watcher.Start(ctx)
	require.NoError(t, err)

	// Create a JSONL file
	jsonlPath := filepath.Join(tmpDir, "550e8400-e29b-41d4-a716-446655440000.jsonl")
	err = os.WriteFile(jsonlPath, []byte(`{"test": true}`), 0600)
	require.NoError(t, err)

	// Wait for the callback to be fired
	assert.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(received) > 0
	}, 2*time.Second, 50*time.Millisecond, "callback should have been called")

	mu.Lock()
	defer mu.Unlock()
	assert.NotEmpty(t, received)
}

func TestHistoryFileWatcher_DoesNotFireOnNonJSONL(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	var mu sync.Mutex
	var received []string

	watcher := NewHistoryFileWatcher(tmpDir, func(filePath string) {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, filePath)
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := watcher.Start(ctx)
	require.NoError(t, err)

	// Create a non-JSONL file
	txtPath := filepath.Join(tmpDir, "somefile.txt")
	err = os.WriteFile(txtPath, []byte("hello"), 0600)
	require.NoError(t, err)

	// Wait briefly to ensure callback is NOT fired
	_ = wait.WaitForCondition(func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(received) > 0
	}, wait.WaitConfig{Timeout: 200 * time.Millisecond, PollInterval: 20 * time.Millisecond, Description: "non-jsonl callback (should not fire)"})

	mu.Lock()
	defer mu.Unlock()
	assert.Empty(t, received, "non-.jsonl file should not trigger callback")
}

func TestHistoryFileWatcher_FiresOnRename(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	var mu sync.Mutex
	var received []string

	watcher := NewHistoryFileWatcher(tmpDir, func(filePath string) {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, filePath)
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := watcher.Start(ctx)
	require.NoError(t, err)

	// Create a temp file then rename it to .jsonl
	tmpFile := filepath.Join(tmpDir, "tempfile.tmp")
	err = os.WriteFile(tmpFile, []byte(`{}`), 0600)
	require.NoError(t, err)

	finalPath := filepath.Join(tmpDir, "a1b2c3d4-e5f6-7890-abcd-ef1234567890.jsonl")
	err = os.Rename(tmpFile, finalPath)
	require.NoError(t, err)

	// Wait for the callback
	assert.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(received) > 0
	}, 2*time.Second, 50*time.Millisecond, "rename to .jsonl should trigger callback")
}

func TestHistoryFileWatcher_DirectoryNotExist_NoError(t *testing.T) {
	t.Parallel()
	nonExistentDir := filepath.Join(t.TempDir(), "does-not-exist")

	watcher := NewHistoryFileWatcher(nonExistentDir, func(filePath string) {})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Must not return an error even if directory doesn't exist
	err := watcher.Start(ctx)
	assert.NoError(t, err)
}

func TestHistoryFileWatcher_ContextCancellationStopsWatcher(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	callbackCount := 0
	var mu sync.Mutex

	watcher := NewHistoryFileWatcher(tmpDir, func(filePath string) {
		mu.Lock()
		defer mu.Unlock()
		callbackCount++
	})

	ctx, cancel := context.WithCancel(context.Background())

	err := watcher.Start(ctx)
	require.NoError(t, err)

	// Cancel the context and wait for the goroutine to fully exit before writing.
	// Without this, the goroutine's select may pick up the fsnotify event before
	// processing ctx.Done(), causing a spurious callback.
	cancel()
	<-watcher.Stopped()

	// Create a file after cancellation — should NOT trigger callback
	jsonlPath := filepath.Join(tmpDir, "550e8400-e29b-41d4-a716-446655440000.jsonl")
	_ = os.WriteFile(jsonlPath, []byte(`{}`), 0600)

	// Wait briefly to ensure callback is NOT fired
	_ = wait.WaitForCondition(func() bool {
		mu.Lock()
		defer mu.Unlock()
		return callbackCount > 0
	}, wait.WaitConfig{Timeout: 200 * time.Millisecond, PollInterval: 20 * time.Millisecond, Description: "post-cancel callback (should not fire)"})

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 0, callbackCount, "no callbacks should fire after context cancellation")
}

func TestHistoryFileWatcher_FiresOnJSONLInSubdirCreatedAfterStart(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	var mu sync.Mutex
	var received []string

	watcher := NewHistoryFileWatcher(tmpDir, func(filePath string) {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, filePath)
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := watcher.Start(ctx)
	require.NoError(t, err)

	// Simulate a new git worktree's own ~/.claude/projects/<encoded-path>/
	// directory appearing after the watcher's initial boot-time walk.
	subDir := filepath.Join(tmpDir, "-Users-tstapler--stapler-squad-worktrees-triage-abc123")
	require.NoError(t, os.Mkdir(subDir, 0755))

	jsonlPath := filepath.Join(subDir, "550e8400-e29b-41d4-a716-446655440000.jsonl")
	require.NoError(t, os.WriteFile(jsonlPath, []byte(`{"test": true}`), 0600))

	assert.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(received) > 0
	}, 2*time.Second, 50*time.Millisecond, "jsonl written into a post-start subdirectory should trigger callback")

	mu.Lock()
	defer mu.Unlock()
	assert.Contains(t, received, jsonlPath)
}

func TestHistoryFileWatcher_FiltersAgentJSONL(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	var mu sync.Mutex
	var received []string

	watcher := NewHistoryFileWatcher(tmpDir, func(filePath string) {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, filePath)
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := watcher.Start(ctx)
	require.NoError(t, err)

	// Create an agent JSONL file — should be filtered out
	agentPath := filepath.Join(tmpDir, "agent-abc123.jsonl")
	err = os.WriteFile(agentPath, []byte(`{}`), 0600)
	require.NoError(t, err)

	// Wait briefly to ensure callback is NOT fired
	_ = wait.WaitForCondition(func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(received) > 0
	}, wait.WaitConfig{Timeout: 200 * time.Millisecond, PollInterval: 20 * time.Millisecond, Description: "agent-jsonl callback (should not fire)"})

	mu.Lock()
	defer mu.Unlock()
	assert.Empty(t, received, "agent-*.jsonl files should not trigger callback")
}

// A callback stuck on one path must not make handleEvent block: events for
// other paths are accepted at once, repeats of the stuck path coalesce, and
// everything is delivered once the callback is released.
func TestHistoryFileWatcher_should_KeepAcceptingEvents_When_CallbackIsSlow(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	entered := make(chan struct{})
	var enterOnce sync.Once
	var mu sync.Mutex
	counts := map[string]int{}

	w := NewHistoryFileWatcher(t.TempDir(), func(path string) {
		enterOnce.Do(func() { close(entered) })
		<-release
		mu.Lock()
		counts[path]++
		mu.Unlock()
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.queue.Start(ctx)

	event := func(name string) fsnotify.Event {
		return fsnotify.Event{Name: filepath.Join("/proj", name+".jsonl"), Op: fsnotify.Write}
	}
	go w.handleEvent(event("stuck")) // off the test goroutine so an inline callback fails by timeout
	select {
	case <-entered: // the single worker is now blocked inside the callback
	case <-time.After(5 * time.Second):
		t.Fatal("callback never started")
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 50; i++ {
			w.handleEvent(event("stuck")) // coalesces into one re-run
		}
		for i := 0; i < 100; i++ {
			w.handleEvent(event(fmt.Sprintf("other-%d", i)))
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("handleEvent blocked behind a slow callback")
	}

	close(release)
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(counts) == 101 && counts[filepath.Join("/proj", "stuck.jsonl")] == 2
	}, 5*time.Second, time.Millisecond)
}

// Stopped (and so HistoryLinker.Wait) must not fire until an in-flight callback
// finishes, and no callback may start after shutdown begins.
func TestHistoryFileWatcher_should_WaitForInFlightCallback_When_Stopping(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	release := make(chan struct{})
	entered := make(chan struct{})
	var enterOnce sync.Once
	var calls atomic.Int32
	w := NewHistoryFileWatcher(dir, func(string) {
		calls.Add(1)
		enterOnce.Do(func() { close(entered) })
		<-release
	})
	require.NoError(t, w.Start(context.Background()))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.jsonl"), []byte("{}"), 0600))
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("callback never started")
	}
	w.Stop()
	w.queue.Enqueue(filepath.Join(dir, "b.jsonl")) // racing the cancel

	select {
	case <-w.Stopped():
		t.Fatal("Stopped fired while a callback was still running")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	select {
	case <-w.Stopped():
	case <-time.After(5 * time.Second):
		t.Fatal("Stopped never fired after the callback finished")
	}
	w.runCallback(filepath.Join(dir, "c.jsonl"))
	assert.EqualValues(t, 1, calls.Load(), "no callback may run after shutdown began")
}

func TestHistoryFileWatcher_should_GiveUpWaiting_When_CallbackExceedsDrainTimeout(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	release := make(chan struct{})
	entered := make(chan struct{})
	var enterOnce sync.Once
	w := NewHistoryFileWatcher(dir, func(string) {
		enterOnce.Do(func() { close(entered) })
		<-release
	})
	w.drainTimeout = 10 * time.Millisecond
	require.NoError(t, w.Start(context.Background()))
	t.Cleanup(func() { close(release) })
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.jsonl"), []byte("{}"), 0600))
	<-entered
	w.Stop()
	select {
	case <-w.Stopped():
	case <-time.After(5 * time.Second):
		t.Fatal("drain timeout did not bound the wait")
	}
}
