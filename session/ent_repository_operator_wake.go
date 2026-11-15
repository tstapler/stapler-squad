package session

import (
	"context"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/waketrigger"
)

// Operator-facing reads of wake triggers (Story 4.6); peer-help rows live in session/guidancestore.

// ListWakeTriggersByStatus returns the rows of every owner in any of statuses,
// newest first. The operator list needs all owners, which ListWakeTriggersByOwner
// cannot give.
func (r *EntRepository) ListWakeTriggersByStatus(ctx context.Context, statuses ...domain.WakeStatus) ([]*domain.WakeTriggerData, error) {
	if len(statuses) == 0 {
		return nil, nil
	}
	return r.listWakeTriggers(ctx, r.Client.WakeTrigger.Query().
		Where(waketrigger.StatusIn(wakeStatusStrings(statuses)...)).
		Order(ent.Desc(waketrigger.FieldCreatedAt)))
}
