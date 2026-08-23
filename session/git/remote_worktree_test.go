package git

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	gossh "golang.org/x/crypto/ssh"

	"github.com/tstapler/stapler-squad/session/tmux"
)

// newTestSSHClientConfig builds an ssh.ClientConfig trusting exactly hostKey,
// mirroring session/tmux/ssh_runner_test.go's newTestClientConfig -- the
// "fixed-key-shaped HostKeyCallback" pattern the plan names as acceptable
// test plumbing ahead of Phase 3's real KnownHostsStore.
func newTestSSHClientConfig(t *testing.T, hostKey gossh.PublicKey) gossh.ClientConfig {
	t.Helper()
	return gossh.ClientConfig{
		User:            "test",
		Auth:            []gossh.AuthMethod{testClientAuth(t)},
		HostKeyCallback: gossh.FixedHostKey(hostKey),
	}
}

// newTestSSHRunner builds a tmux.SSHRunner against srv, backed by a fresh,
// per-test SSHClientPool so tests never share pooled connections.
func newTestSSHRunner(t *testing.T, srv *testSSHServer) *tmux.SSHRunner {
	t.Helper()
	cfg := newTestSSHClientConfig(t, srv.HostKey)
	return tmux.NewSSHRunner(
		tmux.SSHTarget{Name: t.Name(), Addr: srv.Addr},
		cfg,
		tmux.WithSSHClientPool(tmux.NewSSHClientPool()),
	)
}

// initTestBareRepo creates a real local git repository at dir with an
// initial commit on branchName, standing in for the "existing bare repo at
// /srv/repos/foo.git" the plan's acceptance criteria describe -- the test
// SSH server executes real shell commands against the local filesystem, so a
// real local repo IS the "remote" repo these tests exercise CreateWorktree
// against. Not created bare: `git worktree add` works the same either way,
// and a non-bare repo is simpler to also verify (via go-git) from the test
// side.
func initTestBareRepo(t *testing.T, dir, branchName string) {
	t.Helper()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("failed to init test repo: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("failed to get worktree: %v", err)
	}
	readme := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readme, []byte("test repo\n"), 0644); err != nil {
		t.Fatalf("failed to write README: %v", err)
	}
	if _, err := wt.Add("README.md"); err != nil {
		t.Fatalf("failed to add README: %v", err)
	}
	sig := &object.Signature{Name: "Test", Email: "test@localhost", When: time.Now()}
	if _, err := wt.Commit("initial commit", &git.CommitOptions{Author: sig}); err != nil {
		t.Fatalf("failed to commit: %v", err)
	}
	if branchName != "master" && branchName != "main" {
		headRef, err := repo.Head()
		if err != nil {
			t.Fatalf("failed to resolve HEAD: %v", err)
		}
		branchRef := plumbing.NewHashReference(plumbing.NewBranchReferenceName(branchName), headRef.Hash())
		if err := repo.Storer.SetReference(branchRef); err != nil {
			t.Fatalf("failed to create branch %s: %v", branchName, err)
		}
	}
}

// --- CreateWorktree: real SSH round-trip ---

// TestRemoteWorktreeOps_CreateWorktree_Success verifies CreateWorktree runs
// `git worktree add` over a real in-process SSH server and produces a real
// worktree directory on the "remote" host, matching the plan's Given/When/Then:
// an existing repo, a configured base_path, CreateWorktree(ctx, ...) called,
// then a follow-up `test -d <path>` on the remote succeeding.
func TestRemoteWorktreeOps_CreateWorktree_Success(t *testing.T) {
	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "repo")
	basePath := filepath.Join(tmpDir, "worktrees")
	if err := os.MkdirAll(basePath, 0755); err != nil {
		t.Fatalf("failed to create base path: %v", err)
	}
	initTestBareRepo(t, repoPath, "feature-x")

	srv := startTestSSHServer(t)
	runner := newTestSSHRunner(t, srv)
	ops := NewRemoteWorktreeOps(runner)

	worktreePath := filepath.Join(basePath, "feature-x")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := ops.CreateWorktree(ctx, RemoteWorktree{
		RepoPath:     repoPath,
		WorktreePath: worktreePath,
		Branch:       "feature-x",
	}); err != nil {
		t.Fatalf("CreateWorktree() error = %v", err)
	}

	// Given/When/Then's own verification step: a follow-up `test -d <path>` on
	// the remote succeeds.
	if _, err := runner.Run(ctx, "", "test", "-d", worktreePath); err != nil {
		t.Errorf("worktree directory does not exist on remote after CreateWorktree: %v", err)
	}

	// And it's a real, usable worktree: README.md from the base commit is present.
	if _, err := os.Stat(filepath.Join(worktreePath, "README.md")); err != nil {
		t.Errorf("expected README.md in new worktree: %v", err)
	}
}

