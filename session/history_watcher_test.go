package session

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

func TestHistoryFileWatcher_FiresOnJSONLCreate(t *testing.T) {
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
	nonExistentDir := filepath.Join(t.TempDir(), "does-not-exist")

	watcher := NewHistoryFileWatcher(nonExistentDir, func(filePath string) {})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Must not return an error even if directory doesn't exist
	err := watcher.Start(ctx)
	assert.NoError(t, err)
}

func TestHistoryFileWatcher_ContextCancellationStopsWatcher(t *testing.T) {
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

func TestHistoryFileWatcher_FiltersAgentJSONL(t *testing.T) {
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
