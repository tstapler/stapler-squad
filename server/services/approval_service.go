package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ciConclusionFailure is the GitHub CI check conclusion value the CI-red guard blocks
// on. Mirrors ciConclusionSuccess in pkg/classifier/classifier.go (Task 1.1.1d) — kept
// as a separate unexported copy since the two packages don't share this constant.
const ciConclusionFailure = "failure"

// ApprovalService handles Claude Code hook approval RPCs.
type ApprovalService struct {
	approvalStore      *ApprovalStore
	notificationStore  approvalNotificationStamper // optional; nil-safe
	eventBus           *events.EventBus            // optional; nil-safe; broadcasts resolution to connected clients
	liveFinder         LiveInstanceFinder          // optional; nil-safe — CI status for the block-on-red guard (not persisted, see plan.md's Implementation Deviations)
	reviewQueueRemover session.ReviewQueueRemover  // optional; nil-safe — removes the review-queue item on a reconciled resolve (Epic 2.3.1)
}

// NewApprovalService creates an ApprovalService with the given ApprovalStore.
func NewApprovalService(store *ApprovalStore) *ApprovalService {
	return &ApprovalService{approvalStore: store}
}

// SetNotificationStore wires in the notification history store so that resolved
// approvals are stamped with their decision in the notification metadata.
func (as *ApprovalService) SetNotificationStore(store approvalNotificationStamper) {
	as.notificationStore = store
}

// SetEventBus wires in the event bus so that resolved approvals are broadcast to all
// connected clients via the watchSessions stream. Without this, Device B has no
// real-time signal when Device A resolves an approval.
func (as *ApprovalService) SetEventBus(bus *events.EventBus) {
	as.eventBus = bus
}

// SetLiveInstanceFinder wires the live in-memory instance lookup used by the
// block-on-red-CI guard (AC5). See ApprovalHandler.SetLiveInstanceFinder for why this
// must be the live registry rather than *session.Storage.
func (as *ApprovalService) SetLiveInstanceFinder(f LiveInstanceFinder) {
	as.liveFinder = f
}

// SetReviewQueueRemover wires in the review queue's removal side so that a
// reconciliation-driven resolve (ResolveApprovalReconciled) also removes the
// corresponding review-queue item, carrying the rule name that resolved it
// (Epic 2.3.1) rather than leaving that to the poller's next pass.
func (as *ApprovalService) SetReviewQueueRemover(r session.ReviewQueueRemover) {
	as.reviewQueueRemover = r
}

// ---------------------------------------------------------------------------
// RPC methods
// ---------------------------------------------------------------------------

// resolutionSource identifies which caller is driving resolveApproval, so the
// human-vs-reconciliation arbitration below can tell the two apart.
type resolutionSource int

const (
	resolutionSourceHuman resolutionSource = iota
	resolutionSourceReconciliation
)

