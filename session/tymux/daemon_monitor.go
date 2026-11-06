package tymux

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/log"
)

type daemonMonitor struct {
	interval, maxBackoff time.Duration
	threshold            int
	needed               func() bool
	healthy              func(context.Context, DaemonConfig) bool
	tcpProbe             func(context.Context, DaemonConfig) (daemonProbeResult, bool) // nil: no TCP-liveness signal; false: inconclusive
	probe                func(context.Context, DaemonConfig) daemonProbeResult         // preferred over healthy when set; lets the monitor tell Busy from Healthy
	ensure               func(context.Context, DaemonConfig) (TymuxdReady, error)
	onSpawn              func()
	onWedged             func() // test hook, called after each wedged report
}

// daemonMonitorThreshold is how many consecutive failed probes (5s apart, each up to ~15s) it
// takes before repair: restarting tymuxd kills every pane child, so only a sustained outage
// justifies it.
const daemonMonitorThreshold = 6

// daemonWedgedAfter is how long the daemon may be continuously Busy (accepts connections, never
// answers) with no Healthy probe before the monitor reports it as likely wedged. Report only: a
// busy daemon under load is indistinguishable from a wedged one, and killing it destroys every pane.
const daemonWedgedAfter = 10 * time.Minute

// daemonDegradedWarnEvery rate-limits the "control socket down, using the TCP fallback" warning:
// the state can last for hours, and the transport selector already logs each switch.
const daemonDegradedWarnEvery = 10 * time.Minute

// neededCacheTTL spares the persisted-session query the monitor's 5s tick would otherwise repeat.
const neededCacheTTL = 15 * time.Second

// MonitorDaemon repairs an unavailable daemon until ctx is cancelled. It does not revive
// session processes; existing streams retain their own reconnect and archival policies.
// onSpawn, if non-nil, runs once when the monitor first starts a daemon, so the caller can
// register that daemon's shutdown with its own lifecycle (the monitor never stops it itself:
// stopping before the sessions' own close hooks ran would make their KillSession hit a dead daemon).
func MonitorDaemon(ctx context.Context, cfg DaemonConfig, needed func() bool, onSpawn func()) {
	newDaemonMonitor(needed, onSpawn).run(ctx, cfg)
}

func newDaemonMonitor(needed func() bool, onSpawn func()) daemonMonitor {
	probe, tcpProbe := monitorProbes()
	return daemonMonitor{
		interval: 5 * time.Second, maxBackoff: time.Minute, threshold: daemonMonitorThreshold,
		probe: probe, tcpProbe: tcpProbe, ensure: EnsureDaemonRunning, needed: cachedFor(neededCacheTTL, time.Now, needed), onSpawn: onSpawn,
	}
}

// tcpLivenessProbeTimeout bounds the monitor's extra TCP probe so a hung daemon cannot stretch a tick.
var tcpLivenessProbeTimeout = 2 * time.Second

// tcpObservation is the TCP probe result probeDaemon already obtained on this tick.
type tcpObservation struct {
	result daemonProbeResult
	ok     bool
}

// monitorProbes returns the default daemon probe and TCP-liveness probe. They share the TCP result
// probeDaemon obtained, and are only ever called from the monitor's single run goroutine.
func monitorProbes() (probe func(context.Context, DaemonConfig) daemonProbeResult, tcp func(context.Context, DaemonConfig) (daemonProbeResult, bool)) {
	var last tcpObservation
	probe = func(ctx context.Context, cfg DaemonConfig) daemonProbeResult {
		result, tcpResult, probed := probeDaemonWithTCP(ctx, cfg)
		last = tcpObservation{result: tcpResult, ok: probed}
		return result
	}
	tcp = func(ctx context.Context, cfg DaemonConfig) (daemonProbeResult, bool) {
		if o := last; o.ok {
			last = tcpObservation{}
			return o.result, true
		}
		tctx, cancel := context.WithTimeout(ctx, tcpLivenessProbeTimeout)
		defer cancel()
		res := probeTCP(tctx, cfg)
		return res, tctx.Err() == nil // our own deadline says nothing about the daemon
	}
	return probe, tcp
}

// cachedFor memoizes fn for ttl. A nil fn stays nil.
func cachedFor(ttl time.Duration, now func() time.Time, fn func() bool) func() bool {
	if fn == nil {
		return nil
	}
	var mu sync.Mutex
	var at time.Time
	var val bool
	return func() bool {
		mu.Lock()
		defer mu.Unlock()
		if t := now(); at.IsZero() || t.Sub(at) >= ttl {
			val, at = fn(), t
		}
		return val
	}
}

// check probes the daemon; healthy-only callers (tests) map to Healthy or NotListening.
func (m daemonMonitor) check(ctx context.Context, cfg DaemonConfig) daemonProbeResult {
	if m.probe != nil {
		return m.probe(ctx, cfg)
	}
	if m.healthy(ctx, cfg) {
		return daemonProbeHealthy
	}
	return daemonProbeNotListening
}

// wedgeTracker reports a daemon that has been Busy, never Healthy, for daemonWedgedAfter. It warns
// at most once per daemonWedgedAfter while the streak lasts.
type wedgeTracker struct {
	busySince, lastReport time.Time
}

// observe reports whether this probe emitted a wedged report.
func (w *wedgeTracker) observe(ctx context.Context, cfg DaemonConfig, result daemonProbeResult, now time.Time) bool {
	if result != daemonProbeBusy {
		*w = wedgeTracker{}
		return false
	}
	if w.busySince.IsZero() {
		w.busySince = now
	}
	if now.Sub(w.busySince) < daemonWedgedAfter || (!w.lastReport.IsZero() && now.Sub(w.lastReport) < daemonWedgedAfter) {
		return false
	}
	w.lastReport = now
	daemonWedgedCounter.Add(ctx, 1)
	log.Warn("[tymux] daemon accepts connections but has not answered a probe for a long time; it may be wedged (not restarting it automatically)",
		"addr", cfg.Addr, "busy_for", now.Sub(w.busySince).Round(time.Second))
	return true
}

