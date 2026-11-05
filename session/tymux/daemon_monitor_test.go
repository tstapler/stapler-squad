package tymux

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
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

func TestDaemonMonitor_ClaimsOwnershipOnlyOfDaemonItSpawned(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		ready TymuxdReady
		err   error
		want  int64
	}{
		{"reused", TymuxdReady{}, nil, 0},
		{"spawned", TymuxdReady{Spawned: true}, nil, 1},
		{"spawned then failed", TymuxdReady{Spawned: true}, errors.New("never became healthy"), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				var claims atomic.Int64
				m := daemonMonitor{interval: time.Second, maxBackoff: time.Second, threshold: 1,
					healthy: func(context.Context, DaemonConfig) bool { return false },
					ensure:  func(context.Context, DaemonConfig) (TymuxdReady, error) { return tc.ready, tc.err },
					onSpawn: func() { claims.Add(1) },
				}
				done := startDaemonMonitorTest(ctx, m)
				for range 3 {
					time.Sleep(time.Second) //nolint:notimesleeptest synctest advances virtual time, not wall-clock time
					synctest.Wait()
				}
				cancel()
				<-done
				require.Equal(t, tc.want, claims.Load(), "ownership is claimed once, as soon as a spawn happened")
			})
		})
	}
}

func TestCachedFor_QueriesOncePerTTL(t *testing.T) {
	t.Parallel()
	now := time.Unix(0, 0)
	var calls int
	f := cachedFor(10*time.Second, func() time.Time { return now }, func() bool { calls++; return calls > 1 })

	require.False(t, f())
	now = now.Add(9 * time.Second)
	require.False(t, f(), "within the TTL the cached answer is reused")
	require.Equal(t, 1, calls)
	now = now.Add(time.Second)
	require.True(t, f())
	require.Equal(t, 2, calls)
	require.Nil(t, cachedFor(time.Second, time.Now, nil))
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

func TestCheckDaemonHealthy_ReleasesConnections(t *testing.T) {
	t.Parallel()
	var open atomic.Int32
	// An empty, successful gRPC ListSessions reply, so the client connection ends up idle
	// (a protocol error would make the client close it and hide the leak).
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/grpc")
		w.Header().Set("Trailer", "Grpc-Status")
		_, _ = w.Write([]byte{0, 0, 0, 0, 0})
		w.Header().Set("Grpc-Status", "0")
	}))
	srv.Config.Protocols = new(http.Protocols)
	srv.Config.Protocols.SetUnencryptedHTTP2(true)
	srv.Listener = &countingListener{Listener: srv.Listener, open: &open} // h2c hijacks conns, so ConnState can't count them
	srv.Start()
	t.Cleanup(srv.Close)

	cfg := DaemonConfig{Addr: srv.URL}
	for range 5 {
		require.True(t, checkDaemonHealthy(context.Background(), cfg))
	}
	require.Eventually(t, func() bool { return open.Load() == 0 }, 2*time.Second, 10*time.Millisecond,
		"probes must not leave connections open on the daemon")
}

type countingListener struct {
	net.Listener
	open *atomic.Int32
}

func (l *countingListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	l.open.Add(1)
	return &countingConn{Conn: c, open: l.open}, nil
}

type countingConn struct {
	net.Conn
	open *atomic.Int32
	once sync.Once
}

func (c *countingConn) Close() error {
	c.once.Do(func() { c.open.Add(-1) })
	return c.Conn.Close()
}
