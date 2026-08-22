package services

// backlog_service_events.go — WatchBacklogItems streaming RPC handler for
// BacklogService. Mirrors session_service.go's WatchSessions: subscribe to
// the event bus before building the snapshot, then either replay events
// buffered since after_seq (reconnect) or send a fresh per-item snapshot,
// then fan out live events until the client disconnects.
//
// The registered RPC method (WatchBacklogItems) is a thin wrapper around the
// unexported watchBacklogItems core-logic method, which sends through the
// narrow backlogItemEventSender interface instead of a concrete
// *connect.ServerStream[T]. This exists so Epic 3.2's tests can drive the
// branching logic (fresh snapshot / after_seq replay / live fan-out with
// filters) directly with a fake sender: connectrpc.com/connect v1.19.0's
// ServerStream[Res] is a concrete struct with an unexported `conn` field and
// no exported constructor, so a hand-rolled fake ServerStream[T] is not
// buildable outside the connect package. *connect.ServerStream[
// sessionv1.BacklogItemEvent] already has a matching Send method, so it
// satisfies backlogItemEventSender structurally with zero adapter code.

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"
	"time"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// backlogItemEventSender is the narrow interface watchBacklogItems sends
// through. Satisfied structurally (no explicit "implements") by
// *connect.ServerStream[sessionv1.BacklogItemEvent] in production, and by a
// mutex-guarded fake in tests (server/services/backlog_service_events_test.go).
type backlogItemEventSender interface {
	Send(*sessionv1.BacklogItemEvent) error
}

// testAfterSubscribeHook, when set for a given *events.EventBus, is invoked
// immediately after that bus's Subscribe(ctx) below, before the after_seq
// branch's EventsSince(afterSeq) read. Production code never sets this — it
// exists solely so Epic 3.2's tests can deterministically land a Publish()
// call inside the narrow race window between Subscribe() and EventsSince()
// described in the forceIsSnapshot call site's comment (pre-mortem P2 #4):
// without a seam here, reproducing that specific interleaving depends on
// non-deterministic goroutine scheduling, which cannot be turned into a
// reliable regression test. See backlog_service_events_test.go's
// race-window test for the only caller.
//
// Keyed by *events.EventBus rather than a single package-global func: each
// test builds its own isolated bus via newTestBacklogServiceWithBus, but
// watchBacklogItems is exercised by many t.Parallel() tests in this package
// concurrently. A single global hook fired on every Subscribe() call
// regardless of which bus it belonged to, so the race-window test's hook
// could be (and under real scheduling interleavings, was) triggered a
// second time by an unrelated parallel test's Subscribe() call, publishing
// the race event into this test's bus twice instead of once and doubling
// the expected event count. Scoping the hook to the specific bus instance
// closes that cross-test interference.
//
// testAfterSubscribeHookMu guards concurrent read/write of the map from a
// test goroutine (setter) and every parallel test's watchBacklogItems call
// (reader) — required under -race since the t.Parallel() rollout made this
// package's WatchBacklogItems tests run concurrently. Modeled on
// backlog_service_triage.go's testTriageCompleteHook.
var (
	testAfterSubscribeHookMu sync.Mutex
	testAfterSubscribeHooks  = map[*events.EventBus]func(){}
)

func setTestAfterSubscribeHook(bus *events.EventBus, hook func()) {
	testAfterSubscribeHookMu.Lock()
	defer testAfterSubscribeHookMu.Unlock()
	if hook == nil {
		delete(testAfterSubscribeHooks, bus)
		return
	}
	testAfterSubscribeHooks[bus] = hook
}

func callTestAfterSubscribeHook(bus *events.EventBus) {
	testAfterSubscribeHookMu.Lock()
	hook := testAfterSubscribeHooks[bus]
	testAfterSubscribeHookMu.Unlock()
	if hook != nil {
		hook()
	}
}

