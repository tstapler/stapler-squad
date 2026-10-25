//go:build integration

package tymux

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/envtest"
)

func TestIntegration_DetectDaemonVersions_FindsRealRunningDaemon(t *testing.T) {
	bin := findTymuxdBinary(t)
	envtest.NewIsolatedStateDir(t)
	withFastRetryBounds(t, 8, 50*time.Millisecond, 300*time.Millisecond)
	cfg, _, err := ensureDaemonOnFreshLoopbackPort(t, bin)
	require.NoError(t, err)
	t.Cleanup(func() { _ = stopTymuxdFn() })

	info := DetectDaemonVersions(context.Background(), cfg)

	assert.True(t, info.Running)
	assert.Equal(t, readTymuxdPID(t), info.PID, "listener pid must match the daemon we spawned")
	assert.NotZero(t, info.ProtocolVersion, "version must come from GetServerInfo, not the process probe")
	assert.NotEmpty(t, info.RunningVersion)
	assert.Equal(t, info.InstalledVersion, info.RunningVersion)
	assert.Equal(t, VersionSame, info.Direction)
}

func TestIntegration_ReplaceDaemon_RestartsOntoConfiguredBinary(t *testing.T) {
	bin := findTymuxdBinary(t)
	envtest.NewIsolatedStateDir(t)
	withFastRetryBounds(t, 8, 50*time.Millisecond, 300*time.Millisecond)
	cfg, _, err := ensureDaemonOnFreshLoopbackPort(t, bin)
	require.NoError(t, err)
	t.Cleanup(func() { _ = stopTymuxdFn() })
	oldPID := readTymuxdPID(t)

	after, err := ReplaceDaemon(context.Background(), cfg, false)

	require.NoError(t, err, "no live sessions, so no opt-in is needed")
	assert.True(t, after.Running)
	assert.NotEqual(t, oldPID, after.PID, "the daemon must be a new process")
	assert.True(t, checkDaemonHealthy(context.Background(), cfg))
}
