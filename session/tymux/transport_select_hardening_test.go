package tymux

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"
	"github.com/tstapler/tymux/clients/go/selecting"
)

func eperm() error { return syscallDialErr(syscall.EPERM) }

func TestSelectingTransport_UnixEPERM_IsHardErrorAndNeverFallsBack(t *testing.T) {
	t.Parallel()
	s, tcp, unix, _ := newTestSelector()
	unix.fail(eperm())

	err := list(t, s)

	require.ErrorIs(t, err, ErrTymuxdSocketAccessDenied)
	require.ErrorIs(t, err, syscall.EPERM)
	require.Zero(t, tcp.calls.Load())
	require.Equal(t, selecting.Unix, s.Active())
}

func TestSelectingTransport_ReprobeSeesEPERM_SwitchesBackToUnixSoTheErrorSurfaces(t *testing.T) {
	t.Parallel()
	s, tcp, unix, clock := newTestSelector()
	unix.fail(enoent())
	require.NoError(t, list(t, s))
	unix.fail(eperm())
	clock.advance(reprobeInterval)
	tcpCalls := tcp.calls.Load()

	require.ErrorIs(t, list(t, s), ErrTymuxdSocketAccessDenied)
	require.Equal(t, selecting.Unix, s.Active())
	require.Equal(t, tcpCalls, tcp.calls.Load())
}

// A socket path that can never work (a non-directory component, as for an over-long sun_path)
// must fall back to TCP instead of failing permanently with TCP never tried.
func TestSelectingTransport_BrokenSocketPath_FallsBackToTCP(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), "afile")
	require.NoError(t, os.WriteFile(file, nil, 0o600))
	unix, _ := newUnixTransport(filepath.Join(file, "d.sock"))
	tcp := &fakeEndpoint{}
	s := selecting.NewFromTransports(unix, tcp, selecting.Options{})

	require.NoError(t, list(t, s))
	require.Equal(t, selecting.TCP, s.Active())
	require.Positive(t, tcp.calls.Load())
}

func TestSharedTransportFor_ConcurrentFirstCallersBuildOnce(t *testing.T) {
	t.Parallel()
	var built atomic.Int32
	key := fmt.Sprintf("shared-concurrent-%p", &built)
	got := make([]*selecting.Client, 32)
	var wg sync.WaitGroup
	for i := range got {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got[i] = sharedTransportFor(key, func() *selecting.Client {
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
	addr := tcpAnswering(t)
	t.Setenv("STAPLER_SQUAD_INSTANCE", "")
	t.Setenv("TYMUXD_ADDR", addr)
	t.Setenv("TYMUXD_SOCKET_PATH", missingSocketPath(t))
	first := NewRealTransport(addr)
	_, err := first.GetServerInfo(context.Background(), connect.NewRequest(&v1.GetServerInfoRequest{}))
	require.NoError(t, err, "the socket is missing and TCP answers: the call fails over")
	require.EqualValues(t, 1, transportGauge(t, addr, string(selecting.TCP)))

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); _ = NewRealTransport(addr) }()
	}
	wg.Wait()

	require.EqualValues(t, 1, transportGauge(t, addr, string(selecting.TCP)), "constructing more session transports must not reset the gauge")
	require.EqualValues(t, 0, transportGauge(t, addr, string(selecting.Unix)))
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
	_, selects := NewRealTransport(addr).(*selecting.Client)
	require.False(t, selects, "TCP-only transport")
}

func TestSocketPathUsable_PlatformLimit(t *testing.T) {
	t.Parallel()
	require.True(t, socketPathUsable("/tmp/a.sock"))
	require.False(t, socketPathUsable("/"+strings.Repeat("x", 200)))
	require.False(t, socketPathUsable("/"+strings.Repeat("x", socketPathMax)), "sun_path needs room for the NUL")
	require.True(t, socketPathUsable("/"+strings.Repeat("x", socketPathMax-2)))
}
