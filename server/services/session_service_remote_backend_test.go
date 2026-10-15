package services

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/git/backend/cli"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// locRecordingBackend records the location of every worktree add and ref resolution while
// delegating to the real CLI backend, so the real in-process SSH host still does the work.
type locRecordingBackend struct {
	backend.Backend
	mu   sync.Mutex
	locs []backend.RepoLocation
}

func (b *locRecordingBackend) record(loc backend.RepoLocation) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.locs = append(b.locs, loc)
}

func (b *locRecordingBackend) AddWorktreeForExistingBranch(ctx context.Context, loc backend.RepoLocation, p backend.WorktreePath, br backend.BranchName) error {
	b.record(loc)
	return b.Backend.AddWorktreeForExistingBranch(ctx, loc, p, br)
}

func (b *locRecordingBackend) ResolveRef(ctx context.Context, loc backend.RepoLocation, ref backend.RefName) (backend.CommitSHA, error) {
	b.record(loc)
	return b.Backend.ResolveRef(ctx, loc, ref)
}

// countingRunner counts spawns; the gogit stand-in below is a CLI backend over it, so any
// call that reaches the "gogit" backend shows up as a spawn.
type countingRunner struct{ n atomic.Int64 }

func (r *countingRunner) Run(context.Context, string, string, ...string) ([]byte, error) {
	r.n.Add(1)
	return nil, nil
}

// TestCreateSession_RemoteTarget_GitRunsThroughBackendWithTheTargetsRunner covers plan Story
// 1.1.5: with every cohort set to gogit, remote worktree creation still reaches the CLI
// backend (never gogit), and the runner inside the Remote location is the identical value the
// RemoteExecutionTarget returns.
func TestCreateSession_RemoteTarget_GitRunsThroughBackendWithTheTargetsRunner(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test that starts a real tmux session")
	}
	srv := startRemoteSessionTestSSHServer(t)
	fix := newRemoteSessionFixture(t, srv)

	var cohorts backend.CohortMap
	for _, c := range backend.AllCohorts() {
		if c.ValidateMode(backend.BackendGoGit) == nil {
			cohorts = cohorts.With(c, backend.BackendGoGit)
		}
	}
	gogitSpawns := &countingRunner{}
	recorder := &locRecordingBackend{Backend: cli.New(tmux.LocalRunner{})}
	router, err := backend.NewRouter(backend.RouterConfig{
		Cohorts: cohorts,
		CLI:     recorder,
		GoGit:   cli.New(gogitSpawns),
	})
	require.NoError(t, err)
	fix.svc.setGitBackend(router)

	_, err = fix.svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:       "remote-backend-wiring",
		Path:        fix.repoPath,
		Branch:      "feature-backend",
		Program:     "sh",
		SessionType: sessionv1.SessionType_SESSION_TYPE_NEW_WORKTREE,
		Remote:      &sessionv1.RemoteTarget{RemoteName: "test-remote"},
	}))
	require.NoError(t, err)

	_, statErr := os.Stat(filepath.Join(fix.basePath, "remote-backend-wiring"))
	require.NoError(t, statErr, "the worktree must have been created on the remote host")

	var inst *session.Instance
	for _, candidate := range fix.poller.GetInstances() {
		if candidate.Title == "remote-backend-wiring" {
			inst = candidate
		}
	}
	require.NotNil(t, inst)
	target, ok := inst.GetExecutionTarget().(session.RemoteExecutionTarget)
	require.True(t, ok)

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	require.NotEmpty(t, recorder.locs, "git must have gone through the backend")
	for _, loc := range recorder.locs {
		remote, isRemote := loc.(backend.Remote)
		require.True(t, isRemote, "remote git must never be located locally, got %T", loc)
		assert.Same(t, target.Runner(), remote.Runner, "the runner reaching Backend must be the execution target's runner")
	}
	assert.Zero(t, gogitSpawns.n.Load(), "the gogit backend must never be reached for a Remote location")
}
