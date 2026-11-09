package flow

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
)

func TestDrillKeys_should_ListStaticClassStuckAndFunnelKeys_When_Called(t *testing.T) {
	t.Parallel()
	keys := DrillKeys()
	require.NotEmpty(t, keys)
	assert.True(t, slices.IsSorted(keys), "keys must be sorted and stable")
	assert.Equal(t, len(keys), len(slices.Compact(slices.Clone(keys))), "keys must be unique")

	for _, k := range staticDrillKeys {
		assert.Contains(t, keys, k)
	}
	for _, c := range AllVerdictClasses() {
		assert.Contains(t, keys, DrillVerdictClass(c))
	}
	for _, r := range domain.AllStuckReasons {
		assert.Contains(t, keys, DrillStuckReason(string(r)))
	}
	assert.Contains(t, keys, DrillFunnelStage("in_progress"))
}

func TestIsKnownDrillKey_should_AcceptRegistryAndDynamicPrefixes_When_KeyChecked(t *testing.T) {
	t.Parallel()
	assert.True(t, IsKnownDrillKey(DrillFPY))
	assert.True(t, IsKnownDrillKey("stuck.some_future_reason"))
	assert.True(t, IsKnownDrillKey("funnel.stage.qa"))
	assert.True(t, IsKnownDrillKey("review.class.judgment"))
	assert.False(t, IsKnownDrillKey("rework.reviewfail"))
	assert.False(t, IsKnownDrillKey(""))
	assert.NotEmpty(t, DrillKeyPatterns())
}
