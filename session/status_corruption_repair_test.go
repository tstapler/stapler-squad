package session

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestStatusCorruptionRepair_FixesOnceThenStaysStable is the property the
// removed runStatusRemap migration got wrong (see
// ent_repository_migrations.go's doc comment): its own output (a +100
// shifted value) could re-trigger its own "needs fixing" condition on the
// next restart, compounding without bound. This test proves the
// replacement repair cannot do that — the repaired value is always back
// inside the valid range, so a second (and third) "restart" against the
// same on-disk database leaves it untouched instead of shifting it again.
func TestStatusCorruptionRepair_FixesOnceThenStaysStable(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "sessions.db")
	ctx := context.Background()

	// "Restart" 1: create a session already corrupted the way the removed
	// migration used to leave one — status = PermanentlyFailed (7) shifted
	// by two restarts' worth of +100, i.e. 207. Status is a plain int type
	// with no compile-time range check, exactly like the raw SQL the old
	// migration and ent's generated SetStatus(int) both write with.
	repo, err := NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)
	data := createTestSession("corrupted")
	data.Status = Status(207)
	require.NoError(t, repo.Create(ctx, data))
	require.NoError(t, repo.Close())

	// "Restart" 2: NewEntRepository runs startupMigrations, including
	// statusCorruptionRepairMigration — the corrupted row must come back
	// repaired to its recoverable original value (207 % 100 = 7).
	repo, err = NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)
	got, err := repo.Get(ctx, "corrupted")
	require.NoError(t, err)
	require.Equal(t, PermanentlyFailed, got.Status, "restart 2: corrupted status 207 was not repaired to its recoverable value (PermanentlyFailed=7)")
	require.NoError(t, repo.Close())

	// "Restart" 3: the repaired value (7) is inside the valid range, so the
	// repair's own query (status > 8) must not select it again. If it did
	// — the exact bug class being guarded against — this would either
	// error or, worse, silently reapply some transformation.
	repo, err = NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)
	got, err = repo.Get(ctx, "corrupted")
	require.NoError(t, err)
	require.Equal(t, PermanentlyFailed, got.Status, "restart 3: already-repaired status changed again — the repair is compounding, not idempotent")
	require.NoError(t, repo.Close())
}

// TestStatusCorruptionRepair_UnrecoverableValueLeftUnchanged covers a status
// corrupted by something other than the known +100 shift (never observed in
// practice, but the repair must not guess): mod 100 is still out of range,
// so the row is left as-is rather than silently rewritten to some assumed
// value.
func TestStatusCorruptionRepair_UnrecoverableValueLeftUnchanged(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "sessions.db")
	ctx := context.Background()

	repo, err := NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)
	data := createTestSession("unrecoverable")
	data.Status = Status(99) // 99 % 100 == 99, still > maxValidPersistedStatus
	require.NoError(t, repo.Create(ctx, data))
	require.NoError(t, repo.Close())

	repo, err = NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)
	got, err := repo.Get(ctx, "unrecoverable")
	require.NoError(t, err)
	require.Equal(t, Status(99), got.Status, "an unrecoverable status must be left unchanged, not guessed at")
	require.NoError(t, repo.Close())
}

// TestStatusCorruptionRepair_ValueOutsideShiftEligibleRangeLeftUnchanged
// covers the specific gap an adversarial review of this migration found: a
// status like 103 decodes via %100 to 3 (Stopped), which IS inside the
// overall valid range [0,8] — but the removed migration's own "is this
// legacy data" check was `status > 4`, so 0-4 (Creating/Active/Paused/
// Stopped/Hibernated) were never eligible for its +100 shift in the first
// place. A repair that accepted any value in [0,8] would confidently
// rewrite 103 to Stopped=3 — turning a loud, diagnosable error into a
// silently wrong value for a status that was never even corrupted by the
// mechanism this migration exists to undo. It must be left unchanged.
func TestStatusCorruptionRepair_ValueOutsideShiftEligibleRangeLeftUnchanged(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "sessions.db")
	ctx := context.Background()

	repo, err := NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)
	data := createTestSession("not-shift-eligible")
	data.Status = Status(103) // 103 % 100 == 3 (Stopped) — in [0,8] but not in the shift-eligible [5,8] range
	require.NoError(t, repo.Create(ctx, data))
	require.NoError(t, repo.Close())

	repo, err = NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)
	got, err := repo.Get(ctx, "not-shift-eligible")
	require.NoError(t, err)
	require.Equal(t, Status(103), got.Status, "a status whose recovered value was never shift-eligible must be left unchanged, not silently repaired to a plausible-but-wrong value")
	require.NoError(t, repo.Close())
}
