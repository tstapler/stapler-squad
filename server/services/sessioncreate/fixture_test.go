package sessioncreate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	githubpkg "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services/autonomysvc"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/autonomous"
	"github.com/tstapler/stapler-squad/session/backlog"
	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/headless"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/registry"
	"github.com/tstapler/stapler-squad/session/sshremote"
	"github.com/tstapler/stapler-squad/session/summaries"
	"github.com/tstapler/stapler-squad/session/tagpoller"
	"github.com/tstapler/stapler-squad/session/tmux"
)

var testSocketCounter atomic.Uint64

// fixture is a Handler over a real test storage, with every Deps func answering
// "nothing installed". Tests replace fields of deps before calling handler().
type fixture struct {
	deps    Deps
	storage *session.Storage
	// agent is a program that stays alive, standing in for an agent CLI.
	agent  string
	socket string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{storage: svctest.CreateTestStorage(t)}
	f.agent = filepath.Join(t.TempDir(), "agent")
	require.NoError(t, os.WriteFile(f.agent, []byte("#!/bin/sh\nexec sleep 300\n"), 0o755)) //nolint:gosec // test script
	// A private tmux server: never the default one.
	f.socket = fmt.Sprintf("test_sessioncreate_%d_%d", os.Getpid(), testSocketCounter.Add(1))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = safeexec.CommandContext(ctx, tmux.Binary(), tmux.ResolveSocket(f.socket).Args("kill-server")...).Run()
	})
	bus := events.NewEventBus(64)
	t.Cleanup(bus.Close)
	f.deps = Deps{
		Bus:              bus,
		Store:            f.storage,
		TmuxServerSocket: f.socket,

		ConcreteStorage: func() *session.Storage { return f.storage },
		ConcStorage:     func() *session.Storage { return f.storage },

		Registry:      func() *registry.Registry { return nil },
		Poller:        func() *session.ReviewQueuePoller { return nil },
		TagPoller:     func() *tagpoller.SessionTagClassificationPoller { return nil },
		StatusManager: func() *instance.InstanceStatusManager { return nil },
		UserPRCache:   func() *githubpkg.UserPRCache { return nil },
		RemoteStores: func() (*sshremote.KeyStore, *sshremote.KnownHostsStore) {
			return nil, nil
		},
		PermissionHandler: func() sshremote.PermissionRequestHandler { return nil },
		BacklogListener:   func() *backlog.BacklogLifecycleListener { return nil },
		SummaryGenerator:  func() *summaries.SessionSummaryGenerator { return nil },
		HeadlessPool:      func() *headless.Pool { return nil },
		PhaseHook:         func() func(string) { return nil },

		DriverClient:      func() autonomous.HeadlessPoolClient { return nil },
		Autonomy:          func() *autonomysvc.AutonomousOrchestrationService { return nil },
		SSHClientPool:     tmux.NewSSHClientPool,
		GitBackend:        func() backend.Backend { return nil },
		GitHubResolver:    func() GitHubResolver { return instance.ResolveGitHubInputCtxWithHosts },
		ResolutionTimeout: func() time.Duration { return time.Minute },
		MCPServerURL:      func() string { return "" },
		WorkflowNames:     func() map[string]string { return nil },

		WorkspacePeersBlock: func(context.Context, string) string { return "" },

		Track:                func(fn func()) { fn() },
		WireCallbacks:        func(*instance.Instance) {},
		IndexForDelivery:     func(*instance.Instance) {},
		FindLive:             func(string) *instance.Instance { return nil },
		Instances:            func() []*instance.Instance { return nil },
		RemoveFromAllPollers: func(string) {},
	}
	return f
}

func (f *fixture) handler() *Handler { return New(f.deps) }

// storedInstance returns an unstarted directory session that has a storage row.
func (f *fixture) storedInstance(t *testing.T, title string) *instance.Instance {
	t.Helper()
	inst, err := instance.NewInstance(instance.InstanceOptions{
		Title: title, Path: t.TempDir(), Program: f.agent, TmuxServerSocket: f.socket,
	})
	require.NoError(t, err)
	require.NoError(t, f.storage.AddInstance(inst))
	t.Cleanup(func() {
		instance.StopSessionDriver(inst)
		instance.JoinSessionDriver(inst)
	})
	return inst
}

// closingPM is a process manager with a session that is not alive, which counts
// Close calls. Any other method panics.
type closingPM struct {
	instance.ProcessManager
	closed *atomic.Int32
}

func (closingPM) HasSession() bool { return true }
func (closingPM) IsAlive() bool    { return false }
func (p closingPM) Close() error {
	p.closed.Add(1)
	return nil
}

// leftoverInstance is storedInstance holding a tmux session a dead creation left behind.
func (f *fixture) leftoverInstance(t *testing.T, title string) (*instance.Instance, *atomic.Int32) {
	t.Helper()
	inst := f.storedInstance(t, title)
	closed := &atomic.Int32{}
	inst.SetProcessManagerForTest(closingPM{closed: closed})
	return inst, closed
}