// WatchBacklogItems streams real-time backlog item events. Sends an initial
// snapshot (or, on reconnect via after_seq, a replay of buffered events)
// followed by live fan-out, filtered by status_filter/category_filter.
// +api: backlog:watch
func (s *BacklogService) WatchBacklogItems(
	ctx context.Context,
	req *connect.Request[sessionv1.WatchBacklogItemsRequest],
	stream *connect.ServerStream[sessionv1.BacklogItemEvent],
) error {
	return s.watchBacklogItems(ctx, req.Msg, stream)
}

// watchBacklogItems is WatchBacklogItems's core logic, extracted behind the
// backlogItemEventSender interface (see its doc comment) so it is directly
// unit-testable without a real RPC round-trip.
func (s *BacklogService) watchBacklogItems(
	ctx context.Context,
	msg *sessionv1.WatchBacklogItemsRequest,
	sender backlogItemEventSender,
) error {
	if s.eventBus == nil {
		return connect.NewError(connect.CodeUnimplemented, fmt.Errorf("backlog event stream is not available"))
	}

	// Subscribe before building the snapshot/replay batch so no events are
	// lost between the two phases (snapshot races are resolved by
	// client-side upsert semantics) — mirrors session_service.go's
	// WatchSessions ordering-safety comment.
	eventCh, subID := s.eventBus.Subscribe(ctx)
	defer s.eventBus.Unsubscribe(subID)

	callTestAfterSubscribeHook(s.eventBus)

	costFor := s.buildCostLookup()

	// Counts events actually sent during the initial phase below (replay or
	// fresh snapshot) so we can tell whether it sent nothing at all — see the
	// unconditional-marker send after this if/else-if block.
	initialPhaseSent := 0

	if msg.GetAfterSeq() > 0 {
		// Reconnecting client: replay events missed since last disconnect.
		// This covers the window between disconnect and the new
		// subscription above.
		for _, evt := range s.eventBus.EventsSince(msg.GetAfterSeq()) {
			if evt.Type != events.EventBacklogItemChanged || evt.BacklogItemPayload == nil {
				continue
			}
			if !backlogItemMatchesFilters(evt.BacklogItemPayload.Item, msg) {
				continue
			}
			converted := convertEventToBacklogItemEvent(evt, costFor)
			// Force is_snapshot: true on every replayed event, unconditionally,
			// regardless of the value the event was originally published
			// with. A live event published in the race window between
			// Subscribe() (above) and this EventsSince(after_seq) read can be
			// delivered via BOTH this replay branch and the live fan-out loop
			// below. Forcing it here guarantees the replay-branch copy is
			// always treated as non-flash/non-announce-worthy by the
			// frontend, even though the live-branch copy of the SAME event
			// (correctly) is not — preventing a double-flash/double-
			// aria-live-announce on reconnect.
			forceIsSnapshot(converted)
			if err := sender.Send(converted); err != nil {
				return fmt.Errorf("failed to send replayed backlog event: %w", err)
			}
			initialPhaseSent++
		}
	} else if s.storage != nil {
		// Fresh connection: send one event per currently-visible item as its
		// current state. There is no prior state to diff against, so every
		// item is delivered as an item_updated variant (empty updated_fields).
		items, err := s.storage.ListBacklogItems(ctx, session.BacklogItemFilter{})
		if err != nil {
			return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load backlog items: %w", err))
		}
		for i := range items {
			item := &items[i]
			if !backlogItemMatchesFilters(item, msg) {
				continue
			}
			if err := sender.Send(snapshotEventForItem(item, costFor)); err != nil {
				return fmt.Errorf("failed to send initial backlog snapshot: %w", err)
			}
			initialPhaseSent++
		}
	}

	// A zero-item backlog (or a status_filter/category_filter matching
	// nothing) means the branch above sent literally zero bytes on the wire.
	// ConnectRPC's client-side `for await` loop (useWatchBacklogItems.ts)
	// never resolves past its first iteration until *something* arrives, so
	// without this the client is stuck at connectionState "connecting"
	// forever even though the stream is healthy and correctly has nothing to
	// report. Send an explicit, content-free marker so the client always
	// receives at least one message and can promptly settle to "live".
	if initialPhaseSent == 0 {
		if err := sender.Send(&sessionv1.BacklogItemEvent{
			Timestamp: timestamppb.Now(),
			Event:     &sessionv1.BacklogItemEvent_SnapshotComplete{SnapshotComplete: &sessionv1.BacklogSnapshotCompleteEvent{}},
		}); err != nil {
			return fmt.Errorf("failed to send snapshot-complete marker: %w", err)
		}
	}

	// Stream events until client disconnects or context is canceled.
	for {
		select {
		case <-ctx.Done():
			return nil
		case evt, ok := <-eventCh:
			if !ok {
				return nil
			}
			if evt.Type != events.EventBacklogItemChanged || evt.BacklogItemPayload == nil {
				continue
			}
			if !backlogItemMatchesFilters(evt.BacklogItemPayload.Item, msg) {
				continue
			}
			if err := sender.Send(convertEventToBacklogItemEvent(evt, costFor)); err != nil {
				return fmt.Errorf("failed to send backlog event: %w", err)
			}
		}
	}
}

