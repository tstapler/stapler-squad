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

func TestFlags_ShouldBeAllOff_WhenConfigEmptyOrScannerBuiltWithoutFlags(t *testing.T) {
	assert.Equal(t, Flags{}, FlagsFromConfig(config.UnfinishedCaches{}))
	assert.Equal(t, Flags{}, NewScanner(nil, nil).flags)
	assert.Equal(t, Flags{}, (&GoGitVCSReader{}).effectiveFlags())
	assert.Equal(t, Flags{}, (*Scanner)(nil).flagsOrOff())
}

func TestFlags_ShouldBeRestartOnly_WhenConfigChangesAfterStartup(t *testing.T) {
	cfg := config.UnfinishedCaches{DiffCounter: true}
	s := NewScanner(nil, nil, WithFlags(FlagsFromConfig(cfg)))
	cfg.DiffCounter = false // a later config edit or reload
	cfg.SharedCommitState = true
	assert.Equal(t, Flags{DiffCounter: true}, s.flags, "flags change only by building a new Scanner")
	assert.Equal(t, Flags{DiffCounter: true}, s.reader.(*GoGitVCSReader).effectiveFlags(), "the built-in reader adopts the scanner's flags")
}

func TestFlags_ShouldNotLeakBetweenScanners_WhenBuiltInAnyOrder(t *testing.T) {
	on := NewScanner(nil, nil, WithFlags(Flags{ScanSkip: true, ScanSkipPrelude: true}))
	off := NewScanner(nil, nil)
	assert.True(t, on.preludeEnabled(), "a later flagless scanner must not change an earlier one's view")
	assert.False(t, off.preludeEnabled())
	assert.True(t, BuildReport(on).Flags["scan_skip"])
	assert.False(t, BuildReport(off).Flags["scan_skip"])
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
