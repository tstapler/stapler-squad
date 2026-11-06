package session

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

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
	MaxAcAuditPerItem    = 500
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

// AcActorFrom returns the attached actor, or an Unverified api caller.
func AcActorFrom(ctx context.Context) AcActor {
	if a, ok := ctx.Value(acActorKey{}).(AcActor); ok {
		return a
	}
	return AcActor{Class: AcActorUnverified, Source: "api"}
}

// Label is the human-readable "who" shown next to a waiver.
func (a AcActor) Label() string {
	switch a.Class {
	case AcActorAuthenticatedSession:
		return "authenticated web session"
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

// createAcAuditEvent appends one row and trims the item's trail to MaxAcAuditPerItem (oldest
// first). It runs on the caller's transaction (or client) so the row commits with the change.
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
	return pruneAcAuditOverflow(ctx, client, itemID)
}

func pruneAcAuditOverflow(ctx context.Context, client *ent.AcAuditEventClient, itemID uuid.UUID) error {
	n, err := client.Query().Where(acauditevent.ItemID(itemID)).Count(ctx)
	if err != nil || n <= MaxAcAuditPerItem {
		return err
	}
	oldest, err := client.Query().Where(acauditevent.ItemID(itemID)).
		Order(ent.Asc(acauditevent.FieldCreatedAt)).Limit(n - MaxAcAuditPerItem).IDs(ctx)
	if err != nil {
		return err
	}
	_, err = client.Delete().Where(acauditevent.IDIn(oldest...)).Exec(ctx)
	return err
}

// PruneAcAuditOlderThan deletes audit rows older than cutoff (retention for the trail of deleted
// items, which no per-item write ever trims) and returns how many were removed.
func (r *EntRepository) PruneAcAuditOlderThan(ctx context.Context, cutoff time.Time) (int, error) {
	return r.client.AcAuditEvent.Delete().Where(acauditevent.CreatedAtLT(cutoff)).Exec(ctx)
}

// RecordAcAudit appends an audit row for an attempt refused before it reached the criterion
// (for example a denied caller), using the actor attached to ctx.
func (r *EntRepository) RecordAcAudit(ctx context.Context, d AcAuditEventData) error {
	if d.ItemPublicID == "" {
		if id, err := uuid.Parse(d.ItemID); err == nil {
			if it, err := r.client.BacklogItem.Get(ctx, id); err == nil {
				d.ItemPublicID = it.PublicID
			}
		}
	}
	return r.createAcAuditEvent(ctx, r.client.AcAuditEvent, d)
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
		return nil, fmt.Errorf("%w: invalid item id %q", ErrNotFound, itemID)
	}
	rows, err := r.client.AcAuditEvent.Query().Where(where).
		Order(ent.Asc(acauditevent.FieldCreatedAt)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list ac audit events: %w", err)
	}
	if len(rows) == 0 {
		if exists, lookupErr := r.backlogItemExists(ctx, itemID); lookupErr != nil || !exists {
			return nil, fmt.Errorf("%w: backlog item %s", ErrNotFound, itemID)
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
	return r.client.BacklogItem.Query().Where(backlogitem.ID(id)).Exist(ctx)
}