// backlogItemMatchesFilters reports whether item passes msg's status_filter
// and category_filter. Empty filters match everything. category_filter has
// no dedicated field on BacklogItemData/BacklogItemSummary today, so it is
// matched against RepoPath — the closest existing grouping concept — as a
// deliberate deviation from the plan's "category" wording; see the
// implementation report for this task.
func backlogItemMatchesFilters(item *session.BacklogItemData, msg *sessionv1.WatchBacklogItemsRequest) bool {
	if item == nil {
		return false
	}
	if statusFilter := msg.GetStatusFilter(); len(statusFilter) > 0 && !slices.Contains(statusFilter, item.Status) {
		return false
	}
	if categoryFilter := msg.GetCategoryFilter(); len(categoryFilter) > 0 && !slices.Contains(categoryFilter, item.RepoPath) {
		return false
	}
	return true
}

// snapshotEventForItem builds the initial-snapshot BacklogItemEvent for a
// single currently-visible item (fresh-connection branch, after_seq == 0).
func snapshotEventForItem(item *session.BacklogItemData, costFor func(tmuxUUID string) float64) *sessionv1.BacklogItemEvent {
	return &sessionv1.BacklogItemEvent{
		Timestamp: timestamppb.Now(),
		// Seq intentionally left at its zero value: this synthetic per-item
		// snapshot isn't derived from a single published bus event, so it has
		// no real sequence number. The frontend treats seq == 0 as "no seq
		// info" and excludes these events from afterSeq/gap-detection
		// bookkeeping — see BacklogItemEvent.seq's proto doc comment.
		Event: &sessionv1.BacklogItemEvent_ItemUpdated{
			ItemUpdated: &sessionv1.BacklogItemUpdatedEvent{
				ItemId:     item.ID,
				Item:       backlogItemToProto(item, costFor),
				IsSnapshot: true,
			},
		},
	}
}

