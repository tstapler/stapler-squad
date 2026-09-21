package session

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeProgramExtension is a pointer-receiver programExtension test double
// (see the interface's doc comment on the pointer-type constraint). All
// fields are guarded by mu so it's safe to drive from the concurrent race
// test below.
type fakeProgramExtension struct {
	mu         sync.Mutex
	supported  bool
	running    bool
	startCalls int
	stopCalls  int
}

var _ programExtension = (*fakeProgramExtension)(nil)

func (f *fakeProgramExtension) Supported(*Instance) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.supported
}

func (f *fakeProgramExtension) Running() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.running
}

func (f *fakeProgramExtension) StartController(*Instance) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.startCalls++
	f.running = true
	return nil
}

func (f *fakeProgramExtension) StopController(*Instance) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopCalls++
	f.running = false
}

func (f *fakeProgramExtension) getStartCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.startCalls
}

func (f *fakeProgramExtension) getStopCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stopCalls
}

// TestStartController_RoutesToRegisteredExtensionWhenSupported proves
// RegisterProgramExtension's basic contract: a registered extension that
// reports Supported()==true is the one StartController dispatches to,
// instead of falling through to the default Claude-controller path.
func TestStartController_RoutesToRegisteredExtensionWhenSupported(t *testing.T) {
	fake := &fakeProgramExtension{supported: true}
	RegisterProgramExtension(fake)
	t.Cleanup(func() { UnregisterProgramExtension(fake) })

	inst := &Instance{Title: "start-controller-registered-ext-test"}

	err := inst.StartController()

	require.NoError(t, err)
	assert.Equal(t, 1, fake.getStartCalls(), "StartController must dispatch to the registered, Supported() extension")
	assert.False(t, inst.controllerManager.HasController(), "the default Claude-controller path must not also run")
}

// TestStopController_StopsRegisteredExtensionRunningButUnsupported is the
// regression guard for the Fix-2 BLOCKER: controllerExtensions() used to
// pre-filter registered extensions by Supported(), so a registered extension
// that was still Running() but no longer Supported() (e.g. a live-settable
// flag flipped mid-session) was silently dropped from the slice
// StopController ranges over and never got StopController() called on it --
// reintroducing "Bug 2" (see piExtension.Supported's doc comment) for the
// generic registry, even though the built-in &i.piExtension was immune
// because it was always unconditionally included.
//
// This MUST fail against the pre-fix controllerExtensions() (which drops
// fake from the slice because Supported()==false) and pass once
// controllerExtensions() returns every registered extension unconditionally.
func TestStopController_StopsRegisteredExtensionRunningButUnsupported(t *testing.T) {
	fake := &fakeProgramExtension{supported: false, running: true}
	RegisterProgramExtension(fake)
	t.Cleanup(func() { UnregisterProgramExtension(fake) })

	inst := &Instance{Title: "stop-controller-unsupported-running-ext-test"}

	inst.StopController()

	assert.Equal(t, 1, fake.getStopCalls(), "StopController must stop a registered extension that is Running() even though Supported() is now false")
}

// TestControllerExtensions_ConcurrentRegisterUnregisterRace is the
// regression guard for the Fix-1 BLOCKER: programExtensions was a
// package-level slice mutated by RegisterProgramExtension/
// UnregisterProgramExtension with no synchronization and read unguarded by
// controllerExtensions() -- a real data race, reproduced live under
// `go test -race` during code review. Must pass cleanly under -race once
// programExtensionsMu guards all three functions; it would trip
// `WARNING: DATA RACE` against the pre-fix, unguarded code.
func TestControllerExtensions_ConcurrentRegisterUnregisterRace(t *testing.T) {
	inst := &Instance{Title: "controller-extensions-race-test"}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_ = inst.controllerExtensions()
			}
		}
	}()

	const iterations = 200
	for i := 0; i < iterations; i++ {
		fake := &fakeProgramExtension{}
		RegisterProgramExtension(fake)
		UnregisterProgramExtension(fake)
	}

	close(stop)
	wg.Wait()
}

// TestControllerExtensions_MostRecentlyRegisteredWins pins the deliberate
// precedence order controllerExtensions() documents: among extensions
// simultaneously Supported() for the same instance, the most recently
// registered one wins (LIFO), with the built-in &i.piExtension always last
// as the final fallback. Before this test existed, that ordering was an
// accidental byproduct of the old prepend-on-match loop with nothing pinning
// it -- a future refactor could silently flip it with no test failing.
func TestControllerExtensions_MostRecentlyRegisteredWins(t *testing.T) {
	first := &fakeProgramExtension{supported: true}
	second := &fakeProgramExtension{supported: true}

	RegisterProgramExtension(first)
	t.Cleanup(func() { UnregisterProgramExtension(first) })
	RegisterProgramExtension(second)
	t.Cleanup(func() { UnregisterProgramExtension(second) })

	inst := &Instance{Title: "controller-extensions-precedence-test"}

	extensions := inst.controllerExtensions()

	require.Len(t, extensions, 3, "both registered fakes plus the built-in piExtension fallback")
	assert.Same(t, second, extensions[0], "most-recently-registered extension must win precedence")
	assert.Same(t, first, extensions[1])
	assert.Same(t, &inst.piExtension, extensions[2], "built-in piExtension must remain the final fallback")
}
