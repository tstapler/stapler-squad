package tymux

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func einval() error { return syscallDialErr(syscall.EINVAL) }
func eperm() error  { return syscallDialErr(syscall.EPERM) }

func TestSelectingTransport_UnixEPERM_IsHardErrorAndNeverFallsBack(t *testing.T) {
	t.Parallel()
	s, tcp, unix, _ := newTestSelector()
	unix.fail(eperm())

	err := list(t, s)

	require.ErrorIs(t, err, ErrTymuxdSocketAccessDenied)
	require.ErrorIs(t, err, syscall.EPERM)
	require.Zero(t, tcp.calls.Load())
	require.Equal(t, transportUnix, s.Active())
}

func TestSelectingTransport_ReprobeSeesEPERM_SwitchesBackToUnixSoTheErrorSurfaces(t *testing.T) {
	t.Parallel()
	s, tcp, unix, clock := newTestSelector()
	unix.fail(enoent())
	require.NoError(t, list(t, s))
	unix.fail(eperm())
	clock.advance(transportReprobeInterval)
	tcpCalls := tcp.calls.Load()

	require.ErrorIs(t, list(t, s), ErrTymuxdSocketAccessDenied)
	require.Equal(t, transportUnix, s.Active())
	require.Equal(t, tcpCalls, tcp.calls.Load())
}

// EINVAL is what connect() returns for a sun_path over the platform limit: the socket can never
// work, so it must fall back (loudly) instead of failing permanently with TCP never tried.
func TestSelectingTransport_UnixEINVAL_FallsBackToTCP(t *testing.T) {
	t.Parallel()
	s, tcp, unix, _ := newTestSelector()
	unix.fail(einval())

	require.NoError(t, list(t, s))
	require.Equal(t, transportTCP, s.Active())
	require.Positive(t, tcp.calls.Load())
}

func TestSelectingTransport_StickyOnTCP_OtherProbeErrorsWarnRateLimited(t *testing.T) {
	t.Parallel()
	s, _, unix, clock := newTestSelector()
	unix.fail(enoent())
	require.NoError(t, list(t, s))
	odd := syscallDialErr(syscall.ENOTDIR)

	require.True(t, s.warnProbeError(odd), "first odd probe error is logged")
	require.False(t, s.warnProbeError(odd), "then rate-limited")
	clock.advance(transportProbeWarnEvery)
	require.True(t, s.warnProbeError(odd))

	unix.fail(odd)
	clock.advance(transportReprobeInterval)
	require.NoError(t, list(t, s))
	require.Equal(t, transportTCP, s.Active(), "an odd probe error keeps the fallback, only louder")
}

func TestSelectingTransport_ReprobeIsShortBounded(t *testing.T) {
	t.Parallel()
	s, _, unix, clock := newTestSelector()
	unix.fail(enoent())
	require.NoError(t, list(t, s))
	clock.advance(transportReprobeInterval)

	require.NoError(t, list(t, s))

	require.Positive(t, unix.lastInfoTimeout.Load())
	require.LessOrEqual(t, unix.lastInfoTimeout.Load(), int64(500*time.Millisecond), "a caller never waits the full probe timeout for a re-probe")
}

func TestSelectingTransport_TCPRefusedWithSocketStillDown_DoesNotSwitchOrLie(t *testing.T) {
	t.Parallel()
	s, tcp, unix, _ := newTestSelector()
	unix.fail(enoent())
	require.NoError(t, list(t, s))
	require.Equal(t, transportTCP, s.Active())
	tcp.fail(refused())
	before := unix.calls.Load()

	require.Error(t, list(t, s))

	require.Equal(t, transportTCP, s.Active(), "switching to a socket that was never probed would log a transition that did not happen")
	require.Greater(t, unix.calls.Load(), before, "the socket is probed before any switch")
}

func TestSharedTransportFor_ConcurrentFirstCallersBuildOnce(t *testing.T) {
	t.Parallel()
	var built atomic.Int32
	key := fmt.Sprintf("shared-concurrent-%p", &built)
	got := make([]*selectingTransport, 32)
	var wg sync.WaitGroup
	for i := range got {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got[i] = sharedTransportFor(key, func() *selectingTransport {
				built.Add(1)
				s, _, _, _ := newTestSelector()
				return s
			})
		}()
	}
	wg.Wait()

	require.EqualValues(t, 1, built.Load(), "losers must not build (and leak) their own clients")
	for _, s := range got {
		require.Same(t, got[0], s)
	}
}

func TestNewRealTransport_KeyIncludesSocket(t *testing.T) {
	addr := closedAddr(t)
	sockA, sockB := missingSocketPath(t), missingSocketPath(t)
	t.Setenv("STAPLER_SQUAD_INSTANCE", "")
	t.Setenv("TYMUXD_ADDR", addr)
	t.Setenv("TYMUXD_SOCKET_PATH", sockA)
	a1, a2 := NewRealTransport(addr), NewRealTransport(addr)
	t.Setenv("TYMUXD_SOCKET_PATH", sockB)
	b := NewRealTransport(addr)

	require.Same(t, a1, a2, "same addr and socket: one selector")
	require.NotSame(t, a1, b, "same addr, different socket: a different daemon, a different selector")
}

func TestNewRealTransport_GaugeIsPerDaemonAndNotResetByLaterConstruction(t *testing.T) {
	addr := closedAddr(t)
	t.Setenv("STAPLER_SQUAD_INSTANCE", "")
	t.Setenv("TYMUXD_ADDR", addr)
	t.Setenv("TYMUXD_SOCKET_PATH", missingSocketPath(t))
	first := NewRealTransport(addr).(*selectingTransport)
	first.switchTo(transportTCP, "test")
	require.EqualValues(t, 1, transportGauge(t, addr, transportTCP))

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); _ = NewRealTransport(addr) }()
	}
	wg.Wait()

	require.EqualValues(t, 1, transportGauge(t, addr, transportTCP), "constructing more session transports must not reset the gauge")
	require.EqualValues(t, 0, transportGauge(t, addr, transportUnix))
}

func TestSocketPathForAddr_OverlongPathIsTCPOnly(t *testing.T) {
	const addr = "http://127.0.0.1:19998"
	long := "/tmp/" + strings.Repeat("d", 120) + ".sock"
	t.Setenv("STAPLER_SQUAD_INSTANCE", "")
	t.Setenv("TYMUXD_ADDR", addr)
	t.Setenv("TYMUXD_SOCKET_PATH", long)
	cfg := ResolveDaemonConfig()

	require.Empty(t, socketPathForAddr(addr), "a sun_path over the limit can never be dialed")
	require.Empty(t, unixSocketPath(cfg))
	require.Equal(t, long, ownedSocketPath(cfg), "the never-kill veto still names the path")
	require.IsType(t, &grpcTransport{}, NewRealTransport(addr), "TCP-only transport")
}

func TestSocketPathUsable_PlatformLimit(t *testing.T) {
	t.Parallel()
	require.True(t, socketPathUsable("/tmp/a.sock"))
	require.False(t, socketPathUsable("/"+strings.Repeat("x", 200)))
	require.False(t, socketPathUsable("/"+strings.Repeat("x", socketPathMax)), "sun_path needs room for the NUL")
	require.True(t, socketPathUsable("/"+strings.Repeat("x", socketPathMax-2)))
}
