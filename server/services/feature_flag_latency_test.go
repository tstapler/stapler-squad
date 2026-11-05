package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// stateConfigDir is the isolated state directory withIsolatedStateDir set up.
func stateConfigDir(t *testing.T) string {
	t.Helper()
	dir, err := config.GetConfigDir()
	require.NoError(t, err)
	return dir
}

// countConfigSyncs counts config.json fsyncs under dir; each may stall for
// seconds when the disk is busy (F_FULLFSYNC on macOS), so the count is the
// latency multiplier. Saves elsewhere in the process are ignored.
func countConfigSyncs(t *testing.T, dir string) *atomic.Int64 {
	t.Helper()
	var n atomic.Int64
	t.Cleanup(config.SetSyncConfigFileForTest(func(f *os.File) error {
		if filepath.Dir(f.Name()) == dir {
			n.Add(1)
		}
		return f.Sync()
	}))
	return &n
}

func TestUpdateFeatureFlag_ShouldFsyncConfigOnce_WhenGlobalFlagIsSet(t *testing.T) {
	withIsolatedStateDir(t)
	svc := NewFeatureFlagService()
	syncs := countConfigSyncs(t, stateConfigDir(t))

	_, err := mutateFlag(svc, "changelog", setOn, "global")
	require.NoError(t, err)

	assert.EqualValues(t, 1, syncs.Load(), "one config.json fsync per flag flip")
}

func TestUpdateFeatureFlag_ShouldFsyncConfigOnce_WhenScopedFlagIsSetAndCleared(t *testing.T) {
	withIsolatedStateDir(t)
	svc := NewFeatureFlagService()
	syncs := countConfigSyncs(t, stateConfigDir(t))

	_, err := mutateFlag(svc, gateFlag, setOff, "kind:review")
	require.NoError(t, err)
	assert.EqualValues(t, 1, syncs.Load(), "scoped set")

	_, err = mutateFlag(svc, gateFlag, clearScope, "kind:review")
	require.NoError(t, err)
	assert.EqualValues(t, 2, syncs.Load(), "scoped clear adds exactly one")
}

// A failed controller costs exactly two fsyncs (persist, then rollback) and
// leaves the file as it was.
func TestUpdateFeatureFlag_ShouldFsyncTwiceAndRestoreFile_WhenGlobalControllerFails(t *testing.T) {
	withIsolatedStateDir(t)
	svc := NewFeatureFlagService()
	svc.SetFeatureController("changelog", &fakeFeatureController{failEnable: errors.New("boom")})
	syncs := countConfigSyncs(t, stateConfigDir(t))

	_, err := mutateFlag(svc, "changelog", setOn, "global")
	require.Error(t, err)
	assert.EqualValues(t, 2, syncs.Load(), "persist + rollback")
	_, ok := config.LoadConfig().GetFeatureFlagOverride("changelog")
	assert.False(t, ok, "an absent key is deleted again, not left as false")

	require.NoError(t, config.LoadConfig().SetFeatureFlag("changelog", false))
	syncs.Store(0)
	_, err = mutateFlag(svc, "changelog", setOn, "global")
	require.Error(t, err)
	assert.EqualValues(t, 2, syncs.Load())
	v, ok := config.LoadConfig().GetFeatureFlagOverride("changelog")
	assert.True(t, ok && !v, "an existing key returns to its previous value")
}

func TestUpdateFeatureFlag_ShouldFsyncTwiceAndRestoreFile_WhenScopedControllerFails(t *testing.T) {
	withIsolatedStateDir(t)
	svc := NewFeatureFlagService()
	svc.SetFeatureController(gateFlag, &scopedController{failScope: errors.New("boom")})
	syncs := countConfigSyncs(t, stateConfigDir(t))

	_, err := mutateFlag(svc, gateFlag, setOff, "kind:review")
	require.Error(t, err)
	assert.EqualValues(t, 2, syncs.Load(), "persist + rollback")
	_, ok := config.LoadConfig().GetFeatureFlagScopedOverride(gateFlag, "kind:review")
	assert.False(t, ok, "an absent scope key is deleted again")

	require.NoError(t, config.LoadConfig().SetFeatureFlagScope(gateFlag, "kind:review", true))
	syncs.Store(0)
	_, err = mutateFlag(svc, gateFlag, setOff, "kind:review")
	require.Error(t, err)
	assert.EqualValues(t, 2, syncs.Load())
	v, ok := config.LoadConfig().GetFeatureFlagScopedOverride(gateFlag, "kind:review")
	assert.True(t, ok && v, "an existing scope key returns to its previous value")
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

func configReadsDuringList(t *testing.T, svc *FeatureFlagService) int64 {
	t.Helper()
	config.LoadConfig() // create config.json so the measured read is not the first-run default write
	path := filepath.Join(stateConfigDir(t), config.ConfigFileName)
	before := config.ConfigReads(path)
	resp, err := svc.GetFeatureFlags(context.Background(), connect.NewRequest(&sessionv1.GetFeatureFlagsRequest{}))
	require.NoError(t, err)
	require.Greater(t, len(resp.Msg.Flags), 10)
	return config.ConfigReads(path) - before
}

func TestGetFeatureFlags_ShouldReadConfigFileOnce_WhenListingEveryFlag(t *testing.T) {
	withIsolatedStateDir(t)
	assert.EqualValues(t, 1, configReadsDuringList(t, NewFeatureFlagService()),
		"config.json is parsed once, not once per flag")
}

// The production service wires status-detail sources of its own (the gate's
// global and per-kind lines); they must share the one snapshot too.
func TestGetFeatureFlags_ShouldReadConfigFileOnce_WhenGateStatusSourcesAreWired(t *testing.T) {
	svc := newGatedFlagService(t)
	assert.EqualValues(t, 1, configReadsDuringList(t, svc.featureFlagSvc))
}
