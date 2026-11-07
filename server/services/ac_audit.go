package services

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/middleware"
	"github.com/tstapler/stapler-squad/session"
)

const maxAuditUserAgent = 200

// acActorFromRequest derives the best-available caller identity for an AC mutation.
//
// This is a claim about the caller, not proof of who "the owner" is. A request that carries
// X-Stapler-Session-UUID or a session proof is treated as an agent session (verified only when
// the proof checks out). A request that passed the auth middleware is recorded with the credential
// it presented (cookie or bearer token; either may derive from the local API token). Everything
// else is "unverified": on the local loopback surface an agent's curl and the operator's own CLI
// look identical, so the absence of an agent header must never be read as "operator".
func acActorFromRequest(ctx context.Context, h http.Header, peerAddr string) session.AcActor {
	actor := session.AcActor{
		Class:      session.AcActorUnverified,
		Source:     "api",
		RemoteAddr: truncateAuditField(peerAddr, 100),
		UserAgent:  truncateAuditField(h.Get("User-Agent"), maxAuditUserAgent),
	}
	if h.Get("Sec-Fetch-Site") == "same-origin" {
		actor.Source = "ui"
	}
	sid, proof := strings.TrimSpace(h.Get("X-Stapler-Session-UUID")), h.Get(session.MCPProofHeader)
	switch {
	case sid != "" || proof != "":
		actor.Class = session.AcActorAgentSession
		if _, err := uuid.Parse(sid); err == nil {
			actor.SessionUUID = sid
			if p := DefaultMcpProofs(); p != nil && proof != "" {
				actor.SessionVerified = p.Verify(sid, proof)
			}
		} else {
			actor.SessionUUID = "(invalid id)" // never echo attacker-controlled text into the trail
		}
	case middleware.AuthCredentialKind(ctx) == middleware.CredentialCookie:
		actor.Class = session.AcActorAuthenticatedSession
	case middleware.AuthCredentialKind(ctx) == middleware.CredentialBearer:
		actor.Class = session.AcActorAuthenticatedBearer
	}
	return actor
}

func truncateAuditField(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// recordDeniedAcAttempt appends an audit row for an attempt refused before it reached storage.
// Failing to write it is logged, never allowed to turn a denial into an allow.
func (s *BacklogService) recordDeniedAcAttempt(ctx context.Context, itemID string, index int, reason, outcome string) {
	err := s.storage.RecordAcAudit(ctx, session.AcAuditEventData{
		ItemID: itemID, CriterionIndex: index, NewStatus: string(session.AcStatusWaived), Reason: reason, Outcome: outcome,
	})
	if err != nil {
		log.Warn("failed to record AC audit event", "item", itemID, "outcome", outcome, "err", err)
	}
}

// ListAcAuditEvents returns the read-only audit trail of AC changes for an item.
// +api: ListAcAuditEvents
func (s *BacklogService) ListAcAuditEvents(
	ctx context.Context,
	req *connect.Request[sessionv1.ListAcAuditEventsRequest],
) (*connect.Response[sessionv1.ListAcAuditEventsResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}
	rows, err := s.storage.ListAcAudit(ctx, req.Msg.ItemId)
	if errors.Is(err, session.ErrNotFound) {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("backlog item %q not found", req.Msg.ItemId))
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	out := make([]*sessionv1.AcAuditEvent, len(rows))
	for i, r := range rows {
		out[i] = &sessionv1.AcAuditEvent{
			Id: r.ID, ItemId: r.ItemID,
			// #nosec G115 -- index into one item's short AC list.
			CriterionIndex: int32(r.CriterionIndex),
			PreviousStatus: r.PreviousStatus, NewStatus: r.NewStatus, Reason: r.Reason, Outcome: r.Outcome,
			ActorClass: r.ActorClass, ActorSessionUuid: r.ActorSessionUUID, SessionVerified: r.SessionVerified,
			Source: r.Source, RemoteAddr: r.RemoteAddr, UserAgent: r.UserAgent,
			CreatedAt: timestamppb.New(r.CreatedAt),
		}
	}
	return connect.NewResponse(&sessionv1.ListAcAuditEventsResponse{Events: out}), nil
}

// acCriteriaToProto converts criteria (used for the waivers an update dropped) to proto form.
func acCriteriaToProto(cs []session.AcCriterion) []*sessionv1.AcCriterion {
	out := make([]*sessionv1.AcCriterion, len(cs))
	for i, c := range cs {
		out[i] = &sessionv1.AcCriterion{
			// #nosec G115 -- index into one item's short AC list.
			Index: int32(c.Index), Text: c.Text, Status: string(c.Status),
			WaiverReason: c.WaiverReason, WaivedBy: c.WaivedBy, WaivedAt: c.WaivedAt,
		}
	}
	return out
}
