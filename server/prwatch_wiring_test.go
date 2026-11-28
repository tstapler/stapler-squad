package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/pkg/events"
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
