package services

import (
	sessionv1 "claude-squad/gen/proto/go/session/v1"
	"claude-squad/log"
	"claude-squad/server/notifications"
	"connectrpc.com/connect"
	"context"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// GetNotificationHistory returns persisted notification history with optional filtering.
func (s *SessionService) GetNotificationHistory(
	ctx context.Context,
	req *connect.Request[sessionv1.GetNotificationHistoryRequest],
) (*connect.Response[sessionv1.GetNotificationHistoryResponse], error) {
	if s.notificationStore == nil {
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

	records, totalCount, err := s.notificationStore.List(opts)
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

	unreadCount := s.notificationStore.GetUnreadCount()

	return connect.NewResponse(&sessionv1.GetNotificationHistoryResponse{
		Notifications: protoRecords,
		TotalCount:    int32(totalCount),
		UnreadCount:   int32(unreadCount),
		HasMore:       hasMore,
	}), nil
}

// MarkNotificationRead marks specific notifications as read.
// If notification_ids is empty, marks all notifications as read.
func (s *SessionService) MarkNotificationRead(
	ctx context.Context,
	req *connect.Request[sessionv1.MarkNotificationReadRequest],
) (*connect.Response[sessionv1.MarkNotificationReadResponse], error) {
	if s.notificationStore == nil {
		return connect.NewResponse(&sessionv1.MarkNotificationReadResponse{
			Success:     true,
			MarkedCount: 0,
		}), nil
	}

	ids := req.Msg.NotificationIds
	count, err := s.notificationStore.MarkRead(ids)
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
func (s *SessionService) ClearNotificationHistory(
	ctx context.Context,
	req *connect.Request[sessionv1.ClearNotificationHistoryRequest],
) (*connect.Response[sessionv1.ClearNotificationHistoryResponse], error) {
	if s.notificationStore == nil {
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

	count, err := s.notificationStore.Clear(before)
	if err != nil {
		log.ErrorLog.Printf("[NotificationHistory] Failed to clear notifications: %v", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&sessionv1.ClearNotificationHistoryResponse{
		Success:      true,
		ClearedCount: int32(count),
	}), nil
}

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
	}

	if r.ReadAt != nil {
		record.ReadAt = timestamppb.New(*r.ReadAt)
	}

	return record
}
