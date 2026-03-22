package services

import (
	"context"
	"fmt"
	"time"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// notificationMetadataStore is the narrow interface ApprovalService needs to stamp
// the approval decision onto the notification record after it is resolved.
type notificationMetadataStore interface {
	SetMetadata(id, key, value string) error
}

// ApprovalService handles Claude Code hook approval RPCs.
type ApprovalService struct {
	approvalStore     *ApprovalStore
	notificationStore notificationMetadataStore // optional; nil-safe
}

// NewApprovalService creates an ApprovalService with the given ApprovalStore.
func NewApprovalService(store *ApprovalStore) *ApprovalService {
	return &ApprovalService{approvalStore: store}
}

// SetNotificationStore wires in the notification history store so that resolved
// approvals are stamped with their decision in the notification metadata.
func (as *ApprovalService) SetNotificationStore(store notificationMetadataStore) {
	as.notificationStore = store
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

	if err := as.approvalStore.Resolve(req.Msg.ApprovalId, decision); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	// Stamp the decision on the notification record so the panel can render the
	// correct badge after a page refresh. Approval ID == notification ID by convention
	// (wired in ApprovalHandler.broadcastApprovalNotification).
	if as.notificationStore != nil {
		if err := as.notificationStore.SetMetadata(req.Msg.ApprovalId, "approval_decision", req.Msg.Decision); err != nil {
			log.WarningLog.Printf("[ApprovalService] Could not persist approval decision in notification: %v", err)
		}
	}

	log.InfoLog.Printf("[ApprovalService] Resolved approval %s: %s", req.Msg.ApprovalId, req.Msg.Decision)

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
		})
	}

	return connect.NewResponse(&sessionv1.ListPendingApprovalsResponse{
		Approvals: protos,
	}), nil
}
