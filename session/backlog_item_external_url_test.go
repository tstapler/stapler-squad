package session

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGetBacklogItemByExternalURL_DedupsManualImports guards the dedup lookup
// ImportGitHubIssue uses (server/services/backlog_service_sync.go) to avoid
// creating a second backlog item for a GitHub issue URL that was already
// imported — unlike GetBacklogItemByExternalID, this has no ItemSource to
// scope by, since a manual import creates no source row.
func TestGetBacklogItemByExternalURL_DedupsManualImports(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	const url = "https://github.com/acme/widgets/issues/42"
	created, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:       "Widget bug",
		Status:      string(BacklogStatusIdea),
		ExternalID:  "42",
		ExternalURL: url,
	})
	require.NoError(t, err)

	found, err := storage.GetBacklogItemByExternalURL(ctx, url)
	require.NoError(t, err)
	require.Equal(t, created.ID, found.ID)

	_, err = storage.GetBacklogItemByExternalURL(ctx, "https://github.com/acme/widgets/issues/999")
	require.True(t, errors.Is(err, ErrNotFound))
}
