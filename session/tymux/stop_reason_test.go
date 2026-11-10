package tymux

import (
	"context"
	"errors"
	"net"
	"os"
	"strconv"
	"sync/atomic"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

func resetStopRing() {
	stopRing.Lock()
	stopRing.events = nil
	stopRing.Unlock()
}

// fakeStopEnv pins the process age and a clock that advances one second per reading, and shrinks
// the SIGTERM grace so the escalation path is fast. It touches package globals: callers must not
// be parallel.
func fakeStopEnv(t *testing.T, signal func(pid int, sig os.Signal) error) {
	t.Helper()
	origSig, origGrace, origAge, origNow := signalProcessFn, daemonStopGracePeriod, processAgeFn, stopNowFn
	t.Cleanup(func() {
		signalProcessFn, daemonStopGracePeriod, processAgeFn, stopNowFn = origSig, origGrace, origAge, origNow
	})
	signalProcessFn = signal
	daemonStopGracePeriod = 20 * time.Millisecond
	processAgeFn = func(context.Context, int) (time.Duration, error) { return 3*time.Hour + 5*time.Minute, nil }
	var ticks atomic.Int64
	base := time.Unix(1_700_000_000, 0)
	stopNowFn = func() time.Time { return base.Add(time.Duration(ticks.Add(1)) * time.Second) }
	resetStopRing()
	t.Cleanup(resetStopRing)
}

func TestTerminateProcess_LogsReasonAgeProbesAndOutcome(t *testing.T) {
	reason := StopReason{
		Kind:   StopMonitorRecovery,
		Detail: "6 consecutive failed probes (threshold 6)",
		Probes: ProbeSummary{TCP: "not_listening", Unix: "no_socket_file"},
	}
	for _, tc := range []struct {
		name        string
		exitsOnTerm bool
		alreadyGone bool
		outcome     StopOutcome
		wantLogs    []string
	}{
		{"exits after SIGTERM", true, false, StopExitedAfterTerm, nil},
		{"escalates to SIGKILL", false, false, StopEscalatedToKill, []string{"ignored SIGTERM, escalating to SIGKILL"}},
		{"already gone", false, true, StopAlreadyGone, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dead := tc.alreadyGone
			fakeStopEnv(t, func(_ int, sig os.Signal) error {
				if dead {
					return syscall.ESRCH
				}
				if sig == syscall.SIGTERM && tc.exitsOnTerm || sig == syscall.SIGKILL {
					dead = true
				}
				return nil
			})
			logs := captureTymuxLogs(t)

			require.NoError(t, terminateProcess(5014, reason))

			out := logs.String()
			for _, want := range append([]string{
				"stopping tymuxd", "pid=5014", "daemon_age=3h5m0s",
				"reason_kind=monitor_recovery", "monitor recovery: 6 consecutive failed probes",
				"tcp_probe=not_listening", "unix_probe=no_socket_file",
				"tymuxd stop finished", "outcome=" + string(tc.outcome),
			}, tc.wantLogs...) {
				require.Contains(t, out, want)
			}
			stops := RecentDaemonStops()
			require.Len(t, stops, 1)
			require.Equal(t, tc.outcome, stops[0].Outcome)
			require.Equal(t, 5014, stops[0].PID)
			require.Equal(t, reason, stops[0].Reason)
			require.True(t, stops[0].AgeKnown)
			require.Equal(t, time.Second, stops[0].Elapsed, "fake clock advances 1s between begin and finish")
		})
	}
}

func TestTerminateProcess_UnknownAgeAndProbesAreNamedNotOmitted(t *testing.T) {
	fakeStopEnv(t, func(int, os.Signal) error { return syscall.ESRCH })
	processAgeFn = func(context.Context, int) (time.Duration, error) { return 0, errors.New("ps failed") }
	logs := captureTymuxLogs(t)

	require.NoError(t, terminateProcess(77, StopReason{Kind: StopManual}))

	out := logs.String()
	require.Contains(t, out, "daemon_age=unknown")
	require.Contains(t, out, "tcp_probe=unknown")
	require.Contains(t, out, "unix_probe=unknown")
	require.False(t, RecentDaemonStops()[0].AgeKnown)
}

func TestStopRing_KeepsOnlyTheLast50(t *testing.T) {
	resetStopRing()
	t.Cleanup(resetStopRing)
	for i := range stopRingSize + 7 {
		appendStop(StopEvent{PID: i})
	}
	got := RecentDaemonStops()
	require.Len(t, got, stopRingSize)
	require.Equal(t, 7, got[0].PID, "oldest events fall off first")
	require.Equal(t, stopRingSize+6, got[len(got)-1].PID)
}

