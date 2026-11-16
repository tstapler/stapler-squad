package sessionpr

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/session/internal/fixturetest"
)

func TestMergeBackfillSeeds_ShouldBeIndependentOfOrder(t *testing.T) {
	t.Parallel()
	ref := fixturetest.MustPRRef(t, "", "acme", "app", 21)
	term := BackfillCandidate{ref: ref, terminal: true}
	open := BackfillCandidate{ref: ref, terminal: false}
	assert.False(t, mergeBackfillSeeds([]BackfillCandidate{term}, []BackfillCandidate{open})[ref])
	assert.False(t, mergeBackfillSeeds([]BackfillCandidate{open}, []BackfillCandidate{term})[ref])
	assert.True(t, mergeBackfillSeeds([]BackfillCandidate{term}, []BackfillCandidate{term})[ref])
}
