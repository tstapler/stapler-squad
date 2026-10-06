package mcp

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"connectrpc.com/connect"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
)

type notificationHandlers struct {
	svc *services.SessionService
}

const (
	notificationTypePrefix     = "NOTIFICATION_TYPE_"
	notificationPriorityPrefix = "NOTIFICATION_PRIORITY_"
)

// notificationTypeNames is the enum surface get_notification_history's
// type_filter argument accepts, without the NOTIFICATION_TYPE_ prefix.
// Derived from the generated NotificationType_name map (skipping the
// UNSPECIFIED sentinel) rather than hand-maintained, so it can't silently
// drift out of sync when the proto enum gains a new value.
var notificationTypeNames = enumNamesWithoutPrefix(
	sessionv1.NotificationType_name, notificationTypePrefix,
	sessionv1.NotificationType_NOTIFICATION_TYPE_UNSPECIFIED.String())

// enumNamesWithoutPrefix strips prefix from every value in names except
// excludeFull (typically the enum's UNSPECIFIED sentinel), sorted for
// deterministic schema/error-message output — map iteration order is random.
func enumNamesWithoutPrefix(names map[int32]string, prefix, excludeFull string) []string {
	out := make([]string, 0, len(names))
	for _, full := range names {
		if full == excludeFull {
			continue
		}
		out = append(out, strings.TrimPrefix(full, prefix))
	}
	sort.Strings(out)
	return out
}

// parseNotificationTypeFilter maps a friendly type-filter string (with or
// without the NOTIFICATION_TYPE_ prefix) to sessionv1.NotificationType via
// the generated NotificationType_value map, rejecting an unrecognized value
// fast instead of silently resolving to NOTIFICATION_TYPE_UNSPECIFIED.
func parseNotificationTypeFilter(s string) (sessionv1.NotificationType, error) {
	name := s
	if !strings.HasPrefix(name, notificationTypePrefix) {
		name = notificationTypePrefix + name
	}
	v, ok := sessionv1.NotificationType_value[name]
	if !ok || sessionv1.NotificationType(v) == sessionv1.NotificationType_NOTIFICATION_TYPE_UNSPECIFIED {
		return 0, fmt.Errorf("invalid type_filter %q — must be one of: %s", s, strings.Join(notificationTypeNames, ", "))
	}
	return sessionv1.NotificationType(v), nil
}

// --- get_notification_history ---

func (h *notificationHandlers) getNotificationHistory(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()

	limit := 10
	if lf, ok := args["limit"].(float64); ok && lf > 0 {
		limit = int(lf)
	}
	if limit > 50 {
		limit = 50
	}
	limit32 := int32(limit)

	offset32 := int32(0)
	if of, ok := args["offset"].(float64); ok && of > 0 {
		offset32 = int32(of)
	}

	var typeFilter *sessionv1.NotificationType
	if tf, ok := args["type_filter"].(string); ok && tf != "" {
		parsed, err := parseNotificationTypeFilter(tf)
		if err != nil {
			return errResult(ErrInvalidArgument, err.Error(), ""), nil
		}
		typeFilter = &parsed
	}

	var sessionID *string
	if sid, ok := args["session_id"].(string); ok && sid != "" {
		sessionID = &sid
	}

	var unreadOnly *bool
	if uo, ok := args["unread_only"].(bool); ok {
		unreadOnly = &uo
	}

	if h.svc == nil {
		return errResult(ErrInternalError, "session service unavailable on this server configuration", ""), nil
	}

	resp, err := h.svc.GetNotificationHistory(ctx, connect.NewRequest(&sessionv1.GetNotificationHistoryRequest{
		Limit:      &limit32,
		Offset:     &offset32,
		TypeFilter: typeFilter,
		SessionId:  sessionID,
		UnreadOnly: unreadOnly,
	}))
	if err != nil {
		return errResult(ErrInternalError, fmt.Sprintf("failed to get notification history: %v", err), ""), nil
	}

	records := make([]NotificationRecordResult, len(resp.Msg.Notifications))
	for i, n := range resp.Msg.Notifications {
		var createdAt time.Time
		if n.CreatedAt != nil {
			createdAt = n.CreatedAt.AsTime()
		}
		records[i] = NotificationRecordResult{
			ID:        n.Id,
			SessionID: n.SessionId,
			Type:      strings.TrimPrefix(n.NotificationType.String(), notificationTypePrefix),
			Priority:  strings.TrimPrefix(n.Priority.String(), notificationPriorityPrefix),
			Title:     session.SanitizeForAgentContext(n.Title, 200),
			Message:   session.SanitizeForAgentContext(n.Message, 500),
			CreatedAt: createdAt,
			IsRead:    n.IsRead,
		}
	}

	return okResult(GetNotificationHistoryResult{
		MCPResult:     MCPResult{Success: true},
		Notifications: records,
		TotalCount:    int(resp.Msg.TotalCount),
		UnreadCount:   int(resp.Msg.UnreadCount),
		HasMore:       resp.Msg.HasMore,
	}), nil
}

// --- Registration ---

// registerNotificationTools registers the notification-history MCP tool.
// Not feature-flag-gated — no equivalent flag exists for notifications,
// matching search_sessions/list_sessions' existing ungated precedent.
func registerNotificationTools(s *mcpserver.MCPServer, h *notificationHandlers) {
	s.AddTool(
		mcpgo.NewTool("get_notification_history",
			mcpgo.WithDescription("Query notification history (approvals, errors, task completions, etc.) with the same filters the web UI supports. Default limit is 10 to avoid filling LLM context."),
			mcpgo.WithNumber("limit",
				mcpgo.Description("Max notifications per page (default 10, max 50)"),
				mcpgo.DefaultNumber(10),
				mcpgo.Min(1),
				mcpgo.Max(50),
			),
			mcpgo.WithNumber("offset",
				mcpgo.Description("Number of notifications to skip, for pagination"),
				mcpgo.DefaultNumber(0),
				mcpgo.Min(0),
			),
			mcpgo.WithString("type_filter",
				mcpgo.Description("Filter to a single notification type, e.g. TASK_COMPLETE or NOTIFICATION_TYPE_TASK_COMPLETE"),
				mcpgo.Enum(notificationTypeNames...),
			),
			mcpgo.WithString("session_id",
				mcpgo.Description("Filter to notifications for a single session"),
			),
			mcpgo.WithBoolean("unread_only",
				mcpgo.Description("Only return unread notifications"),
			),
		),
		h.getNotificationHistory,
	)
}
