package dbpool

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func queryString(t *testing.T, db *sql.DB, q string) string {
	t.Helper()
	var s string
	require.NoError(t, db.QueryRowContext(context.Background(), q).Scan(&s))
	return s
}

func TestOpenCore_should_EnforceWALAndShape_When_OpeningFreshFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "sessions.db")
	core, err := OpenCore(path, 3)
	require.NoError(t, err)
	t.Cleanup(func() { _ = core.Close() })

	assert.Equal(t, "wal", queryString(t, core.Writer(), "PRAGMA journal_mode"))
	assert.Equal(t, "67108864", queryString(t, core.Writer(), "PRAGMA journal_size_limit"))
	assert.Equal(t, 1, core.Writer().Stats().MaxOpenConnections)

	infos := infoFor(core.Path())
	require.Len(t, infos, 1)
	assert.Equal(t, UseCaseCore, infos[0].UseCase)
	assert.Equal(t, "wal", infos[0].JournalMode)
	assert.Equal(t, 3, infos[0].ReaderConns)
}

func TestOpenEscape_should_KeepTelemetryProfile_When_Opened(t *testing.T) {
	t.Parallel()
	esc, err := OpenEscape(filepath.Join(t.TempDir(), "analytics.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = esc.Close() })

	assert.Equal(t, "wal", queryString(t, esc.Writer(), "PRAGMA journal_mode"))
	assert.Equal(t, "1", queryString(t, esc.Writer(), "PRAGMA synchronous"), "telemetry class runs synchronous=NORMAL")
	assert.Equal(t, "1000", queryString(t, esc.Writer(), "PRAGMA wal_autocheckpoint"))
	assert.Equal(t, "2", queryString(t, esc.Writer(), "PRAGMA auto_vacuum"), "incremental")
	assert.Equal(t, "1", queryString(t, esc.Writer(), "PRAGMA foreign_keys"))
}

func TestOpen_should_ConvertRollbackJournalToWAL_When_FileWasNotWAL(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "legacy.db")
	raw, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = raw.Exec("PRAGMA journal_mode=DELETE")
	require.NoError(t, err)
	_, err = raw.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY)")
	require.NoError(t, err)
	require.NoError(t, raw.Close())

	core, err := OpenCore(path, 0)
	require.NoError(t, err)
	t.Cleanup(func() { _ = core.Close() })
	assert.Equal(t, "wal", queryString(t, core.Writer(), "PRAGMA journal_mode"))
}

func TestSpecValidate_should_RejectUnsafePaths_And_NonCacheSyncOff(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"", ":memory:", "file:x.db?mode=memory", "x.db?_pragma=a(b)", "x.db#frag"} {
		_, err := Open(CoreSpec(path, 0))
		require.ErrorIs(t, err, errBadPath, path)
	}
	spec := EscapeSpec(filepath.Join(t.TempDir(), "x.db"))
	spec.Profile.Synchronous = "OFF"
	_, err := Open(spec)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "synchronous=OFF"))
}

func TestClose_should_BeIdempotent_And_UnregisterFromSnapshot(t *testing.T) {
	t.Parallel()
	core, err := OpenCore(filepath.Join(t.TempDir(), "sessions.db"), 0)
	require.NoError(t, err)
	require.Len(t, infoFor(core.Path()), 1)

	require.NoError(t, core.Close())
	require.NoError(t, core.Close())
	assert.Empty(t, infoFor(core.Path()))
}

func TestDriverClose_should_CloseTheDatabase(t *testing.T) {
	t.Parallel()
	core, err := OpenCore(filepath.Join(t.TempDir(), "sessions.db"), 2)
	require.NoError(t, err)
	require.NoError(t, core.Driver().Close())
	assert.Empty(t, infoFor(core.Path()))
}

func TestLazy_should_OpenOnFirstUse_And_CloseWhenIdle_Without_ClosingInFlight(t *testing.T) {
	t.Parallel()
	now := time.Unix(1000, 0)
	lz := NewLazy(EscapeSpec(filepath.Join(t.TempDir(), "analytics.db")), time.Minute)
	lz.now = func() time.Time { return now }

	assert.Equal(t, 0, lz.Opens(), "nothing opens before first use")
	closed, err := lz.CloseIdle()
	require.NoError(t, err)
	assert.False(t, closed)

	require.NoError(t, lz.With(func(db *Database) error {
		now = now.Add(2 * time.Minute)
		c, cerr := lz.CloseIdle()
		require.NoError(t, cerr)
		assert.False(t, c, "a database in use is never closed")
		return db.Writer().PingContext(context.Background())
	}))
	assert.Equal(t, 1, lz.Opens())

	now = now.Add(30 * time.Second)
	closed, err = lz.CloseIdle()
	require.NoError(t, err)
	assert.False(t, closed, "not idle long enough")

	now = now.Add(time.Minute)
	closed, err = lz.CloseIdle()
	require.NoError(t, err)
	assert.True(t, closed)

	require.NoError(t, lz.With(func(db *Database) error { return db.Writer().PingContext(context.Background()) }))
	assert.Equal(t, 2, lz.Opens(), "reopens lazily after an idle close")
	require.NoError(t, lz.Close())
}

func TestOpenForeignReadOnly_should_RefuseWrites_And_LeaveJournalModeAlone(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "other.db")
	raw, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = raw.Exec("CREATE TABLE t (id INTEGER)")
	require.NoError(t, err)
	require.NoError(t, raw.Close())

	db, err := OpenForeignReadOnly(path, 0)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec("INSERT INTO t VALUES (1)")
	require.Error(t, err)
	assert.Equal(t, "delete", queryString(t, db, "PRAGMA journal_mode"), "a foreign file's journal mode is not ours to change")
}

func infoFor(path string) []Info {
	var out []Info
	for _, in := range Snapshot() {
		if in.Path == path {
			out = append(out, in)
		}
	}
	return out
}

func TestLazy_should_RefuseUse_When_Closed(t *testing.T) {
	t.Parallel()
	lz := NewLazy(EscapeSpec(filepath.Join(t.TempDir(), "analytics.db")), time.Minute)
	require.NoError(t, lz.With(func(db *Database) error { return db.Writer().PingContext(context.Background()) }))
	require.NoError(t, lz.Close())

	err := lz.With(func(*Database) error { return nil })
	require.ErrorIs(t, err, ErrLazyClosed)
	assert.Equal(t, 1, lz.Opens(), "a closed Lazy never reopens")
}

func TestOpenEscapeEvents_should_OwnItsWriterAndFile_When_OpenedBesideEscape(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	esc, err := OpenEscape(filepath.Join(dir, "analytics.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = esc.Close() })
	ev, err := OpenEscapeEvents(filepath.Join(dir, "db", "escape_events.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = ev.Close() })

	assert.Equal(t, "wal", queryString(t, ev.Writer(), "PRAGMA journal_mode"))
	assert.Equal(t, "1", queryString(t, ev.Writer(), "PRAGMA synchronous"))
	assert.Equal(t, "2", queryString(t, ev.Writer(), "PRAGMA auto_vacuum"))
	assert.Equal(t, 1, ev.Writer().Stats().MaxOpenConnections)
	assert.NotSame(t, esc.Writer(), ev.Writer(), "two files, two writers")

	infos := infoFor(ev.Path())
	require.Len(t, infos, 1)
	assert.Equal(t, UseCaseEscapeEvents, infos[0].UseCase)
	assert.Equal(t, "escape-events", infos[0].Pool)
}
