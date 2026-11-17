package session

// ent_repository_backlog_jules.go — the Jules-specific EntRepository queries (Story
// 2.1.2), split out of ent_repository_backlog_items.go to keep that file under the
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
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

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

	_, err = r.Client.ItemSession.UpdateOneID(parsedID).
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
		r.publishItemChanged(ctx, item, backlogdto.BacklogItemChange{
			Kind:          backlogdto.ChangeItemUpdated,
			UpdatedFields: []string{"itemSessions"},
		})
	}

	return nil
}
