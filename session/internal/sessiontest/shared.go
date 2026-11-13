package sessiontest

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/log"
	ssqlog "github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/classifier"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/git"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.uber.org/goleak"
)

// KnownBackgroundGoroutines lists process-wide goroutines started by the log
// package and lumberjack at init/first-log time, plus the signal-handler
// goroutine started by TestMain that blocks on a signal channel for the
// lifetime of the test binary.  These are not actor leaks; goleak must be
// told to ignore them so the actor-leak assertions stay tight.
var KnownBackgroundGoroutines = []goleak.Option{
	goleak.IgnoreTopFunction("github.com/tstapler/stapler-squad/log.newAsyncWriter.func1"),
	goleak.IgnoreTopFunction("github.com/tstapler/stapler-squad/log.(*AsyncHandler).StartDrain.func1"),
	goleak.IgnoreAnyFunction("gopkg.in/natefinch/lumberjack%2ev2.(*Logger).millRun"),
	// TestMain spawns a signal-handler goroutine (integration_test.go) that
	// blocks on <-sigCh for the entire test-binary lifetime.  It cannot be
	// stopped without os.Exit, so we tell goleak to ignore it.
	goleak.IgnoreTopFunction("github.com/tstapler/stapler-squad/session.TestMain.func1"),
	goleak.IgnoreTopFunction("github.com/tstapler/stapler-squad/session.TestMain.func2"),
}

// CapturedRecord is one slog.Record captured by RecordingHandler, flattened
// to its level/message/attrs for easy assertions.
type CapturedRecord struct {
	level slog.Level
	Msg   string
	Attrs map[string]any
}

// RecordingHandler is a minimal slog.Handler that records every record it
// receives, for asserting exact log counts/fields -- mirrors jules/
// keychain_test.go's warnHandler precedent, generalized to every level and
// every attribute (rather than one fixed message) since these tests need to
// distinguish log.Error from log.Warn and inspect specific field values.
type RecordingHandler struct {
	mu      sync.Mutex
	records []CapturedRecord
}

func (h *RecordingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *RecordingHandler) Handle(_ context.Context, r slog.Record) error {
	attrs := make(map[string]any, r.NumAttrs())
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.Any()
		return true
	})
	h.mu.Lock()
	h.records = append(h.records, CapturedRecord{level: r.Level, Msg: r.Message, Attrs: attrs})
	h.mu.Unlock()
	return nil
}

func (h *RecordingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }

func (h *RecordingHandler) WithGroup(string) slog.Handler { return h }

func (h *RecordingHandler) RecordsAtLevel(level slog.Level) []CapturedRecord {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []CapturedRecord
	for _, r := range h.records {
		if r.level == level {
			out = append(out, r)
		}
	}
	return out
}

func (h *RecordingHandler) All() []CapturedRecord {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]CapturedRecord(nil), h.records...)
}

// InstallRecordingHandler installs a RecordingHandler as the injectable slog
// seam (log.SetSlogDefaultForTest) log.Warn/log.Error actually read from --
// not slog.SetDefault, which log/log.go's logAt no longer consults -- for the
// duration of the calling test, restoring the prior default via t.Cleanup.
func InstallRecordingHandler(t *testing.T) *RecordingHandler {
	t.Helper()
	h := &RecordingHandler{}
	prev := log.SetSlogDefaultForTest(slog.New(h))
	t.Cleanup(func() { log.SetSlogDefaultForTest(prev) })
	return h
}

// MockProcessInspector is a test double for ProcessFileInspector.
type MockProcessInspector struct {
	Files []string
	Err   error
}

func (m *MockProcessInspector) OpenFiles(pid int32) ([]string, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return m.Files, nil
}

func (m *MockProcessInspector) IsAlive(pid int32, expectedCreateTimeMs int64) bool {
	return m.Err == nil
}

// NewTestGitWorktree builds a GitWorktree value for use in unit tests without
// touching the real filesystem or running git commands.
func NewTestGitWorktree(repoPath, worktreePath string) *git.GitWorktree {
	return git.NewGitWorktreeFromStorage(repoPath, worktreePath, "test-session", "test-branch", "abc123")
}

// CaptureLogInfo redirects the log package's slog seam to a text handler writing into the returned buffer.
func CaptureLogInfo(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := ssqlog.SetSlogDefaultForTest(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() {
		ssqlog.SetSlogDefaultForTest(prev)
	})
	return &buf
}

// BugfixSeedRule is a minimal TaggingRule mirroring SeedTaggingRules()' seed-tag-bugfix entry,
// used directly (rather than the full seed set) so these tests assert against one predictable
// rule instead of every seed rule's incidental side effects.
func BugfixSeedRule() classifier.TaggingRule {
	return classifier.TaggingRule{
		RuleMeta:      classifier.RuleMeta{ID: "seed-bugfix", Name: "Bugfix branch", Priority: 100, Enabled: true, Source: "seed"},
		BranchPattern: regexp.MustCompile(`^(bugfix|fix)/`),
		OutputTag:     "Bugfix",
	}
}

