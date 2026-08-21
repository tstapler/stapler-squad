package services

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/headless"
)

// firstCallJSONPR returns a valid first-call JSON response for DraftPullRequest tests,
// matching the schema headless_service_test.go's firstCallJSONHS uses. Marshaled via
// encoding/json (rather than string concatenation) so a multi-line result string is
// escaped correctly.
func firstCallJSONPR(t *testing.T, sessionID, result string) string {
	t.Helper()
	payload, err := json.Marshal(struct {
		SessionID string  `json:"session_id"`
		Result    string  `json:"result"`
		CostUSD   float64 `json:"cost_usd"`
	}{SessionID: sessionID, Result: result, CostUSD: 0.001})
	require.NoError(t, err)
	return string(payload)
}

// newDraftPRTestRepo creates a temp git repo with "main" as the initial branch and one
// commit, so GoGitVCSReader.ResolveDefaultBranch resolves it deterministically via the
// "refs/heads/main" fallback (there is no origin remote in these tests).
func newDraftPRTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-q", "-b", "main")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test repo\n"), 0o644))
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-q", "-m", "initial commit")
	return dir
}

// newDraftPRTestRepoWithOrigin creates a temp git repo like newDraftPRTestRepo, but
// additionally wires up a local (no-network) "origin" remote with
// refs/remotes/origin/HEAD pointed at "main" — the BLOCKER regression fixture.
// newDraftPRTestRepo deliberately has NO origin remote, which is exactly why
// GoGitVCSReader.ResolveDefaultBranch's remote-qualified "origin/main" return value
// (taken only when refs/remotes/origin/HEAD resolves) was never exercised by any
// existing test.
func newDraftPRTestRepoWithOrigin(t *testing.T) string {
	t.Helper()
	originDir := t.TempDir()
	runGit(t, originDir, "init", "-q", "--bare", "-b", "main")

	dir := newDraftPRTestRepo(t)
	runGit(t, dir, "remote", "add", "origin", originDir)
	runGit(t, dir, "push", "-q", "-u", "origin", "main")
	// "git remote set-head origin main" (non -a) writes refs/remotes/origin/HEAD
	// purely from the local arg given — no network round-trip to the remote.
	runGit(t, dir, "remote", "set-head", "origin", "main")
	return dir
}

// runGitOutput runs a git command in dir and returns its combined output, failing the
// test on error. Used where the test needs to inspect output (runGit, defined in
// path_completion_service_test.go, discards it).
func runGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := safeexec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %v failed: %s", args, out)
	return string(out)
}

// findInstanceFunc returns a PRCreationService findInstance closure serving a single
// instance under the given session ID, nil for anything else.
func findInstanceFunc(sessionID string, inst *session.Instance) func(string) *session.Instance {
	return func(id string) *session.Instance {
		if id == sessionID {
			return inst
		}
		return nil
	}
}

func connectErrCode(t *testing.T, err error) connect.Code {
	t.Helper()
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	return connectErr.Code()
}

// --------------------------------------------------------------------------
// DraftPullRequest
// --------------------------------------------------------------------------

func TestDraftPullRequest_should_ReturnNotFound_When_SessionInstanceMissing(t *testing.T) {
	t.Parallel()
	svc := NewPRCreationService(nil, nil, nil, nil, findInstanceFunc("sess-known", nil))

	_, err := svc.DraftPullRequest(context.Background(), connect.NewRequest(&sessionv1.DraftPullRequestRequest{
		SessionId: "sess-unknown",
	}))

	require.Error(t, err)
	assert.Equal(t, connect.CodeNotFound, connectErrCode(t, err))
}

func TestDraftPullRequest_should_PrefillTitleBodyBaseBranch_When_SessionHasCommitsAhead(t *testing.T) {
	t.Parallel()
	dir := newDraftPRTestRepo(t)
	runGit(t, dir, "checkout", "-q", "-b", "feature/rate-limit-toggle")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.go"), []byte("package main\n"), 0o644))
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-q", "-m", "Add rate limit toggle")

	wt := git.NewGitWorktreeFromStorage(dir, dir, "sess-7f3a", "feature/rate-limit-toggle", "")
	inst := &session.Instance{Title: "Add rate limit toggle"}
	inst.SetGitWorktree(wt)

	const draftedBody = "## Summary\n\nAdded a rate limit toggle."
	runner := headless.NewFakeRunner(firstCallJSONPR(t, "s1", draftedBody))
	pool := headless.NewPoolWithRunner(headless.PoolConfig{}, runner)

	svc := NewPRCreationService(nil, nil, pool, nil, findInstanceFunc("sess-7f3a", inst))

	resp, err := svc.DraftPullRequest(context.Background(), connect.NewRequest(&sessionv1.DraftPullRequestRequest{
		SessionId: "sess-7f3a",
	}))
	require.NoError(t, err)

	assert.Equal(t, "Add rate limit toggle", resp.Msg.Title)
	assert.Equal(t, draftedBody, resp.Msg.Body)
	assert.Equal(t, "main", resp.Msg.BaseBranch)
	assert.True(t, resp.Msg.HasCommitsAhead)
	assert.Empty(t, resp.Msg.ExistingPrUrl)
	assert.Zero(t, resp.Msg.ExistingPrNumber)
}

