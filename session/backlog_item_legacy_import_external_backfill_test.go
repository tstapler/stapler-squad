package session

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunBacklogItemLegacyImportExternalBackfill_should_FillOnlyEmptyLegacyItems_And_BeIdempotent(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	legacy, err := repo.Client.BacklogItem.Create().SetTitle("legacy").SetStatus("idea").
		SetNotes("Imported from https://github.com/o/r/issues/9").Save(ctx)
	require.NoError(t, err)
	populated, err := repo.Client.BacklogItem.Create().SetTitle("populated").SetStatus("idea").
		SetNotes("Imported from https://github.com/o/r/issues/10").
		SetExternalURL("https://github.com/o/other/issues/1").SetExternalID("1").Save(ctx)
	require.NoError(t, err)
	partial, err := repo.Client.BacklogItem.Create().SetTitle("partial").SetStatus("idea").
		SetNotes("Imported from https://github.com/o/r/issues/11").SetExternalID("keep").Save(ctx)
	require.NoError(t, err)
	freeText, err := repo.Client.BacklogItem.Create().SetTitle("free text").SetStatus("idea").
		SetNotes("Filed while shipping PR #671, see https://github.com/o/r/issues/12").Save(ctx)
	require.NoError(t, err)
	trailing, err := repo.Client.BacklogItem.Create().SetTitle("trailing").SetStatus("idea").
		SetNotes("Imported from https://github.com/o/r/issues/13 (re-triaged)").Save(ctx)
	require.NoError(t, err)

	for pass := 0; pass < 2; pass++ { // second pass proves idempotency
		require.NoError(t, runBacklogItemLegacyImportExternalBackfill(ctx, repo))

		got, err := repo.Client.BacklogItem.Get(ctx, legacy.ID)
		require.NoError(t, err)
		assert.Equal(t, "https://github.com/o/r/issues/9", got.ExternalURL)
		assert.Equal(t, "9", got.ExternalID)
		assert.True(t, got.UpdatedAt.Equal(legacy.UpdatedAt), "updated_at must be preserved")

		got, err = repo.Client.BacklogItem.Get(ctx, populated.ID)
		require.NoError(t, err)
		assert.Equal(t, "https://github.com/o/other/issues/1", got.ExternalURL)
		assert.Equal(t, "1", got.ExternalID)

		got, err = repo.Client.BacklogItem.Get(ctx, partial.ID)
		require.NoError(t, err)
		assert.Empty(t, got.ExternalURL)
		assert.Equal(t, "keep", got.ExternalID)

		got, err = repo.Client.BacklogItem.Get(ctx, freeText.ID)
		require.NoError(t, err)
		assert.Empty(t, got.ExternalURL)
		assert.Empty(t, got.ExternalID)

		got, err = repo.Client.BacklogItem.Get(ctx, trailing.ID)
		require.NoError(t, err)
		assert.Empty(t, got.ExternalURL)
		assert.Empty(t, got.ExternalID)
	}
}
