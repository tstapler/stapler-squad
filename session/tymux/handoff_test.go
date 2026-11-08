package tymux

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"
)

// stubHandoff fakes a running daemon that advertises graceful_handoff and records every request,
// signal and spawn so tests can assert the daemon is never killed.
type stubHandoff struct {
	reqs     []handoffRequest
	signals  int
	spawns   int
	version  *string // the version the fake daemon reports; replaced by a successful handoff
	live     int
	liveNow  func() int
	reply    func(handoffRequest) (handoffReply, error)
	noSwap   bool // an accepted handoff leaves the old version reporting
	infoErrs int  // GetServerInfo fails this many times first, as during the exec gap
}

func installHandoffStubs(t *testing.T, running, installed string, live int) *stubHandoff {
	t.Helper()
	stubVersionSeams(t, running, installed, 4242)
	origInfo, origLive, origSig, origPrep, origStart := serverInfoFn, liveSessionsFn, signalProcessFn, prepareHandoffFn, startDaemonAttemptFn
	origTimeout, origPoll := handoffVerifyTimeout, handoffVerifyPoll
	t.Cleanup(func() {
		serverInfoFn, liveSessionsFn, signalProcessFn, prepareHandoffFn, startDaemonAttemptFn = origInfo, origLive, origSig, origPrep, origStart
		handoffVerifyTimeout, handoffVerifyPoll = origTimeout, origPoll
	})
	handoffVerifyTimeout, handoffVerifyPoll = 50*time.Millisecond, time.Millisecond

	s := &stubHandoff{live: live}
	s.version = &running
	s.liveNow = func() int { return s.live }
	serverInfoFn = func(context.Context, DaemonConfig) (*v1.ServerInfo, error) {
		if s.infoErrs > 0 && len(s.reqs) == 2 {
			s.infoErrs--
			return nil, connect.NewError(connect.CodeUnavailable, errors.New("daemon is exec'ing"))
		}
		return &v1.ServerInfo{Version: *s.version, Pid: 4242, Capabilities: []string{"graceful_handoff"}}, nil
	}
	liveSessionsFn = func(context.Context, DaemonConfig) (int, error) { return s.liveNow(), nil }
	signalProcessFn = func(int, os.Signal) error { s.signals++; return nil }
	startDaemonAttemptFn = func(DaemonConfig) (*os.Process, error) { s.spawns++; return &os.Process{}, nil }
	s.reply = func(handoffRequest) (handoffReply, error) { return handoffReply{}, nil }
	prepareHandoffFn = func(_ context.Context, _ DaemonConfig, r handoffRequest) (handoffReply, error) {
		s.reqs = append(s.reqs, r)
		rep, err := s.reply(r)
		if err == nil && !rep.Refused && !r.DryRun && !s.noSwap {
			installed := r.ExpectedVersion
			s.version = &installed
		}
		return rep, err
	}
	return s
}

func TestReplaceDaemon_HandoffKeepsLivePanes_UpgradeAndDowngrade(t *testing.T) {
	for name, tc := range map[string]struct{ running, installed string }{
		"upgrade":   {"1.13.1", "1.14.0"},
		"downgrade": {"1.14.0", "1.13.1"},
	} {
		t.Run(name, func(t *testing.T) {
			s := installHandoffStubs(t, tc.running, tc.installed, 3)

			after, err := ReplaceDaemon(context.Background(), DaemonConfig{BinaryPath: "/installed/tymuxd"}, false)

			require.NoError(t, err)
			assert.Equal(t, tc.installed, after.RunningVersion)
			assert.Equal(t, 4242, after.PID, "the daemon keeps its pid across the handoff")
			require.Len(t, s.reqs, 2)
			assert.True(t, s.reqs[0].DryRun, "dry run first")
			assert.False(t, s.reqs[1].DryRun)
			assert.Equal(t, tc.installed, s.reqs[1].ExpectedVersion)
			assert.Equal(t, "/installed/tymuxd", s.reqs[1].BinaryPath)
			assert.Zero(t, s.signals, "a handoff never signals the daemon")
			assert.Zero(t, s.spawns)
		})
	}
}

func TestReplaceDaemon_HandoffRefusalNeverKillsLiveDaemon(t *testing.T) {
	s := installHandoffStubs(t, "1.14.0", "1.13.1", 2)
	s.reply = func(handoffRequest) (handoffReply, error) {
		return handoffReply{Refused: true, Reason: "handoff schema 2 not readable by target"}, nil
	}

	_, err := ReplaceDaemon(context.Background(), DaemonConfig{BinaryPath: "/installed/tymuxd"}, false)

	require.ErrorIs(t, err, ErrHandoffRefused)
	assert.Contains(t, err.Error(), "schema 2")
	assert.Len(t, s.reqs, 1, "a refused dry run stops before the real request")
	assert.Zero(t, s.signals)
	assert.Zero(t, s.spawns)
}

