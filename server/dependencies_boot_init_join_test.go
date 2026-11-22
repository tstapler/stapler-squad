package server

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/envtest"
	"github.com/tstapler/stapler-squad/session"
)

func buildServiceDepsForBootInit(t *testing.T) *ServiceDeps {
	t.Helper()
	envtest.NewIsolatedStateDir(t)
	core, err := BuildCoreDepsWithOptions(BuildOptions{EntClient: session.NewTestEntRepository(t).GetEntClient()})
	require.NoError(t, err)
	svc, err := BuildServiceDeps(core)
	require.NoError(t, err)
	return svc
}

// StopBootInit must block until the goroutine has exited, not just signalled it.
func TestStartInstanceBootInit_ShouldBlockStopUntilGoroutineExits_WhenHeldAtGate(t *testing.T) {
	svc := buildServiceDepsForBootInit(t)
	started, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	releaseGate := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseGate)

	stop := startInstanceBootInit(svc, nil, func() { close(started); <-release })
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("boot-init goroutine never reached the gate")
	}

	stopped := make(chan struct{})
	go func() { stop(); close(stopped) }()
	select {
	case <-stopped:
		t.Fatal("stop returned while the boot-init goroutine was still running")
	case <-time.After(150 * time.Millisecond): // bounded negative check: stop must still be waiting
	}

	releaseGate()
	select {
	case <-stopped:
	case <-time.After(10 * time.Second):
		t.Fatal("stop did not return after the goroutine was released")
	}
}

// Server.Shutdown must call StopBootInit exactly once and wait for it.
func TestShutdown_ShouldJoinStopBootInit_WhenServerBuiltWithDeps(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	deps, err := BuildDependencies()
	require.NoError(t, err)
	orig := deps.StopBootInit
	require.NotNil(t, orig)

	var calls atomic.Int32
	entered, release := make(chan struct{}, 1), make(chan struct{})
	var releaseOnce sync.Once
	releaseStop := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseStop)
	deps.StopBootInit = func() {
		calls.Add(1)
		entered <- struct{}{}
		<-release
		orig()
	}
	srv := NewServerWithDeps("localhost:0", deps)

	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- srv.Shutdown() }()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("Shutdown never called StopBootInit")
	}
	select {
	case <-shutdownDone:
		t.Fatal("Shutdown returned before StopBootInit finished")
	case <-time.After(150 * time.Millisecond): // bounded negative check: Shutdown must still be joining
	}
	releaseStop()
	select {
	case err := <-shutdownDone:
		require.NoError(t, err)
	case <-time.After(15 * time.Second):
		t.Fatal("Shutdown did not return after StopBootInit finished")
	}
	require.Equal(t, int32(1), calls.Load())
}
