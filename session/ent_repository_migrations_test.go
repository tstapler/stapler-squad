package session

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultStartupMigrations_OrderPinned(t *testing.T) {
	t.Parallel()
	migrations := DefaultStartupMigrations()
	names := make([]string, 0, len(migrations))
	for _, m := range migrations {
		names = append(names, m.Name())
	}
	assert.Equal(t, []string{
		"drop obsolete indexes",
		"backlog item updated_at UTC backfill",
		"workflow updated_at UTC backfill",
		"github pr url backfill",
		"backlog item public id backfill",
		"backlog item repo_path canonicalization",
		"status corruption repair",
		"backlog item legacy import external backfill",
	}, names)
}

func TestNewEntRepository_WithMigrationsOverridesDefault(t *testing.T) {
	t.Parallel()
	var ran []string
	rec := func(n string) Migration {
		return funcMigration{n, func(context.Context, *EntRepository) error { ran = append(ran, n); return nil }}
	}
	repo, err := NewEntRepository(
		WithDatabasePath(filepath.Join(t.TempDir(), "s.db")),
		WithMigrations(rec("a"), rec("b")),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo.Client.Close() })
	assert.Equal(t, []string{"a", "b"}, ran)
}
