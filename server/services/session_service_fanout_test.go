package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/session/git"
)

func TestFanOutSession_RejectsCountOutsideCap(t *testing.T) {
	t.Parallel()
	svc := &SessionService{}
	for _, count := range []int32{-1, 0, 1, 6, 100} {
		_, err := svc.FanOutSession(context.Background(), connect.NewRequest(&sessionv1.FanOutSessionRequest{
			Base:  &sessionv1.CreateSessionRequest{Title: "t", Path: "/tmp"},
			Count: count,
		}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("count=%d: want InvalidArgument, got %v", count, err)
		}
	}
}

func TestFanOutSession_RejectsMissingTitleAndExcessOverrides(t *testing.T) {
	t.Parallel()
	svc := &SessionService{}
	_, err := svc.FanOutSession(context.Background(), connect.NewRequest(&sessionv1.FanOutSessionRequest{
		Base: &sessionv1.CreateSessionRequest{}, Count: 2,
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("missing title: want InvalidArgument, got %v", err)
	}
	_, err = svc.FanOutSession(context.Background(), connect.NewRequest(&sessionv1.FanOutSessionRequest{
		Base: &sessionv1.CreateSessionRequest{Title: "t"}, Count: 2, ProgramOverrides: []string{"a", "b", "c"},
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("too many overrides: want InvalidArgument, got %v", err)
	}
}

func TestFanoutWinnerEligibility(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		status session.Status
		claude detection.DetectedStatus
		wantOK bool
	}{
		{"stopped", session.Stopped, detection.StatusUnknown, true},
		{"paused", session.Paused, detection.StatusUnknown, true},
		{"active idle", session.Active, detection.StatusIdle, true},
		{"active ready", session.Active, detection.StatusReady, true},
		{"active still working", session.Active, detection.StatusProcessing, false},
		{"creating", session.Creating, detection.StatusUnknown, false},
	}
	for _, c := range cases {
		err := fanoutWinnerEligibility(c.status, c.claude)
		if (err == nil) != c.wantOK {
			t.Errorf("%s: eligible=%v, err=%v", c.name, err == nil, err)
		}
	}
}

func TestPickFanoutWinner_UnknownGroupIsNotFound(t *testing.T) {
	t.Parallel()
	svc := &SessionService{}
	_, err := svc.PickFanoutWinner(context.Background(), connect.NewRequest(&sessionv1.PickFanoutWinnerRequest{
		GroupId: "nope", WinnerSessionId: "x",
	}))
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("want NotFound, got %v", err)
	}
}

// fanoutGitRepo creates a repo with one commit on branch "main" and returns its path.
func fanoutGitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := safeexec.CommandContext(context.Background(), "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

func fanoutGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	fanoutGitRun(t, dir, "init", "-q", "-b", "main")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("base\n"), 0o644))
	fanoutGitRun(t, dir, "add", ".")
	fanoutGitRun(t, dir, "commit", "-q", "-m", "init")
	return dir
}

// fanoutMember builds a stopped instance backed by a real worktree with one uncommitted file.
func fanoutMember(t *testing.T, repo, groupID string, n int, file, content string) *session.Instance {
	t.Helper()
	title := fmt.Sprintf("fan [variant %d]", n)
	wt, _, err := git.NewGitWorktreeWithBranch(repo, title, fmt.Sprintf("fanout-test-v%d", n))
	require.NoError(t, err)
	require.NoError(t, wt.Setup())
	t.Cleanup(func() { _ = wt.Cleanup() })
	require.NoError(t, os.WriteFile(filepath.Join(wt.GetWorktreePath(), file), []byte(content), 0o644))

	inst := &session.Instance{Title: title, UUID: uuid.NewString(), Path: repo, Status: session.Stopped, Program: "claude"}
	inst.SetGitWorktree(wt)
	inst.ForceStatus(session.Stopped) // Snapshot() is what GetStatus reads, not the struct literal
	require.NoError(t, inst.SetTags([]string{fanoutGroupTagPrefix + groupID, fanoutBaseTagPrefix + "main"}))
	return inst
}

func TestPickFanoutWinner_MergesWinnerAndArchivesLosers(t *testing.T) {
	fix := setupForkTestFixture(t)
	repo := fanoutGitRepo(t)
	const groupID = "g-real"
	winner := fanoutMember(t, repo, groupID, 1, "winner.txt", "w\n")
	loser := fanoutMember(t, repo, groupID, 2, "loser.txt", "l\n")
	fix.poller.SetInstances([]*session.Instance{winner, loser})

	resp, err := fix.svc.PickFanoutWinner(context.Background(), connect.NewRequest(&sessionv1.PickFanoutWinnerRequest{
		GroupId: groupID, WinnerSessionId: winner.UUID,
	}))
	require.NoError(t, err)
	assert.Equal(t, "main", resp.Msg.BaseBranch)
	assert.Equal(t, []string{loser.UUID}, resp.Msg.ArchivedSessionIds)
	assert.Empty(t, resp.Msg.FailedArchiveSessionIds)

	assert.FileExists(t, filepath.Join(repo, "winner.txt"), "winner's changes merged into base branch")
	assert.NoFileExists(t, filepath.Join(repo, "loser.txt"), "loser's changes must not land")
	assert.True(t, loser.IsArchived(), "loser archived via ArchiveSession path")
	assert.False(t, winner.IsArchived())
}

func TestPickFanoutWinner_ConflictReturnsAbortedAndArchivesNothing(t *testing.T) {
	fix := setupForkTestFixture(t)
	repo := fanoutGitRepo(t)
	const groupID = "g-conflict"
	winner := fanoutMember(t, repo, groupID, 1, "a.txt", "from winner\n")
	loser := fanoutMember(t, repo, groupID, 2, "loser.txt", "l\n")
	fix.poller.SetInstances([]*session.Instance{winner, loser})

	// Diverge the base branch on the same file so the winner's merge conflicts.
	require.NoError(t, os.WriteFile(filepath.Join(repo, "a.txt"), []byte("from base\n"), 0o644))
	fanoutGitRun(t, repo, "add", ".")
	fanoutGitRun(t, repo, "commit", "-q", "-m", "base edit")

	_, err := fix.svc.PickFanoutWinner(context.Background(), connect.NewRequest(&sessionv1.PickFanoutWinnerRequest{
		GroupId: groupID, WinnerSessionId: winner.UUID,
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeAborted, connect.CodeOf(err))
	assert.False(t, loser.IsArchived(), "no loser is archived when the merge fails")
}

func TestPickFanoutWinner_RejectsStillRunningWinner(t *testing.T) {
	fix := setupForkTestFixture(t)
	repo := fanoutGitRepo(t)
	const groupID = "g-running"
	winner := fanoutMember(t, repo, groupID, 1, "w.txt", "w\n")
	winner.ForceStatus(session.Creating)
	fix.poller.SetInstances([]*session.Instance{winner})

	_, err := fix.svc.PickFanoutWinner(context.Background(), connect.NewRequest(&sessionv1.PickFanoutWinnerRequest{
		GroupId: groupID, WinnerSessionId: winner.UUID,
	}))
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
}

func TestPickFanoutWinner_RejectsDirtyBaseRepo(t *testing.T) {
	fix := setupForkTestFixture(t)
	repo := fanoutGitRepo(t)
	const groupID = "g-dirty"
	winner := fanoutMember(t, repo, groupID, 1, "w.txt", "w\n")
	fix.poller.SetInstances([]*session.Instance{winner})
	require.NoError(t, os.WriteFile(filepath.Join(repo, "scratch.txt"), []byte("user edit\n"), 0o644))

	_, err := fix.svc.PickFanoutWinner(context.Background(), connect.NewRequest(&sessionv1.PickFanoutWinnerRequest{
		GroupId: groupID, WinnerSessionId: winner.UUID,
	}))
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	assert.NoFileExists(t, filepath.Join(repo, "w.txt"))
}

func TestPickFanoutWinner_PausedWinnerWithRemovedWorktreeMergesItsBranch(t *testing.T) {
	fix := setupForkTestFixture(t)
	repo := fanoutGitRepo(t)
	const groupID = "g-paused"
	winner := fanoutMember(t, repo, groupID, 1, "w.txt", "w\n")
	wt, err := winner.GetGitWorktree()
	require.NoError(t, err)
	require.NoError(t, wt.CommitChanges("variant work"))
	// pause_session deletes the worktree directory but keeps the branch.
	fanoutGitRun(t, repo, "worktree", "remove", "--force", wt.GetWorktreePath())
	winner.ForceStatus(session.Paused)
	fix.poller.SetInstances([]*session.Instance{winner})

	_, err = fix.svc.PickFanoutWinner(context.Background(), connect.NewRequest(&sessionv1.PickFanoutWinnerRequest{
		GroupId: groupID, WinnerSessionId: winner.UUID,
	}))
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(repo, "w.txt"))
}

func TestPickFanoutWinner_ConcurrentPicksMergeOnlyOneWinner(t *testing.T) {
	fix := setupForkTestFixture(t)
	repo := fanoutGitRepo(t)
	const groupID = "g-race"
	a := fanoutMember(t, repo, groupID, 1, "a-only.txt", "a\n")
	b := fanoutMember(t, repo, groupID, 2, "b-only.txt", "b\n")
	fix.poller.SetInstances([]*session.Instance{a, b})

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, w := range []*session.Instance{a, b} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = fix.svc.PickFanoutWinner(context.Background(), connect.NewRequest(&sessionv1.PickFanoutWinnerRequest{
				GroupId: groupID, WinnerSessionId: w.UUID,
			}))
		}()
	}
	wg.Wait()

	succeeded := 0
	for _, e := range errs {
		if e == nil {
			succeeded++
		}
	}
	assert.Equal(t, 1, succeeded, "exactly one pick wins; errs=%v", errs)
	_, aErr := os.Stat(filepath.Join(repo, "a-only.txt"))
	_, bErr := os.Stat(filepath.Join(repo, "b-only.txt"))
	assert.NotEqual(t, aErr == nil, bErr == nil, "exactly one variant's file is merged")
}

func TestFanOutSession_RejectsInvalidBranchName(t *testing.T) {
	t.Parallel()
	svc := &SessionService{}
	for _, branch := range []string{"-rf", "bad..name", "has space", "end."} {
		_, err := svc.FanOutSession(context.Background(), connect.NewRequest(&sessionv1.FanOutSessionRequest{
			Base:  &sessionv1.CreateSessionRequest{Title: "t", Path: "/tmp", Branch: branch},
			Count: 2,
		}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("branch %q: want InvalidArgument, got %v", branch, err)
		}
	}
}
