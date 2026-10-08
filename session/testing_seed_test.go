package session

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/internal/sqlitedsn"
)

// A second connection opened on a test repository's DSN (as
// TestBackdateItemSessionCreatedAt does) must see the seeded schema. A
// sqlite3_deserialize-based seed detaches the connection from the shared cache and
// fails this with "no such table".
func TestNewTestEntRepository_SeededSchemaIsVisibleToSecondConnection(t *testing.T) {
	repo := NewTestEntRepository(t)

	db, err := sql.Open("sqlite", sqlitedsn.New(repo.dbPath).Build())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	var n int
	require.NoError(t, db.QueryRowContext(context.Background(),
		"SELECT count(*) FROM sqlite_master WHERE type='table' AND name='item_sessions'").Scan(&n))
	assert.Equal(t, 1, n)
}

func TestNewTestEntRepository_RepositoriesAreIsolatedFromEachOtherAndTheTemplate(t *testing.T) {
	storageA, err := NewStorageWithRepository(NewTestEntRepository(t))
	require.NoError(t, err)
	storageB, err := NewStorageWithRepository(NewTestEntRepository(t))
	require.NoError(t, err)

	item, err := storageA.CreateBacklogItem(t.Context(), BacklogItemData{Title: "only in A", Status: "ready"})
	require.NoError(t, err)
	_, err = storageB.GetBacklogItem(t.Context(), item.ID)
	assert.Error(t, err, "a write to one test repository must not appear in another")

	storageC, err := NewStorageWithRepository(NewTestEntRepository(t))
	require.NoError(t, err)
	_, err = storageC.GetBacklogItem(t.Context(), item.ID)
	assert.Error(t, err, "writes must not leak back into the template")
}
