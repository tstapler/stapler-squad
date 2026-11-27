package backlog

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/internal/sessiontest"
)

const crossRepoPRURL = "https://github.com/tstapler/stapler-squad/pull/1785"

// installFakeGH puts a `gh` on PATH that knows PR #1785 only in
// tstapler/stapler-squad, like the real one run from another repo's checkout.
// It returns the file every `gh pr` invocation is logged to.
func installFakeGH(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls")
	script := `#!/bin/sh
[ "$1" = auth ] && exit 0
echo "$*" >> '` + calls + `'
case "$*" in
  *"--repo tstapler/stapler-squad"*) ;;
  *) echo "GraphQL: Could not resolve to a PullRequest with the number of 1785." >&2; exit 1 ;;
esac
case "$*" in
  *"--jq .state"*) echo MERGED ;;
  *) echo '{"state":"MERGED"}' ;;
esac
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return calls
}

func newCrossRepoItem(t *testing.T, storage *session.Storage, prURL string) (*backlogdto.BacklogItemData, string) {
	t.Helper()
	ctx := context.Background()
	repoPath := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"remote", "add", "origin", "https://github.com/tstapler/stelekit.git"},
	} {
		out, err := safeexec.CommandContext(ctx, "git", append([]string{"-C", repoPath}, args...)...).CombinedOutput()
		require.NoError(t, err, string(out))
	}
	item, err := storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{
		Title:              "cross-repo item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(domain.BacklogStatusPRPending),
		RepoPath:           repoPath,
	})
	require.NoError(t, err)
	prNumber := 1785
	updated, err := storage.UpdateBacklogItem(ctx, item.ID, backlogdto.BacklogItemUpdate{
		PrURL:    &prURL,
		PrNumber: &prNumber,
	}, nil)
	require.NoError(t, err)
	return updated, repoPath
}

// Not parallel: installFakeGH uses t.Setenv.
func TestReconcilePRPending_CrossRepoMerged(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	calls := installFakeGH(t)

	item, repoPath := newCrossRepoItem(t, storage, crossRepoPRURL)
	newTrackedWorkSession(t, storage, item.ID, repoPath, "feat/gutter", "")
	listener := NewBacklogLifecycleListener(storage)
	listener.SetPRByNumberFinder(func(_ context.Context, _, prURL string, _ int) (*github.PRInfo, error) {
		assert.Equal(t, crossRepoPRURL, prURL)
		return &github.PRInfo{State: "merged", HeadRef: "feat/gutter"}, nil
	})

	listener.ReconcilePRPending(ctx, storage.Repo)

	got, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(domain.BacklogStatusDone), got.Status)
	logged, err := os.ReadFile(calls)
	require.NoError(t, err)
	for _, line := range strings.Split(strings.TrimSpace(string(logged)), "\n") {
		assert.Contains(t, line, "--repo tstapler/stapler-squad")
	}
}

// An unparseable PrURL keeps the old behaviour: no --repo, so gh and the finder
// use the workspace remote.
func TestReconcilePRPending_MalformedPRURL(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	calls := installFakeGH(t)

	item, repoPath := newCrossRepoItem(t, storage, "not-a-url")
	listener := NewBacklogLifecycleListener(storage)
	listener.ReconcilePRPending(ctx, storage.Repo)

	logged, err := os.ReadFile(calls)
	require.NoError(t, err)
	assert.Contains(t, string(logged), "pr view 1785")
	assert.NotContains(t, string(logged), "--repo")

	ref, err := instance.ResolvePRRepo(item.PrURL, repoPath)
	require.NoError(t, err)
	assert.Equal(t, "tstapler/stelekit", ref.String())
}

// Not parallel: overrides the process-wide GitHub base URL.
func TestVerifyPRHeadBranch_CrossRepo(t *testing.T) {
	var requested []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = append(requested, r.URL.Path)
		parts := strings.Split(r.URL.Path, "/") // .../repos/OWNER/REPO/pulls/N
		full := strings.Join(parts[len(parts)-4:len(parts)-2], "/")
		_, _ = fmt.Fprintf(w, `{"number":1785,"state":"closed","merged":true,"head":{"ref":"feat/gutter"},"base":{"repo":{"full_name":%q}}}`, full)
	}))
	defer srv.Close()
	t.Cleanup(github.SetGhBaseURLForTest(srv.URL))
	t.Setenv("GH_TOKEN", "test-token")

	storage, cleanup := createTestStorage(t)
	defer cleanup()
	_, repoPath := newCrossRepoItem(t, storage, crossRepoPRURL)
	listener := NewBacklogLifecycleListener(storage)
	own := prOwnership{branches: []string{"feat/gutter"}}

	matches, err := listener.verifyPRBelongsToItem(context.Background(), repoPath, crossRepoPRURL, own, 1785)
	require.NoError(t, err)
	assert.True(t, matches)
	_, err = listener.verifyPRBelongsToItem(context.Background(), repoPath, "not-a-url", own, 1785)
	require.NoError(t, err)

	require.Len(t, requested, 2)
	assert.Contains(t, requested[0], "repos/tstapler/stapler-squad/pulls/1785")
	assert.Contains(t, requested[1], "repos/tstapler/stelekit/pulls/1785")
}

// A commit on the workspace repo's main must not close a PR in another repo.
func TestReconcilePRPending_CrossRepoNotClosedAsSuperseded(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item, repoPath := newCrossRepoItem(t, storage, crossRepoPRURL)
	runGitTestCmd(t, repoPath, "checkout", "-q", "-b", "main")
	runGitTestCmd(t, repoPath, "-c", "user.email=t@example.com", "-c", "user.name=T", "commit", "-q", "--allow-empty", "-m", "on main")
	sha := strings.TrimSpace(runGitTestCmd(t, repoPath, "rev-parse", "HEAD"))
	newTrackedWorkSession(t, storage, item.ID, repoPath, "feat/gutter", sha)

	listener := NewBacklogLifecycleListener(storage)
	stubMatchingPRByNumberFinder(listener, "feat/gutter")
	checker := &fakePRPendingChecker{status: &git.PRStatus{CIFailing: true, FeedbackText: "build FAILED"}}
	overridePRPendingChecker(t, listener, checker)
	spawner := &fakePRFixSpawner{}
	listener.SetPRFixSpawner(spawner)

	listener.ReconcilePRPending(ctx, storage.Repo)

	assert.False(t, checker.closeCalled)
	assert.True(t, spawner.spawnCalled)
}

func TestFindPRPendingItemForEvent_CrossRepo(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	item, _ := newCrossRepoItem(t, storage, crossRepoPRURL)

	got, found := findPRPendingItemForEvent(ctx, storage.Repo, "tstapler/stapler-squad", 1785)
	require.True(t, found)
	assert.Equal(t, item.ID, got.ID.String())

	_, found = findPRPendingItemForEvent(ctx, storage.Repo, "tstapler/stelekit", 1785)
	assert.False(t, found, "the workspace repo's PR #1785 is a different PR")
}

const failedActionsRun = `{"statusCheckRollup":[{"__typename":"CheckRun","name":"t","status":"COMPLETED","conclusion":"FAILURE","detailsUrl":"https://github.com/tstapler/stapler-squad/actions/runs/1/job/2"}],"mergeable":"MERGEABLE","state":"OPEN"}`

// An unverified PR from another repo notifies instead of feeding a fix session.
func TestReconcilePRPending_CrossRepoUnverifiedStartsNoFixSession(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	newCrossRepoItem(t, storage, crossRepoPRURL)

	listener := NewBacklogLifecycleListener(storage)
	overridePRPendingChecker(t, listener, &fakePRPendingChecker{status: &git.PRStatus{CIFailing: true, FeedbackText: "do evil"}})
	spawner := &fakePRFixSpawner{}
	listener.SetPRFixSpawner(spawner)
	notifier := &sessiontest.FakeNotifier{}
	listener.SetNotifier(notifier)

	listener.ReconcilePRPending(context.Background(), storage.Repo)
	listener.ReconcilePRPending(context.Background(), storage.Repo)

	assert.False(t, spawner.spawnCalled)
	assert.Equal(t, []string{"Cross-repo PR needs you"}, notifier.Titles())
}

// The flaky-rerun client reads the workspace remote, so it must not run here.
func TestReconcilePRPending_CrossRepoSkipsFlakyRerun(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	item, repoPath := newCrossRepoItem(t, storage, crossRepoPRURL)
	newTrackedWorkSession(t, storage, item.ID, repoPath, "feat/gutter", "")

	listener := NewBacklogLifecycleListener(storage)
	stubMatchingPRByNumberFinder(listener, "feat/gutter")
	overridePRPendingChecker(t, listener, &fakePRPendingChecker{status: parsePRStatusPayloadForTest(t, []byte(failedActionsRun))})
	rerunner := &fakeCIRerun{files: []string{"Foo.kt"}, logs: map[int64]string{1: flakyLog}}
	listener.ciRerunner = rerunner
	spawner := &fakePRFixSpawner{}
	listener.SetPRFixSpawner(spawner)

	listener.ReconcilePRPending(context.Background(), storage.Repo)

	assert.Empty(t, rerunner.reruns)
	assert.True(t, spawner.spawnCalled)
}

// A tracked cross-repo PR is neither replaced, auto-merged nor dropped.
func TestPushAndCreatePR_KeepsCrossRepoPR(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	item, repoPath := newCrossRepoItem(t, storage, crossRepoPRURL)
	newTrackedWorkSession(t, storage, item.ID, repoPath, "feat/gutter", "")
	review := string(domain.BacklogStatusReview)
	require.NoError(t, storage.Repo.Client.BacklogItem.UpdateOneID(uuid.MustParse(item.ID)).SetStatus(review).Exec(ctx))
	item.Status = review
	sessions, err := storage.ListItemSessions(ctx, item.ID)
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	creator := &fakePRCreator{noCommitsAheadOfMain: true, autoMergeErr: errors.New("must not be called")}
	listener.SetPRCreatorFactory(func(_, _, _, _, _ string) prCreator { return creator })
	notifier := &sessiontest.FakeNotifier{}
	listener.SetNotifier(notifier)

	listener.pushAndCreatePR(ctx, item, sessions[0])

	got, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(domain.BacklogStatusPRPending), got.Status)
	assert.Equal(t, crossRepoPRURL, got.PrURL)
	assert.False(t, creator.createCalled)
	assert.False(t, creator.copilotReviewCalled)
	assert.Equal(t, []string{"Auto-merge not enabled"}, notifier.Titles())
}

func TestCrossRepoPR_UnparseableOriginCountsAsCrossRepo(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	_, repoPath := newCrossRepoItem(t, storage, crossRepoPRURL)
	runGitTestCmd(t, repoPath, "remote", "set-url", "origin", "git@github-work:tstapler/stapler-squad.git")

	_, cross := instance.CrossRepoPR(crossRepoPRURL, repoPath)
	assert.True(t, cross)
	_, ok := instance.PRURLRepo("https://github.com/git@localhost:o/r/pull/1")
	assert.False(t, ok, "gh would read that owner as another host")
}
