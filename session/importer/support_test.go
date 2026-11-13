package importer

// Test doubles the importer tests share with package session's own tests. They are copies: a _test.go helper of
// package session is not visible here, and session cannot import this package.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/tmux"
)

func makeTestInstance(title string) *session.Instance {
	return &session.Instance{Title: title, Status: domain.Active}
}

// newTestGitWorktree builds a GitWorktree value without touching the filesystem or running git.
func newTestGitWorktree(repoPath, worktreePath string) *git.GitWorktree {
	return git.NewGitWorktreeFromStorage(repoPath, worktreePath, "test-session", "test-branch", "abc123")
}

// createTestStorage returns a Storage over a fresh in-memory test repository; the cleanup func is a no-op kept so
// call sites mirror session's own helper of the same name.
func createTestStorage(t *testing.T) (*session.Storage, func()) {
	t.Helper()
	storage, err := session.NewStorageWithRepository(session.NewTestEntRepository(t))
	require.NoError(t, err)
	return storage, func() {}
}

// mockProcessInspector is a test double for session.ProcessFileInspector.
type mockProcessInspector struct {
	files []string
	err   error
}

func (m *mockProcessInspector) OpenFiles(pid int32) ([]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.files, nil
}

func (m *mockProcessInspector) IsAlive(pid int32, expectedCreateTimeMs int64) bool {
	return m.err == nil
}

// coldRestoreSocket returns a private tmux socket name and kills its server when the test ends.
func coldRestoreSocket(t *testing.T) string {
	t.Helper()
	name := fmt.Sprintf("test_coldrestore_%d_%d", os.Getpid(), time.Now().UnixNano())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = safeexec.CommandContext(ctx, "tmux", "-L", name, "kill-server").Run()
	})
	return name
}

// fakeTmuxSocketQuerier is an in-memory session.TmuxSocketQuerier.
type fakeTmuxSocketQuerier struct {
	mu       sync.Mutex
	sessions map[string]map[string]bool // socket -> set of live session names
	down     map[string]bool            // socket -> is server down
}

func newFakeTmuxSocketQuerier() *fakeTmuxSocketQuerier {
	return &fakeTmuxSocketQuerier{sessions: map[string]map[string]bool{}, down: map[string]bool{}}
}

func (f *fakeTmuxSocketQuerier) ListSessions(serverSocket string) (map[string]bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down[serverSocket] {
		return nil, tmux.ErrServerDown
	}
	set := f.sessions[serverSocket]
	result := make(map[string]bool, len(set))
	for k, v := range set {
		result[k] = v
	}
	return result, nil
}

func (f *fakeTmuxSocketQuerier) IsServerDown(serverSocket string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.down[serverSocket]
}

// The claim fixture: Claude project directories under a private home, with one jsonl per conversation UUID.
const (
	claimSibling  = "cccccccc-0000-0000-0000-000000000000"
	claimOther    = "dddddddd-0000-0000-0000-000000000000"
	claimOldStart = "eeeeeeee-0000-0000-0000-000000000000"
	claimOwnStart = "ffffffff-0000-0000-0000-000000000000"
)

type claimFixture struct {
	detector *session.HistoryFileDetector
	dir      string
	now      time.Time
}

func newClaimFixture(t *testing.T, repoPath string) *claimFixture {
	t.Helper()
	home := t.TempDir()
	dir := filepath.Join(home, ".claude", "projects", session.ClaudeProjectDirName(repoPath))
	require.NoError(t, os.MkdirAll(dir, 0755))
	detector := session.NewHistoryFileDetectorWithHomeDir(&mockProcessInspector{files: []string{}}, home)
	return &claimFixture{detector: detector, dir: dir, now: time.Now()}
}

// write creates <uuid>.jsonl with the given content and mtime = now - age.
func (f *claimFixture) write(t *testing.T, uuid, content string, age time.Duration) string {
	t.Helper()
	p := filepath.Join(f.dir, uuid+".jsonl")
	require.NoError(t, os.WriteFile(p, []byte(content), 0644))
	mt := f.now.Add(-age)
	require.NoError(t, os.Chtimes(p, mt, mt))
	return p
}

func stampedEntry(ts time.Time) string {
	return `{"type":"queue-operation","timestamp":"` + ts.UTC().Format(time.RFC3339Nano) + `"}` + "\n"
}
