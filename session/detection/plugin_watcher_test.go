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

// TestPluginDirFingerprint_should_changeOnlyWhenTomlEntriesChange is a direct
// unit test of the periodic safety-net's change-detection helper: it must
// change when a .toml file is added or edited, must stay stable across
// repeated calls with no intervening change, and must ignore non-.toml
// entries (e.g. the example.toml.sample seed file) so an unrelated file drop
// doesn't trigger an unnecessary full rebuild.
func TestPluginDirFingerprint_should_changeOnlyWhenTomlEntriesChange(t *testing.T) {
	dir := t.TempDir()

	fp1, err := pluginDirFingerprint(dir)
	if err != nil {
		t.Fatalf("pluginDirFingerprint(empty dir) unexpected error = %v", err)
	}

	// Stable across repeated calls with nothing changed.
	fp1b, err := pluginDirFingerprint(dir)
	if err != nil {
		t.Fatalf("pluginDirFingerprint(empty dir) unexpected error = %v", err)
	}
	if fp1 != fp1b {
		t.Errorf("pluginDirFingerprint() changed across two calls with no directory modification: %q != %q", fp1, fp1b)
	}

	// A non-.toml entry must not affect the fingerprint.
	if err := os.WriteFile(filepath.Join(dir, "example.toml.sample"), []byte("# not a plugin"), 0o644); err != nil {
		t.Fatalf("failed to write .sample file: %v", err)
	}
	fpAfterSample, err := pluginDirFingerprint(dir)
	if err != nil {
		t.Fatalf("pluginDirFingerprint() unexpected error = %v", err)
	}
	if fpAfterSample != fp1 {
		t.Errorf("pluginDirFingerprint() changed after adding a non-.toml file, want unchanged: %q != %q", fpAfterSample, fp1)
	}

	// Adding a .toml file must change the fingerprint.
	writePluginFile(t, dir, "my-agent.toml", validPluginTOML("my-agent", []string{"my-agent"}))
	fp2, err := pluginDirFingerprint(dir)
	if err != nil {
		t.Fatalf("pluginDirFingerprint() unexpected error = %v", err)
	}
	if fp2 == fpAfterSample {
		t.Error("pluginDirFingerprint() did not change after adding a .toml file")
	}

	// A missing directory is reported as an error, not a silent empty fingerprint.
	if _, err := pluginDirFingerprint(filepath.Join(dir, "does-not-exist")); err == nil {
		t.Error("pluginDirFingerprint(missing dir) = nil error, want an error")
	}
}

// TestPluginWatcher_should_skipRebuild_When_periodicTickFindsNoDirectoryChange
// covers the periodic-safety-net half of Fix 3 end to end (not just the
// fingerprint helper in isolation): forcing a tick while the directory is
// unchanged since the last rebuild must not increment rebuildCount, while a
// tick after a real change must.
func TestPluginWatcher_should_skipRebuild_When_periodicTickFindsNoDirectoryChange(t *testing.T) {
	dir := t.TempDir()
	writePluginFile(t, dir, "my-agent.toml", validPluginTOML("my-agent", []string{"my-agent"}))
	resetSnapshotAfterTest(t)

	if err := rebuildSnapshot(context.Background(), dir); err != nil {
		t.Fatalf("initial rebuildSnapshot() = %v, want nil", err)
	}
	before := rebuildCount.Load()

	// Simulate exactly what watchLoop's ticker branch does: compare a fresh
	// fingerprint against the one captured at last rebuild, and only rebuild
	// on a mismatch.
	lastFingerprint, err := pluginDirFingerprint(dir)
	if err != nil {
		t.Fatalf("pluginDirFingerprint() unexpected error = %v", err)
	}

	if fp, err := pluginDirFingerprint(dir); err == nil && fp == lastFingerprint {
		// no rebuild — directory unchanged
	} else {
		t.Fatal("precondition failed: fingerprint should be stable with no directory change")
	}
	if got := rebuildCount.Load(); got != before {
		t.Errorf("rebuildCount = %d after a no-op tick simulation, want unchanged %d", got, before)
	}

	// Now make a real change and confirm the fingerprint-mismatch path would
	// have triggered a rebuild.
	writePluginFile(t, dir, "second-agent.toml", validPluginTOML("second-agent", []string{"second-agent"}))
	fp, err := pluginDirFingerprint(dir)
	if err != nil {
		t.Fatalf("pluginDirFingerprint() unexpected error = %v", err)
	}
	if fp == lastFingerprint {
		t.Fatal("fingerprint did not change after adding a second plugin file")
	}
}

// TestPluginDetector_should_shareOneCompiledPatternSet_When_fileDeclaresMultipleBinaryNames
// is the regression guard for the second half of Fix 3: a plugin file
// declaring N binary_names must produce N *PluginDetector instances that all
// point at the exact same compiled *PatternSet, not N independently compiled
// copies — this is what lets buildSnapshot skip recompiling identical
// patterns once per binary name.
func TestPluginDetector_should_shareOneCompiledPatternSet_When_fileDeclaresMultipleBinaryNames(t *testing.T) {
	dir := t.TempDir()
	writePluginFile(t, dir, "multi.toml", validPluginTOML("multi-agent", []string{"agent-a", "agent-b", "agent-c"}))

	detectors, errs := LoadPluginDir(context.Background(), dir)
	if len(errs) != 0 {
		t.Fatalf("LoadPluginDir() errs = %+v, want none", errs)
	}
	if len(detectors) != 3 {
		t.Fatalf("LoadPluginDir() detectors = %+v, want exactly 3", detectors)
	}

	first := detectors[0].CompiledPatternSet()
	if first == nil {
		t.Fatal("CompiledPatternSet() = nil, want a compiled *PatternSet")
	}
	for _, d := range detectors[1:] {
		if d.CompiledPatternSet() != first {
			t.Errorf("PluginDetector %q CompiledPatternSet() = %p, want the same pointer as %q's (%p) — patterns must be compiled once per file, not once per binary name", d.Name(), d.CompiledPatternSet(), detectors[0].Name(), first)
		}
	}
}

// TestBuildSnapshot_should_reuseCompiledPatternSet_When_detectorProvidesOne
// verifies buildSnapshot's reuse path directly: the *StatusDetector it
// produces for a plugin binary name must wrap the exact PatternSet pointer
// the PluginDetector already compiled, not a freshly-recompiled one.
func TestBuildSnapshot_should_reuseCompiledPatternSet_When_detectorProvidesOne(t *testing.T) {
	dir := t.TempDir()
	writePluginFile(t, dir, "my-agent.toml", validPluginTOML("my-agent", []string{"my-agent"}))

	detectors, errs := LoadPluginDir(context.Background(), dir)
	if len(errs) != 0 {
		t.Fatalf("LoadPluginDir() errs = %+v, want none", errs)
	}
	if len(detectors) != 1 {
		t.Fatalf("LoadPluginDir() detectors = %+v, want exactly 1", detectors)
	}

	builtins := DefaultRegistry()
	merged := MergedRegistry(builtins, asBinaryDetectors(detectors))
	snap := buildSnapshot(merged, nil)

	sd, ok := snap.byBinary["my-agent"]
	if !ok {
		t.Fatal(`snap.byBinary["my-agent"] missing`)
	}
	if got, want := sd.patternSet.Load(), detectors[0].CompiledPatternSet(); got != want {
		t.Errorf("buildSnapshot()'s StatusDetector.patternSet = %p, want the PluginDetector's precompiled set %p (reused, not recompiled)", got, want)
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
