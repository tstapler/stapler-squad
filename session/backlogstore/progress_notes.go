package backlogstore

import (
	"context"
	"fmt"
)

// AppendProgressNote records a single report_progress call as an immutable history
// entry, in addition to (not instead of) the current-note-per-criterion stored on
// BacklogItem.AcceptanceCriteria. Callers should treat failures here as best-effort:
// the history is an enrichment for reviewers, not part of report_progress's primary
// contract of updating the criterion's current status/note.
func (b *Backlog) AppendProgressNote(ctx context.Context, itemID string, criterionIndex int, note, status string) error {
	parsedItemID, err := b.ResolveBacklogItemLookup(ctx, itemID)
	if err != nil {
		return fmt.Errorf("invalid item id %q: %w", itemID, err)
	}

	_, err = b.client.BacklogProgressNote.Create().
		SetItemID(parsedItemID).
		SetCriterionIndex(criterionIndex).
		SetNote(note).
		SetStatus(status).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to append progress note for item %s: %w", itemID, err)
	}
	return nil
}
