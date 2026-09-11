package session

// status_corruption_repair.go — one-time idempotent migration that repairs
// rows left behind by the now-removed runStatusRemap migration (see
// ent_repository_migrations.go's doc comment and PR #727). That migration
// ran unconditionally on every process restart and applied a compounding
// +100 sentinel shift to any session sitting in Restoring/Crashed/
// PermanentlyFailed/Failed (all >4) at restart time, since its "is this
// legacy data" check (status > 4) predates those states existing. It was
// deleted once the corruption was understood, which stops any FURTHER
// shifting — but does nothing for rows it had already corrupted before the
// fix shipped. Those rows are still sitting at status = original + 100*n
// for whatever n restarts happened to occur while they were parked in one
// of those states, and every read of them (adapters.StatusToProto) fails
// loudly with "unrecognized session.Status" instead of silently
// misbehaving — confirmed live via repeated log entries for status values
// 207, 334507, and 384507, all ≡ 7 (mod 100) — i.e. three different
// PermanentlyFailed sessions, shifted 2, 3345, and 3845 times respectively
// before #727 landed.
//
// Repair strategy: for any row with status outside the valid range, the
// shift is always a whole-number multiple of 100 (see above), so the
// original value is recoverable exactly via status % 100 — no guessing
// involved, unlike a checksum-less corruption would require.

import (
	"context"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/ent/session"
)

// maxValidPersistedStatus is the highest session.Status constant ever
// legitimately written to storage (session/instance.go). Anything greater
// is corruption, not a newer status this code doesn't know about yet — a
// real new Status value requires a code change here anyway, at which point
// this constant gets bumped in the same commit.
const maxValidPersistedStatus = int(Failed)

// minShiftEligibleStatus is the lowest status the removed migration could
// ever have shifted: its own "is this legacy data" check was `status > 4`,
// so only Restoring(5)/Crashed(6)/PermanentlyFailed(7)/Failed(8) were ever
// eligible — Creating/Active/Paused/Stopped/Hibernated (0-4) never were.
// Recovery below requires the recovered value to land in [5,8], not just
// "somewhere in the full valid range [0,8]": a wider check would let a
// value from some unrelated future corruption source (say, status=103) get
// silently "repaired" to a plausible-looking but wrong value (Stopped=3)
// instead of surfacing as the loud, diagnosable error it currently is —
// worse than not repairing it at all. See
// TestStatusCorruptionRepair_ValueOutsideShiftEligibleRangeLeftUnchanged.
const minShiftEligibleStatus = int(Restoring)

// runStatusCorruptionRepair rewrites any Session row whose stored status is
// outside the valid range by recovering the original value via status %
// 100, but only when that recovered value is itself in the range the
// removed migration could actually have shifted from (see
// minShiftEligibleStatus) — not merely "somewhere valid". Idempotent: rows
// already in range are excluded by the query itself, and re-running against
// an already-repaired row is a no-op (its status is back in range). Safe on
// a fresh/empty database. Best-effort per row: an unrecoverable value
// (status % 100 outside [5,8]) is logged and left alone rather than guessed
// at.
func runStatusCorruptionRepair(ctx context.Context, er *EntRepository) error {
	rows, err := er.client.Session.Query().
		Where(session.StatusGT(maxValidPersistedStatus)).
		All(ctx)
	if err != nil {
		// Table may not exist yet (fresh DB before schema.Create) — ignore,
		// mirroring the removed runStatusRemap's same defensive posture.
		return nil //nolint:nilerr
	}

	var repaired, unrecoverable int
	for _, row := range rows {
		recovered := row.Status % 100
		if recovered < minShiftEligibleStatus || recovered > maxValidPersistedStatus {
			log.WarningLog().Printf("[Migration] status corruption repair: session=%d: status=%d has no recoverable value (mod 100 = %d, outside the shift-eligible range [%d,%d]) — left unchanged", row.ID, row.Status, recovered, minShiftEligibleStatus, maxValidPersistedStatus)
			unrecoverable++
			continue
		}
		if _, saveErr := er.client.Session.UpdateOneID(row.ID).
			SetStatus(recovered).
			Save(ctx); saveErr != nil {
			log.WarningLog().Printf("[Migration] status corruption repair: session=%d: %v", row.ID, saveErr)
			continue
		}
		repaired++
	}
	if repaired > 0 {
		log.InfoLog().Printf("[Migration] status corruption repair: recovered %d row(s) corrupted by the removed status_remap migration", repaired)
	}
	if unrecoverable > 0 {
		log.WarningLog().Printf("[Migration] status corruption repair: %d row(s) had no recoverable status value", unrecoverable)
	}
	return nil
}
