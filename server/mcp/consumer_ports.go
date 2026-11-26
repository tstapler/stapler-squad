package mcp

import (
	"context"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	githubpkg "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/hostclaim"
	"github.com/tstapler/stapler-squad/session/instance"
)

// TerminalScrollback is the slice of the scrollback manager the terminal tools
// read. *scrollback.ScrollbackManager and a test fake are its two implementations.
type TerminalScrollback interface {
	CurrentSequence(sessionID string) uint64
	GetRecentBytes(sessionID string, bytes int64) ([]byte, error)
	// LastEntrySequence is the sequence of the last of the first limit retained entries; 0 if none or unreadable.
	LastEntrySequence(sessionID string, limit int) uint64
}

// AutoReopener transitions a backlog item back to in_progress and respawns work
// after a failed review verdict. *backlogsvc.BacklogService and a test fake are its
// two implementations.
type AutoReopener interface {
	AutoReopenAfterFailedReview(ctx context.Context, itemID string) error
}

// BacklogRPC is the slice of the backlog service the backlog, claim and stuck-report
// tools call. *backlogsvc.BacklogService and a test fake are its two implementations.
// It satisfies claimCheckService, which the claim tool takes as its own narrower port.
type BacklogRPC interface {
	LookupCrossHostClaim(ctx context.Context, externalURL string) (hostclaim.ClaimVerdict, error)
	MaybeTriggerTriage(ctx context.Context, itemID string, skipTriage bool, repoPath string) bool
	IsTriageLive(itemID string) bool
	EnterpriseHosts() []string
	BlockedCleanups() []session.BlockedItem
	ListBacklogItems(context.Context, *connect.Request[sessionv1.ListBacklogItemsRequest]) (*connect.Response[sessionv1.ListBacklogItemsResponse], error)
	AttachSessionToItem(context.Context, *connect.Request[sessionv1.AttachSessionToItemRequest]) (*connect.Response[sessionv1.AttachSessionToItemResponse], error)
	AddBacklogItemDependency(context.Context, *connect.Request[sessionv1.AddBacklogItemDependencyRequest]) (*connect.Response[sessionv1.AddBacklogItemDependencyResponse], error)
	RemoveBacklogItemDependency(context.Context, *connect.Request[sessionv1.RemoveBacklogItemDependencyRequest]) (*connect.Response[sessionv1.RemoveBacklogItemDependencyResponse], error)
	CreateEpicIntegrationChild(context.Context, *connect.Request[sessionv1.CreateEpicIntegrationChildRequest]) (*connect.Response[sessionv1.CreateEpicIntegrationChildResponse], error)
	TriggerTriage(context.Context, *connect.Request[sessionv1.TriggerTriageRequest]) (*connect.Response[sessionv1.TriggerTriageResponse], error)
	TransitionBacklogItemStatus(context.Context, *connect.Request[sessionv1.TransitionBacklogItemStatusRequest]) (*connect.Response[sessionv1.TransitionBacklogItemStatusResponse], error)
}

// FlowService is the slice of the flow-observability service the flow tools
// call. *backlogflow.BacklogFlowService and a test fake are its two
// implementations.
type FlowService interface {
	GetFlowSummary(context.Context, *connect.Request[sessionv1.GetFlowSummaryRequest]) (*connect.Response[sessionv1.GetFlowSummaryResponse], error)
	ListFlowDrilldown(context.Context, *connect.Request[sessionv1.ListFlowDrilldownRequest]) (*connect.Response[sessionv1.ListFlowDrilldownResponse], error)
	CompareFlowWindows(context.Context, *connect.Request[sessionv1.CompareFlowWindowsRequest]) (*connect.Response[sessionv1.CompareFlowWindowsResponse], error)
	ListDeadlockCandidates(context.Context, *connect.Request[sessionv1.ListDeadlockCandidatesRequest]) (*connect.Response[sessionv1.ListDeadlockCandidatesResponse], error)
	ListThrashEpisodes(context.Context, *connect.Request[sessionv1.ListThrashEpisodesRequest]) (*connect.Response[sessionv1.ListThrashEpisodesResponse], error)
	GetItemFlowTimeline(context.Context, *connect.Request[sessionv1.GetItemFlowTimelineRequest]) (*connect.Response[sessionv1.GetItemFlowTimelineResponse], error)
}

// NudgeGate is the write-time safety check the diagnose nudge runs on its
// target (idle eligibility, then pane-ownership re-verification). The real one
// waits on a live pane, so a recording fake is the genuine test seam.
type NudgeGate interface {
	VerifyNudgeSafeToWrite(ctx context.Context, target *instance.Instance) error
}

// UserPRSource is the slice of the user PR cache the GitHub PR tools read.
// *githubpkg.UserPRCache and a test fake are its two implementations.
type UserPRSource interface {
	GetAll() []githubpkg.UserPR
	GetCachedLogins() []string
}