func TestParseEtime(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]time.Duration{
		" 05:07\n":      5*time.Minute + 7*time.Second,
		"01:02:03":      time.Hour + 2*time.Minute + 3*time.Second,
		"2-03:04:05":    48*time.Hour + 3*time.Hour + 4*time.Minute + 5*time.Second,
		"  12-00:00:01": 12*24*time.Hour + time.Second,
	} {
		got, err := parseEtime(in)
		require.NoError(t, err, in)
		require.Equal(t, want, got, in)
	}
	for _, bad := range []string{"", "12", "a:b", "1-xx:00", "1:2:3:4", "-1:00"} {
		_, err := parseEtime(bad)
		require.Error(t, err, bad)
	}
}

func TestStopKindMessagesAreDistinctAndHuman(t *testing.T) {
	t.Parallel()
	seen := map[string]StopKind{}
	for _, k := range []StopKind{StopMonitorRecovery, StopSquadShutdown, StopVersionReplace, StopManual, StopStaleDaemon, StopFailedColdStart} {
		msg := StopReason{Kind: k}.Message()
		require.NotEqual(t, "unspecified", msg, k)
		require.NotContains(t, msg, "_", "human text, not the enum")
		require.NotContains(t, seen, msg)
		seen[msg] = k
	}
	require.Equal(t, "squad shutdown: --tymuxd-keep-server=false", StopReason{Kind: StopSquadShutdown, Detail: "--tymuxd-keep-server=false"}.Message())
}

// The monitor's recovery path must hand EnsureDaemonRunning a reason that names the streak.
func TestDaemonMonitor_RecoveryCarriesStopReason(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		got := make(chan StopReason, 1)
		m := daemonMonitor{interval: time.Second, maxBackoff: time.Second, threshold: 3,
			healthy: func(context.Context, DaemonConfig) bool { return false },
			ensure: func(c context.Context, _ DaemonConfig) (TymuxdReady, error) {
				r, _ := stopReasonFrom(c)
				select {
				case got <- r:
				default:
				}
				return TymuxdReady{}, errors.New("stop here")
			},
		}
		done := startDaemonMonitorTest(ctx, m)
		for range 4 {
			time.Sleep(time.Second) //nolint:notimesleeptest synctest advances virtual time, not wall-clock time
			synctest.Wait()
		}
		cancel()
		<-done
		r := <-got
		require.Equal(t, StopMonitorRecovery, r.Kind)
		require.Equal(t, "3 consecutive failed probes (threshold 3)", r.Detail)
	})
}

// With nothing listening, the stop EnsureDaemonRunning issues carries the caller's reason plus the
// last probe results recorded for that address, and a stale-daemon default when none was given.
func TestEnsureDaemonRunning_StopCarriesReasonAndLastProbes(t *testing.T) {
	for _, tc := range []struct {
		name string
		ctx  func() context.Context
		want StopReason
	}{
		{"default reason", context.Background, StopReason{Kind: StopStaleDaemon, Detail: "no listener answers a probe"}},
		{"monitor reason", func() context.Context {
			return withStopReason(context.Background(), StopReason{Kind: StopMonitorRecovery, Detail: "6 consecutive failed probes (threshold 6)"})
		}, StopReason{Kind: StopMonitorRecovery, Detail: "6 consecutive failed probes (threshold 6)"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withFastRetryBounds(t, 1, time.Millisecond, time.Millisecond)
			cfg := DaemonConfig{Addr: "http://127.0.0.1:19971", BinaryPath: "tymuxd"}
			recordProbe(cfg.Addr, ProbeSummary{TCP: "not_listening", Unix: "no_socket_file"})
			defer stubCheckDaemonHealthy(func(context.Context, DaemonConfig) bool { return false })()
			defer stubPortListening(func(DaemonConfig) bool { return false })()
			defer stubUnixListening(func(DaemonConfig) bool { return false })()
			defer stubStartDaemonAttempt(func(DaemonConfig) (*os.Process, error) { return nil, errors.New("no spawn") })()
			var stops []StopReason
			orig := stopTymuxdFn
			stopTymuxdFn = func(r StopReason) error { stops = append(stops, r); return nil }
			defer func() { stopTymuxdFn = orig }()

			_, err := ensureDaemonRunning(tc.ctx(), cfg, daemonRestartColdStart)

			require.Error(t, err)
			require.Len(t, stops, 1)
			want := tc.want
			want.Probes = ProbeSummary{TCP: "not_listening", Unix: "no_socket_file"}
			require.Equal(t, want, stops[0])
		})
	}
}

