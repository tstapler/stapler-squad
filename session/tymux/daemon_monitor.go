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
	ensure               func(context.Context, DaemonConfig) (TymuxdReady, error)
	onSpawn              func()
}

// daemonMonitorThreshold is how many consecutive failed probes (5s apart, each up to ~15s) it
// takes before repair: restarting tymuxd kills every pane child, so only a sustained outage
// justifies it.
const daemonMonitorThreshold = 6

// neededCacheTTL spares the persisted-session query the monitor's 5s tick would otherwise repeat.
const neededCacheTTL = 15 * time.Second

// MonitorDaemon repairs an unavailable daemon until ctx is cancelled. It does not revive
// session processes; existing streams retain their own reconnect and archival policies.
// onSpawn, if non-nil, runs once when the monitor first starts a daemon, so the caller can
// register that daemon's shutdown with its own lifecycle (the monitor never stops it itself:
// stopping before the sessions' own close hooks ran would make their KillSession hit a dead daemon).
func MonitorDaemon(ctx context.Context, cfg DaemonConfig, needed func() bool, onSpawn func()) {
	m := daemonMonitor{
		interval: 5 * time.Second, maxBackoff: time.Minute, threshold: daemonMonitorThreshold,
		healthy: checkDaemonHealthy, ensure: EnsureDaemonRunning, needed: cachedFor(neededCacheTTL, time.Now, needed), onSpawn: onSpawn,
	}
	m.run(ctx, cfg)
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

func (m daemonMonitor) run(ctx context.Context, cfg DaemonConfig) {
	owned := false
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
			failures = 0
			delay = m.interval
			continue
		}
		healthy := m.healthy(ctx, cfg)
		if ctx.Err() != nil {
			return
		}
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
