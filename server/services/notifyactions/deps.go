// Package notifyactions dispatches inline notification actions
// (InvokeNotificationAction): validation, tier policy, flags, idempotency and
// audit, then delegation to the handlers that own each decision.
package notifyactions

import (
	"context"

	"github.com/tstapler/stapler-squad/server/notifications"
	"github.com/tstapler/stapler-squad/server/services/approvalkit"
	"github.com/tstapler/stapler-squad/server/services/approvals"
	"github.com/tstapler/stapler-squad/server/services/auditkit"
	"github.com/tstapler/stapler-squad/server/services/featureflags"
	"github.com/tstapler/stapler-squad/server/services/guidance"
	"github.com/tstapler/stapler-squad/server/services/notify"
	"github.com/tstapler/stapler-squad/session/domain"
)

// Storage is what the descriptor lookups and the backlog decider read.
type Storage interface {
	backlogStorage
	backlogLister
	ListAllPendingGuidanceRequests(ctx context.Context) ([]*domain.GuidanceRequestData, error)
}

// Deps are the production collaborators of the dispatcher and its lookups.
type Deps struct {
	Approvals     *approvals.ApprovalService
	ApprovalStore *approvalkit.ApprovalStore
	Questions     QuestionReplier
	History       *notifications.NotificationHistoryStore
	// Guidance, Storage and Transitions are optional; leave each a nil value
	// (never a boxed nil pointer) to disable the kinds that need it.
	Guidance    *guidance.GuidanceRequestService
	Storage     Storage
	Transitions BacklogTransitioner
	Merger      PRMerger
	Cancels     CancelDelegate
	// Audit may return nil while no sink exists.
	Audit func() *auditkit.AuditSink
	// Registry must be non-nil for NewDispatcher.
	Registry *featureflags.Registry
}

// DescriptorSources returns the lookups d can serve: approvals, plus backlog and
// guidance when there is storage. The caller adds the question sources.
func (d Deps) DescriptorSources() notify.DescriptorSources {
	src := notify.DescriptorSources{Approvals: NewApprovalLookup(d.ApprovalStore)}
	if d.Storage != nil {
		src.Backlog = NewBacklogDecisionLookup(d.Storage)
		src.Guidance = NewGuidanceLookup(d.Storage)
	}
	return src
}

// NewDispatcher builds the production dispatcher over the existing handlers.
func NewDispatcher(d Deps) *NotificationActionDispatcher {
	delegates := ActionDelegates{
		Approvals: NewApprovalResolver(d.Approvals, d.ApprovalStore),
		Questions: d.Questions,
		Resolver:  NewNotificationResolver(d.History),
		Cancels:   d.Cancels,
	}
	if d.Guidance != nil {
		delegates.Guidance = NewGuidanceAnswerer(d.Guidance)
	}
	if d.Transitions != nil && d.Storage != nil {
		delegates.Backlog = NewBacklogDecider(BacklogDeciderDeps{
			Status:      NewItemStatusReader(d.Storage),
			Transitions: d.Transitions,
			Sessions:    NewWorkSessionFinder(d.Storage),
			Merger:      d.Merger,
		})
	}
	audit := d.Audit
	return NewNotificationActionDispatcher(delegates, func() ActionAudit {
		if audit == nil {
			return nil
		}
		if sink := audit(); sink != nil {
			return sink
		}
		return nil
	}, d.Registry)
}
