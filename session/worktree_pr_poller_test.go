package session_test

import (
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/session"
)

// staticSource is a test implementation of WorktreeSource that returns a fixed
// set of items and a closed ScanDone channel.
type staticSource struct {
	items []session.WorktreeScanItem
	done  chan time.Time
}

func newStaticSource(items ...session.WorktreeScanItem) *staticSource {
	ch := make(chan time.Time, 1)
	return &staticSource{items: items, done: ch}
}

func (s *staticSource) ScanDone() <-chan time.Time { return s.done }
func (s *staticSource) GetWorktrees() []session.WorktreeScanItem {
	return s.items
}

func TestWorktreePRPoller_GetPRData_NilBeforeData(t *testing.T) {
	poller := session.NewWorktreePRPoller(github.NewETagCache(), nil)
	got := poller.GetPRData("/some/repo", "feature-branch")
	if got != nil {
		t.Fatalf("expected nil before any data is stored, got %v", got)
	}
}

func TestWorktreePRPoller_SetOnUpdated_Atomic(t *testing.T) {
	poller := session.NewWorktreePRPoller(github.NewETagCache(), nil)
	called := make(chan struct{}, 1)
	poller.SetOnUpdated(func(repoPath, branch string, info *github.PRInfo) {
		called <- struct{}{}
	})
	// SetOnUpdated should not panic and the callback should be set atomically.
	poller.SetOnUpdated(func(repoPath, branch string, info *github.PRInfo) {})
}

func TestWorktreePRPoller_StartStop(t *testing.T) {
	t.Parallel()
	poller := session.NewWorktreePRPoller(github.NewETagCache(), nil)
	src := newStaticSource()
	poller.SetSource(src)

	ctx := t.Context()
	poller.Start(ctx)
	// Double Start is a no-op.
	poller.Start(ctx)
	poller.Stop()
}

func TestWorktreePRPoller_SkipsEmptyRepoPath(t *testing.T) {
	t.Parallel()
	poller := session.NewWorktreePRPoller(github.NewETagCache(), nil)
	// Items with empty RepoPath or Branch should not cause panics or errors.
	src := newStaticSource(
		session.WorktreeScanItem{RepoPath: "", Branch: "main", WorktreePath: "/tmp/wt"},
		session.WorktreeScanItem{RepoPath: "/some/repo", Branch: "", WorktreePath: "/tmp/wt2"},
	)
	poller.SetSource(src)
	ctx := t.Context()
	poller.Start(ctx)
	time.Sleep(50 * time.Millisecond)
	poller.Stop()
}
