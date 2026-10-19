package services

import (
	"context"

	"connectrpc.com/connect"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// InvokeNotificationAction applies one inline notification action. Until the
// dispatcher is wired it reports FLAG_OFF so no caller sees a side effect.
// +api: notification:invoke-action
// +api: InvokeNotificationAction
func (s *SessionService) InvokeNotificationAction(
	_ context.Context,
	_ *connect.Request[sessionv1.InvokeNotificationActionRequest],
) (*connect.Response[sessionv1.InvokeNotificationActionResponse], error) {
	return connect.NewResponse(&sessionv1.InvokeNotificationActionResponse{
		Outcome: sessionv1.ActionOutcome_ACTION_OUTCOME_FLAG_OFF,
		Detail:  flagOffMessage,
	}), nil
}
