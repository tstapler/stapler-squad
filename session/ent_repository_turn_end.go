package session

import (
	"context"
	"fmt"
	"time"

	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/predicate"
	"github.com/tstapler/stapler-squad/session/ent/session"
)

// The turn_end_* and acked_episode_id columns have narrow writers (UpdateTurnEnded,
// UpdateAckedEpisode) that run concurrently with the full save. A full save carries an
// in-memory snapshot that may predate a narrow write, so Update merges these columns with
// guarded statements instead of overwriting them.

func setTurnEndCreate(c *ent.SessionCreate, t TurnEnded) {
	if t.IsZero() {
		return
	}
	at := t.At.UTC()
	c.SetTurnEndEpisode(string(t.Episode)).
		SetTurnEndKind(t.Kind.String()).
		SetTurnEndConfidence(t.Confidence.String()).
		SetTurnEndExcerpt(t.Excerpt).
		SetTurnEndSource(string(t.Source)).
		SetTurnEndAt(at)
}

func turnEndFromRow(sess *ent.Session) TurnEnded {
	if sess.TurnEndEpisode == nil || *sess.TurnEndEpisode == "" {
		return TurnEnded{}
	}
	t := TurnEnded{Episode: EpisodeID(*sess.TurnEndEpisode)}
	if sess.TurnEndKind != nil {
		t.Kind = ParseTurnEndKind(*sess.TurnEndKind)
	}
	if sess.TurnEndConfidence != nil {
		t.Confidence = ParseTurnEndConfidence(*sess.TurnEndConfidence)
	}
	if sess.TurnEndExcerpt != nil {
		t.Excerpt = *sess.TurnEndExcerpt
	}
	if sess.TurnEndSource != nil {
		t.Source = TurnEndSource(*sess.TurnEndSource)
	}
	if sess.TurnEndAt != nil {
		t.At = *sess.TurnEndAt
	}
	return t
}

// mergeNarrowColumns applies the full save's turn-end and acked-episode values to the row
// only where they are not older than what a narrow writer already stored. Each statement
// carries its own freshness predicate, so correctness does not depend on how stale the
// caller's earlier SELECT is. It runs before the main update so the acked-clear rule sees
// the stored (not the just-rewritten) last_acknowledged.
func mergeNarrowColumns(ctx context.Context, tx *ent.Tx, sess *ent.Session, data InstanceData) error {
	if err := mergeAckedEpisode(ctx, tx, sess.ID, data); err != nil {
		return err
	}
	if data.TurnEnded.IsZero() {
		return nil
	}
	at := data.TurnEnded.At.UTC()
	upd := tx.Session.Update().
		Where(session.ID(sess.ID), session.Or(session.TurnEndAtIsNil(), session.TurnEndAtLTE(at)))
	setTurnEndColumns(upd, data.TurnEnded)
	if _, err := upd.Save(ctx); err != nil {
		return fmt.Errorf("failed to merge turn_end columns: %w", err)
	}
	return nil
}

// mergeAckedEpisode writes a non-empty acked episode only if the in-memory LastAcknowledged is
// not older than the stored one. An empty one clears the column only on that same freshness
// test or when paired with a strictly newer turn end (an unconditional "" would also be what a
// snapshot taken before a narrow ack looks like, so equal turn ends must not clear).
func mergeAckedEpisode(ctx context.Context, tx *ent.Tx, id int, data InstanceData) error {
	fresh := session.Or(session.LastAcknowledgedIsNil(), session.LastAcknowledgedLTE(data.LastAcknowledged.UTC()))
	upd := tx.Session.Update()
	if data.AckedEpisodeID != "" {
		upd.Where(session.ID(id), fresh).SetAckedEpisodeID(string(data.AckedEpisodeID))
	} else {
		clearOn := []predicate.Session{fresh}
		if !data.TurnEnded.IsZero() {
			clearOn = append(clearOn, session.TurnEndAtIsNil(), session.TurnEndAtLT(data.TurnEnded.At.UTC()))
		}
		upd.Where(session.ID(id), session.AckedEpisodeIDNotNil(), session.Or(clearOn...)).ClearAckedEpisodeID()
	}
	if _, err := upd.Save(ctx); err != nil {
		return fmt.Errorf("failed to merge acked_episode_id: %w", err)
	}
	return nil
}

func setTurnEndColumns(upd *ent.SessionUpdate, t TurnEnded) {
	if t.IsZero() {
		upd.ClearTurnEndEpisode().ClearTurnEndKind().ClearTurnEndConfidence().
			ClearTurnEndExcerpt().ClearTurnEndSource().ClearTurnEndAt()
		return
	}
	upd.SetTurnEndEpisode(string(t.Episode)).
		SetTurnEndKind(t.Kind.String()).
		SetTurnEndConfidence(t.Confidence.String()).
		SetTurnEndExcerpt(t.Excerpt).
		SetTurnEndSource(string(t.Source)).
		SetTurnEndAt(t.At.UTC())
}

// UpdateAckedEpisode stores the acknowledged episode and last_acknowledged in one UPDATE
// (no SELECT). Monotonic: an older ackAt never overwrites a newer stored acknowledgement.
// An empty episode clears acked_episode_id.
func (r *EntRepository) UpdateAckedEpisode(ctx context.Context, title string, episode EpisodeID, ackAt time.Time) error {
	ackAt = ackAt.UTC()
	upd := r.client.Session.Update().
		Where(session.Title(title), session.Or(session.LastAcknowledgedIsNil(), session.LastAcknowledgedLTE(ackAt))).
		SetLastAcknowledged(ackAt).
		SetUpdatedAt(time.Now())
	if episode == "" {
		upd.ClearAckedEpisodeID()
	} else {
		upd.SetAckedEpisodeID(string(episode))
	}
	n, err := upd.Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to update acked_episode_id: %w", err)
	}
	if n == 0 {
		return r.missingOrStale(ctx, title)
	}
	return nil
}

// UpdateTurnEnded stores the turn-end columns in one UPDATE (no SELECT). Monotonic: it only
// applies when the stored turn_end_at is null or not after turn.At. A zero turn clears them.
func (r *EntRepository) UpdateTurnEnded(ctx context.Context, title string, turn TurnEnded) error {
	upd := r.client.Session.Update().
		Where(session.Title(title)).
		SetUpdatedAt(time.Now())
	if !turn.IsZero() {
		upd.Where(session.Or(session.TurnEndAtIsNil(), session.TurnEndAtLTE(turn.At.UTC())))
	}
	// A zero turn has no timestamp to order by, so it clears unconditionally.
	setTurnEndColumns(upd, turn)
	n, err := upd.Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to update turn_end columns: %w", err)
	}
	if n == 0 {
		return r.missingOrStale(ctx, title)
	}
	return nil
}

// missingOrStale distinguishes "no such session" (an error) from "a newer value is already
// stored" (a successful no-op) after a guarded UPDATE matched no row.
func (r *EntRepository) missingOrStale(ctx context.Context, title string) error {
	exists, err := r.client.Session.Query().Where(session.Title(title)).Exist(ctx)
	if err != nil {
		return fmt.Errorf("failed to check session %s: %w", title, err)
	}
	if !exists {
		return fmt.Errorf("session not found: %s", title)
	}
	return nil
}
