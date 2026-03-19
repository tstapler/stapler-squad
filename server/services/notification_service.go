package services

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	sessionv1 "claude-squad/gen/proto/go/session/v1"
	"claude-squad/log"
	"claude-squad/server/events"
	"claude-squad/server/notifications"
	"claude-squad/session"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// NotificationService handles notification sending and history RPCs.
//
// Dependencies:
//   - notificationStore:      persists notification history
//   - notificationRateLimiter: rate-limits per-session notification sends
//   - eventBus:               broadcasts notification events to connected clients
//   - reviewQueuePoller:      late-wired; used to resolve session names
type NotificationService struct {
	notificationStore       *notifications.NotificationHistoryStore
	notificationRateLimiter *NotificationRateLimiter
	eventBus                *events.EventBus
	reviewQueuePoller       *session.ReviewQueuePoller
}

// NewNotificationService creates a NotificationService with the given dependencies.
func NewNotificationService(
	rateLimiter *NotificationRateLimiter,
	eventBus *events.EventBus,
) *NotificationService {
	return &NotificationService{
		notificationRateLimiter: rateLimiter,
		eventBus:                eventBus,
	}
}

// SetNotificationStore sets the notification history store (late-wired).
func (ns *NotificationService) SetNotificationStore(store *notifications.NotificationHistoryStore) {
	ns.notificationStore = store
}

// GetNotificationStore returns the notification history store.
func (ns *NotificationService) GetNotificationStore() *notifications.NotificationHistoryStore {
	return ns.notificationStore
}

// SetReviewQueuePoller sets the review queue poller for resolving session names.
func (ns *NotificationService) SetReviewQueuePoller(poller *session.ReviewQueuePoller) {
	ns.reviewQueuePoller = poller
}

// ---------------------------------------------------------------------------
// RPC methods
// ---------------------------------------------------------------------------

// SendNotification allows tmux sessions and external Claude processes to send notifications.
// Enforces localhost-only restriction and rate limiting. Accepts both managed sessions
// and external sessions (e.g., Claude running in IntelliJ, VS Code, or other terminals).
func (ns *NotificationService) SendNotification(
	ctx context.Context,
	req *connect.Request[sessionv1.SendNotificationRequest],
) (*connect.Response[sessionv1.SendNotificationResponse], error) {
	// Validate localhost-only origin
	if err := validateLocalhostOrigin(ctx, req); err != nil {
		return nil, err
	}

	// Validate required fields
	if req.Msg.SessionId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_id is required"))
	}
	if req.Msg.Title == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("title is required"))
	}

	// Use the session ID as the display name. LoadInstances() cannot be used here because
	// it calls FromInstanceData() which calls Start() on every non-paused session --
	// a catastrophic side-effect that restarts all sessions on each notification.
	// The poller holds live instances; if the session exists there, use its title.
	sessionName := req.Msg.SessionId // Default to session ID
	if ns.reviewQueuePoller != nil {
		if inst := ns.reviewQueuePoller.FindInstance(req.Msg.SessionId); inst != nil {
			sessionName = inst.Title
		}
	}

	// Apply rate limiting (applies to both managed and external sessions)
	if !ns.notificationRateLimiter.Allow(req.Msg.SessionId) {
		return nil, connect.NewError(connect.CodeResourceExhausted, fmt.Errorf("rate limit exceeded for session: %s", req.Msg.SessionId))
	}

	// Generate notification ID
	notificationID := uuid.New().String()

	// Broadcast notification via event bus
	event := events.NewNotificationEvent(
		req.Msg.SessionId,
		sessionName,
		notificationID,
		int32(req.Msg.NotificationType),
		int32(req.Msg.Priority),
		req.Msg.Title,
		req.Msg.Message,
		req.Msg.Metadata,
	)
	ns.eventBus.Publish(event)

	log.InfoS("Notification sent", map[string]interface{}{
		"session_id":        req.Msg.SessionId,
		"session_name":      sessionName,
		"notification_type": req.Msg.NotificationType.String(),
		"priority":          req.Msg.Priority.String(),
		"title":             req.Msg.Title,
		"notification_id":   notificationID,
	})

	return connect.NewResponse(&sessionv1.SendNotificationResponse{
		Success:        true,
		Message:        "Notification sent successfully",
		NotificationId: notificationID,
	}), nil
}

