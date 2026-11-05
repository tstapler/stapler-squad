package tymux

import (
	"context"
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
	probe                func(context.Context, DaemonConfig) daemonProbeResult // preferred over healthy when set; lets the monitor tell Busy from Healthy
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
	return daemonMonitor{
		interval: 5 * time.Second, maxBackoff: time.Minute, threshold: daemonMonitorThreshold,
		probe: probeDaemon, ensure: EnsureDaemonRunning, needed: cachedFor(neededCacheTTL, time.Now, needed), onSpawn: onSpawn,
	}
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

func (m daemonMonitor) run(ctx context.Context, cfg DaemonConfig) {
	owned := false
	var wedge wedgeTracker
	var degraded degradedTracker
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
		ready, err := m.ensure(ctx, cfg)
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