// TestRemoteWorktreeOps_CreateWorktree_BasePathMissing_RealSSH verifies the
// base_path check over a real SSH round-trip (paired with the fake-runner
// variant below, which additionally proves no git command ever runs).
func TestRemoteWorktreeOps_CreateWorktree_BasePathMissing_RealSSH(t *testing.T) {
	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "repo")
	initTestBareRepo(t, repoPath, "feature-x")

	srv := startTestSSHServer(t)
	runner := newTestSSHRunner(t, srv)
	ops := NewRemoteWorktreeOps(runner)

	// basePath ("worktrees") is deliberately never created.
	worktreePath := filepath.Join(tmpDir, "worktrees", "feature-x")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := ops.CreateWorktree(ctx, RemoteWorktree{
		RepoPath:     repoPath,
		WorktreePath: worktreePath,
		Branch:       "feature-x",
	})
	if !errors.Is(err, ErrRemoteBasePathMissing) {
		t.Fatalf("CreateWorktree() error = %v, want errors.Is(err, ErrRemoteBasePathMissing)", err)
	}
}

// --- CreateWorktree: fake CommandRunner for precise error-path assertions ---

// recordingRunner is a hand-rolled fake tmux.CommandRunner: it records every
// Run() call's (dir, name, args) and returns a scripted result per command
// name, letting a test assert exactly which remote commands ran (and in what
// order) without a real SSH round-trip -- used for the "no git command runs
// before the base_path check fails" and exact-argument-construction
// assertions, per the task's own suggestion to use "a hand-rolled fake
// CommandRunner for the error-path tests where a real SSH round-trip isn't
// needed."
type recordingRunner struct {
	calls   []recordedCall
	results map[string]recordingResult // keyed by command name (args[0] equivalent, i.e. "name")
}

type recordedCall struct {
	dir  string
	name string
	args []string
}

type recordingResult struct {
	out []byte
	err error
}

func newRecordingRunner() *recordingRunner {
	return &recordingRunner{results: make(map[string]recordingResult)}
}

func (f *recordingRunner) Run(_ context.Context, dir, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, recordedCall{dir: dir, name: name, args: append([]string(nil), args...)})
	res := f.results[name]
	return res.out, res.err
}

func (f *recordingRunner) Start(context.Context, string, string, ...string) (io.WriteCloser, io.ReadCloser, func() error, error) {
	panic("recordingRunner.Start: not implemented, not exercised by these tests")
}

func (f *recordingRunner) IsRemote() bool { return true }

var _ tmux.CommandRunner = (*recordingRunner)(nil)

// TestRemoteWorktreeOps_CreateWorktree_BasePathMissing_NoGitCommandRuns proves
// the base_path check happens strictly before any git command, per the
// acceptance criteria's "verified via runner.Run(ctx, "test", "-d", basePath)
// failing before any git command runs" -- a real SSH round-trip can't easily
// prove a *negative* (git never ran), but a call-recording fake can.
func TestRemoteWorktreeOps_CreateWorktree_BasePathMissing_NoGitCommandRuns(t *testing.T) {
	runner := newRecordingRunner()
	runner.results["test"] = recordingResult{out: []byte("no such file or directory"), err: errors.New("exit status 1")}
	ops := NewRemoteWorktreeOps(runner)

	err := ops.CreateWorktree(context.Background(), RemoteWorktree{
		RepoPath:     "/srv/repos/foo.git",
		WorktreePath: "/srv/workspaces/feature-x",
		Branch:       "feature-x",
	})
	if !errors.Is(err, ErrRemoteBasePathMissing) {
		t.Fatalf("CreateWorktree() error = %v, want errors.Is(err, ErrRemoteBasePathMissing)", err)
	}

	if len(runner.calls) != 1 {
		t.Fatalf("Run() called %d times, want exactly 1 (the base_path check) — no git command should run", len(runner.calls))
	}
	got := runner.calls[0]
	if got.name != "test" || len(got.args) != 2 || got.args[0] != "-d" || got.args[1] != "/srv/workspaces" {
		t.Errorf("first Run() call = %+v, want test -d /srv/workspaces", got)
	}
}

