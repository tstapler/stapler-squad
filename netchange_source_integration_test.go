//go:build integration

package main

import (
	"testing"
	"time"

	"go.uber.org/goleak"
)

// TestTailscaleNetmonSource_Close_NoGoroutineLeak covers Task 3.1.2c's
// mandatory leak check. It exercises a real *netmon.Monitor (netmon.New
// starts real OS-monitoring goroutines on darwin/linux) rather than a fake,
// since the point is to prove Close() actually tears down everything New +
// Start spun up, including the eventbus.Bus tailscaleNetmonSource owns.
//
// Gated behind the integration build tag (not run by default `make test`/
// `go test ./...`): it starts real OS-level network monitoring goroutines,
// which can flake on sandboxed CI runners lacking full netlink access.
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
//
// Gated behind the integration build tag for the same reason as the leak
// test above.
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