// TestDraftPullRequest_should_StripRemotePrefix_When_OriginHEADResolvesToRemoteQualifiedRef
// is the regression guard for the BLOCKER bug: for a normally-cloned repo (has an
// origin remote — the overwhelming common case), GoGitVCSReader.ResolveDefaultBranch
// takes the refs/remotes/origin/HEAD path and returns the remote-qualified short ref
// name "origin/main", not the bare branch name "main". `gh pr create --base` rejects
// a remote-qualified value ("Base ref must be a branch"), so DraftPullRequest must
// strip the leading "origin/" before it becomes BaseBranch — verified here against a
// real (local, no-network) origin remote rather than the no-origin fixture every other
// test in this file uses.
func TestDraftPullRequest_should_StripRemotePrefix_When_OriginHEADResolvesToRemoteQualifiedRef(t *testing.T) {
	t.Parallel()
	dir := newDraftPRTestRepoWithOrigin(t)
	runGit(t, dir, "checkout", "-q", "-b", "feature/strip-remote-prefix")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.go"), []byte("package main\n"), 0o644))
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-q", "-m", "Add a change")

	wt := git.NewGitWorktreeFromStorage(dir, dir, "sess-strip-prefix", "feature/strip-remote-prefix", "")
	inst := &session.Instance{Title: "Strip remote prefix session"}
	inst.SetGitWorktree(wt)

	svc := NewPRCreationService(nil, nil, nil, nil, findInstanceFunc("sess-strip-prefix", inst))

	resp, err := svc.DraftPullRequest(context.Background(), connect.NewRequest(&sessionv1.DraftPullRequestRequest{
		SessionId: "sess-strip-prefix",
	}))
	require.NoError(t, err)

	assert.Equal(t, "main", resp.Msg.BaseBranch,
		"BaseBranch must be the bare branch name, not the remote-qualified \"origin/main\" gh pr create --base rejects")
}

func TestDraftPullRequest_should_ReturnExistingPR_When_SessionAlreadyHasOne(t *testing.T) {
	t.Parallel()
	dir := newDraftPRTestRepo(t)
	runGit(t, dir, "checkout", "-q", "-b", "feature/already-has-pr")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.go"), []byte("package main\n"), 0o644))
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-q", "-m", "some work")

	wt := git.NewGitWorktreeFromStorage(dir, dir, "sess-existing-pr", "feature/already-has-pr", "")
	inst := &session.Instance{Title: "Already has a PR"}
	inst.SetGitWorktree(wt)
	inst.GitHubPRURL = "https://github.com/tstapler/stapler-squad/pull/512"
	inst.GitHubPRNumber = 512

	// If DraftPullRequest incorrectly did diff/draft work before short-circuiting, this
	// FakeRunner would record a call.
	runner := headless.NewFakeRunner(firstCallJSONPR(t, "s1", "should never be used"))
	pool := headless.NewPoolWithRunner(headless.PoolConfig{}, runner)

	svc := NewPRCreationService(nil, nil, pool, nil, findInstanceFunc("sess-existing-pr", inst))

	resp, err := svc.DraftPullRequest(context.Background(), connect.NewRequest(&sessionv1.DraftPullRequestRequest{
		SessionId: "sess-existing-pr",
	}))
	require.NoError(t, err)

	assert.Equal(t, "https://github.com/tstapler/stapler-squad/pull/512", resp.Msg.ExistingPrUrl)
	assert.EqualValues(t, 512, resp.Msg.ExistingPrNumber)
	assert.Empty(t, runner.Calls, "existing-PR short-circuit must skip diff/draft work entirely")
}

func TestDraftPullRequest_should_ReportHasCommitsAhead_False_When_BranchIsUpToDate(t *testing.T) {
	t.Parallel()
	dir := newDraftPRTestRepo(t)
	runGit(t, dir, "checkout", "-q", "-b", "feature/up-to-date")

	wt := git.NewGitWorktreeFromStorage(dir, dir, "sess-9c21", "feature/up-to-date", "")
	inst := &session.Instance{Title: "Up to date session"}
	inst.SetGitWorktree(wt)

	svc := NewPRCreationService(nil, nil, nil, nil, findInstanceFunc("sess-9c21", inst))

	resp, err := svc.DraftPullRequest(context.Background(), connect.NewRequest(&sessionv1.DraftPullRequestRequest{
		SessionId: "sess-9c21",
	}))
	require.NoError(t, err)

	assert.False(t, resp.Msg.HasCommitsAhead)
	assert.Equal(t, "main", resp.Msg.BaseBranch)
}

