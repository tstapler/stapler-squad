package session

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// prMaintenanceRetryInterval is how often a failed startup pass is retried.
const prMaintenanceRetryInterval = 5 * time.Minute

// prMaintenanceBackoffs are the in-process retry delays after a failed pass.
var prMaintenanceBackoffs = []time.Duration{time.Second, 4 * time.Second, 16 * time.Second}

// prMaintenanceConfig holds the injectable seams of the maintenance pass.
type prMaintenanceConfig struct {
	sleep    func(ctx context.Context, d time.Duration) error
	newTick  func(d time.Duration) (<-chan time.Time, func())
	interval time.Duration
	hook     prBackfillHook
}

func defaultPRMaintenanceConfig() prMaintenanceConfig {
	return prMaintenanceConfig{
		sleep: func(ctx context.Context, d time.Duration) error {
			t := time.NewTimer(d)
			defer t.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-t.C:
				return nil
			}
		},
		newTick: func(d time.Duration) (<-chan time.Time, func()) {
			t := time.NewTicker(d)
			return t.C, t.Stop
		},
		interval: prMaintenanceRetryInterval,
	}
}

// prMaintenanceState is the Storage-owned lifecycle of the startup pass.
type prMaintenanceState struct {
	mu       sync.Mutex
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	failures atomic.Int64
	started  atomic.Bool
}

// RunPRStartupMaintenance imports legacy single-PR references as links, then
// resets the verification counters of unverified-terminal rows, then marks the
// counters trusted. It runs from the service bootstrap only (BuildOptions
// .RunPRMaintenance), never from NewEntRepository or the MCP-fallback deps.
//
// It never fails the boot: a pass that fails is retried 3 times with backoff,
// then logged and counted, and a 5-minute ticker retries until one pass
// succeeds. Until then unverified-terminal rows read as non-terminal. The ticker
// is bound to ctx and to StopPRMaintenance, which waits for its goroutine.
func (s *Storage) RunPRStartupMaintenance(ctx context.Context) error {
	s.runPRStartupMaintenance(ctx, defaultPRMaintenanceConfig())
	return nil
}

func (s *Storage) runPRStartupMaintenance(ctx context.Context, cfg prMaintenanceConfig) {
	m := &s.prMaintenance
	if !m.started.CompareAndSwap(false, true) {
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	m.mu.Lock()
	m.cancel = cancel
	m.mu.Unlock()

	if s.runPRMaintenancePassWithRetries(ctx, cfg) {
		return
	}
	m.failures.Add(1)
	log.Error("session pr startup maintenance failed; retrying in the background", "interval", cfg.interval)
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		s.retryPRMaintenance(ctx, cfg)
	}()
}

// StopPRMaintenance cancels the retry ticker and waits for its goroutine to exit.
// It is safe to call when maintenance never ran, and more than once.
func (s *Storage) StopPRMaintenance() {
	m := &s.prMaintenance
	m.mu.Lock()
	cancel := m.cancel
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	m.wg.Wait()
}

// PRMaintenanceFailures is the ssq_pr_maintenance_failed_total counter: how many
// startup passes exhausted their in-process retries.
func (s *Storage) PRMaintenanceFailures() int64 { return s.prMaintenance.failures.Load() }

// runPRMaintenancePassWithRetries tries the pass once plus one retry per backoff.
func (s *Storage) runPRMaintenancePassWithRetries(ctx context.Context, cfg prMaintenanceConfig) bool {
	for attempt := 0; ; attempt++ {
		err := s.runPRMaintenancePass(ctx, cfg)
		if err == nil {
			return true
		}
		log.Warn("session pr startup maintenance pass failed", "attempt", attempt+1, "error", err)
		if attempt >= len(prMaintenanceBackoffs) || ctx.Err() != nil {
			return false
		}
		if cfg.sleep(ctx, prMaintenanceBackoffs[attempt]) != nil {
			return false
		}
	}
}

func (s *Storage) runPRMaintenancePass(ctx context.Context, cfg prMaintenanceConfig) error {
	if _, err := s.repo.runPRBackfill(ctx, cfg.hook); err != nil {
		return err
	}
	if _, err := s.repo.ResetVerifyCounters(ctx); err != nil {
		return err
	}
	s.repo.MarkPRMaintenanceSucceeded()
	return nil
}

func (s *Storage) retryPRMaintenance(ctx context.Context, cfg prMaintenanceConfig) {
	ticks, stop := cfg.newTick(cfg.interval)
	defer stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticks:
			if err := s.runPRMaintenancePass(ctx, cfg); err != nil {
				log.Warn("session pr maintenance retry failed", "error", err)
				continue
			}
			log.Info("session pr maintenance retry succeeded; stopping the retry ticker")
			return
		}
	}
}
