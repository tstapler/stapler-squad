package services

import (
	"context"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/wakes"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// Peer-help row states shown to the operator (UX-S4).
const (
	peerStateWaiting   = "waiting"
	peerStateAnswered  = "answered"
	peerStateNotSent   = "not_sent"
	peerStateCancelled = "cancelled"
)

// ListPeerHelpRequests lists open and recently ended agent-to-agent requests.
//
// +api: wake-trigger:list-peer-help
func (s *WakeTriggerService) ListPeerHelpRequests(
	ctx context.Context, _ *connect.Request[sessionv1.ListPeerHelpRequestsRequest],
) (*connect.Response[sessionv1.ListPeerHelpRequestsResponse], error) {
	rows, err := s.deps.Store.ListPeerGuidanceRequests(ctx, s.deps.Now().Add(-peerListWindow))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	facts := s.ownerFactsCache(ctx)
	out := &sessionv1.ListPeerHelpRequestsResponse{}
	for _, r := range rows {
		out.Requests = append(out.Requests, s.peerInfoOf(r, facts))
	}
	return connect.NewResponse(out), nil
}

func (s *WakeTriggerService) peerInfoOf(r *session.GuidanceRequestData, facts func(string) wakes.OwnerFacts) *sessionv1.PeerHelpInfo {
	info := &sessionv1.PeerHelpInfo{
		Id: r.ID, AskerSessionUuid: r.AskerSessionUUID, AskerTitle: s.title(r.AskerSessionUUID),
		AnswererSessionUuid: r.AnswererSessionUUID, AnswererTitle: s.title(r.AnswererSessionUUID),
		Question: r.QuestionText, Escalated: r.NotifiedAt != nil,
		CreatedAt: timestamppb.New(r.CreatedAt), AnsweredAt: tsOrNil(r.AnsweredAt),
	}
	switch r.Status() {
	case session.GuidanceRequestStatusCancelled:
		info.State, info.StateReason = peerStateCancelled, cancelText(r.CancelReason)
	case session.GuidanceRequestStatusAnswered:
		info.State, info.Answer = peerStateAnswered, r.Answer
		info.AnsweredBy = s.answeredBy(r)
		if r.AnswererKind == domain.AnswererKindHuman && r.AnswererSessionUUID != "" {
			info.OnBehalfOf = info.AnswererTitle
		}
	default:
		info.State = peerStateWaiting
		if reason, notSent, faulted := wakes.PeerFaultOf(facts(r.AnswererSessionUUID), facts(r.AskerSessionUUID)); faulted {
			info.StateReason = reason
			if notSent {
				info.State = peerStateNotSent
			}
		}
	}
	return info
}

func cancelText(reason domain.CancelReason) string {
	if reason == wakes.OperatorCancelReason {
		return "cancelled by the operator"
	}
	return "cancelled"
}
