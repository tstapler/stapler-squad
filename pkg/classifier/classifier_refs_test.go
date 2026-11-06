package classifier_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/pkg/classifier"
	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/gitprovider"
	_ "github.com/tstapler/stapler-squad/session/gitwiring" // registers the production backend with gitprovider
	"github.com/tstapler/stapler-squad/testutil/gitfake"
)

// The production adapter satisfies the classifier port.
var _ classifier.RefReader = gitprovider.RefReader{}

type fakeRefs struct {
	toplevel, gitDir       string
	toplevelErr, gitDirErr error
	gitDirCalls            int
}

func (f *fakeRefs) ShowToplevel(context.Context, string) (string, error) {
	return f.toplevel, f.toplevelErr
}

func (f *fakeRefs) GitDir(context.Context, string) (string, error) {
	f.gitDirCalls++
	return f.gitDir, f.gitDirErr
}

func TestBuildContext_ReadsGitStateThroughRefReader(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name          string
		refs          fakeRefs
		wantRepo      bool
		wantWorktree  bool
		wantGitDirHit bool
	}{
		{"not a repo", fakeRefs{toplevelErr: errors.New("not a git repository")}, false, false, false},
		{"plain checkout", fakeRefs{toplevel: "/code/app\n", gitDir: "/code/app/.git"}, true, false, true},
		{"linked worktree", fakeRefs{toplevel: "/wt/feat", gitDir: "/code/app/.git/worktrees/feat"}, true, true, true},
		{"plain checkout under a directory named worktrees", fakeRefs{toplevel: "/home/u/worktrees/app", gitDir: "/home/u/worktrees/app/.git"}, true, false, true},
		{"plain checkout whose root is named worktrees", fakeRefs{toplevel: "/home/u/worktrees", gitDir: "/home/u/worktrees/.git"}, true, false, true},
		{"git-dir lookup failure keeps IsWorktree false", fakeRefs{toplevel: "/r", gitDirErr: errors.New("boom")}, true, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			refs := tc.refs
			ctx := classifier.NewRuleBasedClassifierWithRefs(&refs).BuildContext("/cwd")

			assert.Equal(t, tc.wantRepo, ctx.IsGitRepo)
			assert.Equal(t, tc.wantWorktree, ctx.IsWorktree)
			assert.Equal(t, tc.wantGitDirHit, refs.gitDirCalls == 1, "GitDir is only asked inside a repository")
			if tc.wantRepo {
				assert.Equal(t, strings.TrimSpace(refs.toplevel), ctx.RepoRoot, "toplevel is trimmed")
			} else {
				assert.Empty(t, ctx.RepoRoot)
			}
		})
	}
}

// With refs=gogit the production adapter must reach the gogit backend, with a Local location
// naming the classifier's cwd. Not parallel: it swaps the process-wide backend.
func TestBuildContext_RefsGoGitReachesGoGitForLocal(t *testing.T) {
	rig := gitfake.NewRig(t)
	rig.Install(t)
	rig.GoGit.OnRepoRoot = func(backend.RepoLocation) (backend.RepoRoot, error) { return "/code/app", nil }
	rig.GoGit.OnGitDir = func(backend.RepoLocation) (backend.GitDir, error) { return "/code/app/.git/worktrees/w", nil }

	ctx := classifier.NewRuleBasedClassifier().BuildContext("/code/app/sub")

	assert.True(t, ctx.IsGitRepo)
	assert.True(t, ctx.IsWorktree)
	assert.Equal(t, "/code/app", ctx.RepoRoot)
	rig.RequireOnlyGoGit(t, backend.OpRepoRoot)
	require.Len(t, rig.GoGit.CallsTo(backend.OpGitDir), 1)
	assert.Equal(t, backend.Local{Root: "/code/app/sub"}, rig.GoGit.CallsTo(backend.OpGitDir)[0].Loc, "git-dir is asked of the cwd, like the old `git -C cwd`")
	assert.Equal(t, backend.Local{Root: "/code/app/sub"}, rig.GoGit.CallsTo(backend.OpRepoRoot)[0].Loc)
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := safeexec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

// Behaviour preserved against the real CLI backend: a subdirectory of a linked worktree, a
// plain checkout and a non-repository classify as they did when BuildContext spawned git itself.
func TestBuildContext_RealGit_RepoWorktreeAndNonRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	main := filepath.Join(root, "main")
	require.NoError(t, os.Mkdir(main, 0o755))
	runGit(t, main, "init", "-q", "-b", "main")
	runGit(t, main, "commit", "-q", "--allow-empty", "-m", "init")
	wt := filepath.Join(root, "worktrees", "feat")
	runGit(t, main, "worktree", "add", "-q", "-b", "feat", wt)
	sub := filepath.Join(wt, "pkg")
	require.NoError(t, os.Mkdir(sub, 0o755))
	c := classifier.NewRuleBasedClassifier()

	inWorktree := c.BuildContext(sub)
	assert.True(t, inWorktree.IsGitRepo)
	assert.True(t, inWorktree.IsWorktree)
	assert.Equal(t, wt, inWorktree.RepoRoot)

	plain := c.BuildContext(main)
	assert.True(t, plain.IsGitRepo)
	assert.False(t, plain.IsWorktree)
	assert.Equal(t, main, plain.RepoRoot)

	nonRepo := c.BuildContext(t.TempDir())
	assert.False(t, nonRepo.IsGitRepo)
	assert.False(t, nonRepo.IsWorktree)
	assert.Empty(t, nonRepo.RepoRoot)
}