// resolveApproval is the single internal entry point every resolution path
// funnels through — a live human's RPC click and the rule-reconciliation
// pass — so the CI-red guard, notification stamping, and event broadcast
// apply uniformly, and the human/automation race (research/ux.md's "favor
// the human, not the automation" mandate) is arbitrated in exactly one
// place.
//
// Residual risk (accepted, adversarial-review.md iteration 3): the
// reconciliation branch checks IsHumanResolving once, at entry, and does
// not re-check immediately before approvalStore.Resolve() further down.
// A human's MarkHumanResolving landing after that single check but before
// reconciliation's Resolve() call still loses. This narrows the race
// window from iteration 2's "full RPC round-trip + guard lookup" down to
// "in-process CI-guard-lookup latency only" — not zero, but small enough
// that the reviewer did not classify it as a blocker for a single-operator
// tool. Fully closing it would require moving the IsHumanResolving check
// inside ApprovalStore.Resolve() itself, under the same lock as the
// delete — deferred as a follow-up, not required for this project's scope.
func (as *ApprovalService) resolveApproval(ctx context.Context, approvalID, decisionStr, message string, overrideCIBlock bool, source resolutionSource) (*sessionv1.ResolveApprovalResponse, error) {
	if approvalID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("approval_id is required"))
	}
	if decisionStr != "allow" && decisionStr != "deny" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("decision must be 'allow' or 'deny'"))
	}

	if source == resolutionSourceHuman {
		// Claim before any other work — including the CI-guard lookup below,
		// which can block on a live-instance-registry read — so a human
		// decision that has already reached the server can never lose to
		// reconciliation's in-process call during that lookup.
		as.approvalStore.MarkHumanResolving(approvalID)
		defer as.approvalStore.ClearHumanResolving(approvalID)
	} else if as.approvalStore.IsHumanResolving(approvalID) {
		// A human decision is already in flight for this exact approval —
		// defer to it entirely rather than racing the store's mutex.
		return nil, connect.NewError(connect.CodeAborted,
			fmt.Errorf("approval %s has a human decision in flight", approvalID))
	}

	decision := ApprovalDecision{
		Behavior: decisionStr,
		Message:  message,
	}

	// Fetch session ID before removing from store (needed below for both the CI-red
	// guard and the event broadcast).
	sessionID := ""
	if a, ok := as.approvalStore.Get(approvalID); ok {
		sessionID = a.SessionID
	}

	// AC5: block manual Approve when the session's branch has failing CI, unless the
	// reviewer explicitly overrides (Story 2.2.4). The lookup itself always runs (its
	// result is what the override log line reports); only the early-return decision is
	// conditional on overrideCIBlock.
	if decisionStr == "allow" && config.LoadConfig().GetFeatureFlag(blockApprovalOnCIFailureFlagName) && as.liveFinder != nil {
		if inst := as.liveFinder.FindLiveInstance(sessionID); inst != nil {
			// Read via Snapshot(), not raw fields: PRStatusPoller mutates these same
			// fields on its own goroutine under inst.mu (session/instance.go's mu doc
			// comment mandates Snapshot() for reads outside the actor).
			ghInfo := inst.Snapshot().GitHub
			blocked := ghInfo.GitHubPRNumber > 0 && ghInfo.GitHubCheckConclusion == ciConclusionFailure
			if blocked && overrideCIBlock {
				log.Info("[ApprovalService] approved despite failing CI (override)",
					"approval_id", approvalID, "session_id", sessionID, "ci_conclusion", ghInfo.GitHubCheckConclusion)
			} else if blocked {
				msg := "Approval blocked: CI is failing on this branch — review before approving."
				if ghInfo.GitHubPRURL != "" {
					msg += " " + ghInfo.GitHubPRURL + "/checks"
				}
				return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New(msg))
			}
		}
		// inst == nil (session not found/not live): fail open — an infrastructure lookup
		// miss should never hard-fail a human's explicit "Approve" click.
	}

	if err := as.approvalStore.Resolve(approvalID, decision); err != nil {
		if as.notificationStore != nil {
			if rec, ok := as.notificationStore.GetByID(approvalID); ok && rec.IsReconciled() {
				ruleName := rec.Metadata["classifier_rule_name"]
				return nil, connect.NewError(connect.CodeFailedPrecondition,
					fmt.Errorf("already auto-resolved by rule %q while you were reviewing it — no action needed", ruleName))
			}
		}
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	// Stamp the decision on the notification record so the panel can render the
	// correct badge after a page refresh. Approval ID == notification ID by convention
	// (wired in ApprovalHandler.broadcastApprovalNotification).
	if as.notificationStore != nil {
		if err := as.notificationStore.SetMetadata(approvalID, "approval_decision", decisionStr); err != nil {
			log.Warn("[ApprovalService] could not persist approval decision in notification", "err", err)
		}
		if _, err := as.notificationStore.MarkRead([]string{approvalID}); err != nil {
			log.Warn("[ApprovalService] could not mark approval notification read", "approval_id", approvalID, "err", err)
		}
	}

	// Broadcast resolution to all connected clients so Device B updates in real-time
	// without waiting for reconnect. Context carries the approval ID so clients can
	// correlate the event with the pending notification they're displaying.
	if as.eventBus != nil && sessionID != "" {
		approved := decisionStr == "allow"
		as.eventBus.Publish(events.NewApprovalResponseEvent(sessionID, approved, approvalID))
	}

	log.Info("[ApprovalService] resolved approval", "approval_id", approvalID, "decision", decisionStr)

	return &sessionv1.ResolveApprovalResponse{
		Success: true,
		Message: fmt.Sprintf("Approval %s resolved: %s", approvalID, decisionStr),
	}, nil
}

