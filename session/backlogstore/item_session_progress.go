package backlogstore

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
func (b *Backlog) TouchItemSessionProgress(ctx context.Context, id string, at time.Time) error {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid id %q: %w", id, err)
	}

	_, err = b.client.ItemSession.UpdateOneID(parsedID).
		SetLastProgressAt(at).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to touch progress on item session %s: %w", id, err)
	}

	// Best-effort publish: never blocks or fails the update itself, matching
	// UpdateItemSessionFileTouch's/UpdateItemSessionGitActivity's convention.
	if item, lookupErr := b.backlogItemForItemSession(ctx, parsedID); lookupErr != nil {
		log.WarningLog().Printf("[EntRepository] TouchItemSessionProgress: failed to resolve owning backlog item for item session %s: %v", id, lookupErr)
	} else {
		b.PublishItemChanged(ctx, item, backlogdto.BacklogItemChange{
			Kind:          backlogdto.ChangeItemUpdated,
			UpdatedFields: []string{"itemSessions"},
		})
	}

	return nil
}
