package session

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite" // Pure Go SQLite driver
)

func TestBackfillLockFilePath_SkipsInMemoryDSN(t *testing.T) {
	require.Empty(t, (&EntRepository{DbPath: ":memory:"}).backfillLockFilePath("x.lock"))
	require.Equal(t, filepath.Join("/data", "x.lock"), (&EntRepository{DbPath: "/data/sessions.db"}).backfillLockFilePath("x.lock"))
}
