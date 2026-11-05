package dbobs

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsBusyError(t *testing.T) {
	assert.True(t, isBusyError(fmt.Errorf("database is locked (5) (SQLITE_BUSY)")))
	assert.True(t, isBusyError(fmt.Errorf("wrapped: %w", fmt.Errorf("database table is locked (6) (SQLITE_LOCKED)"))))
	assert.False(t, isBusyError(fmt.Errorf("no such table: t")))
	assert.False(t, isBusyError(nil))
}

// A second writer on a separate connection with busy_timeout=0 gets SQLITE_BUSY
// at once; the collector must count it.
func TestBusyErrorsAreCounted(t *testing.T) {
	path := "file:" + t.TempDir() + "/busy.db?_pragma=journal_mode(WAL)&_pragma=busy_timeout(0)"
	holder, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = holder.Close() })
	holder.SetMaxOpenConns(1)
	_, err = holder.Exec("CREATE TABLE t (v TEXT)")
	require.NoError(t, err)
	tx, err := holder.Begin()
	require.NoError(t, err)
	_, err = tx.Exec("INSERT INTO t (v) VALUES ('held')")
	require.NoError(t, err)

	other, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = other.Close() })
	other.SetMaxOpenConns(1)
	drv := Wrap(entsql.OpenDB(dialect.SQLite, other), other, Config{Name: "busytest", Warn: func(string, ...any) {}})
	t.Cleanup(func() { _ = drv.Close() })

	var res sql.Result
	require.Error(t, drv.Exec(context.Background(), "INSERT INTO t (v) VALUES (?)", []any{"x"}, &res))
	require.NoError(t, tx.Rollback())

	for _, s := range Snapshot() {
		if s.Name == "busytest" {
			assert.EqualValues(t, 1, s.BusyErrors)
			assert.EqualValues(t, 1, s.Errors)
			return
		}
	}
	t.Fatal("busytest pool missing from Snapshot")
}
