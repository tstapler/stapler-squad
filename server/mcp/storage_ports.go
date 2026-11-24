package mcp

import (
	"context"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/itemlink"
	"github.com/tstapler/stapler-squad/session/workspacepeers"
)

// GoalStore is the slice of *session.Storage the goal tools call.
// *session.Storage and a recording test fake are its two implementations.
type GoalStore interface {
	SetSessionGoal(ctx context.Context, sessionUUID, goal, status string, tasks []instance.TaskNode, setBy, workspaceKey string) (*instance.SessionGoalData, error)
	GetSessionGoal(ctx context.Context, sessionUUID string) (*instance.SessionGoalData, error)
	UpdateSessionTaskStatus(ctx context.Context, sessionUUID, taskID, newStatus string) (*instance.SessionGoalData, error)
}

var _ GoalStore = (*session.Storage)(nil)

// PeerLister is the workspace-peer query behind list_workspace_peers.
// *workspacepeers.Lister and a recording test fake are its two implementations.
type PeerLister interface {
	ListWorkspacePeers(ctx context.Context, workspaceKey, excludeSessionUUID string) ([]workspacepeers.WorkspacePeer, error)
}

var _ PeerLister = (*workspacepeers.Lister)(nil)

// GuidanceStore is the slice of *session.Storage the guidance, request_help and
// request_owner_action tools call: the guidance-request writes and reads, plus
// the item lookups behind itemlink.ResolveItemLink and the owner-action item
// check. *session.Storage and a recording test fake are its two implementations.
type GuidanceStore interface {
	itemlink.ItemLinkStore
	CreateGuidanceRequest(ctx context.Context, in domain.CreateGuidanceRequestInput) (*domain.GuidanceRequestData, error)
	CreateMirroredGuidanceRequest(ctx context.Context, req domain.GuidanceRequest, capLimit int) (*domain.GuidanceRequestData, error)
	GetGuidanceRequest(ctx context.Context, id uuid.UUID) (*domain.GuidanceRequestData, error)
	CancelGuidanceRequest(ctx context.Context, id uuid.UUID, reason string) (bool, error)
}

var _ GuidanceStore = (*session.Storage)(nil)
