package session

import (
	"context"
	"fmt"
	"time"

	entsql "entgo.io/ent/dialect/sql"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogitem"
	"github.com/tstapler/stapler-squad/session/ent/predicate"
	"github.com/tstapler/stapler-squad/session/ent/pullrequeststate"
	entsession "github.com/tstapler/stapler-squad/session/ent/session"
	"github.com/tstapler/stapler-squad/session/ent/sessionpr"
	"github.com/tstapler/stapler-squad/session/ent/sessionprtombstone"
)

// orphanGCGrace is how old a row must be before the GC may collect it, so an
// attach racing the GC (EnsureState, then the link insert) never loses its state.
const orphanGCGrace = 24 * time.Hour

// deleteOwnerPRRows removes one owner's links and tombstones; used when the
// owner (a session or a backlog item) is deleted. State rows are left to the GC.
func deleteOwnerPRRows(ctx context.Context, tx *ent.Tx, owner PROwner) error {
	if _, err := tx.SessionPR.Delete().Where(linkOwnerPredicates(owner)...).Exec(ctx); err != nil {
		return fmt.Errorf("delete pr links: %w", err)
	}
	if _, err := tx.SessionPRTombstone.Delete().Where(
		sessionprtombstone.SessionUUID(owner.SessionUUID), sessionprtombstone.BacklogItemID(owner.BacklogItemID)).Exec(ctx); err != nil {
		return fmt.Errorf("delete pr tombstones: %w", err)
	}
	return nil
}

// ownerMissing matches rows whose ownerColumn value has no row in table.idColumn.
func ownerMissing(ownerColumn, table, idColumn string) func(*entsql.Selector) {
	return func(s *entsql.Selector) {
		t := entsql.Table(table)
		owners := entsql.Select(t.C(idColumn)).From(t).Where(entsql.ColumnsEQ(t.C(idColumn), s.C(ownerColumn)))
		s.Where(entsql.NotExists(owners))
	}
}

// SessionPRGCResult counts what one GC pass deleted.
type SessionPRGCResult struct {
	Links, Tombstones, States int
}

// RunSessionPRLinkGC deletes, and only deletes: links and tombstones whose
// single owner (session or backlog item) no longer exists and that are older
// than orphanGCGrace, then state rows with no remaining link and the same age.
// It writes no state column, so it is not a second state writer (invariant I1).
func (r *EntRepository) RunSessionPRLinkGC(ctx context.Context, now time.Time) (SessionPRGCResult, error) {
	cutoff := now.Add(-orphanGCGrace)
	var res SessionPRGCResult
	step := func(label string, fn func() (int, error)) error {
		n, err := fn()
		if err != nil {
			return fmt.Errorf("session pr gc: %s: %w", label, err)
		}
		res.countFor(label, n)
		return nil
	}
	sessionGone := predicate.SessionPR(ownerMissing(sessionpr.FieldSessionUUID, entsession.Table, entsession.FieldUUID))
	itemGone := predicate.SessionPR(ownerMissing(sessionpr.FieldBacklogItemID, backlogitem.Table, backlogitem.FieldID))
	tombSessionGone := predicate.SessionPRTombstone(ownerMissing(sessionprtombstone.FieldSessionUUID, entsession.Table, entsession.FieldUUID))
	tombItemGone := predicate.SessionPRTombstone(ownerMissing(sessionprtombstone.FieldBacklogItemID, backlogitem.Table, backlogitem.FieldID))

	steps := []struct {
		label string
		fn    func() (int, error)
	}{
		{"links", func() (int, error) {
			return r.client.SessionPR.Delete().Where(sessionpr.SessionUUIDNEQ(""), sessionpr.AttachedAtLT(cutoff), sessionGone).Exec(ctx)
		}},
		{"links", func() (int, error) {
			return r.client.SessionPR.Delete().Where(sessionpr.BacklogItemIDNEQ(""), sessionpr.AttachedAtLT(cutoff), itemGone).Exec(ctx)
		}},
		{"tombstones", func() (int, error) {
			return r.client.SessionPRTombstone.Delete().Where(sessionprtombstone.SessionUUIDNEQ(""), sessionprtombstone.CreatedAtLT(cutoff), tombSessionGone).Exec(ctx)
		}},
		{"tombstones", func() (int, error) {
			return r.client.SessionPRTombstone.Delete().Where(sessionprtombstone.BacklogItemIDNEQ(""), sessionprtombstone.CreatedAtLT(cutoff), tombItemGone).Exec(ctx)
		}},
		{"states", func() (int, error) {
			return r.client.PullRequestState.Delete().Where(pullrequeststate.Not(pullrequeststate.HasLinks()), pullrequeststate.CreatedAtLT(cutoff)).Exec(ctx)
		}},
	}
	for _, s := range steps {
		if err := step(s.label, s.fn); err != nil {
			return res, err
		}
	}
	return res, nil
}

func (g *SessionPRGCResult) countFor(label string, n int) {
	switch label {
	case "links":
		g.Links += n
	case "tombstones":
		g.Tombstones += n
	case "states":
		g.States += n
	}
}

// SessionPRLinkGC runs RunSessionPRLinkGC on an interval until ctx is cancelled.
type SessionPRLinkGC struct {
	repo     *EntRepository
	interval time.Duration
	now      func() time.Time
}

// NewSessionPRLinkGC builds the nightly collector; interval <= 0 means 24h.
func NewSessionPRLinkGC(repo *EntRepository, interval time.Duration) *SessionPRLinkGC {
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	return &SessionPRLinkGC{repo: repo, interval: interval, now: time.Now}
}

// Run blocks until ctx is cancelled, collecting once per interval. Errors are
// logged and the loop continues; the pass never revives or starts anything.
func (g *SessionPRLinkGC) Run(ctx context.Context) {
	ticker := time.NewTicker(g.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			res, err := g.repo.RunSessionPRLinkGC(ctx, g.now())
			if err != nil {
				log.Warn("session pr link gc failed", "error", err)
				continue
			}
			if res != (SessionPRGCResult{}) {
				log.Info("session pr link gc", "links", res.Links, "tombstones", res.Tombstones, "states", res.States)
			}
		}
	}
}
