package sessionstream

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
)

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := safeexec.CommandContext(context.Background(), "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return strings.TrimSpace(string(out))
}

// A stored record that carries a worktree is diffed there, not at its Path.
func TestGetSessionDiff_ShouldDiffTheRecordedWorktree_When_TheStoredRecordHasOne(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q", "-b", "main")
	file := filepath.Join(dir, "f.txt")
	require.NoError(t, os.WriteFile(file, []byte("base\n"), 0o600))
	gitIn(t, dir, "add", ".")
	gitIn(t, dir, "commit", "-q", "-m", "base")
	base := gitIn(t, dir, "rev-parse", "HEAD")
	require.NoError(t, os.WriteFile(file, []byte("changed\n"), 0o600))

	record := domain.InstanceData{
		Title: "done", Path: t.TempDir(), // not a repo
		Worktree: domain.GitWorktreeData{RepoPath: dir, WorktreePath: dir, SessionName: "done", BranchName: "main", BaseCommitSHA: base},
	}
	h := New(nil, Ports{
		FindInstance:     func(string) *instance.Instance { return nil },
		ListInstanceData: func() ([]domain.InstanceData, error) { return []domain.InstanceData{record}, nil },
	})

	resp, err := h.GetSessionDiff(context.Background(), connect.NewRequest(&sessionv1.GetSessionDiffRequest{Id: "done"}))
	require.NoError(t, err)
	assert.EqualValues(t, 1, resp.Msg.DiffStats.Added)
	assert.Contains(t, resp.Msg.DiffStats.Content, "+changed")
}
