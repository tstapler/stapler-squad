// This file mirrors the real server/services/superseded_session_sweeper.go:
// an identical unguarded ArchiveSessionByUUID call, but in a file that does
// NOT start with "diagnose_" — one of the plan's 8 pre-existing bypasses,
// deliberately out of scope for this analyzer (Tech Debt Disposition table).
package services

import "context"

// OUT-OF-SCOPE: same unguarded-archive shape as diagnose_fixture.go's
// archiveFromDiagnose, but in a file outside the matched diagnose_ prefix —
// must NOT be flagged.
func archiveIfNotLive(ctx context.Context, stopper *SessionStopper, inst *Instance) {
	_ = stopper.ArchiveSessionByUUID(ctx, inst.SessionUUID)
}
