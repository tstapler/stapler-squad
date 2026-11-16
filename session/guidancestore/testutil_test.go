package guidancestore

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/internal/entdbtest"
)

// newTestRepo builds a Repo over a fresh migrated in-memory database with the
// pending-list cache wired, as every EntRepository constructor does.
func newTestRepo(t testing.TB) *Repo {
	t.Helper()
	r := New(entdbtest.NewClient(t), nil)
	r.WatchGuidanceWrites()
	return r
}

// createTestBacklogItemForGuidance inserts a minimal BacklogItem to scope a
// backlog-item GuidanceRequest against.
func createTestBacklogItemForGuidance(t testing.TB, repo *Repo) uuid.UUID {
	t.Helper()
	item, err := repo.client.BacklogItem.Create().SetTitle("guidance test item").Save(context.Background())
	require.NoError(t, err)
	return item.ID
}