// TestRemoteWorktreeOps_CreateWorktree_ArgumentConstruction pins the exact
// `git worktree add <path> <branch>` shape CreateWorktree builds, mirroring
// GitWorktree.setupFromExistingBranch's runGitCommand(repoPath, "worktree",
// "add", worktreePath, branchName) call (worktree_ops.go) -- dir=RepoPath,
// no `-b` flag, path before branch.
func TestRemoteWorktreeOps_CreateWorktree_ArgumentConstruction(t *testing.T) {
	runner := newRecordingRunner()
	runner.results["test"] = recordingResult{out: nil, err: nil} // base_path exists
	ops := NewRemoteWorktreeOps(runner)

	if err := ops.CreateWorktree(context.Background(), RemoteWorktree{
		RepoPath:     "/srv/repos/foo.git",
		WorktreePath: "/srv/workspaces/feature-x",
		Branch:       "feature-x",
	}); err != nil {
		t.Fatalf("CreateWorktree() error = %v", err)
	}

	if len(runner.calls) != 2 {
		t.Fatalf("Run() called %d times, want exactly 2 (base_path check, then git worktree add)", len(runner.calls))
	}
	gitCall := runner.calls[1]
	wantArgs := []string{"worktree", "add", "/srv/workspaces/feature-x", "feature-x"}
	if gitCall.dir != "/srv/repos/foo.git" || gitCall.name != "git" || !equalStrings(gitCall.args, wantArgs) {
		t.Errorf("git worktree add call = dir:%q name:%q args:%v, want dir:/srv/repos/foo.git name:git args:%v",
			gitCall.dir, gitCall.name, gitCall.args, wantArgs)
	}
}

// TestRemoteWorktreeOps_CreateWorktree_GitCommandFails verifies a git-level
// failure (base_path exists, but `git worktree add` itself fails — e.g. the
// TOCTOU race ErrRemoteBasePathMissing's doc comment describes, or simply a
// bad branch name) surfaces as a plain wrapped error, NOT
// ErrRemoteBasePathMissing.
func TestRemoteWorktreeOps_CreateWorktree_GitCommandFails(t *testing.T) {
	runner := newRecordingRunner()
	runner.results["test"] = recordingResult{out: nil, err: nil}
	runner.results["git"] = recordingResult{out: []byte("fatal: invalid reference: nope"), err: errors.New("exit status 128")}
	ops := NewRemoteWorktreeOps(runner)

	err := ops.CreateWorktree(context.Background(), RemoteWorktree{
		RepoPath:     "/srv/repos/foo.git",
		WorktreePath: "/srv/workspaces/feature-x",
		Branch:       "nope",
	})
	if err == nil {
		t.Fatal("CreateWorktree() error = nil, want non-nil")
	}
	if errors.Is(err, ErrRemoteBasePathMissing) {
		t.Errorf("CreateWorktree() error wrongly classified as ErrRemoteBasePathMissing: %v", err)
	}
	if !strings.Contains(err.Error(), "invalid reference") {
		t.Errorf("CreateWorktree() error = %v, want it to include the underlying git failure output", err)
	}
}

// --- RemoveWorktree ---

// TestRemoteWorktreeOps_RemoveWorktree_Success verifies RemoveWorktree over a
// real SSH round-trip: a worktree created by CreateWorktree is gone afterward.
func TestRemoteWorktreeOps_RemoveWorktree_Success(t *testing.T) {
	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "repo")
	basePath := filepath.Join(tmpDir, "worktrees")
	if err := os.MkdirAll(basePath, 0755); err != nil {
		t.Fatalf("failed to create base path: %v", err)
	}
	initTestBareRepo(t, repoPath, "feature-x")

	srv := startTestSSHServer(t)
	runner := newTestSSHRunner(t, srv)
	ops := NewRemoteWorktreeOps(runner)

	worktreePath := filepath.Join(basePath, "feature-x")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	w := RemoteWorktree{RepoPath: repoPath, WorktreePath: worktreePath, Branch: "feature-x"}
	if err := ops.CreateWorktree(ctx, w); err != nil {
		t.Fatalf("CreateWorktree() error = %v", err)
	}
	if err := ops.RemoveWorktree(ctx, w); err != nil {
		t.Fatalf("RemoveWorktree() error = %v", err)
	}

	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Errorf("worktree directory still exists after RemoveWorktree: stat err = %v", err)
	}

	// git itself no longer considers it a registered worktree.
	out, err := runner.Run(ctx, repoPath, "git", "worktree", "list", "--porcelain")
	if err != nil {
		t.Fatalf("git worktree list failed: %v", err)
	}
	if strings.Contains(string(out), worktreePath) {
		t.Errorf("git worktree list still references removed worktree %s:\n%s", worktreePath, out)
	}
}

