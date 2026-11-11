package session

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/acauditevent"
	"github.com/tstapler/stapler-squad/session/ent/backlogitem"
	"github.com/tstapler/stapler-squad/session/ent/predicate"
)

// ErrAcCriterionAlreadyWaived is returned when a waiver targets a criterion that is already waived.
var ErrAcCriterionAlreadyWaived = errors.New("acceptance criterion is already waived")

// ErrAcWaiverViaUpdate is returned when an item update tries to introduce a waived criterion;
// waivers are recorded only through WaiveAcCriterion so they are audited.
var ErrAcWaiverViaUpdate = errors.New("a waiver can only be recorded through WaiveAcCriterion")

// ErrAcCriterionAlreadyDone is returned when a waiver targets a criterion already done.
var ErrAcCriterionAlreadyDone = errors.New("acceptance criterion is already done; nothing to waive")

// AcActorClass says how a caller identified itself. None of these prove the caller is "the owner":
// authenticated classes only show a credential was presented, an agent class is a claim unless
// SessionVerified, and Unverified means no identity signal at all (on the local loopback surface
// an agent's curl and the operator's own CLI look identical).
type AcActorClass string

const (
	AcActorAuthenticatedSession AcActorClass = "authenticated_session_cookie"
	AcActorAuthenticatedBearer  AcActorClass = "authenticated_bearer_token"
	AcActorAgentSession         AcActorClass = "agent_session"
	AcActorMCPSession           AcActorClass = "mcp_session"
	AcActorSystem               AcActorClass = "system"
	AcActorUnverified           AcActorClass = "unverified"
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

// AcActor is the best-available caller identity recorded with an AC mutation.
type AcActor struct {
	Class           AcActorClass
	SessionUUID     string
	SessionVerified bool
	Source          string // ui, api or mcp
	RemoteAddr      string
	UserAgent       string
}

type acActorKey struct{}

// WithAcActor attaches the caller identity the AC audit trail will record.
func WithAcActor(ctx context.Context, a AcActor) context.Context {
	return context.WithValue(ctx, acActorKey{}, a)
}

// AcActorFrom returns the attached actor. Request handlers always attach one; a context without
// an actor belongs to a non-request writer (lifecycle, sync, verdict application), recorded as system.
func AcActorFrom(ctx context.Context) AcActor {
	if a, ok := ctx.Value(acActorKey{}).(AcActor); ok {
		return a
	}
	return AcActor{Class: AcActorSystem, Source: "system"}
}

// Label is the human-readable "who" shown next to a waiver.
func (a AcActor) Label() string {
	switch a.Class {
	case AcActorAuthenticatedSession:
		return "session cookie credential"
	case AcActorAuthenticatedBearer:
		return "authenticated bearer token"
	case AcActorSystem:
		return "system (automated)"
	case AcActorAgentSession, AcActorMCPSession:
		if a.SessionVerified {
			return fmt.Sprintf("agent session %s (verified)", a.SessionUUID)
		}
		return fmt.Sprintf("claimed agent session %s (unverified claim)", a.SessionUUID)
	default:
		return "unverified caller"
	}
}

// AcAuditEventData is the domain DTO of one AcAuditEvent row.
type AcAuditEventData struct {
	ID               string
	ItemID           string
	ItemPublicID     string
	CriterionIndex   int
	PreviousStatus   string
	NewStatus        string
	Reason           string
	Outcome          string
	ActorClass       string
	ActorSessionUUID string
	SessionVerified  bool
	Source           string
	RemoteAddr       string
	UserAgent        string
	CreatedAt        time.Time
}

func capString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "")
}

