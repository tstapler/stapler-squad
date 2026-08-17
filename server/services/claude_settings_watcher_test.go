package services

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/pkg/classifier"
)

// reloadSpy records every onReload callback invocation for assertions.
type reloadSpy struct {
	mu    sync.Mutex
	calls []reloadCall
}

type reloadCall struct {
	ruleCount int
	origin    string
	notify    bool
}

func (s *reloadSpy) callback() func(rules []classifier.Rule, origin string, notify bool) {
	return func(rules []classifier.Rule, origin string, notify bool) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.calls = append(s.calls, reloadCall{ruleCount: len(rules), origin: origin, notify: notify})
	}
}

func (s *reloadSpy) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

// last returns the most recent call. Callers must check count() > 0 first.
func (s *reloadSpy) last() reloadCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls[len(s.calls)-1]
}

func TestClaudeSettingsWatcher_Reload_ValidPath_UpdatesLastGoodAndReturnsRuleCount(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeSettingsFile(t, filepath.Join(home, ".claude", "settings.json"),
		`{"permissions":{"allow":["Bash(git *)","Read"]}}`)

	spy := &reloadSpy{}
	w := NewClaudeSettingsWatcher("", spy.callback())

	ruleCount, failedPaths := w.Reload(context.Background())

	assert.Equal(t, 2, ruleCount)
	assert.Empty(t, failedPaths)
	assert.Equal(t, 1, spy.count())
	assert.Equal(t, 2, spy.last().ruleCount)

	resolved := resolveSettingsPathOrOriginal(filepath.Join(home, ".claude", "settings.json"))
	assert.Len(t, w.lastGood[resolved], 2)
}

func TestClaudeSettingsWatcher_Reload_MalformedPath_KeepsLastKnownGood(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	writeSettingsFile(t, settingsPath, `{"permissions":{"allow":["Bash(git *)"]}}`)

	spy := &reloadSpy{}
	w := NewClaudeSettingsWatcher("", spy.callback())

	firstCount, firstFailed := w.Reload(context.Background())
	require.Equal(t, 1, firstCount)
	require.Empty(t, firstFailed)

	// Corrupt the file (mimics a mid-autosave truncated write).
	writeSettingsFile(t, settingsPath, `{"permissions": {"allow": [`)

	secondCount, secondFailed := w.Reload(context.Background())

	assert.Equal(t, firstCount, secondCount, "rule count must be unchanged when the only edit is a parse failure")
	assert.NotEmpty(t, secondFailed)
}

// TestClaudeSettingsWatcher_Reload_FileDeleted_RevokesRules is a regression test found in
// review: reloadLocked previously re-merged w.lastGood unconditionally for ANY error,
// including ErrSettingsNotFound — so deleting a settings file never actually revoked the
// auto-approval rules it had contributed; they lived on forever from the last-known-good
// cache, and ReloadClaudeSettingsRules reported success (no failedPaths) even though nothing
// was actually removed.
func TestClaudeSettingsWatcher_Reload_FileDeleted_RevokesRules(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	writeSettingsFile(t, settingsPath, `{"permissions":{"allow":["Bash(git *)"]}}`)

	spy := &reloadSpy{}
	w := NewClaudeSettingsWatcher("", spy.callback())

	firstCount, firstFailed := w.Reload(context.Background())
	require.Equal(t, 1, firstCount)
	require.Empty(t, firstFailed)

	require.NoError(t, os.Remove(settingsPath))

	secondCount, secondFailed := w.Reload(context.Background())

	assert.Equal(t, 0, secondCount, "deleting the settings file must actually revoke its rules, not resurrect them from last-known-good")
	assert.Empty(t, secondFailed, "a legitimately deleted file is not a parse failure")
	resolved := resolveSettingsPathOrOriginal(settingsPath)
	assert.Empty(t, w.lastGood[resolved], "lastGood must be cleared for a deleted path")
}

func TestClaudeSettingsWatcher_Reload_OnReloadCallbackInvokedWithRuleCountAndOrigin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeSettingsFile(t, filepath.Join(home, ".claude", "settings.json"),
		`{"permissions":{"allow":["Bash(git *)"]}}`)

	spy := &reloadSpy{}
	w := NewClaudeSettingsWatcher("", spy.callback())

	w.Reload(context.Background())

	require.Equal(t, 1, spy.count())
	assert.Equal(t, 1, spy.last().ruleCount)
	assert.Equal(t, "global", spy.last().origin)
}

func TestClaudeSettingsWatcher_Reload_CwdEqualsHome_OriginIsGlobalNotMixed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeSettingsFile(t, filepath.Join(home, ".claude", "settings.json"),
		`{"permissions":{"allow":["Bash(git *)"]}}`)

	spy := &reloadSpy{}
	w := NewClaudeSettingsWatcher(home, spy.callback()) // projectDir == home, mirrors the live deployed config

	w.Reload(context.Background())

	require.Equal(t, 1, spy.count())
	assert.Equal(t, "global", spy.last().origin, "a projectDir==home collision must never surface as origin=mixed")
}