// ResolveApproval sends the user's decision to the blocked HTTP hook handler.
func (as *ApprovalService) ResolveApproval(
	ctx context.Context,
	req *connect.Request[sessionv1.ResolveApprovalRequest],
) (*connect.Response[sessionv1.ResolveApprovalResponse], error) {
	message := ""
	if req.Msg.Message != nil {
		message = *req.Msg.Message
	}
	resp, err := as.resolveApproval(ctx, req.Msg.ApprovalId, req.Msg.Decision, message, req.Msg.OverrideCiBlock, resolutionSourceHuman)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// ResolveApprovalReconciled resolves a pending approval on behalf of the
// rule-reconciliation pass (RulesService.reconcilePendingApprovals),
// reusing resolveApproval's exact guard/arbitration/stamp/broadcast logic
// so a reconciled resolution is indistinguishable in effect from a live
// one — then stamps two extra metadata keys so the UI and a later human
// click can both tell it apart from a live decision. Returns a
// connect.CodeAborted error, without resolving anything, if a human
// decision is currently in flight for approvalID (see resolveApproval).
func (as *ApprovalService) ResolveApprovalReconciled(ctx context.Context, approvalID, decision, ruleName string) error {
	// Capture the session ID before resolveApproval removes the approval from
	// the store below — needed to remove the corresponding review-queue item.
	sessionID := ""
	if a, ok := as.approvalStore.Get(approvalID); ok {
		sessionID = a.SessionID
	}

	if _, err := as.resolveApproval(ctx, approvalID, decision, "", false, resolutionSourceReconciliation); err != nil {
		return err
	}
	if as.notificationStore != nil {
		_ = as.notificationStore.SetMetadata(approvalID, "classifier_rule_name", ruleName)
		_ = as.notificationStore.SetMetadata(approvalID, "reconciled", "true")
	}
	if as.reviewQueueRemover != nil && sessionID != "" {
		as.reviewQueueRemover.RemoveWithInfo(sessionID, session.AutoResolvedByRuleRemoval(ruleName))
	}
	return nil
}

// IsApprovalPending reports whether approvalID is still present in the
// pending store. Used by reconciliation to disambiguate a genuine
// CI-red-guard decline (item remains pending) from "lost the race to a
// concurrent reconciliation pass" (item already gone) when both surface
// the same connect.CodeFailedPrecondition (adversarial-review.md Concern 2).
func (as *ApprovalService) IsApprovalPending(approvalID string) bool {
	_, ok := as.approvalStore.Get(approvalID)
	return ok
}

// ListPendingApprovalsInternal returns all pending approvals for internal Go
// callers (rule-reconciliation) that don't need ConnectRPC proto marshaling.
func (as *ApprovalService) ListPendingApprovalsInternal() []*PendingApproval {
	return as.approvalStore.ListAll()
}

// ListPendingApprovals returns all pending approval requests, optionally filtered by session ID.
func (as *ApprovalService) ListPendingApprovals(
	ctx context.Context,
	req *connect.Request[sessionv1.ListPendingApprovalsRequest],
) (*connect.Response[sessionv1.ListPendingApprovalsResponse], error) {
	var approvals []*PendingApproval
	if req.Msg.SessionId != nil && *req.Msg.SessionId != "" {
		approvals = as.approvalStore.GetBySession(*req.Msg.SessionId)
	} else {
		approvals = as.approvalStore.ListAll()
	}

	now := time.Now()
	protos := make([]*sessionv1.PendingApprovalProto, 0, len(approvals))
	for _, a := range approvals {
		remaining := int32(a.ExpiresAt.Sub(now).Seconds())
		if remaining < 0 {
			remaining = 0
		}
		toolInput := make(map[string]string, len(a.ToolInput))
		for k, v := range a.ToolInput {
			if str, ok := v.(string); ok {
				toolInput[k] = str
			} else {
				toolInput[k] = fmt.Sprintf("%v", v)
			}
		}
		protos = append(protos, &sessionv1.PendingApprovalProto{
			Id:               a.ID,
			SessionId:        a.SessionID,
			ToolName:         a.ToolName,
			ToolInput:        toolInput,
			Cwd:              a.Cwd,
			PermissionMode:   a.PermissionMode,
			CreatedAt:        timestamppb.New(a.CreatedAt),
			ExpiresAt:        timestamppb.New(a.ExpiresAt),
			SecondsRemaining: remaining,
			RiskLevel:        a.RiskLevel,
		})
	}

	return connect.NewResponse(&sessionv1.ListPendingApprovalsResponse{
		Approvals: protos,
	}), nil
}
