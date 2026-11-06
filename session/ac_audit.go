package session

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/acauditevent"
)

// ErrAcCriterionAlreadyDone is returned when the owner waiver targets a criterion already done.
var ErrAcCriterionAlreadyDone = errors.New("acceptance criterion is already done; nothing to waive")

// AcActorClass says how strongly a caller's identity is known. Only OperatorAuthenticated
// and a SessionVerified MCP session are proven; Unverified means "no identity signal at all",
// which on the local Connect surface is what both the operator's CLI and an agent's curl look like.
type AcActorClass string

const (
	AcActorOperatorAuthenticated AcActorClass = "operator_authenticated"
	AcActorAgentSession          AcActorClass = "agent_session"
	AcActorMCPSession            AcActorClass = "mcp_session"
	AcActorUnverified            AcActorClass = "unverified"
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
	case AcActorOperatorAuthenticated:
		return "operator (authenticated)"
	case AcActorAgentSession, AcActorMCPSession:
		verified := "unverified"
		if a.SessionVerified {
			verified = "verified"
		}
		return fmt.Sprintf("agent session %s (%s)", a.SessionUUID, verified)
	default:
		return "unverified caller"
	}
}

// AcAuditEventData is the domain DTO of one AcAuditEvent row.
type AcAuditEventData struct {
	ID               string
	ItemID           string
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

func (r *EntRepository) createAcAuditEvent(ctx context.Context, c *ent.AcAuditEventCreate, d AcAuditEventData) error {
	itemID, err := uuid.Parse(d.ItemID)
	if err != nil {
		return fmt.Errorf("ac audit: invalid item id %q: %w", d.ItemID, err)
	}
	a := AcActorFrom(ctx)
	if _, err := c.SetItemID(itemID).
		SetCriterionIndex(d.CriterionIndex).
		SetPreviousStatus(d.PreviousStatus).
		SetNewStatus(d.NewStatus).
		SetReason(d.Reason).
		SetOutcome(d.Outcome).
		SetActorClass(string(a.Class)).
		SetActorSessionUUID(a.SessionUUID).
		SetSessionVerified(a.SessionVerified).
		SetSource(a.Source).
		SetRemoteAddr(a.RemoteAddr).
		SetUserAgent(a.UserAgent).
		Save(ctx); err != nil {
		return fmt.Errorf("record ac audit event: %w", err)
	}
	return nil
}

// RecordAcAudit appends an audit row for an attempt refused before it reached the criterion
// (for example a denied caller), using the actor attached to ctx.
func (r *EntRepository) RecordAcAudit(ctx context.Context, d AcAuditEventData) error {
	return r.createAcAuditEvent(ctx, r.client.AcAuditEvent.Create(), d)
}

// ListAcAudit returns an item's audit rows oldest first.
func (r *EntRepository) ListAcAudit(ctx context.Context, itemID string) ([]AcAuditEventData, error) {
	parsedID, err := r.resolveBacklogItemLookup(ctx, itemID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: backlog item %s", ErrNotFound, itemID)
		}
		return nil, fmt.Errorf("invalid item id %q: %w", itemID, err)
	}
	rows, err := r.client.AcAuditEvent.Query().
		Where(acauditevent.ItemID(parsedID)).
		Order(ent.Asc(acauditevent.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list ac audit events: %w", err)
	}
	out := make([]AcAuditEventData, len(rows))
	for i, e := range rows {
		out[i] = AcAuditEventData{
			ID: e.ID.String(), ItemID: e.ItemID.String(), CriterionIndex: e.CriterionIndex,
			PreviousStatus: e.PreviousStatus, NewStatus: e.NewStatus, Reason: e.Reason, Outcome: e.Outcome,
			ActorClass: e.ActorClass, ActorSessionUUID: e.ActorSessionUUID, SessionVerified: e.SessionVerified,
			Source: e.Source, RemoteAddr: e.RemoteAddr, UserAgent: e.UserAgent, CreatedAt: e.CreatedAt,
		}
	}
	return out, nil
}
