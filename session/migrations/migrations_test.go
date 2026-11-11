package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"sync/atomic"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver

	"github.com/tstapler/stapler-squad/session/ent"
)

var dbCounter atomic.Int64

func newEnv(t *testing.T) Env {
	t.Helper()
	dsn := fmt.Sprintf("file:migrations%d?mode=memory&cache=shared&_fk=1", dbCounter.Add(1))
	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	drv := entsql.OpenDB(dialect.SQLite, db)
	client := ent.NewClient(ent.Driver(drv))
	t.Cleanup(func() { _ = client.Close() })
	require.NoError(t, client.Schema.Create(context.Background()))
	return Env{Client: client, Driver: drv}
}

// The order is pinned so a reorder or accidental drop is a visible diff;
// session's own order test covers the adapted list.
func TestDefault_NamesAndOrderPinned(t *testing.T) {
	t.Parallel()
	defaults := Default(Hooks{})
	names := make([]string, 0, len(defaults))
	for _, m := range defaults {
		names = append(names, m.Name)
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

func TestDropObsoleteIndexes_should_DropExistingIndex_And_NoOpWithoutDriver(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	env := newEnv(t)
	idx := obsoleteIndexes[0]

	var res sql.Result
	require.NoError(t, env.Driver.Exec(ctx, "CREATE TABLE obs_probe (a TEXT)", []any{}, &res))
	require.NoError(t, env.Driver.Exec(ctx, fmt.Sprintf("CREATE INDEX %q ON obs_probe (a)", idx), []any{}, &res))
	require.Equal(t, 1, indexCount(t, env, idx))

	require.NoError(t, DropObsoleteIndexes(ctx, env))
	assert.Equal(t, 0, indexCount(t, env, idx))
	require.NoError(t, DropObsoleteIndexes(ctx, env), "idempotent")

	require.NoError(t, DropObsoleteIndexes(ctx, Env{Client: env.Client}), "nil Driver is a no-op")
}

func indexCount(t *testing.T, env Env, name string) int {
	t.Helper()
	rows := &entsql.Rows{}
	require.NoError(t, env.Driver.Query(context.Background(),
		"SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?", []any{name}, rows))
	defer rows.Close()
	var n int
	require.True(t, rows.Next())
	require.NoError(t, rows.Scan(&n))
	return n
}