// convertEventToBacklogItemEvent converts an internal events.Event carrying a
// BacklogItemEventPayload into the wire sessionv1.BacklogItemEvent, switching
// on Kind to build the matching oneof variant. Mirrors convertEventToProto's
// switch-on-event.Type pattern already used for session events
// (event_converter.go).
func convertEventToBacklogItemEvent(evt *events.Event, costFor func(tmuxUUID string) float64) *sessionv1.BacklogItemEvent {
	out := &sessionv1.BacklogItemEvent{
		Timestamp: timestamppb.New(evt.Timestamp),
		// evt.Seq is assigned by EventBus.Publish (0 means unpublished, which
		// should never happen here since this is only called for events read
		// from the subscription channel or EventsSince — both post-Publish).
		// Threading it through lets the frontend's afterSeq/gap-detection
		// bookkeeping (useWatchBacklogItems.ts) work for both the live
		// fan-out path and the after_seq replay path.
		Seq: evt.Seq,
	}

	payload := evt.BacklogItemPayload
	if payload == nil {
		return out
	}

	var itemID string
	if payload.Item != nil {
		itemID = payload.Item.ID
	}
	protoItem := backlogItemToProtoOrNil(payload.Item, costFor)

	switch payload.Kind {
	case events.BacklogChangeStatusTransition:
		out.Event = &sessionv1.BacklogItemEvent_StatusChanged{
			StatusChanged: &sessionv1.BacklogItemStatusChangedEvent{
				ItemId:     itemID,
				OldStatus:  payload.OldStatus,
				NewStatus:  payload.NewStatus,
				Item:       protoItem,
				IsSnapshot: payload.IsSnapshot,
			},
		}

	case events.BacklogChangeVerdictRecorded:
		out.Event = &sessionv1.BacklogItemEvent_VerdictRecorded{
			VerdictRecorded: &sessionv1.BacklogItemVerdictRecordedEvent{
				ItemId:     itemID,
				Verdict:    reviewVerdictDataToProto(payload.Verdict, evt.Timestamp),
				Item:       protoItem,
				IsSnapshot: payload.IsSnapshot,
			},
		}

	case events.BacklogChangeSessionAttached:
		out.Event = &sessionv1.BacklogItemEvent_SessionAttached{
			SessionAttached: &sessionv1.BacklogItemSessionAttachedEvent{
				ItemId:         itemID,
				SessionId:      payload.SessionID,
				Item:           protoItem,
				IsSnapshot:     payload.IsSnapshot,
				ClaimantHostId: payload.ClaimantHostID,
			},
		}

	case events.BacklogChangeItemUpdated, events.BacklogChangeTriageProgressUpdated:
		// A triage-progress write (UpdateItemSessionTriageResult) reuses this
		// same wire event rather than a new proto message — a field update on
		// the item's derived state is exactly what BacklogItemUpdatedEvent's
		// updated_fields shape already models (plan.md Epic 2.2.5).
		out.Event = &sessionv1.BacklogItemEvent_ItemUpdated{
			ItemUpdated: &sessionv1.BacklogItemUpdatedEvent{
				ItemId:        itemID,
				UpdatedFields: payload.UpdatedFields,
				Item:          protoItem,
				IsSnapshot:    payload.IsSnapshot,
			},
		}

	case events.BacklogChangeActivityNoteAdded:
		// Deliberately never touches protoItem (ADR-002): this event's payload
		// carries only the new note, never a full item snapshot.
		out.Event = &sessionv1.BacklogItemEvent_ActivityNoteAdded{
			ActivityNoteAdded: &sessionv1.BacklogItemActivityNoteAddedEvent{
				ItemId: itemID,
				Note:   activityNoteDataToProto(payload.ActivityNote),
			},
		}

	case events.BacklogChangeItemArchived:
		archived := &sessionv1.BacklogItemArchivedEvent{
			ItemId:     itemID,
			IsSnapshot: payload.IsSnapshot,
		}
		if payload.ArchivedAt != nil {
			archived.ArchivedAt = timestamppb.New(*payload.ArchivedAt)
		}
		out.Event = &sessionv1.BacklogItemEvent_ItemArchived{ItemArchived: archived}

	case events.BacklogChangeItemRemoved:
		// BacklogItemRemovedEvent has no is_snapshot field by design — a
		// removed item can never be part of a snapshot (nothing left to show).
		out.Event = &sessionv1.BacklogItemEvent_ItemRemoved{
			ItemRemoved: &sessionv1.BacklogItemRemovedEvent{
				ItemId: itemID,
				Reason: payload.RemovedReason,
			},
		}
	}

	return out
}

