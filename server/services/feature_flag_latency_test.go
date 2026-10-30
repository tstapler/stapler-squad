package services

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// countConfigSyncs counts config.json fsyncs; each may stall for seconds when
// the disk is busy (F_FULLFSYNC on macOS), so the count is the latency multiplier.
func countConfigSyncs(t *testing.T, stall time.Duration) *atomic.Int64 {
	t.Helper()
	var n atomic.Int64
	t.Cleanup(config.SetSyncConfigFileForTest(func(f *os.File) error {
		n.Add(1)
		time.Sleep(stall)
		return f.Sync()
	}))
	return &n
}

func TestUpdateFeatureFlag_ShouldFsyncConfigOnce_WhenGlobalFlagIsSet(t *testing.T) {
	withIsolatedStateDir(t)
	svc := NewFeatureFlagService()
	syncs := countConfigSyncs(t, 0)

	_, err := mutateFlag(svc, "changelog", setOn, "global")
	require.NoError(t, err)

	assert.EqualValues(t, 1, syncs.Load(), "one config.json fsync per flag flip")
}

func TestUpdateFeatureFlag_ShouldFsyncConfigOnce_WhenScopedFlagIsSetAndCleared(t *testing.T) {
	withIsolatedStateDir(t)
	svc := NewFeatureFlagService()
	syncs := countConfigSyncs(t, 0)

	_, err := mutateFlag(svc, gateFlag, setOff, "kind:review")
	require.NoError(t, err)
	assert.EqualValues(t, 1, syncs.Load(), "scoped set")

	_, err = mutateFlag(svc, gateFlag, clearScope, "kind:review")
	require.NoError(t, err)
	assert.EqualValues(t, 2, syncs.Load(), "scoped clear adds exactly one")
}

func TestUpdateFeatureFlag_ShouldPersistValueOnDisk_WhenSavedOnce(t *testing.T) {
	withIsolatedStateDir(t)
	svc := NewFeatureFlagService()

	_, err := mutateFlag(svc, "changelog", setOn, "global")
	require.NoError(t, err)
	v, ok := config.LoadConfig().GetFeatureFlagOverride("changelog")
	assert.True(t, ok && v)

	_, err = mutateFlag(svc, "changelog", resetGlob, "")
	require.NoError(t, err)
	_, ok = config.LoadConfig().GetFeatureFlagOverride("changelog")
	assert.False(t, ok, "reset deletes the explicit key on disk")
}

func TestGetFeatureFlags_ShouldReadConfigFileOnce_WhenListingEveryFlag(t *testing.T) {
	withIsolatedStateDir(t)
	svc := NewFeatureFlagService()

	before := config.LoadConfigCalls()
	resp, err := svc.GetFeatureFlags(context.Background(), connect.NewRequest(&sessionv1.GetFeatureFlagsRequest{}))
	require.NoError(t, err)
	require.Greater(t, len(resp.Msg.Flags), 10)

	assert.LessOrEqual(t, config.LoadConfigCalls()-before, int64(2),
		"config.json is parsed a bounded number of times, not once per flag")
}
