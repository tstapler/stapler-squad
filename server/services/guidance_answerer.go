package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// ErrAnswererForbidden is returned when the answerer is not the one the request
// names; the request is left unchanged.
var ErrAnswererForbidden = errors.New("answerer is not permitted to answer this request")

// Answerer is who answers a guidance request. The zero value is not valid; use
// HumanAnswerer or SessionAnswerer.
type Answerer struct {
	kind        domain.AnswererKind
	sessionUUID string
}

// HumanAnswerer is the operator, who may answer any request (including a
// stalled peer request, on the peer's behalf).
func HumanAnswerer() Answerer { return Answerer{kind: domain.AnswererKindHuman} }

// SessionAnswerer is a session whose identity was verified by the caller.
func SessionAnswerer(sessionUUID string) Answerer {
	return Answerer{kind: domain.AnswererKindSession, sessionUUID: sessionUUID}
}

// authorize says whether who may answer data. A session may answer only a peer
// request that names it; it can never answer one addressed to the human or to
// another session (ADR-007 rule 3).
func (who Answerer) authorize(data *session.GuidanceRequestData) error {
	switch who.kind {
	case domain.AnswererKindHuman:
		return nil
	case domain.AnswererKindSession:
		if who.sessionUUID != "" && data.Source == domain.QuestionSourcePeer &&
			data.AnswererKind == domain.AnswererKindSession && data.AnswererSessionUUID == who.sessionUUID {
			return nil
		}
	}
	return ErrAnswererForbidden
}

// AnswerAs records answer on request id if who is permitted to give it. applied
// is false when the request was already answered or cancelled; the returned
// data then reflects the current persisted state. On an applied answer it
// notifies, resumes triage and calls the answer listener, as for a human answer.
func (s *GuidanceRequestService) AnswerAs(ctx context.Context, id, answer string, who Answerer) (data *session.GuidanceRequestData, applied bool, err error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return nil, false, fmt.Errorf("invalid guidance request id: %w", err)
	}
	data, err = s.storage.GetGuidanceRequest(ctx, parsed)
	if err != nil {
		return nil, false, err
	}
	if err := who.authorize(data); err != nil {
		return data, false, err
	}
	applied, err = s.recordAnswer(ctx, parsed, answer, who, data)
	if err != nil {
		return nil, false, err
	}
	data, err = s.storage.GetGuidanceRequest(ctx, parsed)
	if err != nil {
		return nil, false, err
	}
	if applied {
		s.saveForLaterIfSessionEnded(ctx, data)
		s.notifyAnswered(data)
		s.resumeTriageIfHalted(data)
		s.notifyAnswerListener(ctx, data.ID)
	}
	return data, applied, nil
}

// saveForLaterIfSessionEnded marks an answer to an agent-created session question
// as saved for later when its session is stopped (resume route) or archived (read_only
// route): the agent reads it with get_guidance_request once the operator resumes the
// session. Nothing is delivered or revived here (ADR-005 rung 3). Hook and peer rows
// die with their session and are never saved.
func (s *GuidanceRequestService) saveForLaterIfSessionEnded(ctx context.Context, data *session.GuidanceRequestData) {
	p := s.probes.Load()
	if p == nil || data.Source != domain.QuestionSourceAgentMCP || data.Scope != domain.RequestScopeSession {
		return
	}
	sessionUUID := askerSessionOf(data)
	route := domain.DeliveryRouteResume
	switch {
	case p.archived(sessionUUID):
		route = domain.DeliveryRouteReadOnly
	case !p.ended(sessionUUID):
		return
	}
	id, err := uuid.Parse(data.ID)
	if err != nil {
		return
	}
	if _, err := s.storage.SetGuidanceDelivery(ctx, id, route, domain.DeliveryStateSavedForLater, 0); err != nil {
		log.Warn("[GuidanceRequest] answer stored but not marked saved for later", "id", data.ID, "err", err)
	}
}

// recordAnswer persists the answer. The operator answering a peer request
// stands in for the peer, and the row records that so the asker is told.
func (s *GuidanceRequestService) recordAnswer(ctx context.Context, id uuid.UUID, answer string, who Answerer, data *session.GuidanceRequestData) (bool, error) {
	if who.kind == domain.AnswererKindHuman && data.Source == domain.QuestionSourcePeer && data.AnswererSessionUUID != "" {
		return s.storage.AnswerGuidanceRequestOnBehalf(ctx, id, answer)
	}
	return s.storage.AnswerGuidanceRequest(ctx, id, answer)
}
