//go:build gitoracle

package capability

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/testutil/gitoracle"
)

type realRepo struct {
	t      *testing.T
	home   string
	root   string
	linked string
	g      gitoracle.Git
}

func newRealRepo(t *testing.T) realRepo {
	t.Helper()
	gitoracle.RequireGit(t)
	home, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	r := realRepo{t: t, home: home, root: filepath.Join(home, "repo"), linked: filepath.Join(home, "linked"), g: gitoracle.Git{T: t, Home: home}}
	require.NoError(t, os.MkdirAll(r.root, 0o700))
	r.g.Run(r.root, "init", "-q", "-b", "main", "--template=")
	r.g.Write(r.root, "a.txt", "1\n")
	r.g.Write(r.root, "sub/b.txt", "2\n")
	r.g.Commit(r.root, "first")
	r.g.Run(r.root, "worktree", "add", "-q", "-b", "wt", r.linked)
	return r
}

func (r realRepo) probe(dir string, op backend.OperationName, env ...string) []backend.Capability {
	r.t.Helper()
	d := New(Config{Environ: func() []string {
		return append(gitoracle.Env(r.home), env...)
	}, SystemAttributes: []string{}, SSHConfigs: []string{}})
	caps, err := d.Capabilities(context.Background(), backend.Local{Root: backend.RepoRoot(dir)}, op)
	require.NoError(r.t, err)
	return caps
}

func hasKind(caps []backend.Capability, k backend.CapabilityKind) bool {
	for _, c := range caps {
		if c.Kind == k {
			return true
		}
	}
	return false
}

// TestHookDetectionAgreesWithGitRunningTheHook: a hook is "present" exactly when real git runs it.
func TestHookDetectionAgreesWithGitRunningTheHook(t *testing.T) {
	r := newRealRepo(t)
	marker := filepath.Join(r.home, "ran")
	hooks := filepath.Join(r.root, ".git", "hooks")
	require.NoError(t, os.MkdirAll(hooks, 0o700))
	script := "#!/bin/sh\ntouch '" + marker + "'\n"
	for name, c := range map[string]struct {
		file string
		mode os.FileMode
		runs bool
	}{
		"executable":     {"pre-commit", 0o755, true},
		"not executable": {"pre-commit", 0o644, false},
		"sample":         {"pre-commit.sample", 0o755, false},
		"other hook":     {"post-commit", 0o755, true},
	} {
		t.Run(name, func(t *testing.T) {
			entries, _ := os.ReadDir(hooks)
			for _, e := range entries {
				require.NoError(t, os.Remove(filepath.Join(hooks, e.Name())))
			}
			_ = os.Remove(marker)
			require.NoError(t, os.WriteFile(filepath.Join(hooks, c.file), []byte(script), c.mode))
			require.NoError(t, os.Chmod(filepath.Join(hooks, c.file), c.mode))
			r.g.Write(r.root, "a.txt", name+"\n")
			r.g.Commit(r.root, name)
			_, err := os.Stat(marker)
			ran := err == nil
			require.Equal(t, c.runs, ran, "premise: real git hook behaviour")
			for _, dir := range []string{r.root, r.linked} {
				assert.Equal(t, ran, hasKind(r.probe(dir, backend.OpCommit), backend.CapHooks), dir)
			}
		})
	}
}

func TestRealIndexFeatures(t *testing.T) {
	t.Run("split index", func(t *testing.T) {
		r := newRealRepo(t)
		r.g.Run(r.root, "update-index", "--split-index")
		r.g.Write(r.root, "a.txt", "changed\n")
		r.g.Run(r.root, "add", "a.txt")
		assert.True(t, hasKind(r.probe(r.root, backend.OpStatus), backend.CapUnsupportedIndex))
	})
	t.Run("v3 with intent-to-add and v4 are not unsupported", func(t *testing.T) {
		r := newRealRepo(t)
		r.g.Write(r.root, "new.txt", "n\n")
		r.g.Run(r.root, "add", "-N", "new.txt")
		assert.False(t, hasKind(r.probe(r.root, backend.OpStatus), backend.CapUnsupportedIndex))
		r.g.Run(r.root, "update-index", "--index-version", "4")
		assert.False(t, hasKind(r.probe(r.root, backend.OpStatus), backend.CapUnsupportedIndex))
	})
	t.Run("sparse checkout", func(t *testing.T) {
		r := newRealRepo(t)
		r.g.Run(r.root, "sparse-checkout", "set", "--no-cone", "/a.txt")
		assert.True(t, hasKind(r.probe(r.root, backend.OpStatus), backend.CapSparse))
	})
	t.Run("gitlink", func(t *testing.T) {
		r := newRealRepo(t)
		head := r.g.Run(r.root, "rev-parse", "HEAD")
		r.g.Run(r.root, "update-index", "--add", "--cacheinfo", "160000,"+head+",vendored")
		assert.True(t, hasKind(r.probe(r.root, backend.OpStatus), backend.CapSubmodule))
	})
	t.Run("plain repository has none of them", func(t *testing.T) {
		r := newRealRepo(t)
		for _, dir := range []string{r.root, r.linked} {
			assert.Empty(t, r.probe(dir, backend.OpStatus), dir)
			assert.Empty(t, r.probe(dir, backend.OpCommit), dir)
		}
	})
}

