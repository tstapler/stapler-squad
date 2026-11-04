package tymux

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

func startDaemonMonitorTest(ctx context.Context, m daemonMonitor) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		m.run(ctx, DaemonConfig{})
	}()
	synctest.Wait()
	return done
}

func TestDaemonMonitor_StopsOnlyDaemonItSpawned(t *testing.T) {
	t.Run("reused", func(t *testing.T) { checkDaemonMonitorStopOwnership(t, TymuxdReady{}, 0) })
	t.Run("spawned", func(t *testing.T) { checkDaemonMonitorStopOwnership(t, TymuxdReady{Spawned: true}, 1) })
}

func checkDaemonMonitorStopOwnership(t *testing.T, ready TymuxdReady, expectedStops int64) {
	t.Helper()
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var stops atomic.Int64
		m := daemonMonitor{interval: time.Second, maxBackoff: time.Second, threshold: 1,
			healthy: func(context.Context, DaemonConfig) bool { return false },
			ensure:  func(context.Context, DaemonConfig) (TymuxdReady, error) { return ready, nil },
			stop:    func() error { stops.Add(1); return nil },
		}
		done := startDaemonMonitorTest(ctx, m)
		time.Sleep(time.Second) //nolint:notimesleeptest synctest advances virtual time, not wall-clock time
		synctest.Wait()
		cancel()
		<-done
		require.Equal(t, expectedStops, stops.Load())
	})
}

func TestDaemonMonitor_DisabledDoesNotProbeOrRepair(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var enabled atomic.Bool
		var probes atomic.Int64
		m := daemonMonitor{interval: time.Second, maxBackoff: 4 * time.Second, threshold: 1,
			needed:  enabled.Load,
			healthy: func(context.Context, DaemonConfig) bool { probes.Add(1); return true },
			ensure: func(context.Context, DaemonConfig) (TymuxdReady, error) {
				t.Error("unexpected repair")
				return TymuxdReady{}, nil
			},
		}
		done := startDaemonMonitorTest(ctx, m)
		time.Sleep(time.Second) //nolint:notimesleeptest synctest advances virtual time, not wall-clock time
		synctest.Wait()
		require.Zero(t, probes.Load())
		enabled.Store(true)
		time.Sleep(time.Second) //nolint:notimesleeptest synctest advances virtual time, not wall-clock time
		synctest.Wait()
		require.EqualValues(t, 1, probes.Load())
		enabled.Store(false)
		time.Sleep(time.Second) //nolint:notimesleeptest synctest advances virtual time, not wall-clock time
		synctest.Wait()
		require.EqualValues(t, 1, probes.Load())
		cancel()
		<-done
	})
}

func TestDaemonMonitor_RecoversAfterThresholdAndKeepsChecking(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var probes, repairs atomic.Int64
		var healthy atomic.Bool
		m := daemonMonitor{interval: time.Second, maxBackoff: 4 * time.Second, threshold: 2,
			healthy: func(context.Context, DaemonConfig) bool { probes.Add(1); return healthy.Load() },
			ensure: func(context.Context, DaemonConfig) (TymuxdReady, error) {
				repairs.Add(1)
				healthy.Store(true)
				return TymuxdReady{Spawned: true}, nil
			},
		}
		done := startDaemonMonitorTest(ctx, m)
		time.Sleep(time.Second) //nolint:notimesleeptest synctest advances virtual time, not wall-clock time
		synctest.Wait()
		require.Zero(t, repairs.Load())
		time.Sleep(time.Second) //nolint:notimesleeptest synctest advances virtual time, not wall-clock time
		synctest.Wait()
		require.EqualValues(t, 1, repairs.Load())
		time.Sleep(time.Second) //nolint:notimesleeptest synctest advances virtual time, not wall-clock time
		synctest.Wait()
		require.EqualValues(t, 3, probes.Load())
		cancel()
		<-done
	})
}

func TestDaemonMonitor_RetriesWithCappedBackoffAndCancels(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var mu sync.Mutex
		var attempts []time.Duration
		start := time.Now()
		m := daemonMonitor{interval: time.Second, maxBackoff: 4 * time.Second, threshold: 1,
			healthy: func(context.Context, DaemonConfig) bool { return false },
			ensure: func(context.Context, DaemonConfig) (TymuxdReady, error) {
				mu.Lock()
				attempts = append(attempts, time.Since(start))
				mu.Unlock()
				return TymuxdReady{}, errors.New("unavailable")
			},
		}
		done := startDaemonMonitorTest(ctx, m)
		for range 11 {
			time.Sleep(time.Second) //nolint:notimesleeptest synctest advances virtual time, not wall-clock time
			synctest.Wait()
		}
		mu.Lock()
		require.Equal(t, []time.Duration{time.Second, 3 * time.Second, 7 * time.Second, 11 * time.Second}, attempts)
		mu.Unlock()
		cancel()
		<-done
		time.Sleep(10 * time.Second) //nolint:notimesleeptest synctest advances virtual time, not wall-clock time
		require.Len(t, attempts, 4)
	})
}

func TestDaemonMonitor_HealthyProbeResetsFailureThreshold(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var probes, repairs atomic.Int64
		m := daemonMonitor{interval: time.Second, maxBackoff: 4 * time.Second, threshold: 2,
			healthy: func(context.Context, DaemonConfig) bool { return probes.Add(1) == 2 },
			ensure:  func(context.Context, DaemonConfig) (TymuxdReady, error) { repairs.Add(1); return TymuxdReady{}, nil },
		}
		done := startDaemonMonitorTest(ctx, m)
		for range 3 {
			time.Sleep(time.Second) //nolint:notimesleeptest synctest advances virtual time, not wall-clock time
			synctest.Wait()
		}
		require.Zero(t, repairs.Load())
		time.Sleep(time.Second) //nolint:notimesleeptest synctest advances virtual time, not wall-clock time
		synctest.Wait()
		require.EqualValues(t, 1, repairs.Load())
		cancel()
		<-done
	})
}
