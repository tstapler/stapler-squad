package session

import (
	"context"
	"path/filepath"
	"testing"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/stretchr/testify/require"
)

func queryInt(t *testing.T, repo *EntRepository, q string) int {
	t.Helper()
	var rows entsql.Rows
	require.NoError(t, repo.driver.Query(context.Background(), q, []any{}, &rows))
	defer rows.Close()
	require.True(t, rows.Next(), q)
	var n int
	require.NoError(t, rows.Scan(&n))
	return n
}

// Both pools must run with synchronous=NORMAL. Reads route to the reader pool
// (pragma_* table functions are plain SELECTs); a bare PRAGMA routes to the writer.
func TestNewEntRepository_should_ApplyTuningPragmas_When_OpeningOnDiskDatabase(t *testing.T) {
	t.Parallel()
	repo, err := NewEntRepository(WithDatabasePath(filepath.Join(t.TempDir(), "sessions.db")))
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo.Close() })
	require.NotNil(t, repo.driver)

	const normal = 1
	require.Equal(t, normal, queryInt(t, repo, "PRAGMA synchronous"), "writer synchronous")
	require.Equal(t, normal, queryInt(t, repo, "SELECT synchronous FROM pragma_synchronous"), "reader synchronous")
}
