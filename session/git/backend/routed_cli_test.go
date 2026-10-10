package backend_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/git/backend/cli"
)

type countingRunner struct {
	calls atomic.Int32
	err   error
}

func (c *countingRunner) Run(context.Context, string, string, ...string) ([]byte, error) {
	c.calls.Add(1)
	return []byte("main\n"), c.err
}

func realRouter(t *testing.T, local backend.Runner, gg backend.Backend) *backend.Router {
	t.Helper()
	_, mp := newMetricsRig(t)
	r, err := backend.NewRouter(backend.RouterConfig{
		Cohorts: allCohorts(backend.BackendGoGit), CLI: cli.New(local), GoGit: gg,
		Preflight: noPreflight, Meter: mp.Meter("test"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestRemoteWithNoRunnerNeverRunsLocally(t *testing.T) {
	local := &countingRunner{}
	gg := strictGoGit(t, newEventLog())
	r := realRouter(t, local, gg)
	if _, err := r.CurrentBranch(ctxBG, backend.Remote{Host: "h", Path: "/p"}); !errors.Is(err, backend.ErrNoRemoteRunner) {
		t.Fatalf("err = %v, want ErrNoRemoteRunner", err)
	}
	if local.calls.Load() != 0 {
		t.Fatal("a remote location with no runner must never fall through to the local runner")
	}
}

func TestRemoteDialErrorIsReturnedNotRetriedLocally(t *testing.T) {
	local := &countingRunner{}
	remote := &countingRunner{err: errors.New("dial tcp: connection refused")}
	r := realRouter(t, local, strictGoGit(t, newEventLog()))
	_, err := r.CurrentBranch(ctxBG, backend.Remote{Host: "h", Path: "/p", Runner: remote})
	if err == nil {
		t.Fatal("the dial error must be returned")
	}
	if local.calls.Load() != 0 || remote.calls.Load() == 0 {
		t.Fatalf("local=%d remote=%d: only the remote runner may be used", local.calls.Load(), remote.calls.Load())
	}
}

func TestRemoteRunnerReceivesGitArgvAtRemotePath(t *testing.T) {
	local := &countingRunner{}
	remote := &recordingRunner{}
	r := realRouter(t, local, strictGoGit(t, newEventLog()))
	if got, err := r.CurrentBranch(ctxBG, backend.Remote{Host: "h", Path: "/p", Runner: remote}); err != nil || got != "main" {
		t.Fatalf("CurrentBranch = %q, %v", got, err)
	}
	if remote.dir != "/p" || remote.name != "git" || len(remote.args) != 3 || remote.args[1] != "--abbrev-ref" {
		t.Fatalf("remote saw dir=%q %s %v", remote.dir, remote.name, remote.args)
	}
	if local.calls.Load() != 0 {
		t.Fatal("local runner must not be used")
	}
}

type recordingRunner struct {
	dir, name string
	args      []string
}

func (r *recordingRunner) Run(_ context.Context, dir, name string, args ...string) ([]byte, error) {
	r.dir, r.name, r.args = dir, name, args
	return []byte("main\n"), nil
}
