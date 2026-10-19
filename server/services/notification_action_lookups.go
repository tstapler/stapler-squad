package services

import (
	"context"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// notification_action_lookups.go holds the production adapters behind the
// per-page descriptor lookups. Each performs one store read per page.

// approvalStoreLookup answers ApprovalLookup from the in-memory approval store.
type approvalStoreLookup struct{ store *ApprovalStore }

// NewApprovalLookup adapts the approval store to ApprovalLookup.
func NewApprovalLookup(store *ApprovalStore) ApprovalLookup { return approvalStoreLookup{store} }

func (l approvalStoreLookup) PendingApprovalIDs() map[string]bool {
	out := map[string]bool{}
	if l.store == nil {
		return out
	}
	for _, a := range l.store.ListAll() {
		out[a.ID] = true
	}
	return out
}

// guidanceLister is the one storage read the guidance lookup needs.
type guidanceLister interface {
	ListAllPendingGuidanceRequests(ctx context.Context) ([]*session.GuidanceRequestData, error)
}

type guidanceStorageLookup struct{ storage guidanceLister }

// NewGuidanceLookup adapts guidance storage to GuidanceLookup.
func NewGuidanceLookup(storage guidanceLister) GuidanceLookup { return guidanceStorageLookup{storage} }

func (l guidanceStorageLookup) OpenGuidance(ctx context.Context, ids []string) (map[string]OpenGuidance, error) {
	rows, err := l.storage.ListAllPendingGuidanceRequests(ctx)
	if err != nil {
		return nil, err
	}
	want := make(map[string]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	out := map[string]OpenGuidance{}
	for _, r := range rows {
		if want[r.ID] {
			out[r.ID] = OpenGuidance{Options: decodeGuidanceOptions(r.Options), Source: r.Source, PeerHelp: r.Source == domain.QuestionSourcePeer}
		}
	}
	return out, nil
}

// backlogLister is the one storage read the backlog decision lookup needs.
type backlogLister interface {
	ListBacklogItems(ctx context.Context, filter session.BacklogItemFilter) ([]session.BacklogItemData, error)
}

type backlogStorageLookup struct{ storage backlogLister }

// NewBacklogDecisionLookup adapts backlog storage to BacklogDecisionLookup. One
// query lists the items that can be awaiting a decision (review, pr_pending); an
// item in any other status is absent, which reduces its row to OPEN.
func NewBacklogDecisionLookup(storage backlogLister) BacklogDecisionLookup {
	return backlogStorageLookup{storage}
}

func (l backlogStorageLookup) DecisionStates(ctx context.Context, ids []string) (map[string]BacklogDecisionState, error) {
	rows, err := l.storage.ListBacklogItems(ctx, session.BacklogItemFilter{
		Statuses: []string{backlogStatusReview, backlogStatusPRPending},
	})
	if err != nil {
		return nil, err
	}
	want := make(map[string]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	out := map[string]BacklogDecisionState{}
	for _, r := range rows {
		if want[r.ID] {
			out[r.ID] = BacklogDecisionState{Status: r.Status}
		}
	}
	return out, nil
}
