package session

// storage_backlog_jules.go — the Jules-specific EntRepository queries (Story
// 2.1.2), split out of storage_backlog.go to keep that file under the
// file-length-limit gate (`make ready-complexity-gate`) rather than growing
// it past 1000 non-comment lines. Mirrors this repo's existing convention of
// isolating Jules feature code into dedicated `*_jules.go` files (e.g.
// server/services/backlog_service_jules.go, jules_dispatch_service.go).

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/ent/itemsession"
)

// julesPendingUUIDPrefix marks a jules_work ItemSession's session_uuid as a
// reservation written before the CreateSession POST, swapped for the real
// "jules-sessions/{id}" name once the call succeeds (ADR-004). Declared here
// rather than imported from server/services (which will hold the write-side
// copy, e.g. jules_dispatch_service.go) because that package imports session,
// so the reverse import would cycle — same precedent as headlessTriageUUIDPrefix/
// headlessTriageSessionUUIDPrefix (session/backlog_lifecycle_triage.go:43-50).
// Keep byte-identical with the server/services copy; a future value change
// must update both.
const julesPendingUUIDPrefix = "jules-pending-"

// julesDispatchFailedEndReason marks a jules_work ItemSession that was created
// as a reservation but never reached a real Jules session — the CreateSession
// call itself failed. Excluded from CountJulesItemSessionsSince alongside
// julesPendingUUIDPrefix rows so a bad key or Jules outage doesn't burn the
// daily dispatch cap on attempts nothing was ever billed for.
const julesDispatchFailedEndReason = "dispatch_failed"

// ListOpenJulesItemSessions returns every not-yet-ended jules_work ItemSession
// across all backlog items, joined with enough of its parent item's metadata
// (via the ItemSessionBacklogEntry shape already returned by
// GetAllItemSessionsWithBacklogInfo) for the dispatcher/poller to act on it
// without a second query.
func (r *EntRepository) ListOpenJulesItemSessions(ctx context.Context) ([]ItemSessionBacklogEntry, error) {
	sessions, err := r.client.ItemSession.Query().
		Where(
			itemsession.SessionRole(SessionRoleJulesWork),
			itemsession.EndedAtIsNil(),
		).
		WithBacklogItem().
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list open jules item sessions: %w", err)
	}
	results := make([]ItemSessionBacklogEntry, 0, len(sessions))
	for _, is := range sessions {
		if is.Edges.BacklogItem == nil {
			continue
		}
		results = append(results, ItemSessionBacklogEntry{
			SessionUUID: is.SessionUUID,
			SessionRole: is.SessionRole,
			ItemID:      is.Edges.BacklogItem.ID.String(),
			ItemTitle:   is.Edges.BacklogItem.Title,
			ItemStatus:  is.Edges.BacklogItem.Status,
		})
	}
	return results, nil
}

// CountJulesItemSessionsSince counts jules_work ItemSessions created since since
// that reached a confirmed, billed Jules session — feeds MaxJulesSessionsPerDay
// (Story 2.2.2), the daily cloud-spend cap. Excludes rows still carrying the
// julesPendingUUIDPrefix reservation (never confirmed) and rows that ended with
// end_reason "dispatch_failed" (failed at CreateSession), so a bad key or a
// Jules-side outage doesn't consume the day's cap on attempts nothing was
// actually billed for (pre-mortem P2 #5).
func (r *EntRepository) CountJulesItemSessionsSince(ctx context.Context, since time.Time) (int, error) {
	n, err := r.client.ItemSession.Query().
		Where(
			itemsession.SessionRole(SessionRoleJulesWork),
			itemsession.CreatedAtGTE(since),
			itemsession.Not(itemsession.SessionUUIDHasPrefix(julesPendingUUIDPrefix)),
			itemsession.EndReasonNEQ(julesDispatchFailedEndReason),
		).
		Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to count jules item sessions since %s: %w", since, err)
	}
	return n, nil
}

// TouchItemSessionProgress updates only last_progress_at on an ItemSession —
// used by the Jules poller on every non-terminal state observation. Deliberately
// distinct from UpdateItemSessionFileTouch, whose name asserts a filesystem
// event that never happens for a Jules session running on Google's
// infrastructure.
func (r *EntRepository) TouchItemSessionProgress(ctx context.Context, id string, at time.Time) error {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid id %q: %w", id, err)
	}

	_, err = r.client.ItemSession.UpdateOneID(parsedID).
		SetLastProgressAt(at).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to touch progress on item session %s: %w", id, err)
	}

	// Best-effort publish: never blocks or fails the update itself, matching
	// UpdateItemSessionFileTouch's/UpdateItemSessionGitActivity's convention.
	if item, lookupErr := r.backlogItemForItemSession(ctx, parsedID); lookupErr != nil {
		log.WarningLog().Printf("[EntRepository] TouchItemSessionProgress: failed to resolve owning backlog item for item session %s: %v", id, lookupErr)
	} else {
		r.publishItemChanged(ctx, item, BacklogItemChange{
			Kind:          ChangeItemUpdated,
			UpdatedFields: []string{"itemSessions"},
		})
	}

	return nil
}
