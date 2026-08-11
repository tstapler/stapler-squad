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

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ciConclusionFailure is the GitHub CI check conclusion value the CI-red guard blocks
// on. Mirrors ciConclusionSuccess in pkg/classifier/classifier.go (Task 1.1.1d) — kept
// as a separate unexported copy since the two packages don't share this constant.
const ciConclusionFailure = "failure"

// ApprovalService handles Claude Code hook approval RPCs.
type ApprovalService struct {
	approvalStore     *ApprovalStore
	notificationStore approvalNotificationStamper // optional; nil-safe
	eventBus          *events.EventBus            // optional; nil-safe; broadcasts resolution to connected clients
	liveFinder        LiveInstanceFinder          // optional; nil-safe — CI status for the block-on-red guard (not persisted, see plan.md's Implementation Deviations)
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

// ---------------------------------------------------------------------------
// RPC methods
// ---------------------------------------------------------------------------

// ResolveApproval sends the user's decision to the blocked HTTP hook handler.
func (as *ApprovalService) ResolveApproval(
	ctx context.Context,
	req *connect.Request[sessionv1.ResolveApprovalRequest],
) (*connect.Response[sessionv1.ResolveApprovalResponse], error) {
	if req.Msg.ApprovalId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("approval_id is required"))
	}
	if req.Msg.Decision != "allow" && req.Msg.Decision != "deny" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("decision must be 'allow' or 'deny'"))
	}

	message := ""
	if req.Msg.Message != nil {
		message = *req.Msg.Message
	}

	decision := ApprovalDecision{
		Behavior: req.Msg.Decision,
		Message:  message,
	}

	// Fetch session ID before removing from store (needed below for both the CI-red
	// guard and the event broadcast).
	sessionID := ""
	if a, ok := as.approvalStore.Get(req.Msg.ApprovalId); ok {
		sessionID = a.SessionID
	}

	// AC5: block manual Approve when the session's branch has failing CI, unless the
	// reviewer explicitly overrides (Story 2.2.4). The lookup itself always runs (its
	// result is what the override log line reports); only the early-return decision is
	// conditional on OverrideCiBlock.
	if req.Msg.Decision == "allow" && config.LoadConfig().GetFeatureFlag(blockApprovalOnCIFailureFlagName) && as.liveFinder != nil {
		if inst := as.liveFinder.FindLiveInstance(sessionID); inst != nil {
			// Read via Snapshot(), not raw fields: PRStatusPoller mutates these same
			// fields on its own goroutine under inst.mu (session/instance.go's mu doc
			// comment mandates Snapshot() for reads outside the actor).
			ghInfo := inst.Snapshot().GitHub
			blocked := ghInfo.GitHubPRNumber > 0 && ghInfo.GitHubCheckConclusion == ciConclusionFailure
			if blocked && req.Msg.OverrideCiBlock {
				log.Info("[ApprovalService] approved despite failing CI (override)",
					"approval_id", req.Msg.ApprovalId, "session_id", sessionID, "ci_conclusion", ghInfo.GitHubCheckConclusion)
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

	if err := as.approvalStore.Resolve(req.Msg.ApprovalId, decision); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	// Stamp the decision on the notification record so the panel can render the
	// correct badge after a page refresh. Approval ID == notification ID by convention
	// (wired in ApprovalHandler.broadcastApprovalNotification).
	if as.notificationStore != nil {
		if err := as.notificationStore.SetMetadata(req.Msg.ApprovalId, "approval_decision", req.Msg.Decision); err != nil {
			log.Warn("[ApprovalService] could not persist approval decision in notification", "err", err)
		}
		if _, err := as.notificationStore.MarkRead([]string{req.Msg.ApprovalId}); err != nil {
			log.Warn("[ApprovalService] could not mark approval notification read", "approval_id", req.Msg.ApprovalId, "err", err)
		}
	}

	// Broadcast resolution to all connected clients so Device B updates in real-time
	// without waiting for reconnect. Context carries the approval ID so clients can
	// correlate the event with the pending notification they're displaying.
	if as.eventBus != nil && sessionID != "" {
		approved := req.Msg.Decision == "allow"
		as.eventBus.Publish(events.NewApprovalResponseEvent(sessionID, approved, req.Msg.ApprovalId))
	}

	log.Info("[ApprovalService] resolved approval", "approval_id", req.Msg.ApprovalId, "decision", req.Msg.Decision)

	return connect.NewResponse(&sessionv1.ResolveApprovalResponse{
		Success: true,
		Message: fmt.Sprintf("Approval %s resolved: %s", req.Msg.ApprovalId, req.Msg.Decision),
	}), nil
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
