package mcp

import (
	"context"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/instance"
)

// GoalStore is the slice of *session.Storage the goal and workspace-peer tools call.
// *session.Storage and a recording test fake are its two implementations.
type GoalStore interface {
	SetSessionGoal(ctx context.Context, sessionUUID, goal, status string, tasks []instance.TaskNode, setBy, workspaceKey string) (*instance.SessionGoalData, error)
	GetSessionGoal(ctx context.Context, sessionUUID string) (*instance.SessionGoalData, error)
	UpdateSessionTaskStatus(ctx context.Context, sessionUUID, taskID, newStatus string) (*instance.SessionGoalData, error)
	ListWorkspacePeers(ctx context.Context, workspaceKey, excludeSessionUUID string) ([]session.WorkspacePeer, error)
}

var _ GoalStore = (*session.Storage)(nil)
