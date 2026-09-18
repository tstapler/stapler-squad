package main

import (
	"context"
	"sort"
	"time"

	"github.com/tstapler/stapler-squad/server"
	serverauth "github.com/tstapler/stapler-squad/server/auth"
)

// TriggerSource identifies what caused a HostnameDetector cycle to run.
type TriggerSource string

const (
	TriggerStartup       TriggerSource = "startup"
	TriggerTimer         TriggerSource = "timer"
	TriggerNetworkChange TriggerSource = "network-change"
	TriggerManual        TriggerSource = "manual"
)

// redetectCycle summarizes one HostnameDetector.redetect run for logging
// (Epic 4.1) and the manual-trigger endpoint's response body (Epic 4.2).
type redetectCycle struct {
	Trigger   TriggerSource
	Duration  time.Duration
	PrevCount int
	NewCount  int
	Added     []string
}

// HostnameDetectorConfig is HostnameDetector's constructor input. It exists
// as its own struct (rather than a long positional-parameter list) so that
// Epic 2.2's security-mandatory validation gate can add WAHandler/CertStore/
// ValidateFn without changing every existing call site's parameter order --
// see project_plans/network-hostname-redetect/implementation/plan.md Task
// 2.2.1b. This epic (2.1) leaves those three fields nil/unset: redetect only
// calls Srv.SetHostnames, never RegisterHostname or a TLS cert update.
type HostnameDetectorConfig struct {
	Srv             *server.Server
	InitialNetworks map[string][]string
	Tick            <-chan time.Time
	Events          <-chan struct{}

	// WAHandler, CertStore, and ValidateFn are wired by Epic 2.2. Until then
	// they are nil, and redetect does not use them.
	WAHandler  *serverauth.Handler
	CertStore  *server.NetworkCertStore
	ValidateFn func(string) bool
}

// HostnameDetector periodically (re)detects this host's LAN IPs and their
// resolvable hostnames, publishing the result to Server.hostnames so a
// process that outlives a network change (Wi-Fi switch, VPN toggle) doesn't
// need to restart to pick up its new address. See plan.md Epic 2.1 for the
// full design; Epic 2.2 adds RPID/TLS-SAN publication behind a validation
// gate on top of the loop implemented here.
type HostnameDetector struct {
	srv *server.Server

	// detectFn/resolveFn wrap the real detectLANIPs/resolveLANHostnames
	// package functions by default; tests inject fakes so Run/redetect are
	// unit-testable with no real subprocess/DNS calls.
	detectFn  func(context.Context) []string
	resolveFn func(context.Context, string) []string

	// cycleTimeout bounds each redetect call so one hung subprocess/DNS
	// call cannot stall Run's single-goroutine select loop indefinitely
	// (pre-mortem.md P1).
	cycleTimeout time.Duration

	// networks maps a detected IP to the hostnames known to resolve there.
	// Only Run's own goroutine ever reads or mutates this -- see manual's
	// doc comment.
	networks map[string][]string

	tick   <-chan time.Time
	events <-chan struct{}

	// manual is the sole channel through which a caller outside Run's own
	// goroutine (e.g. the manual-trigger HTTP handler, Task 4.2.2a) may
	// request a cycle. Run receives on it and calls redetect itself, so
	// every mutation of the unsynchronized networks map stays serialized
	// inside Run's single select loop.
	manual chan chan redetectCycle

	// done is closed when Run returns, letting tests join on shutdown
	// without a wall-clock sleep.
	done chan struct{}

	// waHandler, certStore, and validateFn are unused by this epic's
	// redetect -- Epic 2.2 wires and consumes them for RPID/TLS-SAN
	// publication behind a validation gate.
	waHandler  *serverauth.Handler
	certStore  *server.NetworkCertStore
	validateFn func(string) bool
}

