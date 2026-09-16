package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session"
)

// TestTaggingRulesStore_ToRules_should_CompilePatternsFromStoredStrings_When_SpecValid covers
// Story 2.3.1's first acceptance criterion.
func TestTaggingRulesStore_ToRules_should_CompilePatternsFromStoredStrings_When_SpecValid(t *testing.T) {
	repo := session.NewTestEntRepository(t)
	storage, err := session.NewStorageWithRepository(repo)
	require.NoError(t, err)

	store, err := NewTaggingRulesStore(storage)
	require.NoError(t, err)

	spec := TaggingRuleSpec{
		ID:            "seed-bugfix",
		Name:          "Bugfix branch",
		BranchPattern: "^(bugfix|fix)/",
		OutputTag:     "Bugfix",
		Priority:      50,
		Enabled:       true,
		Source:        "seed",
	}
	_, err = store.Upsert(context.Background(), spec)
	require.NoError(t, err)

	rules := store.ToRules()
	require.Len(t, rules, 1)
	require.NotNil(t, rules[0].BranchPattern)
	assert.True(t, rules[0].BranchPattern.MatchString("bugfix/foo"))
}

// TestTaggingRulesStore_Upsert_should_RejectInvalidRegex_When_PatternUnterminated covers
// Story 2.3.1's second acceptance criterion.
func TestTaggingRulesStore_Upsert_should_RejectInvalidRegex_When_PatternUnterminated(t *testing.T) {
	repo := session.NewTestEntRepository(t)
	storage, err := session.NewStorageWithRepository(repo)
	require.NoError(t, err)

	store, err := NewTaggingRulesStore(storage)
	require.NoError(t, err)

	spec := TaggingRuleSpec{
		ID:            "bad-regex",
		Name:          "Bad regex",
		BranchPattern: "^(unterminated[",
		OutputTag:     "X",
	}
	_, err = store.Upsert(context.Background(), spec)
	require.Error(t, err)

	all := store.All()
	for _, s := range all {
		assert.NotEqual(t, "bad-regex", s.ID, "invalid spec must not be persisted")
	}
}