// GetNotificationHistory returns persisted notification history with optional filtering.
func (ns *NotificationService) GetNotificationHistory(
	ctx context.Context,
	req *connect.Request[sessionv1.GetNotificationHistoryRequest],
) (*connect.Response[sessionv1.GetNotificationHistoryResponse], error) {
	if ns.notificationStore == nil {
		return connect.NewResponse(&sessionv1.GetNotificationHistoryResponse{
			Notifications: []*sessionv1.NotificationHistoryRecord{},
		}), nil
	}

	msg := req.Msg

	opts := notifications.ListOptions{}
	if msg.Limit != nil {
		opts.Limit = int(*msg.Limit)
	}
	if msg.Offset != nil {
		opts.Offset = int(*msg.Offset)
	}
	if msg.TypeFilter != nil {
		typeVal := int32(*msg.TypeFilter)
		opts.TypeFilter = &typeVal
	}
	if msg.SessionId != nil {
		opts.SessionID = *msg.SessionId
	}
	if msg.UnreadOnly != nil {
		opts.UnreadOnly = *msg.UnreadOnly
	}

	records, totalCount, err := ns.notificationStore.List(opts)
	if err != nil {
		log.ErrorLog.Printf("[NotificationHistory] Failed to list notifications: %v", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Convert internal records to proto records
	protoRecords := make([]*sessionv1.NotificationHistoryRecord, 0, len(records))
	for _, r := range records {
		protoRecords = append(protoRecords, recordToProto(r))
	}

	// Calculate hasMore based on offset + limit vs total
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := opts.Offset
	if offset < 0 {
		offset = 0
	}
	hasMore := (offset + limit) < totalCount

	unreadCount := ns.notificationStore.GetUnreadCount()

	return connect.NewResponse(&sessionv1.GetNotificationHistoryResponse{
		Notifications: protoRecords,
		TotalCount:    int32(totalCount),
		UnreadCount:   int32(unreadCount),
		HasMore:       hasMore,
	}), nil
}

// MarkNotificationRead marks specific notifications as read.
// If notification_ids is empty, marks all notifications as read.
func (ns *NotificationService) MarkNotificationRead(
	ctx context.Context,
	req *connect.Request[sessionv1.MarkNotificationReadRequest],
) (*connect.Response[sessionv1.MarkNotificationReadResponse], error) {
	if ns.notificationStore == nil {
		return connect.NewResponse(&sessionv1.MarkNotificationReadResponse{
			Success:     true,
			MarkedCount: 0,
		}), nil
	}

	ids := req.Msg.NotificationIds
	count, err := ns.notificationStore.MarkRead(ids)
	if err != nil {
		log.ErrorLog.Printf("[NotificationHistory] Failed to mark notifications read: %v", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&sessionv1.MarkNotificationReadResponse{
		Success:     true,
		MarkedCount: int32(count),
	}), nil
}

// ClearNotificationHistory removes notifications from the history.
func (ns *NotificationService) ClearNotificationHistory(
	ctx context.Context,
	req *connect.Request[sessionv1.ClearNotificationHistoryRequest],
) (*connect.Response[sessionv1.ClearNotificationHistoryResponse], error) {
	if ns.notificationStore == nil {
		return connect.NewResponse(&sessionv1.ClearNotificationHistoryResponse{
			Success:      true,
			ClearedCount: 0,
		}), nil
	}

	var before *time.Time
	if req.Msg.BeforeTimestamp != nil {
		t, err := time.Parse(time.RFC3339, *req.Msg.BeforeTimestamp)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		before = &t
	}

	count, err := ns.notificationStore.Clear(before)
	if err != nil {
		log.ErrorLog.Printf("[NotificationHistory] Failed to clear notifications: %v", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&sessionv1.ClearNotificationHistoryResponse{
		Success:      true,
		ClearedCount: int32(count),
	}), nil
}

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

// recordToProto converts a NotificationRecord to a protobuf NotificationHistoryRecord.
func recordToProto(r *notifications.NotificationRecord) *sessionv1.NotificationHistoryRecord {
	record := &sessionv1.NotificationHistoryRecord{
		Id:               r.ID,
		SessionId:        r.SessionID,
		SessionName:      r.SessionName,
		NotificationType: sessionv1.NotificationType(r.NotificationType),
		Priority:         sessionv1.NotificationPriority(r.Priority),
		Title:            r.Title,
		Message:          r.Message,
		Metadata:         r.Metadata,
		CreatedAt:        timestamppb.New(r.CreatedAt),
		IsRead:           r.IsRead,
		OccurrenceCount:  int32(r.OccurrenceCount),
	}

	if r.ReadAt != nil {
		record.ReadAt = timestamppb.New(*r.ReadAt)
	}

	if r.LastOccurredAt != nil {
		record.LastOccurredAt = timestamppb.New(*r.LastOccurredAt)
	}

	return record
}

// validateLocalhostOrigin ensures the request comes from localhost.
// This is a security measure to prevent external actors from sending notifications.
func validateLocalhostOrigin(ctx context.Context, req *connect.Request[sessionv1.SendNotificationRequest]) error {
	// Get peer address from request headers or context
	// ConnectRPC provides X-Forwarded-For or we can check the connection directly

	// Check X-Real-IP header first (if behind a proxy)
	realIP := req.Header().Get("X-Real-IP")
	if realIP != "" {
		if !isLocalhostIP(realIP) {
			return connect.NewError(connect.CodePermissionDenied, fmt.Errorf("notifications can only be sent from localhost"))
		}
		return nil
	}

	// Check X-Forwarded-For header
	forwardedFor := req.Header().Get("X-Forwarded-For")
	if forwardedFor != "" {
		// Take the first IP in the chain (original client)
		ips := strings.Split(forwardedFor, ",")
		if len(ips) > 0 {
			clientIP := strings.TrimSpace(ips[0])
			if !isLocalhostIP(clientIP) {
				return connect.NewError(connect.CodePermissionDenied, fmt.Errorf("notifications can only be sent from localhost"))
			}
			return nil
		}
	}

	// If no proxy headers, we're in direct connection mode
	// The server already binds to localhost, so requests reaching here are local
	// This is a defense-in-depth check
	return nil
}

// isLocalhostIP checks if the given IP string represents localhost.
func isLocalhostIP(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	return parsed.IsLoopback()
}