func TestRealShallowClone(t *testing.T) {
	r := newRealRepo(t)
	r.g.Write(r.root, "a.txt", "second\n")
	r.g.Commit(r.root, "second")
	shallow := filepath.Join(r.home, "shallow")
	r.g.Run(r.home, "-c", "protocol.file.allow=always", "clone", "-q", "--depth", "1", "file://"+r.root, shallow)
	assert.True(t, hasKind(r.probe(shallow, backend.OpMergeBase), backend.CapShallow))
	assert.False(t, hasKind(r.probe(r.root, backend.OpMergeBase), backend.CapShallow))
}

// TestLinkedWorktreeGitdirIncludeIfMatchesLikeGit: the maintainer's case — a gitdir: includeIf
// on the parent of the main repo applies to its linked worktrees, which live elsewhere.
func TestLinkedWorktreeGitdirIncludeIfMatchesLikeGit(t *testing.T) {
	r := newRealRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(r.home, "sign.cfg"), []byte("[commit]\n\tgpgsign = true\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(r.home, ".gitconfig"), []byte("[user]\n\tname = x\n[includeIf \"gitdir:"+r.home+"/repo/\"]\n\tpath = "+filepath.Join(r.home, "sign.cfg")+"\n"), 0o600))
	for _, dir := range []string{r.root, r.linked} {
		env := append(gitoracle.Env(r.home), "GIT_CONFIG_GLOBAL="+filepath.Join(r.home, ".gitconfig"))
		d := New(Config{Environ: func() []string { return env }, SystemAttributes: []string{}})
		caps, err := d.Capabilities(context.Background(), backend.Local{Root: backend.RepoRoot(dir)}, backend.OpCommit)
		require.NoError(t, err)
		assert.True(t, hasKind(caps, backend.CapGPGSign), dir)
	}
}

// TestFilterDetectionAgreesWithGitRunningTheFilter: the detector reports a filter exactly when
// `git add` really runs the clean driver, including untracked nested .gitattributes and nested
// or global macros (each a fail-open found in review).
func TestFilterDetectionAgreesWithGitRunningTheFilter(t *testing.T) {
	cases := map[string]struct {
		setup func(r realRepo)
		runs  bool
	}{
		"untracked nested attributes": {func(r realRepo) {
			r.g.Write(r.root, "newdir/.gitattributes", "* filter=foo\n")
			r.g.Write(r.root, "newdir/x.txt", "x\n")
		}, true},
		"nested macros": {func(r realRepo) {
			r.g.Write(r.root, ".gitattributes", "[attr]inner filter=foo\n[attr]outer inner\n*.dat outer\n")
			r.g.Write(r.root, "x.dat", "x\n")
		}, true},
		"global macro used by nested file": {func(r realRepo) {
			require.NoError(r.t, os.MkdirAll(filepath.Join(r.home, ".config", "git"), 0o700))
			require.NoError(r.t, os.WriteFile(filepath.Join(r.home, ".config", "git", "attributes"), []byte("[attr]viafoo filter=foo\n"), 0o600))
			r.g.Write(r.root, "sub/.gitattributes", "*.dat viafoo\n")
			r.g.Write(r.root, "sub/x.dat", "x\n")
		}, true},
		"attribute for a filter nobody configured": {func(r realRepo) {
			r.g.Write(r.root, ".gitattributes", "*.dat filter=other\n")
			r.g.Write(r.root, "x.dat", "x\n")
		}, false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			r := newRealRepo(t)
			marker := filepath.Join(r.home, "filter-ran")
			require.NoError(t, os.WriteFile(filepath.Join(r.home, ".gitconfig"), []byte("[filter \"foo\"]\n\tclean = touch "+marker+"; cat\n"), 0o600))
			c.setup(r)
			env := append(gitoracle.Env(r.home), "GIT_CONFIG_GLOBAL="+filepath.Join(r.home, ".gitconfig"), "XDG_CONFIG_HOME="+filepath.Join(r.home, ".config"))
			cmd := execGit(r.root, env, "add", "-A")
			require.NoError(t, cmd.Run())
			_, err := os.Stat(marker)
			ran := err == nil
			require.Equal(t, c.runs, ran, "premise: real git filter behaviour")
			d := New(Config{Environ: func() []string { return env }, SystemAttributes: []string{}})
			caps, err := d.Capabilities(context.Background(), backend.Local{Root: backend.RepoRoot(r.root)}, backend.OpAdd)
			require.NoError(t, err)
			assert.Equal(t, ran, hasKind(caps, backend.CapLFS))
		})
	}
}