func TestReplaceDaemon_HandoffUnavailableBuildNeverKills(t *testing.T) {
	s := installHandoffStubs(t, "1.13.1", "1.14.0", 2)
	prepareHandoffFn = func(context.Context, DaemonConfig, handoffRequest) (handoffReply, error) {
		return handoffReply{}, ErrHandoffUnavailable
	}

	_, err := ReplaceDaemon(context.Background(), DaemonConfig{BinaryPath: "/installed/tymuxd"}, false)

	require.ErrorIs(t, err, ErrHandoffUnavailable)
	assert.Zero(t, s.signals)
	assert.Zero(t, s.spawns)
}

func TestReplaceDaemon_HandoffVerifyFailureLeavesDaemonRunning(t *testing.T) {
	cases := map[string]func(*stubHandoff){
		"version never changes": func(s *stubHandoff) { s.noSwap = true },
		"pane count drops": func(s *stubHandoff) {
			s.liveNow = func() int {
				if len(s.reqs) == 2 {
					return 1
				}
				return s.live
			}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			s := installHandoffStubs(t, "1.13.1", "1.14.0", 3)
			mutate(s)

			_, err := ReplaceDaemon(context.Background(), DaemonConfig{BinaryPath: "/installed/tymuxd"}, false)

			require.ErrorIs(t, err, ErrHandoffVerifyFailed)
			assert.Zero(t, s.signals, "an unverified handoff must not kill the daemon")
			assert.Zero(t, s.spawns)
		})
	}
}

func TestReplaceDaemon_FailedHandoffFallsBackOnlyWithOperatorOverride(t *testing.T) {
	s := installHandoffStubs(t, "1.13.1", "1.14.0", 2)
	s.reply = func(handoffRequest) (handoffReply, error) { return handoffReply{Refused: true, Reason: "no"}, nil }
	origPort, origCheck, origStop := portListeningFn, checkDaemonHealthyFn, stopTymuxdFn
	t.Cleanup(func() { portListeningFn, checkDaemonHealthyFn, stopTymuxdFn = origPort, origCheck, origStop })
	portListeningFn = func(DaemonConfig) bool { return s.signals == 0 }
	checkDaemonHealthyFn = func(context.Context, DaemonConfig) bool { return s.spawns > 0 }
	stopTymuxdFn = func(StopReason) error { return nil }

	_, err := ReplaceDaemon(context.Background(), DaemonConfig{BinaryPath: "/installed/tymuxd"}, true)

	require.NoError(t, err)
	assert.Equal(t, 1, s.signals, "allowInterrupt restores the explicit lossy replace")
	assert.Equal(t, 1, s.spawns)
}

func TestReplaceDaemon_NoLivePanesSkipsHandoff(t *testing.T) {
	s := installHandoffStubs(t, "1.13.1", "1.14.0", 0)
	origPort, origCheck, origStop := portListeningFn, checkDaemonHealthyFn, stopTymuxdFn
	t.Cleanup(func() { portListeningFn, checkDaemonHealthyFn, stopTymuxdFn = origPort, origCheck, origStop })
	portListeningFn = func(DaemonConfig) bool { return s.signals == 0 }
	checkDaemonHealthyFn = func(context.Context, DaemonConfig) bool { return s.spawns > 0 }
	stopTymuxdFn = func(StopReason) error { return nil }

	_, err := ReplaceDaemon(context.Background(), DaemonConfig{BinaryPath: "/installed/tymuxd"}, false)

	require.NoError(t, err)
	assert.Empty(t, s.reqs, "an idle daemon is replaced the ordinary way")
}

func TestReplaceDaemon_HandoffToleratesTheExecGap(t *testing.T) {
	s := installHandoffStubs(t, "1.13.1", "1.14.0", 3)
	s.infoErrs = 5
	handoffVerifyTimeout = time.Second

	after, err := ReplaceDaemon(context.Background(), DaemonConfig{BinaryPath: "/installed/tymuxd"}, false)

	require.NoError(t, err)
	assert.Equal(t, "1.14.0", after.RunningVersion)
	assert.Zero(t, s.signals)
}

func TestReplaceDaemon_TransportErrorOnTheRealRequestIsVerifiedNotAssumedFatal(t *testing.T) {
	s := installHandoffStubs(t, "1.13.1", "1.14.0", 3)
	s.reply = func(r handoffRequest) (handoffReply, error) {
		if r.DryRun {
			return handoffReply{}, nil
		}
		// The daemon swapped, then the connection dropped under the exec.
		v := r.ExpectedVersion
		s.version = &v
		return handoffReply{}, connect.NewError(connect.CodeUnavailable, errors.New("connection reset"))
	}
	s.noSwap = true

	after, err := ReplaceDaemon(context.Background(), DaemonConfig{BinaryPath: "/installed/tymuxd"}, false)

	require.NoError(t, err)
	assert.Equal(t, "1.14.0", after.RunningVersion)
	assert.Zero(t, s.signals)
}

