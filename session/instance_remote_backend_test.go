package session

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/tmux"
)

type attachRecorder struct {
	backend.Backend
	locs      []backend.RepoLocation
	branchErr error
}

func (r *attachRecorder) ResolveRef(_ context.Context, loc backend.RepoLocation, _ backend.RefName) (backend.CommitSHA, error) {
	r.locs = append(r.locs, loc)
	return "0123456789abcdef0123456789abcdef01234567", nil
}

func (r *attachRecorder) CurrentBranch(_ context.Context, loc backend.RepoLocation) (backend.BranchName, error) {
	r.locs = append(r.locs, loc)
	if r.branchErr != nil {
		return "", r.branchErr
	}
	return "feature-x", nil
}

func attachRemoteWorktree(t *testing.T, rec *attachRecorder) (*Instance, RemoteExecutionTarget) {
	t.Helper()
	runner := tmux.NewSSHRunner(tmux.SSHTarget{Name: "box", Addr: "127.0.0.1:1"}, ssh.ClientConfig{})
	target := NewRemoteExecutionTarget(RemoteTarget{Name: "box", Host: "127.0.0.1:1"}, runner)
	inst := &Instance{
		Title:            "remote-attach",
		Path:             "/srv/repo",
		SessionType:      SessionTypeExistingWorktree,
		ExistingWorktree: "/srv/wt",
		ExecutionTarget:  target,
		GitBackend:       rec,
	}
	require.NoError(t, inst.setupFirstTimeWorktree())
	return inst, target
}

// TestSetupFirstTimeWorktree_RemoteAttach_ResolvesViaBackendWithTargetRunner pins that the remote
// attach lookups run through the injected Backend on a Remote location carrying the execution
// target's own runner (the identical value), never a Local one.
func TestSetupFirstTimeWorktree_RemoteAttach_ResolvesViaBackendWithTargetRunner(t *testing.T) {
	t.Parallel()
	rec := &attachRecorder{}
	inst, target := attachRemoteWorktree(t, rec)

	require.Len(t, rec.locs, 2, "ResolveRef and CurrentBranch")
	for _, loc := range rec.locs {
		remote, ok := loc.(backend.Remote)
		require.True(t, ok, "remote attach must never be located locally, got %T", loc)
		assert.Equal(t, backend.RemoteHost("box"), remote.Host)
		assert.Equal(t, "/srv/wt", remote.Dir())
		assert.Same(t, target.Runner(), remote.Runner)
	}
	assert.Equal(t, "feature-x", inst.Branch)
}

func TestSetupFirstTimeWorktree_RemoteAttach_DetachedHeadKeepsUnknownBranch(t *testing.T) {
	t.Parallel()
	inst, _ := attachRemoteWorktree(t, &attachRecorder{branchErr: backend.ErrDetachedHead})
	assert.Equal(t, "unknown", inst.Branch)
}