// TestHooksPathInsideWorktreeHookArrivesWithTheCheckout: a relative core.hooksPath whose hook
// only exists on the branch being checked out still runs (husky pattern); the detector must route.
func TestHooksPathInsideWorktreeHookArrivesWithTheCheckout(t *testing.T) {
	r := newRealRepo(t)
	marker := filepath.Join(r.home, "post-checkout-ran")
	r.g.Run(r.root, "config", "core.hooksPath", ".githooks")
	r.g.Run(r.root, "checkout", "-q", "-b", "other")
	r.g.Write(r.root, ".githooks/post-checkout", "#!/bin/sh\ntouch '"+marker+"'\n")
	require.NoError(t, os.Chmod(filepath.Join(r.root, ".githooks", "post-checkout"), 0o755))
	r.g.Commit(r.root, "add hook")
	_ = os.Remove(marker)
	r.g.Run(r.root, "checkout", "-q", "main")
	_ = os.Remove(marker)
	// main has no .githooks; the detector sees no hook files, yet checking out `other` runs one
	caps := r.probe(r.root, backend.OpSwitchBranch)
	require.NoError(t, execGit(r.root, gitoracle.Env(r.home), "checkout", "-q", "other").Run())
	_, err := os.Stat(marker)
	require.NoError(t, err, "premise: the hook ran on checkout")
	assert.True(t, hasKind(caps, backend.CapHooks))
}

func execGit(dir string, env []string, args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	cmd.Dir, cmd.Env = dir, env
	return cmd
}

func TestUntrackedNestedRepositoryIsAGitlinkForAdd(t *testing.T) {
	r := newRealRepo(t)
	r.g.Run(r.root, "init", "-q", "-b", "main", "--template=", "nested")
	r.g.Write(r.root, "nested/y.txt", "y\n")
	r.g.Commit(filepath.Join(r.root, "nested"), "n")
	assert.True(t, hasKind(r.probe(r.root, backend.OpAdd), backend.CapSubmodule))
	r.g.Run(r.root, "add", "-A")
	assert.Contains(t, r.g.Run(r.root, "ls-files", "-s", "nested"), "160000", "premise: real git stores a gitlink")
}

func TestSmudgeFilterArrivingWithACheckoutRoutes(t *testing.T) {
	r := newRealRepo(t)
	marker := filepath.Join(r.home, "smudge-ran")
	r.g.Run(r.root, "config", "filter.x.smudge", "touch "+marker+"; cat")
	r.g.Run(r.root, "checkout", "-q", "-b", "other")
	r.g.Write(r.root, ".gitattributes", "*.dat filter=x\n")
	r.g.Write(r.root, "f.dat", "d\n")
	r.g.Commit(r.root, "attrs")
	r.g.Run(r.root, "checkout", "-q", "main")
	_ = os.Remove(marker)
	caps := r.probe(r.root, backend.OpSwitchBranch) // main has no .gitattributes on disk
	r.g.Run(r.root, "checkout", "-q", "other")
	_, err := os.Stat(marker)
	require.NoError(t, err, "premise: the smudge filter ran on checkout")
	assert.True(t, hasKind(caps, backend.CapLFS))
}

func TestConfiguredProgramsRunByGitAreReported(t *testing.T) {
	r := newRealRepo(t)
	marker := filepath.Join(r.home, "fsmonitor-ran")
	script := filepath.Join(r.home, "fsm.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\ntouch '"+marker+"'\nprintf '\\0'\n"), 0o755))
	r.g.Run(r.root, "config", "core.fsmonitor", script)
	r.g.Run(r.root, "status", "--porcelain")
	_, err := os.Stat(marker)
	require.NoError(t, err, "premise: git ran the fsmonitor hook for status")
	assert.True(t, hasKind(r.probe(r.root, backend.OpStatus), backend.CapHooks))
}