func TestReplaceDaemon_UnverifiedHandoffNeverFallsBackEvenWithOperatorOverride(t *testing.T) {
	s := installHandoffStubs(t, "1.13.1", "1.14.0", 3)
	s.noSwap = true

	_, err := ReplaceDaemon(context.Background(), DaemonConfig{BinaryPath: "/installed/tymuxd"}, true)

	require.ErrorIs(t, err, ErrHandoffVerifyFailed)
	assert.Zero(t, s.signals, "the daemon may already be the new image; never kill it")
	assert.Zero(t, s.spawns)
}

func TestReplaceDaemon_SameVersionHandoffFailsClosedWithoutGeneration(t *testing.T) {
	s := installHandoffStubs(t, "1.14.0", "1.14.0", 3)

	_, err := ReplaceDaemon(context.Background(), DaemonConfig{BinaryPath: "/installed/tymuxd"}, false)

	require.ErrorIs(t, err, ErrHandoffRefused)
	assert.Empty(t, s.reqs, "no request may be sent when completion cannot be verified")
	assert.Zero(t, s.signals)
}

func TestReplaceDaemon_SameVersionHandoffVerifiedByGeneration(t *testing.T) {
	s := installHandoffStubs(t, "1.14.0", "1.14.0", 3)
	gen := uint64(4)
	orig := readHandoffGenerationFn
	t.Cleanup(func() { readHandoffGenerationFn = orig })
	readHandoffGenerationFn = func(context.Context, DaemonConfig) (uint64, bool, error) { return gen, true, nil }
	s.reply = func(r handoffRequest) (handoffReply, error) {
		if !r.DryRun {
			gen++ // the new image committed
		}
		return handoffReply{}, nil
	}

	after, err := ReplaceDaemon(context.Background(), DaemonConfig{BinaryPath: "/installed/tymuxd"}, false)

	require.NoError(t, err)
	assert.Equal(t, 4242, after.PID)
	assert.Len(t, s.reqs, 2)
	assert.Zero(t, s.signals)
}

func TestReplaceDaemon_SameVersionHandoffWithUnchangedGenerationIsUnverified(t *testing.T) {
	s := installHandoffStubs(t, "1.14.0", "1.14.0", 3)
	orig := readHandoffGenerationFn
	t.Cleanup(func() { readHandoffGenerationFn = orig })
	readHandoffGenerationFn = func(context.Context, DaemonConfig) (uint64, bool, error) { return 4, true, nil }

	_, err := ReplaceDaemon(context.Background(), DaemonConfig{BinaryPath: "/installed/tymuxd"}, true)

	require.ErrorIs(t, err, ErrHandoffVerifyFailed)
	assert.Zero(t, s.signals)
	assert.Zero(t, s.spawns)
}

func TestReplaceDaemon_ExtraSessionsCreatedDuringHandoffAreNotAFailure(t *testing.T) {
	s := installHandoffStubs(t, "1.13.1", "1.14.0", 3)
	s.liveNow = func() int { return 4 }

	_, err := ReplaceDaemon(context.Background(), DaemonConfig{BinaryPath: "/installed/tymuxd"}, false)

	require.NoError(t, err)
}

func TestReplaceDaemon_HandoffVerifyFailureWithOverrideAndRealRefusalNeverKillAfterSwap(t *testing.T) {
	t.Run("pane count drops after the swap, override set", func(t *testing.T) {
		s := installHandoffStubs(t, "1.13.1", "1.14.0", 3)
		s.liveNow = func() int {
			if len(s.reqs) == 2 {
				return 1
			}
			return s.live
		}

		_, err := ReplaceDaemon(context.Background(), DaemonConfig{BinaryPath: "/installed/tymuxd"}, true)

		require.ErrorIs(t, err, ErrHandoffVerifyFailed)
		assert.Zero(t, s.signals)
		assert.Zero(t, s.spawns)
	})
	t.Run("context cancelled mid-verify", func(t *testing.T) {
		s := installHandoffStubs(t, "1.13.1", "1.14.0", 3)
		s.noSwap = true
		handoffVerifyTimeout = time.Minute
		ctx, cancel := context.WithCancel(context.Background())
		prev := prepareHandoffFn
		prepareHandoffFn = func(c context.Context, cfg DaemonConfig, r handoffRequest) (handoffReply, error) {
			if !r.DryRun {
				cancel()
			}
			return prev(c, cfg, r)
		}

		_, err := ReplaceDaemon(ctx, DaemonConfig{BinaryPath: "/installed/tymuxd"}, true)

		require.ErrorIs(t, err, ErrHandoffVerifyFailed)
		assert.Zero(t, s.signals)
	})
}
