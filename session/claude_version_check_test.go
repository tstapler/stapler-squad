package session

import (
	"context"
	"errors"
	"log/slog"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

// capturedRecord is one slog.Record captured by recordingHandler, flattened
// to its level/message/attrs for easy assertions.
type capturedRecord struct {
	level slog.Level
	msg   string
	attrs map[string]any
}

// recordingHandler is a minimal slog.Handler that records every record it
// receives, for asserting exact log counts/fields -- mirrors jules/
// keychain_test.go's warnHandler precedent, generalized to every level and
// every attribute (rather than one fixed message) since these tests need to
// distinguish log.Error from log.Warn and inspect specific field values.
type recordingHandler struct {
	mu      sync.Mutex
	records []capturedRecord
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	attrs := make(map[string]any, r.NumAttrs())
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.Any()
		return true
	})
	h.mu.Lock()
	h.records = append(h.records, capturedRecord{level: r.Level, msg: r.Message, attrs: attrs})
	h.mu.Unlock()
	return nil
}

func (h *recordingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *recordingHandler) WithGroup(string) slog.Handler      { return h }

func (h *recordingHandler) recordsAtLevel(level slog.Level) []capturedRecord {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []capturedRecord
	for _, r := range h.records {
		if r.level == level {
			out = append(out, r)
		}
	}
	return out
}

func (h *recordingHandler) all() []capturedRecord {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]capturedRecord(nil), h.records...)
}

// installRecordingHandler installs a recordingHandler as the injectable slog
// seam (log.SetSlogDefaultForTest) log.Warn/log.Error actually read from --
// not slog.SetDefault, which log/log.go's logAt no longer consults -- for the
// duration of the calling test, restoring the prior default via t.Cleanup.
func installRecordingHandler(t *testing.T) *recordingHandler {
	t.Helper()
	h := &recordingHandler{}
	prev := log.SetSlogDefaultForTest(slog.New(h))
	t.Cleanup(func() { log.SetSlogDefaultForTest(prev) })
	return h
}

// fakeVersionCommandRunner is a CommandRunner test double for
// scrollForwardVersionMismatchCheck's `<binaryPath> --version` shell-out, so
// tests don't depend on a real claude binary being on PATH.
type fakeVersionCommandRunner struct {
	mu    sync.Mutex
	out   []byte
	err   error
	calls int
}

func (f *fakeVersionCommandRunner) Output(cmd *exec.Cmd) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.out, f.err
}

func (f *fakeVersionCommandRunner) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// TestScrollForwardVersionMismatchCheck_should_LogErrorOnceForPath_When_InstalledVersionDiffersFromVerified
// is REQ-14b's happy-path fire case (validation.md): a mismatch logs exactly
// one log.Error for a given binary path, and a second check against the same
// path is a no-op (memoized), not a second shell-out or a second log line.
func TestScrollForwardVersionMismatchCheck_should_LogErrorOnceForPath_When_InstalledVersionDiffersFromVerified(t *testing.T) {
	h := installRecordingHandler(t)
	runner := &fakeVersionCommandRunner{out: []byte("2.2.0\n")}
	binaryPath := "/usr/local/bin/claude-mismatch-" + t.Name()

	scrollForwardVersionMismatchCheck(context.Background(), binaryPath, "2.1.270", runner)
	scrollForwardVersionMismatchCheck(context.Background(), binaryPath, "2.1.270", runner)

	if got := runner.callCount(); got != 1 {
		t.Fatalf("CommandRunner.Output called %d times, want exactly 1 (memoized per binary path)", got)
	}
	errs := h.recordsAtLevel(slog.LevelError)
	if len(errs) != 1 {
		t.Fatalf("got %d log.Error records, want exactly 1", len(errs))
	}
	if errs[0].attrs["binary_path"] != binaryPath {
		t.Fatalf("binary_path = %v, want %v", errs[0].attrs["binary_path"], binaryPath)
	}
	if errs[0].attrs["expected_version"] != "2.1.270" {
		t.Fatalf("expected_version = %v, want 2.1.270", errs[0].attrs["expected_version"])
	}
	if errs[0].attrs["actual_version"] != "2.2.0" {
		t.Fatalf("actual_version = %v, want 2.2.0", errs[0].attrs["actual_version"])
	}
}

// TestScrollForwardVersionMismatchCheck_should_LogNothing_When_InstalledVersionMatchesVerified
// is REQ-14b's silent/match case. Uses the real observed `claude --version`
// output shape (research/stack.md: "2.1.270 (Claude Code)", not a bare
// version string) -- an earlier TrimSpace-only normalizeClaudeVersion treated
// this as a mismatch against every real install, caught by ForwardScroll's
// own tests actually shelling out to the real binary in this environment.
func TestScrollForwardVersionMismatchCheck_should_LogNothing_When_InstalledVersionMatchesVerified(t *testing.T) {
	h := installRecordingHandler(t)
	runner := &fakeVersionCommandRunner{out: []byte("2.1.270 (Claude Code)\n")}
	binaryPath := "/usr/local/bin/claude-match-" + t.Name()

	scrollForwardVersionMismatchCheck(context.Background(), binaryPath, "2.1.270", runner)

	if got := h.all(); len(got) != 0 {
		t.Fatalf("got %d log records, want 0 for a matching version: %+v", len(got), got)
	}
}