// NewHostnameDetector builds a HostnameDetector wired to the real
// detectLANIPs/resolveLANHostnames package functions and a 30s per-cycle
// timeout. cfg.InitialNetworks is copied, not aliased, so a caller mutating
// its own map afterward cannot race with Run's goroutine.
func NewHostnameDetector(cfg HostnameDetectorConfig) *HostnameDetector {
	networks := make(map[string][]string, len(cfg.InitialNetworks))
	for ip, hostnames := range cfg.InitialNetworks {
		networks[ip] = append([]string(nil), hostnames...)
	}

	return &HostnameDetector{
		srv:          cfg.Srv,
		detectFn:     detectLANIPs,
		resolveFn:    resolveLANHostnames,
		cycleTimeout: 30 * time.Second,
		networks:     networks,
		tick:         cfg.Tick,
		events:       cfg.Events,
		manual:       make(chan chan redetectCycle),
		done:         make(chan struct{}),
		waHandler:    cfg.WAHandler,
		certStore:    cfg.CertStore,
		validateFn:   cfg.ValidateFn,
	}
}

// Run performs one detection cycle immediately (TriggerStartup), then loops
// until ctx is cancelled, running another cycle on every tick, network-change
// event, or manual-trigger request. It closes d.done on return so callers
// (tests, graceful shutdown) can join without a wall-clock wait.
func (d *HostnameDetector) Run(ctx context.Context) {
	defer close(d.done)

	d.redetect(ctx, TriggerStartup)

	for {
		select {
		case <-ctx.Done():
			return
		case <-d.tick:
			d.redetect(ctx, TriggerTimer)
		case <-d.events:
			d.redetect(ctx, TriggerNetworkChange)
		case respCh := <-d.manual:
			respCh <- d.redetect(ctx, TriggerManual)
		}
	}
}

// redetect runs one bounded detection cycle: it re-detects LAN IPs, re-
// resolves every currently-known IP (not only newly-seen ones, so a
// transient resolution failure on first sighting can never permanently
// exclude that IP -- see plan.md Story 2.1.1's acceptance criteria),
// add-only-merges any newly-found hostnames into d.networks, and publishes
// the flattened result via d.srv.SetHostnames. It does not call
// RegisterHostname or touch TLS certs -- that gate is Epic 2.2's job.
//
// The cycle is bounded by d.cycleTimeout so a hung detectFn/resolveFn call
// (e.g. a stuck scutil/avahi-resolve/hostname subprocess) cannot stall Run's
// single-goroutine select loop past that deadline; a timed-out cycle still
// returns whatever partial result it collected, and the next tick/event
// naturally retries.
func (d *HostnameDetector) redetect(ctx context.Context, trigger TriggerSource) redetectCycle {
	start := time.Now()
	cycleCtx, cancel := context.WithTimeout(ctx, d.cycleTimeout)
	defer cancel()

	prevCount := d.flattenedHostnameCount()

	ips := d.detectFn(cycleCtx)
	added := make(map[string]struct{})
	for _, ip := range ips {
		if _, known := d.networks[ip]; !known {
			d.networks[ip] = nil
		}
	}
	for ip := range d.networks {
		for _, hostname := range d.resolveFn(cycleCtx, ip) {
			if !containsString(d.networks[ip], hostname) {
				d.networks[ip] = append(d.networks[ip], hostname)
				added[hostname] = struct{}{}
			}
		}
	}

	flattened := d.flattenHostnames()
	d.srv.SetHostnames(flattened)

	addedList := make([]string, 0, len(added))
	for hostname := range added {
		addedList = append(addedList, hostname)
	}
	sort.Strings(addedList)

	return redetectCycle{
		Trigger:   trigger,
		Duration:  time.Since(start),
		PrevCount: prevCount,
		NewCount:  len(flattened),
		Added:     addedList,
	}
}

// flattenHostnames returns the deduplicated union of every hostname across
// every IP in d.networks, sorted for deterministic SetHostnames input.
func (d *HostnameDetector) flattenHostnames() []string {
	seen := make(map[string]struct{})
	var flattened []string
	for _, hostnames := range d.networks {
		for _, hostname := range hostnames {
			if _, ok := seen[hostname]; !ok {
				seen[hostname] = struct{}{}
				flattened = append(flattened, hostname)
			}
		}
	}
	sort.Strings(flattened)
	return flattened
}

func (d *HostnameDetector) flattenedHostnameCount() int {
	return len(d.flattenHostnames())
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
