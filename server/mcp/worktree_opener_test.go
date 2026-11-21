package mcp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/instance"
)

// realWorktreeOpener is the production constructor, for tests that want a real checkout.
func realWorktreeOpener(repoPath, worktreePath, sessionName, branchName, baseCommitSHA string) Worktree {
	wt := git.NewGitWorktreeFromStorage(repoPath, worktreePath, sessionName, branchName, baseCommitSHA)
	if wt == nil {
		return nil
	}
	return wt
}

// fakeWorktree needs no git checkout or gh CLI; it records how it was opened.
type fakeWorktree struct {
	fakeStuckReportPRChecker
	opened [5]string
	diff   *git.DiffStats
	branch string
}

func (f *fakeWorktree) Diff() *git.DiffStats  { return f.diff }
func (f *fakeWorktree) GetBranchName() string { return f.branch }

func recordingOpener(f *fakeWorktree) WorktreeOpener {
	return func(repoPath, worktreePath, sessionName, branchName, baseCommitSHA string) Worktree {
		f.opened = [5]string{repoPath, worktreePath, sessionName, branchName, baseCommitSHA}
		return f
	}
}

func newCoreWithStorage(t *testing.T, store session.InstanceStore, opts ...CoreOption) (*session.Storage, func(name string, args map[string]interface{}) map[string]any) {
	t.Helper()
	storage := newTestBacklogStorage(t)
	s := NewCore(store, nil, nil, storage, nil, nil, nil, nil, nil, nil, opts...)
	return storage, func(name string, args map[string]interface{}) map[string]any {
		tool := s.GetTool(name)
		require.NotNil(t, tool, "tool %q must be registered", name)
		res, err := tool.Handler(context.Background(), makeToolReq(args))
		require.NoError(t, err)
		return parseResult(t, res)
	}
}

func vcsStore(t *testing.T) *stubStore {
	t.Helper()
	inst := newVCSInstance("sess", t.TempDir())
	return &stubStore{instances: []*instance.Instance{inst}}
}

func TestNewCore_should_UseInjectedWorktreeOpener_ForSessionDiff(t *testing.T) {
	t.Parallel()
	store := vcsStore(t)
	fake := &fakeWorktree{diff: &git.DiffStats{Content: "diff --git a/x b/x\n+y\n", Added: 1}, branch: "feat"}
	_, call := newCoreWithStorage(t, store, WithWorktreeOpener(recordingOpener(fake)))

	got := call("get_session_diff", map[string]interface{}{"session_id": "sess"})

	assert.Equal(t, true, got["success"], "%v", got)
	assert.Equal(t, "diff --git a/x b/x\n+y\n", got["diff"], "the injected fake must serve the diff, not a real worktree")
	assert.Equal(t, "sess", fake.opened[2])
}

func TestNewCore_should_UseInjectedWorktreeOpener_ForSessionBranches(t *testing.T) {
	t.Parallel()
	fake := &fakeWorktree{branch: "from-fake"}
	_, call := newCoreWithStorage(t, vcsStore(t), WithWorktreeOpener(recordingOpener(fake)))

	got := call("list_session_branches", map[string]interface{}{"session_id": "sess"})

	assert.Equal(t, "from-fake", got["current_branch"], "%v", got)
}

func TestNewCore_should_ReportWorktreeUnavailable_When_OpenerAbsentOrNil(t *testing.T) {
	t.Parallel()
	nilOpener := WorktreeOpener(func(string, string, string, string, string) Worktree { return nil })
	for name, opts := range map[string][]CoreOption{"absent": nil, "returns nil": {WithWorktreeOpener(nilOpener)}} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, call := newCoreWithStorage(t, vcsStore(t), opts...)
			got := call("get_session_diff", map[string]interface{}{"session_id": "sess"})
			assert.Contains(t, resultErrField(got, "message"), "cannot open git worktree")
		})
	}
}

func prPendingItem(t *testing.T, storage *session.Storage) string {
	t.Helper()
	ctx := context.Background()
	item, err := storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{
		Title: "t", Priority: 3, Status: string(domain.BacklogStatusPRPending), RepoPath: "/repo",
	})
	require.NoError(t, err)
	_, err = storage.UpdateBacklogItem(ctx, item.ID, backlogdto.BacklogItemUpdate{PrNumber: intPtr(7), PrURL: strPtr("https://github.com/o/r/pull/7")}, nil)
	require.NoError(t, err)
	return item.ID
}

func stuckReportText(t *testing.T, opts ...CoreOption) string {
	t.Helper()
	storage := newTestBacklogStorage(t)
	s := NewCore(&stubStore{}, nil, nil, storage, nil, nil, nil, nil, nil, nil, opts...)
	id := prPendingItem(t, storage)
	res, err := s.GetTool("get_backlog_stuck_report").Handler(context.Background(), makeToolReq(map[string]interface{}{"item_id": id}))
	require.NoError(t, err)
	return toolResultText(t, res)
}

func TestNewCore_should_UseInjectedWorktreeOpener_ForStuckReportCICheck(t *testing.T) {
	t.Parallel()
	status, err := git.ParsePRStatusPayload([]byte(`{"statusCheckRollup":[],"mergeable":"MERGEABLE","state":"OPEN"}`))
	require.NoError(t, err)
	fake := &fakeWorktree{}
	fake.status = status

	text := stuckReportText(t, WithWorktreeOpener(recordingOpener(fake)))

	assert.Contains(t, text, "No failing checks right now.", "the fake's PR status must drive the cross-check")
	assert.Equal(t, [5]string{"/repo", "/repo", "", "", ""}, fake.opened)
}

func TestNewCore_should_ReportCICheckUnavailable_When_OpenerAbsent(t *testing.T) {
	t.Parallel()
	text := stuckReportText(t)
	assert.Contains(t, text, "## Live CI cross-check against this PR's own diff")
	assert.Contains(t, text, "unavailable: no worktree for this repo path")
}

func TestWithWorktreeOpener_should_CarryCallersOpener(t *testing.T) {
	t.Parallel()
	assert.Nil(t, WorktreeOpenerFromOptions())
	assert.NotNil(t, WorktreeOpenerFromOptions(WithWorktreeOpener(realWorktreeOpener)))
}
