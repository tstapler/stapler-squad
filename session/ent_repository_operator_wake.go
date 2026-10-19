package session

import (
	"context"
	"fmt"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/guidancerequest"
	"github.com/tstapler/stapler-squad/session/ent/waketrigger"
)

// Operator-facing reads of wake triggers and peer-help rows (Stories 4.6, 4.11).

// ListWakeTriggersByStatus returns the rows of every owner in any of statuses,
// newest first. The operator list needs all owners, which ListWakeTriggersByOwner
// cannot give.
func (r *EntRepository) ListWakeTriggersByStatus(ctx context.Context, statuses ...domain.WakeStatus) ([]*WakeTriggerData, error) {
	if len(statuses) == 0 {
		return nil, nil
	}
	return r.listWakeTriggers(ctx, r.client.WakeTrigger.Query().
		Where(waketrigger.StatusIn(wakeStatusStrings(statuses)...)).
		Order(ent.Desc(waketrigger.FieldCreatedAt)))
}

// ListPeerGuidanceRequests returns peer-help rows that are open or were created
// at or after since, newest first.
func (r *EntRepository) ListPeerGuidanceRequests(ctx context.Context, since time.Time) ([]*GuidanceRequestData, error) {
	rows, err := r.client.GuidanceRequest.Query().
		Where(
			guidancerequest.Source(string(domain.QuestionSourcePeer)),
			guidancerequest.Or(
				guidancerequest.And(guidancerequest.AnsweredAtIsNil(), guidancerequest.CancelledAtIsNil()),
				guidancerequest.CreatedAtGTE(since),
			),
		).
		Order(guidancerequest.ByCreatedAt(sql.OrderDesc())).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list peer guidance requests: %w", err)
	}
	return guidanceRequestsToData(rows), nil
}

// ListOpenPeerGuidanceRequests returns the open peer-help rows, oldest first.
func (r *EntRepository) ListOpenPeerGuidanceRequests(ctx context.Context) ([]*GuidanceRequestData, error) {
	rows, err := r.client.GuidanceRequest.Query().
		Where(
			guidancerequest.Source(string(domain.QuestionSourcePeer)),
			guidancerequest.AnsweredAtIsNil(),
			guidancerequest.CancelledAtIsNil(),
		).
		Order(guidancerequest.ByCreatedAt(sql.OrderAsc())).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list open peer guidance requests: %w", err)
	}
	return guidanceRequestsToData(rows), nil
}

// AnswerGuidanceRequestOnBehalf records the operator's answer to a peer
// request and flips answerer_kind to human, so readers can tell the operator
// answered in the peer's place (answerer_session_uuid keeps naming the peer).
// Same guard as AnswerGuidanceRequest.
func (r *EntRepository) AnswerGuidanceRequestOnBehalf(ctx context.Context, id uuid.UUID, answer string) (applied bool, err error) {
	n, err := r.client.GuidanceRequest.Update().
		Where(
			guidancerequest.ID(id),
			guidancerequest.AnsweredAtIsNil(),
			guidancerequest.CancelledAtIsNil(),
		).
		SetAnswer(answer).
		SetAnsweredAt(time.Now()).
		SetAnswererKind(string(domain.AnswererKindHuman)).
		Save(ctx)
	if err != nil {
		return false, fmt.Errorf("answer guidance request %s on behalf of its peer: %w", id, err)
	}
	if n > 0 {
		r.recordGuidanceAnswered(ctx, id)
	}
	return n > 0, nil
}
