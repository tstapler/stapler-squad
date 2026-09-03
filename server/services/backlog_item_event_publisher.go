package services

import (
	"fmt"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

// BacklogItemEventPublisher adapts an *events.EventBus to
// session.ItemChangePublisher. The session package cannot import pkg/events
// directly (pkg/events imports session, so the reverse import would be a
// cycle), so this adapter lives here instead and is wired in via
// Storage.SetItemChangePublisher (session/storage.go), mirroring
// EventBusNotifier's adapter pattern for session.Notifier.
type BacklogItemEventPublisher struct {
	Bus *events.EventBus
}

// PublishItemChanged implements session.ItemChangePublisher. The entire body
// is wrapped in its own recover() so a panic anywhere inside it (payload
// construction, an unmapped BacklogChangeKind, bus.Publish itself) can never
// propagate into the repository method that called it — the same
// "best-effort side channel must not take down the caller" idiom this
// codebase already uses at runStuckDetector (session/backlog_lifecycle.go)
// and around PTY forwarding (server/services/session_service.go). Because the
// recover happens inside the adapter itself, session.ItemChangePublisher.
// PublishItemChanged deliberately keeps its no-error-return signature: there
// is never an error for a repository caller to log or swallow, because a
// panic can never reach it in the first place.
func (p *BacklogItemEventPublisher) PublishItemChanged(item *session.BacklogItemData, change session.BacklogItemChange) {
	defer func() {
		if r := recover(); r != nil {
			log.WarningLog().Printf("[BacklogItemEventPublisher] PublishItemChanged panicked (recovered): %v", r)
		}
	}()

	if p == nil || p.Bus == nil {
		return
	}

	payload := &events.BacklogItemEventPayload{
		Kind:           mapBacklogChangeKind(change.Kind),
		Item:           item,
		OldStatus:      change.OldStatus,
		NewStatus:      change.NewStatus,
		UpdatedFields:  change.UpdatedFields,
		SessionID:      change.SessionID,
		ArchivedAt:     change.ArchivedAt,
		RemovedReason:  change.RemovedReason,
		Verdict:        change.Verdict,
		ClaimantHostID: change.ClaimantHostID,
		ActivityNote:   change.ActivityNote,
	}
	p.Bus.Publish(events.NewBacklogItemChangedEvent(payload))
}

// mapBacklogChangeKind converts session.BacklogChangeKind (the session
// package's cycle-avoiding mirror, session/backlog_item_change.go) to
// events.BacklogChangeKind (the pkg/events wire type). Kept as an explicit
// switch rather than a raw string conversion so an unmapped kind panics
// (caught by PublishItemChanged's recover(), logged, and never delivered)
// instead of silently publishing an unrecognized value to subscribers.
func mapBacklogChangeKind(kind session.BacklogChangeKind) events.BacklogChangeKind {
	switch kind {
	case session.ChangeStatusTransition:
		return events.BacklogChangeStatusTransition
	case session.ChangeVerdictRecorded:
		return events.BacklogChangeVerdictRecorded
	case session.ChangeSessionAttached:
		return events.BacklogChangeSessionAttached
	case session.ChangeItemUpdated:
		return events.BacklogChangeItemUpdated
	case session.ChangeItemArchived:
		return events.BacklogChangeItemArchived
	case session.ChangeItemRemoved:
		return events.BacklogChangeItemRemoved
	case session.ChangeTriageProgressUpdated:
		return events.BacklogChangeTriageProgressUpdated
	case session.ChangeActivityNoteAdded:
		return events.BacklogChangeActivityNoteAdded
	default:
		panic(fmt.Sprintf("BacklogItemEventPublisher: unmapped BacklogChangeKind %q", kind))
	}
}