// CheckTmuxAvailable skips the test if tmux is not installed.
func CheckTmuxAvailable(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available on this system")
	}
}

// ColdRestoreSocket returns a unique tmux server socket name for cold-restore
// tests and registers a t.Cleanup that kills the isolated server on test exit.
// The PID is embedded in the name so TestMain's PID-aware sweep can reap this
// socket on the next run if the current run was killed with SIGKILL.
func ColdRestoreSocket(t *testing.T) string {
	t.Helper()
	name := fmt.Sprintf("test_coldrestore_%d_%d", os.Getpid(), time.Now().UnixNano())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = safeexec.CommandContext(ctx, "tmux", "-L", name, "kill-server").Run()
	})
	return name
}

// StubClaudeBinary writes an executable "claude" stub (a plain `sleep 300`
// script) to a temp dir and returns its path. Using this as Instance.Program
// (instead of the literal string "claude") keeps isClaude()'s basename match
// — so buildLaunchCommand embeds --resume — while guaranteeing the tmux pane
// can actually exec something that stays alive: the real claude CLI is not
// installed on CI runners, only on a developer machine that happens to have
// it in PATH, and a pane whose command exits almost instantly makes
// TmuxAlive() a race against remain-on-exit instead of a reliable check.
func StubClaudeBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "claude")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\nsleep 300\n"), 0o755))
	return path
}

// MarkStartedRecorder is a test LifecycleListener that records every
// EventStarted reason it observes, for asserting MarkStartedIfTmuxAlive
// fires (or doesn't fire) EventStarted as expected.
type MarkStartedRecorder struct {
	mu      sync.Mutex
	reasons []string
}

func (r *MarkStartedRecorder) OnLifecycleEvent(event domain.LifecycleEvent, reason string) {
	if event != domain.EventStarted {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reasons = append(r.reasons, reason)
}

func (r *MarkStartedRecorder) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.reasons)
}

func NewBugfixTaggingEngine() *classifier.TaggingEngine {
	engine := classifier.NewTaggingEngine()
	engine.ReplaceRules([]classifier.TaggingRule{BugfixSeedRule()})
	return engine
}

// TestPiMetricReader backs the single real MeterProvider installed for this
// test binary. Package-level instruments (piStatusSourceEventsTotal) are
// constructed against OTel's global delegating meter at package-init time,
// before this file's init() runs -- otel.SetMeterProvider rewires what that
// delegating meter forwards to, so a process-lifetime manual reader with
// delta-based (before/after) assertions is the correct fit here, mirroring
// server/services/session_creation_metrics_test.go's identical pattern.
var TestPiMetricReader = sdkmetric.NewManualReader()

// InitTestRepoForReviewQueue creates and commits a minimal git repository for the
// uncommitted-changes detection tests below.
//
// Uses go-git directly rather than shelling out — see
// the `prefer-go-git-over-subshells` skill.
func InitTestRepoForReviewQueue(t *testing.T, repoPath string) {
	t.Helper()
	repo, err := gogit.PlainInit(repoPath, false)
	if err != nil {
		t.Fatalf("Failed to initialize git repo: %v", err)
	}

	testFile := filepath.Join(repoPath, "test.txt")
	if err := os.WriteFile(testFile, []byte("initial content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Failed to get worktree: %v", err)
	}
	if _, err := wt.Add("."); err != nil {
		t.Fatalf("Failed to git add: %v", err)
	}
	if _, err := wt.Commit("Initial commit", &gogit.CommitOptions{
		Author: &object.Signature{Name: "Test User", Email: "test@example.com", When: time.Now()},
	}); err != nil {
		t.Fatalf("Failed to git commit: %v", err)
	}
}

// AllStatuses lists every Status constant in the new 5-state model.
var AllStatuses = []domain.Status{domain.Creating, domain.Active, domain.Paused, domain.Stopped, domain.Hibernated}

// ValidTransitionSet is the ground truth for TestCanTransition_ExhaustiveMatrix.
// Any pair not listed here must return false from CanTransition.
var ValidTransitionSet = map[[2]domain.Status]bool{
	// Creating
	{domain.Creating, domain.Active}:  true,
	{domain.Creating, domain.Stopped}: true,
	// Active
	{domain.Active, domain.Paused}:     true,
	{domain.Active, domain.Stopped}:    true,
	{domain.Active, domain.Hibernated}: true,
	// Paused
	{domain.Paused, domain.Active}:  true,
	{domain.Paused, domain.Stopped}: true,
	// Stopped — recoverable: Stopped → Active is allowed for session revival
	{domain.Stopped, domain.Active}: true,
	// Hibernated
	{domain.Hibernated, domain.Active}:  true,
	{domain.Hibernated, domain.Stopped}: true,
}
