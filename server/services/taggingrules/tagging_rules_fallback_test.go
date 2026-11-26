package taggingrules

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session"
)

func TestNewEmptyTaggingRulesStore_should_SkipLoadingAndStillPersist_When_StorageHasRules(t *testing.T) {
	t.Parallel()
	storage, err := session.NewStorageWithRepository(session.NewTestEntRepository(t))
	require.NoError(t, err)
	seeded, err := NewTaggingRulesStore(storage)
	require.NoError(t, err)
	_, err = seeded.Upsert(context.Background(), TaggingRuleSpec{ID: "a", Name: "a", OutputTag: "A"})
	require.NoError(t, err)

	empty := NewEmptyTaggingRulesStore(storage)
	assert.Empty(t, empty.All())
	assert.Empty(t, empty.ToRules())

	_, err = empty.Upsert(context.Background(), TaggingRuleSpec{ID: "b", Name: "b", OutputTag: "B"})
	require.NoError(t, err)
	rows, err := storage.AllTaggingRules(context.Background())
	require.NoError(t, err)
	assert.Len(t, rows, 2)
}
