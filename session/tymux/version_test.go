package tymux

import (
	"context"
	"errors"
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTymuxdVersion(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"tymuxd 1.3.0\n", "1.3.0", false},
		{"tymuxd 1.4.0-rc.1", "1.4.0-rc.1", false},
		{"tymuxd", "", true},
		{"tmux 3.4", "", true},
		{"tymuxd latest", "", true},
		{"", "", true},
	}
	for _, tc := range tests {
		got, err := ParseTymuxdVersion(tc.in)
		if tc.wantErr {
			assert.Error(t, err, tc.in)
			continue
		}
		require.NoError(t, err, tc.in)
		assert.Equal(t, tc.want, got)
	}
}

func TestCompareVersions(t *testing.T) {
	assert.Equal(t, VersionSame, CompareVersions("1.3.0", "1.3.0"))
	assert.Equal(t, VersionUpgrade, CompareVersions("1.3.0", "1.4.0"))
	assert.Equal(t, VersionUpgrade, CompareVersions("1.4.0-rc.1", "1.4.0"))
	assert.Equal(t, VersionDowngrade, CompareVersions("1.4.0", "1.3.0"))
	assert.Equal(t, VersionUnknown, CompareVersions("", "1.3.0"))
	assert.Equal(t, VersionUnknown, CompareVersions("1.3.0", "garbage"))
}

// stubVersionSeams installs fakes for every probe and restores them on cleanup.
func stubVersionSeams(t *testing.T, running, installed string, pid int) {
	t.Helper()
	origBin, origPID, origExe := binaryVersionFn, listenerPIDFn, processExeFn
	t.Cleanup(func() { binaryVersionFn, listenerPIDFn, processExeFn = origBin, origPID, origExe })
	binaryVersionFn = func(_ context.Context, path string) (string, error) {
		if path == "/running/exe" {
			return running, nil
		}
		return installed, nil
	}
	listenerPIDFn = func(context.Context, string) (int, error) {
		if pid == 0 {
			return 0, errors.New("nothing listening")
		}
		return pid, nil
	}
	processExeFn = func(context.Context, int) (string, error) { return "/running/exe", nil }
}

func TestDetectDaemonVersions_UpgradeAvailable(t *testing.T) {
	stubVersionSeams(t, "1.3.0", "1.4.0", 4242)

	info := DetectDaemonVersions(context.Background(), DaemonConfig{Addr: "127.0.0.1:1", BinaryPath: "/installed/tymuxd"})

	assert.Equal(t, DaemonVersionInfo{Running: true, PID: 4242, RunningVersion: "1.3.0", InstalledVersion: "1.4.0", Direction: VersionUpgrade}, info)
}

func TestDetectDaemonVersions_NothingRunning(t *testing.T) {
	stubVersionSeams(t, "", "1.3.0", 0)

	info := DetectDaemonVersions(context.Background(), DaemonConfig{BinaryPath: "/installed/tymuxd"})

	assert.False(t, info.Running)
	assert.Equal(t, "1.3.0", info.InstalledVersion)
	assert.Equal(t, VersionUnknown, info.Direction)
}

func TestReplaceDaemon_RefusesWhileSessionsAreLive(t *testing.T) {
	stubVersionSeams(t, "1.3.0", "1.2.0", 4242)
	origLive, origSig := liveSessionsFn, signalProcessFn
	t.Cleanup(func() { liveSessionsFn, signalProcessFn = origLive, origSig })
	liveSessionsFn = func(context.Context, DaemonConfig) (int, error) { return 3, nil }
	signalled := false
	signalProcessFn = func(int, os.Signal) error { signalled = true; return nil }

	_, err := ReplaceDaemon(context.Background(), DaemonConfig{BinaryPath: "/installed/tymuxd"}, false)

	require.ErrorIs(t, err, ErrLiveSessionsWouldBeInterrupted)
	assert.False(t, signalled, "must not signal the daemon when refusing")
}

func TestReplaceDaemon_StopsListenerThenEnsuresDaemon(t *testing.T) {
	stubVersionSeams(t, "1.3.0", "1.2.0", 4242)
	origLive, origSig, origPort, origCheck, origStart, origStop := liveSessionsFn, signalProcessFn, portListeningFn, checkDaemonHealthyFn, startDaemonAttemptFn, stopTymuxdFn
	t.Cleanup(func() {
		liveSessionsFn, signalProcessFn, portListeningFn, checkDaemonHealthyFn, startDaemonAttemptFn, stopTymuxdFn = origLive, origSig, origPort, origCheck, origStart, origStop
	})
	liveSessionsFn = func(context.Context, DaemonConfig) (int, error) { return 2, nil }
	var signals []os.Signal
	stopped := false
	signalProcessFn = func(pid int, sig os.Signal) error {
		assert.Equal(t, 4242, pid)
		signals = append(signals, sig)
		stopped = true
		return nil
	}
	portListeningFn = func(DaemonConfig) bool { return !stopped }
	spawned := false
	startDaemonAttemptFn = func(DaemonConfig) (*os.Process, error) { spawned = true; return &os.Process{}, nil }
	checkDaemonHealthyFn = func(context.Context, DaemonConfig) bool { return spawned }
	stopTymuxdFn = func() error { return nil }

	_, err := ReplaceDaemon(context.Background(), DaemonConfig{BinaryPath: "/installed/tymuxd"}, true)

	require.NoError(t, err)
	assert.Equal(t, []os.Signal{syscall.SIGTERM}, signals)
	assert.True(t, spawned, "a replacement daemon must be started")
}
