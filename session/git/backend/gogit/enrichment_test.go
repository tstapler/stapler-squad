package gogit

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadEnrichment_AuthorizesLinkedMetadata(t *testing.T) {
	t.Parallel()
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	work := filepath.Join(root, "work")
	gitdir := filepath.Join(root, "gitdir")
	common := filepath.Join(root, "common")
	for _, p := range []string{work, gitdir, common} {
		require.NoError(t, os.MkdirAll(p, 0700))
	}
	require.NoError(t, os.WriteFile(filepath.Join(work, ".git"), []byte("gitdir: ../gitdir\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(gitdir, "commondir"), []byte("../common\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(common, "config"), []byte("[remote \"origin\"]\n url = https://github.com/example/repo.git\n"), 0600))
	denied := errors.New("consent required")
	allowed := map[string]bool{}
	guard := func(path string) error {
		for _, p := range []string{gitdir, common} {
			if (path == p || strings.HasPrefix(path, p+string(filepath.Separator))) && !allowed[p] {
				return denied
			}
		}
		return nil
	}
	_, err = ReadEnrichment(work, guard)
	require.ErrorIs(t, err, denied)
	allowed[gitdir] = true
	_, err = ReadEnrichment(work, guard)
	require.ErrorIs(t, err, denied)
	allowed[common] = true
	info, err := ReadEnrichment(work, guard)
	require.NoError(t, err)
	require.Equal(t, "https://github.com/example/repo.git", info.Origin)
	require.Equal(t, common, info.CommonDir)
}

func TestReadEnrichment_RejectsIncludesAndUnsupported(t *testing.T) {
	t.Parallel()
	for _, config := range []string{"[include]\n path = /must-not-open\n", "[core]\n worktree = /must-not-open\n", "[extensions]\n worktreeConfig = true\n"} {
		t.Run(config, func(t *testing.T) {
			root := t.TempDir()
			require.NoError(t, os.Mkdir(filepath.Join(root, ".git"), 0700))
			require.NoError(t, os.WriteFile(filepath.Join(root, ".git", "config"), []byte(config), 0600))
			_, err := ReadEnrichment(root, func(path string) error { require.NotContains(t, path, "must-not-open"); return nil })
			require.Error(t, err)
		})
	}
	_, err := ReadEnrichment(t.TempDir(), func(string) error { return nil })
	require.Error(t, err)
}

func TestReadEnrichment_LocalConfigOnly(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, ".git"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".git", "config"), []byte("[remote \"origin\"]\n url = local-only\n"), 0600))
	info, err := ReadEnrichment(root, func(path string) error { require.NotContains(t, path, "must-not-open"); return nil })
	require.NoError(t, err)
	require.Equal(t, "local-only", info.Origin)
}

func TestReadEnrichment_DeniedSymlinkTargets(t *testing.T) {
	t.Parallel()
	for _, parent := range []bool{false, true} {
		t.Run(fmt.Sprint(parent), func(t *testing.T) {
			root, err := filepath.EvalSymlinks(t.TempDir())
			require.NoError(t, err)
			work, target := filepath.Join(root, "work"), filepath.Join(root, "target")
			require.NoError(t, os.Mkdir(work, 0700))
			require.NoError(t, os.Mkdir(target, 0700))
			require.NoError(t, os.WriteFile(filepath.Join(target, "config"), []byte("[remote \"origin\"]\nurl=secret\n"), 0600))
			if parent {
				require.NoError(t, os.Symlink(target, filepath.Join(work, ".git")))
			} else {
				require.NoError(t, os.Mkdir(filepath.Join(work, ".git"), 0700))
				require.NoError(t, os.Symlink(filepath.Join(target, "config"), filepath.Join(work, ".git", "config")))
			}
			denied := errors.New("denied before target access")
			_, err = ReadEnrichment(work, func(path string) error {
				if path == target || strings.HasPrefix(path, target+"/") {
					return denied
				}
				return nil
			})
			require.ErrorIs(t, err, denied)
		})
	}
}

func TestReadEnrichment_RejectsCyclesAndOversizedPointers(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	require.NoError(t, os.Symlink(".git", filepath.Join(root, ".git")))
	_, err := ReadEnrichment(root, func(string) error { return nil })
	require.Error(t, err)
	require.NoError(t, os.Remove(filepath.Join(root, ".git")))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".git"), []byte(strings.Repeat("x", maxGitfileSize+1)), 0600))
	_, err = ReadEnrichment(root, func(string) error { return nil })
	require.Error(t, err)
}