// TestDraftPullRequest_should_PreviewWorkingTreeDiff_When_UncommittedChangesPresent is the
// regression guard for Post-Review Revision #2: DraftPullRequest must preview the
// working-tree-inclusive diff (picking up uncommitted changes), not a committed-only diff,
// and it must never commit anything itself.
func TestDraftPullRequest_should_PreviewWorkingTreeDiff_When_UncommittedChangesPresent(t *testing.T) {
	t.Parallel()
	dir := newDraftPRTestRepo(t)
	runGit(t, dir, "checkout", "-q", "-b", "feature/uncommitted-work")
	// Zero commits ahead of main — the only change is an uncommitted, untracked file.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "uncommitted.txt"), []byte("hello from an uncommitted file"), 0o644))

	beforeCount := runGitOutput(t, dir, "rev-list", "--count", "HEAD")

	wt := git.NewGitWorktreeFromStorage(dir, dir, "sess-uncommitted", "feature/uncommitted-work", "")
	inst := &session.Instance{Title: "Uncommitted work session"}
	inst.SetGitWorktree(wt)

	const draftedBody = "## Summary\n\nDrafted from the working-tree diff."
	runner := headless.NewFakeRunner(firstCallJSONPR(t, "s1", draftedBody))
	pool := headless.NewPoolWithRunner(headless.PoolConfig{}, runner)

	svc := NewPRCreationService(nil, nil, pool, nil, findInstanceFunc("sess-uncommitted", inst))

	resp, err := svc.DraftPullRequest(context.Background(), connect.NewRequest(&sessionv1.DraftPullRequestRequest{
		SessionId: "sess-uncommitted",
	}))
	require.NoError(t, err)

	// The drafted body came from the headless call (not the fallback), which only
	// happens when the diff was non-empty — proving the uncommitted file was picked up.
	assert.Equal(t, draftedBody, resp.Msg.Body)
	require.Len(t, runner.Stdins, 1)
	assert.Contains(t, string(runner.Stdins[0]), "uncommitted.txt",
		"the diff sent to the LLM must include the uncommitted file")
	assert.False(t, resp.Msg.HasCommitsAhead, "zero commits ahead of main — only uncommitted changes")

	// Critical regression guard: DraftPullRequest must never commit anything.
	afterCount := runGitOutput(t, dir, "rev-list", "--count", "HEAD")
	assert.Equal(t, beforeCount, afterCount, "DraftPullRequest must make zero git-mutating calls (no CommitChanges)")
	status := runGitOutput(t, dir, "status", "--porcelain")
	assert.Contains(t, status, "uncommitted.txt", "the file must remain uncommitted after DraftPullRequest")
}

func TestDraftPullRequest_should_UseFallbackBody_When_HeadlessPoolNil(t *testing.T) {
	t.Parallel()
	dir := newDraftPRTestRepo(t)
	runGit(t, dir, "checkout", "-q", "-b", "feature/no-pool")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "change.txt"), []byte("data"), 0o644))
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-q", "-m", "a real commit")

	wt := git.NewGitWorktreeFromStorage(dir, dir, "sess-no-pool", "feature/no-pool", "")
	inst := &session.Instance{Title: "No headless pool session"}
	inst.SetGitWorktree(wt)

	// headlessPool is nil — must not panic, must use the fallback body.
	svc := NewPRCreationService(nil, nil, nil, nil, findInstanceFunc("sess-no-pool", inst))

	resp, err := svc.DraftPullRequest(context.Background(), connect.NewRequest(&sessionv1.DraftPullRequestRequest{
		SessionId: "sess-no-pool",
	}))
	require.NoError(t, err)
	assert.Equal(t, fallbackPRBody, resp.Msg.Body)
}

func TestDraftPullRequest_should_UseFallbackBody_When_DiffIsEmpty(t *testing.T) {
	t.Parallel()
	dir := newDraftPRTestRepo(t)
	runGit(t, dir, "checkout", "-q", "-b", "feature/empty-diff")
	// No commits, no uncommitted changes — diff is empty.

	wt := git.NewGitWorktreeFromStorage(dir, dir, "sess-empty-diff", "feature/empty-diff", "")
	inst := &session.Instance{Title: "Empty diff session"}
	inst.SetGitWorktree(wt)

	runner := headless.NewFakeRunner(firstCallJSONPR(t, "s1", "should never be used"))
	pool := headless.NewPoolWithRunner(headless.PoolConfig{}, runner)

	svc := NewPRCreationService(nil, nil, pool, nil, findInstanceFunc("sess-empty-diff", inst))

	resp, err := svc.DraftPullRequest(context.Background(), connect.NewRequest(&sessionv1.DraftPullRequestRequest{
		SessionId: "sess-empty-diff",
	}))
	require.NoError(t, err)
	assert.Equal(t, fallbackPRBody, resp.Msg.Body)
	assert.Empty(t, runner.Calls, "the headless pool must not be called when the diff is empty")
}

