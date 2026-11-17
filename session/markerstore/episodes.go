package markerstore

import (
	"context"
	"database/sql"
	"errors"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogstuckepisode"
)

// IsEpisodeAlreadyOpen reports whether an episode insert error just means an
// open episode already exists. An ignored ON CONFLICT DO NOTHING insert
// surfaces as sql.ErrNoRows on SQLite (no RETURNING row), or as a constraint
// error from the partial unique index.
func IsEpisodeAlreadyOpen(err error) bool {
	return errors.Is(err, sql.ErrNoRows) || ent.IsConstraintError(err)
}

// OpenStuckEpisodeTx inserts an open episode for (itemID, reason) inside tx.
// It takes the tx, never r.client: the writer pool has one connection, so a
// client call while the tx is open would deadlock. opened is false with a nil
// error when an open episode already exists; any other error is returned.
func OpenStuckEpisodeTx(ctx context.Context, tx *ent.Tx, itemID uuid.UUID, reason string, openedAt time.Time) (opened bool, err error) {
	err = tx.BacklogStuckEpisode.Create().
		SetItemID(itemID).
		SetReason(reason).
		SetOpenedAt(openedAt.UTC()).
		OnConflict(entsql.DoNothing()).
		Exec(ctx)
	if err == nil {
		return true, nil
	}
	if IsEpisodeAlreadyOpen(err) {
		return false, nil
	}
	return false, err
}

// CloseStuckEpisodeTx closes the open episode for (itemID, reason) inside tx,
// leaving older closed episodes untouched.
func CloseStuckEpisodeTx(ctx context.Context, tx *ent.Tx, itemID uuid.UUID, reason string, resolvedAt time.Time) error {
	_, err := tx.BacklogStuckEpisode.Update().
		Where(
			backlogstuckepisode.ItemID(itemID),
			backlogstuckepisode.Reason(reason),
			backlogstuckepisode.ResolvedAtIsNil(),
		).
		SetResolvedAt(resolvedAt.UTC()).
		Save(ctx)
	return err
}

// OpenStuckEpisodeNonFatal opens an episode and logs, rather than returns, an
// unexpected failure so a history gap never blocks the stuck-state write.
// Continuing in the tx after a failed statement is safe on SQLite only; a
// driver that aborts the tx on error would need a savepoint here.
func OpenStuckEpisodeNonFatal(ctx context.Context, tx *ent.Tx, itemID uuid.UUID, reason string, openedAt time.Time) bool {
	opened, err := OpenStuckEpisodeTx(ctx, tx, itemID, reason, openedAt)
	if err != nil {
		log.WarningLog().Printf("[stuckEpisode] failed to open stuck episode item=%s reason=%s: %v", itemID, reason, err)
	}
	return opened
}

// CloseStuckEpisodeNonFatal is the close-side counterpart of
// OpenStuckEpisodeNonFatal.
func CloseStuckEpisodeNonFatal(ctx context.Context, tx *ent.Tx, itemID uuid.UUID, reason string, resolvedAt time.Time) {
	if err := CloseStuckEpisodeTx(ctx, tx, itemID, reason, resolvedAt); err != nil {
		log.WarningLog().Printf("[stuckEpisode] failed to close stuck episode item=%s reason=%s: %v", itemID, reason, err)
	}
}