func TestProbeDaemonWithTCP_RecordsWhatEachTransportReported(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close()) // now refuses
	cfg := DaemonConfig{Addr: "http://" + addr, BinaryPath: "tymuxd"}

	result, _, probed := probeDaemonWithTCP(context.Background(), cfg)

	require.Equal(t, daemonProbeNotListening, result)
	require.True(t, probed)
	require.Equal(t, ProbeSummary{TCP: "not_listening", Unix: probeNotProbed}, lastProbeFor(cfg.Addr))
	require.Equal(t, ProbeSummary{TCP: probeUnknown, Unix: probeUnknown}, lastProbeFor("http://127.0.0.1:"+strconv.Itoa(1)))
}

// Every (tcp dial accepted, unix dial accepted) combination that reaches the "live listener but no
// answer" branch maps to exactly one sentinel and wording, so a mislabel fails here.
func TestEnsureDaemonRunning_UnresponsiveListenerWording(t *testing.T) {
	cases := []struct {
		name      string
		tcp, unix bool
		is        error
		isNot     []error
		contains  []string
		excludes  []string
	}{
		{
			name: "tcp accepts only", tcp: true, is: ErrTymuxdPortSquatted,
			isNot:    []error{ErrTymuxdControlSocketUnresponsive, ErrTymuxdUnresponsive},
			contains: []string{"TCP dial to http://127.0.0.1:19972 accepts connections but nothing answered as tymuxd", "Unix control socket is not listening", "not restarting it"},
		},
		{
			name: "unix accepts only", unix: true, is: ErrTymuxdControlSocketUnresponsive,
			isNot:    []error{ErrTymuxdPortSquatted, ErrTymuxdUnresponsive},
			contains: []string{"TCP dial to http://127.0.0.1:19972 was refused", "Unix control socket accepts connections but does not answer", "not restarting it"},
			excludes: []string{"something is listening", "squat"},
		},
		{
			name: "both accept", tcp: true, unix: true, is: ErrTymuxdUnresponsive,
			isNot:    []error{ErrTymuxdPortSquatted, ErrTymuxdControlSocketUnresponsive},
			contains: []string{"TCP dial to http://127.0.0.1:19972 and the Unix control socket both accept connections", "looks wedged", "not restarting it"},
			excludes: []string{"something is listening"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withFastRetryBounds(t, 2, time.Millisecond, 2*time.Millisecond)
			defer stubCheckDaemonHealthy(func(context.Context, DaemonConfig) bool { return false })()
			defer stubPortListening(func(DaemonConfig) bool { return tc.tcp })()
			defer stubUnixListening(func(DaemonConfig) bool { return tc.unix })()
			stop, spawn := neverCalled(t)
			defer stubStopTymuxd(stop)()
			defer stubStartDaemonAttempt(spawn)()

			_, err := EnsureDaemonRunning(context.Background(), DaemonConfig{Addr: "http://127.0.0.1:19972", BinaryPath: "tymuxd"})

			require.ErrorIs(t, err, tc.is)
			for _, no := range tc.isNot {
				require.NotErrorIs(t, err, no)
			}
			for _, s := range tc.contains {
				require.Contains(t, err.Error(), s)
			}
			for _, s := range tc.excludes {
				require.NotContains(t, err.Error(), s)
			}
		})
	}
}

// The monitor logs the decision too: which dials were accepted, never just "squatted".
func TestEnsureDaemonRunning_UnresponsiveListenerLogNamesBothDials(t *testing.T) {
	withFastRetryBounds(t, 1, time.Millisecond, time.Millisecond)
	logs := captureTymuxLogs(t)
	defer stubCheckDaemonHealthy(func(context.Context, DaemonConfig) bool { return false })()
	defer stubPortListening(func(DaemonConfig) bool { return false })()
	defer stubUnixListening(func(DaemonConfig) bool { return true })()
	stop, spawn := neverCalled(t)
	defer stubStopTymuxd(stop)()
	defer stubStartDaemonAttempt(spawn)()

	_, err := EnsureDaemonRunning(context.Background(), DaemonConfig{Addr: "http://127.0.0.1:19973", BinaryPath: "tymuxd"})

	require.ErrorIs(t, err, ErrTymuxdControlSocketUnresponsive)
	require.Contains(t, logs.String(), "tcp_dial_accepted=false")
	require.Contains(t, logs.String(), "unix_dial_accepted=true")
}
