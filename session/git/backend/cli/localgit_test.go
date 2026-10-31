package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Fake-runner tests assert the command name "git"; pin it so a developer-machine wrapper does not change them.
func init() { directGit = func() string { return "git" } }

func TestGitSubcommand_should_SkipFlagValues(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "rev-parse", gitSubcommand([]string{"rev-parse", "HEAD"}))
	assert.Equal(t, "fetch", gitSubcommand([]string{"-C", "/repo", "-c", "a=b", "--no-pager", "fetch", "origin"}))
	assert.Equal(t, "log", gitSubcommand([]string{"--git-dir", "/x/.git", "log"}))
	assert.Equal(t, "", gitSubcommand([]string{"--version"}))
}

func writeExec(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, []byte(body), 0o755))
	return p
}

const wrapperBody = "#!/usr/bin/env sh\n# git-ssh-fallback: transparent wrapper\nexec /usr/bin/git \"$@\"\n"

func TestResolveDirectGit_should_WalkPathPastTheWrapper(t *testing.T) {
	t.Parallel()
	wrapDir, brewDir, sysDir := t.TempDir(), t.TempDir(), t.TempDir()
	writeExec(t, wrapDir, "git", wrapperBody)
	brew := writeExec(t, brewDir, "git", "\x7fELF-brew")
	writeExec(t, sysDir, "git", "\x7fELF-system")
	path := wrapDir + string(os.PathListSeparator) + brewDir + string(os.PathListSeparator) + sysDir

	assert.Equal(t, brew, resolveDirectGit(path, ""), "same git the wrapper would pick: first non-wrapper on PATH, not a hardcoded /usr/bin")
	assert.Equal(t, "git", resolveDirectGit(brewDir+string(os.PathListSeparator)+wrapDir, ""), "PATH git is not the wrapper: unchanged")
	assert.Equal(t, "git", resolveDirectGit(wrapDir, ""), "nothing but the wrapper: unchanged")
}

func TestResolveDirectGit_should_HonorRealGitOnlyWhenNamedGit(t *testing.T) {
	t.Parallel()
	wrapDir, otherDir, sysDir := t.TempDir(), t.TempDir(), t.TempDir()
	writeExec(t, wrapDir, "git", wrapperBody)
	named := writeExec(t, otherDir, "git", "\x7fELF-pinned")
	odd := writeExec(t, otherDir, "git-real", "\x7fELF-odd")
	sys := writeExec(t, sysDir, "git", "\x7fELF-system")
	path := wrapDir + string(os.PathListSeparator) + sysDir

	assert.Equal(t, named, resolveDirectGit(path, named))
	assert.Equal(t, sys, resolveDirectGit(path, odd), "a REAL_GIT not named git would lose RequiredGitEnv and spawn counting (both key on basename)")
	assert.Equal(t, sys, resolveDirectGit(path, wrapDir+"/git"), "REAL_GIT pointing at the wrapper is ignored")
}

func TestLocalGitName_should_KeepNetworkOpsOnPath(t *testing.T) {
	t.Parallel()
	for _, op := range []string{"fetch", "push", "pull", "clone", "ls-remote", "submodule", "remote"} {
		assert.Equal(t, "git", localGitName([]string{"-C", "/r", op}), op)
	}
}
