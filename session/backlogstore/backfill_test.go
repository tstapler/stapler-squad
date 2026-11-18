package backlogstore

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func backlogWithDbPath(path func() string) *Backlog {
	return New(nil, Ports{DbPath: path})
}

func TestBackfillLockFilePath_SkipsInMemoryDSN(t *testing.T) {
	t.Parallel()
	require.Empty(t, backlogWithDbPath(func() string { return ":memory:" }).backfillLockFilePath("x.lock"))
	require.Equal(t, filepath.Join("/data", "x.lock"), backlogWithDbPath(func() string { return "/data/sessions.db" }).backfillLockFilePath("x.lock"))
}

func TestBackfillLockFilePath_should_SkipLocking_When_PathEmptyFileURIOrPortUnset(t *testing.T) {
	t.Parallel()
	require.Empty(t, backlogWithDbPath(func() string { return "" }).backfillLockFilePath("x.lock"), "empty path (NewEntRepositoryFromClient) must skip locking")
	require.Empty(t, backlogWithDbPath(func() string { return "file:x?mode=memory&cache=shared" }).backfillLockFilePath("x.lock"))
	require.Empty(t, backlogWithDbPath(nil).backfillLockFilePath("x.lock"), "a nil port behaves as an empty path")
}

func TestBackfillLockFilePath_should_ReadDbPathAtCallTime(t *testing.T) {
	t.Parallel()
	path := "/first/sessions.db"
	b := backlogWithDbPath(func() string { return path })
	require.Equal(t, filepath.Join("/first", "x.lock"), b.backfillLockFilePath("x.lock"))
	path = "/second/sessions.db"
	require.Equal(t, filepath.Join("/second", "x.lock"), b.backfillLockFilePath("x.lock"), "a path changed after construction must be seen")
}
