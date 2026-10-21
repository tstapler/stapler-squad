package unfinished

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/config"
)

// legalCombos enumerates every config combination (2^4); the normalizer maps
// each to a legal effective state.
func legalCombos() []config.UnfinishedCaches {
	var out []config.UnfinishedCaches
	for m := 0; m < 16; m++ {
		out = append(out, config.UnfinishedCaches{
			SharedCommitState:    m&1 != 0,
			DiffCounter:          m&2 != 0,
			ScanSkip:             m&4 != 0,
			WatchWorktreeGitdirs: m&8 != 0,
		})
	}
	return out
}

func TestEffectiveFlags_ShouldForceDependentOff_WhenDependencyOffAndWarnOnce(t *testing.T) {
	for _, c := range legalCombos() {
		f, warns := normalizeFlags(c)
		assert.Equal(t, c.SharedCommitState, f.SharedCommitState)
		assert.Equal(t, c.DiffCounter, f.DiffCounter)
		assert.Equal(t, c.ScanSkip, f.ScanSkip)
		assert.Equal(t, c.WatchWorktreeGitdirs, f.WatchWorktreeGitdirs)
		assert.Equal(t, c.ScanSkip && c.SharedCommitState, f.ScanSkipPrelude, "%+v", c)
		assert.Equal(t, c.ScanSkip && c.WatchWorktreeGitdirs, f.ScanSkipDirtyGen, "%+v", c)
		wantWarns := 0
		if c.ScanSkip && !c.SharedCommitState {
			wantWarns++
		}
		if c.ScanSkip && !c.WatchWorktreeGitdirs {
			wantWarns++
		}
		assert.Len(t, warns, wantWarns, "%+v", c)
	}
}

func TestFlags_ShouldBeAllOff_WhenSnapshotUnsetOrConfigEmpty(t *testing.T) {
	prev := flagsSnapshot.Load()
	t.Cleanup(func() { flagsSnapshot.Store(prev) })
	flagsSnapshot.Store(nil)
	assert.Equal(t, Flags{}, EffectiveFlags())
	SetFlags(config.UnfinishedCaches{})
	assert.Equal(t, Flags{}, EffectiveFlags())
}

func TestFlags_ShouldBeRestartOnly_WhenConfigChangesAfterStartup(t *testing.T) {
	prev := flagsSnapshot.Load()
	t.Cleanup(func() { flagsSnapshot.Store(prev) })
	cfg := config.UnfinishedCaches{DiffCounter: true}
	SetFlags(cfg)
	cfg.DiffCounter = false // a later config edit or reload
	cfg.SharedCommitState = true
	assert.Equal(t, Flags{DiffCounter: true}, EffectiveFlags(), "snapshot changes only through SetFlags")
}

func TestFlags_ShouldNotBeExpired_WhenRemoveByDateHasPassed(t *testing.T) {
	now := time.Now() // read once
	for _, spec := range FlagSpecs {
		d, err := time.Parse("2006-01-02", spec.RemoveBy)
		require.NoError(t, err, spec.Name)
		assert.False(t, now.After(d.AddDate(0, 0, 1)),
			"flag %s expired on %s: retire it (Story 5.3.1) or extend RemoveBy with a justification", spec.Name, spec.RemoveBy)
	}
}

func TestFlagSpecs_ShouldHaveTriggerAndMatchConfigKeys(t *testing.T) {
	var keys []string
	ct := reflect.TypeOf(config.UnfinishedCaches{})
	for i := 0; i < ct.NumField(); i++ {
		keys = append(keys, strings.Split(ct.Field(i).Tag.Get("json"), ",")[0])
	}
	names := make([]string, 0, len(FlagSpecs))
	for _, spec := range FlagSpecs {
		names = append(names, spec.Name)
		assert.Contains(t, []string{"T1", "T2", "T3", "T4"}, spec.Trigger, spec.Name)
	}
	assert.ElementsMatch(t, keys, names)
}