// TestRemoteWorktreeOps_RemoveWorktree_ArgumentConstruction pins the exact
// `git worktree remove <path> --force` shape, mirroring CreateWorktree's
// argument construction per the plan (Task 2.2.1e).
func TestRemoteWorktreeOps_RemoveWorktree_ArgumentConstruction(t *testing.T) {
	runner := newRecordingRunner()
	ops := NewRemoteWorktreeOps(runner)

	if err := ops.RemoveWorktree(context.Background(), RemoteWorktree{
		RepoPath:     "/srv/repos/foo.git",
		WorktreePath: "/srv/workspaces/feature-x",
	}); err != nil {
		t.Fatalf("RemoveWorktree() error = %v", err)
	}

	if len(runner.calls) != 1 {
		t.Fatalf("Run() called %d times, want exactly 1", len(runner.calls))
	}
	got := runner.calls[0]
	wantArgs := []string{"worktree", "remove", "/srv/workspaces/feature-x", "--force"}
	if got.dir != "/srv/repos/foo.git" || got.name != "git" || !equalStrings(got.args, wantArgs) {
		t.Errorf("git worktree remove call = dir:%q name:%q args:%v, want dir:/srv/repos/foo.git name:git args:%v",
			got.dir, got.name, got.args, wantArgs)
	}
}

// TestRemoteWorktreeOps_RemoveWorktree_PropagatesError verifies RemoveWorktree
// itself never swallows a failure — "best-effort" (per the plan/Task 2.2.1e)
// describes how a caller should treat the returned error (log, don't let it
// mask an original error), not behavior internal to this method.
func TestRemoteWorktreeOps_RemoveWorktree_PropagatesError(t *testing.T) {
	runner := newRecordingRunner()
	runner.results["git"] = recordingResult{out: []byte("fatal: not a working tree"), err: errors.New("exit status 128")}
	ops := NewRemoteWorktreeOps(runner)

	err := ops.RemoveWorktree(context.Background(), RemoteWorktree{
		RepoPath:     "/srv/repos/foo.git",
		WorktreePath: "/srv/workspaces/gone",
	})
	if err == nil {
		t.Fatal("RemoveWorktree() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "not a working tree") {
		t.Errorf("RemoveWorktree() error = %v, want it to include the underlying git failure output", err)
	}
}

// --- InitializeProjectDirectory ---

// TestRemoteWorktreeOps_InitializeProjectDirectory_CreatesRepoWithInitialCommit
// verifies the remote git-init flow over a real SSH round-trip, mirroring
// InitializeProjectDirectory's local behavior (util.go): a fresh directory
// becomes a git repo with a committed .gitignore.
func TestRemoteWorktreeOps_InitializeProjectDirectory_CreatesRepoWithInitialCommit(t *testing.T) {
	tmpDir := t.TempDir()
	projectPath := filepath.Join(tmpDir, "new project") // embedded space: exercises posixShellQuote

	srv := startTestSSHServer(t)
	runner := newTestSSHRunner(t, srv)
	ops := NewRemoteWorktreeOps(runner)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := ops.InitializeProjectDirectory(ctx, projectPath); err != nil {
		t.Fatalf("InitializeProjectDirectory() error = %v", err)
	}

	repo, err := git.PlainOpen(projectPath)
	if err != nil {
		t.Fatalf("expected a git repo at %s: %v", projectPath, err)
	}
	head, err := repo.Head()
	if err != nil {
		t.Fatalf("expected a resolvable HEAD: %v", err)
	}
	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		t.Fatalf("failed to resolve HEAD commit: %v", err)
	}
	if strings.TrimSpace(commit.Message) != "Initial commit" {
		t.Errorf("HEAD commit message = %q, want %q", commit.Message, "Initial commit")
	}

	gitignore, err := os.ReadFile(filepath.Join(projectPath, ".gitignore"))
	if err != nil {
		t.Fatalf("expected a committed .gitignore: %v", err)
	}
	if string(gitignore) != "# Project gitignore\n" {
		t.Errorf(".gitignore content = %q, want %q", gitignore, "# Project gitignore\n")
	}
}

