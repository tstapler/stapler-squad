package tymux

import (
	"context"
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
	stop                 func() error
}

// MonitorDaemon repairs an unavailable daemon until ctx is cancelled. It does not revive
// session processes; existing streams retain their own reconnect and archival policies.
func MonitorDaemon(ctx context.Context, cfg DaemonConfig, needed func() bool, stop func() error) {
	m := daemonMonitor{
		interval: 5 * time.Second, maxBackoff: time.Minute, threshold: 3,
		healthy: checkDaemonHealthy, ensure: EnsureDaemonRunning, needed: needed, stop: stop,
	}
	m.run(ctx, cfg)
}

func (m daemonMonitor) run(ctx context.Context, cfg DaemonConfig) {
	spawned := false
	defer func() {
		if spawned && m.stop != nil {
			if err := m.stop(); err != nil {
				log.Warn("[tymux] failed to stop supervised daemon", "addr", cfg.Addr, "err", err)
			}
		}
	}()
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
		spawned = spawned || ready.Spawned
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
