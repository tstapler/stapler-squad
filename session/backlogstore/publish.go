package backlogstore

import (
	"context"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogstatusevent"
)

// PublishItemChanged eager-loads item's ItemSessions and StatusEvents (see
// AttachItemSessionsForPublish/attachStatusEventsForPublish) and then
// publishes via hooks.PublishItemChanged. This is the entry point every
// call site should use except DeleteBacklogItem, whose snapshot must be
// captured before the item's sessions are deleted and therefore cannot be
// re-queried here — see hooks.PublishItemChanged's doc comment.
func (b *Backlog) PublishItemChanged(ctx context.Context, item *backlogdto.BacklogItemData, change backlogdto.BacklogItemChange) {
	b.AttachItemSessionsForPublish(ctx, item)
	b.attachStatusEventsForPublish(ctx, item)
	b.guidance.AttachPendingGuidance(ctx, item)
	b.hooks.PublishItemChanged(item, change)
}

// AttachItemSessionsForPublish best-effort loads and attaches this item's
// ItemSessions (with ReviewVerdict) onto data before it's handed to
// PublishItemChanged.
//
// Every publish-hook call site builds its BacklogItemData from a query or
// mutation (Get/UpdateOneID().Save()) that does NOT eager-load the
// item_sessions edge — unlike the REST list/detail read paths
// (ListBacklogItemSummaries, GetBacklogItem's sibling patterns), which do.
// backlogItemToProto (server/services/backlog_service.go) only populates the
// wire ItemSessions field "when eagerly loaded", and the frontend derives
// gateVerdict/gateVerdictSummary/triageStatus entirely from itemSessions
// (web-app/src/lib/hooks/useBacklogService.ts's mapBacklogItem) — so without
// this, every live BacklogItemEvent would silently blank those fields in the
// Redux store (backlogItemsSlice's upsertItem does a wholesale replace, not a
// field merge) even though the initial REST snapshot always carries them
// correctly. This was a confirmed regression found by a Phase 5
// spec-compliance sweep (docs/tasks/backlog-feature-improvement.md) — actively
// worse than the pre-event-driven shouldPoll baseline.
//
// Reuses ListItemSessions (the same tested query the REST list/detail paths
// already run) so the live-event snapshot and the REST snapshot are always
// built from identical data, rather than duplicating the edge-loading query
// at each of the ~10 call sites. Best-effort: a lookup failure is logged and
// skipped, never fails the mutation that already succeeded.
func (b *Backlog) AttachItemSessionsForPublish(ctx context.Context, data *backlogdto.BacklogItemData) {
	if data == nil || data.ID == "" {
		return
	}
	sessions, err := b.ListItemSessions(ctx, data.ID)
	if err != nil {
		log.WarningLog().Printf("[EntRepository] AttachItemSessionsForPublish: failed to load item sessions for item %s: %v", data.ID, err)
		return
	}
	data.ItemSessions = sessions
}

// MaxPublishedStatusEvents caps how many status-transition rows a single
// attachStatusEventsForPublish call loads and re-broadcasts. Without this,
// an item stuck in a transition crash-loop (each retry appends one more
// BacklogStatusEvent row) makes every subsequent publish reload and
// re-broadcast the item's *entire*, ever-growing history — an O(N) cost per
// transition that compounds into unbounded live-heap growth under a fast
// retry loop (confirmed via pprof: this call path was 93% of a 7.6GB live
// heap after ~80 minutes of a stuck session's retry storm). Same cap/pattern
// as maxSourceSyncEventsHistory above; 200 is far more than any legitimate
// transition history needs for the live-update UI merge this exists for
// (see doc comment below) while bounding the pathological case.
const MaxPublishedStatusEvents = 200

// attachStatusEventsForPublish best-effort loads and attaches this item's
// most recent status-transition audit trail (capped at
// MaxPublishedStatusEvents) onto data before it's handed to
// PublishItemChanged, mirroring AttachItemSessionsForPublish above.
//
// Every publish-hook call site builds its BacklogItemData from a plain
// BacklogItem.Get() (e.g. TransitionBacklogItemStatus re-reads the row right
// after recordStatusEvent appends the new row) rather than a query with
// WithStatusEvents(), unlike GetBacklogItem's REST read path — so without
// this, every live BacklogItemEvent following a status transition carries an
// empty StatusEvents slice. BacklogItemDetail.tsx's live-watch merge effect
// does a wholesale item replace (not a field merge), so that empty slice
// immediately stomps the correctly-populated StatusEvents a prior
// GetBacklogItem() fetch had loaded — the Workflow section reverts to "No
// status history recorded" moments after a real transition even though the
// audit rows are persisted correctly. Best-effort: a lookup failure is
// logged and skipped, never fails the mutation that already succeeded.
func (b *Backlog) attachStatusEventsForPublish(ctx context.Context, data *backlogdto.BacklogItemData) {
	if data == nil || data.ID == "" {
		return
	}
	parsedID, err := b.ResolveBacklogItemLookup(ctx, data.ID)
	if err != nil {
		log.WarningLog().Printf("[EntRepository] attachStatusEventsForPublish: invalid item id %s: %v", data.ID, err)
		return
	}
	// Fetch most-recent-first (capped) then reverse, so the attached slice
	// keeps its existing ascending (oldest-first) contract for consumers.
	events, err := b.client.BacklogStatusEvent.Query().
		Where(backlogstatusevent.ItemID(parsedID)).
		Order(ent.Desc(backlogstatusevent.FieldCreatedAt)).
		Limit(MaxPublishedStatusEvents).
		All(ctx)
	if err != nil {
		log.WarningLog().Printf("[EntRepository] attachStatusEventsForPublish: failed to load status events for item %s: %v", data.ID, err)
		return
	}
	data.StatusEvents = make([]backlogdto.BacklogStatusEventData, len(events))
	for i, ev := range events {
		data.StatusEvents[len(events)-1-i] = BacklogStatusEventToData(ev)
	}
}
