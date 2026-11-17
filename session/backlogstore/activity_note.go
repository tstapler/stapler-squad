package backlogstore

import (
	"context"
	"fmt"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogitem"
)

// mapAppendActivityNoteCreateError classifies an error returned from
// AppendActivityNote's BacklogActivityNote.Create().Save(ctx) call. A
// foreign-key-violation constraint error means itemID was deleted between
// AppendActivityNote's earlier existence check and this Create() call (a
// real but non-deterministic-to-test race) and is wrapped as ErrNotFound so
// callers (post_backlog_update, via Storage.AppendActivityNote) can map it to
// ErrItemNotFound the same way the earlier, more common not-found path is
// mapped. Any other error is wrapped as a plain internal error. Extracted as
// its own function (rather than inlined in AppendActivityNote) specifically
// so this classification can be unit-tested without reproducing the
// underlying delete-between-steps race.
func mapAppendActivityNoteCreateError(err error, itemID string) error {
	if ent.IsConstraintError(err) {
		return fmt.Errorf("backlog item %s not found: %w", itemID, domain.ErrNotFound)
	}
	return fmt.Errorf("failed to append activity note for item %s: %w", itemID, err)
}

// AppendActivityNote records a single post_backlog_update call as an
// immutable, append-only history entry. This is the one activity-note write
// path invoked by the ungated post_backlog_update tool — any session, with
// or without STAPLER_SESSION_UUID, whether or not linked to the item, can
// call it. Unlike AppendProgressNote, this DOES publish an item-changed
// event: there is no separate criterion-status write alongside it to carry
// that responsibility, so if AppendActivityNote didn't publish, a live
// WatchBacklogItems subscriber would never learn a note was posted.
//
// Before publishing, this does a cheap, read-only, field-scoped query for
// the item's Status and RepoPath and populates them on the published
// snapshot. This exists solely so backlogItemMatchesFilters
// (server/services/backlog_service_events.go) doesn't silently drop this
// event for WatchBacklogItems callers with a non-empty status/category
// filter — see Blocker 2 in implementation/adversarial-review.md. It is a
// read, not a read-modify-write, so it introduces no new concurrency hazard.
// hooks.PublishItemChanged (not the PublishItemChanged convenience
// wrapper) is used deliberately: this event's payload never carries a full
// item snapshot (ADR-002), so there is nothing for AttachItemSessionsForPublish
// to usefully eager-load beyond the two scalar fields already fetched below.
func (b *Backlog) AppendActivityNote(ctx context.Context, itemID, authorSessionUUID, authorSessionTitle, message string) error {
	parsedItemID, err := b.ResolveBacklogItemLookup(ctx, itemID)
	if err != nil {
		return fmt.Errorf("invalid item id %q: %w", itemID, err)
	}

	itemFields, err := b.client.BacklogItem.Query().
		Where(backlogitem.ID(parsedItemID)).
		Select(backlogitem.FieldStatus, backlogitem.FieldRepoPath).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("backlog item %s not found: %w", itemID, domain.ErrNotFound)
		}
		return fmt.Errorf("failed to look up backlog item %s: %w", itemID, err)
	}

	created, err := b.client.BacklogActivityNote.Create().
		SetItemID(parsedItemID).
		SetMessage(message).
		SetAuthorSessionUUID(authorSessionUUID).
		SetAuthorSessionTitle(authorSessionTitle).
		Save(ctx)
	if err != nil {
		return mapAppendActivityNoteCreateError(err, itemID)
	}

	note := ActivityNoteToData(created)
	b.hooks.PublishItemChanged(&backlogdto.BacklogItemData{
		ID:       parsedItemID.String(),
		Status:   itemFields.Status,
		RepoPath: itemFields.RepoPath,
	}, backlogdto.BacklogItemChange{Kind: backlogdto.ChangeActivityNoteAdded, ActivityNote: &note})
	return nil
}
