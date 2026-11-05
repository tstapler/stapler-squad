package dbsplit

import (
	"context"
	"database/sql"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestIsReadOnly(t *testing.T) {
	yes := []string{"SELECT 1", "select * from t", "  \n\tSELECT x", "/* c */ SELECT 1", "-- c\nSELECT 1", "(SELECT 1)", "SELECT"}
	no := []string{"", "INSERT INTO t VALUES (1)", "UPDATE t SET a=1 RETURNING id", "WITH x AS (SELECT 1) SELECT * FROM x",
		"PRAGMA table_info(t)", "selection", "select_x", "BEGIN", "DELETE FROM t", "/* unterminated SELECT 1", "-- only comment"}
	for _, q := range yes {
		require.True(t, IsReadOnly(q), q)
	}
	for _, q := range no {
		require.False(t, IsReadOnly(q), q)
	}
}

func open(t *testing.T, dsn string, conns int) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	db.SetMaxOpenConns(conns)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestRoutingAndConcurrentReadsDuringTx(t *testing.T) {
	path := t.TempDir() + "/t.db"
	wdb := open(t, "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", 1)
	rdb := open(t, "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=query_only(1)", 4)
	drv := New(entsql.OpenDB(dialect.SQLite, wdb), entsql.OpenDB(dialect.SQLite, rdb))
	ctx := context.Background()

	require.NoError(t, drv.Exec(ctx, "CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)", []any{}, nil))
	require.NoError(t, drv.Exec(ctx, "INSERT INTO t (v) VALUES ('a')", []any{}, nil))

	// A writer transaction holds the only writer connection; reads must not queue behind it.
	tx, err := drv.Tx(ctx)
	require.NoError(t, err)
	require.NoError(t, tx.Exec(ctx, "INSERT INTO t (v) VALUES ('b')", []any{}, nil))

	var rows entsql.Rows
	require.NoError(t, drv.Query(ctx, "SELECT count(*) FROM t", []any{}, &rows))
	var n int
	require.True(t, rows.Next())
	require.NoError(t, rows.Scan(&n))
	require.NoError(t, rows.Close())
	require.Equal(t, 1, n, "reader sees committed state only")
	require.NoError(t, tx.Commit())

	rows = entsql.Rows{}
	require.NoError(t, drv.Query(ctx, "SELECT count(*) FROM t", []any{}, &rows))
	require.True(t, rows.Next())
	require.NoError(t, rows.Scan(&n))
	require.NoError(t, rows.Close())
	require.Equal(t, 2, n, "read after commit sees the write")

	// A write sent through Query (RETURNING) lands on the writer, not the query_only reader.
	rows = entsql.Rows{}
	require.NoError(t, drv.Query(ctx, "INSERT INTO t (v) VALUES ('c') RETURNING id", []any{}, &rows))
	require.NoError(t, rows.Close())

	// The reader pool rejects writes at the SQLite level.
	require.Error(t, rdb.QueryRow("INSERT INTO t (v) VALUES ('x')").Err())
}