func TestClaudeSettingsWatcher_Reload_TagsOriginByChangedPathLabel(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	projectDir := t.TempDir()
	globalPath := filepath.Join(home, ".claude", "settings.json")
	projectPath := filepath.Join(projectDir, ".claude", "settings.json")
	writeSettingsFile(t, globalPath, `{"permissions":{"allow":["Bash(git *)"]}}`)
	writeSettingsFile(t, projectPath, `{"permissions":{"allow":["Read"]}}`)

	spy := &reloadSpy{}
	w := NewClaudeSettingsWatcher(projectDir, spy.callback())

	// First reload: both paths are new (changed from nothing), so global+project both count
	// as changed — establish a baseline, then edit only one path at a time.
	w.Reload(context.Background())

	writeSettingsFile(t, globalPath, `{"permissions":{"allow":["Bash(git *)","Write"]}}`)
	w.Reload(context.Background())
	assert.Equal(t, "global", spy.last().origin)

	writeSettingsFile(t, projectPath, `{"permissions":{"allow":["Read","Edit"]}}`)
	w.Reload(context.Background())
	assert.Equal(t, "project", spy.last().origin)
}

// TestClaudeSettingsWatcher_Reload_BothPathsChanged_OriginIsMixed covers computeReloadOrigin's
// "mixed" branch, which had zero test coverage (found in review).
func TestClaudeSettingsWatcher_Reload_BothPathsChanged_OriginIsMixed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	projectDir := t.TempDir()
	globalPath := filepath.Join(home, ".claude", "settings.json")
	projectPath := filepath.Join(projectDir, ".claude", "settings.json")
	writeSettingsFile(t, globalPath, `{"permissions":{"allow":["Bash(git *)"]}}`)
	writeSettingsFile(t, projectPath, `{"permissions":{"allow":["Read"]}}`)

	spy := &reloadSpy{}
	w := NewClaudeSettingsWatcher(projectDir, spy.callback())
	w.Reload(context.Background()) // baseline

	writeSettingsFile(t, globalPath, `{"permissions":{"allow":["Bash(git *)","Write"]}}`)
	writeSettingsFile(t, projectPath, `{"permissions":{"allow":["Read","Edit"]}}`)
	w.Reload(context.Background())

	assert.Equal(t, "mixed", spy.last().origin)
}

// TestClaudeSettingsWatcher_Reload_SameLengthPatternEdit_DetectedAsChanged is a security
// regression test found in review: rulesEqual previously compared Rule.ID (positional —
// "claude-settings-<label>-<index>") and Decision (always AutoAllow for every claude-settings
// rule), which can never detect an in-place pattern edit at the same array index with the same
// slice length. This silently broke computeReloadOrigin's security-visibility tagging for
// exactly the case ADR-003 names as the threat model: a project-level settings.json edit (e.g.
// from checking out an unreviewed PR branch) that changes what gets auto-allowed without
// changing the array's length would be reported as no change at all, defaulting origin to
// "global" — the opposite of reality.
func TestClaudeSettingsWatcher_Reload_SameLengthPatternEdit_DetectedAsChanged(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	projectDir := t.TempDir()
	projectPath := filepath.Join(projectDir, ".claude", "settings.json")
	writeSettingsFile(t, projectPath, `{"permissions":{"allow":["Read"]}}`)

	spy := &reloadSpy{}
	w := NewClaudeSettingsWatcher(projectDir, spy.callback())
	w.Reload(context.Background()) // baseline: 1 rule at index 0

	// Same array length (1 entry), same index (0), but the pattern itself is now dangerous.
	writeSettingsFile(t, projectPath, `{"permissions":{"allow":["Bash(*)"]}}`)
	count, _ := w.Reload(context.Background())

	require.Equal(t, 1, count)
	assert.Equal(t, "project", spy.last().origin, "an in-place pattern edit must be detected as a real change, not silently defaulted to origin=global")
}

// TestClaudeSettingsWatcher_ConcurrentReloadCalls_NoRace regression-guards
// adversarial-review Blocker 1 / pre-mortem P1 #1: Reload's lastGood map must be safe
// under concurrent calls (simulating the fsnotify debounce-timer goroutine racing
// multiple simultaneous ReloadClaudeSettingsRules RPC calls). Run with -race.
func TestClaudeSettingsWatcher_ConcurrentReloadCalls_NoRace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeSettingsFile(t, filepath.Join(home, ".claude", "settings.json"),
		`{"permissions":{"allow":["Bash(git *)"]}}`)

	spy := &reloadSpy{}
	w := NewClaudeSettingsWatcher("", spy.callback())

	const n = 50
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			w.Reload(context.Background())
		}()
	}
	wg.Wait()

	assert.Equal(t, n, spy.count())
}