// TestDraftPullRequest_should_UseFallbackBody_When_DraftPRDescriptionErrors is the
// regression guard for a previously-untested branch: when headless.DraftPRDescription
// itself returns an error (as opposed to a nil pool or an empty diff, which the two
// tests above already cover), DraftPullRequest must log a warning and fall back to
// fallbackPRBody rather than surfacing the error as an RPC failure.
func TestDraftPullRequest_should_UseFallbackBody_When_DraftPRDescriptionErrors(t *testing.T) {
	t.Parallel()
	dir := newDraftPRTestRepo(t)
	runGit(t, dir, "checkout", "-q", "-b", "feature/draft-error")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "change.txt"), []byte("data"), 0o644))
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-q", "-m", "a real commit")

	wt := git.NewGitWorktreeFromStorage(dir, dir, "sess-draft-error", "feature/draft-error", "")
	inst := &session.Instance{Title: "Draft error session"}
	inst.SetGitWorktree(wt)

	runner := headless.NewFakeRunner()
	runner.SetErrors(errors.New("headless: pool exhausted"))
	pool := headless.NewPoolWithRunner(headless.PoolConfig{}, runner)

	svc := NewPRCreationService(nil, nil, pool, nil, findInstanceFunc("sess-draft-error", inst))

	resp, err := svc.DraftPullRequest(context.Background(), connect.NewRequest(&sessionv1.DraftPullRequestRequest{
		SessionId: "sess-draft-error",
	}))
	require.NoError(t, err, "a DraftPRDescription failure must fall back, not surface as an RPC error")
	assert.Equal(t, fallbackPRBody, resp.Msg.Body)
}

// --------------------------------------------------------------------------
// CreatePullRequest
// --------------------------------------------------------------------------

// fakePRGitExecutor is a fake executor.Executor for CreatePullRequest's
// commit -> push -> CreatePR pipeline. It intercepts every git/gh subprocess
// GitWorktree would otherwise spawn, dispatching by argv shape, so these
// tests exercise the real CommitChanges/PushBranch/CreatePR code paths
// without touching a real remote or a real `gh` binary — mirroring
// capturingGHExecutor's pattern in session/git/worktree_git_test.go, extended
// to cover the git-side commands CreatePullRequest's handler also drives.
//
// Since GitWorktree's cmdExec (once set) intercepts every command it would
// run, and StageAllExceptScaffolding's UntrackScaffolding step uses go-git
// directly against the (fake, nonexistent) worktree path and fails open on a
// missing repo, these tests need no real git repo on disk at all — a fake
// "/fake/repo"-shaped path is enough.
type fakePRGitExecutor struct {
	mu sync.Mutex

	calls []string // every command run, as a space-joined argv, for call-order/never-called assertions

	dirty     bool   // git status --porcelain reports a dirty worktree (drives CommitChanges' real commit path)
	commitErr error  // git commit fails with this error
	pushErr   error  // git push fails with this error
	createOut string // gh pr create's stdout (defaults to a canned PR URL if empty)
	createErr error  // gh pr create fails with this error

	// blockFirstCallStarted, when non-nil, is closed the first time
	// CombinedOutput is invoked (signaling a test-driving goroutine that the
	// first git/gh subprocess of the pipeline has started), after which that
	// same call blocks until blockFirstCallProceed is closed. This lets a test
	// pause the winning goroutine's pipeline mid-flight so a second, real,
	// concurrently-launched goroutine has an actual window to race
	// CreatePullRequest's LoadOrStore in-flight guard, rather than only proving
	// the guard's error path against a pre-populated map entry.
	blockFirstCallStarted chan struct{}
	blockFirstCallProceed chan struct{}
	firstCallBlockOnce    sync.Once
}

func (e *fakePRGitExecutor) Run(_ *exec.Cmd) error              { return nil }
func (e *fakePRGitExecutor) Output(_ *exec.Cmd) ([]byte, error) { return nil, nil }

func (e *fakePRGitExecutor) CombinedOutput(cmd *exec.Cmd) ([]byte, error) {
	e.mu.Lock()
	e.calls = append(e.calls, strings.Join(cmd.Args, " "))
	e.mu.Unlock()

	if e.blockFirstCallStarted != nil {
		e.firstCallBlockOnce.Do(func() {
			close(e.blockFirstCallStarted)
			<-e.blockFirstCallProceed
		})
	}

	args := cmd.Args
	if len(args) == 0 {
		return nil, nil
	}
	prog := filepath.Base(args[0])
	switch {
	case prog == "git" && len(args) > 3 && args[3] == "status":
		if e.dirty {
			return []byte("M file.txt\n"), nil
		}
		return nil, nil
	case prog == "git" && len(args) > 3 && args[3] == "add":
		return nil, nil
	case prog == "git" && len(args) > 3 && args[3] == "diff":
		if e.dirty {
			return []byte("file.txt\n"), nil
		}
		return nil, nil
	case prog == "git" && len(args) > 3 && args[3] == "commit":
		if e.commitErr != nil {
			return []byte("commit failed"), e.commitErr
		}
		return nil, nil
	case prog == "git" && len(args) > 1 && args[1] == "push":
		if e.pushErr != nil {
			return []byte("push failed"), e.pushErr
		}
		return nil, nil
	case prog == "gh" && len(args) > 2 && args[2] == "list":
		// findExistingPR's pre-check: report no existing PR so CreatePR
		// proceeds to `gh pr create`.
		return nil, exec.ErrNotFound
	case prog == "gh" && len(args) > 2 && args[2] == "create":
		if e.createErr != nil {
			return nil, e.createErr
		}
		out := e.createOut
		if out == "" {
			out = "https://github.com/tstapler/stapler-squad/pull/512\n"
		}
		return []byte(out), nil
	}
	return nil, nil
}

