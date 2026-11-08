package gitwiring

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeExec(t *testing.T, dir, name, body string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, []byte(body), 0o755))
	return p
}

const sshFallbackWrapper = "#!/usr/bin/env sh\n# git-ssh-fallback: transparent HTTPS->SSH fallback wrapper for git.\nexec /usr/bin/git \"$@\"\n"

func TestLocalGitPathSkipsSSHFallbackWrapperForLocalOps(t *testing.T) {
	t.Parallel()
	wrapperDir, realDir := t.TempDir(), t.TempDir()
	writeExec(t, wrapperDir, "git", sshFallbackWrapper)
	real := writeExec(t, realDir, "git", "#!/bin/sh\nexit 0\n")
	pathEnv := wrapperDir + string(os.PathListSeparator) + realDir

	for _, sub := range []string{"rev-parse", "status", "log", "diff", "config", "worktree"} {
		assert.Equal(t, real, localGitPath(pathEnv, []string{"-C", "/x", sub}), sub)
	}
}

func TestLocalGitPathKeepsWrapperForNetworkAndUnknownOps(t *testing.T) {
	t.Parallel()
	wrapperDir, realDir := t.TempDir(), t.TempDir()
	writeExec(t, wrapperDir, "git", sshFallbackWrapper)
	writeExec(t, realDir, "git", "#!/bin/sh\nexit 0\n")
	pathEnv := wrapperDir + string(os.PathListSeparator) + realDir

	for _, sub := range []string{"fetch", "clone", "pull", "push", "ls-remote", "submodule", "remote", "credential", "lfs", "no-such-subcommand"} {
		assert.Empty(t, localGitPath(pathEnv, []string{sub}), sub)
	}
	assert.Empty(t, localGitPath(pathEnv, nil), "no subcommand")
}

func TestLocalGitPathLeavesOtherGitScriptsAlone(t *testing.T) {
	t.Parallel()
	stubDir, realDir := t.TempDir(), t.TempDir()
	writeExec(t, stubDir, "git", "#!/bin/sh\necho stub\n")
	writeExec(t, realDir, "git", "#!/bin/sh\nexit 0\n")
	pathEnv := stubDir + string(os.PathListSeparator) + realDir

	assert.Empty(t, localGitPath(pathEnv, []string{"status"}), "only the git-ssh-fallback wrapper is bypassed")
}

func TestLocalGitPathWithoutWrapperOrRealGit(t *testing.T) {
	t.Parallel()
	onlyWrapper := t.TempDir()
	writeExec(t, onlyWrapper, "git", sshFallbackWrapper)
	assert.Empty(t, localGitPath(onlyWrapper, []string{"status"}), "no real git behind the wrapper: keep PATH resolution")
	assert.Empty(t, localGitPath(t.TempDir(), []string{"status"}), "no git at all")
}
