package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/tstapler/stapler-squad/server/notifications"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// backlogItemStateLookup adapts Storage to the notification reaper. A deleted
// item counts as terminal; any other lookup error aborts the pass so a
// transient fault can't retire a live decision.
func backlogItemStateLookup(storage *session.Storage) notifications.ItemStateLookup {
	return func(ctx context.Context, itemIDs []string) (map[string]notifications.ItemState, error) {
		openStuck, err := storage.FindOpenStuckStates(ctx)
		if err != nil {
			return nil, fmt.Errorf("find open stuck states: %w", err)
		}
		hasStuck := make(map[string]bool, len(openStuck))
		for _, row := range openStuck {
			hasStuck[row.ItemID] = true
		}

		states := make(map[string]notifications.ItemState, len(itemIDs))
		for _, id := range itemIDs {
			item, err := storage.GetBacklogItem(ctx, id)
			if errors.Is(err, session.ErrNotFound) {
				states[id] = notifications.ItemState{Terminal: true}
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("get backlog item %s: %w", id, err)
			}
			status := domain.BacklogStatus(item.Status)
			states[id] = notifications.ItemState{
				Terminal:     status == domain.BacklogStatusDone || status == domain.BacklogStatusArchived,
				HasOpenStuck: hasStuck[id],
			}
		}
		return states, nil
	}
}
