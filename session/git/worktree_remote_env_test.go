package git

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/git/backend/cli"
)

// rawRemoteRunner records the argv exactly as it reaches the SSH layer (no `env` unwrapping).
type rawRemoteRunner struct {
	mu    sync.Mutex
	calls [][]string
}

func (r *rawRemoteRunner) Run(_ context.Context, _, name string, args ...string) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, append([]string{name}, args...))
	return nil, nil
}

func (*rawRemoteRunner) Start(context.Context, string, string, ...string) (io.WriteCloser, io.ReadCloser, func() error, error) {
	return nil, nil, nil, errors.New("not supported")
}

func (*rawRemoteRunner) IsRemote() bool { return true }

func (r *rawRemoteRunner) requireAllGitCallsCarryRequiredEnv(t *testing.T) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	require.NotEmpty(t, r.calls)
	prefix := append([]string{"env"}, backend.RequiredGitEnv()...)
	for _, call := range r.calls {
		require.GreaterOrEqual(t, len(call), len(prefix)+1, "call: %v", call)
		assert.Equal(t, prefix, call[:len(prefix)], "remote git must run under RequiredGitEnv: %v", call)
		assert.Equal(t, "git", call[len(prefix)], "call: %v", call)
	}
}

func TestGitWorktreeRemoteGit_CarriesRequiredGitEnv(t *testing.T) {
	newWorktree := func(runner *rawRemoteRunner) *GitWorktree {
		return NewGitWorktreeFromStorageWithExecutor("/srv/repo", "/srv/wt", "sess", "feature", "", WithCommandRunner(runner))
	}

	t.Run("PushBranch", func(t *testing.T) {
		runner := &rawRemoteRunner{}
		require.NoError(t, newWorktree(runner).PushBranch())
		runner.requireAllGitCallsCarryRequiredEnv(t)
	})
	t.Run("RenameBranch", func(t *testing.T) {
		runner := &rawRemoteRunner{}
		require.NoError(t, newWorktree(runner).RenameBranch("renamed"))
		runner.requireAllGitCallsCarryRequiredEnv(t)
	})
	t.Run("remote fetch of default-branch candidates", func(t *testing.T) {
		runner := &rawRemoteRunner{}
		_, _, _ = ResolveRemoteWorktreeBaseCommit(context.Background(), runner, "/srv/repo")
		runner.requireAllGitCallsCarryRequiredEnv(t)
	})
	t.Run("a cli backend over the same runner agrees", func(t *testing.T) {
		runner := &rawRemoteRunner{}
		require.NoError(t, cli.New(nil).CreateBranch(context.Background(),
			backend.Remote{Path: "/srv/repo", Runner: runner}, backend.CreateBranchRequest{Name: "b"}))
		runner.requireAllGitCallsCarryRequiredEnv(t)
	})
}
