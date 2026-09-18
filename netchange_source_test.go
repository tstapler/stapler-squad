package main

import (
	"errors"
	"testing"
	"time"

	"go.uber.org/goleak"
)

// TestTailscaleNetmonSource_Close_NoGoroutineLeak covers Task 3.1.2c's
// mandatory leak check. It exercises a real *netmon.Monitor (netmon.New
// starts real OS-monitoring goroutines on darwin/linux) rather than a fake,
// since the point is to prove Close() actually tears down everything New +
// Start spun up, including the eventbus.Bus tailscaleNetmonSource owns.
func TestTailscaleNetmonSource_Close_NoGoroutineLeak(t *testing.T) {
	defer goleak.VerifyNone(t)

	src, err := newTailscaleNetmonSource()
	if err != nil {
		t.Fatalf("newTailscaleNetmonSource: %v", err)
	}

	unregister := src.RegisterChangeCallback(func() {})
	unregister()

	if err := src.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// TestTailscaleNetmonSource_RegisterChangeCallback_FiresOnInjectedEvent uses
// netmon's own test-injection hook, *netmon.Monitor.InjectEvent (see
// net/netmon/netmon.go in the pinned v1.102.4 source: "InjectEvent forces
// the monitor to pretend there was a network change ... registered
// ChangeFunc callbacks will be called within the event coalescing period").
// The pinned version exposes no higher-level fake/mock monitor type (no
// NewFakeMonitor — confirmed via `go doc tailscale.com/net/netmon`, which
// lists only New and NewStatic as constructors), so this test reaches past
// the NetworkChangeSource interface into the unexported mon field (legal
// from an in-package _test.go file) to call InjectEvent on the real
// monitor's real debounce/pump goroutines — the closest thing to the
// "real interface change occurs" acceptance criterion that CI (no macOS
// runner, per pitfalls.md §5) can exercise. Task 3.1.2d's manual macOS
// checklist remains the only way to verify a *genuine* OS-level network
// change reaches this path end to end.
func TestTailscaleNetmonSource_RegisterChangeCallback_FiresOnInjectedEvent(t *testing.T) {
	defer goleak.VerifyNone(t)

	src, err := newTailscaleNetmonSource()
	if err != nil {
		t.Fatalf("newTailscaleNetmonSource: %v", err)
	}
	defer src.Close()

	fired := make(chan struct{}, 1)
	unregister := src.RegisterChangeCallback(func() {
		select {
		case fired <- struct{}{}:
		default:
		}
	})
	defer unregister()

	src.mon.InjectEvent()

	select {
	case <-fired:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for injected network-change callback to fire")
	}
}

// fakeNetworkChangeSource is a no-op NetworkChangeSource used only to
// confirm setupNetworkChangeEvents' happy path doesn't itself misbehave;
// the error path below is what Task 3.1.2b / validation.md's
// TestNewTailscaleNetmonSource_ErrorDoesNotFailStartup actually cares about.
type fakeNetworkChangeSource struct {
	registerFn func(fn func()) func()
	closeFn    func() error
}

func (f *fakeNetworkChangeSource) RegisterChangeCallback(fn func()) (unregister func()) {
	if f.registerFn != nil {
		return f.registerFn(fn)
	}
	return func() {}
}

func (f *fakeNetworkChangeSource) Close() error {
	if f.closeFn != nil {
		return f.closeFn()
	}
	return nil
}

// TestNewTailscaleNetmonSource_ErrorDoesNotFailStartup verifies Task
// 3.1.2b's "log+continue on error" contract: when the injected constructor
// fails, setupNetworkChangeEvents still returns a usable events channel and
// a safe-to-call cleanup, with no error returned to the caller (there is
// nothing to propagate -- the runtime phase must never fail startup just
// because the OS network-change source is unavailable).
func TestNewTailscaleNetmonSource_ErrorDoesNotFailStartup(t *testing.T) {
	wantErr := errors.New("boom: no route socket available")
	calls := 0

	events, cleanup := setupNetworkChangeEvents(func() (NetworkChangeSource, error) {
		calls++
		return nil, wantErr
	})

	if calls != 1 {
		t.Fatalf("expected newSource to be called once, got %d", calls)
	}
	if events == nil {
		t.Fatal("expected a non-nil events channel even on construction failure")
	}
	if cleanup == nil {
		t.Fatal("expected a non-nil cleanup func even on construction failure")
	}

	// cleanup must be safe to call unconditionally (main.go always
	// registers it with a.OnStop regardless of whether construction
	// succeeded).
	cleanup()

	// The channel must never receive a signal on its own -- degrading to
	// timer-only means truly nothing writes to it.
	select {
	case <-events:
		t.Fatal("events channel should never fire when construction failed")
	case <-time.After(50 * time.Millisecond):
	}
}

// TestNewTailscaleNetmonSource_SuccessRegistersAndCleansUp checks the
// opposite branch: a successful construction registers a callback and
// cleanup calls both unregister and Close.
func TestNewTailscaleNetmonSource_SuccessRegistersAndCleansUp(t *testing.T) {
	var unregistered, closed bool

	fake := &fakeNetworkChangeSource{
		registerFn: func(fn func()) func() {
			return func() { unregistered = true }
		},
		closeFn: func() error {
			closed = true
			return nil
		},
	}

	events, cleanup := setupNetworkChangeEvents(func() (NetworkChangeSource, error) {
		return fake, nil
	})
	if events == nil {
		t.Fatal("expected a non-nil events channel")
	}

	cleanup()

	if !unregistered {
		t.Fatal("expected cleanup to call unregister")
	}
	if !closed {
		t.Fatal("expected cleanup to call Close")
	}
}