// TestRemoteWorktreeOps_InitializeProjectDirectory_NoOpIfAlreadyRepo verifies
// InitializeProjectDirectory does not touch an already-initialized repo —
// mirroring the local InitializeProjectDirectory's "already a git repo (open
// succeeds) -> no-op" first step.
func TestRemoteWorktreeOps_InitializeProjectDirectory_NoOpIfAlreadyRepo(t *testing.T) {
	tmpDir := t.TempDir()
	projectPath := filepath.Join(tmpDir, "existing")
	if err := os.MkdirAll(projectPath, 0755); err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}

	repo, err := git.PlainInit(projectPath, false)
	if err != nil {
		t.Fatalf("failed to pre-init repo: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("failed to get worktree: %v", err)
	}
	customFile := filepath.Join(projectPath, "existing.txt")
	if err := os.WriteFile(customFile, []byte("pre-existing content\n"), 0644); err != nil {
		t.Fatalf("failed to write existing.txt: %v", err)
	}
	if _, err := wt.Add("existing.txt"); err != nil {
		t.Fatalf("failed to add existing.txt: %v", err)
	}
	sig := &object.Signature{Name: "Test", Email: "test@localhost", When: time.Now()}
	if _, err := wt.Commit("pre-existing commit", &git.CommitOptions{Author: sig}); err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	srv := startTestSSHServer(t)
	runner := newTestSSHRunner(t, srv)
	ops := NewRemoteWorktreeOps(runner)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := ops.InitializeProjectDirectory(ctx, projectPath); err != nil {
		t.Fatalf("InitializeProjectDirectory() error = %v", err)
	}

	// The pre-existing commit is still HEAD -- no new "Initial commit" was made,
	// and no .gitignore was written over whatever was already there.
	repo2, err := git.PlainOpen(projectPath)
	if err != nil {
		t.Fatalf("failed to reopen repo: %v", err)
	}
	head, err := repo2.Head()
	if err != nil {
		t.Fatalf("failed to resolve HEAD: %v", err)
	}
	commit, err := repo2.CommitObject(head.Hash())
	if err != nil {
		t.Fatalf("failed to resolve HEAD commit: %v", err)
	}
	if strings.TrimSpace(commit.Message) != "pre-existing commit" {
		t.Errorf("HEAD commit message = %q, want %q (InitializeProjectDirectory should have been a no-op)", commit.Message, "pre-existing commit")
	}
	if _, err := os.Stat(filepath.Join(projectPath, ".gitignore")); !os.IsNotExist(err) {
		t.Errorf("expected no .gitignore to have been written, stat err = %v", err)
	}
}

// TestRemoteWorktreeOps_InitializeProjectDirectory_NoOpDoesNotAffectUnrelatedAncestorRepo
// guards against the tree-walking pitfall this method's doc comment calls
// out: if projectPath is nested inside some unrelated ancestor git repo, `git
// rev-parse --is-inside-work-tree` would wrongly report "already a repo" and
// skip initialization. This test nests projectPath inside tmpDir, which IS
// itself a git repo, and verifies InitializeProjectDirectory still creates a
// real nested repo rather than silently no-op'ing against the ancestor.
func TestRemoteWorktreeOps_InitializeProjectDirectory_DoesNotSkipDueToAncestorRepo(t *testing.T) {
	tmpDir := t.TempDir()
	initTestBareRepo(t, tmpDir, "main")

	projectPath := filepath.Join(tmpDir, "nested-project")

	srv := startTestSSHServer(t)
	runner := newTestSSHRunner(t, srv)
	ops := NewRemoteWorktreeOps(runner)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := ops.InitializeProjectDirectory(ctx, projectPath); err != nil {
		t.Fatalf("InitializeProjectDirectory() error = %v", err)
	}

	repo, err := git.PlainOpen(projectPath)
	if err != nil {
		t.Fatalf("expected a distinct nested git repo at %s: %v", projectPath, err)
	}
	head, err := repo.Head()
	if err != nil {
		t.Fatalf("expected a resolvable HEAD in the nested repo: %v", err)
	}
	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		t.Fatalf("failed to resolve HEAD commit: %v", err)
	}
	if strings.TrimSpace(commit.Message) != "Initial commit" {
		t.Errorf("nested repo HEAD commit message = %q, want %q", commit.Message, "Initial commit")
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
