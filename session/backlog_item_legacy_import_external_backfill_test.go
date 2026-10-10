package session

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseLegacyImportNotes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		notes      string
		wantURL    string
		wantNumber string
		wantOK     bool
	}{
		{"issue", "Imported from https://github.com/tstapler/stapler-squad/issues/123", "https://github.com/tstapler/stapler-squad/issues/123", "123", true},
		{"pull", "Imported from https://github.com/o/r.x/pull/7", "https://github.com/o/r.x/pull/7", "7", true},
		{"GHE host", "Imported from https://git.example.net/org/repo/issues/42", "https://git.example.net/org/repo/issues/42", "42", true},
		{"trailing whitespace", "Imported from https://github.com/o/r/issues/5 \n", "https://github.com/o/r/issues/5", "5", true},
		{"trailing text rejected", "Imported from https://github.com/o/r/issues/5 and more", "", "", false},
		{"leading text rejected", "See: Imported from https://github.com/o/r/issues/5", "", "", false},
		{"free-text mention rejected", "Filed while shipping PR #671; see https://github.com/o/r/issues/5", "", "", false},
		{"http rejected", "Imported from http://github.com/o/r/issues/5", "", "", false},
		{"non-numeric rejected", "Imported from https://github.com/o/r/issues/abc", "", "", false},
		{"other path rejected", "Imported from https://github.com/o/r/discussions/5", "", "", false},
		{"empty", "", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			url, number, ok := parseLegacyImportNotes(tt.notes)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.wantURL, url)
			assert.Equal(t, tt.wantNumber, number)
		})
	}
}

func TestRunBacklogItemLegacyImportExternalBackfill_should_FillOnlyEmptyLegacyItems_And_BeIdempotent(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	legacy, err := repo.client.BacklogItem.Create().SetTitle("legacy").SetStatus("idea").
		SetNotes("Imported from https://github.com/o/r/issues/9").Save(ctx)
	require.NoError(t, err)
	populated, err := repo.client.BacklogItem.Create().SetTitle("populated").SetStatus("idea").
		SetNotes("Imported from https://github.com/o/r/issues/10").
		SetExternalURL("https://github.com/o/other/issues/1").SetExternalID("1").Save(ctx)
	require.NoError(t, err)
	partial, err := repo.client.BacklogItem.Create().SetTitle("partial").SetStatus("idea").
		SetNotes("Imported from https://github.com/o/r/issues/11").SetExternalID("keep").Save(ctx)
	require.NoError(t, err)
	freeText, err := repo.client.BacklogItem.Create().SetTitle("free text").SetStatus("idea").
		SetNotes("Filed while shipping PR #671, see https://github.com/o/r/issues/12").Save(ctx)
	require.NoError(t, err)
	trailing, err := repo.client.BacklogItem.Create().SetTitle("trailing").SetStatus("idea").
		SetNotes("Imported from https://github.com/o/r/issues/13 (re-triaged)").Save(ctx)
	require.NoError(t, err)

	for pass := 0; pass < 2; pass++ { // second pass proves idempotency
		require.NoError(t, runBacklogItemLegacyImportExternalBackfill(ctx, repo))

		got, err := repo.client.BacklogItem.Get(ctx, legacy.ID)
		require.NoError(t, err)
		assert.Equal(t, "https://github.com/o/r/issues/9", got.ExternalURL)
		assert.Equal(t, "9", got.ExternalID)
		assert.True(t, got.UpdatedAt.Equal(legacy.UpdatedAt), "updated_at must be preserved")

		got, err = repo.client.BacklogItem.Get(ctx, populated.ID)
		require.NoError(t, err)
		assert.Equal(t, "https://github.com/o/other/issues/1", got.ExternalURL)
		assert.Equal(t, "1", got.ExternalID)

		got, err = repo.client.BacklogItem.Get(ctx, partial.ID)
		require.NoError(t, err)
		assert.Empty(t, got.ExternalURL)
		assert.Equal(t, "keep", got.ExternalID)

		got, err = repo.client.BacklogItem.Get(ctx, freeText.ID)
		require.NoError(t, err)
		assert.Empty(t, got.ExternalURL)
		assert.Empty(t, got.ExternalID)

		got, err = repo.client.BacklogItem.Get(ctx, trailing.ID)
		require.NoError(t, err)
		assert.Empty(t, got.ExternalURL)
		assert.Empty(t, got.ExternalID)
	}
}
