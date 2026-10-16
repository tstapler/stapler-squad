package notifications

import (
	"context"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// multiReasonTitle is the title of the escalation notification, which is always
// backed by an open multiple_reasons stuck row (session/backlog_lifecycle.go).
const multiReasonTitle = "Multiple stuck reasons open"

// ItemState is what the reaper needs to know about one backlog item.
type ItemState struct {
	Terminal     bool // done or archived: nothing left to decide
	HasOpenStuck bool // at least one unresolved stuck row
}

// ItemStateLookup returns the state of each requested backlog item. Items
// missing from the result are left alone.
type ItemStateLookup func(ctx context.Context, itemIDs []string) (map[string]ItemState, error)

// ReapResolvedItemNotifications marks unread item-scoped notifications read once
// the item no longer needs a decision: every one for a terminal item, and the
// multi-reason escalation once the item has no open stuck row. Stuck-state
// resolution never retired these, so they piled up in "Needs a decision".
func ReapResolvedItemNotifications(ctx context.Context, store *NotificationHistoryStore, lookup ItemStateLookup) (int, error) {
	records, _, err := store.List(ListOptions{UnreadOnly: true, Limit: MaxNotifications})
	if err != nil {
		return 0, err
	}
	seen := map[string]struct{}{}
	var ids []string
	for _, r := range records {
		id := r.Metadata["item_id"]
		if _, dup := seen[id]; id == "" || dup {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return 0, nil
	}
	states, err := lookup(ctx, ids)
	if err != nil {
		return 0, err
	}
	return store.MarkReadWhere(func(r *NotificationRecord) bool {
		st, ok := states[r.Metadata["item_id"]]
		if !ok {
			return false
		}
		return st.Terminal || (!st.HasOpenStuck && r.Title == multiReasonTitle)
	})
}

// StartResolvedItemReaper runs ReapResolvedItemNotifications once immediately
// (clearing the existing backlog of stale rows) and then every interval until
// ctx is canceled.
func StartResolvedItemReaper(ctx context.Context, store *NotificationHistoryStore, lookup ItemStateLookup, interval time.Duration, wg *sync.WaitGroup) {
	if store == nil || lookup == nil {
		return
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		reap := func() {
			n, err := ReapResolvedItemNotifications(ctx, store, lookup)
			if err != nil {
				log.Warn("NotificationHistoryStore: resolved-item reap failed", "err", err)
			} else if n > 0 {
				log.Info("NotificationHistoryStore: marked resolved-item notifications read", "count", n)
			}
		}
		reap()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				reap()
			}
		}
	}()
}
