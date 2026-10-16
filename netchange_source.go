package main

import (
	"fmt"

	"github.com/tstapler/stapler-squad/log"
	"tailscale.com/net/netmon"
	"tailscale.com/types/logger"
	"tailscale.com/util/eventbus"
)

// NetworkChangeSource abstracts the OS-level network-change notifier that
// feeds HostnameDetector.events (Epic 3.1). The interface exists so
// main.go's wiring (setupNetworkChangeEvents) and its tests can substitute a
// fake without depending on tailscaleNetmonSource's real OS hooks.
type NetworkChangeSource interface {
	// RegisterChangeCallback registers fn to be called (on its own
	// goroutine, per netmon.Monitor.RegisterChangeCallback's doc) whenever
	// the underlying monitor observes a network change, coalesced by its
	// own debounce window. The returned unregister func removes the
	// callback; it is also safe to just call Close instead.
	RegisterChangeCallback(fn func()) (unregister func())
	Close() error
}

// tailscaleNetmonSource wraps a real *netmon.Monitor (github.com/tailscale
// /net/netmon, added in Story 3.1.1) as a NetworkChangeSource. It owns both
// the monitor and the eventbus.Bus netmon.New requires -- netmon.Monitor's
// own Close() does not close the bus that constructed it, so
// tailscaleNetmonSource.Close must close both to avoid leaking the bus's
// internal dispatch goroutine.
type tailscaleNetmonSource struct {
	bus *eventbus.Bus
	mon *netmon.Monitor
}

// newTailscaleNetmonSource creates and starts a netmon.Monitor. netmon.New's
// doc comment says it "instantiates and starts" the monitor, but reading the
// pinned v1.102.4 source (net/netmon/netmon.go) shows New only wires up
// state and leaves the monitor inert until Start is called explicitly --
// without this call RegisterChangeCallback would never fire.
func newTailscaleNetmonSource() (*tailscaleNetmonSource, error) {
	bus := eventbus.New()

	mon, err := netmon.New(bus, logger.Discard)
	if err != nil {
		bus.Close()
		return nil, fmt.Errorf("create network monitor: %w", err)
	}
	mon.Start()

	return &tailscaleNetmonSource{bus: bus, mon: mon}, nil
}

// RegisterChangeCallback adapts netmon's ChangeFunc (which carries a
// *netmon.ChangeDelta) down to the source-agnostic func() signature --
// HostnameDetector only needs "something changed", not the delta detail.
func (s *tailscaleNetmonSource) RegisterChangeCallback(fn func()) (unregister func()) {
	return s.mon.RegisterChangeCallback(func(*netmon.ChangeDelta) { fn() })
}

// Close releases the monitor and its eventbus in that order -- the bus must
// outlive the monitor's own shutdown (Monitor.Close waits for its internal
// goroutines, which are bus subscribers, to exit) or Close could race a
// still-running bus dispatch against the bus's own teardown.
func (s *tailscaleNetmonSource) Close() error {
	err := s.mon.Close()
	s.bus.Close()
	return err
}

// setupNetworkChangeEvents constructs the OS-level network-change source
// (Epic 3.1) and wires it into a coalescing events channel suitable for
// HostnameDetectorConfig.Events. newSource is injected (rather than calling
// newTailscaleNetmonSource directly) so tests can substitute a fake or a
// forced error without touching the real netmon/eventbus machinery.
//
// A construction error is logged and degraded to timer-only (Task 3.1.2b):
// the returned events channel is still valid, just never written to, so
// HostnameDetector.Run keeps working off its timer/manual triggers alone --
// this must never fail process startup, since the periodic timer alone
// already satisfies the feature's core requirement.
func setupNetworkChangeEvents(newSource func() (NetworkChangeSource, error)) (events chan struct{}, cleanup func()) {
	events = make(chan struct{}, 1)

	src, err := newSource()
	if err != nil {
		log.Warn("hostname-detect: network-change monitor unavailable, falling back to timer-only redetection", "err", err)
		return events, func() {}
	}

	unregister := src.RegisterChangeCallback(func() {
		// Non-blocking send: coalesces further at the channel level in
		// case netmon's own debounce window still bursts, and never
		// blocks the monitor's own callback goroutine.
		select {
		case events <- struct{}{}:
		default:
		}
	})

	return events, func() {
		unregister()
		if closeErr := src.Close(); closeErr != nil {
			log.Warn("hostname-detect: failed to close network-change monitor", "err", closeErr)
		}
	}
}