func TestClaudeSettingsWatcher_Start_DebouncesRapidWrites(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	writeSettingsFile(t, settingsPath, `{"permissions":{"allow":["Bash(git *)"]}}`)

	spy := &reloadSpy{}
	w := NewClaudeSettingsWatcher("", spy.callback())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx) // performs one synchronous Reload before entering the loop

	require.Equal(t, 1, spy.count(), "Start's initial synchronous Reload should have fired once")
	assert.False(t, spy.last().notify, "Start's initial priming reload must not be notification-worthy")

	for i := 0; i < 5; i++ {
		writeSettingsFile(t, settingsPath, `{"permissions":{"allow":["Bash(git *)","Read"]}}`)
		time.Sleep(5 * time.Millisecond)
	}

	// Debounce window is 250ms; wait past it for the coalesced reload to fire.
	assert.Eventually(t, func() bool {
		return spy.count() == 2
	}, time.Second, 10*time.Millisecond, "5 rapid writes should coalesce into exactly one additional reload")
	assert.True(t, spy.last().notify, "a real fsnotify-triggered reload must be notification-worthy")
}

// TestClaudeSettingsWatcher_Start_InitialPrimingReload_NeverNotifies is the direct
// regression test for the reviewed defect: Start()'s initial synchronous reload always
// invoked onReload with no way to distinguish it from a real change, so the wired-up
// callback in NewSessionServiceWithSearchEngine unconditionally published a "Claude
// Settings Reloaded" notification on every single server startup — including a spurious
// "0 claude-settings rule(s) reloaded" toast when no rules are configured at all, and a
// redundant duplicate of the startup activation notification when rules do exist.
func TestClaudeSettingsWatcher_Start_InitialPrimingReload_NeverNotifies(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// No settings files at all — mirrors the "zero rules configured" case the reviewer
	// flagged as producing spurious noise on every restart.

	spy := &reloadSpy{}
	w := NewClaudeSettingsWatcher("", spy.callback())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)

	require.Equal(t, 1, spy.count())
	assert.Equal(t, 0, spy.last().ruleCount)
	assert.False(t, spy.last().notify, "the priming reload must never be notification-worthy, regardless of rule count")
}

func TestClaudeSettingsWatcher_Start_NoPanicWhenWatcherUnavailable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// No settings dir created at all — parent directories don't exist yet.

	spy := &reloadSpy{}
	w := NewClaudeSettingsWatcher("", spy.callback())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		w.Start(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return/hang gracefully with no watchable directories")
	}
}

func TestClaudeSettingsWatcher_Start_DetectsExternalFileEdit_ReloadsWithoutRestart(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	writeSettingsFile(t, settingsPath, `{"permissions":{"allow":["Bash(git *)"]}}`)

	spy := &reloadSpy{}
	w := NewClaudeSettingsWatcher("", spy.callback())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)
	require.Equal(t, 1, spy.count())

	// Atomic-rename-save: write to a temp file then rename over the original, mimicking
	// most editors' save behavior.
	tmpPath := settingsPath + ".tmp"
	writeSettingsFile(t, tmpPath, `{"permissions":{"allow":["Bash(git *)","Bash(npm test*)"]}}`)
	require.NoError(t, os.Rename(tmpPath, settingsPath))

	assert.Eventually(t, func() bool {
		return spy.count() == 2 && spy.last().ruleCount == 2
	}, time.Second, 10*time.Millisecond, "external edit should be picked up without a restart")

	w.Stopped()
	cancel()
	select {
	case <-w.Stopped():
	case <-time.After(time.Second):
		t.Fatal("watcher goroutine did not exit after context cancellation")
	}
}

// TestClaudeSettingsWatcher_Start_IgnoresUnrelatedFileInWatchedDirectory is a regression test
// found in review: fsnotify.Watcher.Add watches an entire directory, not a single file, but
// run()'s event handling had no filename check — so any write anywhere in ~/.claude/ (a
// directory Claude Code itself writes other files into) would trigger a full re-parse and a
// notification-worthy reload, even though nothing about the actual settings files changed.
// This deviated from the plan's own spec (Task 4.1.1c: "ignore unrelated files in the same
// dir") and risked toast-spam that would undermine Requirement 5's "visible signal" framing.
func TestClaudeSettingsWatcher_Start_IgnoresUnrelatedFileInWatchedDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	writeSettingsFile(t, settingsPath, `{"permissions":{"allow":["Bash(git *)"]}}`)

	spy := &reloadSpy{}
	w := NewClaudeSettingsWatcher("", spy.callback())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)
	require.Equal(t, 1, spy.count(), "priming reload")

	// Write a file in the same directory that is NOT one of the watched settings filenames.
	writeSettingsFile(t, filepath.Join(home, ".claude", "unrelated-state.json"), `{"noise":true}`)

	// Give the debounce window (250ms) more than enough time to have fired if the write were
	// (incorrectly) treated as relevant, then confirm no additional reload happened.
	time.Sleep(400 * time.Millisecond)
	assert.Equal(t, 1, spy.count(), "a write to an unrelated file in the same directory must not trigger a reload")
}