// TestScrollForwardVersionMismatchCheck_should_LogWarnNotError_When_VersionLookupFails
// is REQ-14b's lookup-failure case: a run error (binary not found, non-zero
// exit) is a lower-severity condition than a confirmed mismatch, not the same
// thing reported the same way -- log.Warn, never log.Error, and no mismatch
// is claimed.
func TestScrollForwardVersionMismatchCheck_should_LogWarnNotError_When_VersionLookupFails(t *testing.T) {
	h := installRecordingHandler(t)
	runner := &fakeVersionCommandRunner{err: errors.New(`fake: exec: "claude": executable file not found in $PATH`)}
	binaryPath := "/usr/local/bin/claude-lookup-fail-" + t.Name()

	scrollForwardVersionMismatchCheck(context.Background(), binaryPath, "2.1.270", runner)

	if errs := h.recordsAtLevel(slog.LevelError); len(errs) != 0 {
		t.Fatalf("got %d log.Error records, want 0 -- a lookup failure must not claim a mismatch", len(errs))
	}
	warns := h.recordsAtLevel(slog.LevelWarn)
	if len(warns) != 1 {
		t.Fatalf("got %d log.Warn records, want exactly 1", len(warns))
	}
}

// TestScrollForwardVersionMismatchCheck_should_FireIndependentlyPerDistinctBinaryPath
// closes Story 1.5.3's memoization-key acceptance criteria: the guard is keyed
// by binary path, not global -- two distinct paths each get their own check.
func TestScrollForwardVersionMismatchCheck_should_FireIndependentlyPerDistinctBinaryPath(t *testing.T) {
	h := installRecordingHandler(t)
	runnerA := &fakeVersionCommandRunner{out: []byte("2.2.0\n")}
	runnerB := &fakeVersionCommandRunner{out: []byte("2.2.0\n")}
	pathA := "/usr/local/bin/claude-a-" + t.Name()
	pathB := "/opt/claude/bin/claude-b-" + t.Name()

	scrollForwardVersionMismatchCheck(context.Background(), pathA, "2.1.270", runnerA)
	scrollForwardVersionMismatchCheck(context.Background(), pathB, "2.1.270", runnerB)

	if errs := h.recordsAtLevel(slog.LevelError); len(errs) != 2 {
		t.Fatalf("got %d log.Error records for two distinct binary paths, want 2", len(errs))
	}
}

// TestScrollForwardVersionMismatchCheck_should_FireRegardlessOfScrollVolume_When_SessionNeverScrolls
// is REQ-14b's regression guard for the specific gap pre-mortem P1 #1
// identified: unlike scrollForwardKeybindingCanary, this check must not
// require any ForwardScroll call to have happened. finishInstanceConstruction
// (session/instance.go) -- the one choke-point every Instance construction
// site funnels through -- is where kickOffClaudeVersionMismatchCheck actually
// fires; this test builds an Instance via that path and asserts the check
// runs (surfaced here as a log.Warn from the binary-not-found lookup failure,
// since there's no real claude binary at the test's throwaway path) without
// ForwardScroll ever being called.
func TestScrollForwardVersionMismatchCheck_should_FireRegardlessOfScrollVolume_When_SessionNeverScrolls(t *testing.T) {
	h := installRecordingHandler(t)
	binaryPath := filepath.Join(t.TempDir(), "claude")

	inst := &Instance{Title: t.Name(), Program: binaryPath}
	finishInstanceConstruction(inst) // never calls ForwardScroll

	wait.RequireEventually(t, func() bool {
		return len(h.recordsAtLevel(slog.LevelWarn)) > 0
	}, time.Second, 10*time.Millisecond, "expected scrollForwardVersionMismatchCheck to fire from finishInstanceConstruction alone")

	warns := h.recordsAtLevel(slog.LevelWarn)
	if warns[0].attrs["binary_path"] != binaryPath {
		t.Fatalf("binary_path = %v, want %v", warns[0].attrs["binary_path"], binaryPath)
	}
}

// TestNormalizeClaudeVersion_should_ExtractLeadingSemverToken covers the real
// observed `claude --version` output shape (research/stack.md), not just a
// bare version string.
func TestNormalizeClaudeVersion_should_ExtractLeadingSemverToken(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{raw: "2.1.270 (Claude Code)\n", want: "2.1.270"},
		{raw: "2.1.270\n", want: "2.1.270"},
		{raw: "  2.1.270 (Claude Code)  ", want: "2.1.270"},
		{raw: "unexpected future format", want: "unexpected future format"},
	}
	for _, tt := range tests {
		if got := normalizeClaudeVersion(tt.raw); got != tt.want {
			t.Fatalf("normalizeClaudeVersion(%q) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}

// TestClaudeBinaryPathFromProgram_should_ExtractClaudeToken covers the small
// program-string parsing helper kickOffClaudeVersionMismatchCheck relies on.
func TestClaudeBinaryPathFromProgram_should_ExtractClaudeToken(t *testing.T) {
	tests := []struct {
		program string
		want    string
	}{
		{program: "claude", want: "claude"},
		{program: "/usr/local/bin/claude --resume abc", want: "/usr/local/bin/claude"},
		{program: "env -u FOO claude --dangerously-skip-permissions", want: "claude"},
		{program: "pi --resume abc", want: ""},
		{program: "", want: ""},
	}
	for _, tt := range tests {
		if got := claudeBinaryPathFromProgram(tt.program); got != tt.want {
			t.Fatalf("claudeBinaryPathFromProgram(%q) = %q, want %q", tt.program, got, tt.want)
		}
	}
}
