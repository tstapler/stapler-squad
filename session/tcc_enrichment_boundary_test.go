package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/tcc"
)

// This characterizes the current boundary gap, not a complete consent guarantee.
func TestTCCEnrichmentBoundary_LinkedMetadataIsNotClassified(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	common := filepath.Join(root, "pretend-protected", "repo.git")
	_, err := git.PlainInit(common, true)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(common, "config"), []byte("[core]\n bare = false\n[remote \"origin\"]\n url = https://github.com/example/from-protected-config.git\n"), 0600))
	worktree := filepath.Join(root, "unprotected", "worktree")
	gitdir := filepath.Join(common, "worktrees", "linked")
	require.NoError(t, os.MkdirAll(worktree, 0700))
	require.NoError(t, os.MkdirAll(gitdir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(worktree, ".git"), []byte("gitdir: "+gitdir+"\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(gitdir, "commondir"), []byte("../..\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(gitdir, "HEAD"), []byte("ref: refs/heads/main\n"), 0600))
	var classified []string
	gate := tcc.NewConsentGate(func(path string) tcc.Result {
		classified = append(classified, path)
		return tcc.Result{Protected: path == common || path == gitdir, Root: common}
	}, nil, nil)
	require.True(t, gate.Ready(worktree))
	info, err := instance.DetectWorktree(worktree)
	require.NoError(t, err)
	require.Equal(t, "from-protected-config", info.GitHubRepo)
	require.Equal(t, []string{worktree}, classified)
	require.Empty(t, gate.Roots())
}