// degradedTracker publishes the degraded gauges and rate-limits the warning for one monitor.
type degradedTracker struct {
	since, lastWarn time.Time
}

// observe records the current state and reports whether it emitted the warning.
func (d *degradedTracker) observe(ctx context.Context, cfg DaemonConfig, degraded bool, now time.Time) bool {
	if !degraded {
		*d = degradedTracker{}
		recordDaemonDegraded(ctx, false, 0)
		return false
	}
	if d.since.IsZero() {
		d.since = now
	}
	recordDaemonDegraded(ctx, true, now.Sub(d.since))
	if !d.lastWarn.IsZero() && now.Sub(d.lastWarn) < daemonDegradedWarnEvery {
		return false
	}
	d.lastWarn = now
	log.Warn("[tymux] degraded: tymuxd Unix control socket is not answering, sessions are using its deprecated unauthenticated TCP listener; the daemon is left running",
		"addr", cfg.Addr, "degraded_for", now.Sub(d.since).Round(time.Second))
	return true
}

// tcpListenerTracker publishes tymux_tcp_listener_up and rate-limits the warning. TCP liveness
// never affects daemon health (squad uses the socket); it only tells the operator that other
// TCP-only clients of this daemon are about to fail.
type tcpListenerTracker struct {
	down     bool
	lastWarn time.Time
}

// observe records whether the TCP listener answers and reports whether it emitted the warning.
func (t *tcpListenerTracker) observe(ctx context.Context, cfg DaemonConfig, up bool, now time.Time) bool {
	recordTCPListenerUp(ctx, up)
	if up {
		*t = tcpListenerTracker{}
		return false
	}
	if t.down && now.Sub(t.lastWarn) < daemonDegradedWarnEvery {
		return false
	}
	t.down, t.lastWarn = true, now
	log.Warn("[tymux] tymuxd TCP listener is down; other TCP-only clients of this daemon will fail (squad itself uses the Unix control socket)", "addr", cfg.Addr)
	return true
}

// observeTCP updates the TCP-liveness signal for a tick whose daemon probe returned result. A
// Degraded result already proves TCP answers; a dead daemon is the health gauge's business. Only a
// definite refusal is "down": a busy, forbidden or odd TCP listener is alive, so the gauge and the
// warning are left alone.
func (m daemonMonitor) observeTCP(ctx context.Context, cfg DaemonConfig, result daemonProbeResult, t *tcpListenerTracker) {
	if result == daemonProbeDegraded {
		t.observe(ctx, cfg, true, time.Now())
		return
	}
	if result < daemonProbeBusy || m.tcpProbe == nil {
		return
	}
	switch tcp, ok := m.tcpProbe(ctx, cfg); {
	case !ok:
	case tcp == daemonProbeHealthy:
		t.observe(ctx, cfg, true, time.Now())
	case tcp == daemonProbeNotListening:
		t.observe(ctx, cfg, false, time.Now())
	}
}

func (m daemonMonitor) run(ctx context.Context, cfg DaemonConfig) {
	owned := false
	var wedge wedgeTracker
	var degraded degradedTracker
	var tcp tcpListenerTracker
	defer recordDaemonDegraded(context.Background(), false, 0) // never leave a stale 1 behind
	failures := 0
	delay := m.interval
	for {
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if ctx.Err() != nil {
			return
		}
		if m.needed != nil && !m.needed() {
			degraded.observe(ctx, cfg, false, time.Now())
			failures = 0
			delay = m.interval
			continue
		}
		result := m.check(ctx, cfg)
		if ctx.Err() != nil {
			return
		}
		if wedge.observe(ctx, cfg, result, time.Now()) && m.onWedged != nil {
			m.onWedged()
		}
		degraded.observe(ctx, cfg, result == daemonProbeDegraded, time.Now())
		m.observeTCP(ctx, cfg, result, &tcp)
		healthy := result >= daemonProbeBusy
		if healthy {
			if failures > 0 {
				log.Info("[tymux] daemon health recovered", "addr", cfg.Addr, "consecutive_failures", failures)
			}
			failures = 0
			delay = m.interval
			recordDaemonHealth(ctx, true, failures)
			continue
		}
		failures++
		recordDaemonHealth(ctx, false, failures)
		if failures < m.threshold {
			continue
		}
		log.Warn("[tymux] daemon unhealthy, attempting recovery", "addr", cfg.Addr, "consecutive_failures", failures)
		recoverCtx := withStopReason(ctx, StopReason{Kind: StopMonitorRecovery, Detail: fmt.Sprintf("%d consecutive failed probes (threshold %d)", failures, m.threshold)})
		ready, err := m.ensure(recoverCtx, cfg)
		if ready.Spawned && !owned {
			owned = true
			if m.onSpawn != nil {
				m.onSpawn()
			}
		}
		if ctx.Err() != nil {
			return
		}
		outcome := "ok"
		if err != nil {
			outcome = "failed"
		}
		daemonRecoveryCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", outcome)))
		if err == nil {
			log.Info("[tymux] daemon recovery succeeded", "addr", cfg.Addr, "consecutive_failures", failures)
			failures = 0
			delay = m.interval
			recordDaemonHealth(ctx, true, 0)
			continue
		}
		delay = min(delay*2, m.maxBackoff)
		log.Warn("[tymux] daemon recovery failed", "addr", cfg.Addr, "err", err, "retry_in", delay)
	}
}
