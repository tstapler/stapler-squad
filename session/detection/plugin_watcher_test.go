package detection

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// eventuallyTimeout/eventuallyPoll bound every hot-reload assertion below to
// well under the 2-second acceptance criterion budget, polling rather than
// sleeping (ADR-003 — docs/adr/003-no-static-sleeps-in-tests.md).
const (
	eventuallyTimeout = 2 * time.Second
	eventuallyPoll    = 20 * time.Millisecond
)

// startWatcherForTest starts a PluginWatcher over dir, cancels its context
// and waits for its goroutine to exit in t.Cleanup, and restores the
// built-ins-only snapshot afterward — every plugin_watcher_test.go test must
// call this so no goroutine or global-state change leaks into another test.
func startWatcherForTest(t *testing.T, dir string) *PluginWatcher {
	t.Helper()
	resetSnapshotAfterTest(t)

	ctx, cancel := context.WithCancel(context.Background())
	w, err := StartPluginWatcher(ctx, dir)
	if err != nil {
		t.Fatalf("StartPluginWatcher() unexpected error = %v", err)
	}
	t.Cleanup(func() {
		cancel()
		select {
		case <-w.Stopped():
		case <-time.After(eventuallyTimeout):
			t.Error("PluginWatcher did not stop within timeout after context cancellation")
		}
	})
	return w
}

func TestPluginWatcher_should_detectNewFileWithoutRestart_When_validTomlIsAdded(t *testing.T) {
	dir := t.TempDir()
	startWatcherForTest(t, dir)

	writePluginFile(t, dir, "my-agent.toml", validPluginTOML("my-agent", []string{"my-agent"}))

	sd := NewStatusDetector()
	require.Eventually(t, func() bool {
		return sd.DetectForProgram([]byte("Thinking..."), "my-agent") == StatusProcessing
	}, eventuallyTimeout, eventuallyPoll, "new plugin file was not hot-reloaded within timeout")
}

func TestPluginWatcher_should_detectEditedFileWithoutRestart_When_regexChanges(t *testing.T) {
	dir := t.TempDir()
	startWatcherForTest(t, dir)

	path := writePluginFile(t, dir, "my-agent.toml", validPluginTOML("my-agent", []string{"my-agent"}))

	sd := NewStatusDetector()
	require.Eventually(t, func() bool {
		return sd.DetectForProgram([]byte("Thinking..."), "my-agent") == StatusProcessing
	}, eventuallyTimeout, eventuallyPoll, "initial plugin file was not loaded within timeout")

	edited := `id = "my-agent"
binary_names = ["my-agent"]

[[patterns]]
name = "confirm"
regex = "Proceed\\?"
status = "needs_approval"
`
	if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
		t.Fatalf("failed to edit plugin file: %v", err)
	}

	require.Eventually(t, func() bool {
		return sd.DetectForProgram([]byte("Proceed?"), "my-agent") == StatusNeedsApproval
	}, eventuallyTimeout, eventuallyPoll, "edited plugin file was not hot-reloaded within timeout")
}

func TestPluginWatcher_should_removeBinaryFromProvenance_When_pluginFileIsDeleted(t *testing.T) {
	dir := t.TempDir()
	startWatcherForTest(t, dir)

	path := writePluginFile(t, dir, "my-agent.toml", validPluginTOML("my-agent", []string{"my-agent"}))

	require.Eventually(t, func() bool {
		_, ok := DetectorProvenance()["my-agent"]
		return ok
	}, eventuallyTimeout, eventuallyPoll, "plugin file was not loaded within timeout")

	if err := os.Remove(path); err != nil {
		t.Fatalf("failed to remove plugin file: %v", err)
	}

	require.Eventually(t, func() bool {
		_, ok := DetectorProvenance()["my-agent"]
		return !ok
	}, eventuallyTimeout, eventuallyPoll, "removed plugin file's binary name was not dropped from provenance within timeout")
}

func TestPluginWatcher_should_restoreBuiltin_When_overridingPluginFileIsDeleted(t *testing.T) {
	dir := t.TempDir()
	startWatcherForTest(t, dir)

	path := writePluginFile(t, dir, "claude-override.toml", validPluginTOML("claude-override", []string{"claude"}))

	require.Eventually(t, func() bool {
		return DetectorProvenance()["claude"] == path
	}, eventuallyTimeout, eventuallyPoll, "override plugin file was not loaded within timeout")

	if err := os.Remove(path); err != nil {
		t.Fatalf("failed to remove override plugin file: %v", err)
	}

	sd := NewStatusDetector()
	require.Eventually(t, func() bool {
		return DetectorProvenance()["claude"] == "" &&
			sd.DetectForProgram([]byte("esc to interrupt"), "claude") == StatusExecuting
	}, eventuallyTimeout, eventuallyPoll, "built-in claude detector was not restored within timeout after override removal")
}

func TestPluginWatcher_should_collapseBurstIntoOneReload_When_sameFileWrittenRepeatedly(t *testing.T) {
	dir := t.TempDir()
	startWatcherForTest(t, dir)

	before := rebuildCount.Load()

	path := filepath.Join(dir, "my-agent.toml")
	for i := 0; i < 10; i++ {
		if err := os.WriteFile(path, []byte(validPluginTOML("my-agent", []string{"my-agent"})), 0o644); err != nil {
			t.Fatalf("iteration %d: failed to write plugin file: %v", i, err)
		}
		time.Sleep(5 * time.Millisecond) // stay well inside pluginReloadDebounce (200ms)
	}

	// Wait for the debounced reload to land, then hold steady for a further
	// window to prove no second reload follows.
	require.Eventually(t, func() bool {
		_, ok := DetectorProvenance()["my-agent"]
		return ok
	}, eventuallyTimeout, eventuallyPoll, "burst-written plugin file was never loaded")

	require.Never(t, func() bool {
		return rebuildCount.Load() > before+1
	}, 500*time.Millisecond, eventuallyPoll, "burst of rapid writes triggered more than one rebuild")

	if got := rebuildCount.Load(); got != before+1 {
		t.Errorf("rebuildCount increased by %d across the burst, want exactly 1", got-before)
	}
}

// TestPluginWatcher_should_notPanicOrBlock_When_fsnotifyWatcherIsNil exercises
// the fsnotify-unavailable fallback code path directly: fsnotify.NewWatcher()
// failing is an OS-level condition this test cannot force, so instead it
// constructs a PluginWatcher with a nil *fsnotify.Watcher the same way
// StartPluginWatcher does on that failure path, and confirms watchLoop
// neither panics reading from the resulting nil Events/Errors channels nor
// fails to exit cleanly on context cancellation.
func TestPluginWatcher_should_notPanicOrBlock_When_fsnotifyWatcherIsNil(t *testing.T) {
	dir := t.TempDir()
	resetSnapshotAfterTest(t)

	w := &PluginWatcher{
		dir:     dir,
		watcher: nil,
		stopped: make(chan struct{}),
	}

	ctx, cancel := context.WithCancel(context.Background())
	go w.watchLoop(ctx)

	cancel()
	select {
	case <-w.Stopped():
	case <-time.After(eventuallyTimeout):
		t.Fatal("watchLoop did not exit after context cancellation in periodic-only (nil watcher) mode")
	}
}
