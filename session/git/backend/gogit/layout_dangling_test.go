package gogit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/git/backend"
)

// brokenLink makes a work tree directory whose .git file holds content. "{root}" in content is
// replaced by the temp root, so a test can name a directory that exists or one that does not.
func brokenLink(t *testing.T, content string, setup func(root string)) string {
	t.Helper()
	root := real(t, t.TempDir())
	if setup != nil {
		setup(root)
	}
	wt := filepath.Join(root, "wt")
	require.NoError(t, os.Mkdir(wt, 0o700))
	content = strings.ReplaceAll(content, "{root}", root)
	require.NoError(t, os.WriteFile(filepath.Join(wt, ".git"), []byte(content), 0o600))
	return wt
}

// The .git files below are .git files git answers with "fatal: not a git repository" (checked
// against real git by TestParityBrokenRepos).
func TestDanglingGitfileIsNotARepositoryForEveryServedOperation(t *testing.T) {
	cases := map[string]struct {
		content string
		setup   func(root string)
	}{
		"missing absolute":                {content: "gitdir: {root}/nowhere\n"},
		"missing relative":                {content: "gitdir: ../nowhere\n"},
		"no trailing newline":             {content: "gitdir: {root}/nowhere"},
		"crlf":                            {content: "gitdir: {root}/nowhere\r\n"},
		"second line is part of the path": {content: "gitdir: {root}/nowhere\nsecond\n"},
		"empty directory": {content: "gitdir: {root}/empty\n", setup: func(root string) {
			require.NoError(t, os.Mkdir(filepath.Join(root, "empty"), 0o700))
		}},
		"regular file": {content: "gitdir: {root}/file\n", setup: func(root string) {
			require.NoError(t, os.WriteFile(filepath.Join(root, "file"), []byte("x"), 0o600))
		}},
		"admin dir without HEAD": {content: "gitdir: {root}/admin\n", setup: func(root string) {
			require.NoError(t, os.MkdirAll(filepath.Join(root, "admin", "objects"), 0o700))
			require.NoError(t, os.MkdirAll(filepath.Join(root, "admin", "refs"), 0o700))
		}},
		"admin dir with a garbage HEAD": {content: "gitdir: {root}/admin\n", setup: func(root string) {
			require.NoError(t, os.MkdirAll(filepath.Join(root, "admin", "objects"), 0o700))
			require.NoError(t, os.MkdirAll(filepath.Join(root, "admin", "refs"), 0o700))
			require.NoError(t, os.WriteFile(filepath.Join(root, "admin", "HEAD"), []byte("garbage\n"), 0o600))
		}},
		"commondir names a missing directory": {content: "gitdir: {root}/admin\n", setup: func(root string) {
			require.NoError(t, os.Mkdir(filepath.Join(root, "admin"), 0o700))
			require.NoError(t, os.WriteFile(filepath.Join(root, "admin", "HEAD"), []byte("ref: refs/heads/main\n"), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(root, "admin", "commondir"), []byte("../../nowhere\n"), 0o600))
		}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			b := newBackend(t)
			loc := local(brokenLink(t, tc.content, tc.setup))
			_, errCommon := b.CommonDir(t.Context(), loc)
			_, errGit := b.GitDir(t.Context(), loc)
			_, errRoot := b.RepoRoot(t.Context(), loc)
			_, errBranch := b.CurrentBranch(t.Context(), loc)
			_, errRef := b.ResolveRef(t.Context(), loc, "HEAD")
			_, errCfg := b.GetConfig(t.Context(), loc, "user.name")
			for op, err := range map[string]error{"CommonDir": errCommon, "GitDir": errGit, "RepoRoot": errRoot, "CurrentBranch": errBranch, "ResolveRef": errRef, "GetConfig": errCfg} {
				assert.ErrorIs(t, err, backend.ErrNotARepo, op)
				assert.False(t, isFallback(err), "%s must answer, not fall back: %v", op, err)
			}
		})
	}
}

func TestMalformedOrUnreadableGitfileIsLeftToTheCLI(t *testing.T) {
	cases := map[string]struct {
		content string
		setup   func(root string)
	}{
		"garbage":   {content: "garbage\n"},
		"empty":     {content: ""},
		"no space":  {content: "gitdir:{root}/nowhere\n"},
		"no path":   {content: "gitdir: \n"},
		"oversized": {content: "gitdir: " + string(make([]byte, 5000))},
		"commondir with leading whitespace": {content: "gitdir: {root}/admin\n", setup: func(root string) {
			require.NoError(t, os.MkdirAll(filepath.Join(root, "admin"), 0o700))
			require.NoError(t, os.WriteFile(filepath.Join(root, "admin", "HEAD"), []byte("ref: refs/heads/main\n"), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(root, "admin", "commondir"), []byte("  ../..\n"), 0o600))
		}},
		"empty commondir": {content: "gitdir: {root}/admin\n", setup: func(root string) {
			require.NoError(t, os.MkdirAll(filepath.Join(root, "admin"), 0o700))
			require.NoError(t, os.WriteFile(filepath.Join(root, "admin", "HEAD"), []byte("ref: refs/heads/main\n"), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(root, "admin", "commondir"), nil, 0o600))
		}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := newBackend(t).CommonDir(t.Context(), local(brokenLink(t, tc.content, tc.setup)))
			assert.True(t, isFallback(err), "git dies with its own message here; got %v", err)
			assert.NotErrorIs(t, err, backend.ErrNotARepo)
		})
	}
}

// A directory with no repository anywhere is still "outside a repository" for `git config`, and
// only a dangling .git file turns that into a failure.
func TestGetConfigOutsideAnyRepositoryStillReadsGlobalScope(t *testing.T) {
	_, err := newBackend(t).GetConfig(t.Context(), local(real(t, t.TempDir())), "user.name")
	assert.ErrorIs(t, err, backend.ErrConfigUnset)
}
