package session

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// A transaction holding the single writer connection must not block reads:
// they run on the query_only pool (STAPLER_SQUAD_DB_READ_CONNS).
func TestEntRepository_ReadsProceedWhileWriterTxOpen(t *testing.T) {
	repo, err := NewEntRepository(WithDatabasePath(filepath.Join(t.TempDir(), "s.db")))
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo.Close() })

	ctx := context.Background()
	tx, err := repo.client.Tx(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()

	done := make(chan error, 1)
	go func() {
		_, err := repo.client.Session.Query().Count(ctx)
		done <- err
	}()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("read queued behind the writer transaction; reader pool not in use")
	}
}

func TestEntRepository_ReadPoolDisabledByEnv(t *testing.T) {
	t.Setenv(ReadPoolEnv, "0")
	repo, err := NewEntRepository(WithDatabasePath(filepath.Join(t.TempDir(), "s.db")))
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo.Close() })

	ctx := context.Background()
	tx, err := repo.client.Tx(ctx)
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() {
		_, err := repo.client.Session.Query().Count(ctx)
		done <- err
	}()
	select {
	case <-done:
		t.Fatal("read completed while the only connection was held; split not disabled")
	case <-time.After(300 * time.Millisecond):
	}
	require.NoError(t, tx.Rollback())
	require.NoError(t, <-done)
}

func TestIsOnDiskDatabase(t *testing.T) {
	require.True(t, isOnDiskDatabase("/tmp/x/s.db"))
	for _, p := range []string{"", ":memory:", "file:x?mode=memory&cache=shared", "file:/tmp/x.db"} {
		require.False(t, isOnDiskDatabase(p), p)
	}
}

// ":memory:" must stay single-pool: a second pool would open a separate empty database.
func TestEntRepository_InMemoryDatabaseKeepsSchemaVisibleToReads(t *testing.T) {
	repo, err := NewEntRepository(WithDatabasePath(":memory:"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo.Close() })
	_, err = repo.client.Session.Query().Count(context.Background())
	require.NoError(t, err)
}
