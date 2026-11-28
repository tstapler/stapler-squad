package server

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/envtest"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/prwatchhost"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
)

func TestPRWatchDeps_should_CarryTheServerAddrDependenciesAndLifecycle_When_Built(t *testing.T) {
	t.Parallel()
	srv := &Server{}
	addr := "localhost:9222"
	srv.addr.Store(&addr)
	deps := &ServerDependencies{SessionService: &services.SessionService{}, Storage: &session.Storage{}, EventBus: events.NewEventBus(1)}

	d := prwatchDeps(srv, deps)

	assert.Equal(t, addr, d.Addr)
	assert.Same(t, deps.SessionService, d.Sessions)
	assert.Same(t, deps.Storage, d.Store)
	assert.Same(t, deps.EventBus, d.Bus)

	release, joined := make(chan struct{}), make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	d.Go(func() { <-release })
	go func() { srv.backgroundTasksWG.Wait(); close(joined) }()
	select {
	case <-joined:
		t.Fatal("Go must run fn under the server's background WaitGroup")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	select {
	case <-joined:
	case <-time.After(5 * time.Second):
		t.Fatal("background task never finished")
	}

	ran := false
	d.OnShutdown(func() { ran = true })
	require.Len(t, srv.shutdownHooks, 1)
	srv.shutdownHooks[0]()
	assert.True(t, ran)
}

// A nil *SessionService or *Storage would reach the leaf as a non-nil interface,
// so startPRWatch has to stop before building Deps. Reads the process-global
// status provider, so not parallel.
func TestStartPRWatch_should_StartNothing_When_SessionServiceOrStorageIsNil(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	t.Cleanup(func() { prwatchhost.SetStatusProvider(nil) })
	for name, deps := range map[string]*ServerDependencies{
		"no session service": {Storage: &session.Storage{}},
		"no storage":         {SessionService: &services.SessionService{}},
	} {
		prwatchhost.SetStatusProvider(nil)
		ctx, cancel := context.WithCancel(context.Background())
		srv := &Server{}

		startPRWatch(ctx, srv, deps)

		assert.Nil(t, prwatchhost.CurrentStatusProvider(), name)
		assert.Empty(t, srv.shutdownHooks, name)
		cancel()
		srv.backgroundTasksWG.Wait()
	}
}
