package session

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNewEntRepository_RestartDoesNotAlterStatusValuesAbove4 guards against a
// regression of the removed runStatusRemap migration (see
// ent_repository_migrations.go's doc comment): that migration treated any
// persisted status > 4 as legacy data needing a remap, which was true only
// for the old 7-value session.Status iota it was written against. Once
// session.Status grew past 5 values (Restoring=5, Crashed=6,
// PermanentlyFailed=7, Failed=8+), every restart corrupted any session
// sitting in one of those states by shifting its stored status +100 and
// never mapping it back down — repeated restarts drove the value arbitrarily
// high (observed status=66707 in production).
//
// This test opens the same on-disk database three times in a row, simulating
// three service restarts, and asserts a session's status is byte-for-byte
// unchanged across all of them for every status value the state machine can
// actually leave a session parked in (not just the 0-4 range the old
// migration assumed was exhaustive).
func TestNewEntRepository_RestartDoesNotAlterStatusValuesAbove4(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "sessions.db")
	ctx := context.Background()

	statuses := []Status{Creating, Active, Paused, Stopped, Hibernated, Crashed, PermanentlyFailed, Failed}

	// "Restart" 1: create one session per status value.
	repo, err := NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)
	for _, status := range statuses {
		data := createTestSession(status.String())
		data.Status = status
		require.NoError(t, repo.Create(ctx, data))
	}
	require.NoError(t, repo.Close())

	// "Restart" 2 and 3: reopen the same file — this is exactly what
	// NewEntRepository does on every real service restart — and confirm no
	// status value drifted from what was persisted in "restart" 1.
	for restart := 2; restart <= 3; restart++ {
		repo, err = NewEntRepository(WithDatabasePath(dbPath))
		require.NoError(t, err)
		for _, status := range statuses {
			got, err := repo.Get(ctx, status.String())
			require.NoErrorf(t, err, "restart %d: get session for status %s", restart, status)
			require.Equalf(t, status, got.Status,
				"restart %d: status for session %q drifted from %d to %d — the removed status_remap migration's compounding-shift bug has regressed",
				restart, status.String(), status, got.Status)
		}
		require.NoError(t, repo.Close())
	}
}