// backlogItemToProtoOrNil is backlogItemToProto's nil-safe counterpart:
// backlogItemToProto dereferences item unconditionally, but
// BacklogItemEventPayload.Item may legitimately be nil (defensive only —
// production publishers always populate it) so every call site here must
// guard first.
func backlogItemToProtoOrNil(item *session.BacklogItemData, costFor func(tmuxUUID string) float64) *sessionv1.BacklogItem {
	if item == nil {
		return nil
	}
	return backlogItemToProto(item, costFor)
}

// activityNoteDataToProto converts a session.ActivityNoteData (the payload
// carried by BacklogChangeActivityNoteAdded) to the wire
// sessionv1.BacklogActivityNote. Nil-safe: BacklogItemChange.ActivityNote is
// only guaranteed non-nil when Kind == ChangeActivityNoteAdded, but this is
// called unconditionally from that switch case, so a defensive nil check
// still guards against a caller bug rather than trusting the invariant.
func activityNoteDataToProto(n *session.ActivityNoteData) *sessionv1.BacklogActivityNote {
	if n == nil {
		return nil
	}
	return &sessionv1.BacklogActivityNote{
		Id:                 n.ID,
		Message:            n.Message,
		AuthorSessionUuid:  n.AuthorSessionUUID,
		AuthorSessionTitle: n.AuthorSessionTitle,
		CreatedAt:          timestamppb.New(n.CreatedAt),
	}
}

// reviewVerdictDataToProto converts a session.ReviewVerdictData (the payload
// carried by BacklogChangeVerdictRecorded) to the wire sessionv1.ReviewVerdict.
// ReviewVerdictData has no Id/CreatedAt of its own (it is a save-input DTO,
// session/storage_backlog.go), so CreatedAt is set from the event's own
// timestamp and Id is left empty — this mirrors itemSessionToProto's
// ReviewVerdict-building logic (backlog_service.go) minus those two fields.
func reviewVerdictDataToProto(v *session.ReviewVerdictData, occurredAt time.Time) *sessionv1.ReviewVerdict {
	if v == nil {
		return nil
	}
	p := &sessionv1.ReviewVerdict{
		OverallOutcome: string(v.OverallOutcome),
		Summary:        v.Summary,
		DiffTokenCount: int32(v.DiffTokenCount),
		DiffTruncated:  v.DiffTruncated,
		OverrideBy:     v.OverrideBy,
		OverrideReason: v.OverrideReason,
		CreatedAt:      timestamppb.New(occurredAt),
	}
	if v.OverrideAt != nil {
		p.OverrideAt = timestamppb.New(*v.OverrideAt)
	}
	if v.PerCriterion != "" {
		var cvs []session.CriterionVerdict
		if err := json.Unmarshal([]byte(v.PerCriterion), &cvs); err == nil {
			p.PerCriterion = make([]*sessionv1.CriterionVerdict, len(cvs))
			for i, cv := range cvs {
				p.PerCriterion[i] = &sessionv1.CriterionVerdict{
					CriterionIndex: int32(cv.CriterionIndex),
					Outcome:        string(cv.Outcome),
					Evidence:       cv.Evidence,
				}
			}
		}
	}
	return p
}

// forceIsSnapshot sets is_snapshot: true on whichever oneof variant evt has
// populated. Used only by the after_seq replay branch — see its call site's
// comment for why. BacklogItemEvent_ItemRemoved has no is_snapshot field and
// is intentionally not a case here (nothing to force).
func forceIsSnapshot(evt *sessionv1.BacklogItemEvent) {
	switch e := evt.GetEvent().(type) {
	case *sessionv1.BacklogItemEvent_StatusChanged:
		e.StatusChanged.IsSnapshot = true
	case *sessionv1.BacklogItemEvent_VerdictRecorded:
		e.VerdictRecorded.IsSnapshot = true
	case *sessionv1.BacklogItemEvent_SessionAttached:
		e.SessionAttached.IsSnapshot = true
	case *sessionv1.BacklogItemEvent_ItemUpdated:
		e.ItemUpdated.IsSnapshot = true
	case *sessionv1.BacklogItemEvent_ItemArchived:
		e.ItemArchived.IsSnapshot = true
	}
}
