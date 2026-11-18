package backlogstore

import (
	"context"
	"fmt"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/acauditevent"
	"github.com/tstapler/stapler-squad/session/ent/backlogitem"
	"github.com/tstapler/stapler-squad/session/ent/predicate"
)

// Audit field bounds and per-item retention.
const (
	maxAcAuditRemoteAddr = 100
	maxAcAuditReason     = 2000
	// MaxAcAuditRefusalsPerItem bounds denied/rejected rows per item. Rows recording a real change
	// (applied, waiver_cleared) are never evicted: they are bounded by real mutations, and a flood
	// of refused attempts must not be able to push a waiver's provenance out.
	MaxAcAuditRefusalsPerItem = 100
)

func capString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "")
}

// CreateAcAuditEvent appends one row and, for a refused attempt, trims the item's refusals to
// MaxAcAuditRefusalsPerItem (oldest first). It runs on the caller's transaction (or client) so the row commits with the change.
func (*Backlog) CreateAcAuditEvent(ctx context.Context, client *ent.AcAuditEventClient, d backlogdto.AcAuditEventData) error {
	itemID, err := uuid.Parse(d.ItemID)
	if err != nil {
		return fmt.Errorf("ac audit: invalid item id %q: %w", d.ItemID, err)
	}
	a := domain.AcActorFrom(ctx)
	if _, err := client.Create().SetItemID(itemID).
		SetItemPublicID(d.ItemPublicID).
		SetCriterionIndex(d.CriterionIndex).
		SetPreviousStatus(d.PreviousStatus).
		SetNewStatus(d.NewStatus).
		SetReason(capString(d.Reason, maxAcAuditReason)).
		SetOutcome(capString(d.Outcome, 300)).
		SetActorClass(string(a.Class)).
		SetActorSessionUUID(capString(a.SessionUUID, 64)).
		SetSessionVerified(a.SessionVerified).
		SetSource(a.Source).
		SetRemoteAddr(capString(a.RemoteAddr, maxAcAuditRemoteAddr)).
		SetUserAgent(capString(a.UserAgent, 200)).
		Save(ctx); err != nil {
		return fmt.Errorf("record ac audit event: %w", err)
	}
	if isAcAuditChangeOutcome(d.Outcome) {
		return nil
	}
	return pruneAcAuditRefusals(ctx, client, itemID)
}

// isAcAuditChangeOutcome reports whether an outcome records a real change (kept forever, per item).
func isAcAuditChangeOutcome(outcome string) bool {
	return outcome == "applied" || strings.HasPrefix(outcome, "waiver_cleared")
}

func acAuditRefusalPredicate() predicate.AcAuditEvent {
	return acauditevent.And(acauditevent.OutcomeNEQ("applied"), acauditevent.Not(acauditevent.OutcomeHasPrefix("waiver_cleared")))
}

func pruneAcAuditRefusals(ctx context.Context, client *ent.AcAuditEventClient, itemID uuid.UUID) error {
	where := acauditevent.And(acauditevent.ItemID(itemID), acAuditRefusalPredicate())
	n, err := client.Query().Where(where).Count(ctx)
	if err != nil || n <= MaxAcAuditRefusalsPerItem {
		return err
	}
	oldest, err := client.Query().Where(where).
		Order(ent.Asc(acauditevent.FieldCreatedAt)).Limit(n - MaxAcAuditRefusalsPerItem).IDs(ctx)
	if err != nil {
		return err
	}
	_, err = client.Delete().Where(acauditevent.IDIn(oldest...)).Exec(ctx)
	return err
}

// PruneAcAuditOlderThan applies age retention and returns how many rows were removed: refused
// attempts older than cutoff go for every item, but rows recording a real change are removed only
// once their item no longer exists, so a live item's waiver keeps its provenance.
func (b *Backlog) PruneAcAuditOlderThan(ctx context.Context, cutoff time.Time) (int, error) {
	refused, err := b.client.AcAuditEvent.Delete().
		Where(acauditevent.CreatedAtLT(cutoff), acAuditRefusalPredicate()).Exec(ctx)
	if err != nil {
		return refused, err
	}
	orphaned, err := b.client.AcAuditEvent.Delete().Where(
		acauditevent.CreatedAtLT(cutoff),
		func(s *sql.Selector) {
			s.Where(sql.NotIn(s.C(acauditevent.FieldItemID), sql.Select(backlogitem.FieldID).From(sql.Table(backlogitem.Table))))
		},
	).Exec(ctx)
	return refused + orphaned, err
}

// RecordAcAudit appends an audit row for an attempt refused before it reached the criterion
// (for example a denied caller), using the actor attached to ctx.
func (b *Backlog) RecordAcAudit(ctx context.Context, d backlogdto.AcAuditEventData) error {
	if d.ItemPublicID == "" {
		if id, err := uuid.Parse(d.ItemID); err == nil {
			if it, err := b.client.BacklogItem.Get(ctx, id); err == nil {
				d.ItemPublicID = it.PublicID
			}
		}
	}
	return b.CreateAcAuditEvent(ctx, b.client.AcAuditEvent, d)
}
