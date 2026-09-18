package main

import (
	"context"
	"sort"
	"time"

	"github.com/tstapler/stapler-squad/log"
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
// as its own struct (rather than a long positional-parameter list) per this
// repo's primitive-obsession-checklist guidance for >3 same-typed/optional
// params -- see project_plans/network-hostname-redetect/implementation/plan.md
// Task 2.2.1b.
type HostnameDetectorConfig struct {
	Srv             *server.Server
	InitialNetworks map[string][]string
	Tick            <-chan time.Time
	Events          <-chan struct{}

	// WAHandler and CertStore gate Epic 2.2's RPID/TLS-SAN publication. Both
	// are nil-safe: leaving either nil (remote access disabled) keeps
	// redetect updating only Server.hostnames, per Story 2.2.1's third
	// acceptance criterion.
	WAHandler *serverauth.Handler
	CertStore *server.NetworkCertStore
	// ValidateFn defaults to verifyHostnameOwnership (set by
	// NewHostnameDetector) when left nil -- the security gate must never be
	// silently disabled just because a caller forgot to wire it.
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

	// waHandler and certStore gate RPID/TLS-SAN publication; both are
	// nil-safe (see HostnameDetectorConfig). validateFn is the security
	// gate itself -- redetect never merges a hostname into networks (and
	// therefore never reaches waHandler/certPublisher) without it passing
	// validateFn first.
	waHandler  *serverauth.Handler
	certStore  *server.NetworkCertStore
	validateFn func(string) bool

	// certPublisher wraps server.EnsureNetworkTLSCerts by default; tests
	// inject a fake to simulate issuance failure or to count invocations
	// without touching disk.
	certPublisher func(map[string][]string) (string, map[string]*server.NetworkCert, error)
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

	validateFn := cfg.ValidateFn
	if validateFn == nil {
		validateFn = verifyHostnameOwnership
	}

	return &HostnameDetector{
		srv:           cfg.Srv,
		detectFn:      detectLANIPs,
		resolveFn:     resolveLANHostnames,
		cycleTimeout:  30 * time.Second,
		networks:      networks,
		tick:          cfg.Tick,
		events:        cfg.Events,
		manual:        make(chan chan redetectCycle),
		done:          make(chan struct{}),
		waHandler:     cfg.WAHandler,
		certStore:     cfg.CertStore,
		validateFn:    validateFn,
		certPublisher: server.EnsureNetworkTLSCerts,
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
// exclude that IP -- see plan.md Story 2.1.1's acceptance criteria), and
// add-only-merges any newly-found hostname into d.networks only after it
// passes d.validateFn -- forward-DNS proof the hostname actually resolves
// to this host's own IP, per Story 2.2.1's security-mandatory gate. A
// hostname that fails validation is logged and dropped: it is not added to
// d.networks (so it never reaches waHandler/certPublisher) but does still
// reach Server.hostnames as internal bookkeeping, since Server.hostnames is
// itself an add-only union and carries no trust implication on its own.
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
	for _, ip := range ips {
		if _, known := d.networks[ip]; !known {
			d.networks[ip] = nil
		}
	}

	verifiedNew, unverifiedThisCycle := d.resolveAndValidate(cycleCtx)

	// Server.hostnames is bookkeeping only -- publish every hostname
	// discovered this cycle, verified or not, on top of the verified set
	// already folded into d.networks. SetHostnames' own add-only union
	// merge means an unverified hostname published here once stays in
	// Server.GetHostnames() forever, same as a verified one; it just never
	// reaches d.networks, waHandler, or certPublisher.
	flattened := d.flattenHostnames()
	if len(unverifiedThisCycle) > 0 {
		flattened = append(append([]string(nil), flattened...), unverifiedThisCycle...)
		sort.Strings(flattened)
	}
	d.srv.SetHostnames(flattened)

	if len(verifiedNew) > 0 {
		d.publishVerified(verifiedNew)
	}

	addedSet := make(map[string]struct{}, len(verifiedNew))
	for _, hostname := range verifiedNew {
		addedSet[hostname] = struct{}{}
	}
	addedList := make([]string, 0, len(addedSet))
	for hostname := range addedSet {
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

// resolveAndValidate re-resolves every currently-known IP in d.networks and
// splits the newly-discovered hostnames (not already recorded for their IP)
// into verified (added to d.networks) and unverified (dropped, logged, and
// excluded from d.networks so they are simply re-validated next cycle
// rather than permanently blacklisted) -- see plan.md Task 2.2.1a.
func (d *HostnameDetector) resolveAndValidate(cycleCtx context.Context) (verifiedNew, unverified []string) {
	for ip := range d.networks {
		for _, hostname := range d.resolveFn(cycleCtx, ip) {
			if containsString(d.networks[ip], hostname) {
				continue
			}
			if !d.validateFn(hostname) {
				log.Warn("hostname-detect: dropped unverified candidate", "hostname", hostname)
				unverified = append(unverified, hostname)
				continue
			}
			d.networks[ip] = append(d.networks[ip], hostname)
			verifiedNew = append(verifiedNew, hostname)
		}
	}
	return verifiedNew, unverified
}

// publishVerified registers each newly-verified hostname as a trusted RPID
// and (re)issues TLS certs covering the updated network SAN sets. Both
// steps are nil-safe -- a nil waHandler/certStore (remote access disabled)
// simply skips its half of publication, per Story 2.2.1's third acceptance
// criterion.
//
// Ordering is deliberate: RegisterHostname calls happen first and are never
// rolled back if the subsequent EnsureNetworkTLSCerts call fails. Unregistering
// an RPID a WebAuthn ceremony may already be relying on is a correctness
// regression with no auto-retry; a transient TLS-SAN gap self-heals on the
// next cycle via EnsureNetworkTLSCerts' own sanHash short-circuit. See
// plan.md Task 2.2.1a.
func (d *HostnameDetector) publishVerified(verifiedNew []string) {
	if d.waHandler != nil {
		for _, hostname := range verifiedNew {
			if err := d.waHandler.RegisterHostname(hostname); err != nil {
				log.Error("hostname-detect: RegisterHostname failed", "hostname", hostname, "err", err)
			}
		}
	}

	if d.certStore != nil {
		_, newCerts, err := d.certPublisher(d.networks)
		if err != nil {
			log.Error("hostname-detect: TLS cert issuance failed", "err", err)
			return
		}
		d.certStore.Store(newCerts)
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
