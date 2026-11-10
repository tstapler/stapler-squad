package sessiontest

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/testutil/gitfixture"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

// setupTestGitRepo creates a temporary git repository with an initial commit and a
// configured test identity, mirroring session/git/worktree_creation_test.go's
// setupTestRepo helper (duplicated here rather than imported since that one lives in
// package git and is unexported). Uses go-git directly rather than shelling out — see
// the `prefer-go-git-over-subshells` skill.
func SetupTestGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	repo, err := gogit.PlainInitWithOptions(dir, &gogit.PlainInitOptions{
		InitOptions: gogit.InitOptions{DefaultBranch: plumbing.NewBranchReferenceName("main")},
	})
	if err != nil {
		t.Fatalf("git init failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test\n"), 0o644); err != nil {
		t.Fatalf("failed to write README.md: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree failed: %v", err)
	}
	if _, err := wt.Add("README.md"); err != nil {
		t.Fatalf("git add failed: %v", err)
	}
	if _, err := wt.Commit("Initial commit", &gogit.CommitOptions{
		Author: &object.Signature{Name: "Test User", Email: "test@example.com", When: time.Now()},
	}); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}

	return dir
}

// setupLinkedWorktree builds a real linked worktree (git worktree add topology)
// on top of an existing main repo — the topology where --git-dir and
// --git-common-dir resolve to different directories, which a plain setupTestGitRepo
// fixture cannot exercise.
func SetupLinkedWorktree(t *testing.T, repoPath, sessionName string) string {
	t.Helper()
	worktree, _, err := git.NewGitWorktree(repoPath, sessionName)
	if err != nil {
		t.Fatalf("git.NewGitWorktree failed: %v", err)
	}
	if err := worktree.Setup(); err != nil {
		t.Fatalf("worktree.Setup failed: %v", err)
	}
	t.Cleanup(func() {
		if err := worktree.Cleanup(); err != nil {
			t.Logf("worktree.Cleanup failed (non-fatal): %v", err)
		}
	})
	return worktree.GetWorktreePath()
}

// GitLsFiles returns the list of paths currently tracked in the git index at dir.
func GitLsFiles(t *testing.T, dir string) []string { return gitfixture.LsFiles(t, dir) }

// containsPath reports whether paths contains target.
func ContainsPath(paths []string, target string) bool {
	for _, p := range paths {
		if p == target {
			return true
		}
	}
	return false
}

// fakeNotifierCall records a single Notify/NotifySession invocation's title, message
// body, notification type, and urgent/important axes, so tests can assert on
// interpolated message content (e.g. that a verdict/outcome actually reached
// the message) and on differentiated ERROR/URGENT vs WARNING/HIGH severity,
// not just which notification fired. Method/RecipientID (added for
// session/worktree_consistency_sweep.go's Architecture-A1 regression guard) record which
// Notifier method fired and the itemID (Notify) or sessionID (NotifySession) it was
// called with.
type FakeNotifierCall struct {
	Method           string // "Notify" or "NotifySession"
	RecipientID      string // itemID (Notify) or sessionID (NotifySession)
	Title            string
	Message          string
	NotificationType int32
	Urgent           bool
	Important        bool
}

// fakeNotifier is a test double implementing Notifier, recording every call.
type FakeNotifier struct {
	Calls []FakeNotifierCall // one per Notify/NotifySession call, in order
}

func (f *FakeNotifier) Notify(itemID, title, message string, notificationType int32, urgent, important bool) {
	f.Calls = append(f.Calls, FakeNotifierCall{Method: "Notify", RecipientID: itemID, Title: title, Message: message, NotificationType: notificationType, Urgent: urgent, Important: important})
}

func (f *FakeNotifier) NotifySession(sessionID, title, message string, notificationType int32, urgent, important bool) {
	f.Calls = append(f.Calls, FakeNotifierCall{Method: "NotifySession", RecipientID: sessionID, Title: title, Message: message, NotificationType: notificationType, Urgent: urgent, Important: important})
}

// Titles returns just the Title of every recorded call, in order — for tests (the
// majority) that only care which notification fired, not its message body.
func (f *FakeNotifier) Titles() []string {
	titles := make([]string, len(f.Calls))
	for i, c := range f.Calls {
		titles[i] = c.Title
	}
	return titles
}

// timePtr is a small helper for constructing *time.Time literals inline in
// table-driven/setup code.
func TimePtr(t time.Time) *time.Time { return &t }

// RunGit runs a git command in dir for test setup, failing the test on error.
func RunGit(t *testing.T, dir string, args ...string) { gitfixture.Run(t, dir, args...) }

// waitForClockDelta polls until the wall clock has advanced by delta, so two
// persisted timestamps taken around the call are guaranteed to differ.
func WaitForClockDelta(t *testing.T, delta time.Duration) {
	t.Helper()
	start := time.Now()
	wait.RequireEventually(t, func() bool { return time.Since(start) >= delta }, delta+5*time.Second, time.Millisecond)
}

// GitTemplateRepos caches one committed repo per branch name. Copying it replaces
// six git subprocesses per test (init, two configs, add, commit), whose
// fork/exec serializes on the process-wide fork lock when many parallel tests
// set up repos at once.
var GitTemplateRepos = struct {
	Mu   sync.Mutex
	Dirs map[string]string
}{Dirs: map[string]string{}}

// RemoveGitTemplateRepos deletes the cached template repos; call it at the end of TestMain.
func RemoveGitTemplateRepos() {
	GitTemplateRepos.Mu.Lock()
	defer GitTemplateRepos.Mu.Unlock()
	for _, dir := range GitTemplateRepos.Dirs {
		_ = os.RemoveAll(dir)
	}
}

const RefsTestSHA = "0123456789abcdef0123456789abcdef01234567"

// RunGitOrFail runs a git command in dir and fails the test on error, for building real repo fixtures.
func RunGitOrFail(t *testing.T, dir string, args ...string) { gitfixture.Run(t, dir, args...) }

// RunGitOutputOrFail is RunGitOrFail returning the combined output.
func RunGitOutputOrFail(t *testing.T, dir string, args ...string) string {
	return gitfixture.RunOutput(t, dir, args...)
}

// cloneWithOrigin clones originDir into a fresh temp directory with a real "origin"
// remote pointing back at originDir, so git.EnsureBranchSyncedWithMain's fetch/merge/
// push calls have something real to talk to (unlike this file's other fixtures, which
// use a single standalone repo with no remote at all — fine for GetGitDiff, but not
// for exercising the BUG-044 drift precondition, which fetches before every check).
func CloneWithOrigin(t *testing.T, originDir string) string {
	t.Helper()
	parent := t.TempDir()
	cloneDir := filepath.Join(parent, "clone")
	RunGitOrFail(t, parent, "clone", originDir, cloneDir)
	RunGitOrFail(t, cloneDir, "config", "user.email", "test@example.com")
	RunGitOrFail(t, cloneDir, "config", "user.name", "Test")
	return cloneDir
}

// scheduleWakeupToken is the client-side wake tool agents must not use. It is
// spelled in two parts so this file, and every test that needs the word, does
// not count as a reference itself.
const ClientWakeToolToken = "Schedule" + "Wakeup"

type SelectorClock struct{ now atomic.Int64 }

func NewSelectorClock() *SelectorClock {
	c := &SelectorClock{}
	c.now.Store(time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC).UnixNano())
	return c
}

func (c *SelectorClock) Now() time.Time { return time.Unix(0, c.now.Load()).UTC() }

func (c *SelectorClock) Advance(d time.Duration) { c.now.Add(int64(d)) }

var SoakNow = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