// calledWith reports whether any recorded call contains sub as a substring —
// used to prove a step (e.g. "gh pr create") was, or was never, reached.
func (e *fakePRGitExecutor) calledWith(sub string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, c := range e.calls {
		if strings.Contains(c, sub) {
			return true
		}
	}
	return false
}

// newFakePRWorktree builds a *git.GitWorktree backed by a fakePRGitExecutor and
// a fake (never touched on disk) repo/worktree path — CommitChanges/PushBranch/
// CreatePR never need a real repo since every command they'd run is intercepted.
func newFakePRWorktree(sessionID, branchName string, mock *fakePRGitExecutor) *git.GitWorktree {
	return git.NewGitWorktreeFromStorageWithExecutor("/fake/repo", "/fake/worktree", sessionID, branchName, "", mock)
}

// fakePRInstanceStore is a minimal session.InstanceStore fake. Unlike the real
// ent-backed Storage (whose SaveInstances silently no-ops for any instance
// where Started() is false, and never returns an error), this fake lets tests
// directly control and observe SaveInstances' outcome — needed to exercise
// both the persist-success path and Task 1.4.1d's explicit
// persisted=false/persistError partial-failure signaling.
type fakePRInstanceStore struct {
	mu             sync.Mutex
	saveErr        error
	savedInstances []*session.Instance
	saveCalls      int
}

func (f *fakePRInstanceStore) LoadInstances() ([]*session.Instance, error)            { return nil, nil }
func (f *fakePRInstanceStore) ListInstanceData() ([]session.InstanceData, error)      { return nil, nil }
func (f *fakePRInstanceStore) AddInstance(*session.Instance) error                    { return nil }
func (f *fakePRInstanceStore) DeleteInstance(string) error                            { return nil }
func (f *fakePRInstanceStore) UpdateInstanceLastUserResponse(string, time.Time) error { return nil }
func (f *fakePRInstanceStore) UpdateInstanceMetadata(string, *string, *string, *string, *string) error {
	return nil
}

func (f *fakePRInstanceStore) SaveInstances(instances []*session.Instance) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.saveCalls++
	if f.saveErr != nil {
		return f.saveErr
	}
	f.savedInstances = append(f.savedInstances, instances...)
	return nil
}

var _ session.InstanceStore = (*fakePRInstanceStore)(nil)

// withPathPrepended prepends dir to PATH for the duration of the test. Uses
// t.Setenv, so the test must not call t.Parallel().
func withPathPrepended(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// fakeGHNotAuthenticated installs a fake `gh` binary on PATH (ahead of the
// real one) that fails `gh auth status`, causing checkGHCLI() — called for
// real inside GitWorktree.CreatePR, not routed through the fake executor —
// to return its exact "not configured" message. This is the only piece of
// CreatePullRequest's pipeline that can't be faked via cmdExec, since
// checkGHCLI shells out directly rather than through GitWorktree's injectable
// executor. Must not run in a parallel test (uses t.Setenv).
func fakeGHNotAuthenticated(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "gh")
	content := "#!/bin/sh\nif [ \"$1\" = \"auth\" ] && [ \"$2\" = \"status\" ]; then\n  echo 'not logged in' >&2\n  exit 1\nfi\nexit 0\n"
	require.NoError(t, os.WriteFile(script, []byte(content), 0o755))
	withPathPrepended(t, dir)
}

func TestCreatePullRequest_should_CallCreatePRDirectly_NotHeadlessPool(t *testing.T) {
	t.Parallel()
	mock := &fakePRGitExecutor{}
	wt := newFakePRWorktree("sess-direct", "feature/direct", mock)
	inst := &session.Instance{Title: "Direct PR session", UUID: uuid.New().String()}
	inst.SetGitWorktree(wt)

	runner := headless.NewFakeRunner(firstCallJSONPR(t, "s1", "should never be used"))
	pool := headless.NewPoolWithRunner(headless.PoolConfig{}, runner)

	svc := NewPRCreationService(&fakePRInstanceStore{}, events.NewEventBus(1), pool, nil, findInstanceFunc("sess-direct", inst))

	resp, err := svc.CreatePullRequest(context.Background(), connect.NewRequest(&sessionv1.CreatePullRequestRequest{
		SessionId: "sess-direct",
		Title:     "Add direct PR flow",
	}))
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Msg.PrUrl)
	assert.Empty(t, runner.Calls, "CreatePullRequest must call wt.CreatePR directly, never the headless pool")
}

