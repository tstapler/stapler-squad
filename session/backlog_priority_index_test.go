package session

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// Schema.Create (auto-migrate) is how this repo applies schema changes, so
// the (priority, updated_at) index must exist after opening a fresh repo.
func TestBacklogItemsSchema_should_createPriorityUpdatedAtIndex_When_FreshRepo(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "sessions.db")
	repo, err := NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)
	defer repo.Close()

	rawDB, err := sql.Open("sqlite3", dbPath+"?_timeout=5000")
	require.NoError(t, err)
	defer rawDB.Close()

	require.Equal(t, []string{"priority", "updated_at"}, pragmaIndexColumns(t, rawDB, "backlogitem_priority_updated_at"))
}
