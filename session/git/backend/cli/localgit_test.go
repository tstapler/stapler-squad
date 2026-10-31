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

func TestResolveDirectGit_should_BypassWrapperOnly_When_PathGitIsSSHFallback(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	wrapper := writeExec(t, dir, "git", "#!/usr/bin/env sh\n# git-ssh-fallback: transparent wrapper\nexec /usr/bin/git \"$@\"\n")
	real := writeExec(t, dir, "realgit", "\x7fELF-not-a-script")
	lookup := func(string) (string, error) { return wrapper, nil }

	assert.Equal(t, real, resolveDirectGit(lookup, real, nil), "wrapper on PATH: use the real binary")
	assert.Equal(t, "git", resolveDirectGit(lookup, "", []string{wrapper, filepath.Join(dir, "missing")}), "no usable real git: fall back to PATH lookup")
	assert.Equal(t, "git", resolveDirectGit(func(string) (string, error) { return real, nil }, real, nil), "PATH git is not the wrapper: unchanged")
}

func TestLocalGitName_should_KeepNetworkOpsOnPath(t *testing.T) {
	t.Parallel()
	for _, op := range []string{"fetch", "push", "pull", "clone", "ls-remote", "submodule", "remote"} {
		assert.Equal(t, "git", localGitName([]string{"-C", "/r", op}), op)
	}
}