// createAcAuditEvent appends one row and, for a refused attempt, trims the item's refusals to
// MaxAcAuditRefusalsPerItem (oldest first). It runs on the caller's transaction (or client) so the row commits with the change.
func (r *EntRepository) createAcAuditEvent(ctx context.Context, client *ent.AcAuditEventClient, d AcAuditEventData) error {
	itemID, err := uuid.Parse(d.ItemID)
	if err != nil {
		return fmt.Errorf("ac audit: invalid item id %q: %w", d.ItemID, err)
	}
	a := AcActorFrom(ctx)
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
func (r *EntRepository) PruneAcAuditOlderThan(ctx context.Context, cutoff time.Time) (int, error) {
	refused, err := r.Client.AcAuditEvent.Delete().
		Where(acauditevent.CreatedAtLT(cutoff), acAuditRefusalPredicate()).Exec(ctx)
	if err != nil {
		return refused, err
	}
	orphaned, err := r.Client.AcAuditEvent.Delete().Where(
		acauditevent.CreatedAtLT(cutoff),
		func(s *sql.Selector) {
			s.Where(sql.NotIn(s.C(acauditevent.FieldItemID), sql.Select(backlogitem.FieldID).From(sql.Table(backlogitem.Table))))
		},
	).Exec(ctx)
	return refused + orphaned, err
}

// RecordAcAudit appends an audit row for an attempt refused before it reached the criterion
// (for example a denied caller), using the actor attached to ctx.
func (r *EntRepository) RecordAcAudit(ctx context.Context, d AcAuditEventData) error {
	if d.ItemPublicID == "" {
		if id, err := uuid.Parse(d.ItemID); err == nil {
			if it, err := r.Client.BacklogItem.Get(ctx, id); err == nil {
				d.ItemPublicID = it.PublicID
			}
		}
	}
	return r.createAcAuditEvent(ctx, r.Client.AcAuditEvent, d)
}

// ListAcAudit returns an item's audit rows oldest first. It stays readable after the item is
// deleted: a UUID is queried directly and a bl_ id through the stored item_public_id. It returns
// ErrNotFound only when the item is gone and no rows remain.
func (r *EntRepository) ListAcAudit(ctx context.Context, itemID string) ([]AcAuditEventData, error) {
	var where predicate.AcAuditEvent
	if IsBacklogItemIDShape(itemID) {
		where = acauditevent.ItemPublicID(itemID)
	} else if parsed, err := uuid.Parse(itemID); err == nil {
		where = acauditevent.ItemID(parsed)
	} else {
		return nil, fmt.Errorf("%w: invalid item id %q", domain.ErrNotFound, itemID)
	}
	rows, err := r.Client.AcAuditEvent.Query().Where(where).
		Order(ent.Asc(acauditevent.FieldCreatedAt)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list ac audit events: %w", err)
	}
	if len(rows) == 0 {
		if exists, lookupErr := r.backlogItemExists(ctx, itemID); lookupErr != nil || !exists {
			return nil, fmt.Errorf("%w: backlog item %s", domain.ErrNotFound, itemID)
		}
	}
	out := make([]AcAuditEventData, len(rows))
	for i, e := range rows {
		out[i] = AcAuditEventData{
			ID: e.ID.String(), ItemID: e.ItemID.String(), ItemPublicID: e.ItemPublicID,
			CriterionIndex: e.CriterionIndex,
			PreviousStatus: e.PreviousStatus, NewStatus: e.NewStatus, Reason: e.Reason, Outcome: e.Outcome,
			ActorClass: e.ActorClass, ActorSessionUUID: e.ActorSessionUUID, SessionVerified: e.SessionVerified,
			Source: e.Source, RemoteAddr: e.RemoteAddr, UserAgent: e.UserAgent, CreatedAt: e.CreatedAt,
		}
	}
	return out, nil
}

func (r *EntRepository) backlogItemExists(ctx context.Context, itemID string) (bool, error) {
	id, err := r.resolveBacklogItemLookup(ctx, itemID)
	if err != nil {
		return false, err
	}
	return r.Client.BacklogItem.Query().Where(backlogitem.ID(id)).Exist(ctx)
}