func TestCreatePullRequest_should_PersistAndPublishEvent_When_CreateSucceeds(t *testing.T) {
	t.Parallel()
	mock := &fakePRGitExecutor{createOut: "https://github.com/tstapler/stapler-squad/pull/512\n"}
	wt := newFakePRWorktree("sess-7f3a", "feature/rate-limit-toggle", mock)
	inst := &session.Instance{Title: "Add rate limit toggle", UUID: uuid.New().String()}
	inst.SetGitWorktree(wt)

	store := &fakePRInstanceStore{}
	bus := events.NewEventBus(1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	eventCh, subID := bus.Subscribe(ctx)
	defer bus.Unsubscribe(subID)

	svc := NewPRCreationService(store, bus, nil, nil, findInstanceFunc("sess-7f3a", inst))

	resp, err := svc.CreatePullRequest(context.Background(), connect.NewRequest(&sessionv1.CreatePullRequestRequest{
		SessionId:  "sess-7f3a",
		Title:      "Add per-user rate limiting",
		Body:       "...edited body...",
		BaseBranch: "release/1.2",
	}))
	require.NoError(t, err)

	assert.Equal(t, "https://github.com/tstapler/stapler-squad/pull/512", resp.Msg.PrUrl)
	assert.EqualValues(t, 512, resp.Msg.PrNumber)
	assert.False(t, resp.Msg.AlreadyExisted)
	assert.True(t, resp.Msg.Persisted)
	assert.Empty(t, resp.Msg.PersistError)

	require.Equal(t, 1, store.saveCalls)
	require.Len(t, store.savedInstances, 1)
	assert.Equal(t, "https://github.com/tstapler/stapler-squad/pull/512", store.savedInstances[0].GitHubPRURL)
	assert.Equal(t, 512, store.savedInstances[0].GitHubPRNumber)

	select {
	case ev := <-eventCh:
		assert.Equal(t, events.EventSessionUpdated, ev.Type)
		assert.ElementsMatch(t, []string{"github_pr_url", "github_pr_number"}, ev.UpdatedFields)
		require.NotNil(t, ev.Session)
		assert.Equal(t, inst.UUID, ev.Session.UUID)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SessionUpdatedEvent")
	}

	// AC2/AC3: the edited title/body/base-branch flowed through to gh pr create verbatim.
	require.True(t, mock.calledWith("--title Add per-user rate limiting"))
	require.True(t, mock.calledWith("--base release/1.2"))
}

func TestCreatePullRequest_should_ReturnPersistedFalse_When_SaveInstancesFails(t *testing.T) {
	t.Parallel()
	mock := &fakePRGitExecutor{}
	wt := newFakePRWorktree("sess-persist-fail", "feature/persist-fail", mock)
	inst := &session.Instance{Title: "Persist-fail session", UUID: uuid.New().String()}
	inst.SetGitWorktree(wt)

	store := &fakePRInstanceStore{saveErr: errors.New("db unavailable")}
	svc := NewPRCreationService(store, nil, nil, nil, findInstanceFunc("sess-persist-fail", inst))

	resp, err := svc.CreatePullRequest(context.Background(), connect.NewRequest(&sessionv1.CreatePullRequestRequest{
		SessionId: "sess-persist-fail",
		Title:     "Some title",
	}))
	require.NoError(t, err, "a persist failure must still be a connect SUCCESS response — the PR is real")
	assert.NotEmpty(t, resp.Msg.PrUrl)
	assert.False(t, resp.Msg.Persisted)
	assert.Equal(t, "db unavailable", resp.Msg.PersistError)
}

func TestCreatePullRequest_should_SurfaceSpecificError_When_GHNotAuthenticated(t *testing.T) {
	// Not parallel: fakeGHNotAuthenticated uses t.Setenv.
	fakeGHNotAuthenticated(t)

	mock := &fakePRGitExecutor{}
	wt := newFakePRWorktree("sess-no-auth", "feature/no-auth", mock)
	inst := &session.Instance{Title: "No auth session", UUID: uuid.New().String()}
	inst.SetGitWorktree(wt)

	svc := NewPRCreationService(&fakePRInstanceStore{}, nil, nil, nil, findInstanceFunc("sess-no-auth", inst))

	_, err := svc.CreatePullRequest(context.Background(), connect.NewRequest(&sessionv1.CreatePullRequestRequest{
		SessionId: "sess-no-auth",
		Title:     "Some title",
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeUnavailable, connectErrCode(t, err))

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, "GitHub CLI is not configured. Please run 'gh auth login' first", connectErr.Message(),
		"the literal checkGHCLI error must surface verbatim, not wrapped in a generic message")
}

func TestCreatePullRequest_should_SurfaceError_When_CommitFails(t *testing.T) {
	t.Parallel()
	mock := &fakePRGitExecutor{dirty: true, commitErr: errors.New("disk full")}
	wt := newFakePRWorktree("sess-commit-fail", "feature/commit-fail", mock)
	inst := &session.Instance{Title: "Commit-fail session", UUID: uuid.New().String()}
	inst.SetGitWorktree(wt)

	svc := NewPRCreationService(&fakePRInstanceStore{}, nil, nil, nil, findInstanceFunc("sess-commit-fail", inst))

	_, err := svc.CreatePullRequest(context.Background(), connect.NewRequest(&sessionv1.CreatePullRequestRequest{
		SessionId: "sess-commit-fail",
		Title:     "Some title",
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInternal, connectErrCode(t, err))

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Contains(t, connectErr.Message(), "disk full")

	assert.False(t, mock.calledWith("push"), "PushBranch must never be reached after a commit failure")
	assert.False(t, mock.calledWith("gh"), "CreatePR must never be reached after a commit failure")
}

func TestCreatePullRequest_should_SurfaceError_When_PushFails(t *testing.T) {
	t.Parallel()
	mock := &fakePRGitExecutor{pushErr: errors.New("non-fast-forward")}
	wt := newFakePRWorktree("sess-push-fail", "feature/push-fail", mock)
	inst := &session.Instance{Title: "Push-fail session", UUID: uuid.New().String()}
	inst.SetGitWorktree(wt)

	svc := NewPRCreationService(&fakePRInstanceStore{}, nil, nil, nil, findInstanceFunc("sess-push-fail", inst))

	_, err := svc.CreatePullRequest(context.Background(), connect.NewRequest(&sessionv1.CreatePullRequestRequest{
		SessionId: "sess-push-fail",
		Title:     "Some title",
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeUnavailable, connectErrCode(t, err))

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Contains(t, connectErr.Message(), "non-fast-forward")

	assert.False(t, mock.calledWith("gh"), "CreatePR must never be reached after a push failure")
}

func TestCreatePullRequest_should_SetAlreadyExisted_When_SessionHasCachedPRUrl(t *testing.T) {
	t.Parallel()
	mock := &fakePRGitExecutor{}
	wt := newFakePRWorktree("sess-cached-pr", "feature/cached-pr", mock)
	inst := &session.Instance{Title: "Cached PR session", UUID: uuid.New().String()}
	inst.SetGitWorktree(wt)
	inst.GitHubPRURL = "https://github.com/tstapler/stapler-squad/pull/999"
	inst.GitHubPRNumber = 999

	svc := NewPRCreationService(&fakePRInstanceStore{}, events.NewEventBus(1), nil, nil, findInstanceFunc("sess-cached-pr", inst))

	resp, err := svc.CreatePullRequest(context.Background(), connect.NewRequest(&sessionv1.CreatePullRequestRequest{
		SessionId: "sess-cached-pr",
		Title:     "Some title",
	}))
	require.NoError(t, err)
	assert.True(t, resp.Msg.AlreadyExisted)
	assert.Equal(t, "https://github.com/tstapler/stapler-squad/pull/999", resp.Msg.PrUrl)
	assert.EqualValues(t, 999, resp.Msg.PrNumber)

	assert.False(t, mock.calledWith("gh pr create"), "the fast path must avoid a second gh pr create call")

	// Task 1.4.1b: commit+push still happen unconditionally, even on the fast path.
	assert.True(t, mock.calledWith("push"), "commit+push must still run before the existing-PR fast path")
}

func TestCreatePullRequest_should_RejectConcurrentCall_When_AlreadyInFlight(t *testing.T) {
	t.Parallel()
	mock := &fakePRGitExecutor{}
	wt := newFakePRWorktree("sess-in-flight", "feature/in-flight", mock)
	inst := &session.Instance{Title: "In-flight session", UUID: uuid.New().String()}
	inst.SetGitWorktree(wt)

	svc := NewPRCreationService(&fakePRInstanceStore{}, nil, nil, nil, findInstanceFunc("sess-in-flight", inst))
	svc.prCreationInFlight.Store("sess-in-flight", true)
	defer svc.prCreationInFlight.Delete("sess-in-flight")

	_, err := svc.CreatePullRequest(context.Background(), connect.NewRequest(&sessionv1.CreatePullRequestRequest{
		SessionId: "sess-in-flight",
		Title:     "Some title",
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeAlreadyExists, connectErrCode(t, err))
	assert.False(t, mock.calledWith("push"), "a second concurrent call must be rejected before touching git at all")
}

// TestCreatePullRequest_should_RejectConcurrentCall_When_RacingRealGoroutines is the
// real-concurrency companion to TestCreatePullRequest_should_RejectConcurrentCall_When_AlreadyInFlight
// above. That test only proves the "already in flight" error path is taken when the
// in-flight map is pre-populated by the test itself — it never actually races two
// CreatePullRequest calls against each other, so it can't prove prCreationInFlight's
// LoadOrStore guard is atomic under real concurrency. Here, two goroutines call
// CreatePullRequest for the same session at (as close to) the same time; the mock
// executor blocks the first goroutine's first git subprocess call until the second
// goroutine has actually called in and been rejected, closing the race window a
// sequential test can't exercise.
func TestCreatePullRequest_should_RejectConcurrentCall_When_RacingRealGoroutines(t *testing.T) {
	t.Parallel()
	mock := &fakePRGitExecutor{
		blockFirstCallStarted: make(chan struct{}),
		blockFirstCallProceed: make(chan struct{}),
	}
	wt := newFakePRWorktree("sess-real-race", "feature/real-race", mock)
	inst := &session.Instance{Title: "Real race session", UUID: uuid.New().String()}
	inst.SetGitWorktree(wt)

	svc := NewPRCreationService(&fakePRInstanceStore{}, events.NewEventBus(1), nil, nil, findInstanceFunc("sess-real-race", inst))

	type result struct {
		resp *connect.Response[sessionv1.CreatePullRequestResponse]
		err  error
	}
	firstResultCh := make(chan result, 1)
	go func() {
		resp, err := svc.CreatePullRequest(context.Background(), connect.NewRequest(&sessionv1.CreatePullRequestRequest{
			SessionId: "sess-real-race",
			Title:     "Some title",
		}))
		firstResultCh <- result{resp, err}
	}()

	// Wait for the first goroutine to have passed the in-flight guard and
	// entered its git pipeline (blocked there), so the second call below is
	// guaranteed to observe a live in-flight entry rather than racing to be
	// first itself.
	select {
	case <-mock.blockFirstCallStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the first goroutine to enter its git pipeline")
	}

	_, secondErr := svc.CreatePullRequest(context.Background(), connect.NewRequest(&sessionv1.CreatePullRequestRequest{
		SessionId: "sess-real-race",
		Title:     "Some title",
	}))
	require.Error(t, secondErr, "a real concurrent second call must be rejected while the first is still in flight")
	assert.Equal(t, connect.CodeAlreadyExists, connectErrCode(t, secondErr))

	close(mock.blockFirstCallProceed)

	select {
	case firstResult := <-firstResultCh:
		require.NoError(t, firstResult.err, "the first (winning) goroutine must succeed")
		assert.NotEmpty(t, firstResult.resp.Msg.PrUrl)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the first goroutine to finish")
	}
}

func TestCreatePullRequest_should_ReturnInternalError_When_PRNumberIsZero(t *testing.T) {
	t.Parallel()
	mock := &fakePRGitExecutor{createOut: "https://github.com/tstapler/stapler-squad/pull/not-a-number\n"}
	wt := newFakePRWorktree("sess-zero-number", "feature/zero-number", mock)
	inst := &session.Instance{Title: "Zero PR number session", UUID: uuid.New().String()}
	inst.SetGitWorktree(wt)

	svc := NewPRCreationService(&fakePRInstanceStore{}, nil, nil, nil, findInstanceFunc("sess-zero-number", inst))

	_, err := svc.CreatePullRequest(context.Background(), connect.NewRequest(&sessionv1.CreatePullRequestRequest{
		SessionId: "sess-zero-number",
		Title:     "Some title",
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInternal, connectErrCode(t, err))
}

func TestCreatePullRequest_should_CallRecordPRCreatedOutOfBand_When_BacklogListenerPresent(t *testing.T) {
	t.Parallel()
	mock := &fakePRGitExecutor{}
	wt := newFakePRWorktree("sess-backlog-linked", "feature/backlog-linked", mock)
	inst := &session.Instance{Title: "Backlog-linked session", UUID: uuid.New().String()}
	inst.SetGitWorktree(wt)

	backlogStorage := createTestStorage(t)
	ctx := context.Background()
	item, err := backlogStorage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Backlog item behind this session",
		Status: string(session.BacklogStatusReview),
	})
	require.NoError(t, err)
	_, err = backlogStorage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: inst.UUID,
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	listener := session.NewBacklogLifecycleListener(backlogStorage)
	listener.SetEnabled(true)

	svc := NewPRCreationService(&fakePRInstanceStore{}, events.NewEventBus(1), nil, listener, findInstanceFunc("sess-backlog-linked", inst))

	resp, err := svc.CreatePullRequest(ctx, connect.NewRequest(&sessionv1.CreatePullRequestRequest{
		SessionId: "sess-backlog-linked",
		Title:     "Add backlog feature",
	}))
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Msg.PrUrl)

	updated, err := backlogStorage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusPRPending), updated.Status,
		"RecordPRCreatedOutOfBand must be called unconditionally and transition the linked item to pr_pending")
}
